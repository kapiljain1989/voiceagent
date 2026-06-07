package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/oauth2/google"
)

const (
	wsWriteWait     = 5 * time.Second
	pcmChanBufSize  = 50 // ~1s of 20ms PCM frames
	playbackFrameMs = 20

	// VAD parameters
	vadRMSThreshold  = 50    // RMS energy above this = speech (tuned for laptop mics)
	vadSilenceMs     = 400   // ms of silence triggers flush
	vadMaxBufferSecs = 4     // cap at 4s to keep Whisper fast
	vadFrameMs       = 20    // expected frame duration from FreeSWITCH
	sampleRate       = 16000 // Hz
	bytesPerSample   = 2     // 16-bit
)

// -------------------------------------------------------------------
// Configuration
// -------------------------------------------------------------------

type Config struct {
	ListenAddr   string
	STTURL       string // Whisper.cpp server
	TTSURL       string // Piper TTS server
	GCPProjectID string
	GCPRegion    string
	ClaudeModel  string
	SystemPrompt string
	ESLHost         string
	ESLPort         string
	ESLPassword     string
	TTSAudioDir     string
	CRMWebhookURL   string
	CRMWebhookToken string
	DBURL           string
	ChromaURL       string
	SIPListenAddr   string
}

func loadConfig() Config {
	return Config{
		ListenAddr:   envOr("LISTEN_ADDR", ":8080"),
		SIPListenAddr: envOr("SIP_LISTEN_ADDR", ""),
		STTURL:       envOr("STT_URL", "http://whisper:8000/v1/audio/transcriptions"),
		TTSURL:       envOr("TTS_URL", "http://piper:5000"),
		GCPProjectID: envOr("GCP_PROJECT_ID", os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID")),
		GCPRegion:    envOr("GCP_REGION", envOr("CLOUD_ML_REGION", "us-east5")),
		ClaudeModel:  envOr("CLAUDE_MODEL", "claude-3-5-haiku@20241022"),
		SystemPrompt: envOr("SYSTEM_PROMPT",
			"You are a helpful voice assistant. Keep responses concise and conversational. "+
				"Do not use markdown, bullet points, or any text formatting — your responses will be spoken aloud. "+
				"Respond in 1-3 short sentences unless the user asks for detail."),
		ESLHost:     envOr("ESL_HOST", "freeswitch.voiceagent.svc.cluster.local"),
		ESLPort:     envOr("ESL_PORT", "8022"),
		ESLPassword: envOr("ESL_PASSWORD", "ClueCon"),
		TTSAudioDir:     envOr("TTS_AUDIO_DIR", ""),
		CRMWebhookURL:   os.Getenv("CRM_WEBHOOK_URL"),
		CRMWebhookToken: os.Getenv("CRM_WEBHOOK_TOKEN"),
		DBURL:           os.Getenv("DATABASE_URL"),
		ChromaURL:       envOr("CHROMA_URL", ""),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// -------------------------------------------------------------------
// Wire protocol types
// -------------------------------------------------------------------

type FSMetadata struct {
	Type       string `json:"type"`
	CallID     string `json:"callId"`
	StreamID   string `json:"streamId"`
	SampleRate int    `json:"sampleRate"`
	Channels   int    `json:"channels"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// -------------------------------------------------------------------
// Shared gateway state
// -------------------------------------------------------------------

type gateway struct {
	cfg        *Config
	gcpCreds   *google.Credentials
	sessions   atomic.Int64
	api        *APIHandler
	robocall   *RobocallDetector
	actions    *ActionExecutor
	security   *SecurityHandler
	store      SessionStore
	metrics    *Metrics
	sttPool    *WorkerPool
	ttsPool    *WorkerPool
	rateLimiter *RateLimiter
	admission  *AdmissionController
}

// -------------------------------------------------------------------
// Session — one per bridged call
//
//	readFromFS ──pcmIn──▶ sttPipeline ──transcripts──▶ claudeWorker ──sentences──▶ ttsWorker ──pcmOut──▶ writeToFS
// -------------------------------------------------------------------

type session struct {
	id     string
	fsConn *websocket.Conn
	gw     *gateway

	pcmIn       chan []byte
	transcripts chan string
	sentences   chan string
	pcmOut      chan []byte
	events      chan []byte // JSON text frames sent back to client

	history []claudeMessage
	histMu  sync.Mutex

	playing atomic.Bool // true while TTS is being played back — STT discards frames

	voiceSentiment *VoiceSentiment

	cancel context.CancelFunc
	wg     sync.WaitGroup
	log    *slog.Logger
}

func (s *session) sendEvent(eventType, text string) {
	evt, _ := json.Marshal(map[string]string{"event": eventType, "text": text})
	select {
	case s.events <- evt:
	default:
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
	Subprotocols:    []string{"audio.drachtio.org"},
}

// -------------------------------------------------------------------
// Server bootstrap
// -------------------------------------------------------------------

func main() {
	cfg := loadConfig()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if cfg.GCPProjectID == "" {
		slog.Error("GCP_PROJECT_ID (or ANTHROPIC_VERTEX_PROJECT_ID) is required")
		os.Exit(1)
	}

	ctx := context.Background()
	gcpCreds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		slog.Error("gcp credentials", "err", err)
		os.Exit(1)
	}

	gw := &gateway{
		cfg:         &cfg,
		gcpCreds:    gcpCreds,
		store:       NewSessionStore(envOr("REDIS_URL", "")),
		metrics:     NewMetrics(),
		sttPool:     NewWorkerPool("stt", parseWorkerURLs(cfg.STTURL)),
		ttsPool:     NewWorkerPool("tts", parseWorkerURLs(cfg.TTSURL)),
		rateLimiter: NewRateLimiter(100, 200), // 100 req/s per IP, burst 200
		admission:   NewAdmissionController(500), // max 500 concurrent sessions
	}

	mux := http.NewServeMux()
	api := NewAPIHandler(gw)
	gw.api = api
	api.RegisterRoutes(mux)

	gw.robocall = NewRobocallDetector(api.db)
	gw.robocall.RegisterRoutes(mux)

	gw.actions = NewActionExecutor(gw)
	gw.actions.RegisterRoutes(mux)

	gw.security = NewSecurityHandler(api.db)
	gw.security.RegisterRoutes(mux)

	failover := NewFailoverManager(gw)
	mux.HandleFunc("/api/failover/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, failover.Status())
	})

	handleDTMFRoutes(mux)

	mux.HandleFunc("/metrics", gw.metrics.Handler())
	mux.HandleFunc("/api/scale/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"stt_pool":  gw.sttPool.Status(),
			"tts_pool":  gw.ttsPool.Status(),
			"admission": gw.admission.Status(),
		})
	})

	mux.HandleFunc("/ws", gw.handleFS)
	mux.HandleFunc("/call", gw.handleCall)
	mux.HandleFunc("/siprec", gw.handleSIPREC)
	mux.HandleFunc("/siprec/events", gw.handleEvents)
	mux.HandleFunc("/siprec/summary", gw.handleSummaryQuery)
	mux.HandleFunc("/api/copilot/active", gw.handleActiveCopilot)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","sessions":%d}`, gw.sessions.Load())
	})

	auth := NewAuthHandler(api.db, envOr("JWT_SECRET", ""))
	auth.RegisterRoutes(mux)

	trunks := NewTrunkHandler(api.db, gw)
	trunks.RegisterRoutes(mux)

	// Wrap all routes with CORS + auth middleware
	var handler http.Handler = mux
	if envOr("AUTH_ENABLED", "") == "true" {
		handler = auth.Middleware(mux)
		slog.Info("authentication enabled")
	}
	handler = corsMiddleware(handler)

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: handler}

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	failover.StartHealthMonitor(sigCtx)

	go func() {
		slog.Info("gateway listening", "addr", cfg.ListenAddr, "stt", cfg.STTURL, "tts", cfg.TTSURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	// Start native SIP server for direct SIPREC (no FreeSWITCH needed)
	if cfg.SIPListenAddr != "" {
		sipSrv, err := NewSIPServer(gw, cfg.SIPListenAddr)
		if err != nil {
			slog.Error("sip server init", "err", err)
		} else {
			if err := sipSrv.Start(); err != nil {
				slog.Error("sip server start", "err", err)
			}
		}
	}

	<-sigCtx.Done()
	slog.Info("shutting down", "sessions", gw.sessions.Load())
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
}

// -------------------------------------------------------------------
// Inbound handler
// -------------------------------------------------------------------

func (gw *gateway) handleFS(w http.ResponseWriter, r *http.Request) {
	// Extract FreeSWITCH channel UUID from query params (if present)
	fsUUID := r.URL.Query().Get("uuid")

	fsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade", "err", err)
		return
	}

	// mod_audio_fork may or may not send a JSON metadata frame first.
	// Read the first frame: if JSON, parse it; if binary, it's already audio.
	mt, raw, err := fsConn.ReadMessage()
	if err != nil {
		slog.Error("first frame read", "err", err)
		fsConn.Close()
		return
	}

	var meta FSMetadata
	var firstAudioFrame []byte

	cleaned := bytes.TrimLeft(raw, "\x00")
	if mt == websocket.TextMessage || (len(cleaned) > 0 && cleaned[0] == '{') {
		if err := json.Unmarshal(cleaned, &meta); err != nil {
			slog.Warn("unparseable first frame, treating as audio", "len", len(raw))
			meta = FSMetadata{CallID: fmt.Sprintf("sip-%d", time.Now().UnixMilli()), SampleRate: sampleRate, Channels: 1}
			firstAudioFrame = raw
		}
	} else {
		meta = FSMetadata{CallID: fmt.Sprintf("sip-%d", time.Now().UnixMilli()), SampleRate: sampleRate, Channels: 1}
		firstAudioFrame = raw
	}

	callID := meta.CallID
	if fsUUID != "" {
		callID = fsUUID
	}

	log := slog.With("call_id", callID)
	log.Info("session starting", "sample_rate", meta.SampleRate, "fs_uuid", fsUUID)

	ctx, cancel := context.WithCancel(context.Background())

	s := &session{
		id:             callID,
		fsConn:         fsConn,
		gw:             gw,
		pcmIn:          make(chan []byte, pcmChanBufSize),
		transcripts:    make(chan string, 4),
		sentences:      make(chan string, 8),
		pcmOut:         make(chan []byte, 20),
		events:         make(chan []byte, 10),
		voiceSentiment: NewVoiceSentiment(),
		cancel:         cancel,
		log:            log,
	}

	// If the first frame was audio (no JSON metadata), inject it into the pipeline.
	if len(firstAudioFrame) > 0 {
		go func() { s.pcmIn <- firstAudioFrame }()
	}

	gw.sessions.Add(1)

	s.wg.Add(5)
	go s.readFromFS(ctx)
	go s.sttPipeline(ctx)
	go s.claudeWorker(ctx)
	go s.ttsWorker(ctx)
	go s.writeToFS(ctx)

	go func() {
		s.wg.Wait()
		gw.sessions.Add(-1)
		fsConn.Close()
		log.Info("session ended")
	}()
}

