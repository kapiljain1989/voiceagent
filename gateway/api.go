package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// -------------------------------------------------------------------
// REST API for the call center UI
//
// All endpoints are prefixed with /api/ and return JSON.
// PostgreSQL is optional — when DB_URL is not set, endpoints return
// mock data to allow the UI to function without a database.
// -------------------------------------------------------------------

type APIHandler struct {
	gw  *gateway
	db  *sql.DB
	rag *RAGClient
	llm *LLMProvider
}

func NewAPIHandler(gw *gateway) *APIHandler {
	h := &APIHandler{gw: gw}

	if gw.cfg.DBURL != "" {
		db, err := sql.Open("pgx", gw.cfg.DBURL)
		if err != nil {
			slog.Warn("database not available, using mock data", "err", err)
		} else {
			h.db = db
			h.initDB()
		}
	}

	if gw.cfg.ChromaURL != "" {
		h.rag = NewRAGClient(gw.cfg.ChromaURL)
		h.rag.EnsureCollection(context.Background())
	}

	h.llm = NewLLMProvider(gw.gcpCreds, gw.cfg.GCPProjectID, gw.cfg.GCPRegion)

	return h
}

func (h *APIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/agents", h.handleAgents)
	mux.HandleFunc("/api/calls", h.handleCalls)
	mux.HandleFunc("/api/calls/active", h.handleActiveCalls)
	mux.HandleFunc("/api/documents", h.handleDocuments)
	mux.HandleFunc("/api/documents/search", h.handleDocSearch)
	mux.HandleFunc("/api/llm/configs", h.handleLLMConfigs)
	mux.HandleFunc("/api/llm/test", h.handleLLMTest)
	mux.HandleFunc("/api/stats", h.handleStats)
}

func (h *APIHandler) initDB() {
	if h.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schema := `
	CREATE TABLE IF NOT EXISTS agents (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name TEXT NOT NULL,
		email TEXT UNIQUE NOT NULL,
		phone TEXT,
		expertise TEXT[] DEFAULT '{}',
		status TEXT DEFAULT 'available',
		max_calls INT DEFAULT 3,
		active_calls INT DEFAULT 0,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMPTZ DEFAULT NOW()
	);
	CREATE TABLE IF NOT EXISTS calls (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		caller_number TEXT,
		called_number TEXT,
		agent_id UUID REFERENCES agents(id),
		mode TEXT,
		status TEXT,
		start_time TIMESTAMPTZ DEFAULT NOW(),
		end_time TIMESTAMPTZ,
		duration INT,
		summary TEXT,
		sentiment TEXT,
		action_items TEXT[] DEFAULT '{}',
		commitments TEXT[] DEFAULT '{}',
		transcript JSONB,
		suggestions JSONB,
		llm_model TEXT DEFAULT 'claude-3-5-haiku@20241022',
		created_at TIMESTAMPTZ DEFAULT NOW()
	);
	CREATE TABLE IF NOT EXISTS documents (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name TEXT NOT NULL,
		type TEXT,
		size INT,
		category TEXT,
		chroma_id TEXT,
		chunks INT DEFAULT 0,
		status TEXT DEFAULT 'processing',
		uploaded_at TIMESTAMPTZ DEFAULT NOW()
	);
	CREATE TABLE IF NOT EXISTS llm_configs (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name TEXT NOT NULL,
		provider TEXT,
		model TEXT,
		region TEXT,
		is_default BOOLEAN DEFAULT FALSE,
		system_prompt TEXT,
		max_tokens INT DEFAULT 512,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);`

	if _, err := h.db.ExecContext(ctx, schema); err != nil {
		slog.Error("db schema init", "err", err)
	} else {
		slog.Info("database schema initialized")
	}
}

// -------------------------------------------------------------------
// Agents CRUD
// -------------------------------------------------------------------

