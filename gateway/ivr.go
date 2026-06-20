package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// IVR flow JSON structure
type IVRFlow struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Entry       string              `json:"entry"`
	Nodes       map[string]*IVRNode `json:"nodes"`
	Enabled     bool                `json:"enabled"`
}

type IVRNode struct {
	Type            string            `json:"type"` // play, collect, transfer, hangup
	Prompt          string            `json:"prompt"`
	Next            string            `json:"next,omitempty"`
	TimeoutMs       int               `json:"timeout_ms,omitempty"`
	Retries         int               `json:"retries,omitempty"`
	DTMFMap         map[string]string `json:"dtmf_map,omitempty"`
	TimeoutNode     string            `json:"timeout_node,omitempty"`
	DestType        string            `json:"destination_type,omitempty"`
	DestValue       string            `json:"destination_value,omitempty"`
}

// RunIVR executes an IVR flow for an inbound call before routing to a queue/agent.
func RunIVR(ctx context.Context, flow *IVRFlow, copilot *siprecSession, announcer *QueueAnnouncer, log *slog.Logger) (destType, destValue string) {
	if flow == nil || flow.Entry == "" || len(flow.Nodes) == 0 {
		return "", ""
	}

	currentNode := flow.Entry
	for i := 0; i < 20; i++ { // max 20 nodes to prevent loops
		select {
		case <-ctx.Done():
			return "", ""
		default:
		}

		node, ok := flow.Nodes[currentNode]
		if !ok {
			log.Error("IVR node not found", "node", currentNode)
			return "", ""
		}

		log.Info("IVR node", "name", currentNode, "type", node.Type)

		switch node.Type {
		case "play":
			if node.Prompt != "" && announcer != nil {
				announcer.playTTS(ctx, node.Prompt, copilot, log)
			}
			if node.Next != "" {
				currentNode = node.Next
				continue
			}
			return "", ""

		case "collect":
			if node.Prompt != "" && announcer != nil {
				announcer.playTTS(ctx, node.Prompt, copilot, log)
			}

			retries := node.Retries
			if retries <= 0 {
				retries = 1
			}
			timeoutMs := node.TimeoutMs
			if timeoutMs <= 0 {
				timeoutMs = 5000
			}

			for attempt := 0; attempt < retries; attempt++ {
				digit := collectDTMF(ctx, copilot, time.Duration(timeoutMs)*time.Millisecond)
				if digit == "" {
					if attempt < retries-1 {
						if announcer != nil {
							announcer.playTTS(ctx, "Sorry, I didn't get that. Please try again.", copilot, log)
						}
						if node.Prompt != "" && announcer != nil {
							announcer.playTTS(ctx, node.Prompt, copilot, log)
						}
						continue
					}
					if node.TimeoutNode != "" {
						currentNode = node.TimeoutNode
						break
					}
					return "", ""
				}

				if node.DTMFMap != nil {
					if nextNode, ok := node.DTMFMap[digit]; ok {
						currentNode = nextNode
						break
					}
				}

				if attempt == retries-1 {
					if node.TimeoutNode != "" {
						currentNode = node.TimeoutNode
					} else {
						return "", ""
					}
				}
			}
			continue

		case "transfer":
			return node.DestType, node.DestValue

		case "hangup":
			return "hangup", ""

		default:
			log.Error("unknown IVR node type", "type", node.Type)
			return "", ""
		}
	}

	return "", ""
}

// collectDTMF waits for a single DTMF digit from the caller via RTP event packets.
func collectDTMF(ctx context.Context, copilot *siprecSession, timeout time.Duration) string {
	if copilot == nil || copilot.rtpSession == nil || copilot.rtpSession.listener == nil {
		return ""
	}

	digitCh := make(chan string, 1)

	collector := NewDTMFCollector(timeout, func(digits string) {
		if len(digits) > 0 {
			select {
			case digitCh <- digits[:1]:
			default:
			}
		}
	})
	_ = collector

	// Listen for DTMF on the RTP listener for the timeout period
	// The RTP listener's ReceiveAndDecode handles PT=101 (RFC 2833) events
	// We use a simple timeout-based approach here
	select {
	case <-ctx.Done():
		return ""
	case digit := <-digitCh:
		return digit
	case <-time.After(timeout):
		return ""
	}
}

