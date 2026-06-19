package main

import (
	"context"
	"database/sql"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

// ACD — Automatic Call Distributor
// Watches queues and assigns waiting callers to the best available agent.
type ACD struct {
	db       *sql.DB
	gw       *gateway
	agentMgr *AgentSessionManager
	mu       sync.Mutex
	stopCh   chan struct{}
	ringing  map[string]time.Time // call_id → ring start time (prevent re-ring)
}

type acdAgent struct {
	ID          string
	Name        string
	Status      string
	Expertise   []string
	ActiveCalls int
	MaxCalls    int
	Priority    int
	LastCallEnd *time.Time
}

type acdScore struct {
	Agent acdAgent
	Score int
}

func NewACD(db *sql.DB, gw *gateway, agentMgr *AgentSessionManager) *ACD {
	return &ACD{
		db:       db,
		gw:       gw,
		agentMgr: agentMgr,
		stopCh:   make(chan struct{}),
		ringing:  make(map[string]time.Time),
	}
}

// Start begins the ACD loop — checks queues every 2 seconds
func (a *ACD) Start() {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		slog.Info("ACD started")
		for {
			select {
			case <-a.stopCh:
				return
			case <-ticker.C:
				a.processQueues()
			}
		}
	}()
}

func (a *ACD) Stop() {
	close(a.stopCh)
}

func (a *ACD) processQueues() {
	if a.gw == nil || a.gw.queueMgr == nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	qm := a.gw.queueMgr
	qm.mu.Lock()
	defer qm.mu.Unlock()

	for queueName, q := range qm.queues {
		if len(q.Callers) == 0 {
			continue
		}

		// Get queue skills
		queueSkills := a.getQueueSkills(queueName)

		// Find best agent for the first waiting caller
		caller := q.Callers[0]

		// Skip if already ringing (prevent re-ring loop)
		if ringTime, ok := a.ringing[caller.CallID]; ok {
			if time.Since(ringTime) < 30*time.Second {
				continue // still ringing, wait
			}
			// Ring timeout — could try next agent, for now re-ring
			delete(a.ringing, caller.CallID)
		}

		agent := a.findBestAgent(queueName, queueSkills)
		if agent == nil {
			continue
		}

		slog.Info("ACD assigning call",
			"queue", queueName,
			"caller", caller.Number,
			"agent", agent.Name,
			"agent_id", agent.ID,
		)

		// Mark as ringing so we don't re-assign
		a.ringing[caller.CallID] = time.Now()

		// Ring the agent via SSE
		if a.agentMgr != nil {
			a.agentMgr.RingAgent(agent.ID, caller.CallID, caller.Number, queueName, int(caller.WaitSec))
		}
	}
}

func (a *ACD) findBestAgent(queueName string, queueSkills []string) *acdAgent {
	if a.db == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Get agents assigned to this queue
	rows, err := a.db.QueryContext(ctx, `
		SELECT a.id, a.name, COALESCE(a.status,'Offline'),
		       COALESCE(a.expertise, '{}'), COALESCE(a.active_calls,0),
		       COALESCE(a.max_calls,3), COALESCE(a.priority,1), a.last_call_end
		FROM agents a
		JOIN agent_queues aq ON a.id = aq.agent_id
		JOIN queues q ON aq.queue_id = q.id
		WHERE q.name = $1 AND a.status = 'Available'
	`, queueName)
	if err != nil {
		slog.Debug("ACD query agents", "err", err)
		return nil
	}
	defer rows.Close()

	var candidates []acdScore
	for rows.Next() {
		var ag acdAgent
		var expertise string
		var lastCallEnd sql.NullTime
		rows.Scan(&ag.ID, &ag.Name, &ag.Status, &expertise,
			&ag.ActiveCalls, &ag.MaxCalls, &ag.Priority, &lastCallEnd)

		if lastCallEnd.Valid {
			ag.LastCallEnd = &lastCallEnd.Time
		}

		// Parse expertise array
		expertise = strings.Trim(expertise, "{}")
		if expertise != "" {
			ag.Expertise = strings.Split(expertise, ",")
		}

		// Filter: must have capacity
		if ag.ActiveCalls >= ag.MaxCalls {
			continue
		}

		// Filter: wrap-up time (15 seconds after last call)
		if ag.LastCallEnd != nil && time.Since(*ag.LastCallEnd) < 15*time.Second {
			continue
		}

		// Score the agent
		score := a.scoreAgent(ag, queueSkills)
		candidates = append(candidates, acdScore{Agent: ag, Score: score})
	}

	if len(candidates) == 0 {
		return nil
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	return &candidates[0].Agent
}

func (a *ACD) scoreAgent(agent acdAgent, queueSkills []string) int {
	score := 0

	// Skill match: +10 per matching skill
	for _, qs := range queueSkills {
		for _, as := range agent.Expertise {
			if strings.TrimSpace(as) == strings.TrimSpace(qs) {
				score += 10
			}
		}
	}

	// Priority tier: +5 × priority
	score += 5 * agent.Priority

	// Load penalty: -10 × active calls
	score -= 10 * agent.ActiveCalls

	// Idle bonus: +1 per 10 seconds idle
	if agent.LastCallEnd != nil {
		idleSec := int(time.Since(*agent.LastCallEnd).Seconds())
		score += idleSec / 10
	} else {
		score += 30 // never had a call = very idle
	}

	return score
}

func (a *ACD) getQueueSkills(queueName string) []string {
	if a.db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var skills string
	err := a.db.QueryRowContext(ctx,
		`SELECT COALESCE(skills_required, '{}') FROM queues WHERE name=$1`, queueName).Scan(&skills)
	if err != nil {
		return nil
	}

	skills = strings.Trim(skills, "{}")
	if skills == "" {
		return nil
	}
	return strings.Split(skills, ",")
}

// OnCallEnd updates agent state after a call ends
func (a *ACD) OnCallEnd(agentID string) {
	if a.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	a.db.ExecContext(ctx, `UPDATE agents SET last_call_end=NOW(), active_calls=GREATEST(active_calls-1, 0) WHERE id=$1`, agentID)
	// If no more active calls, set status back to Available
	a.db.ExecContext(ctx, `UPDATE agents SET status='Available' WHERE id=$1 AND active_calls <= 0 AND status='On Call'`, agentID)
	slog.Info("ACD call ended", "agent_id", agentID)
}

// OnCallStart updates agent state when a call starts
func (a *ACD) OnCallStart(agentID string) {
	if a.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	a.db.ExecContext(ctx, `UPDATE agents SET active_calls=active_calls+1, status='On Call' WHERE id=$1`, agentID)
}
