package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

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

	// Create output audio track (gateway → browser)
	outTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
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
		sess.close()
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok"}`)
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

	// Send outgoing audio (TTS/response) back to browser
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case pcm, ok := <-s.pcmOut:
				if !ok {
					return
				}
				// Write PCM as Opus sample to the output track
				if err := s.outTrack.WriteSample(media.Sample{
					Data:     pcm,
					Duration: 20 * time.Millisecond,
				}); err != nil {
					s.log.Debug("write sample", "err", err)
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
