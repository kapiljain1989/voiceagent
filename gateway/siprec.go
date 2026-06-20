package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// -------------------------------------------------------------------
// SIPREC Co-Pilot types
// -------------------------------------------------------------------

type Utterance struct {
	Speaker   string    `json:"speaker"` // "customer" | "agent"
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

type Suggestion struct {
	Text       string  `json:"suggestion"`
	Context    string  `json:"context"`
	Category   string  `json:"category"` // "answer", "upsell", "compliance", "empathy", "none"
	Confidence float64 `json:"confidence"`
}

type CallSummary struct {
	ConversationID string       `json:"conversation_id"`
	Duration       int          `json:"duration_seconds"`
	Transcript     []Utterance  `json:"transcript"`
	Summary        string       `json:"summary"`
	ActionItems    []string     `json:"action_items"`
	Commitments    []string     `json:"commitments_made"`
	Sentiment      string       `json:"sentiment"`
	Suggestions    []Suggestion `json:"suggestions_given"`
}

// -------------------------------------------------------------------
// SIPREC session — passive observation of a two-party call
// -------------------------------------------------------------------

type siprecSession struct {
	callID     string
	startTime  time.Time
	gw         *gateway

	callerNumber string // caller phone number / SIP URI
	agentNumber  string // agent extension / SIP URI

	callerConn *websocket.Conn
	agentConn  *websocket.Conn

	pcmCaller   chan []byte
	pcmAgent    chan []byte
	transcripts chan *Utterance
	suggestions chan *Suggestion

	conversation []Utterance
	allSuggs     []Suggestion
	convMu       sync.Mutex

	sseClients map[chan []byte]struct{}
	sseMu      sync.Mutex

	voiceSentiment *VoiceSentiment

	// Audio taps — additional listeners for caller audio (e.g., WebRTC bridge)
	audioTaps   []chan []byte
	audioTapsMu sync.Mutex

	// RTP session — set when audio arrives via SIP/RTP (standalone mode)
	rtpSession *siprecRTPSession

	conference *ConferenceSession // non-nil during 3-way conference

	cancel context.CancelFunc
	wg     sync.WaitGroup
	log    *slog.Logger
}

// Active SIPREC sessions keyed by call_id
var (
	siprecSessions   = make(map[string]*siprecSession)
	siprecSessionsMu sync.Mutex
)

func getOrCreateSIPRECSession(gw *gateway, callID string) *siprecSession {
	siprecSessionsMu.Lock()
	defer siprecSessionsMu.Unlock()

	if s, ok := siprecSessions[callID]; ok {
		return s
	}

	ctx, cancel := context.WithCancel(context.Background())
	_ = ctx

	s := &siprecSession{
		callID:         callID,
		startTime:      time.Now(),
		gw:             gw,
		pcmCaller:      make(chan []byte, pcmChanBufSize),
		pcmAgent:       make(chan []byte, pcmChanBufSize),
		transcripts:    make(chan *Utterance, 8),
		suggestions:    make(chan *Suggestion, 8),
		sseClients:     make(map[chan []byte]struct{}),
		voiceSentiment: NewVoiceSentiment(),
		cancel:         cancel,
		log:            slog.With("call_id", callID, "mode", "copilot"),
	}
	siprecSessions[callID] = s

	s.wg.Add(3)
	go s.callerSTT(ctx)
	go s.agentSTT(ctx)
	go s.coachWorker(ctx)

	go func() {
		s.wg.Wait()
		// Grace period — let any in-flight Whisper/Claude requests complete
		time.Sleep(3 * time.Second)
		s.onCallEnd()
		// Keep session in map for 30s so SSE clients can receive the summary
		time.Sleep(30 * time.Second)
		siprecSessionsMu.Lock()
		delete(siprecSessions, callID)
		siprecSessionsMu.Unlock()
		s.log.Info("copilot session ended")
	}()

	s.log.Info("copilot session starting")
	return s
}

func (s *siprecSession) AddAudioTap() chan []byte {
	ch := make(chan []byte, pcmChanBufSize)
	s.audioTapsMu.Lock()
	s.audioTaps = append(s.audioTaps, ch)
	s.audioTapsMu.Unlock()
	return ch
}

func (s *siprecSession) RemoveAudioTap(ch chan []byte) {
	s.audioTapsMu.Lock()
	for i, t := range s.audioTaps {
		if t == ch {
			s.audioTaps = append(s.audioTaps[:i], s.audioTaps[i+1:]...)
			break
		}
	}
	s.audioTapsMu.Unlock()
	close(ch)
}

func (s *siprecSession) broadcastToTaps(frame []byte) {
	s.audioTapsMu.Lock()
	for _, ch := range s.audioTaps {
		select {
		case ch <- frame:
		default:
		}
	}
	s.audioTapsMu.Unlock()
}

// -------------------------------------------------------------------
// /siprec — WebSocket endpoint for FreeSWITCH audio fork legs
//
// FreeSWITCH connects twice per call:
//   ws://gateway:8080/siprec?role=caller&call_id=xxx
//   ws://gateway:8080/siprec?role=agent&call_id=xxx
// -------------------------------------------------------------------

func (gw *gateway) handleSIPREC(w http.ResponseWriter, r *http.Request) {
	callID := r.URL.Query().Get("call_id")
	role := r.URL.Query().Get("role")

	if callID == "" || (role != "caller" && role != "agent") {
		http.Error(w, `{"error":"call_id and role (caller|agent) required"}`, http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("siprec ws upgrade", "err", err)
		return
	}

	s := getOrCreateSIPRECSession(gw, callID)

	// Extract caller/agent identity from query params if provided
	if caller := r.URL.Query().Get("caller"); caller != "" {
		s.callerNumber = caller
	}
	if agent := r.URL.Query().Get("agent"); agent != "" {
		s.agentNumber = agent
	}

	if role == "caller" {
		s.callerConn = conn
		s.log.Info("caller leg connected")

		// Auto-add to queue for Console visibility
		if gw.queueMgr != nil {
			callerNum := s.callerNumber
			if callerNum == "" {
				callerNum = callID[:12]
			}
			gw.queueMgr.AddCaller("Support", queueEntry{
				ID:       fmt.Sprintf("q-%d", time.Now().UnixNano()),
				CallID:   callID,
				Number:   callerNum,
				Reason:   "Co-pilot session",
				Priority: "normal",
			})
		}

		s.readLeg(conn, s.pcmCaller, "caller")
	} else {
		s.agentConn = conn
		s.log.Info("agent leg connected")
		s.readLeg(conn, s.pcmAgent, "agent")
	}
}

func (s *siprecSession) readLeg(conn *websocket.Conn, ch chan []byte, role string) {
	defer conn.Close()
	defer func() {
		if r := recover(); r != nil {
			s.log.Info("readLeg recovered from panic", "role", role, "err", r)
		}
	}()

	// Read initial frame (metadata or first audio)
	mt, raw, err := conn.ReadMessage()
	if err != nil {
		s.log.Info("readLeg initial read error", "role", role, "err", err)
		return
	}
	s.log.Info("readLeg initial frame", "role", role, "type", mt, "bytes", len(raw))
	if mt == websocket.BinaryMessage && len(raw) > 0 {
		select {
		case ch <- raw:
		default:
		}
		if role == "caller" {
			s.broadcastToTaps(raw)
		}
	}

	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			s.log.Info("leg disconnected", "role", role)
			if role == "caller" {
				close(s.pcmCaller)
			} else {
				close(s.pcmAgent)
			}
			s.cancel()
			return
		}
		if mt == websocket.TextMessage {
			var evt FSMetadata
			if json.Unmarshal(data, &evt) == nil && evt.Type == "stop" {
				if role == "caller" {
					close(s.pcmCaller)
				} else {
					close(s.pcmAgent)
				}
				s.cancel()
				return
			}
			continue
		}
		if mt == websocket.BinaryMessage && len(data) > 0 {
			buf := make([]byte, len(data))
			copy(buf, data)
			select {
			case ch <- buf:
			default:
			}
			if role == "caller" {
				s.broadcastToTaps(buf)
			}
		}
	}
}

