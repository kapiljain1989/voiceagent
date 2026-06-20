package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type callControlRequest struct {
	CallID  string `json:"call_id"`
	AgentID string `json:"agent_id"`
}

type transferRequest struct {
	CallID       string `json:"call_id"`
	TransferType string `json:"transfer_type"` // blind, warm
	TargetType   string `json:"target_type"`   // queue, agent, external
	TargetValue  string `json:"target_value"`
	AgentID      string `json:"agent_id"`
	// Legacy fields
	Target     string `json:"target"`
	Mode       string `json:"mode"`
	Department string `json:"department,omitempty"`
}

type conferenceRequest struct {
	CallID     string `json:"call_id"`
	Target     string `json:"target"`
	TargetType string `json:"target_type"` // agent, external
	AgentID    string `json:"agent_id"`
}

type conferenceDropRequest struct {
	CallID string `json:"call_id"`
	Who    string `json:"who"` // third, self
}

func (gw *gateway) newESLClient() *eslClient {
	return &eslClient{
		host:     gw.cfg.ESLHost,
		port:     gw.cfg.ESLPort,
		password: gw.cfg.ESLPassword,
	}
}

func (gw *gateway) registerCallControlRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/call/hold", gw.handleCallHold)
	mux.HandleFunc("/api/call/resume", gw.handleCallResume)
	mux.HandleFunc("/api/call/mute", gw.handleCallMute)
	mux.HandleFunc("/api/call/unmute", gw.handleCallUnmute)
	mux.HandleFunc("/api/call/transfer", gw.handleCallTransfer)
	mux.HandleFunc("/api/call/conference", gw.handleCallConference)
	mux.HandleFunc("/api/call/conference/drop", gw.handleConferenceDrop)
}

// findWebRTCSession looks up a WebRTC session by call ID
func (gw *gateway) findWebRTCSession(callID string) *WebRTCSession {
	// Search all WebRTC managers — we need access to the sessions map
	// This is called from call control handlers
	return nil // Will be set via the webrtcMgr reference
}

func (gw *gateway) handleCallHold(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req callControlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CallID == "" {
		http.Error(w, `{"error":"call_id required"}`, http.StatusBadRequest)
		return
	}

	// Find WebRTC session and set onHold
	if gw.webrtcMgr != nil {
		gw.webrtcMgr.mu.Lock()
		if sess, ok := gw.webrtcMgr.sessions[req.CallID]; ok {
			sess.onHold = true
			slog.Info("call on hold", "call_id", req.CallID)
		}
		gw.webrtcMgr.mu.Unlock()
	}

	// Send hold music to caller
	siprecCallID := extractSIPRECCallID(req.CallID)
	siprecSessionsMu.Lock()
	siprecSess, exists := siprecSessions[siprecCallID]
	siprecSessionsMu.Unlock()
	if exists && gw.announcer != nil {
		go gw.announcer.playHoldMusic(siprecSess, req.CallID)
	}

	gw.broadcastCallState(siprecCallID, "hold")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "state": "hold"})
}

func (gw *gateway) handleCallResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req callControlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CallID == "" {
		http.Error(w, `{"error":"call_id required"}`, http.StatusBadRequest)
		return
	}

	// Resume WebRTC audio bridge
	if gw.webrtcMgr != nil {
		gw.webrtcMgr.mu.Lock()
		if sess, ok := gw.webrtcMgr.sessions[req.CallID]; ok {
			sess.onHold = false
			slog.Info("call resumed", "call_id", req.CallID)
		}
		gw.webrtcMgr.mu.Unlock()
	}

	siprecCallID := extractSIPRECCallID(req.CallID)
	gw.broadcastCallState(siprecCallID, "connected")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "state": "connected"})
}

func (gw *gateway) handleCallMute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req callControlRequest
	json.NewDecoder(r.Body).Decode(&req)
	// Mute is handled client-side (WebRTC track.enabled = false)
	slog.Info("call mute", "call_id", req.CallID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "state": "muted"})
}

func (gw *gateway) handleCallUnmute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req callControlRequest
	json.NewDecoder(r.Body).Decode(&req)
	slog.Info("call unmute", "call_id", req.CallID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "state": "connected"})
}

