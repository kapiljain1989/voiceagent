package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type CallRecorder struct {
	callID      string
	callerBuf   []byte
	agentBuf    []byte
	callerTap   chan []byte
	agentTap    chan []byte
	mu          sync.Mutex
	recording   bool
	paused      bool
	startTime   time.Time
	cancel      context.CancelFunc
	log         *slog.Logger
}

func newCallRecorder(callID string, copilot *siprecSession) *CallRecorder {
	callerTap := copilot.AddAudioTap()
	agentTap := copilot.AddAgentTap()

	return &CallRecorder{
		callID:    callID,
		callerTap: callerTap,
		agentTap:  agentTap,
		startTime: time.Now(),
		log:       slog.With("call_id", callID, "component", "recorder"),
	}
}

func (r *CallRecorder) start(ctx context.Context, copilot *siprecSession) {
	ctx, r.cancel = context.WithCancel(ctx)
	r.recording = true
	r.log.Info("recording started")

	go r.collectAudio(ctx, r.callerTap, true)
	go r.collectAudio(ctx, r.agentTap, false)
}

func (r *CallRecorder) collectAudio(ctx context.Context, ch chan []byte, isCaller bool) {
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-ch:
			if !ok {
				return
			}
			if r.paused {
				continue
			}
			r.mu.Lock()
			if isCaller {
				r.callerBuf = append(r.callerBuf, frame...)
			} else {
				r.agentBuf = append(r.agentBuf, frame...)
			}
			r.mu.Unlock()
		}
	}
}

func (r *CallRecorder) stop(copilot *siprecSession) {
	if !r.recording {
		return
	}
	r.recording = false
	if r.cancel != nil {
		r.cancel()
	}

	copilot.RemoveAudioTap(r.callerTap)
	copilot.RemoveAgentTap(r.agentTap)
}

func (r *CallRecorder) save(gw *gateway, copilot *siprecSession) (string, error) {
	r.mu.Lock()
	callerPCM := make([]byte, len(r.callerBuf))
	agentPCM := make([]byte, len(r.agentBuf))
	copy(callerPCM, r.callerBuf)
	copy(agentPCM, r.agentBuf)
	r.mu.Unlock()

	if len(callerPCM) == 0 && len(agentPCM) == 0 {
		r.log.Info("no audio to record")
		return "", nil
	}

	// Interleave caller (left) + agent (right) into stereo PCM
	stereoPCM := interleaveStereo(callerPCM, agentPCM)

	wav := buildWAV(stereoPCM, sampleRate, 2, 16)

	// Write to disk
	recordingDir := gw.cfg.RecordingDir
	if recordingDir == "" {
		recordingDir = "/tmp/recordings"
	}

	now := time.Now()
	subDir := filepath.Join(recordingDir, now.Format("2006/01/02"))
	os.MkdirAll(subDir, 0755)

	fileName := fmt.Sprintf("%s.wav", sanitizeCallID(r.callID))
	filePath := filepath.Join(subDir, fileName)

	if err := os.WriteFile(filePath, wav, 0644); err != nil {
		return "", fmt.Errorf("write recording: %w", err)
	}

	duration := int(time.Since(r.startTime).Seconds())

	r.log.Info("recording saved",
		"path", filePath,
		"size", len(wav),
		"duration", duration,
		"caller_samples", len(callerPCM)/2,
		"agent_samples", len(agentPCM)/2)

	// Save to database
	if database != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		database.DB().ExecContext(ctx, `
			INSERT INTO call_recordings (call_id, file_path, format, sample_rate, channels, duration_sec, file_size_bytes)
			VALUES ($1, $2, 'wav', $3, 2, $4, $5)
			ON CONFLICT DO NOTHING`,
			r.callID, filePath, sampleRate, duration, len(wav))
	}

	return filePath, nil
}

func interleaveStereo(left, right []byte) []byte {
	// Ensure both channels are same length (pad shorter with silence)
	maxLen := len(left)
	if len(right) > maxLen {
		maxLen = len(right)
	}

	if len(left) < maxLen {
		left = append(left, make([]byte, maxLen-len(left))...)
	}
	if len(right) < maxLen {
		right = append(right, make([]byte, maxLen-len(right))...)
	}

	nSamples := maxLen / 2
	stereo := make([]byte, nSamples*4) // 2 channels × 2 bytes per sample

	for i := 0; i < nSamples; i++ {
		// Left channel (caller)
		binary.LittleEndian.PutUint16(stereo[i*4:i*4+2],
			binary.LittleEndian.Uint16(left[i*2:i*2+2]))
		// Right channel (agent)
		binary.LittleEndian.PutUint16(stereo[i*4+2:i*4+4],
			binary.LittleEndian.Uint16(right[i*2:i*2+2]))
	}

	return stereo
}

func (gw *gateway) handleRecordingsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}

	if database == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := database.DB().QueryContext(ctx, `
		SELECT id, call_id, file_path, duration_sec, file_size_bytes, created_at
		FROM call_recordings ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	defer rows.Close()

	var recs []map[string]any
	for rows.Next() {
		var id, callID, filePath string
		var dur, size int64
		var created time.Time
		rows.Scan(&id, &callID, &filePath, &dur, &size, &created)
		recs = append(recs, map[string]any{
			"id":        id,
			"call_id":   callID,
			"duration":  dur,
			"size":      size,
			"created_at": created.Format(time.RFC3339),
			"url":       fmt.Sprintf("/api/recordings/%s", id),
		})
	}
	if recs == nil {
		recs = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, recs)
}

func (gw *gateway) handleRecordingFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/api/recordings/"):]
	if id == "" {
		http.Error(w, "recording ID required", http.StatusBadRequest)
		return
	}

	if database == nil {
		http.Error(w, "no database", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var filePath string
	err := database.DB().QueryRowContext(ctx,
		`SELECT file_path FROM call_recordings WHERE id=$1`, id).Scan(&filePath)
	if err != nil {
		http.Error(w, "recording not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%s.wav", id))
	http.ServeFile(w, r, filePath)
}

func sanitizeCallID(callID string) string {
	safe := make([]byte, 0, len(callID))
	for _, c := range []byte(callID) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			safe = append(safe, c)
		} else {
			safe = append(safe, '_')
		}
	}
	return string(safe)
}