// -------------------------------------------------------------------
// STT pipelines — one per leg, reuses VAD + Whisper logic
// -------------------------------------------------------------------

func (s *siprecSession) callerSTT(ctx context.Context) {
	defer s.wg.Done()
	s.runSTT(ctx, s.pcmCaller, "customer")
}

func (s *siprecSession) agentSTT(ctx context.Context) {
	defer s.wg.Done()
	s.runSTT(ctx, s.pcmAgent, "agent")
}

func (s *siprecSession) runSTT(ctx context.Context, pcmCh chan []byte, speaker string) {
	// Time-based chunking: collect audio in fixed windows, let Whisper handle VAD.
	// This avoids RMS-based VAD problems with low-amplitude G.711 telephony audio.
	const chunkMs = 3000
	const chunkFrames = chunkMs / vadFrameMs // 150 frames = 3 seconds
	warmupFrames := 500 / vadFrameMs         // skip first 0.5s

	var audioBuf []byte
	frameCount := 0
	sttFrames := 0

	for {
		select {
		case <-ctx.Done():
			s.log.Info("STT context done", "speaker", speaker, "frames", sttFrames)
			if len(audioBuf) > 0 {
				flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
				s.transcribeAndEmit(flushCtx, audioBuf, speaker)
				flushCancel()
			}
			return
		case pcm, ok := <-pcmCh:
			if !ok {
				s.log.Info("STT channel closed", "speaker", speaker, "frames", sttFrames)
				if len(audioBuf) > 0 {
					flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
					s.transcribeAndEmit(flushCtx, audioBuf, speaker)
					flushCancel()
				}
				return
			}

			sttFrames++
			frameCount++
			if frameCount <= warmupFrames {
				continue
			}

			// Feed every frame to voice sentiment analyzer + broadcast periodically
			if speaker == "customer" {
				s.voiceSentiment.ProcessFrame(pcm)
				if frameCount%250 == 0 {
					vs := s.voiceSentiment.Analyze()
					s.broadcastSSE(map[string]any{
						"type":              "voice_sentiment",
						"agitation":         vs.Agitation,
						"frustration":       vs.Frustration,
						"engagement":        vs.Engagement,
						"avg_pitch_hz":      vs.AvgPitch,
						"speaking_rate_wpm": vs.SpeakingRate,
						"avg_energy":        vs.AvgEnergy,
						"energy_trend":      vs.EnergyTrend,
						"pitch_variance":    vs.PitchVariance,
						"silence_ratio":     vs.SilenceRatio,
						"sentiment":         vs.Sentiment,
						"confidence":        vs.Confidence,
					})
				}
			}

			audioBuf = append(audioBuf, pcm...)

			// Flush every chunkFrames (3 seconds)
			if (frameCount-warmupFrames)%chunkFrames == 0 && len(audioBuf) > 0 {
				buf := make([]byte, len(audioBuf))
				copy(buf, audioBuf)
				audioBuf = audioBuf[:0]
				go s.transcribeAndEmit(ctx, buf, speaker)
			}
		}
	}
}

