package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// QueueAnnouncer sends position/wait announcements to callers waiting in queue.
// Generates TTS via Piper/Whisper and sends audio back via RTP.
type QueueAnnouncer struct {
	gw      *gateway
	active  map[string]context.CancelFunc // callID → cancel
	mu      sync.Mutex
	ttsURL  string
}

func NewQueueAnnouncer(gw *gateway) *QueueAnnouncer {
	return &QueueAnnouncer{
		gw:     gw,
		active: make(map[string]context.CancelFunc),
		ttsURL: gw.cfg.TTSURL,
	}
}

// StartAnnouncements begins periodic announcements for a caller in queue.
func (qa *QueueAnnouncer) StartAnnouncements(callID, queueName string, copilot *siprecSession) {
	qa.mu.Lock()
	if _, exists := qa.active[callID]; exists {
		qa.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	qa.active[callID] = cancel
	qa.mu.Unlock()

	go qa.announcementLoop(ctx, callID, queueName, copilot)
}

// StopAnnouncements stops announcements for a caller (when picked or hung up).
func (qa *QueueAnnouncer) StopAnnouncements(callID string) {
	qa.mu.Lock()
	if cancel, ok := qa.active[callID]; ok {
		cancel()
		delete(qa.active, callID)
	}
	qa.mu.Unlock()
}

func (qa *QueueAnnouncer) announcementLoop(ctx context.Context, callID, queueName string, copilot *siprecSession) {
	defer func() {
		qa.mu.Lock()
		delete(qa.active, callID)
		qa.mu.Unlock()
	}()

	log := slog.With("call_id", callID, "queue", queueName)

	// Welcome message
	welcome := fmt.Sprintf("Thank you for calling %s. Please hold while we connect you to the next available agent.", queueName)
	qa.playTTS(ctx, welcome, copilot, log)

	// Periodic announcements every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			position, waitEst := qa.getQueuePosition(callID, queueName)
			if position <= 0 {
				return // caller no longer in queue
			}

			var msg string
			if waitEst > 0 {
				msg = fmt.Sprintf("You are caller number %d in the queue. Estimated wait time is %d minutes.", position, waitEst)
			} else {
				msg = fmt.Sprintf("You are caller number %d in the queue. An agent will be with you shortly.", position)
			}

			qa.playTTS(ctx, msg, copilot, log)
		}
	}
}

func (qa *QueueAnnouncer) getQueuePosition(callID, queueName string) (position int, waitMinutes int) {
	if qa.gw == nil || qa.gw.queueMgr == nil {
		return 0, 0
	}

	qa.gw.queueMgr.mu.Lock()
	defer qa.gw.queueMgr.mu.Unlock()

	q, ok := qa.gw.queueMgr.queues[queueName]
	if !ok {
		return 0, 0
	}

	for i, c := range q.Callers {
		if c.CallID == callID {
			position = i + 1
			// Estimate: average 3 minutes per caller ahead
			waitMinutes = position * 3
			if waitMinutes < 1 {
				waitMinutes = 1
			}
			return
		}
	}

	return 0, 0 // not in queue anymore
}

func (qa *QueueAnnouncer) playTTS(ctx context.Context, text string, copilot *siprecSession, log *slog.Logger) {
	if copilot == nil || copilot.rtpSession == nil {
		return
	}

	select {
	case <-ctx.Done():
		return
	default:
	}

	// Generate TTS audio
	pcm, err := qa.synthesize(ctx, text)
	if err != nil {
		log.Debug("TTS for announcement", "err", err)
		// Fallback: send silence (caller hears nothing, better than error)
		return
	}

	log.Info("queue announcement", "text", text[:min(60, len(text))])

	// Send PCM via RTP to caller (16kHz L16 → 8kHz → G.711 → RTP)
	listener := copilot.rtpSession.listener
	frameSize := 640 // 20ms at 16kHz L16

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
		chunk := pcm[off:end]

		if len(chunk) < frameSize {
			// Pad last frame with silence
			padded := make([]byte, frameSize)
			copy(padded, chunk)
			chunk = padded
		}

		if err := listener.SendPCM(chunk); err != nil {
			return
		}
		time.Sleep(18 * time.Millisecond) // ~20ms pacing
	}
}

func (qa *QueueAnnouncer) synthesize(ctx context.Context, text string) ([]byte, error) {
	if qa.ttsURL == "" {
		return nil, fmt.Errorf("TTS not configured")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", qa.ttsURL, strings.NewReader(text))
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
		return nil, fmt.Errorf("TTS %d: %s", resp.StatusCode, b)
	}

	wav, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Strip WAV header → raw PCM
	pcm := stripWAVHeader(wav)

	// Piper outputs 22050 Hz → resample to 16000 Hz
	return resample(pcm, 22050, sampleRate), nil
}

// GenerateTone creates a simple tone as PCM for testing (no TTS needed)
func GenerateTone(freq float64, durationMs int) []byte {
	samples := sampleRate * durationMs / 1000
	pcm := make([]byte, samples*bytesPerSample)
	for i := 0; i < samples; i++ {
		t := float64(i) / float64(sampleRate)
		val := int16(8000 * sin(2*3.14159*freq*t))
		pcm[i*2] = byte(val)
		pcm[i*2+1] = byte(val >> 8)
	}
	return pcm
}

func sin(x float64) float64 {
	// Taylor series approximation for math.Sin without import
	x = x - float64(int(x/(2*3.14159)))*2*3.14159
	if x > 3.14159 {
		x -= 2 * 3.14159
	}
	x3 := x * x * x
	x5 := x3 * x * x
	x7 := x5 * x * x
	return x - x3/6 + x5/120 - x7/5040
}

// min returns the smaller of a or b
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Ensure bytes import is used
var _ = bytes.NewReader
