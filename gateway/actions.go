package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// -------------------------------------------------------------------
// Smart Self-Service Actions + Intelligent Call Transfer
//
// The LLM parses customer intent from natural speech. When it detects
// an actionable request or an escalation trigger, it returns a structured
// action instead of (or alongside) a spoken response.
//
// Action types:
//   - "api_call"   → execute a backend CRM/database API call
//   - "transfer"   → SIP REFER to human agent with context headers
//   - "speak"      → normal TTS response (default)
// -------------------------------------------------------------------

type CallAction struct {
	Type       string         `json:"type"`       // speak, api_call, transfer
	Text       string         `json:"text"`       // spoken response to caller
	Intent     string         `json:"intent"`     // reschedule, cancel, check_status, escalate
	APICall    *APICAction    `json:"api_call,omitempty"`
	Transfer   *TransferAction `json:"transfer,omitempty"`
	Confidence float64        `json:"confidence"`
}

type APICAction struct {
	Endpoint string         `json:"endpoint"` // CRM API endpoint
	Method   string         `json:"method"`   // GET, POST, PUT
	Payload  map[string]any `json:"payload"`  // request body
}

type TransferAction struct {
	Reason      string `json:"reason"`       // angry, complex_legal, complex_financial, request
	Department  string `json:"department"`    // billing, technical, retention, supervisor
	Extension   string `json:"extension"`     // target extension number
	Priority    string `json:"priority"`      // normal, urgent
	Summary     string `json:"summary"`       // 1-sentence context for the agent
}

// ActionExecutor handles self-service actions and call transfers.
type ActionExecutor struct {
	gw          *gateway
	webhookURLs map[string]string // intent → CRM API URL
}

func NewActionExecutor(gw *gateway) *ActionExecutor {
	return &ActionExecutor{
		gw: gw,
		webhookURLs: map[string]string{
			"reschedule":   envOr("ACTION_RESCHEDULE_URL", ""),
			"cancel":       envOr("ACTION_CANCEL_URL", ""),
			"check_status": envOr("ACTION_STATUS_URL", ""),
			"update_info":  envOr("ACTION_UPDATE_URL", ""),
		},
	}
}

// -------------------------------------------------------------------
// Action-aware system prompt
//
// This replaces the default system prompt when self-service is enabled.
// Claude returns JSON with action instructions instead of plain text.
// -------------------------------------------------------------------

const actionSystemPrompt = `You are a voice assistant for a call center. You can perform actions AND speak to the customer.

When the customer makes an actionable request, respond with a JSON object:

For self-service actions (reschedule, cancel, check status, update info):
{"type":"api_call","text":"I've rescheduled your delivery to Thursday at 3 PM.","intent":"reschedule","api_call":{"endpoint":"/deliveries","method":"PUT","payload":{"date":"2026-06-12","time":"15:00"}},"confidence":0.9}

For escalation to a human agent (customer is angry, complex legal/financial issue, or explicitly asks):
{"type":"transfer","text":"I understand this is frustrating. Let me connect you with a specialist who can help.","intent":"escalate","transfer":{"reason":"angry","department":"retention","priority":"urgent","summary":"Customer upset about billing error of $142. Wants refund."},"confidence":0.95}

For normal conversation (no action needed):
{"type":"speak","text":"Your current plan includes 5GB of data and unlimited calls.","intent":"info","confidence":1.0}

RULES:
- Always include a "text" field with what to speak to the customer
- Parse dates, times, amounts from natural language ("Thursday at 3" → "2026-06-12T15:00")
- Detect anger signals: raised voice context, profanity, "let me speak to a manager", repeated complaints
- Detect complexity: legal terms, regulatory mentions, multi-party disputes
- Keep spoken responses conversational and brief (1-2 sentences)
- Return ONLY valid JSON, nothing else`

// -------------------------------------------------------------------
// ParseAction extracts a CallAction from Claude's JSON response.
// Falls back to a plain "speak" action if JSON parsing fails.
// -------------------------------------------------------------------

func ParseAction(response string) *CallAction {
	response = strings.TrimSpace(response)

	// Try to extract JSON from the response
	var action CallAction
	if err := json.Unmarshal([]byte(response), &action); err == nil && action.Type != "" {
		return &action
	}

	// Try to find JSON embedded in text
	if idx := strings.Index(response, "{"); idx >= 0 {
		end := strings.LastIndex(response, "}")
		if end > idx {
			if err := json.Unmarshal([]byte(response[idx:end+1]), &action); err == nil && action.Type != "" {
				return &action
			}
		}
	}

	// Fallback: treat entire response as spoken text
	return &CallAction{
		Type:       "speak",
		Text:       response,
		Intent:     "conversation",
		Confidence: 1.0,
	}
}

// -------------------------------------------------------------------
// ExecuteAction processes the action: API call, transfer, or speak.
// Returns the text to speak back to the caller.
// -------------------------------------------------------------------

func (ae *ActionExecutor) ExecuteAction(ctx context.Context, s *session, action *CallAction) string {
	switch action.Type {
	case "api_call":
		return ae.executeAPICall(ctx, s, action)
	case "transfer":
		return ae.executeTransfer(ctx, s, action)
	default:
		return action.Text
	}
}