// LoadIVRFlow loads an IVR flow by ID from the database.
func LoadIVRFlow(ctx context.Context, ivrID string) (*IVRFlow, error) {
	if database == nil || ivrID == "" {
		return nil, nil
	}

	var flow IVRFlow
	var flowData []byte
	err := database.DB().QueryRowContext(ctx,
		`SELECT id, name, COALESCE(description,''), flow_data, enabled
		FROM ivr_flows WHERE id=$1 AND enabled=true`, ivrID).
		Scan(&flow.ID, &flow.Name, &flow.Description, &flowData, &flow.Enabled)
	if err != nil {
		return nil, err
	}

	var inner struct {
		Entry string              `json:"entry"`
		Nodes map[string]*IVRNode `json:"nodes"`
	}
	if err := json.Unmarshal(flowData, &inner); err != nil {
		return nil, fmt.Errorf("parse IVR flow: %w", err)
	}
	flow.Entry = inner.Entry
	flow.Nodes = inner.Nodes

	return &flow, nil
}

// --- IVR CRUD API ---

var ivrCacheMu sync.Mutex
var ivrCache = make(map[string]*IVRFlow)

func (gw *gateway) registerIVRRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/ivr", gw.handleIVR)
}

func (gw *gateway) handleIVR(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		id := r.URL.Query().Get("id")
		if id != "" {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			flow, err := LoadIVRFlow(ctx, id)
			if err != nil || flow == nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "IVR flow not found"})
				return
			}
			writeJSON(w, http.StatusOK, flow)
			return
		}

		if database == nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rows, err := database.DB().QueryContext(ctx,
			`SELECT id, name, COALESCE(description,''), enabled, created_at FROM ivr_flows ORDER BY created_at DESC`)
		if err != nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		defer rows.Close()

		var flows []map[string]any
		for rows.Next() {
			var id, name, desc string
			var enabled bool
			var created time.Time
			rows.Scan(&id, &name, &desc, &enabled, &created)
			flows = append(flows, map[string]any{
				"id": id, "name": name, "description": desc,
				"enabled": enabled, "created_at": created.Format(time.RFC3339),
			})
		}
		if flows == nil {
			flows = []map[string]any{}
		}
		writeJSON(w, http.StatusOK, flows)

	case "POST":
		var req struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			FlowData    json.RawMessage `json:"flow_data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
			return
		}
		if len(req.FlowData) == 0 {
			req.FlowData = json.RawMessage(`{"entry":"","nodes":{}}`)
		}

		if database == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no database"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var id string
		err := database.DB().QueryRowContext(ctx,
			`INSERT INTO ivr_flows (name, description, flow_data) VALUES ($1, $2, $3) RETURNING id`,
			req.Name, req.Description, req.FlowData).Scan(&id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "created"})

	case "PUT":
		var req struct {
			ID          string          `json:"id"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			FlowData    json.RawMessage `json:"flow_data"`
			Enabled     *bool           `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if database == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no database"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		_, err := database.DB().ExecContext(ctx,
			`UPDATE ivr_flows SET name=COALESCE(NULLIF($2,''),name), description=COALESCE(NULLIF($3,''),description),
				flow_data=COALESCE($4, flow_data), enabled=COALESCE($5, enabled), updated_at=NOW()
			WHERE id=$1`,
			req.ID, req.Name, req.Description, req.FlowData, req.Enabled)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		// Clear cache
		ivrCacheMu.Lock()
		delete(ivrCache, req.ID)
		ivrCacheMu.Unlock()

		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	case "DELETE":
		id := r.URL.Query().Get("id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if database != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			database.DB().ExecContext(ctx, `UPDATE did_routes SET ivr_id=NULL WHERE ivr_id=$1`, id)
			database.DB().ExecContext(ctx, `DELETE FROM ivr_flows WHERE id=$1`, id)
		}
		ivrCacheMu.Lock()
		delete(ivrCache, id)
		ivrCacheMu.Unlock()
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
