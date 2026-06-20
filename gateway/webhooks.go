package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type Webhook struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	URL        string   `json:"url"`
	Method     string   `json:"method"`
	Headers    map[string]string `json:"headers"`
	AuthType   string   `json:"auth_type"`
	AuthValue  string   `json:"auth_value,omitempty"`
	Events     []string `json:"events"`
	Enabled    bool     `json:"enabled"`
	RetryCount int      `json:"retry_count"`
}

type WebhookManager struct {
	webhooks []Webhook
	mu       sync.RWMutex
	gw       *gateway
}

func NewWebhookManager(gw *gateway) *WebhookManager {
	wm := &WebhookManager{gw: gw}
	wm.loadWebhooks()
	return wm
}

func (wm *WebhookManager) loadWebhooks() {
	if database == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := database.DB().QueryContext(ctx,
		`SELECT id, name, url, COALESCE(method,'POST'), COALESCE(headers,'{}'),
			COALESCE(auth_type,'none'), COALESCE(auth_value,''), events, enabled,
			COALESCE(retry_count,3)
		FROM webhooks WHERE enabled=true`)
	if err != nil {
		slog.Error("load webhooks", "err", err)
		return
	}
	defer rows.Close()

	var hooks []Webhook
	for rows.Next() {
		var w Webhook
		var headersJSON []byte
		var eventsStr string
		err := rows.Scan(&w.ID, &w.Name, &w.URL, &w.Method, &headersJSON,
			&w.AuthType, &w.AuthValue, &eventsStr, &w.Enabled, &w.RetryCount)
		if err != nil {
			slog.Debug("scan webhook", "err", err)
			continue
		}
		json.Unmarshal(headersJSON, &w.Headers)
		w.Events = parsePostgresArray(eventsStr)
		hooks = append(hooks, w)
	}

	wm.mu.Lock()
	wm.webhooks = hooks
	wm.mu.Unlock()
	slog.Info("webhooks loaded", "count", len(hooks))
}

func (wm *WebhookManager) FireEvent(eventType string, payload map[string]any) {
	wm.mu.RLock()
	hooks := make([]Webhook, len(wm.webhooks))
	copy(hooks, wm.webhooks)
	wm.mu.RUnlock()

	payload["event"] = eventType
	payload["timestamp"] = time.Now().Format(time.RFC3339)

	for _, hook := range hooks {
		if !hook.Enabled {
			continue
		}
		if len(hook.Events) > 0 && !sliceContains(hook.Events, eventType) {
			continue
		}
		go wm.sendWebhook(hook, eventType, payload)
	}
}

func (wm *WebhookManager) sendWebhook(hook Webhook, eventType string, payload map[string]any) {
	body, _ := json.Marshal(payload)
	retries := hook.RetryCount
	if retries <= 0 {
		retries = 1
	}

	var lastErr string
	var lastStatus int

	for attempt := 0; attempt < retries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			time.Sleep(backoff)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		req, err := http.NewRequestWithContext(ctx, hook.Method, hook.URL, bytes.NewReader(body))
		if err != nil {
			cancel()
			lastErr = err.Error()
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		for k, v := range hook.Headers {
			req.Header.Set(k, v)
		}

		switch hook.AuthType {
		case "bearer":
			req.Header.Set("Authorization", "Bearer "+hook.AuthValue)
		case "basic":
			req.Header.Set("Authorization", "Basic "+hook.AuthValue)
		case "api_key":
			req.Header.Set("X-API-Key", hook.AuthValue)
		}

		resp, err := http.DefaultClient.Do(req)
		cancel()
		if err != nil {
			lastErr = err.Error()
			lastStatus = 0
			continue
		}

		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		lastStatus = resp.StatusCode
		lastErr = ""

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			slog.Info("webhook sent", "name", hook.Name, "event", eventType, "status", resp.StatusCode)
			wm.logWebhook(hook.ID, eventType, payload["call_id"], lastStatus, string(respBody), "")
			return
		}

		lastErr = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	slog.Error("webhook failed after retries", "name", hook.Name, "event", eventType,
		"status", lastStatus, "err", lastErr, "retries", retries)
	callID, _ := payload["call_id"].(string)
	wm.logWebhook(hook.ID, eventType, callID, lastStatus, "", lastErr)
}

