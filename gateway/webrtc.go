package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

type WebRTCManager struct {
	gw       *gateway
	sessions map[string]*WebRTCSession
	mu       sync.Mutex
}

type WebRTCSession struct {
	callID     string
	agentID    string
	pc         *webrtc.PeerConnection
	outTrack   *webrtc.TrackLocalStaticSample
	pcmIn      chan []byte
	pcmOut     chan []byte
	cancel     context.CancelFunc
	startTime  time.Time
	log        *slog.Logger
	onHold     bool
}

// newPCMUPeerConnection creates a PeerConnection that only supports PCMU codec.
// This forces the browser to send PCMU (not Opus), so we can decode without CGO.
func newPCMUPeerConnection(config webrtc.Configuration) (*webrtc.PeerConnection, error) {
	me := &webrtc.MediaEngine{}
	if err := me.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMU,
			ClockRate: 8000,
			Channels:  1,
		},
		PayloadType: 0,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, err
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(me))
	return api.NewPeerConnection(config)
}

func NewWebRTCManager(gw *gateway) *WebRTCManager {
	return &WebRTCManager{
		gw:       gw,
		sessions: make(map[string]*WebRTCSession),
	}
}

func (wm *WebRTCManager) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/webrtc/offer", wm.handleOffer)
	mux.HandleFunc("/api/webrtc/candidate", wm.handleCandidate)
	mux.HandleFunc("/api/webrtc/hangup", wm.handleHangup)
	mux.HandleFunc("/api/webrtc/sessions", wm.handleSessions)
	mux.HandleFunc("/api/webrtc/bridge", wm.handleBridge)
}

