package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// -------------------------------------------------------------------
// Native DTMF Parsing — RFC 2833/4733 RTP Payload Interception
//
// When ASR fails in noisy environments, callers resort to keypad input.
// This parser intercepts DTMF digits from the RTP stream and delivers
// them as clean text to the LLM pipeline — 100% accurate, no audio
// processing needed.
//
// Captures:
//   - Digits 0-9, *, #
//   - Duration and inter-digit timing
//   - Accumulates sequences for account numbers, PINs, dates
//   - Feeds to LLM as: "User typed: 482910"
// -------------------------------------------------------------------

type DTMFEvent struct {
	Digit    string    `json:"digit"`
	Duration int       `json:"duration_ms"`
	Time     time.Time `json:"time"`
}

type DTMFCollector struct {
	buffer    []DTMFEvent
	mu        sync.Mutex
	timeout   time.Duration // inter-digit timeout before flushing
	lastDigit time.Time
	callback  func(digits string) // called when sequence complete
}

func NewDTMFCollector(timeout time.Duration, callback func(string)) *DTMFCollector {
	dc := &DTMFCollector{
		timeout:  timeout,
		callback: callback,
	}
	go dc.flushLoop()
	return dc
}

// ParseRFC2833 extracts DTMF digit from an RFC 2833 RTP event payload.
// RFC 2833 payload format (4 bytes):
//   Byte 0: event (0-15 maps to 0-9,*,#,A-D)
//   Byte 1: E(1bit) + R(1bit) + volume(6bits)
//   Byte 2-3: duration (16-bit, in timestamp units)
func ParseRFC2833(payload []byte) *DTMFEvent {
	if len(payload) < 4 {
		return nil
	}

	eventCode := payload[0]
	endBit := (payload[1] & 0x80) != 0
	duration := int(payload[2])<<8 | int(payload[3])

	// Only process end-of-event (E bit set) to avoid duplicates
	if !endBit {
		return nil
	}

	digit := dtmfEventToChar(eventCode)
	if digit == "" {
		return nil
	}

	// Duration is in timestamp units (8000 Hz clock for G.711)
	durationMs := duration * 1000 / 8000

	return &DTMFEvent{
		Digit:    digit,
		Duration: durationMs,
		Time:     time.Now(),
	}
}

// ParseInbandDTMF detects DTMF tones from raw PCM audio using Goertzel algorithm.
// Fallback for when RFC 2833 is not available (some legacy PBX systems).
func ParseInbandDTMF(pcm []byte, sampleRate int) string {
	samples := len(pcm) / 2
	if samples < 160 {
		return ""
	}

	// DTMF frequency pairs
	lowFreqs := []float64{697, 770, 852, 941}
	highFreqs := []float64{1209, 1336, 1477, 1633}

	dtmfMap := [4][4]string{
		{"1", "2", "3", "A"},
		{"4", "5", "6", "B"},
		{"7", "8", "9", "C"},
		{"*", "0", "#", "D"},
	}

	// Goertzel algorithm for each target frequency
	lowIdx := -1
	highIdx := -1
	maxLowPower := 0.0
	maxHighPower := 0.0

	for i, freq := range lowFreqs {
		power := goertzel(pcm, sampleRate, freq)
		if power > maxLowPower && power > 1e6 {
			maxLowPower = power
			lowIdx = i
		}
	}

	for i, freq := range highFreqs {
		power := goertzel(pcm, sampleRate, freq)
		if power > maxHighPower && power > 1e6 {
			maxHighPower = power
			highIdx = i
		}
	}

	if lowIdx >= 0 && highIdx >= 0 {
		return dtmfMap[lowIdx][highIdx]
	}
	return ""
}

// goertzel computes the power of a specific frequency in PCM audio.
func goertzel(pcm []byte, sampleRate int, targetFreq float64) float64 {
	import_math_cos := func(x float64) float64 {
		// Taylor series approximation for cos (avoid math import conflict)
		x2 := x * x
		return 1 - x2/2 + x2*x2/24 - x2*x2*x2/720
	}

	n := len(pcm) / 2
	k := int(0.5 + float64(n)*targetFreq/float64(sampleRate))
	w := 2.0 * 3.14159265358979 * float64(k) / float64(n)
	coeff := 2.0 * import_math_cos(w)

	var s0, s1, s2 float64
	for i := 0; i < n; i++ {
		sample := float64(int16(pcm[i*2]) | int16(pcm[i*2+1])<<8)
		s0 = sample + coeff*s1 - s2
		s2 = s1
		s1 = s0
	}

	return s1*s1 + s2*s2 - coeff*s1*s2
}

func (dc *DTMFCollector) AddDigit(evt *DTMFEvent) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.buffer = append(dc.buffer, *evt)
	dc.lastDigit = evt.Time

	slog.Info("dtmf digit", "digit", evt.Digit, "duration_ms", evt.Duration, "buffer", dc.String())
}

func (dc *DTMFCollector) String() string {
	var sb strings.Builder
	for _, e := range dc.buffer {
		sb.WriteString(e.Digit)
	}
	return sb.String()
}

func (dc *DTMFCollector) Flush() string {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if len(dc.buffer) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, e := range dc.buffer {
		sb.WriteString(e.Digit)
	}
	digits := sb.String()
	dc.buffer = dc.buffer[:0]

	slog.Info("dtmf sequence complete", "digits", digits)
	return digits
}

func (dc *DTMFCollector) flushLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		dc.mu.Lock()
		if len(dc.buffer) > 0 && time.Since(dc.lastDigit) > dc.timeout {
			var sb strings.Builder
			for _, e := range dc.buffer {
				sb.WriteString(e.Digit)
			}
			digits := sb.String()
			dc.buffer = dc.buffer[:0]
			dc.mu.Unlock()

			if dc.callback != nil {
				dc.callback(digits)
			}
		} else {
			dc.mu.Unlock()
		}
	}
}

func dtmfEventToChar(code byte) string {
	switch {
	case code <= 9:
		return fmt.Sprintf("%d", code)
	case code == 10:
		return "*"
	case code == 11:
		return "#"
	case code == 12:
		return "A"
	case code == 13:
		return "B"
	case code == 14:
		return "C"
	case code == 15:
		return "D"
	default:
		return ""
	}
}

// -------------------------------------------------------------------
// DTMF API endpoints
// -------------------------------------------------------------------

func handleDTMFRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dtmf/test", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Payload []byte `json:"payload"` // RFC 2833 4-byte payload
			Text    string `json:"text"`    // or test with text digits
		}
		json.NewDecoder(r.Body).Decode(&req)

		if req.Text != "" {
			writeJSON(w, http.StatusOK, map[string]string{
				"input":  req.Text,
				"parsed": fmt.Sprintf("User typed: %s", req.Text),
			})
			return
		}

		if len(req.Payload) >= 4 {
			evt := ParseRFC2833(req.Payload)
			if evt != nil {
				writeJSON(w, http.StatusOK, evt)
			} else {
				writeJSON(w, http.StatusOK, map[string]string{"status": "no_event"})
			}
			return
		}

		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload or text required"})
	})
}
