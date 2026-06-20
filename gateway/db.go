package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Database struct {
	conn *sql.DB
}

func NewDatabase(databaseURL string) (*Database, error) {
	if databaseURL == "" {
		return nil, nil
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Database{conn: db}, nil
}

func (d *Database) RunMigrations() error {
	if d == nil || d.conn == nil {
		return nil
	}

	// Read and execute migration files in order
	files := []string{
		"migrations/001_initial_schema.up.sql",
		"migrations/002_seed_data.up.sql",
		"migrations/003_custom_security_rules.up.sql",
		"migrations/004_agent_routing.up.sql",
		"migrations/005_sip_security.up.sql",
		"migrations/006_did_routing.up.sql",
		"migrations/007_call_recordings.up.sql",
	}

	for _, f := range files {
		data, err := migrationsFS.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", f, err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, err = d.conn.ExecContext(ctx, string(data))
		cancel()

		if err != nil {
			slog.Warn("migration warning", "file", f, "err", err)
		}
	}

	slog.Info("database migrations complete")
	return nil
}

func (d *Database) DB() *sql.DB {
	if d == nil {
		return nil
	}
	return d.conn
}

// --- Calls ---

type CallRecord struct {
	ID              string          `json:"id"`
	CallerNumber    string          `json:"caller_number"`
	CalledNumber    string          `json:"called_number"`
	AgentID         *string         `json:"agent_id,omitempty"`
	Mode            string          `json:"mode"`
	Status          string          `json:"status"`
	StartTime       time.Time       `json:"start_time"`
	EndTime         *time.Time      `json:"end_time,omitempty"`
	Duration        int             `json:"duration"`
	Summary         string          `json:"summary"`
	Sentiment       string          `json:"sentiment"`
	ActionItems     []string        `json:"action_items"`
	Commitments     []string        `json:"commitments"`
	Transcript      json.RawMessage `json:"transcript"`
	Suggestions     json.RawMessage `json:"suggestions"`
	VoiceSentiment  json.RawMessage `json:"voice_sentiment,omitempty"`
	RobocallScore   float64         `json:"robocall_score"`
	RobocallCategory string         `json:"robocall_category"`
	PIIDetected     bool            `json:"pii_detected"`
	LLMModel        string          `json:"llm_model"`
}

func (d *Database) SaveCall(ctx context.Context, c *CallRecord) error {
	if d == nil {
		return nil
	}
	_, err := d.conn.ExecContext(ctx, `
		INSERT INTO calls (id, caller_number, called_number, agent_id, mode, status,
			start_time, end_time, duration, summary, sentiment, action_items, commitments,
			transcript, suggestions, voice_sentiment, robocall_score, robocall_category,
			pii_detected, llm_model)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		ON CONFLICT (id) DO UPDATE SET
			end_time=$8, duration=$9, summary=$10, sentiment=$11, action_items=$12,
			commitments=$13, transcript=$14, suggestions=$15, voice_sentiment=$16,
			status=$6`,
		c.ID, c.CallerNumber, c.CalledNumber, c.AgentID, c.Mode, c.Status,
		c.StartTime, c.EndTime, c.Duration, c.Summary, c.Sentiment,
		c.ActionItems, c.Commitments, c.Transcript, c.Suggestions,
		c.VoiceSentiment, c.RobocallScore, c.RobocallCategory,
		c.PIIDetected, c.LLMModel,
	)
	return err
}

func (d *Database) ListCalls(ctx context.Context, limit int) ([]CallRecord, error) {
	if d == nil {
		return nil, nil
	}
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, caller_number, called_number, mode, status, start_time,
			end_time, duration, summary, sentiment, llm_model
		FROM calls ORDER BY start_time DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var calls []CallRecord
	for rows.Next() {
		var c CallRecord
		rows.Scan(&c.ID, &c.CallerNumber, &c.CalledNumber, &c.Mode, &c.Status,
			&c.StartTime, &c.EndTime, &c.Duration, &c.Summary, &c.Sentiment, &c.LLMModel)
		calls = append(calls, c)
	}
	return calls, nil
}

// --- Transcripts ---

func (d *Database) SaveTranscript(ctx context.Context, callID, speaker, text string) error {
	if d == nil {
		return nil
	}
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO call_transcripts (call_id, speaker, text) VALUES ($1, $2, $3)`,
		callID, speaker, text)
	return err
}

// --- Agents ---

func (d *Database) ListAgents(ctx context.Context) ([]map[string]any, error) {
	if d == nil {
		return nil, nil
	}
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, name, email, extension, department, expertise, status, active_calls FROM agents ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []map[string]any
	for rows.Next() {
		var id, name, department, status string
		var email, extension sql.NullString
		var expertise []string
		var activeCalls int
		rows.Scan(&id, &name, &email, &extension, &department, &expertise, &status, &activeCalls)
		agents = append(agents, map[string]any{
			"id": id, "name": name, "email": email.String,
			"extension": extension.String, "department": department,
			"expertise": expertise, "status": status, "active_calls": activeCalls,
		})
	}
	return agents, nil
}

func (d *Database) UpdateAgentStatus(ctx context.Context, agentID, status string) error {
	if d == nil {
		return nil
	}
	_, err := d.conn.ExecContext(ctx,
		`UPDATE agents SET status=$1, updated_at=NOW() WHERE id=$2`, status, agentID)
	return err
}

// --- Queue ---

func (d *Database) EnqueueCaller(ctx context.Context, callID, callerNumber, queueName, reason, priority string) error {
	if d == nil {
		return nil
	}
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO queue_entries (call_id, caller_number, queue_name, reason, priority)
		 VALUES ($1,$2,$3,$4,$5)`,
		callID, callerNumber, queueName, reason, priority)
	return err
}