// -------------------------------------------------------------------
// Stage 1: FS WebSocket reader → pcmIn
// -------------------------------------------------------------------

func (s *session) readFromFS(ctx context.Context) {
	defer s.wg.Done()
	defer s.cancel()
	defer close(s.pcmIn)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		mt, data, err := s.fsConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				s.log.Error("fs read", "err", err)
			}
			return
		}

		if mt == websocket.TextMessage {
			var evt FSMetadata
			if json.Unmarshal(data, &evt) == nil && evt.Type == "stop" {
				s.log.Info("fs stop event")
				return
			}
			continue
		}

		if mt != websocket.BinaryMessage || len(data) == 0 {
			continue
		}

		buf := make([]byte, len(data))
		copy(buf, data)

		select {
		case s.pcmIn <- buf:
		case <-ctx.Done():
			return
		default:
			s.log.Debug("pcmIn drop")
		}
	}
}

// -------------------------------------------------------------------
// Stage 2: STT pipeline — energy VAD + Whisper.cpp batch HTTP
//
// Accumulates PCM audio, detects speech boundaries via RMS energy,
// and POSTs completed utterances to the Whisper.cpp server as WAV.
// -------------------------------------------------------------------

func (s *session) sttPipeline(ctx context.Context) {
	defer s.wg.Done()
	defer close(s.transcripts)

	var audioBuf []byte
	speechActive := false
	silentFrames := 0
	frameCount := 0
	silenceLimit := vadSilenceMs / vadFrameMs
	maxBytes := vadMaxBufferSecs * sampleRate * bytesPerSample
	// Skip the first ~1s of frames — media path warmup produces zeros
	warmupFrames := 500 / vadFrameMs // 25 frames at 20ms = 0.5 second

	for {
		select {
		case <-ctx.Done():
			return
		case pcm, ok := <-s.pcmIn:
			if !ok {
				if len(audioBuf) > 0 && speechActive {
					s.transcribeAndSend(ctx, audioBuf)
				}
				return
			}

			frameCount++
			rms := rmsEnergy(pcm)

			if frameCount == 1 {
				s.log.Info("first audio frame", "bytes", len(pcm), "rms", int(rms))
			}
			if frameCount <= warmupFrames {
				continue // skip warmup — media path not stable yet
			}
			if frameCount == warmupFrames+1 {
				s.log.Info("warmup complete, VAD active", "rms", int(rms))
			}

			// Feed frames to voice sentiment analyzer
			s.voiceSentiment.ProcessFrame(pcm)

			// Discard frames during TTS playback to prevent feedback loop
			if s.playing.Load() {
				if speechActive {
					audioBuf = audioBuf[:0]
					speechActive = false
					silentFrames = 0
				}
				continue
			}

			if !speechActive && rms > vadRMSThreshold {
				s.log.Info("speech detected", "rms", int(rms))
			}

			if rms > vadRMSThreshold {
				audioBuf = append(audioBuf, pcm...)
				speechActive = true
				silentFrames = 0
			} else if speechActive {
				audioBuf = append(audioBuf, pcm...)
				silentFrames++
				if silentFrames >= silenceLimit {
					s.transcribeAndSend(ctx, audioBuf)
					audioBuf = audioBuf[:0]
					speechActive = false
					silentFrames = 0
				}
			}

			// Prevent unbounded growth
			if len(audioBuf) > maxBytes {
				if speechActive {
					s.transcribeAndSend(ctx, audioBuf)
				}
				audioBuf = audioBuf[:0]
				speechActive = false
				silentFrames = 0
			}
		}
	}
}