func (h *APIHandler) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		h.listAgents(w, r)
	case "POST":
		h.createAgent(w, r)
	case "PUT":
		h.updateAgent(w, r)
	case "DELETE":
		h.deleteAgent(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *APIHandler) listAgents(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		writeJSON(w, http.StatusOK, []map[string]any{
			{"id": "1", "name": "Priya Sharma", "email": "priya@co.com", "expertise": []string{"billing", "retention"}, "status": "available", "activeCalls": 1, "maxCalls": 3},
			{"id": "2", "name": "Raj Patel", "email": "raj@co.com", "expertise": []string{"technical"}, "status": "busy", "activeCalls": 3, "maxCalls": 3},
		})
		return
	}

	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, name, COALESCE(email,''), COALESCE(phone,''), expertise, status,
			max_calls, active_calls, COALESCE(extension,''), COALESCE(department,'Support'),
			languages, COALESCE(priority,1), COALESCE(current_calls,0)
		FROM agents ORDER BY
			CASE status WHEN 'Available' THEN 0 WHEN 'Busy' THEN 1 WHEN 'On Break' THEN 2 WHEN 'Wrap-up' THEN 3 ELSE 4 END,
			name`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var agents []map[string]any
	for rows.Next() {
		var id, name, email, phone, status, extension, department string
		var expertiseStr, languagesStr string
		var maxCalls, activeCalls, priority, currentCalls int
		if err := rows.Scan(&id, &name, &email, &phone, &expertiseStr, &status,
			&maxCalls, &activeCalls, &extension, &department,
			&languagesStr, &priority, &currentCalls); err != nil {
			slog.Error("scan agent", "err", err)
			continue
		}
		agents = append(agents, map[string]any{
			"id": id, "name": name, "email": email, "phone": phone,
			"expertise": parsePostgresArray(expertiseStr), "status": status,
			"max_calls": maxCalls, "active_calls": currentCalls,
			"extension": extension, "department": department,
			"languages": parsePostgresArray(languagesStr), "priority": priority,
		})
	}
	if agents == nil {
		agents = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, agents)
}

func (h *APIHandler) createAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string   `json:"name"`
		Email     string   `json:"email"`
		Phone     string   `json:"phone"`
		Expertise []string `json:"expertise"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if h.db == nil {
		writeJSON(w, http.StatusOK, map[string]any{"id": "mock-id", "name": req.Name, "status": "created"})
		return
	}

	var id string
	err := h.db.QueryRowContext(r.Context(),
		"INSERT INTO agents (name, email, phone, expertise) VALUES ($1, $2, $3, $4) RETURNING id",
		req.Name, req.Email, req.Phone, req.Expertise).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "created"})
}

