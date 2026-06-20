package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4/pkg/media"
)

const (
	mixFrameBytes = 640 // 320 samples × 2 bytes = 20ms at 16kHz
	mixTickMs     = 20
	mixChanBuf    = 50
)

type mixParticipant struct {
	name   string
	pcmIn  chan []byte
	pcmOut chan []byte
	active bool
}

type AudioMixer struct {
	participants [3]*mixParticipant
	cancel       context.CancelFunc
	log          *slog.Logger
}

type ConferenceSession struct {
	callID    string
	mixer     *AudioMixer
	siprecSes *siprecSession
	agentWRTC *WebRTCSession
	thirdType string // "agent" or "external"
	thirdWRTC *WebRTCSession      // set if thirdType == "agent"
	thirdRTP  *siprecRTPSession   // set if thirdType == "external"
	thirdID   string
	gw        *gateway
	cancel    context.CancelFunc
	log       *slog.Logger
}

var (
	conferencesMu sync.Mutex
	conferences   = make(map[string]*ConferenceSession)
)

func newAudioMixer(log *slog.Logger) *AudioMixer {
	return &AudioMixer{
		participants: [3]*mixParticipant{
			{name: "caller", pcmIn: make(chan []byte, mixChanBuf), pcmOut: make(chan []byte, mixChanBuf), active: true},
			{name: "agent", pcmIn: make(chan []byte, mixChanBuf), pcmOut: make(chan []byte, mixChanBuf), active: true},
			{name: "third", pcmIn: make(chan []byte, mixChanBuf), pcmOut: make(chan []byte, mixChanBuf), active: false},
		},
		log: log,
	}
}

func (m *AudioMixer) run(ctx context.Context) {
	ticker := time.NewTicker(mixTickMs * time.Millisecond)
	defer ticker.Stop()

	silence := make([]byte, mixFrameBytes)
	frames := [3][]byte{silence, silence, silence}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		for i, p := range m.participants {
			if !p.active {
				frames[i] = silence
				continue
			}
			select {
			case f := <-p.pcmIn:
				frames[i] = f
			default:
				frames[i] = silence
			}
		}

		for i, p := range m.participants {
			if !p.active {
				continue
			}
			mixed := mixExcluding(frames, i)
			select {
			case p.pcmOut <- mixed:
			default:
			}
		}
	}
}

func mixExcluding(frames [3][]byte, exclude int) []byte {
	out := make([]byte, mixFrameBytes)
	nSamples := mixFrameBytes / 2

	for s := 0; s < nSamples; s++ {
		var sum int32
		for i := 0; i < 3; i++ {
			if i == exclude {
				continue
			}
			if s*2+1 < len(frames[i]) {
				sum += int32(int16(binary.LittleEndian.Uint16(frames[i][s*2 : s*2+2])))
			}
		}
		if sum > 32767 {
			sum = 32767
		} else if sum < -32768 {
			sum = -32768
		}
		binary.LittleEndian.PutUint16(out[s*2:s*2+2], uint16(int16(sum)))
	}
	return out
}

func startConference(gw *gateway, callID, targetType, targetValue string) (*ConferenceSession, error) {
	siprecSessionsMu.Lock()
	copilot, ok := siprecSessions[callID]
	siprecSessionsMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("siprec session not found: %s", callID)
	}

	bridgeCallID := "bridge-" + callID
	var agentSess *WebRTCSession
	if gw.webrtcMgr != nil {
		gw.webrtcMgr.mu.Lock()
		agentSess = gw.webrtcMgr.sessions[bridgeCallID]
		gw.webrtcMgr.mu.Unlock()
	}

	log := slog.With("call_id", callID, "conference", true)
	ctx, cancel := context.WithCancel(context.Background())

	mixer := newAudioMixer(log)
	go mixer.run(ctx)

	cs := &ConferenceSession{
		callID:    callID,
		mixer:     mixer,
		siprecSes: copilot,
		agentWRTC: agentSess,
		thirdType: targetType,
		thirdID:   targetValue,
		gw:        gw,
		cancel:    cancel,
		log:       log,
	}

	copilot.conference = cs

	// Start output goroutines for caller and agent
	go cs.callerOutputLoop(ctx)
	go cs.agentOutputLoop(ctx)

	// Add third party
	switch targetType {
	case "agent":
		if gw.acd != nil && gw.acd.agentMgr != nil {
			callerNum := copilot.callerNumber
			if callerNum == "" {
				callerNum = "Conference"
			}
			gw.acd.agentMgr.RingAgent(targetValue, callID, callerNum, "Conference", 0)
		}
		log.Info("conference: ringing third-party agent", "target", targetValue)

	case "external":
		go cs.dialExternalThird(ctx, targetValue)
	}

	conferencesMu.Lock()
	conferences[callID] = cs
	conferencesMu.Unlock()

	copilot.broadcastSSE(map[string]any{
		"type":        "conference_started",
		"call_id":     callID,
		"third_party": targetValue,
		"third_type":  targetType,
	})

	log.Info("conference started", "target_type", targetType, "target", targetValue)
	return cs, nil
}

func (cs *ConferenceSession) activateThirdAgent(webrtcSess *WebRTCSession) {
	cs.thirdWRTC = webrtcSess
	cs.mixer.participants[2].active = true
	go cs.thirdAgentOutputLoop(context.Background())
	cs.log.Info("third-party agent joined conference")
}