func (s *session) transcribeAndSend(ctx context.Context, pcm []byte) {
	if len(pcm) == 0 {
		return
	}

	text, err := s.whisperTranscribe(ctx, pcm)
	if err != nil {
		s.log.Error("whisper", "err", err)
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	// Filter Whisper hallucinations
	if isWhisperHallucination(text) {
		s.log.Debug("filtered hallucination", "text", text)
		return
	}

	// PII masking — redact credit cards, SSNs, etc. before logging or sending to LLM
	if s.gw.security != nil {
		masked, detections := s.gw.security.masker.MaskTranscript(text)
		if len(detections) > 0 {
			s.log.Warn("pii detected and masked", "detections", len(detections), "types", piiTypes(detections))
			s.sendEvent("pii_masked", fmt.Sprintf("Detected %d PII items: %s", len(detections), piiTypes(detections)))
			text = masked
		}
	}

	s.log.Info("heard", "text", text)

	// Layer 3 robocall detection — keyword check on transcript
	if s.gw.robocall != nil {
		kwResult := s.gw.robocall.ClassifyTranscript(text)
		if kwResult.Score > 0.3 {
			s.log.Warn("robocall keywords detected", "score", kwResult.Score, "keywords", kwResult.Keywords)
			s.sendEvent("robocall", fmt.Sprintf("score=%.0f%% keywords=%v", kwResult.Score*100, kwResult.Keywords))
		}
	}

	s.sendEvent("transcript", text)
	select {
	case s.transcripts <- text:
	case <-ctx.Done():
	}
}

func (s *session) whisperTranscribe(ctx context.Context, pcm []byte) (string, error) {
	wav := buildWAV(pcm, sampleRate, 1, 16)

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "audio.wav")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(wav); err != nil {
		return "", err
	}
	w.WriteField("model", "Systran/faster-whisper-base.en")
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
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Text, nil
}

