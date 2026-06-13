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

// -------------------------------------------------------------------
// In-memory queue manager
//
// Tracks callers waiting for agents. Queue entries are added when calls
// arrive and picked by agents via the console UI.
// -------------------------------------------------------------------

type queueEntry struct {
	ID        string    `json:"id"`
	CallID    string    `json:"call_id"`
	Number    string    `json:"number"`
	WaitSec   int       `json:"waitSec"`
	Reason    string    `json:"reason"`
	Priority  string    `json:"priority"` // "low", "normal", "high"
	QueueName string    `json:"queue_name"`
	EnterTime time.Time `json:"-"`
}

type queueInfo struct {
	Name      string       `json:"name"`
	AvgHandle string       `json:"avgHandle"`
	SLA       int          `json:"sla"`
	Callers   []queueEntry `json:"callers"`
}

type QueueManager struct {
	mu     sync.Mutex
	queues map[string]*queueInfo
	gw     *gateway
}

func NewQueueManager(gw *gateway) *QueueManager {
	qm := &QueueManager{
		gw: gw,
		queues: map[string]*queueInfo{
			"Support":    {Name: "Support", AvgHandle: "6:30", SLA: 85, Callers: nil},
			"Sales":      {Name: "Sales", AvgHandle: "8:15", SLA: 92, Callers: nil},
			"Billing":    {Name: "Billing", AvgHandle: "4:45", SLA: 78, Callers: nil},
			"Escalation": {Name: "Escalation", AvgHandle: "12:00", SLA: 95, Callers: nil},
		},
	}
	go qm.tickWaitTimes()
	return qm
}

func (qm *QueueManager) tickWaitTimes() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		qm.mu.Lock()
		for _, q := range qm.queues {
			for i := range q.Callers {
				q.Callers[i].WaitSec = int(time.Since(q.Callers[i].EnterTime).Seconds())
			}
		}
		qm.mu.Unlock()
	}
}

func (qm *QueueManager) AddCaller(queueName string, entry queueEntry) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	q, ok := qm.queues[queueName]
	if !ok {
		q = &queueInfo{Name: queueName, AvgHandle: "5:00", SLA: 80}
		qm.queues[queueName] = q
	}
	entry.EnterTime = time.Now()
	entry.QueueName = queueName
	q.Callers = append(q.Callers, entry)

	// Persist to DB
	if database != nil {
		database.EnqueueCaller(context.Background(), entry.CallID, entry.Number, queueName, entry.Reason, entry.Priority)
	}
}

func (qm *QueueManager) RemoveCaller(entryID string) (queueEntry, bool) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	for _, q := range qm.queues {
		for i, c := range q.Callers {
			if c.ID == entryID {
				q.Callers = append(q.Callers[:i], q.Callers[i+1:]...)
				return c, true
			}
		}
	}
	return queueEntry{}, false
}

func (qm *QueueManager) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/queues", qm.handleListQueues)
	mux.HandleFunc("/api/queue/pick", qm.handlePickCall)
	mux.HandleFunc("/api/queue/add", qm.handleAddToQueue)
}

func (qm *QueueManager) handleListQueues(w http.ResponseWriter, r *http.Request) {
	qm.mu.Lock()
	result := make([]queueInfo, 0, len(qm.queues))
	for _, q := range qm.queues {
		callers := q.Callers
		if callers == nil {
			callers = []queueEntry{}
		}
		result = append(result, queueInfo{
			Name:      q.Name,
			AvgHandle: q.AvgHandle,
			SLA:       q.SLA,
			Callers:   callers,
		})
	}
	qm.mu.Unlock()

	writeJSON(w, http.StatusOK, result)
}

type pickRequest struct {
	QueueEntryID string `json:"queue_entry_id"`
	AgentID      string `json:"agent_id"`
}

