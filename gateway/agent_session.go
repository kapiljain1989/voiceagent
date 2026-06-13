package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)


type AgentProfile struct {
	ID            string   `json:"id"`
	UserID        string   `json:"user_id"`
	Name          string   `json:"name"`
	Email         string   `json:"email"`
	Extension     string   `json:"extension"`
	Department    string   `json:"department"`
	Expertise     []string `json:"expertise"`
	Languages     []string `json:"languages"`
	Priority      int      `json:"priority"`
	MaxCalls      int      `json:"max_calls"`
	CurrentCalls  int      `json:"current_calls"`
	Status        string   `json:"status"`
	CustomerTiers []string `json:"customer_tiers"`
	Queues        []string `json:"queues"`
}

type AgentSessionManager struct {
	db       *sql.DB
	sessions map[string]*AgentOnlineSession // agentID → session
	mu       sync.RWMutex
}

type AgentOnlineSession struct {
	AgentID   string
	UserID    string
	Status    string
	SSEChans  []chan []byte
	LoginTime time.Time
}

func NewAgentSessionManager(db *sql.DB) *AgentSessionManager {
	return &AgentSessionManager{
		db:       db,
		sessions: make(map[string]*AgentOnlineSession),
	}
}

func (m *AgentSessionManager) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/agent/me", m.handleAgentMe)
	mux.HandleFunc("/api/agent/me/status", m.handleUpdateStatus)
	mux.HandleFunc("/api/agent/me/queues", m.handleMyQueues)
	mux.HandleFunc("/api/agent/me/events", m.handleAgentSSE)
	mux.HandleFunc("/api/agents/online", m.handleOnlineAgents)
	mux.HandleFunc("/api/agents/assign-queue", m.handleAssignQueue)
	mux.HandleFunc("/api/agents/link-user", m.handleLinkUser)
}

// Get agent profile for the logged-in user
func (m *AgentSessionManager) handleAgentMe(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromRequest(r)
	if userID == "" {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	profile, err := m.getAgentByUserID(r.Context(), userID)
	if err != nil || profile == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"linked": false,
			"message": "No agent profile linked to this user account. Ask an admin to link your account.",
		})
		return
	}

	// Mark agent as online
	m.mu.Lock()
	if _, ok := m.sessions[profile.ID]; !ok {
		m.sessions[profile.ID] = &AgentOnlineSession{
			AgentID:   profile.ID,
			UserID:    userID,
			Status:    "Available",
			LoginTime: time.Now(),
		}
		if m.db != nil {
			m.db.ExecContext(r.Context(), `UPDATE agents SET status='Available' WHERE id=$1`, profile.ID)
		}
	}
	m.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"linked":  true,
		"profile": profile,
	})
}

func (m *AgentSessionManager) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	userID := getUserIDFromRequest(r)
	profile, _ := m.getAgentByUserID(r.Context(), userID)
	if profile == nil {
		http.Error(w, "no agent profile", http.StatusNotFound)
		return
	}

	var body struct{ Status string `json:"status"` }
	json.NewDecoder(r.Body).Decode(&body)

	validStatuses := map[string]bool{"Available": true, "Busy": true, "On Break": true, "Wrap-up": true, "Offline": true}
	if !validStatuses[body.Status] {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}

	if m.db != nil {
		m.db.ExecContext(r.Context(), `UPDATE agents SET status=$1, updated_at=NOW() WHERE id=$2`, body.Status, profile.ID)
	}

	m.mu.Lock()
	if sess, ok := m.sessions[profile.ID]; ok {
		sess.Status = body.Status
	}
	m.mu.Unlock()

	slog.Info("agent status changed", "agent", profile.Name, "status", body.Status)
	writeJSON(w, http.StatusOK, map[string]string{"status": body.Status})
}

func (m *AgentSessionManager) handleMyQueues(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromRequest(r)
	profile, _ := m.getAgentByUserID(r.Context(), userID)
	if profile == nil {
		writeJSON(w, http.StatusOK, []string{})
		return
	}
	writeJSON(w, http.StatusOK, profile.Queues)
}