func (gw *gateway) handleCallTransfer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req transferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.CallID == "" {
		http.Error(w, `{"error":"call_id required"}`, http.StatusBadRequest)
		return
	}

	// Support both new and legacy field names
	targetType := req.TargetType
	targetValue := req.TargetValue
	if targetType == "" && req.Target != "" {
		targetValue = req.Target
		if req.Department != "" {
			targetType = "queue"
			targetValue = req.Department
		} else {
			targetType = "agent"
		}
	}

	siprecCallID := extractSIPRECCallID(req.CallID)
	slog.Info("call transfer", "call_id", req.CallID, "target_type", targetType, "target", targetValue)

	switch targetType {
	case "queue":
		// Disconnect current agent's WebRTC
		if gw.webrtcMgr != nil {
			gw.webrtcMgr.mu.Lock()
			if sess, ok := gw.webrtcMgr.sessions[req.CallID]; ok {
				sess.close()
				delete(gw.webrtcMgr.sessions, req.CallID)
			}
			gw.webrtcMgr.mu.Unlock()
		}

		// Update agent state
		if gw.acd != nil && req.AgentID != "" {
			gw.acd.OnCallEnd(req.AgentID)
		}

		// Stop current announcements
		if gw.announcer != nil {
			gw.announcer.StopAnnouncements(siprecCallID)
		}

		// Re-queue in target queue
		if gw.queueMgr != nil {
			gw.queueMgr.RemoveCallerByCallID(siprecCallID)
			gw.queueMgr.AddCaller(targetValue, queueEntry{
				ID:       fmt.Sprintf("q-%d", time.Now().UnixNano()),
				CallID:   siprecCallID,
				Number:   "Transfer",
				Reason:   "Transferred from agent",
				Priority: "normal",
			})
		}

		// Start announcements for new queue
		if gw.announcer != nil {
			siprecSessionsMu.Lock()
			siprecSess, exists := siprecSessions[siprecCallID]
			siprecSessionsMu.Unlock()
			if exists {
				gw.announcer.StartAnnouncements(siprecCallID, targetValue, siprecSess)
			}
		}

		slog.Info("call transferred to queue", "call_id", siprecCallID, "queue", targetValue)
		writeJSON(w, http.StatusOK, map[string]string{"status": "transferred", "target": targetValue})

	case "agent":
		// Disconnect current agent's WebRTC
		if gw.webrtcMgr != nil {
			gw.webrtcMgr.mu.Lock()
			if sess, ok := gw.webrtcMgr.sessions[req.CallID]; ok {
				sess.close()
				delete(gw.webrtcMgr.sessions, req.CallID)
			}
			gw.webrtcMgr.mu.Unlock()
		}

		// Update current agent state
		if gw.acd != nil && req.AgentID != "" {
			gw.acd.OnCallEnd(req.AgentID)
		}

		// Re-queue so the target agent can pick via Console
		if gw.queueMgr != nil {
			gw.queueMgr.RemoveCallerByCallID(siprecCallID)
			gw.queueMgr.AddCaller("Transfer", queueEntry{
				ID:       fmt.Sprintf("q-%d", time.Now().UnixNano()),
				CallID:   siprecCallID,
				Number:   "Transfer",
				Reason:   fmt.Sprintf("Transfer from agent"),
				Priority: "high",
			})
		}

		// Ring the target agent directly
		if gw.acd != nil && gw.acd.agentMgr != nil {
			// Get caller number from SIPREC session
			callerNum := "Transfer"
			siprecSessionsMu.Lock()
			if s, ok := siprecSessions[siprecCallID]; ok {
				if s.callerNumber != "" {
					callerNum = s.callerNumber
				}
			}
			siprecSessionsMu.Unlock()

			gw.acd.agentMgr.RingAgent(targetValue, siprecCallID, callerNum, "Transfer", 0)
		}

		slog.Info("call transferred to agent", "call_id", siprecCallID, "target_agent", targetValue)
		writeJSON(w, http.StatusOK, map[string]string{"status": "transferred", "target": targetValue})

	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported target_type: " + targetType})
	}
}

func (gw *gateway) handleCallConference(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req conferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.CallID == "" || req.Target == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "call_id and target required"})
		return
	}

	targetType := req.TargetType
	if targetType == "" {
		targetType = "agent"
	}

	siprecCallID := extractSIPRECCallID(req.CallID)
	slog.Info("conference request", "call_id", siprecCallID, "target", req.Target, "type", targetType)

	cs, err := startConference(gw, siprecCallID, targetType, req.Target)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":      "conference_started",
		"call_id":     cs.callID,
		"third_party": req.Target,
	})
}

func (gw *gateway) handleConferenceDrop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req conferenceDropRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	siprecCallID := extractSIPRECCallID(req.CallID)

	switch req.Who {
	case "third":
		if err := dropThirdParty(siprecCallID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "third_party_dropped"})
	case "self":
		endConference(siprecCallID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "left_conference"})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "who must be 'third' or 'self'"})
	}
}

func (gw *gateway) broadcastCallState(callID, state string) {
	siprecSessionsMu.Lock()
	s, ok := siprecSessions[callID]
	clientCount := 0
	if ok {
		s.sseMu.Lock()
		clientCount = len(s.sseClients)
		s.sseMu.Unlock()
	}
	siprecSessionsMu.Unlock()

	slog.Info("broadcastCallState", "call_id", callID, "state", state, "session_found", ok, "sse_clients", clientCount)

	if ok {
		s.broadcastSSE(map[string]any{
			"type":    "call_state",
			"state":   state,
			"call_id": callID,
		})
	}
}

func extractSIPRECCallID(callID string) string {
	const prefix = "bridge-"
	if len(callID) > len(prefix) && callID[:len(prefix)] == prefix {
		return callID[len(prefix):]
	}
	return callID
}

// departmentExtension and sanitizeHeader are defined in actions.go
