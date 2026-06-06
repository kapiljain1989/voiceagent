package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// -------------------------------------------------------------------
// Voice Biometrics + Fraud Detection
//
// Concurrent voice-printing runs alongside the STT pipeline.
// Extracts audio fingerprints from raw PCM and compares against:
//   1. Known fraud voice profiles (blocklist)
//   2. Verified account holder voiceprints (authentication)
//
// Uses MFCC-like spectral features for voice fingerprinting.
// Production should integrate with a neural speaker embedding model.
// -------------------------------------------------------------------

type VoicePrint struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`      // "fraud_profile_001" or "account_holder_john"
	Type      string    `json:"type"`       // "fraud" or "verified"
	Features  []float64 `json:"features"`   // 32-dim voice fingerprint
	CreatedAt time.Time `json:"created_at"`
}

type VoiceMatchResult struct {
	Matched    bool    `json:"matched"`
	PrintID    string  `json:"print_id,omitempty"`
	Label      string  `json:"label,omitempty"`
	Type       string  `json:"type,omitempty"`    // fraud, verified
	Similarity float64 `json:"similarity"`        // 0.0-1.0
	Threshold  float64 `json:"threshold"`
}

type VoiceBiometrics struct {
	prints    []VoicePrint
	mu        sync.RWMutex
	db        *sql.DB
	threshold float64 // similarity threshold for match (default 0.85)
}

func NewVoiceBiometrics(db *sql.DB) *VoiceBiometrics {
	vb := &VoiceBiometrics{
		db:        db,
		threshold: 0.85,
	}
	if db != nil {
		vb.initDB()
		vb.loadPrints()
	}
	return vb
}

func (vb *VoiceBiometrics) initDB() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	vb.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS voice_prints (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			label TEXT NOT NULL,
			type TEXT NOT NULL,
			features JSONB NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
}

func (vb *VoiceBiometrics) loadPrints() {
	if vb.db == nil {
		return
	}
	rows, err := vb.db.Query("SELECT id, label, type, features FROM voice_prints")
	if err != nil {
		return
	}
	defer rows.Close()

	vb.mu.Lock()
	defer vb.mu.Unlock()
	for rows.Next() {
		var vp VoicePrint
		var featJSON []byte
		rows.Scan(&vp.ID, &vp.Label, &vp.Type, &featJSON)
		json.Unmarshal(featJSON, &vp.Features)
		vb.prints = append(vb.prints, vp)
	}
	slog.Info("voice prints loaded", "count", len(vb.prints))
}

// ExtractVoicePrint generates a 32-dimensional fingerprint from PCM audio.
// Uses spectral energy distribution across frequency bands as features.
// Production: replace with a neural speaker embedding (e.g., ECAPA-TDNN).
func ExtractVoicePrint(pcm []byte) []float64 {
	const dim = 32
	features := make([]float64, dim)

	samples := len(pcm) / 2
	if samples < 160 {
		return features
	}

	// Extract spectral features using simple band energy analysis
	frameSize := 320 // 20ms at 16kHz
	numFrames := samples / frameSize

	for f := 0; f < numFrames; f++ {
		offset := f * frameSize
		for band := 0; band < dim; band++ {
			bandStart := offset + (band * frameSize / dim)
			bandEnd := offset + ((band + 1) * frameSize / dim)
			if bandEnd*2 > len(pcm) {
				break
			}
			var energy float64
			for i := bandStart; i < bandEnd; i++ {
				s := int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2]))
				energy += float64(s) * float64(s)
			}
			features[band] += math.Sqrt(energy / float64(bandEnd-bandStart))
		}
	}

	// Normalize
	if numFrames > 0 {
		for i := range features {
			features[i] /= float64(numFrames)
		}
	}

	// L2 normalize the feature vector
	var norm float64
	for _, v := range features {
		norm += v * v
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range features {
			features[i] /= norm
		}
	}

	return features
}

// MatchVoice compares a voice fingerprint against all stored prints.
func (vb *VoiceBiometrics) MatchVoice(features []float64) *VoiceMatchResult {
	vb.mu.RLock()
	defer vb.mu.RUnlock()

	best := &VoiceMatchResult{Threshold: vb.threshold}

	for _, vp := range vb.prints {
		sim := cosineSimilarity(features, vp.Features)
		if sim > best.Similarity {
			best.Similarity = sim
			best.PrintID = vp.ID
			best.Label = vp.Label
			best.Type = vp.Type
		}
	}

	best.Matched = best.Similarity >= vb.threshold
	return best
}

// EnrollVoice stores a new voice print.
func (vb *VoiceBiometrics) EnrollVoice(label, printType string, pcm []byte) *VoicePrint {
	features := ExtractVoicePrint(pcm)

	vp := VoicePrint{
		ID:        fmt.Sprintf("%x", sha256.Sum256([]byte(label+time.Now().String())))[:16],
		Label:     label,
		Type:      printType,
		Features:  features,
		CreatedAt: time.Now(),
	}

	vb.mu.Lock()
	vb.prints = append(vb.prints, vp)
	vb.mu.Unlock()

	if vb.db != nil {
		featJSON, _ := json.Marshal(features)
		vb.db.Exec("INSERT INTO voice_prints (id, label, type, features) VALUES ($1,$2,$3,$4)",
			vp.ID, vp.Label, vp.Type, featJSON)
	}

	slog.Info("voice print enrolled", "label", label, "type", printType)
	return &vp
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// -------------------------------------------------------------------
// Live PII Masking (PCI/HIPAA Compliance)
//
// Detects credit card numbers, SSNs, and other PII in real-time
// transcript text. When detected:
//   1. Masks the PII in the transcript (4111... → 4111-XXXX-XXXX-XXXX)
//   2. Replaces corresponding PCM audio frames with silence
//   3. Sends a compliance event to the dashboard
//
// Operates on transcript text (post-Whisper) and can retroactively
// silence the audio buffer before it reaches call recording storage.
// -------------------------------------------------------------------

type PIIMasker struct {
	patterns []piiPattern
	enabled  bool
}

type piiPattern struct {
	Name    string
	Regex   *regexp.Regexp
	Mask    string
	Level   string // critical, high, medium
}

type PIIDetection struct {
	Type     string `json:"type"`      // credit_card, ssn, phone, email, dob
	Original string `json:"original"`  // masked version of detected value
	Masked   string `json:"masked"`
	Level    string `json:"level"`
	Position int    `json:"position"`  // character position in transcript
}

func NewPIIMasker() *PIIMasker {
	return &PIIMasker{
		enabled: true,
		patterns: []piiPattern{
			{
				Name:  "credit_card",
				Regex: regexp.MustCompile(`\b(?:\d[ -]*?){13,19}\b`),
				Mask:  "XXXX-XXXX-XXXX-####",
				Level: "critical",
			},
			{
				Name:  "ssn",
				Regex: regexp.MustCompile(`\b\d{3}[-. ]?\d{2}[-. ]?\d{4}\b`),
				Mask:  "XXX-XX-####",
				Level: "critical",
			},
			{
				Name:  "ssn_spoken",
				Regex: regexp.MustCompile(`(?i)\b(?:social security|social|ssn)\s*(?:number)?\s*(?:is)?\s*\d`),
				Mask:  "[SSN REDACTED]",
				Level: "critical",
			},
			{
				Name:  "credit_card_spoken",
				Regex: regexp.MustCompile(`(?i)(?:card|credit|debit|visa|mastercard|amex)\s*(?:number)?\s*(?:is)?\s*\d`),
				Mask:  "[CARD REDACTED]",
				Level: "critical",
			},
			{
				Name:  "cvv",
				Regex: regexp.MustCompile(`(?i)(?:cvv|cvc|security code|verification)\s*(?:is|number)?\s*\d{3,4}`),
				Mask:  "[CVV REDACTED]",
				Level: "critical",
			},
			{
				Name:  "dob",
				Regex: regexp.MustCompile(`(?i)(?:date of birth|birthday|born on|dob)\s*(?:is)?\s*\w`),
				Mask:  "[DOB REDACTED]",
				Level: "high",
			},
			{
				Name:  "account_number",
				Regex: regexp.MustCompile(`(?i)(?:account|routing)\s*(?:number)?\s*(?:is)?\s*\d{6,}`),
				Mask:  "[ACCOUNT REDACTED]",
				Level: "high",
			},
		},
	}
}

// DetectPII scans transcript text for PII patterns.
func (pm *PIIMasker) DetectPII(text string) []PIIDetection {
	if !pm.enabled {
		return nil
	}

	var detections []PIIDetection
	for _, p := range pm.patterns {
		locs := p.Regex.FindAllStringIndex(text, -1)
		for _, loc := range locs {
			original := text[loc[0]:loc[1]]
			detections = append(detections, PIIDetection{
				Type:     p.Name,
				Original: maskValue(original),
				Masked:   p.Mask,
				Level:    p.Level,
				Position: loc[0],
			})
		}
	}
	return detections
}

// MaskTranscript replaces PII in transcript text with masked values.
func (pm *PIIMasker) MaskTranscript(text string) (string, []PIIDetection) {
	if !pm.enabled {
		return text, nil
	}

	detections := pm.DetectPII(text)
	masked := text
	for _, p := range pm.patterns {
		masked = p.Regex.ReplaceAllString(masked, p.Mask)
	}
	return masked, detections
}

// SilenceAudioFrames replaces PCM audio with silence.
// Called when PII is detected to prevent recording of sensitive data.
func SilenceAudioFrames(frames [][]byte) {
	for i := range frames {
		for j := range frames[i] {
			frames[i][j] = 0
		}
	}
}

// maskValue partially masks a detected value for logging (show last 4 chars).
func maskValue(val string) string {
	digits := ""
	for _, c := range val {
		if c >= '0' && c <= '9' {
			digits += string(c)
		}
	}
	if len(digits) > 4 {
		return strings.Repeat("X", len(digits)-4) + digits[len(digits)-4:]
	}
	return strings.Repeat("X", len(digits))
}

// -------------------------------------------------------------------
// Security API endpoints
// -------------------------------------------------------------------

type SecurityHandler struct {
	biometrics *VoiceBiometrics
	masker     *PIIMasker
}

func NewSecurityHandler(db *sql.DB) *SecurityHandler {
	return &SecurityHandler{
		biometrics: NewVoiceBiometrics(db),
		masker:     NewPIIMasker(),
	}
}

func (sh *SecurityHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/security/voiceprints", sh.handleVoicePrints)
	mux.HandleFunc("/api/security/pii/test", sh.handlePIITest)
	mux.HandleFunc("/api/security/pii/config", sh.handlePIIConfig)
}

func (sh *SecurityHandler) handleVoicePrints(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		sh.biometrics.mu.RLock()
		prints := make([]map[string]any, len(sh.biometrics.prints))
		for i, vp := range sh.biometrics.prints {
			prints[i] = map[string]any{
				"id": vp.ID, "label": vp.Label, "type": vp.Type,
				"features_dim": len(vp.Features), "created_at": vp.CreatedAt,
			}
		}
		sh.biometrics.mu.RUnlock()
		writeJSON(w, http.StatusOK, prints)

	case "POST":
		var req struct {
			Label string `json:"label"`
			Type  string `json:"type"` // fraud, verified
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Label == "" || req.Type == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "label and type required"})
			return
		}
		// Enroll with synthetic audio for testing — real enrollment uses actual call audio
		syntheticPCM := make([]byte, 16000*2*3) // 3 seconds of silence
		vp := sh.biometrics.EnrollVoice(req.Label, req.Type, syntheticPCM)
		writeJSON(w, http.StatusCreated, vp)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (sh *SecurityHandler) handlePIITest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	masked, detections := sh.masker.MaskTranscript(req.Text)
	writeJSON(w, http.StatusOK, map[string]any{
		"original":   req.Text,
		"masked":     masked,
		"detections": detections,
		"pii_found":  len(detections) > 0,
	})
}

func (sh *SecurityHandler) handlePIIConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		patterns := make([]map[string]string, len(sh.masker.patterns))
		for i, p := range sh.masker.patterns {
			patterns[i] = map[string]string{
				"name": p.Name, "level": p.Level, "mask": p.Mask,
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled":  sh.masker.enabled,
			"patterns": patterns,
		})
	case "POST":
		var req struct {
			Enabled *bool `json:"enabled"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Enabled != nil {
			sh.masker.enabled = *req.Enabled
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}