func (d *Database) DequeueCaller(ctx context.Context, entryID, agentID string) error {
	if d == nil {
		return nil
	}
	_, err := d.conn.ExecContext(ctx,
		`UPDATE queue_entries SET status='assigned', assigned_agent=$2 WHERE id=$1`,
		entryID, agentID)
	return err
}

func (d *Database) ListQueueEntries(ctx context.Context, queueName string) ([]map[string]any, error) {
	if d == nil {
		return nil, nil
	}
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, call_id, caller_number, queue_name, reason, priority,
			EXTRACT(EPOCH FROM NOW() - wait_start)::INT as wait_sec
		FROM queue_entries WHERE status='waiting' AND ($1='' OR queue_name=$1)
		ORDER BY priority DESC, wait_start`, queueName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []map[string]any
	for rows.Next() {
		var id, callID, queueName, priority string
		var callerNumber, reason sql.NullString
		var waitSec int
		rows.Scan(&id, &callID, &callerNumber, &queueName, &reason, &priority, &waitSec)
		entries = append(entries, map[string]any{
			"id": id, "call_id": callID, "number": callerNumber.String,
			"queue": queueName, "reason": reason.String,
			"priority": priority, "waitSec": waitSec,
		})
	}
	return entries, nil
}

// --- Documents ---

func (d *Database) SaveDocument(ctx context.Context, id, name, category, content, chromaID string, chunks int) error {
	if d == nil {
		return nil
	}
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO documents (id, name, category, content, chroma_id, chunks)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (id) DO UPDATE SET name=$2, category=$3, content=$4, chunks=$6`,
		id, name, category, content, chromaID, chunks)
	return err
}

func (d *Database) ListDocuments(ctx context.Context) ([]map[string]any, error) {
	if d == nil {
		return nil, nil
	}
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, name, category, chunks, status, uploaded_at FROM documents ORDER BY uploaded_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []map[string]any
	for rows.Next() {
		var id, name, category, status string
		var chunks int
		var uploadedAt time.Time
		rows.Scan(&id, &name, &category, &chunks, &status, &uploadedAt)
		docs = append(docs, map[string]any{
			"id": id, "name": name, "category": category,
			"chunks": chunks, "status": status, "uploaded_at": uploadedAt,
		})
	}
	return docs, nil
}

// --- Stats ---

func (d *Database) DashboardStats(ctx context.Context) (map[string]any, error) {
	if d == nil {
		return map[string]any{
			"activeCalls": 0, "totalToday": 0, "avgDuration": 0,
			"sentimentBreakdown": map[string]int{"positive": 0, "neutral": 0, "negative": 0},
		}, nil
	}

	var totalToday, avgDuration int
	var positive, neutral, negative int

	d.conn.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(AVG(duration),0) FROM calls WHERE start_time > CURRENT_DATE`).
		Scan(&totalToday, &avgDuration)

	d.conn.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(CASE WHEN sentiment='positive' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN sentiment='neutral' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN sentiment='negative' THEN 1 ELSE 0 END),0)
		FROM calls WHERE start_time > CURRENT_DATE`).
		Scan(&positive, &neutral, &negative)

	return map[string]any{
		"totalToday":  totalToday,
		"avgDuration": avgDuration,
		"sentimentBreakdown": map[string]int{
			"positive": positive, "neutral": neutral, "negative": negative,
		},
	}, nil
}

// --- Utility ---

func arrayToPostgres(arr []string) string {
	if len(arr) == 0 {
		return "{}"
	}
	return "{" + strings.Join(arr, ",") + "}"
}