func (h *APIHandler) updateAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID         string   `json:"id"`
		Name       string   `json:"name"`
		Email      string   `json:"email"`
		Phone      string   `json:"phone"`
		Extension  string   `json:"extension"`
		Department string   `json:"department"`
		Expertise  []string `json:"expertise"`
		Languages  []string `json:"languages"`
		Priority   int      `json:"priority"`
		MaxCalls   int      `json:"max_calls"`
		Status     string   `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}
	if h.db == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
		return
	}

	_, err := h.db.ExecContext(r.Context(), `
		UPDATE agents SET
			name=COALESCE(NULLIF($2,''),name),
			email=COALESCE(NULLIF($3,''),email),
			phone=COALESCE(NULLIF($4,''),phone),
			extension=COALESCE(NULLIF($5,''),extension),
			department=COALESCE(NULLIF($6,''),department),
			expertise=CASE WHEN $7::text[] IS NOT NULL AND array_length($7::text[],1)>0 THEN $7 ELSE expertise END,
			languages=CASE WHEN $8::text[] IS NOT NULL AND array_length($8::text[],1)>0 THEN $8 ELSE languages END,
			priority=CASE WHEN $9>0 THEN $9 ELSE priority END,
			max_calls=CASE WHEN $10>0 THEN $10 ELSE max_calls END,
			status=COALESCE(NULLIF($11,''),status),
			updated_at=NOW()
		WHERE id=$1`,
		req.ID, req.Name, req.Email, req.Phone, req.Extension, req.Department,
		req.Expertise, req.Languages, req.Priority, req.MaxCalls, req.Status)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *APIHandler) deleteAgent(w http.ResponseWriter, r *http.Request) {
	var req struct{ ID string `json:"id"` }
	json.NewDecoder(r.Body).Decode(&req)
	if req.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}
	if h.db != nil {
		h.db.ExecContext(r.Context(), "DELETE FROM agents WHERE id=$1", req.ID)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// -------------------------------------------------------------------
// Calls
// -------------------------------------------------------------------

func (h *APIHandler) handleCalls(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		writeJSON(w, http.StatusOK, []map[string]any{
			{"id": "c1", "callerNumber": "+1555999001", "calledNumber": "1000", "mode": "interactive", "status": "completed", "duration": 312, "sentiment": "positive", "summary": "Billing inquiry resolved"},
		})
		return
	}

	rows, err := h.db.QueryContext(r.Context(),
		"SELECT id, caller_number, called_number, mode, status, duration, sentiment, summary, start_time FROM calls ORDER BY start_time DESC LIMIT 50")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var calls []map[string]any
	for rows.Next() {
		var id, callerNum, calledNum, mode, status string
		var duration sql.NullInt64
		var sentiment, summary sql.NullString
		var startTime time.Time
		rows.Scan(&id, &callerNum, &calledNum, &mode, &status, &duration, &sentiment, &summary, &startTime)
		calls = append(calls, map[string]any{
			"id": id, "callerNumber": callerNum, "calledNumber": calledNum,
			"mode": mode, "status": status, "duration": duration.Int64,
			"sentiment": sentiment.String, "summary": summary.String,
			"startTime": startTime,
		})
	}
	if calls == nil {
		calls = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, calls)
}

func (h *APIHandler) handleActiveCalls(w http.ResponseWriter, r *http.Request) {
	active := h.gw.sessions.Load()

	siprecSessionsMu.Lock()
	copilot := len(siprecSessions)
	siprecSessionsMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"interactive": active,
		"copilot":     copilot,
		"total":       int64(copilot) + active,
	})
}

// -------------------------------------------------------------------
// Documents + RAG
// -------------------------------------------------------------------

func (h *APIHandler) handleDocuments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		h.listDocuments(w, r)
	case "POST":
		h.uploadDocument(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *APIHandler) listDocuments(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		writeJSON(w, http.StatusOK, []map[string]any{
			{"id": "d1", "name": "Insurance Policy v4.2", "type": "pdf", "size": 2400000, "category": "policy", "chunks": 48, "status": "indexed"},
		})
		return
	}

	rows, err := h.db.QueryContext(r.Context(),
		"SELECT id, name, type, size, category, chunks, status FROM documents ORDER BY uploaded_at DESC")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var docs []map[string]any
	for rows.Next() {
		var id, name, typ, category, status string
		var size, chunks int
		rows.Scan(&id, &name, &typ, &size, &category, &chunks, &status)
		docs = append(docs, map[string]any{
			"id": id, "name": name, "type": typ, "size": size,
			"category": category, "chunks": chunks, "status": status,
		})
	}
	if docs == nil {
		docs = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, docs)
}

func (h *APIHandler) uploadDocument(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Category string `json:"category"`
		Content  string `json:"content"`
	}
	b, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(b, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	docID := fmt.Sprintf("doc_%d", time.Now().UnixMilli())
	chunks := 0

	if h.rag != nil {
		var err error
		chunks, err = h.rag.IndexDocument(r.Context(), docID, req.Name, req.Content, 500)
		if err != nil {
			slog.Error("rag index", "err", err)
		}
	}

	if h.db != nil {
		h.db.ExecContext(r.Context(),
			"INSERT INTO documents (id, name, type, size, category, chroma_id, chunks, status) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)",
			docID, req.Name, "txt", len(req.Content), req.Category, docID, chunks, "indexed")
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id": docID, "name": req.Name, "chunks": chunks, "status": "indexed",
	})
}

func (h *APIHandler) handleDocSearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.TopK <= 0 {
		req.TopK = 3
	}

	if h.rag == nil {
		writeJSON(w, http.StatusOK, []RAGChunk{})
		return
	}

	chunks, err := h.rag.Query(r.Context(), req.Query, req.TopK)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, chunks)
}

// -------------------------------------------------------------------
// LLM Configs
// -------------------------------------------------------------------

func (h *APIHandler) handleLLMConfigs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		if h.db == nil {
			writeJSON(w, http.StatusOK, []map[string]any{
				{"id": "l1", "name": "Claude Haiku", "provider": "anthropic-vertex", "model": "claude-3-5-haiku@20241022", "region": "us-east5", "isDefault": true, "maxTokens": 512},
				{"id": "l2", "name": "Gemini Flash", "provider": "gemini-vertex", "model": "gemini-2.0-flash", "region": "us-central1", "isDefault": false, "maxTokens": 1024},
			})
			return
		}
		rows, err := h.db.QueryContext(r.Context(),
			"SELECT id, name, provider, model, region, is_default, max_tokens FROM llm_configs ORDER BY created_at")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		defer rows.Close()
		var configs []map[string]any
		for rows.Next() {
			var id, name, provider, model, region string
			var isDefault bool
			var maxTokens int
			rows.Scan(&id, &name, &provider, &model, &region, &isDefault, &maxTokens)
			configs = append(configs, map[string]any{
				"id": id, "name": name, "provider": provider, "model": model,
				"region": region, "isDefault": isDefault, "maxTokens": maxTokens,
			})
		}
		if configs == nil {
			configs = []map[string]any{}
		}
		writeJSON(w, http.StatusOK, configs)

	case "POST":
		var req struct {
			Name     string `json:"name"`
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Region   string `json:"region"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if h.db != nil {
			h.db.ExecContext(r.Context(),
				"INSERT INTO llm_configs (name, provider, model, region) VALUES ($1,$2,$3,$4)",
				req.Name, req.Provider, req.Model, req.Region)
		}
		writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *APIHandler) handleLLMTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Prompt   string `json:"prompt"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.Prompt == "" {
		req.Prompt = "Say hello in one sentence."
	}

	client := h.llm.Client(req.Provider, req.Model)
	start := time.Now()
	response, err := client.Chat(r.Context(),
		[]LLMMessage{{Role: "user", Content: req.Prompt}},
		"You are a helpful assistant.", 256)
	elapsed := time.Since(start)

	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":   err.Error(),
			"latency": elapsed.Milliseconds(),
			"model":   client.Name(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"response": response,
		"latency":  elapsed.Milliseconds(),
		"model":    client.Name(),
	})
}

// -------------------------------------------------------------------
// Dashboard Stats
// -------------------------------------------------------------------

func (h *APIHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	active := h.gw.sessions.Load()

	siprecSessionsMu.Lock()
	copilot := len(siprecSessions)
	siprecSessionsMu.Unlock()

	stats := map[string]any{
		"activeCalls": int64(copilot) + active,
		"totalToday":  0,
		"avgDuration": 0,
		"sentimentBreakdown": map[string]int{
			"positive": 0,
			"neutral":  0,
			"negative": 0,
		},
	}

	if h.db != nil {
		today := time.Now().Truncate(24 * time.Hour)
		var totalToday, avgDuration int
		h.db.QueryRowContext(r.Context(),
			"SELECT COUNT(*) FROM calls WHERE start_time >= $1", today).Scan(&totalToday)
		h.db.QueryRowContext(r.Context(),
			"SELECT COALESCE(AVG(duration), 0) FROM calls WHERE start_time >= $1 AND duration IS NOT NULL", today).Scan(&avgDuration)
		stats["totalToday"] = totalToday
		stats["avgDuration"] = avgDuration

		var pos, neu, neg int
		h.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM calls WHERE sentiment='positive' AND start_time >= $1", today).Scan(&pos)
		h.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM calls WHERE sentiment='neutral' AND start_time >= $1", today).Scan(&neu)
		h.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM calls WHERE sentiment='negative' AND start_time >= $1", today).Scan(&neg)
		stats["sentimentBreakdown"] = map[string]int{"positive": pos, "neutral": neu, "negative": neg}
	}

	writeJSON(w, http.StatusOK, stats)
}

// SaveCall stores a completed call in the database.
func (h *APIHandler) SaveCall(ctx context.Context, call map[string]any) {
	if h.db == nil {
		return
	}
	_, err := h.db.ExecContext(ctx,
		`INSERT INTO calls (id, caller_number, called_number, mode, status, duration, summary, sentiment, action_items, commitments, transcript, suggestions, llm_model)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		call["id"], call["callerNumber"], call["calledNumber"], call["mode"], call["status"],
		call["duration"], call["summary"], call["sentiment"], call["actionItems"], call["commitments"],
		call["transcript"], call["suggestions"], call["llmModel"])
	if err != nil {
		slog.Error("save call", "err", err)
	}
}

// GetRAGContext retrieves relevant document context for a query.
func (h *APIHandler) GetRAGContext(ctx context.Context, query string) string {
	if h.rag == nil {
		return ""
	}
	return h.rag.BuildRAGContext(ctx, query, 3)
}