// -------------------------------------------------------------------
// Stage 3: Claude worker — Vertex AI streaming SSE (unchanged)
// -------------------------------------------------------------------

func (s *session) claudeWorker(ctx context.Context) {
	defer s.wg.Done()
	defer close(s.sentences)

	for {
		select {
		case <-ctx.Done():
			return
		case transcript, ok := <-s.transcripts:
			if !ok {
				return
			}

			s.histMu.Lock()
			s.history = append(s.history, claudeMessage{Role: "user", Content: transcript})
			msgs := make([]claudeMessage, len(s.history))
			copy(msgs, s.history)
			s.histMu.Unlock()

			full, err := s.streamClaudeNoSplit(ctx, msgs)
			if err != nil {
				s.log.Error("claude", "err", err)
				continue
			}

			// Parse structured action response — extract spoken text from JSON
			action := ParseAction(full)
			spokenText := action.Text
			if spokenText == "" {
				// If ParseAction couldn't extract text, try to strip JSON manually
				spokenText = stripJSONToText(full)
			}
			s.log.Info("claude raw", "full", full, "parsed_text", spokenText, "action_type", action.Type)

			if action.Type == "api_call" || action.Type == "transfer" {
				s.log.Info("action detected", "type", action.Type, "intent", action.Intent, "confidence", action.Confidence)
				spokenText = s.gw.actions.ExecuteAction(ctx, s, action)
			}

			s.log.Info("replied", "text", spokenText, "action", action.Type)
			s.sendEvent("response", spokenText)

			// Send extracted text to TTS (not raw JSON)
			for i, sentence := range splitSentences(spokenText) {
				s.log.Info("tts sentence", "seq", i, "text", sentence)
				select {
				case s.sentences <- sentence:
				case <-ctx.Done():
				}
			}

			s.histMu.Lock()
			s.history = append(s.history, claudeMessage{Role: "assistant", Content: spokenText})
			s.histMu.Unlock()
		}
	}
}