// handleBridge connects an agent's WebRTC to an existing SIPREC session.
// Agent hears the caller's audio through their browser.
func (wm *WebRTCManager) handleBridge(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SDP        string `json:"sdp"`
		Type       string `json:"type"`
		AgentID    string `json:"agent_id"`
		SIPRECCallID string `json:"siprec_call_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.SIPRECCallID == "" {
		http.Error(w, "siprec_call_id required", http.StatusBadRequest)
		return
	}

	// Find the existing SIPREC session
	siprecSessionsMu.Lock()
	siprecSess, exists := siprecSessions[req.SIPRECCallID]
	siprecSessionsMu.Unlock()

	if !exists {
		http.Error(w, "SIPREC session not found", http.StatusNotFound)
		return
	}

	callID := "bridge-" + req.SIPRECCallID
	log := slog.With("call_id", callID, "siprec", req.SIPRECCallID, "agent", req.AgentID)
	log.Info("WebRTC bridge request")

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}

	// PCMU-only PC so browser sends PCMU (decodable without Opus/CGO)
	pc, err := newPCMUPeerConnection(config)
	if err != nil {
		log.Error("peer connection", "err", err)
		http.Error(w, "peer connection failed", http.StatusInternalServerError)
		return
	}

	// Use PCMU (G.711 μ-law) — no external encoder dependency, all browsers support it
	outTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000, Channels: 1},
		"audio", "voiceagent-bridge",
	)
	if err != nil {
		pc.Close()
		http.Error(w, "track creation failed", http.StatusInternalServerError)
		return
	}
	if _, err = pc.AddTrack(outTrack); err != nil {
		pc.Close()
		http.Error(w, "add track failed", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	sess := &WebRTCSession{
		callID:    callID,
		agentID:   req.AgentID,
		pc:        pc,
		outTrack:  outTrack,
		pcmIn:     make(chan []byte, pcmChanBufSize),
		pcmOut:    make(chan []byte, 20),
		cancel:    cancel,
		startTime: time.Now(),
		log:       log,
	}

	wm.mu.Lock()
	wm.sessions[callID] = sess
	wm.mu.Unlock()

	// Agent's mic audio → decode PCMU → send back to caller via RTP or WebSocket + copilot
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Info("agent audio track received", "codec", track.Codec().MimeType,
			"rtp_path", siprecSess.rtpSession != nil,
			"ws_path", siprecSess.callerConn != nil)
		go func() {
			agentFrames := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				pkt, _, err := track.ReadRTP()
				if err != nil {
					log.Info("agent track read ended", "err", err, "frames_sent", agentFrames)
					return
				}
				if len(pkt.Payload) == 0 || sess.onHold {
					continue
				}

				// PCMU payload → L16 PCM 8kHz → resample to 16kHz
				pcm8k := DecodeG711Ulaw(pkt.Payload)
				pcm16k := resample(pcm8k, 8000, 16000)

				agentFrames++

				// Send agent voice to caller: prefer RTP path (standalone), fall back to WebSocket
				if siprecSess.rtpSession != nil && siprecSess.rtpSession.listener != nil {
					if err := siprecSess.rtpSession.listener.SendPCM(pcm16k); err != nil {
						if agentFrames == 1 {
							log.Error("RTP send failed", "err", err)
						}
					}
				} else if siprecSess.callerConn != nil {
					if err := siprecSess.callerConn.WriteMessage(websocket.BinaryMessage, pcm16k); err != nil {
						log.Debug("write to callerConn", "err", err)
					}
				}

				// Feed to copilot agent channel for transcription
				select {
				case siprecSess.pcmAgent <- pcm16k:
				default:
				}
			}
		}()
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Info("ICE state", "state", state.String())
		if state == webrtc.ICEConnectionStateFailed || state == webrtc.ICEConnectionStateDisconnected {
			sess.close()
			wm.mu.Lock()
			delete(wm.sessions, callID)
			wm.mu.Unlock()
		}
	})

	offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: req.SDP}
	if err := pc.SetRemoteDescription(offer); err != nil {
		cancel(); pc.Close()
		http.Error(w, "invalid SDP", http.StatusBadRequest)
		return
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		cancel(); pc.Close()
		http.Error(w, "answer failed", http.StatusInternalServerError)
		return
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		cancel(); pc.Close()
		http.Error(w, "set local desc failed", http.StatusInternalServerError)
		return
	}

	<-webrtc.GatheringCompletePromise(pc)

	// Bridge: tap caller audio → downsample → G.711 μ-law → WebRTC agent speaker
	callerTap := siprecSess.AddAudioTap()
	go func() {
		defer siprecSess.RemoveAudioTap(callerTap)

		const frame16kSize = 640 // 20ms at 16kHz L16 (320 samples × 2 bytes)
		var frameBuf []byte

		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-callerTap:
				if !ok {
					return
				}
				// Skip forwarding caller audio when on hold
				if sess.onHold {
					continue
				}
				frameBuf = append(frameBuf, frame...)

				for len(frameBuf) >= frame16kSize {
					chunk := frameBuf[:frame16kSize]
					frameBuf = frameBuf[frame16kSize:]

					pcm8k := resample(chunk, 16000, 8000)
					ulaw := EncodeG711Ulaw(pcm8k)

					if err := outTrack.WriteSample(media.Sample{
						Data:     ulaw,
						Duration: 20 * time.Millisecond,
					}); err != nil {
						log.Debug("write to agent", "err", err)
					}
				}
			}
		}
	}()

	log.Info("WebRTC bridge established (bidirectional)")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"sdp":     pc.LocalDescription().SDP,
		"type":    pc.LocalDescription().Type.String(),
		"call_id": callID,
	})
}

func (wm *WebRTCManager) handleOffer(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SDP     string `json:"sdp"`
		Type    string `json:"type"`
		AgentID string `json:"agent_id"`
		Target  string `json:"target"`
		CallID  string `json:"call_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	callID := req.CallID
	if callID == "" {
		callID = fmt.Sprintf("webrtc-%d", time.Now().UnixMilli())
	}

	log := slog.With("call_id", callID, "agent", req.AgentID)
	log.Info("WebRTC offer received", "target", req.Target)

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Error("create peer connection", "err", err)
		http.Error(w, "peer connection failed", http.StatusInternalServerError)
		return
	}

	// Create output audio track (gateway → browser) — use PCMU for zero-dependency encoding
	outTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000, Channels: 1},
		"audio", "voiceagent",
	)
	if err != nil {
		log.Error("create output track", "err", err)
		pc.Close()
		http.Error(w, "track creation failed", http.StatusInternalServerError)
		return
	}
	if _, err = pc.AddTrack(outTrack); err != nil {
		log.Error("add output track", "err", err)
		pc.Close()
		http.Error(w, "add track failed", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	sess := &WebRTCSession{
		callID:    callID,
		agentID:   req.AgentID,
		pc:        pc,
		outTrack:  outTrack,
		pcmIn:     make(chan []byte, pcmChanBufSize),
		pcmOut:    make(chan []byte, 20),
		cancel:    cancel,
		startTime: time.Now(),
		log:       log,
	}

	wm.mu.Lock()
	wm.sessions[callID] = sess
	wm.mu.Unlock()

	// Handle incoming audio from browser
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Info("WebRTC audio track received", "codec", track.Codec().MimeType)
		go sess.readIncomingAudio(ctx, track)
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Info("ICE state", "state", state.String())
		if state == webrtc.ICEConnectionStateFailed || state == webrtc.ICEConnectionStateDisconnected {
			sess.close()
			wm.mu.Lock()
			delete(wm.sessions, callID)
			wm.mu.Unlock()
		}
	})

	// Set remote description (browser's offer)
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  req.SDP,
	}
	if err := pc.SetRemoteDescription(offer); err != nil {
		log.Error("set remote desc", "err", err)
		cancel()
		pc.Close()
		http.Error(w, "invalid SDP offer", http.StatusBadRequest)
		return
	}

	// Create answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Error("create answer", "err", err)
		cancel()
		pc.Close()
		http.Error(w, "answer creation failed", http.StatusInternalServerError)
		return
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		log.Error("set local desc", "err", err)
		cancel()
		pc.Close()
		http.Error(w, "set local desc failed", http.StatusInternalServerError)
		return
	}

	// Wait for ICE gathering
	<-webrtc.GatheringCompletePromise(pc)

	// Start the AI pipeline for this session
	copilot := getOrCreateSIPRECSession(wm.gw, callID)
	copilot.callerNumber = req.Target
	copilot.agentNumber = req.AgentID

	go sess.pipelineWorker(ctx, copilot)

	log.Info("WebRTC session established", "target", req.Target)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"sdp":     pc.LocalDescription().SDP,
		"type":    pc.LocalDescription().Type.String(),
		"call_id": callID,
	})
}