func (ae *ActionExecutor) executeAPICall(ctx context.Context, s *session, action *CallAction) string {
	if action.APICall == nil {
		return action.Text
	}

	s.log.Info("self-service action",
		"intent", action.Intent,
		"endpoint", action.APICall.Endpoint,
		"confidence", action.Confidence,
	)

	// Check if we have a configured webhook for this intent
	webhookURL := ae.webhookURLs[action.Intent]
	if webhookURL == "" {
		s.log.Info("no webhook configured for intent, simulating success", "intent", action.Intent)
		s.sendEvent("action", fmt.Sprintf(`{"intent":"%s","status":"simulated","payload":%s}`,
			action.Intent, mustJSON(action.APICall.Payload)))
		return action.Text
	}

	// Execute the actual API call
	body, _ := json.Marshal(action.APICall.Payload)
	req, err := http.NewRequestWithContext(ctx, action.APICall.Method, webhookURL+action.APICall.Endpoint, strings.NewReader(string(body)))
	if err != nil {
		s.log.Error("action api build", "err", err)
		return "I'm sorry, I wasn't able to process that request. Let me try again."
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Call-ID", s.id)
	req.Header.Set("X-Intent", action.Intent)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.log.Error("action api call", "err", err)
		return "I'm having trouble connecting to our system right now. Can I try again in a moment?"
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		s.log.Info("action api success", "intent", action.Intent, "status", resp.StatusCode)
		s.sendEvent("action", fmt.Sprintf(`{"intent":"%s","status":"success"}`, action.Intent))
		return action.Text
	}

	s.log.Warn("action api failed", "intent", action.Intent, "status", resp.StatusCode)
	s.sendEvent("action", fmt.Sprintf(`{"intent":"%s","status":"failed","code":%d}`, action.Intent, resp.StatusCode))
	return "I wasn't able to complete that action. Would you like me to connect you with an agent who can help?"
}

// -------------------------------------------------------------------
// Intelligent Call Transfer via ESL
//
// Sends SIP REFER through FreeSWITCH with custom X-headers containing:
//   X-Transfer-Summary: 1-sentence context
//   X-Transfer-Reason: angry/complex/request
//   X-Transfer-Transcript: last 3 utterances
//   X-Transfer-Sentiment: positive/neutral/negative
//
// The receiving agent's Cisco/Avaya softphone displays these headers.
// -------------------------------------------------------------------

func (ae *ActionExecutor) executeTransfer(ctx context.Context, s *session, action *CallAction) string {
	if action.Transfer == nil {
		return action.Text
	}

	transfer := action.Transfer
	s.log.Info("call transfer",
		"reason", transfer.Reason,
		"department", transfer.Department,
		"priority", transfer.Priority,
		"summary", transfer.Summary,
	)

	s.sendEvent("transfer", fmt.Sprintf(
		`{"reason":"%s","department":"%s","priority":"%s","summary":"%s"}`,
		transfer.Reason, transfer.Department, transfer.Priority, transfer.Summary,
	))

	// Build the last 3 transcript lines for context
	s.histMu.Lock()
	recentTranscript := ""
	start := len(s.history) - 3
	if start < 0 {
		start = 0
	}
	for _, h := range s.history[start:] {
		recentTranscript += fmt.Sprintf("[%s] %s | ", h.Role, h.Content)
	}
	s.histMu.Unlock()

	// Determine target extension
	targetExt := transfer.Extension
	if targetExt == "" {
		targetExt = departmentExtension(transfer.Department)
	}

	// Execute transfer via ESL with custom SIP headers
	esl := &eslClient{
		host:     ae.gw.cfg.ESLHost,
		port:     ae.gw.cfg.ESLPort,
		password: ae.gw.cfg.ESLPassword,
	}

	// Set custom SIP headers on the channel before transfer
	headers := map[string]string{
		"X-Transfer-Summary":    truncate(transfer.Summary, 200),
		"X-Transfer-Reason":     transfer.Reason,
		"X-Transfer-Department": transfer.Department,
		"X-Transfer-Priority":   transfer.Priority,
		"X-Transfer-Transcript": truncate(recentTranscript, 500),
		"X-Transfer-CallID":     s.id,
	}

	for k, v := range headers {
		cmd := fmt.Sprintf("uuid_setvar %s sip_h_%s %s", s.id, k, sanitizeHeader(v))
		esl.execute(cmd)
	}

	// Transfer the call
	transferCmd := fmt.Sprintf("uuid_transfer %s %s XML outbound", s.id, targetExt)
	resp, err := esl.execute(transferCmd)
	if err != nil {
		s.log.Error("esl transfer", "err", err)
		return "I'm having trouble connecting you right now. Please hold while I try again."
	}

	s.log.Info("transfer executed", "target", targetExt, "resp", strings.TrimSpace(resp))
	return action.Text
}

// -------------------------------------------------------------------
// Department routing table
// -------------------------------------------------------------------

func departmentExtension(dept string) string {
	extensions := map[string]string{
		"billing":    "3001",
		"technical":  "3002",
		"sales":      "3003",
		"retention":  "3004",
		"supervisor": "3005",
		"legal":      "3006",
		"claims":     "3007",
	}
	if ext, ok := extensions[dept]; ok {
		return ext
	}
	return "3000" // general queue
}

// -------------------------------------------------------------------
// API endpoint — manage action webhooks and test actions
// -------------------------------------------------------------------

func (ae *ActionExecutor) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/actions/test", ae.handleTestAction)
	mux.HandleFunc("/api/actions/webhooks", ae.handleWebhooks)
}

func (ae *ActionExecutor) handleTestAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	action := ParseAction(req.Text)
	writeJSON(w, http.StatusOK, action)
}

func (ae *ActionExecutor) handleWebhooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		writeJSON(w, http.StatusOK, ae.webhookURLs)
	case "POST":
		var req map[string]string
		json.NewDecoder(r.Body).Decode(&req)
		for k, v := range req {
			ae.webhookURLs[k] = v
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// -------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func sanitizeHeader(s string) string {
	r := strings.NewReplacer("\n", " ", "\r", " ", "|", "/")
	return r.Replace(s)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