func (s *session) streamClaude(ctx context.Context, messages []claudeMessage) (string, error) {
	url := fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:streamRawPredict",
		s.gw.cfg.GCPRegion, s.gw.cfg.GCPProjectID, s.gw.cfg.GCPRegion, s.gw.cfg.ClaudeModel,
	)

	systemPrompt := s.gw.cfg.SystemPrompt
	if s.gw.actions != nil {
		systemPrompt = actionSystemPrompt
	}

	body, _ := json.Marshal(map[string]any{
		"anthropic_version": "vertex-2023-10-16",
		"max_tokens":        512,
		"system":            systemPrompt,
		"messages":          messages,
		"stream":            true,
	})

	tok, err := s.gw.gcpCreds.TokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("gcp token: %w", err)
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

	var full, sentBuf strings.Builder
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var evt struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if json.Unmarshal([]byte(data), &evt) != nil {
			continue
		}
		if evt.Type == "message_stop" {
			break
		}
		if evt.Type != "content_block_delta" || evt.Delta.Type != "text_delta" {
			continue
		}

		full.WriteString(evt.Delta.Text)
		sentBuf.WriteString(evt.Delta.Text)

		if idx := sentenceEnd(sentBuf.String()); idx >= 0 {
			sentence := sentBuf.String()[:idx+1]
			remaining := sentBuf.String()[idx+1:]
			select {
			case s.sentences <- strings.TrimSpace(sentence):
			case <-ctx.Done():
				return full.String(), ctx.Err()
			}
			sentBuf.Reset()
			sentBuf.WriteString(remaining)
		}
	}

	if rest := strings.TrimSpace(sentBuf.String()); rest != "" {
		select {
		case s.sentences <- rest:
		case <-ctx.Done():
		}
	}

	return full.String(), nil
}

func sentenceEnd(s string) int {
	for i, c := range s {
		if c == '.' || c == '!' || c == '?' {
			next := i + 1
			if next >= len(s) || s[next] == ' ' {
				return i
			}
		}
	}
	return -1
}

func stripJSONToText(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return s
	}
	// Try to extract "text" field from JSON
	var obj map[string]any
	if json.Unmarshal([]byte(s), &obj) == nil {
		if t, ok := obj["text"].(string); ok && t != "" {
			return t
		}
	}
	// Try embedded JSON
	if idx := strings.Index(s, `"text"`); idx >= 0 {
		rest := s[idx+6:]
		if ci := strings.Index(rest, `"`); ci >= 0 {
			rest = rest[ci+1:]
			if ei := strings.Index(rest, `"`); ei >= 0 {
				return rest[:ei]
			}
		}
	}
	// If it looks like JSON but we can't parse it, strip non-speech characters
	if strings.HasPrefix(s, "{") {
		return ""
	}
	return s
}

func splitSentences(text string) []string {
	var sentences []string
	buf := text
	for {
		idx := sentenceEnd(buf)
		if idx < 0 {
			break
		}
		s := strings.TrimSpace(buf[:idx+1])
		if s != "" {
			sentences = append(sentences, s)
		}
		buf = buf[idx+1:]
	}
	if rest := strings.TrimSpace(buf); rest != "" {
		sentences = append(sentences, rest)
	}
	if len(sentences) == 0 && strings.TrimSpace(text) != "" {
		sentences = append(sentences, strings.TrimSpace(text))
	}
	return sentences
}

func (s *session) streamClaudeNoSplit(ctx context.Context, messages []claudeMessage) (string, error) {
	url := fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:streamRawPredict",
		s.gw.cfg.GCPRegion, s.gw.cfg.GCPProjectID, s.gw.cfg.GCPRegion, s.gw.cfg.ClaudeModel,
	)

	systemPrompt := s.gw.cfg.SystemPrompt
	if s.gw.actions != nil {
		systemPrompt = actionSystemPrompt
	}

	body, _ := json.Marshal(map[string]any{
		"anthropic_version": "vertex-2023-10-16",
		"max_tokens":        512,
		"system":            systemPrompt,
		"messages":          messages,
		"stream":            true,
	})

	tok, err := s.gw.gcpCreds.TokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("gcp token: %w", err)
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
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("claude %d: %s", resp.StatusCode, string(raw))
	}

	var full strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var evt struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if json.Unmarshal([]byte(data), &evt) != nil {
			continue
		}
		if evt.Type == "message_stop" {
			break
		}
		if evt.Type != "content_block_delta" || evt.Delta.Type != "text_delta" {
			continue
		}
		full.WriteString(evt.Delta.Text)
	}

	return full.String(), nil
}

// -------------------------------------------------------------------
// Stage 4: TTS worker — Piper HTTP
// -------------------------------------------------------------------

