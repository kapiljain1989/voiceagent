package main

import (
	"encoding/binary"
	"math"
	"sync"
)

// VoiceSentiment analyzes acoustic features from raw PCM audio to detect
// emotional state: pitch trend, energy trend, speaking rate, silence ratio.
// Combined with text-based sentiment from Claude for multi-modal scoring.
type VoiceSentiment struct {
	mu sync.Mutex

	// Per-utterance acoustic features
	rmsHistory    []float64 // RMS energy per frame
	pitchHistory  []float64 // Estimated F0 (Hz) per frame
	speechFrames  int       // frames with speech detected
	silenceFrames int       // frames with silence
	totalFrames   int
	utteranceCount int

	// Aggregated scores
	avgRMS       float64
	rmsVariance  float64
	avgPitch     float64
	pitchVariance float64
	speakingRate float64 // words per minute (set externally from Whisper)
	wordCount    int
	startTimeMs  int64
}

type SentimentResult struct {
	// Acoustic features
	AvgEnergy      float64 `json:"avg_energy"`
	EnergyTrend    string  `json:"energy_trend"`    // rising, falling, stable
	AvgPitch       float64 `json:"avg_pitch_hz"`
	PitchVariance  float64 `json:"pitch_variance"`
	SpeakingRate   float64 `json:"speaking_rate_wpm"`
	SilenceRatio   float64 `json:"silence_ratio"`

	// Derived sentiment signals
	Agitation      float64 `json:"agitation"`       // 0.0-1.0
	Engagement     float64 `json:"engagement"`      // 0.0-1.0
	Frustration    float64 `json:"frustration"`     // 0.0-1.0

	// Combined score
	Sentiment      string  `json:"sentiment"`       // positive, neutral, negative
	Confidence     float64 `json:"confidence"`      // 0.0-1.0
}

func NewVoiceSentiment() *VoiceSentiment {
	return &VoiceSentiment{
		rmsHistory:   make([]float64, 0, 500),
		pitchHistory: make([]float64, 0, 500),
	}
}

// ProcessFrame analyzes a 20ms PCM frame (16-bit LE, 16kHz mono)
func (vs *VoiceSentiment) ProcessFrame(pcm []byte) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	vs.totalFrames++

	rms := rmsEnergy(pcm)
	vs.rmsHistory = append(vs.rmsHistory, rms)

	if rms > float64(vadRMSThreshold) {
		vs.speechFrames++

		// Estimate pitch via autocorrelation
		pitch := estimatePitch(pcm, sampleRate)
		if pitch > 50 && pitch < 500 {
			vs.pitchHistory = append(vs.pitchHistory, pitch)
		}
	} else {
		vs.silenceFrames++
	}
}

// AddUtterance records a transcribed utterance for speaking rate calculation
func (vs *VoiceSentiment) AddUtterance(wordCount int) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.wordCount += wordCount
	vs.utteranceCount++
}