func (qm *QueueManager) handlePickCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req pickRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.QueueEntryID == "" {
		http.Error(w, `{"error":"queue_entry_id required"}`, http.StatusBadRequest)
		return
	}

	caller, ok := qm.RemoveCaller(req.QueueEntryID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "queue entry not found"})
		return
	}

	// Bridge the waiting call to the agent via ESL
	if caller.CallID != "" && qm.gw != nil {
		esl := qm.gw.newESLClient()
		agentExt := req.AgentID
		if agentExt == "" {
			agentExt = "1000"
		}
		cmd := fmt.Sprintf("uuid_transfer %s %s XML default", caller.CallID, agentExt)
		resp, err := esl.execute(cmd)
		if err != nil {
			slog.Error("esl bridge pick", "call_id", caller.CallID, "agent", agentExt, "err", err)
		} else {
			slog.Info("queue pick bridged", "call_id", caller.CallID, "agent", agentExt, "resp", resp)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"call_id": caller.CallID,
	})
}

type addQueueRequest struct {
	QueueName string `json:"queue_name"`
	CallID    string `json:"call_id"`
	Number    string `json:"number"`
	Reason    string `json:"reason"`
	Priority  string `json:"priority"`
}

func (qm *QueueManager) handleAddToQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req addQueueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.QueueName == "" {
		http.Error(w, `{"error":"queue_name required"}`, http.StatusBadRequest)
		return
	}

	if req.Priority == "" {
		req.Priority = "normal"
	}

	entry := queueEntry{
		ID:       fmt.Sprintf("q-%d", time.Now().UnixNano()),
		CallID:   req.CallID,
		Number:   req.Number,
		Reason:   req.Reason,
		Priority: req.Priority,
	}
	qm.AddCaller(req.QueueName, entry)

	slog.Info("queue add", "queue", req.QueueName, "number", req.Number, "id", entry.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "id": entry.ID})
}

// -------------------------------------------------------------------
// Agent Status + Directory
// -------------------------------------------------------------------

type agentInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Ext         string `json:"ext"`
	Status      string `json:"status"`
	Department  string `json:"department"`
	ActiveCalls int    `json:"activeCalls"`
}

type AgentManager struct {
	mu     sync.Mutex
	agents map[string]*agentInfo
}

func NewAgentManager() *AgentManager {
	return &AgentManager{
		agents: make(map[string]*agentInfo),
	}
}

func (am *AgentManager) SetStatus(agentID, status string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	if a, ok := am.agents[agentID]; ok {
		a.Status = status
	} else {
		am.agents[agentID] = &agentInfo{ID: agentID, Status: status}
	}
}

func (am *AgentManager) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/agent/status", am.handleAgentStatus)
	mux.HandleFunc("/api/agents/directory", am.handleAgentDirectory)
}

type statusRequest struct {
	AgentID string `json:"agent_id"`
	Status  string `json:"status"`
}

func (am *AgentManager) handleAgentStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req statusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AgentID == "" || req.Status == "" {
		http.Error(w, `{"error":"agent_id and status required"}`, http.StatusBadRequest)
		return
	}

	am.SetStatus(req.AgentID, req.Status)
	slog.Info("agent status", "agent_id", req.AgentID, "status", req.Status)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (am *AgentManager) handleAgentDirectory(w http.ResponseWriter, r *http.Request) {
	am.mu.Lock()
	result := make([]agentInfo, 0, len(am.agents))
	for _, a := range am.agents {
		result = append(result, *a)
	}
	am.mu.Unlock()

	// Enrich with active SIPREC session data
	siprecSessionsMu.Lock()
	activeAgents := make(map[string]int)
	for _, s := range siprecSessions {
		if s.agentNumber != "" {
			activeAgents[s.agentNumber]++
		}
	}
	siprecSessionsMu.Unlock()

	for i := range result {
		if count, ok := activeAgents[result[i].Ext]; ok {
			result[i].ActiveCalls = count
			if result[i].Status == "Available" {
				result[i].Status = "On Call"
			}
		}
	}

	writeJSON(w, http.StatusOK, result)
}
