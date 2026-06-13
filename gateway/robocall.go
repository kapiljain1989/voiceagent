package main

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// -------------------------------------------------------------------
// Robocall Detection — Three-layer spam filtering
//
// Layer 1: Blocklist (< 1ms) — in-memory hash map
// Layer 2: Audio Pattern (~2s) — RMS variance, silence ratio
// Layer 3: Transcript Keywords (~after first STT) — phrase matching
// -------------------------------------------------------------------

type RobocallResult struct {
	Score    float64 `json:"score"`    // 0.0 (human) → 1.0 (robocall)
	Category string  `json:"category"` // human, robocall, uncertain, voicemail
	Reason   string  `json:"reason"`   // blocklist, audio_pattern, keyword_match
	Keywords []string `json:"keywords,omitempty"`
	Blocked  bool    `json:"blocked"`
}

type BlocklistEntry struct {
	Number    string `json:"number"`
	Reason    string `json:"reason"`
	Source    string `json:"source"` // manual, auto_detected, reported
	CallCount int    `json:"call_count"`
	CreatedAt string `json:"created_at"`
}

type RobocallDetector struct {
	blocklist      map[string]string // number → reason
	customKeywords []struct{ phrase string; weight float64 }
	mu             sync.RWMutex
	db             *sql.DB
	threshold      float64
	autoBlock      bool
}

func (d *RobocallDetector) AddKeyword(phrase string, weight float64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.customKeywords = append(d.customKeywords, struct{ phrase string; weight float64 }{phrase, weight})
}

func (d *RobocallDetector) RemoveKeyword(phrase string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	filtered := d.customKeywords[:0]
	for _, kw := range d.customKeywords {
		if kw.phrase != phrase {
			filtered = append(filtered, kw)
		}
	}
	d.customKeywords = filtered
}

func (d *RobocallDetector) ListKeywords() []CustomRobocallKeyword {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var result []CustomRobocallKeyword
	for _, kw := range robocallKeywords {
		result = append(result, CustomRobocallKeyword{
			Phrase: kw.phrase, Weight: kw.weight, Category: "built-in", Enabled: true, IsDefault: true,
		})
	}
	for _, kw := range d.customKeywords {
		result = append(result, CustomRobocallKeyword{
			Phrase: kw.phrase, Weight: kw.weight, Category: "custom", Enabled: true, IsDefault: false,
		})
	}
	return result
}

func NewRobocallDetector(db *sql.DB) *RobocallDetector {
	d := &RobocallDetector{
		blocklist: make(map[string]string),
		db:        db,
		threshold: 0.7,
		autoBlock: false,
	}
	if db != nil {
		d.initDB()
		d.loadBlocklist()
	}
	return d
}