func (s *siprecSession) transcribeAndEmit(ctx context.Context, pcm []byte, speaker string) {
	minBytes := sampleRate * bytesPerSample * 3 / 10 // 0.3s minimum
	if len(pcm) < minBytes {
		return
	}

	// Amplify quiet telephony audio (G.711 decodes to low amplitude)
	amplified := make([]byte, len(pcm))
	copy(amplified, pcm)
	const gain = 8
	for i := 0; i < len(amplified)-1; i += 2 {
		s16 := int16(amplified[i]) | int16(amplified[i+1])<<8
		v := int32(s16) * gain
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		amplified[i] = byte(v)
		amplified[i+1] = byte(v >> 8)
	}
	pcm = amplified

	wav := buildWAV(pcm, sampleRate, 1, 16)
	text, err := s.whisperTranscribe(ctx, wav)
	if err != nil {
		s.log.Error("whisper", "err", err, "speaker", speaker)
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		s.log.Info("whisper returned empty", "speaker", speaker, "pcm_bytes", len(pcm))
		return
	}

	if isWhisperHallucination(text) {
		s.log.Info("filtered hallucination", "speaker", speaker, "text_len", len(text), "preview", text[:min(len(text), 80)])
		return
	}

	// Track word count for speaking rate analysis
	if speaker == "customer" {
		s.voiceSentiment.AddUtterance(len(strings.Fields(text)))
	}

	utt := &Utterance{
		Speaker:   speaker,
		Text:      text,
		Timestamp: time.Now(),
	}

	s.log.Info("heard", "speaker", speaker, "text", text)

	s.convMu.Lock()
	s.conversation = append(s.conversation, *utt)
	s.convMu.Unlock()

	s.broadcastSSE(map[string]any{
		"type":    "transcript",
		"speaker": speaker,
		"text":    text,
	})

	select {
	case s.transcripts <- utt:
	case <-ctx.Done():
	}
}