func (wm *WebhookManager) logWebhook(webhookID, eventType string, callID any, statusCode int, respBody, errMsg string) {
	if database == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	database.DB().ExecContext(ctx,
		`INSERT INTO webhook_logs (webhook_id, event_type, call_id, status_code, response_body, error)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		webhookID, eventType, callID, statusCode, respBody, errMsg)
}

func sliceContains(arr []string, val string) bool {
	for _, a := range arr {
		if a == val {
			return true
		}
	}
	return false
}

// --- API Handlers ---

func (gw *gateway) registerWebhookRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/webhooks", gw.handleWebhooks)
	mux.HandleFunc("/api/webhooks/test", gw.handleWebhookTest)
	mux.HandleFunc("/api/webhooks/logs", gw.handleWebhookLogs)
}

func (gw *gateway) handleWebhooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		if database == nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rows, err := database.DB().QueryContext(ctx,
			`SELECT id, name, url, COALESCE(method,'POST'), COALESCE(auth_type,'none'),
				events, enabled, COALESCE(retry_count,3), created_at
			FROM webhooks ORDER BY created_at DESC`)
		if err != nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		defer rows.Close()
		var hooks []map[string]any
		for rows.Next() {
			var id, name, url, method, authType, eventsStr string
			var enabled bool
			var retryCount int
			var createdAt time.Time
			rows.Scan(&id, &name, &url, &method, &authType, &eventsStr, &enabled, &retryCount, &createdAt)
			events := parsePostgresArray(eventsStr)
			hooks = append(hooks, map[string]any{
				"id": id, "name": name, "url": url, "method": method,
				"auth_type": authType, "events": events, "enabled": enabled,
				"retry_count": retryCount, "created_at": createdAt.Format(time.RFC3339),
			})
		}
		if hooks == nil {
			hooks = []map[string]any{}
		}
		writeJSON(w, http.StatusOK, hooks)

	case "POST":
		var hook Webhook
		if err := json.NewDecoder(r.Body).Decode(&hook); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if hook.Name == "" || hook.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and url required"})
			return
		}
		if hook.Method == "" {
			hook.Method = "POST"
		}
		if hook.RetryCount == 0 {
			hook.RetryCount = 3
		}

		if database == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no database"})
			return
		}

		headersJSON, _ := json.Marshal(hook.Headers)
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		eventsArr := arrayToPostgres(hook.Events)
		var id string
		err := database.DB().QueryRowContext(ctx,
			`INSERT INTO webhooks (name, url, method, headers, auth_type, auth_value, events, enabled, retry_count)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
			hook.Name, hook.URL, hook.Method, headersJSON, hook.AuthType, hook.AuthValue,
			eventsArr, true, hook.RetryCount).Scan(&id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if gw.webhookMgr != nil {
			gw.webhookMgr.loadWebhooks()
		}

		writeJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "created"})

	case "DELETE":
		id := r.URL.Query().Get("id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if database != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			database.DB().ExecContext(ctx, `DELETE FROM webhooks WHERE id=$1`, id)
		}
		if gw.webhookMgr != nil {
			gw.webhookMgr.loadWebhooks()
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (gw *gateway) handleWebhookTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		WebhookID string `json:"webhook_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if gw.webhookMgr != nil {
		gw.webhookMgr.FireEvent("test", map[string]any{
			"call_id":  "test-000",
			"caller":   "+15551234567",
			"agent":    "TestAgent",
			"duration": 120,
			"sentiment": "positive",
			"summary":  "This is a test webhook payload.",
		})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "test_sent"})
}

func (gw *gateway) handleWebhookLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	if database == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	rows, err := database.DB().QueryContext(ctx,
		`SELECT wl.id, wl.event_type, wl.call_id, wl.status_code, COALESCE(wl.error,''),
			wl.sent_at, w.name
		FROM webhook_logs wl
		JOIN webhooks w ON w.id = wl.webhook_id
		ORDER BY wl.sent_at DESC LIMIT 50`)
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	defer rows.Close()

	var logs []map[string]any
	for rows.Next() {
		var id, eventType, callID, errMsg, webhookName string
		var statusCode int
		var sentAt time.Time
		rows.Scan(&id, &eventType, &callID, &statusCode, &errMsg, &sentAt, &webhookName)
		logs = append(logs, map[string]any{
			"id": id, "event_type": eventType, "call_id": callID,
			"status_code": statusCode, "error": errMsg,
			"sent_at": sentAt.Format(time.RFC3339), "webhook_name": webhookName,
		})
	}
	if logs == nil {
		logs = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, logs)
}