func (d *RobocallDetector) initDB() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS blocklist (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			number TEXT UNIQUE NOT NULL,
			reason TEXT,
			source TEXT DEFAULT 'manual',
			call_count INT DEFAULT 1,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		ALTER TABLE calls ADD COLUMN IF NOT EXISTS robocall_score FLOAT DEFAULT 0;
		ALTER TABLE calls ADD COLUMN IF NOT EXISTS robocall_category TEXT DEFAULT 'human';
		ALTER TABLE calls ADD COLUMN IF NOT EXISTS blocked BOOLEAN DEFAULT FALSE;
	`)
}

func (d *RobocallDetector) loadBlocklist() {
	if d.db == nil {
		return
	}
	rows, err := d.db.Query("SELECT number, reason FROM blocklist")
	if err != nil {
		slog.Warn("blocklist load", "err", err)
		return
	}
	defer rows.Close()

	d.mu.Lock()
	defer d.mu.Unlock()
	for rows.Next() {
		var num, reason string
		rows.Scan(&num, &reason)
		d.blocklist[normalizeNumber(num)] = reason
	}
	slog.Info("blocklist loaded", "entries", len(d.blocklist))
}

// -------------------------------------------------------------------
// Layer 1: Blocklist — O(1) lookup
// -------------------------------------------------------------------

func (d *RobocallDetector) CheckBlocklist(number string) *RobocallResult {
	d.mu.RLock()
	reason, found := d.blocklist[normalizeNumber(number)]
	d.mu.RUnlock()

	if found {
		if d.db != nil {
			d.db.Exec("UPDATE blocklist SET call_count = call_count + 1 WHERE number = $1", normalizeNumber(number))
		}
		return &RobocallResult{
			Score:    1.0,
			Category: "robocall",
			Reason:   "blocklist: " + reason,
			Blocked:  true,
		}
	}
	return nil
}

func (d *RobocallDetector) AddToBlocklist(number, reason, source string) {
	norm := normalizeNumber(number)
	d.mu.Lock()
	d.blocklist[norm] = reason
	d.mu.Unlock()

	if d.db != nil {
		d.db.Exec(
			"INSERT INTO blocklist (number, reason, source) VALUES ($1, $2, $3) ON CONFLICT (number) DO UPDATE SET reason = $2, call_count = blocklist.call_count + 1",
			norm, reason, source)
	}
	slog.Info("blocklist add", "number", norm, "reason", reason)
}

func (d *RobocallDetector) RemoveFromBlocklist(number string) {
	norm := normalizeNumber(number)
	d.mu.Lock()
	delete(d.blocklist, norm)
	d.mu.Unlock()

	if d.db != nil {
		d.db.Exec("DELETE FROM blocklist WHERE number = $1", norm)
	}
}

// -------------------------------------------------------------------
// Layer 2: Audio Pattern Analysis
//
// Analyzes first 2 seconds of PCM to detect pre-recorded audio:
// - Low RMS variance = monotone/pre-recorded
// - High silence ratio = dead air padding
// - Consistent energy = not natural speech
// -------------------------------------------------------------------

func (d *RobocallDetector) AnalyzeAudio(frames [][]byte) *RobocallResult {
	if len(frames) < 10 {
		return &RobocallResult{Score: 0, Category: "uncertain", Reason: "insufficient_audio"}
	}

	energies := make([]float64, len(frames))
	silentFrames := 0
	totalEnergy := 0.0

	for i, frame := range frames {
		e := rmsEnergy(frame)
		energies[i] = e
		totalEnergy += e
		if e < 30 {
			silentFrames++
		}
	}

	avgEnergy := totalEnergy / float64(len(frames))
	silenceRatio := float64(silentFrames) / float64(len(frames))

	// Compute RMS variance (how much energy fluctuates)
	var variance float64
	for _, e := range energies {
		diff := e - avgEnergy
		variance += diff * diff
	}
	variance /= float64(len(energies))
	stdDev := math.Sqrt(variance)

	// Coefficient of variation (normalized variance)
	cv := 0.0
	if avgEnergy > 0 {
		cv = stdDev / avgEnergy
	}

	score := 0.0
	reasons := []string{}

	// Low energy variance = monotone/pre-recorded
	if cv < 0.3 && avgEnergy > 50 {
		score += 0.4
		reasons = append(reasons, "monotone_audio")
	}

	// Very high silence ratio = padding + burst pattern
	if silenceRatio > 0.7 {
		score += 0.3
		reasons = append(reasons, "high_silence")
	}

	// Zero energy = no audio at all (dead channel)
	if avgEnergy < 10 {
		score += 0.2
		reasons = append(reasons, "dead_channel")
	}

	if score > 1.0 {
		score = 1.0
	}

	category := "human"
	if score > 0.7 {
		category = "robocall"
	} else if score > 0.4 {
		category = "uncertain"
	}

	return &RobocallResult{
		Score:    score,
		Category: category,
		Reason:   strings.Join(reasons, ", "),
	}
}

// -------------------------------------------------------------------
// Layer 3: Transcript Keyword Classification
//
// Matches against known robocall phrases after Whisper transcription.
// -------------------------------------------------------------------

var robocallKeywords = []struct {
	phrase string
	weight float64
}{
	{"press 1", 0.6},
	{"press 2", 0.5},
	{"press 9", 0.5},
	{"do not hang up", 0.7},
	{"this is not a sales call", 0.6},
	{"your account has been", 0.5},
	{"your amazon", 0.6},
	{"your apple", 0.5},
	{"irs", 0.6},
	{"social security", 0.6},
	{"auto warranty", 0.8},
	{"car warranty", 0.7},
	{"extended warranty", 0.8},
	{"student loan", 0.6},
	{"loan forgiveness", 0.5},
	{"tech support", 0.4},
	{"microsoft support", 0.6},
	{"your computer has", 0.6},
	{"you have won", 0.7},
	{"congratulations", 0.3},
	{"free vacation", 0.7},
	{"lower your interest", 0.6},
	{"reduce your debt", 0.5},
	{"this is an important message", 0.5},
	{"we have been trying to reach you", 0.6},
	{"final notice", 0.5},
	{"legal action", 0.5},
	{"arrest warrant", 0.7},
}

func (d *RobocallDetector) ClassifyTranscript(text string) *RobocallResult {
	lower := strings.ToLower(text)

	score := 0.0
	var matched []string

	for _, kw := range robocallKeywords {
		if strings.Contains(lower, kw.phrase) {
			score += kw.weight
			matched = append(matched, kw.phrase)
		}
	}
	d.mu.RLock()
	for _, kw := range d.customKeywords {
		if strings.Contains(lower, kw.phrase) {
			score += kw.weight
			matched = append(matched, kw.phrase)
		}
	}
	d.mu.RUnlock()

	if score > 1.0 {
		score = 1.0
	}

	category := "human"
	if score > 0.6 {
		category = "robocall"
	} else if score > 0.3 {
		category = "uncertain"
	}

	return &RobocallResult{
		Score:    score,
		Category: category,
		Reason:   "keyword_match",
		Keywords: matched,
	}
}

// -------------------------------------------------------------------
// Combined detection — merges all three layers
// -------------------------------------------------------------------

func (d *RobocallDetector) CombinedScore(blocklistResult, audioResult, keywordResult *RobocallResult) *RobocallResult {
	if blocklistResult != nil && blocklistResult.Score > 0 {
		return blocklistResult
	}

	score := 0.0
	reasons := []string{}

	if audioResult != nil && audioResult.Score > 0 {
		score += audioResult.Score * 0.4
		reasons = append(reasons, audioResult.Reason)
	}
	if keywordResult != nil && keywordResult.Score > 0 {
		score += keywordResult.Score * 0.6
		reasons = append(reasons, keywordResult.Reason)
	}

	if score > 1.0 {
		score = 1.0
	}

	category := "human"
	if score > d.threshold {
		category = "robocall"
	} else if score > d.threshold*0.6 {
		category = "uncertain"
	}

	result := &RobocallResult{
		Score:    score,
		Category: category,
		Reason:   strings.Join(reasons, "; "),
		Blocked:  score >= d.threshold && d.autoBlock,
	}

	if keywordResult != nil {
		result.Keywords = keywordResult.Keywords
	}
	return result
}

// -------------------------------------------------------------------
// API handlers
// -------------------------------------------------------------------

func (d *RobocallDetector) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/blocklist", d.handleBlocklist)
	mux.HandleFunc("/api/robocall/stats", d.handleStats)
	mux.HandleFunc("/api/robocall/test", d.handleTest)
}

func (d *RobocallDetector) handleBlocklist(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		d.mu.RLock()
		entries := make([]map[string]string, 0, len(d.blocklist))
		for num, reason := range d.blocklist {
			entries = append(entries, map[string]string{"number": num, "reason": reason})
		}
		d.mu.RUnlock()
		writeJSON(w, http.StatusOK, entries)

	case "POST":
		var req struct {
			Number string `json:"number"`
			Reason string `json:"reason"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Number == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "number required"})
			return
		}
		d.AddToBlocklist(req.Number, req.Reason, "manual")
		writeJSON(w, http.StatusCreated, map[string]string{"status": "added", "number": normalizeNumber(req.Number)})

	case "DELETE":
		var req struct {
			Number string `json:"number"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		d.RemoveFromBlocklist(req.Number)
		writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (d *RobocallDetector) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := map[string]any{
		"blocklist_size": len(d.blocklist),
		"threshold":      d.threshold,
		"auto_block":     d.autoBlock,
	}

	if d.db != nil {
		today := time.Now().Truncate(24 * time.Hour)
		var total, blocked, robocalls, uncertain int
		d.db.QueryRow("SELECT COUNT(*) FROM calls WHERE start_time >= $1", today).Scan(&total)
		d.db.QueryRow("SELECT COUNT(*) FROM calls WHERE blocked = TRUE AND start_time >= $1", today).Scan(&blocked)
		d.db.QueryRow("SELECT COUNT(*) FROM calls WHERE robocall_category = 'robocall' AND start_time >= $1", today).Scan(&robocalls)
		d.db.QueryRow("SELECT COUNT(*) FROM calls WHERE robocall_category = 'uncertain' AND start_time >= $1", today).Scan(&uncertain)

		stats["total_calls_today"] = total
		stats["robocalls_detected"] = robocalls
		stats["blocked"] = blocked
		stats["uncertain"] = uncertain
	}

	writeJSON(w, http.StatusOK, stats)
}

func (d *RobocallDetector) handleTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text   string `json:"text"`
		Number string `json:"number"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	results := map[string]any{}

	if req.Number != "" {
		bl := d.CheckBlocklist(req.Number)
		if bl != nil {
			results["blocklist"] = bl
		} else {
			results["blocklist"] = map[string]string{"status": "not_found"}
		}
	}

	if req.Text != "" {
		kw := d.ClassifyTranscript(req.Text)
		results["keyword"] = kw
	}

	writeJSON(w, http.StatusOK, results)
}

// -------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------

func normalizeNumber(num string) string {
	var digits []byte
	for _, c := range num {
		if c >= '0' && c <= '9' {
			digits = append(digits, byte(c))
		}
	}
	return string(digits)
}

// rmsVariance computes the variance of RMS energy across frames.
// Used by Layer 2 for monotone detection.
func rmsVariance(frames [][]byte) (avg, variance float64) {
	if len(frames) == 0 {
		return 0, 0
	}
	energies := make([]float64, len(frames))
	for i, f := range frames {
		n := len(f) / 2
		if n == 0 {
			continue
		}
		var sum float64
		for j := 0; j < n; j++ {
			s := int16(binary.LittleEndian.Uint16(f[j*2 : j*2+2]))
			sum += float64(s) * float64(s)
		}
		energies[i] = math.Sqrt(sum / float64(n))
		avg += energies[i]
	}
	avg /= float64(len(frames))
	for _, e := range energies {
		d := e - avg
		variance += d * d
	}
	variance /= float64(len(frames))
	return avg, variance
}
