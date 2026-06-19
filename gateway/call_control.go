package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// CallControlHandler manages hold/resume, transfer, and conference.
type CallControlHandler struct {
	gw      *gateway
	webrtc  *WebRTCManager
}

func NewCallControlHandler(gw *gateway, webrtc *WebRTCManager) *CallControlHandler {
	return &CallControlHandler{gw: gw, webrtc: webrtc}
}

func (cc *CallControlHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/call/hold", cc.handleHold)
	mux.HandleFunc("/api/call/resume", cc.handleResume)
	mux.HandleFunc("/api/call/transfer", cc.handleTransfer)
}

// ─── Hold ──────────────────────────────────────────────────────

func (cc *CallControlHandler) handleHold(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CallID string `json:"call_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.CallID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "call_id required"})
		return
	}

	cc.webrtc.mu.Lock()
	sess, ok := cc.webrtc.sessions[req.CallID]
	cc.webrtc.mu.Unlock()

	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "call not found"})
		return
	}

	sess.onHold = true
	slog.Info("call on hold", "call_id", req.CallID)

	// Start hold music for the caller
	siprecCallID := extractSIPRECCallID(req.CallID)
	if siprecCallID != "" {
		siprecSessionsMu.Lock()
		siprecSess, exists := siprecSessions[siprecCallID]
		siprecSessionsMu.Unlock()

		if exists && cc.gw.announcer != nil {
			go cc.gw.announcer.playHoldMusic(siprecSess, req.CallID)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "on_hold"})
}

// ─── Resume ────────────────────────────────────────────────────

func (cc *CallControlHandler) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CallID string `json:"call_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.CallID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "call_id required"})
		return
	}

	cc.webrtc.mu.Lock()
	sess, ok := cc.webrtc.sessions[req.CallID]
	cc.webrtc.mu.Unlock()

	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "call not found"})
		return
	}

	sess.onHold = false
	slog.Info("call resumed", "call_id", req.CallID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

// ─── Transfer ──────────────────────────────────────────────────

func (cc *CallControlHandler) handleTransfer(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CallID       string `json:"call_id"`
		TransferType string `json:"transfer_type"` // blind
		TargetType   string `json:"target_type"`   // queue, agent, external
		TargetValue  string `json:"target_value"`  // queue name, agent ID, or phone number
		AgentID      string `json:"agent_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.CallID == "" || req.TargetType == "" || req.TargetValue == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "call_id, target_type, target_value required"})
		return
	}

	siprecCallID := extractSIPRECCallID(req.CallID)
	slog.Info("call transfer", "call_id", req.CallID, "target_type", req.TargetType, "target", req.TargetValue)

	switch req.TargetType {
	case "queue":
		// Move caller to a different queue
		if cc.gw.queueMgr == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "queue manager not available"})
			return
		}

		// Remove from current queue
		caller, found := cc.gw.queueMgr.RemoveCaller("")
		if !found && siprecCallID != "" {
			cc.gw.queueMgr.RemoveCallerByCallID(siprecCallID)
		}

		// Disconnect current agent's WebRTC
		cc.webrtc.mu.Lock()
		if sess, ok := cc.webrtc.sessions[req.CallID]; ok {
			sess.close()
			delete(cc.webrtc.sessions, req.CallID)
		}
		cc.webrtc.mu.Unlock()

		// Update agent state
		if cc.gw.acd != nil && req.AgentID != "" {
			cc.gw.acd.OnCallEnd(req.AgentID)
		}

		// Re-queue the caller in the target queue
		callerNum := ""
		if caller.Number != "" {
			callerNum = caller.Number
		} else if siprecCallID != "" {
			callerNum = siprecCallID[:12]
		}

		cc.gw.queueMgr.AddCaller(req.TargetValue, queueEntry{
			ID:       caller.ID,
			CallID:   siprecCallID,
			Number:   callerNum,
			Reason:   "Transferred from agent",
			Priority: "normal",
		})

		// Start announcements for new queue
		if cc.gw.announcer != nil && siprecCallID != "" {
			siprecSessionsMu.Lock()
			siprecSess, exists := siprecSessions[siprecCallID]
			siprecSessionsMu.Unlock()
			if exists {
				cc.gw.announcer.StartAnnouncements(siprecCallID, req.TargetValue, siprecSess)
			}
		}

		slog.Info("call transferred to queue", "call_id", siprecCallID, "queue", req.TargetValue)
		writeJSON(w, http.StatusOK, map[string]string{"status": "transferred", "queue": req.TargetValue})

	case "agent":
		// Ring the target agent directly
		if cc.gw.acd != nil && cc.gw.acd.agentMgr != nil {
			cc.gw.acd.agentMgr.RingAgent(req.TargetValue, siprecCallID, "Transfer", "Transfer", 0)
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ringing_agent"})

	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported target_type: " + req.TargetType})
	}
}

// extractSIPRECCallID gets the SIPREC call ID from a bridge call ID
func extractSIPRECCallID(callID string) string {
	const prefix = "bridge-"
	if len(callID) > len(prefix) && callID[:len(prefix)] == prefix {
		return callID[len(prefix):]
	}
	return callID
}