func (cs *ConferenceSession) callerOutputLoop(ctx context.Context) {
	p := cs.mixer.participants[0]
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-p.pcmOut:
			if !ok {
				return
			}
			if cs.siprecSes.rtpSession != nil && cs.siprecSes.rtpSession.listener != nil {
				cs.siprecSes.rtpSession.listener.SendPCM(frame)
			}
		}
	}
}

func (cs *ConferenceSession) agentOutputLoop(ctx context.Context) {
	p := cs.mixer.participants[1]
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-p.pcmOut:
			if !ok {
				return
			}
			if cs.agentWRTC == nil || cs.agentWRTC.outTrack == nil {
				continue
			}
			pcm8k := resample(frame, 16000, 8000)
			ulaw := EncodeG711Ulaw(pcm8k)
			cs.agentWRTC.outTrack.WriteSample(media.Sample{
				Data:     ulaw,
				Duration: 20 * time.Millisecond,
			})
		}
	}
}

func (cs *ConferenceSession) thirdAgentOutputLoop(ctx context.Context) {
	p := cs.mixer.participants[2]
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-p.pcmOut:
			if !ok {
				return
			}
			if cs.thirdWRTC == nil || cs.thirdWRTC.outTrack == nil {
				continue
			}
			pcm8k := resample(frame, 16000, 8000)
			ulaw := EncodeG711Ulaw(pcm8k)
			cs.thirdWRTC.outTrack.WriteSample(media.Sample{
				Data:     ulaw,
				Duration: 20 * time.Millisecond,
			})
		}
	}
}

func (cs *ConferenceSession) dialExternalThird(ctx context.Context, number string) {
	if cs.gw.sipServer == nil {
		cs.log.Error("no SIP server for external conference")
		return
	}

	trunk := cs.gw.sipServer.findOutboundTrunk("")
	if trunk == nil {
		cs.log.Error("no outbound trunk for conference")
		return
	}

	thirdCallID := "conf-" + cs.callID
	callerID := trunk.CallerID
	if callerID == "" {
		callerID = "voiceagent"
	}

	sess, err := cs.gw.sipServer.dialSIPCall(thirdCallID, number, callerID, trunk)
	if err != nil {
		cs.log.Error("conference dial failed", "err", err, "number", number)
		cs.siprecSes.broadcastSSE(map[string]any{
			"type":  "conference_failed",
			"error": "dial failed: " + err.Error(),
		})
		return
	}

	cs.thirdRTP = sess
	cs.mixer.participants[2].active = true

	go func() {
		listener := sess.listener
		buf := make([]byte, 1500)
		pkt := &rtp.Packet{}
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			n, _, err := listener.conn.ReadFromUDP(buf)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			if err := pkt.Unmarshal(buf[:n]); err != nil {
				continue
			}
			if len(pkt.Payload) == 0 {
				continue
			}
			pcm := ResampleG711toL16(pkt.Payload, "PCMU")
			select {
			case cs.mixer.participants[2].pcmIn <- pcm:
			default:
			}
		}
	}()

	go func() {
		p := cs.mixer.participants[2]
		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-p.pcmOut:
				if !ok {
					return
				}
				sess.listener.SendPCM(frame)
			}
		}
	}()

	cs.log.Info("external third party connected", "number", number)
	cs.siprecSes.broadcastSSE(map[string]any{
		"type":        "conference_started",
		"third_party": number,
		"third_type":  "external",
	})
}

func dropThirdParty(callID string) error {
	conferencesMu.Lock()
	cs, ok := conferences[callID]
	conferencesMu.Unlock()
	if !ok {
		return fmt.Errorf("no conference for call: %s", callID)
	}

	cs.mixer.participants[2].active = false

	if cs.thirdWRTC != nil {
		cs.thirdWRTC.close()
		if cs.gw.webrtcMgr != nil {
			cs.gw.webrtcMgr.mu.Lock()
			for k, v := range cs.gw.webrtcMgr.sessions {
				if v == cs.thirdWRTC {
					delete(cs.gw.webrtcMgr.sessions, k)
					break
				}
			}
			cs.gw.webrtcMgr.mu.Unlock()
		}
		cs.thirdWRTC = nil
	}

	if cs.thirdRTP != nil {
		if cs.thirdRTP.listener != nil {
			cs.thirdRTP.listener.Close()
		}
		if cs.thirdRTP.sipConn != nil {
			cs.thirdRTP.sipConn.Close()
		}
		cs.thirdRTP = nil
	}

	cs.siprecSes.broadcastSSE(map[string]any{
		"type":    "conference_ended",
		"call_id": callID,
	})

	cs.log.Info("third party dropped from conference")
	return nil
}

func endConference(callID string) {
	conferencesMu.Lock()
	cs, ok := conferences[callID]
	if ok {
		delete(conferences, callID)
	}
	conferencesMu.Unlock()

	if !ok {
		return
	}

	cs.cancel()
	if cs.siprecSes != nil {
		cs.siprecSes.conference = nil
	}

	if cs.thirdWRTC != nil {
		cs.thirdWRTC.close()
	}
	if cs.thirdRTP != nil {
		if cs.thirdRTP.listener != nil {
			cs.thirdRTP.listener.Close()
		}
		if cs.thirdRTP.sipConn != nil {
			cs.thirdRTP.sipConn.Close()
		}
	}

	cs.log.Info("conference ended")
}

