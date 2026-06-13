package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type CallRouter struct {
	db *sql.DB
}

type QueueDef struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	SkillsRequired []string `json:"skills_required"`
	MaxWaitSeconds int      `json:"max_wait_seconds"`
	OverflowQueue  string   `json:"overflow_queue"`
	CallerCount    int      `json:"caller_count"`
}

type AgentScore struct {
	AgentID   string  `json:"agent_id"`
	Name      string  `json:"name"`
	Extension string  `json:"extension"`
	Score     float64 `json:"score"`
	Reason    string  `json:"reason"`
}

func NewCallRouter(db *sql.DB) *CallRouter {
	return &CallRouter{db: db}
}

func (r *CallRouter) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/queues/list", r.handleListQueues)
	mux.HandleFunc("/api/queues/create", r.handleCreateQueue)
	mux.HandleFunc("/api/queues/agents", r.handleQueueAgents)
	mux.HandleFunc("/api/routing/test", r.handleTestRouting)
}

// RouteCall finds the best agent for a call based on intent and caller info
func (r *CallRouter) RouteCall(ctx context.Context, intent string, language string) (*AgentScore, string, error) {
	if r.db == nil {
		return nil, "", nil
	}

	// Find matching queue
	queueName := r.matchQueue(ctx, intent)
	if queueName == "" {
		queueName = "Support"
	}

	// Score available agents in that queue
	agents, err := r.scoreAgents(ctx, queueName, intent, language)
	if err != nil || len(agents) == 0 {
		return nil, queueName, err
	}

	return &agents[0], queueName, nil
}

func (r *CallRouter) matchQueue(ctx context.Context, intent string) string {
	rows, err := r.db.QueryContext(ctx,
		`SELECT name, skills_required FROM queues ORDER BY name`)
	if err != nil {
		return "Support"
	}
	defer rows.Close()

	intentLower := strings.ToLower(intent)
	bestQueue := "Support"
	bestScore := 0

	for rows.Next() {
		var name string
		var skills []string
		rows.Scan(&name, &skills)

		score := 0
		for _, skill := range skills {
			if strings.Contains(intentLower, strings.ToLower(skill)) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestQueue = name
		}
	}
	return bestQueue
}

func (r *CallRouter) scoreAgents(ctx context.Context, queueName, intent, language string) ([]AgentScore, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT a.id, a.name, a.extension, a.expertise, a.languages, a.priority,
			a.current_calls, a.max_calls, a.status
		FROM agents a
		JOIN agent_queues aq ON a.id = aq.agent_id
		JOIN queues q ON q.id = aq.queue_id
		WHERE q.name = $1 AND a.status = 'Available' AND a.current_calls < a.max_calls
		ORDER BY a.priority DESC, a.current_calls ASC`, queueName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	intentLower := strings.ToLower(intent)
	langLower := strings.ToLower(language)
	if langLower == "" {
		langLower = "english"
	}

	var scored []AgentScore
	for rows.Next() {
		var id, name, status string
		var extension sql.NullString
		var expertise, languages []string
		var priority, currentCalls, maxCalls int

		rows.Scan(&id, &name, &extension, &expertise, &languages, &priority, &currentCalls, &maxCalls, &status)

		score := 0.0
		var reasons []string

		// Skill match
		for _, skill := range expertise {
			if strings.Contains(intentLower, strings.ToLower(skill)) {
				score += 10
				reasons = append(reasons, "skill:"+skill)
			}
		}

		// Language match
		for _, lang := range languages {
			if strings.Contains(langLower, strings.ToLower(lang)) {
				score += 20
				reasons = append(reasons, "language:"+lang)
			}
		}

		// Priority tier bonus
		score += float64(priority) * 5
		if priority > 1 {
			reasons = append(reasons, "senior")
		}

		// Load penalty
		score -= float64(currentCalls) * 10
		if currentCalls > 0 {
			reasons = append(reasons, "busy")
		}

		scored = append(scored, AgentScore{
			AgentID:   id,
			Name:      name,
			Extension: extension.String,
			Score:     score,
			Reason:    strings.Join(reasons, ", "),
		})
	}

	// Sort by score descending
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].Score > scored[i].Score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	return scored, nil
}

// --- API Handlers ---

func (r *CallRouter) handleListQueues(w http.ResponseWriter, req *http.Request) {
	if r.db == nil {
		writeJSON(w, http.StatusOK, []QueueDef{})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := r.db.QueryContext(ctx, `
		SELECT q.id, q.name, q.description, q.skills_required, q.max_wait_seconds,
			COALESCE(q.overflow_queue, ''),
			(SELECT COUNT(*) FROM queue_entries WHERE queue_name=q.name AND status='waiting')
		FROM queues q ORDER BY q.name`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var queues []QueueDef
	for rows.Next() {
		var q QueueDef
		rows.Scan(&q.ID, &q.Name, &q.Description, &q.SkillsRequired, &q.MaxWaitSeconds, &q.OverflowQueue, &q.CallerCount)
		queues = append(queues, q)
	}
	writeJSON(w, http.StatusOK, queues)
}

func (r *CallRouter) handleCreateQueue(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if r.db == nil {
		http.Error(w, "database required", http.StatusServiceUnavailable)
		return
	}

	var q QueueDef
	if err := json.NewDecoder(req.Body).Decode(&q); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO queues (name, description, skills_required, max_wait_seconds, overflow_queue)
		VALUES ($1,$2,$3,$4,$5) ON CONFLICT (name) DO UPDATE SET description=$2, skills_required=$3`,
		q.Name, q.Description, q.SkillsRequired, q.MaxWaitSeconds, q.OverflowQueue)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("queue created", "name", q.Name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "created"})
}

func (r *CallRouter) handleQueueAgents(w http.ResponseWriter, req *http.Request) {
	queueName := req.URL.Query().Get("queue")
	if queueName == "" {
		http.Error(w, "queue parameter required", http.StatusBadRequest)
		return
	}
	if r.db == nil {
		writeJSON(w, http.StatusOK, []AgentScore{})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agents, err := r.scoreAgents(ctx, queueName, "", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

func (r *CallRouter) handleTestRouting(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Intent   string `json:"intent"`
		Language string `json:"language"`
	}
	json.NewDecoder(req.Body).Decode(&body)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agent, queue, err := r.RouteCall(ctx, body.Intent, body.Language)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"queue": queue,
		"agent": agent,
	})
}