func (s *siprecSession) whisperTranscribe(ctx context.Context, wav []byte) (string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("file", "audio.wav")
	part.Write(wav)
	w.WriteField("model", "Systran/faster-whisper-small.en")
	w.WriteField("response_format", "json")
	w.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", s.gw.cfg.STTURL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("whisper %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Text string `json:"text"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Text, nil
}

// -------------------------------------------------------------------
// Coach worker — Claude in agent-assist mode
// -------------------------------------------------------------------

const coachSystemPrompt = `You are a real-time agent coach on a live call. Provide ONE brief suggestion to help the human agent respond to the customer.

IMPORTANT RULES:
- Output ONLY a single valid JSON object. No extra text, no multiple suggestions, no explanations.
- Quote specific facts from the knowledge base context when available (policy numbers, dollar amounts, deadlines).
- If the customer's words are unclear, interpret the most likely intent (e.g. "brushed pipe" = "burst pipe", "lead feed" = "late fee").
- Categories: "answer", "compliance", "empathy", "upsell"
- If no suggestion needed: {"suggestion":"","category":"none","confidence":0}

Output format (ONLY this, nothing else):
{"suggestion":"...","category":"...","confidence":0.9}`

func (s *siprecSession) coachWorker(ctx context.Context) {
	defer s.wg.Done()

	var history []claudeMessage

	for {
		select {
		case <-ctx.Done():
			return
		case utt, ok := <-s.transcripts:
			if !ok {
				return
			}

			role := "user"
			prefix := fmt.Sprintf("[%s]: ", utt.Speaker)
			history = append(history, claudeMessage{
				Role:    role,
				Content: prefix + utt.Text,
			})

			// Only generate suggestions on customer utterances
			if utt.Speaker != "customer" {
				continue
			}

			// Keep history bounded (last 10 turns)
			if len(history) > 10 {
				history = history[len(history)-10:]
			}

			// RAG: retrieve relevant knowledge base context
			ragContext := ""
			if s.gw.api != nil {
				ragContext = s.gw.api.GetRAGContext(ctx, utt.Text)
			}

			sugg, err := s.streamCoachWithRAG(ctx, history, ragContext)
			if err != nil {
				s.log.Error("coach", "err", err)
				continue
			}

			if sugg.Text == "" || sugg.Category == "none" {
				continue
			}

			sugg.Context = utt.Text
			s.log.Info("suggestion", "category", sugg.Category, "text", sugg.Text)

			s.convMu.Lock()
			s.allSuggs = append(s.allSuggs, *sugg)
			s.convMu.Unlock()

			s.broadcastSSE(map[string]any{
				"type":       "suggestion",
				"suggestion": sugg.Text,
				"category":   sugg.Category,
				"confidence": sugg.Confidence,
				"context":    sugg.Context,
			})
		}
	}
}

func (s *siprecSession) streamCoachWithRAG(ctx context.Context, history []claudeMessage, ragContext string) (*Suggestion, error) {
	systemPrompt := coachSystemPrompt
	if ragContext != "" {
		systemPrompt = ragContext + "\n\nUsing the knowledge base context above, " + coachSystemPrompt
	}
	url := fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:rawPredict",
		s.gw.cfg.GCPRegion, s.gw.cfg.GCPProjectID, s.gw.cfg.GCPRegion, s.gw.cfg.ClaudeModel,
	)

	body, _ := json.Marshal(map[string]any{
		"anthropic_version": "vertex-2023-10-16",
		"max_tokens":        256,
		"system":            systemPrompt,
		"messages":          history,
	})

	tok, err := s.gw.gcpCreds.TokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("gcp token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("claude %d: %s", resp.StatusCode, b)
	}

	var claudeResp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&claudeResp); err != nil {
		return nil, err
	}

	if len(claudeResp.Content) == 0 {
		return &Suggestion{Category: "none"}, nil
	}

	text := claudeResp.Content[0].Text
	var sugg Suggestion
	if err := json.Unmarshal([]byte(text), &sugg); err != nil {
		return &Suggestion{Text: text, Category: "answer", Confidence: 0.7}, nil
	}
	return &sugg, nil
}

// -------------------------------------------------------------------
// Post-call summary + CRM webhook
// -------------------------------------------------------------------

const summarySystemPrompt = `Generate a structured JSON summary of this customer-agent phone call.

Output format:
{
  "summary": "2-3 sentence summary of the call",
  "action_items": ["list of follow-up items"],
  "commitments_made": ["promises the agent made to the customer"],
  "sentiment": "positive|neutral|negative"
}`

func (s *siprecSession) onCallEnd() {
	s.convMu.Lock()
	conv := make([]Utterance, len(s.conversation))
	copy(conv, s.conversation)
	suggs := make([]Suggestion, len(s.allSuggs))
	copy(suggs, s.allSuggs)
	s.convMu.Unlock()

	if len(conv) == 0 {
		s.log.Info("no conversation to summarize")
		return
	}

	duration := int(time.Since(s.startTime).Seconds())

	// Build transcript text for Claude
	var transcript strings.Builder
	for _, u := range conv {
		fmt.Fprintf(&transcript, "[%s]: %s\n", u.Speaker, u.Text)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s.log.Info("generating summary", "transcript_length", transcript.Len(), "utterances", len(conv))

	summaryText, err := s.generateSummary(ctx, transcript.String())
	if err != nil {
		s.log.Error("summary generation failed", "err", err)
		summaryText = `{"summary":"Summary generation failed","action_items":[],"commitments_made":[],"sentiment":"neutral"}`
	}

	s.log.Info("summary raw response", "text", summaryText[:min(200, len(summaryText))])

	var parsed struct {
		Summary     string   `json:"summary"`
		ActionItems []string `json:"action_items"`
		Commitments []string `json:"commitments_made"`
		Sentiment   string   `json:"sentiment"`
	}
	if err := json.Unmarshal([]byte(summaryText), &parsed); err != nil {
		// Claude may return text with JSON embedded — try to extract
		if idx := strings.Index(summaryText, "{"); idx >= 0 {
			json.Unmarshal([]byte(summaryText[idx:]), &parsed)
		}
		if parsed.Summary == "" {
			parsed.Summary = summaryText
			parsed.Sentiment = "neutral"
		}
	}

	// Voice sentiment analysis (acoustic features from raw audio)
	voiceSentimentResult := s.voiceSentiment.Analyze()

	// Combine text sentiment (from Claude) with voice sentiment (from audio)
	finalSentiment := parsed.Sentiment
	if voiceSentimentResult.Frustration > 0.6 && parsed.Sentiment != "negative" {
		finalSentiment = "negative"
	}

	summary := CallSummary{
		ConversationID: s.callID,
		Duration:       duration,
		Transcript:     conv,
		Summary:        parsed.Summary,
		ActionItems:    parsed.ActionItems,
		Commitments:    parsed.Commitments,
		Sentiment:      finalSentiment,
		Suggestions:    suggs,
	}

	s.log.Info("call summary",
		"duration", duration,
		"utterances", len(conv),
		"suggestions", len(suggs),
		"text_sentiment", parsed.Sentiment,
		"voice_sentiment", voiceSentimentResult.Sentiment,
		"agitation", fmt.Sprintf("%.2f", voiceSentimentResult.Agitation),
		"frustration", fmt.Sprintf("%.2f", voiceSentimentResult.Frustration),
		"avg_pitch", fmt.Sprintf("%.0f", voiceSentimentResult.AvgPitch),
		"speaking_rate", fmt.Sprintf("%.0f", voiceSentimentResult.SpeakingRate),
		"final_sentiment", finalSentiment,
		"summary", parsed.Summary,
	)

	s.broadcastSSE(map[string]any{
		"type":            "summary",
		"summary":         parsed.Summary,
		"action_items":    parsed.ActionItems,
		"commitments":     parsed.Commitments,
		"sentiment":       finalSentiment,
		"duration":        duration,
		"voice_sentiment": voiceSentimentResult,
	})

	if s.gw.cfg.CRMWebhookURL != "" {
		s.postWebhook(summary)
	}
}

func (s *siprecSession) generateSummary(ctx context.Context, transcript string) (string, error) {
	url := fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:rawPredict",
		s.gw.cfg.GCPRegion, s.gw.cfg.GCPProjectID, s.gw.cfg.GCPRegion, s.gw.cfg.ClaudeModel,
	)

	body, _ := json.Marshal(map[string]any{
		"anthropic_version": "vertex-2023-10-16",
		"max_tokens":        512,
		"system":            summarySystemPrompt,
		"messages": []claudeMessage{
			{Role: "user", Content: transcript},
		},
	})

	tok, err := s.gw.gcpCreds.TokenSource.Token()
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("claude %d: %s", resp.StatusCode, b)
	}

	var claudeResp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	json.NewDecoder(resp.Body).Decode(&claudeResp)
	if len(claudeResp.Content) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return claudeResp.Content[0].Text, nil
}

func (s *siprecSession) postWebhook(summary CallSummary) {
	body, _ := json.Marshal(summary)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", s.gw.cfg.CRMWebhookURL, bytes.NewReader(body))
	if err != nil {
		s.log.Error("webhook build", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if s.gw.cfg.CRMWebhookToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.gw.cfg.CRMWebhookToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.log.Error("webhook post", "err", err)
		return
	}
	defer resp.Body.Close()

	s.log.Info("webhook sent", "status", resp.StatusCode, "url", s.gw.cfg.CRMWebhookURL)
}

// -------------------------------------------------------------------
// SSE event broadcaster — agent dashboard subscribes here
// -------------------------------------------------------------------

func (s *siprecSession) broadcastSSE(event map[string]any) {
	data, _ := json.Marshal(event)

	s.sseMu.Lock()
	clientCount := len(s.sseClients)
	for ch := range s.sseClients {
		select {
		case ch <- data:
		default:
		}
	}
	s.sseMu.Unlock()

	if t, ok := event["type"]; ok && (t == "voice_sentiment" || t == "transcript") {
		s.log.Info("SSE broadcast", "type", t, "clients", clientCount)
	}
}

func (s *siprecSession) addSSEClient(ch chan []byte) {
	s.sseMu.Lock()
	s.sseClients[ch] = struct{}{}
	s.sseMu.Unlock()
}

func (s *siprecSession) removeSSEClient(ch chan []byte) {
	s.sseMu.Lock()
	delete(s.sseClients, ch)
	s.sseMu.Unlock()
}

// /siprec/events — SSE endpoint for agent dashboard
func (gw *gateway) handleEvents(w http.ResponseWriter, r *http.Request) {
	callID := r.URL.Query().Get("call_id")
	if callID == "" {
		http.Error(w, `{"error":"call_id required"}`, http.StatusBadRequest)
		return
	}

	siprecSessionsMu.Lock()
	s, ok := siprecSessions[callID]
	siprecSessionsMu.Unlock()

	if !ok {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
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
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := make(chan []byte, 20)
	s.addSSEClient(ch)
	slog.Info("SSE client connected", "call_id", callID)
	defer func() {
		s.removeSSEClient(ch)
		slog.Info("SSE client disconnected", "call_id", callID)
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// /api/copilot/active — list active copilot sessions for UI auto-connect
func (gw *gateway) handleActiveCopilot(w http.ResponseWriter, r *http.Request) {
	siprecSessionsMu.Lock()
	sessions := make([]map[string]any, 0, len(siprecSessions))
	for id, s := range siprecSessions {
		entry := map[string]any{
			"call_id":    id,
			"started_at": s.startTime,
			"duration":   int(time.Since(s.startTime).Seconds()),
			"caller":     s.callerNumber,
			"agent":      s.agentNumber,
		}
		if s.voiceSentiment != nil {
			vs := s.voiceSentiment.Analyze()
			entry["voice_sentiment"] = vs
		}
		sessions = append(sessions, entry)
	}
	siprecSessionsMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

// /siprec/summary — GET past summaries (placeholder for DB integration)
func (gw *gateway) handleSummaryQuery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"note":   "Connect a database to store and query past summaries",
	})
}