func (m *AgentSessionManager) handleAgentSSE(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromRequest(r)
	profile, _ := m.getAgentByUserID(r.Context(), userID)
	if profile == nil {
		http.Error(w, "no agent profile", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan []byte, 10)

	m.mu.Lock()
	if sess, ok := m.sessions[profile.ID]; ok {
		sess.SSEChans = append(sess.SSEChans, ch)
	}
	m.mu.Unlock()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			w.Write([]byte("data: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}

// Send a ring event to a specific agent
func (m *AgentSessionManager) RingAgent(agentID, callID, callerNumber, queueName string) {
	m.mu.RLock()
	sess, ok := m.sessions[agentID]
	m.mu.RUnlock()

	if !ok {
		return
	}

	evt, _ := json.Marshal(map[string]string{
		"type":   "ring",
		"call_id": callID,
		"caller": callerNumber,
		"queue":  queueName,
	})

	for _, ch := range sess.SSEChans {
		select {
		case ch <- evt:
		default:
		}
	}
}

func (m *AgentSessionManager) handleOnlineAgents(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	var online []map[string]any
	for id, sess := range m.sessions {
		online = append(online, map[string]any{
			"agent_id": id,
			"status":   sess.Status,
			"since":    sess.LoginTime,
		})
	}
	m.mu.RUnlock()
	writeJSON(w, http.StatusOK, online)
}

func (m *AgentSessionManager) handleAssignQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		AgentID string `json:"agent_id"`
		Queue   string `json:"queue"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	if m.db == nil {
		http.Error(w, "database required", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := m.db.ExecContext(ctx, `
		INSERT INTO agent_queues (agent_id, queue_id)
		SELECT $1, id FROM queues WHERE name=$2
		ON CONFLICT DO NOTHING`, body.AgentID, body.Queue)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}

func (m *AgentSessionManager) handleLinkUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		AgentID string `json:"agent_id"`
		UserID  string `json:"user_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	if m.db == nil {
		http.Error(w, "database required", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := m.db.ExecContext(ctx, `UPDATE agents SET user_id=$1 WHERE id=$2`, body.UserID, body.AgentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "linked"})
}

// --- Helpers ---

func (m *AgentSessionManager) getAgentByUserID(ctx context.Context, userID string) (*AgentProfile, error) {
	if m.db == nil || userID == "" {
		return nil, nil
	}

	var p AgentProfile

	err := m.db.QueryRowContext(ctx, `
		SELECT id, COALESCE(user_id::text,''), name, COALESCE(email,''), COALESCE(department,'Support'),
			COALESCE(expertise, '{}'), COALESCE(status, 'Available')
		FROM agents WHERE user_id=$1`, userID).
		Scan(&p.ID, &p.UserID, &p.Name, &p.Email, &p.Department, &p.Expertise, &p.Status)

	// Try loading extended fields (may not exist in old schema)
	m.db.QueryRowContext(ctx, `
		SELECT COALESCE(extension,''), COALESCE(languages,'{English}'), COALESCE(priority,1),
			COALESCE(max_calls,3), COALESCE(current_calls,0), COALESCE(customer_tiers,'{standard}')
		FROM agents WHERE id=$1`, p.ID).
		Scan(&p.Extension, &p.Languages, &p.Priority, &p.MaxCalls, &p.CurrentCalls, &p.CustomerTiers)
	// Load queues
	rows, err := m.db.QueryContext(ctx, `
		SELECT q.name FROM queues q
		JOIN agent_queues aq ON q.id = aq.queue_id
		WHERE aq.agent_id = $1`, p.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var q string
			rows.Scan(&q)
			p.Queues = append(p.Queues, q)
		}
	}

	return &p, nil
}

func getUserIDFromRequest(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")

	// Token format: hex(json_payload).signature
	parts := strings.SplitN(token, ".", 2)
	if len(parts) < 1 {
		return ""
	}

	decoded, err := hex.DecodeString(parts[0])
	if err != nil {
		return ""
	}

	var claims struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return ""
	}
	return claims.UserID
}