func (wm *WebRTCManager) handleCandidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CallID    string `json:"call_id"`
		Candidate string `json:"candidate"`
		SDPMid    string `json:"sdpMid"`
		SDPMLine  uint16 `json:"sdpMLineIndex"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	wm.mu.Lock()
	sess, ok := wm.sessions[req.CallID]
	wm.mu.Unlock()

	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	candidate := webrtc.ICECandidateInit{
		Candidate: req.Candidate,
		SDPMid:    &req.SDPMid,
	}
	if err := sess.pc.AddICECandidate(candidate); err != nil {
		slog.Error("add ICE candidate", "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok"}`)
}

func (wm *WebRTCManager) handleHangup(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CallID string `json:"call_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	wm.mu.Lock()
	sess, ok := wm.sessions[req.CallID]
	if ok {
		delete(wm.sessions, req.CallID)
	}
	wm.mu.Unlock()

	if ok {
		sess.log.Info("WebRTC hangup")
		// Update agent state: active_calls--, status=Available
		if wm.gw != nil && wm.gw.acd != nil && sess.agentID != "" {
			wm.gw.acd.OnCallEnd(sess.agentID)
		}
		sess.close()
	}

	// For bridge calls, also terminate the underlying SIPREC/SIP session
	siprecCallID := strings.TrimPrefix(req.CallID, "bridge-")
	if siprecCallID != req.CallID {
		slog.Info("ending SIPREC session from bridge hangup", "siprec_call_id", siprecCallID)

		// Send SIP BYE to caller's phone
		if wm.gw.sipServer != nil {
			wm.gw.sipServer.SendBYE(siprecCallID)
		}

		siprecSessionsMu.Lock()
		siprecSess, exists := siprecSessions[siprecCallID]
		siprecSessionsMu.Unlock()

		if exists {
			siprecSess.cancel()

			// Close RTP session (standalone SIP path)
			if siprecSess.rtpSession != nil {
				if siprecSess.rtpSession.listener != nil {
					siprecSess.rtpSession.listener.Close()
				}
				siprecSess.rtpSession.cancelFunc()
				slog.Info("RTP session closed on hangup", "call_id", siprecCallID)
			}

			// Close WebSocket legs (FS audio_fork path)
			if siprecSess.callerConn != nil {
				siprecSess.callerConn.Close()
			}
			if siprecSess.agentConn != nil {
				siprecSess.agentConn.Close()
			}
		}

		// Try ESL uuid_kill (works when FS is in the path)
		if wm.gw != nil {
			esl := wm.gw.newESLClient()
			cmd := fmt.Sprintf("uuid_kill %s", siprecCallID)
			if resp, err := esl.execute(cmd); err != nil {
				slog.Debug("esl uuid_kill (FS may not be running)", "call_id", siprecCallID, "err", err)
			} else {
				slog.Info("call terminated via ESL", "call_id", siprecCallID, "resp", resp)
			}
		}

		// Remove from queue
		if wm.gw != nil && wm.gw.queueMgr != nil {
			wm.gw.queueMgr.RemoveCallerByCallID(siprecCallID)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (wm *WebRTCManager) handleSessions(w http.ResponseWriter, r *http.Request) {
	wm.mu.Lock()
	sessions := make([]map[string]any, 0, len(wm.sessions))
	for id, s := range wm.sessions {
		sessions = append(sessions, map[string]any{
			"call_id":  id,
			"agent_id": s.agentID,
			"duration": int(time.Since(s.startTime).Seconds()),
		})
	}
	wm.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

// readIncomingAudio reads Opus RTP from browser, decodes to PCM, feeds pipeline
func (s *WebRTCSession) readIncomingAudio(ctx context.Context, track *webrtc.TrackRemote) {
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, _, err := track.Read(buf)
		if err != nil {
			s.log.Info("WebRTC track read ended", "err", err)
			return
		}

		if n > 0 {
			frame := make([]byte, n)
			copy(frame, buf[:n])
			select {
			case s.pcmIn <- frame:
			default:
			}
		}
	}
}

// pipelineWorker bridges WebRTC audio with the copilot/AI pipeline
func (s *WebRTCSession) pipelineWorker(ctx context.Context, copilot *siprecSession) {
	s.log.Info("WebRTC pipeline started")

	// Feed incoming audio (from browser mic) into copilot caller channel
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-s.pcmIn:
				if !ok {
					return
				}
				select {
				case copilot.pcmCaller <- frame:
				default:
				}
			}
		}
	}()

	// Send outgoing audio (TTS/response) back to browser as PCMU
	go func() {
		const frame16kSize = 640 // 20ms at 16kHz L16
		var frameBuf []byte

		for {
			select {
			case <-ctx.Done():
				return
			case pcm, ok := <-s.pcmOut:
				if !ok {
					return
				}
				frameBuf = append(frameBuf, pcm...)
				for len(frameBuf) >= frame16kSize {
					chunk := frameBuf[:frame16kSize]
					frameBuf = frameBuf[frame16kSize:]
					pcm8k := resample(chunk, 16000, 8000)
					ulaw := EncodeG711Ulaw(pcm8k)
					if err := s.outTrack.WriteSample(media.Sample{
						Data:     ulaw,
						Duration: 20 * time.Millisecond,
					}); err != nil {
						s.log.Debug("write sample", "err", err)
					}
				}
			}
		}
	}()

	<-ctx.Done()
	s.log.Info("WebRTC pipeline ended")
}

func (s *WebRTCSession) close() {
	s.cancel()
	if s.pc != nil {
		s.pc.Close()
	}
	close(s.pcmIn)
}
