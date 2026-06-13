package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

// -------------------------------------------------------------------
// Call Control API — hold, resume, mute, unmute, transfer, conference
//
// All endpoints accept POST with JSON body containing call_id.
// Commands are executed via FreeSWITCH ESL.
// -------------------------------------------------------------------

type callControlRequest struct {
	CallID string `json:"call_id"`
}

type transferRequest struct {
	CallID     string `json:"call_id"`
	Target     string `json:"target"`
	Mode       string `json:"mode"` // "blind" or "attended"
	Department string `json:"department,omitempty"`
}

type conferenceRequest struct {
	CallID string `json:"call_id"`
	Target string `json:"target"`
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

	esl := gw.newESLClient()
	resp, err := esl.execute(fmt.Sprintf("uuid_hold %s", req.CallID))
	if err != nil {
		slog.Error("esl hold", "call_id", req.CallID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "esl command failed"})
		return
	}

	gw.broadcastCallState(req.CallID, "hold")
	slog.Info("call hold", "call_id", req.CallID, "resp", strings.TrimSpace(resp))
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

	esl := gw.newESLClient()
	resp, err := esl.execute(fmt.Sprintf("uuid_hold off %s", req.CallID))
	if err != nil {
		slog.Error("esl resume", "call_id", req.CallID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "esl command failed"})
		return
	}

	gw.broadcastCallState(req.CallID, "connected")
	slog.Info("call resume", "call_id", req.CallID, "resp", strings.TrimSpace(resp))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "state": "connected"})
}

func (gw *gateway) handleCallMute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req callControlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CallID == "" {
		http.Error(w, `{"error":"call_id required"}`, http.StatusBadRequest)
		return
	}

	esl := gw.newESLClient()
	resp, err := esl.execute(fmt.Sprintf("uuid_audio %s stop write", req.CallID))
	if err != nil {
		slog.Error("esl mute", "call_id", req.CallID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "esl command failed"})
		return
	}

	gw.broadcastCallState(req.CallID, "muted")
	slog.Info("call mute", "call_id", req.CallID, "resp", strings.TrimSpace(resp))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "state": "muted"})
}

func (gw *gateway) handleCallUnmute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req callControlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CallID == "" {
		http.Error(w, `{"error":"call_id required"}`, http.StatusBadRequest)
		return
	}

	esl := gw.newESLClient()
	resp, err := esl.execute(fmt.Sprintf("uuid_audio %s start write", req.CallID))
	if err != nil {
		slog.Error("esl unmute", "call_id", req.CallID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "esl command failed"})
		return
	}

	gw.broadcastCallState(req.CallID, "connected")
	slog.Info("call unmute", "call_id", req.CallID, "resp", strings.TrimSpace(resp))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "state": "connected"})
}

func (gw *gateway) handleCallTransfer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req transferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CallID == "" {
		http.Error(w, `{"error":"call_id and target required"}`, http.StatusBadRequest)
		return
	}

	// Resolve target extension
	targetExt := req.Target
	if req.Department != "" {
		targetExt = departmentExtension(strings.ToLower(req.Department))
	}

	esl := gw.newESLClient()

	// Set transfer headers
	headers := map[string]string{
		"X-Transfer-Department": req.Department,
		"X-Transfer-Mode":      req.Mode,
		"X-Transfer-CallID":    req.CallID,
	}
	for k, v := range headers {
		if v != "" {
			cmd := fmt.Sprintf("uuid_setvar %s sip_h_%s %s", req.CallID, k, sanitizeHeader(v))
			esl.execute(cmd)
		}
	}

	// Execute transfer
	resp, err := esl.execute(fmt.Sprintf("uuid_transfer %s %s XML outbound", req.CallID, targetExt))
	if err != nil {
		slog.Error("esl transfer", "call_id", req.CallID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "esl command failed"})
		return
	}

	gw.broadcastCallState(req.CallID, "transferred")
	slog.Info("call transfer", "call_id", req.CallID, "target", targetExt, "mode", req.Mode, "resp", strings.TrimSpace(resp))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "state": "transferred", "target": targetExt})
}

func (gw *gateway) handleCallConference(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req conferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CallID == "" || req.Target == "" {
		http.Error(w, `{"error":"call_id and target required"}`, http.StatusBadRequest)
		return
	}

	esl := gw.newESLClient()

	confName := fmt.Sprintf("voiceagent_%s", req.CallID)

	// Move current call into conference
	_, err := esl.execute(fmt.Sprintf("uuid_transfer %s conference:%s XML default", req.CallID, confName))
	if err != nil {
		slog.Error("esl conference move", "call_id", req.CallID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "esl command failed"})
		return
	}

	// Originate third party into same conference
	originateCmd := fmt.Sprintf(
		"originate {origination_caller_id_number=conference}sofia/gateway/sbc/%s &conference(%s)",
		req.Target, confName,
	)
	resp, err := esl.execute(originateCmd)
	if err != nil {
		slog.Error("esl conference originate", "call_id", req.CallID, "target", req.Target, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "conference originate failed"})
		return
	}

	gw.broadcastCallState(req.CallID, "conference")
	slog.Info("conference started", "call_id", req.CallID, "target", req.Target, "resp", strings.TrimSpace(resp))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "state": "conference", "conference": confName})
}

// broadcastCallState sends a call_state event to all SSE clients for a session.
func (gw *gateway) broadcastCallState(callID, state string) {
	siprecSessionsMu.Lock()
	s, ok := siprecSessions[callID]
	siprecSessionsMu.Unlock()

	if ok {
		s.broadcastSSE(map[string]any{
			"type":    "call_state",
			"state":   state,
			"call_id": callID,
		})
	}
}