// Analyze computes the final sentiment result from all collected data
func (vs *VoiceSentiment) Analyze() SentimentResult {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	result := SentimentResult{
		Sentiment:  "neutral",
		Confidence: 0.5,
	}

	if vs.totalFrames == 0 {
		return result
	}

	// Silence ratio
	result.SilenceRatio = float64(vs.silenceFrames) / float64(vs.totalFrames)

	// Average RMS energy
	if len(vs.rmsHistory) > 0 {
		sum := 0.0
		for _, r := range vs.rmsHistory {
			sum += r
		}
		result.AvgEnergy = sum / float64(len(vs.rmsHistory))

		// Energy variance
		varSum := 0.0
		for _, r := range vs.rmsHistory {
			d := r - result.AvgEnergy
			varSum += d * d
		}
		vs.rmsVariance = varSum / float64(len(vs.rmsHistory))

		// Energy trend (compare first half vs second half)
		result.EnergyTrend = computeTrend(vs.rmsHistory)
	}

	// Average pitch
	if len(vs.pitchHistory) > 0 {
		sum := 0.0
		for _, p := range vs.pitchHistory {
			sum += p
		}
		result.AvgPitch = sum / float64(len(vs.pitchHistory))

		// Pitch variance (jitter = emotional instability)
		varSum := 0.0
		for _, p := range vs.pitchHistory {
			d := p - result.AvgPitch
			varSum += d * d
		}
		result.PitchVariance = math.Sqrt(varSum / float64(len(vs.pitchHistory)))
	}

	// Speaking rate (words per minute)
	durationSecs := float64(vs.totalFrames) * float64(vadFrameMs) / 1000.0
	if durationSecs > 0 && vs.wordCount > 0 {
		result.SpeakingRate = float64(vs.wordCount) / (durationSecs / 60.0)
	}

	// --- Derive sentiment signals ---

	// Agitation: high energy + high pitch variance + fast speaking
	energyScore := clamp(result.AvgEnergy/500.0, 0, 1)
	pitchJitter := clamp(result.PitchVariance/50.0, 0, 1)
	speedScore := clamp((result.SpeakingRate-120)/80.0, 0, 1) // >120 wpm = fast
	result.Agitation = clamp((energyScore*0.3 + pitchJitter*0.4 + speedScore*0.3), 0, 1)

	// Engagement: low silence ratio + consistent speech
	result.Engagement = clamp(1.0-result.SilenceRatio, 0, 1)

	// Frustration: rising energy + high pitch + agitation
	risingEnergy := 0.0
	if result.EnergyTrend == "rising" {
		risingEnergy = 0.5
	}
	result.Frustration = clamp((result.Agitation*0.4 + risingEnergy*0.3 + pitchJitter*0.3), 0, 1)

	// Overall sentiment
	if result.Frustration > 0.6 || result.Agitation > 0.7 {
		result.Sentiment = "negative"
		result.Confidence = clamp(result.Frustration, 0.5, 1.0)
	} else if result.Engagement > 0.7 && result.Frustration < 0.3 {
		result.Sentiment = "positive"
		result.Confidence = clamp(result.Engagement, 0.5, 1.0)
	} else {
		result.Sentiment = "neutral"
		result.Confidence = 0.6
	}

	return result
}

// estimatePitch uses autocorrelation to find the fundamental frequency (F0)
func estimatePitch(pcm []byte, sr int) float64 {
	n := len(pcm) / 2
	if n < 160 {
		return 0
	}

	samples := make([]float64, n)
	for i := 0; i < n; i++ {
		samples[i] = float64(int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2])))
	}

	// Autocorrelation for lag range 30-500 Hz (sr/500 to sr/30 samples)
	minLag := sr / 500 // 32 samples at 16kHz
	maxLag := sr / 80  // 200 samples at 16kHz
	if maxLag > n/2 {
		maxLag = n / 2
	}

	bestLag := 0
	bestCorr := 0.0

	for lag := minLag; lag <= maxLag; lag++ {
		corr := 0.0
		norm1 := 0.0
		norm2 := 0.0
		for i := 0; i < n-lag; i++ {
			corr += samples[i] * samples[i+lag]
			norm1 += samples[i] * samples[i]
			norm2 += samples[i+lag] * samples[i+lag]
		}
		norm := math.Sqrt(norm1 * norm2)
		if norm > 0 {
			corr /= norm
		}
		if corr > bestCorr {
			bestCorr = corr
			bestLag = lag
		}
	}

	if bestLag == 0 || bestCorr < 0.3 {
		return 0
	}

	return float64(sr) / float64(bestLag)
}

func computeTrend(values []float64) string {
	if len(values) < 10 {
		return "stable"
	}

	half := len(values) / 2
	firstHalf := 0.0
	for _, v := range values[:half] {
		firstHalf += v
	}
	firstHalf /= float64(half)

	secondHalf := 0.0
	for _, v := range values[half:] {
		secondHalf += v
	}
	secondHalf /= float64(len(values) - half)

	ratio := secondHalf / (firstHalf + 1)
	if ratio > 1.2 {
		return "rising"
	} else if ratio < 0.8 {
		return "falling"
	}
	return "stable"
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
