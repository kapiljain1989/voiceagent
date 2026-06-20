package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type Tenant struct {
	ID                 string         `json:"id"`
	Name               string         `json:"name"`
	Domain             string         `json:"domain,omitempty"`
	Settings           map[string]any `json:"settings,omitempty"`
	MaxAgents          int            `json:"max_agents"`
	MaxConcurrentCalls int            `json:"max_concurrent_calls"`
	Enabled            bool           `json:"enabled"`
	CreatedAt          string         `json:"created_at,omitempty"`
}

// GetTenantIDForUser looks up the tenant_id for a user by their user_id.
func GetTenantIDForUser(ctx context.Context, userID string) string {
	if database == nil || userID == "" {
		return ""
	}
	var tenantID *string
	database.DB().QueryRowContext(ctx,
		`SELECT tenant_id FROM users WHERE id=$1`, userID).Scan(&tenantID)
	if tenantID != nil {
		return *tenantID
	}
	return ""
}

func (gw *gateway) registerTenantRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/tenants", gw.handleTenants)
	mux.HandleFunc("/api/tenants/users", gw.handleTenantUsers)
}

func (gw *gateway) handleTenants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		if database == nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rows, err := database.DB().QueryContext(ctx,
			`SELECT id, name, COALESCE(domain,''), COALESCE(settings,'{}'),
				max_agents, max_concurrent_calls, enabled, created_at
			FROM tenants ORDER BY created_at`)
		if err != nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		defer rows.Close()

		var tenants []Tenant
		for rows.Next() {
			var t Tenant
			var settingsJSON []byte
			var created time.Time
			rows.Scan(&t.ID, &t.Name, &t.Domain, &settingsJSON,
				&t.MaxAgents, &t.MaxConcurrentCalls, &t.Enabled, &created)
			json.Unmarshal(settingsJSON, &t.Settings)
			t.CreatedAt = created.Format(time.RFC3339)
			tenants = append(tenants, t)
		}
		if tenants == nil {
			tenants = []Tenant{}
		}

		// Count agents and active calls per tenant
		type tenantStats struct {
			Agents int `json:"agents"`
			Calls  int `json:"calls"`
		}
		stats := make(map[string]tenantStats)
		agentRows, _ := database.DB().QueryContext(ctx,
			`SELECT COALESCE(tenant_id::text,''), COUNT(*) FROM agents GROUP BY tenant_id`)
		if agentRows != nil {
			for agentRows.Next() {
				var tid string
				var count int
				agentRows.Scan(&tid, &count)
				s := stats[tid]
				s.Agents = count
				stats[tid] = s
			}
			agentRows.Close()
		}

		var result []map[string]any
		for _, t := range tenants {
			s := stats[t.ID]
			result = append(result, map[string]any{
				"id": t.ID, "name": t.Name, "domain": t.Domain,
				"max_agents": t.MaxAgents, "max_concurrent_calls": t.MaxConcurrentCalls,
				"enabled": t.Enabled, "created_at": t.CreatedAt,
				"agents": s.Agents, "active_calls": s.Calls,
			})
		}
		if result == nil {
			result = []map[string]any{}
		}
		writeJSON(w, http.StatusOK, result)

	case "POST":
		var req struct {
			Name               string `json:"name"`
			Domain             string `json:"domain"`
			MaxAgents          int    `json:"max_agents"`
			MaxConcurrentCalls int    `json:"max_concurrent_calls"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
			return
		}
		if req.MaxAgents == 0 {
			req.MaxAgents = 50
		}
		if req.MaxConcurrentCalls == 0 {
			req.MaxConcurrentCalls = 20
		}

		if database == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no database"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var id string
		err := database.DB().QueryRowContext(ctx,
			`INSERT INTO tenants (name, domain, max_agents, max_concurrent_calls)
			VALUES ($1, $2, $3, $4) RETURNING id`,
			req.Name, req.Domain, req.MaxAgents, req.MaxConcurrentCalls).Scan(&id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		slog.Info("tenant created", "id", id, "name", req.Name)
		writeJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "created"})

	case "PUT":
		var req struct {
			ID                 string `json:"id"`
			Name               string `json:"name"`
			Domain             string `json:"domain"`
			MaxAgents          int    `json:"max_agents"`
			MaxConcurrentCalls int    `json:"max_concurrent_calls"`
			Enabled            *bool  `json:"enabled"`
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
		database.DB().ExecContext(ctx,
			`UPDATE tenants SET
				name=COALESCE(NULLIF($2,''),name),
				domain=COALESCE(NULLIF($3,''),domain),
				max_agents=CASE WHEN $4>0 THEN $4 ELSE max_agents END,
				max_concurrent_calls=CASE WHEN $5>0 THEN $5 ELSE max_concurrent_calls END,
				enabled=COALESCE($6, enabled)
			WHERE id=$1`,
			req.ID, req.Name, req.Domain, req.MaxAgents, req.MaxConcurrentCalls, req.Enabled)
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
			database.DB().ExecContext(ctx, `DELETE FROM tenants WHERE id=$1`, id)
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (gw *gateway) handleTenantUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID   string `json:"user_id"`
		TenantID string `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.UserID == "" || req.TenantID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id and tenant_id required"})
		return
	}

	if database == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no database"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Assign user to tenant
	database.DB().ExecContext(ctx, `UPDATE users SET tenant_id=$1 WHERE id=$2`, req.TenantID, req.UserID)
	// Also assign their agent profile
	database.DB().ExecContext(ctx, `UPDATE agents SET tenant_id=$1 WHERE user_id=$2`, req.TenantID, req.UserID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}
