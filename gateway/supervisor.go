package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

type SupervisorSession struct {
	callID     string
	mode       string // listen, whisper, barge
	pc         *webrtc.PeerConnection
	outTrack   *webrtc.TrackLocalStaticSample
	callerTap  chan []byte
	agentTap   chan []byte
	siprecSess *siprecSession
	cancel     context.CancelFunc
	log        *slog.Logger
}

var (
	supervisorMu       sync.Mutex
	supervisorSessions = make(map[string]*SupervisorSession) // keyed by callID+userID
)

type supervisorMonitorRequest struct {
	CallID string `json:"call_id"`
	Mode   string `json:"mode"` // listen, whisper, barge
	SDP    string `json:"sdp"`
}

type supervisorStopRequest struct {
	CallID string `json:"call_id"`
}

func (gw *gateway) registerSupervisorRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/supervisor/monitor", gw.handleSupervisorMonitor)
	mux.HandleFunc("/api/supervisor/stop", gw.handleSupervisorStop)
	mux.HandleFunc("/api/supervisor/calls", gw.handleSupervisorCalls)
}

func (gw *gateway) handleSupervisorCalls(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"GET required"}`, http.StatusMethodNotAllowed)
		return
	}

	siprecSessionsMu.Lock()
	var calls []map[string]any
	for _, s := range siprecSessions {
		entry := map[string]any{
			"call_id":    s.callID,
			"started_at": s.startTime.Format(time.RFC3339),
			"duration":   int(time.Since(s.startTime).Seconds()),
			"caller":     s.callerNumber,
			"agent":      s.agentNumber,
		}
		if s.voiceSentiment != nil {
			vs := s.voiceSentiment.Analyze()
			entry["voice_sentiment"] = vs
		}

		s.convMu.Lock()
		if len(s.conversation) > 0 {
			last := s.conversation[len(s.conversation)-1]
			entry["last_utterance"] = map[string]any{
				"speaker": last.Speaker,
				"text":    last.Text,
			}
		}
		entry["utterance_count"] = len(s.conversation)
		s.convMu.Unlock()

		// Check if being monitored
		supervisorMu.Lock()
		monitored := false
		for k := range supervisorSessions {
			if len(k) > len(s.callID) && k[:len(s.callID)] == s.callID {
				monitored = true
				break
			}
		}
		supervisorMu.Unlock()
		entry["monitored"] = monitored

		calls = append(calls, entry)
	}
	siprecSessionsMu.Unlock()

	if calls == nil {
		calls = []map[string]any{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(calls)
}

func (gw *gateway) handleSupervisorMonitor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}

	var req supervisorMonitorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.CallID == "" || req.Mode == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "call_id and mode required"})
		return
	}

	userID := getUserIDFromRequest(r)
	if userID == "" {
		userID = "supervisor"
	}

	siprecSessionsMu.Lock()
	copilot, ok := siprecSessions[req.CallID]
	siprecSessionsMu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "call not found"})
		return
	}

	log := slog.With("call_id", req.CallID, "supervisor", userID, "mode", req.Mode)

	if req.Mode == "barge" {
		cs, err := startConference(gw, req.CallID, "agent", userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		log.Info("supervisor barge started")
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "monitoring",
			"mode":    "barge",
			"call_id": cs.callID,
		})
		return
	}

	// Listen or Whisper — create WebRTC session
	if req.SDP == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sdp required for listen/whisper"})
		return
	}

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}

	pc, err := newPCMUPeerConnection(config)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "peer connection failed"})
		return
	}

	outTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000, Channels: 1},
		"audio", "supervisor-monitor",
	)
	if err != nil {
		pc.Close()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "track failed"})
		return
	}
	if _, err = pc.AddTrack(outTrack); err != nil {
		pc.Close()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "add track failed"})
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	sess := &SupervisorSession{
		callID:     req.CallID,
		mode:       req.Mode,
		pc:         pc,
		outTrack:   outTrack,
		siprecSess: copilot,
		cancel:     cancel,
		log:        log,
	}

	// Whisper: capture supervisor's mic and feed to agent
	if req.Mode == "whisper" {
		if copilot.whisperCh == nil {
			copilot.whisperCh = make(chan []byte, pcmChanBufSize)
		}

		pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
			log.Info("supervisor whisper track received")
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					default:
					}
					pkt, _, err := track.ReadRTP()
					if err != nil {
						return
					}
					if len(pkt.Payload) == 0 {
						continue
					}
					pcm8k := DecodeG711Ulaw(pkt.Payload)
					pcm16k := resample(pcm8k, 8000, 16000)
					select {
					case copilot.whisperCh <- pcm16k:
					default:
					}
				}
			}()
		})
	}

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Info("supervisor ICE state", "state", state.String())
		if state == webrtc.ICEConnectionStateFailed || state == webrtc.ICEConnectionStateClosed {
			stopSupervisorSession(req.CallID, userID)
		}
	})

	offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: req.SDP}
	if err := pc.SetRemoteDescription(offer); err != nil {
		cancel()
		pc.Close()
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid SDP"})
		return
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		cancel()
		pc.Close()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "answer failed"})
		return
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		cancel()
		pc.Close()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "set local desc failed"})
		return
	}

	<-webrtc.GatheringCompletePromise(pc)

	// Tap caller and agent audio, mix, and send to supervisor
	callerTap := copilot.AddAudioTap()
	agentTap := copilot.AddAgentTap()
	sess.callerTap = callerTap
	sess.agentTap = agentTap

	go supervisorAudioLoop(ctx, callerTap, agentTap, outTrack, log)

	sessKey := req.CallID + ":" + userID
	supervisorMu.Lock()
	supervisorSessions[sessKey] = sess
	supervisorMu.Unlock()

	log.Info("supervisor monitoring started")

	copilot.broadcastSSE(map[string]any{
		"type": "supervisor_monitoring",
		"mode": req.Mode,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "monitoring",
		"mode":   req.Mode,
		"sdp":    pc.LocalDescription().SDP,
		"type":   pc.LocalDescription().Type.String(),
	})
}

func supervisorAudioLoop(ctx context.Context, callerTap, agentTap chan []byte, outTrack *webrtc.TrackLocalStaticSample, log *slog.Logger) {
	const frameSize = 640 // 20ms at 16kHz L16
	silence := make([]byte, frameSize)

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		var callerFrame, agentFrame []byte

		select {
		case f := <-callerTap:
			callerFrame = f
		default:
			callerFrame = silence
		}

		select {
		case f := <-agentTap:
			agentFrame = f
		default:
			agentFrame = silence
		}

		// Mix caller + agent for supervisor
		mixed := make([]byte, frameSize)
		nSamples := frameSize / 2
		for i := 0; i < nSamples; i++ {
			var sum int32
			if i*2+1 < len(callerFrame) {
				sum += int32(int16(binary.LittleEndian.Uint16(callerFrame[i*2 : i*2+2])))
			}
			if i*2+1 < len(agentFrame) {
				sum += int32(int16(binary.LittleEndian.Uint16(agentFrame[i*2 : i*2+2])))
			}
			if sum > 32767 {
				sum = 32767
			} else if sum < -32768 {
				sum = -32768
			}
			binary.LittleEndian.PutUint16(mixed[i*2:i*2+2], uint16(int16(sum)))
		}

		// Downsample 16k → 8k, encode G.711, send to WebRTC
		pcm8k := resample(mixed, 16000, 8000)
		ulaw := EncodeG711Ulaw(pcm8k)

		outTrack.WriteSample(media.Sample{
			Data:     ulaw,
			Duration: 20 * time.Millisecond,
		})
	}
}

func (gw *gateway) handleSupervisorStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}

	var req supervisorStopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	userID := getUserIDFromRequest(r)
	if userID == "" {
		userID = "supervisor"
	}

	stopSupervisorSession(req.CallID, userID)

	// Also end conference if barge was active
	conferencesMu.Lock()
	_, hasConf := conferences[req.CallID]
	conferencesMu.Unlock()
	if hasConf {
		dropThirdParty(req.CallID)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func stopSupervisorSession(callID, userID string) {
	sessKey := callID + ":" + userID
	supervisorMu.Lock()
	sess, ok := supervisorSessions[sessKey]
	if ok {
		delete(supervisorSessions, sessKey)
	}
	supervisorMu.Unlock()

	if !ok {
		return
	}

	sess.cancel()

	if sess.callerTap != nil && sess.siprecSess != nil {
		sess.siprecSess.RemoveAudioTap(sess.callerTap)
	}
	if sess.agentTap != nil && sess.siprecSess != nil {
		sess.siprecSess.RemoveAgentTap(sess.agentTap)
	}

	if sess.mode == "whisper" && sess.siprecSess != nil {
		sess.siprecSess.whisperCh = nil
	}

	if sess.pc != nil {
		sess.pc.Close()
	}

	sess.log.Info("supervisor monitoring stopped")

	if sess.siprecSess != nil {
		sess.siprecSess.broadcastSSE(map[string]any{
			"type": "supervisor_stopped",
		})
	}
}

// Ensure imports are used
var _ = fmt.Sprintf
