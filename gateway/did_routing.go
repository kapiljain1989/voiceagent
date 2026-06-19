package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

type DIDRoute struct {
	ID                  string          `json:"id"`
	DIDPattern          string          `json:"did_pattern"`
	MatchType           string          `json:"match_type"` // exact, prefix, regex
	TrunkID             *string         `json:"trunk_id"`
	DestinationType     string          `json:"destination_type"` // queue, agent, forward
	DestinationValue    string          `json:"destination_value"`
	Priority            int             `json:"priority"`
	TimeCondition       json.RawMessage `json:"time_condition,omitempty"`
	OverflowDestination string          `json:"overflow_destination,omitempty"`
	Enabled             bool            `json:"enabled"`
	CreatedAt           string          `json:"created_at,omitempty"`
}

type DIDRouter struct {
	db     *sql.DB
	routes []DIDRoute
	mu     sync.RWMutex
}

func NewDIDRouter(db *sql.DB) *DIDRouter {
	r := &DIDRouter{db: db}
	if db != nil {
		r.loadRoutes()
	}
	return r
}

func (r *DIDRouter) loadRoutes() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, did_pattern, match_type, trunk_id, destination_type, destination_value,
		        priority, time_condition, COALESCE(overflow_destination,''), enabled, created_at
		 FROM did_routes WHERE enabled=true ORDER BY priority DESC, created_at ASC`)
	if err != nil {
		slog.Error("load DID routes", "err", err)
		return
	}
	defer rows.Close()

	var routes []DIDRoute
	for rows.Next() {
		var rt DIDRoute
		var trunkID sql.NullString
		var timeCond sql.NullString
		rows.Scan(&rt.ID, &rt.DIDPattern, &rt.MatchType, &trunkID,
			&rt.DestinationType, &rt.DestinationValue,
			&rt.Priority, &timeCond, &rt.OverflowDestination, &rt.Enabled, &rt.CreatedAt)
		if trunkID.Valid {
			rt.TrunkID = &trunkID.String
		}
		if timeCond.Valid && timeCond.String != "" {
			rt.TimeCondition = json.RawMessage(timeCond.String)
		}
		routes = append(routes, rt)
	}

	r.mu.Lock()
	r.routes = routes
	r.mu.Unlock()
	slog.Info("DID routes loaded", "count", len(routes))
}

func (r *DIDRouter) Reload() {
	r.loadRoutes()
}

// MatchDID finds the best route for a dialed number.
// Returns destination type and value, or empty strings if no match.
func (r *DIDRouter) MatchDID(dialedNumber string, trunkID string) (destType, destValue, overflow string) {
	r.mu.RLock()
	routes := r.routes
	r.mu.RUnlock()

	dialedNumber = strings.TrimSpace(dialedNumber)

	for _, rt := range routes {
		// Check trunk scope
		if rt.TrunkID != nil && *rt.TrunkID != trunkID {
			continue
		}

		// Check time condition
		if len(rt.TimeCondition) > 0 && !r.matchTimeCondition(rt.TimeCondition) {
			continue
		}

		// Match pattern
		if r.matchPattern(dialedNumber, rt.DIDPattern, rt.MatchType) {
			return rt.DestinationType, rt.DestinationValue, rt.OverflowDestination
		}
	}

	return "", "", ""
}

func (r *DIDRouter) matchPattern(number, pattern, matchType string) bool {
	switch matchType {
	case "exact":
		return number == pattern
	case "prefix":
		if pattern == "*" {
			return true
		}
		cleanPattern := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(number, cleanPattern)
	case "regex":
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(number)
	default:
		return number == pattern
	}
}

func (r *DIDRouter) matchTimeCondition(condJSON json.RawMessage) bool {
	var cond struct {
		Days  string `json:"days"`  // "mon-fri" or "mon,wed,fri"
		Start string `json:"start"` // "09:00"
		End   string `json:"end"`   // "17:00"
	}
	if json.Unmarshal(condJSON, &cond) != nil {
		return true // invalid condition = always match
	}

	now := time.Now()
	dayNames := map[time.Weekday]string{
		time.Monday: "mon", time.Tuesday: "tue", time.Wednesday: "wed",
		time.Thursday: "thu", time.Friday: "fri", time.Saturday: "sat", time.Sunday: "sun",
	}
	today := dayNames[now.Weekday()]

	// Check day
	if cond.Days != "" {
		dayMatch := false
		if strings.Contains(cond.Days, "-") {
			parts := strings.SplitN(cond.Days, "-", 2)
			dayOrder := []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
			startIdx, endIdx, todayIdx := -1, -1, -1
			for i, d := range dayOrder {
				if d == parts[0] {
					startIdx = i
				}
				if d == parts[1] {
					endIdx = i
				}
				if d == today {
					todayIdx = i
				}
			}
			dayMatch = todayIdx >= startIdx && todayIdx <= endIdx
		} else {
			for _, d := range strings.Split(cond.Days, ",") {
				if strings.TrimSpace(d) == today {
					dayMatch = true
					break
				}
			}
		}
		if !dayMatch {
			return false
		}
	}

	// Check time
	if cond.Start != "" && cond.End != "" {
		nowTime := now.Format("15:04")
		if nowTime < cond.Start || nowTime >= cond.End {
			return false
		}
	}

	return true
}

// RegisterRoutes adds DID routing API endpoints
func (r *DIDRouter) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/routing/dids", r.handleDIDs)
}

func (r *DIDRouter) handleDIDs(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		r.mu.RLock()
		routes := r.routes
		r.mu.RUnlock()
		if routes == nil {
			routes = []DIDRoute{}
		}
		writeJSON(w, http.StatusOK, routes)

	case "POST":
		var rt DIDRoute
		if err := json.NewDecoder(req.Body).Decode(&rt); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		if rt.DIDPattern == "" || rt.DestinationType == "" || rt.DestinationValue == "" {
			http.Error(w, `{"error":"did_pattern, destination_type, destination_value required"}`, http.StatusBadRequest)
			return
		}
		if rt.MatchType == "" {
			rt.MatchType = "exact"
		}

		if r.db == nil {
			writeJSON(w, http.StatusOK, map[string]string{"id": "no-db", "status": "created"})
			return
		}

		var id string
		var timeCond *string
		if len(rt.TimeCondition) > 0 {
			s := string(rt.TimeCondition)
			timeCond = &s
		}
		err := r.db.QueryRowContext(req.Context(),
			`INSERT INTO did_routes (did_pattern, match_type, trunk_id, destination_type, destination_value, priority, time_condition, overflow_destination, enabled)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
			rt.DIDPattern, rt.MatchType, rt.TrunkID, rt.DestinationType, rt.DestinationValue,
			rt.Priority, timeCond, rt.OverflowDestination, true).Scan(&id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		r.loadRoutes()
		slog.Info("DID route created", "id", id, "pattern", rt.DIDPattern, "dest", rt.DestinationType+":"+rt.DestinationValue)
		writeJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "created"})

	case "DELETE":
		var body struct {
			ID string `json:"id"`
		}
		json.NewDecoder(req.Body).Decode(&body)
		if body.ID == "" {
			http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
			return
		}
		if r.db != nil {
			r.db.ExecContext(req.Context(), `DELETE FROM did_routes WHERE id=$1`, body.ID)
			r.loadRoutes()
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