func (s *session) ttsWorker(ctx context.Context) {
	defer s.wg.Done()
	defer close(s.pcmOut)

	for {
		select {
		case <-ctx.Done():
			return
		case text, ok := <-s.sentences:
			if !ok {
				return
			}

			audio, err := s.piperSynthesize(ctx, text)
			if err != nil {
				s.log.Error("tts", "err", err, "text", text)
				continue
			}

			select {
			case s.pcmOut <- audio:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (s *session) piperSynthesize(ctx context.Context, text string) ([]byte, error) {
	// Piper HTTP reads request.data as raw text — send plain text, not JSON
	req, err := http.NewRequestWithContext(ctx, "POST", s.gw.cfg.TTSURL, strings.NewReader(text))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("piper %d: %s", resp.StatusCode, b)
	}

	wav, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Strip WAV header, extract raw PCM
	pcm := stripWAVHeader(wav)

	// Piper outputs 22050 Hz — resample to 16000 Hz for FreeSWITCH
	return resample(pcm, 22050, sampleRate), nil
}

// -------------------------------------------------------------------
// Stage 5: FS WebSocket writer ← pcmOut
// -------------------------------------------------------------------

func (s *session) writeToFS(ctx context.Context) {
	defer s.wg.Done()

	useESL := s.gw.cfg.TTSAudioDir != ""
	seqNum := 0

	for {
		select {
		case evt := <-s.events:
			s.fsConn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			s.fsConn.WriteMessage(websocket.TextMessage, evt)
			continue
		default:
		}

		select {
		case <-ctx.Done():
			return
		case evt := <-s.events:
			s.fsConn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			s.fsConn.WriteMessage(websocket.TextMessage, evt)
		case pcm, ok := <-s.pcmOut:
			if !ok {
				return
			}

			if useESL {
				// Collect this chunk + any others already queued
				batch := [][]byte{pcm}
			drain:
				for {
					select {
					case more, ok2 := <-s.pcmOut:
						if !ok2 {
							break drain
						}
						batch = append(batch, more)
					default:
						break drain
					}
				}

				// Single pause/resume cycle for the entire batch
				s.playBatchESL(batch, &seqNum)
			} else {
				s.playViaWebSocket(ctx, pcm)
			}
		}
	}
}

func (s *session) playBatchESL(batch [][]byte, seqNum *int) {
	esl := &eslClient{
		host:     s.gw.cfg.ESLHost,
		port:     s.gw.cfg.ESLPort,
		password: s.gw.cfg.ESLPassword,
	}

	// Block STT for the entire batch
	s.playing.Store(true)
	esl.execute(fmt.Sprintf("uuid_audio_fork %s pause", s.id))
	time.Sleep(100 * time.Millisecond)

	for _, pcm := range batch {
		wavData := buildWAV(pcm, sampleRate, 1, 16)
		filename := fmt.Sprintf("%s/tts_%s_%d.wav", s.gw.cfg.TTSAudioDir, s.id, *seqNum)
		*seqNum++

		if err := os.WriteFile(filename, wavData, 0644); err != nil {
			s.log.Error("write tts wav", "err", err)
			continue
		}

		cmd := fmt.Sprintf("uuid_broadcast %s %s aleg", s.id, filename)
		resp, err := esl.execute(cmd)
		if err != nil {
			s.log.Error("esl broadcast", "err", err)
			os.Remove(filename)
			continue
		}
		s.log.Info("playing tts", "file", filename, "resp", strings.TrimSpace(resp))

		durationMs := len(pcm) * 1000 / (sampleRate * bytesPerSample)
		time.Sleep(time.Duration(durationMs+300) * time.Millisecond)
		os.Remove(filename)
	}

	// Resume after all sentences played
	esl.execute(fmt.Sprintf("uuid_audio_fork %s resume", s.id))
	time.Sleep(300 * time.Millisecond)
	s.playing.Store(false)
}

func (s *session) playViaWebSocket(ctx context.Context, pcm []byte) {
	frameSize := sampleRate * bytesPerSample * playbackFrameMs / 1000
	for off := 0; off < len(pcm); off += frameSize {
		select {
		case <-ctx.Done():
			return
		default:
		}
		end := off + frameSize
		if end > len(pcm) {
			end = len(pcm)
		}
		s.fsConn.SetWriteDeadline(time.Now().Add(wsWriteWait))
		if err := s.fsConn.WriteMessage(websocket.BinaryMessage, pcm[off:end]); err != nil {
			s.log.Error("fs write", "err", err)
			s.cancel()
			return
		}
		time.Sleep(time.Duration(playbackFrameMs-2) * time.Millisecond)
	}
}

// -------------------------------------------------------------------
// Audio utilities
// -------------------------------------------------------------------

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// parseWorkerURLs splits comma-separated URLs into a pool list.
// Single URL: "http://whisper:8000" → ["http://whisper:8000"]
// Multiple:  "http://whisper-1:8000,http://whisper-2:8000" → pool of 2
func parseWorkerURLs(urls string) []string {
	parts := strings.Split(urls, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func piiTypes(detections []PIIDetection) string {
	types := make([]string, len(detections))
	for i, d := range detections {
		types[i] = d.Type
	}
	return strings.Join(types, ", ")
}

// isWhisperHallucination detects common Whisper false positives from silence/noise.
// Only filters exact hallucination patterns — real phrases like "Thank you very much" pass through.
func isWhisperHallucination(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	cleaned := strings.NewReplacer(".", "", ",", "", "!", "", "?", "", "'", "").Replace(lower)
	cleaned = strings.TrimSpace(cleaned)

	// Empty after cleanup
	if cleaned == "" {
		return true
	}

	// Dots/ellipsis only: ". . . . ." or "... ... ..."
	noDots := strings.NewReplacer(".", "", " ", "").Replace(lower)
	if len(noDots) == 0 {
		return true
	}

	// Exact match hallucinations (1-3 word phrases that Whisper generates from silence)
	exact := []string{
		"thank you", "thanks", "thanks for watching", "thanks for listening",
		"bye", "goodbye", "you", "okay", "oh", "yeah", "yes", "no",
		"hmm", "uh", "um", "so", "i dont know",
		"all right", "alright", "right", "sure",
	}
	for _, h := range exact {
		if cleaned == h {
			return true
		}
	}

	// Repetition: any phrase repeated 3+ times
	repeats := []string{
		"thank you", "all right", "alright", "okay", "good",
		"yeah", "yes", "no", "day", "i'm sorry", "sorry",
		"we'll be right back", "ladies and gentlemen",
		"i don't know", "hello", "we're",
	}
	for _, h := range repeats {
		if strings.Count(lower, h) >= 3 {
			return true
		}
	}

	// Generic repetition: any single word is >50% of all words and appears 4+ times
	words := strings.Fields(cleaned)
	if len(words) >= 4 {
		counts := make(map[string]int)
		for _, w := range words {
			counts[w]++
		}
		for _, c := range counts {
			if c >= 4 && float64(c)/float64(len(words)) > 0.4 {
				return true
			}
		}
	}

	return false
}

// rmsEnergy computes the root-mean-square energy of 16-bit LE PCM samples.
func rmsEnergy(pcm []byte) float64 {
	n := len(pcm) / bytesPerSample
	if n == 0 {
		return 0
	}
	var sum float64
	for i := 0; i < n; i++ {
		s := int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2]))
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(n))
}

// buildWAV wraps raw PCM in a standard 44-byte RIFF/WAV header.
func buildWAV(pcm []byte, rate, channels, bitsPerSample int) []byte {
	dataLen := len(pcm)
	byteRate := rate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8

	buf := make([]byte, 44+dataLen)
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(36+dataLen))
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16) // PCM chunk size
	binary.LittleEndian.PutUint16(buf[20:22], 1)  // PCM format
	binary.LittleEndian.PutUint16(buf[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(buf[24:28], uint32(rate))
	binary.LittleEndian.PutUint32(buf[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(buf[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(buf[34:36], uint16(bitsPerSample))
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(dataLen))
	copy(buf[44:], pcm)
	return buf
}

// stripWAVHeader removes the RIFF/WAV header and returns raw PCM.
func stripWAVHeader(wav []byte) []byte {
	if len(wav) < 44 || string(wav[:4]) != "RIFF" {
		return wav
	}
	// Find the "data" chunk
	pos := 12
	for pos+8 < len(wav) {
		chunkID := string(wav[pos : pos+4])
		chunkSize := int(binary.LittleEndian.Uint32(wav[pos+4 : pos+8]))
		if chunkID == "data" {
			end := pos + 8 + chunkSize
			if end > len(wav) {
				end = len(wav)
			}
			return wav[pos+8 : end]
		}
		pos += 8 + chunkSize
	}
	return wav[44:]
}

// resample performs linear interpolation between sample rates.
// Operates on raw L16 (16-bit signed LE) sample buffers.
func resample(pcm []byte, fromRate, toRate int) []byte {
	if fromRate == toRate || len(pcm) < 2 {
		return pcm
	}

	nIn := len(pcm) / 2
	in := make([]int16, nIn)
	for i := range in {
		in[i] = int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2]))
	}

	nOut := int(float64(nIn) * float64(toRate) / float64(fromRate))
	out := make([]byte, nOut*2)
	ratio := float64(fromRate) / float64(toRate)

	for i := 0; i < nOut; i++ {
		pos := float64(i) * ratio
		idx := int(pos)
		frac := pos - float64(idx)

		var s int16
		if idx+1 < nIn {
			s = int16(float64(in[idx])*(1.0-frac) + float64(in[idx+1])*frac)
		} else if idx < nIn {
			s = in[idx]
		}
		binary.LittleEndian.PutUint16(out[i*2:i*2+2], uint16(s))
	}

	return out
}

// -------------------------------------------------------------------
// ESL client — lightweight FreeSWITCH Event Socket Library client
//
// ESL is a simple line-based TCP protocol:
//   1. Connect to FS ESL port
//   2. Receive "Content-Type: auth/request"
//   3. Send "auth <password>\n\n"
//   4. Receive "+OK accepted"
//   5. Send commands: "api <cmd>\n\n" or "bgapi <cmd>\n\n"
//   6. Read responses until blank line
// -------------------------------------------------------------------

type eslClient struct {
	host     string
	port     string
	password string
}

func (e *eslClient) execute(command string) (string, error) {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(e.host, e.port), 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("esl connect: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	reader := bufio.NewReader(conn)

	// Read auth request
	if _, err := readESLResponse(reader); err != nil {
		return "", fmt.Errorf("esl auth request: %w", err)
	}

	// Authenticate
	fmt.Fprintf(conn, "auth %s\n\n", e.password)
	authResp, err := readESLResponse(reader)
	if err != nil {
		return "", fmt.Errorf("esl auth: %w", err)
	}
	if !strings.Contains(authResp, "+OK") {
		return "", fmt.Errorf("esl auth rejected: %s", authResp)
	}

	// Send command
	fmt.Fprintf(conn, "api %s\n\n", command)
	return readESLResponse(reader)
}

func readESLResponse(r *bufio.Reader) (string, error) {
	var resp strings.Builder
	contentLen := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return resp.String(), err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			fmt.Sscanf(line, "Content-Length: %d", &contentLen)
		}
		resp.WriteString(line)
		resp.WriteString("\n")
	}
	if contentLen > 0 {
		body := make([]byte, contentLen)
		if _, err := io.ReadFull(r, body); err != nil {
			return resp.String(), err
		}
		resp.Write(body)
	}
	return resp.String(), nil
}

// -------------------------------------------------------------------
// POST /call — outbound call origination
//
// Request:
//   POST /call
//   {"to": "+15551234567", "from": "+15559876543", "system_prompt": "..."}
//
// The gateway tells FreeSWITCH (via ESL) to originate an outbound call
// through the SBC gateway. Once answered, FS runs the dialplan which
// invokes uuid_audio_fork, connecting back to our /ws endpoint.
// -------------------------------------------------------------------

type callRequest struct {
	To           string `json:"to"`
	From         string `json:"from"`
	Mode         string `json:"mode,omitempty"` // "sbc" (default) or "loopback"
	SystemPrompt string `json:"system_prompt,omitempty"`
}

type callResponse struct {
	Status string `json:"status"`
	CallID string `json:"call_id,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (gw *gateway) handleCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req callRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, callResponse{Status: "error", Error: "invalid JSON"})
		return
	}
	if req.To == "" {
		writeJSON(w, http.StatusBadRequest, callResponse{Status: "error", Error: "to is required"})
		return
	}

	from := req.From
	if from == "" {
		from = "0000000000"
	}

	slog.Info("originating call", "to", req.To, "from", from)

	esl := &eslClient{
		host:     gw.cfg.ESLHost,
		port:     gw.cfg.ESLPort,
		password: gw.cfg.ESLPassword,
	}

	var originateCmd string
	if req.Mode == "loopback" {
		// Loopback: call ourselves via the external SIP profile.
		// The call re-enters the public context and runs the audio_fork dialplan.
		originateCmd = fmt.Sprintf(
			"originate {origination_caller_id_number=%s,origination_caller_id_name=AI-Agent,sip_auto_answer=true}sofia/external/%s@172.18.0.3:5060 &park",
			from, req.To,
		)
	} else {
		// SBC: originate through the SBC gateway trunk, then transfer to public context.
		originateCmd = fmt.Sprintf(
			"originate {origination_caller_id_number=%s,origination_caller_id_name=AI-Agent}sofia/gateway/sbc/%s &transfer(%s XML public)",
			from, req.To, req.To,
		)
	}

	resp, err := esl.execute(originateCmd)
	if err != nil {
		slog.Error("esl originate failed", "err", err)
		writeJSON(w, http.StatusBadGateway, callResponse{Status: "error", Error: err.Error()})
		return
	}

	// Parse the response — look for +OK <uuid> or -ERR <reason>
	resp = strings.TrimSpace(resp)
	// Find the last meaningful line
	lines := strings.Split(resp, "\n")
	result := strings.TrimSpace(lines[len(lines)-1])

	if strings.Contains(result, "-ERR") {
		errMsg := strings.TrimPrefix(result, "-ERR ")
		slog.Error("originate rejected", "err", errMsg)
		writeJSON(w, http.StatusBadGateway, callResponse{Status: "error", Error: errMsg})
		return
	}

	callID := strings.TrimPrefix(result, "+OK ")
	callID = strings.TrimSpace(callID)

	slog.Info("call originated", "call_id", callID, "to", req.To)
	writeJSON(w, http.StatusOK, callResponse{Status: "originated", CallID: callID})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
