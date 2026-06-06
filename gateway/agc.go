package main

import (
	"encoding/binary"
	"math"
)

// -------------------------------------------------------------------
// Dynamic Audio Ingress Leveling & Telecom-Specific VAD
//
// Web-based VAD assumes clean microphone input. Telephony audio arrives
// compressed via G.711, heavily degraded, with line static, echo, and
// background noise. This causes standard AI to constantly interrupt
// or stay awkwardly silent.
//
// This module provides:
//   1. Automatic Gain Control (AGC) — normalizes volume levels
//   2. Noise Gate — suppresses low-level background noise
//   3. Telecom VAD — endpointing optimized for 8kHz degraded audio
//   4. Comfort Noise Generation (CNG) — replaces harsh silence
// -------------------------------------------------------------------

// AGC performs Automatic Gain Control on 16-bit LE PCM audio.
// Normalizes the signal level to a target RMS, preventing clipping
// while boosting quiet speakers and taming loud ones.
type AGC struct {
	targetRMS    float64 // target output RMS level (default 3000)
	maxGain      float64 // maximum gain multiplier (default 10.0)
	minGain      float64 // minimum gain multiplier (default 0.1)
	attackRate   float64 // how fast gain increases (0.0-1.0, default 0.01)
	releaseRate  float64 // how fast gain decreases (0.0-1.0, default 0.05)
	currentGain  float64 // current gain level
}

func NewAGC() *AGC {
	return &AGC{
		targetRMS:   3000,
		maxGain:     10.0,
		minGain:     0.1,
		attackRate:  0.01,
		releaseRate: 0.05,
		currentGain: 1.0,
	}
}

// Process applies automatic gain control to a PCM frame.
func (agc *AGC) Process(pcm []byte) []byte {
	n := len(pcm) / 2
	if n == 0 {
		return pcm
	}

	// Compute current RMS
	var sum float64
	for i := 0; i < n; i++ {
		s := float64(int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2])))
		sum += s * s
	}
	rms := math.Sqrt(sum / float64(n))

	// Compute desired gain
	desiredGain := agc.currentGain
	if rms > 0 {
		desiredGain = agc.targetRMS / rms
	}

	// Clamp gain
	if desiredGain > agc.maxGain {
		desiredGain = agc.maxGain
	}
	if desiredGain < agc.minGain {
		desiredGain = agc.minGain
	}

	// Smooth gain changes (attack/release)
	if desiredGain > agc.currentGain {
		agc.currentGain += (desiredGain - agc.currentGain) * agc.attackRate
	} else {
		agc.currentGain += (desiredGain - agc.currentGain) * agc.releaseRate
	}

	// Apply gain
	out := make([]byte, len(pcm))
	for i := 0; i < n; i++ {
		s := float64(int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2])))
		s *= agc.currentGain

		// Soft clipping to prevent harsh distortion
		if s > 32000 {
			s = 32000
		} else if s < -32000 {
			s = -32000
		}

		binary.LittleEndian.PutUint16(out[i*2:i*2+2], uint16(int16(s)))
	}

	return out
}

// -------------------------------------------------------------------
// Noise Gate — suppresses background noise below threshold
// -------------------------------------------------------------------

type NoiseGate struct {
	threshold   float64 // RMS below this = noise (default 80)
	holdFrames  int     // frames to keep open after speech (default 5)
	holdCount   int
	open        bool
}

func NewNoiseGate(threshold float64) *NoiseGate {
	return &NoiseGate{
		threshold:  threshold,
		holdFrames: 5,
	}
}

// Process applies the noise gate. Returns the frame unchanged if speech
// is detected, or a silence frame if below threshold.
func (ng *NoiseGate) Process(pcm []byte) ([]byte, bool) {
	rms := rmsEnergy(pcm)

	if rms > ng.threshold {
		ng.open = true
		ng.holdCount = ng.holdFrames
		return pcm, true // speech
	}

	if ng.holdCount > 0 {
		ng.holdCount--
		return pcm, true // hold open
	}

	ng.open = false
	// Replace with comfort noise instead of harsh silence
	return generateComfortNoise(len(pcm)), false
}

// -------------------------------------------------------------------
// Comfort Noise Generation (CNG)
//
// Replaces dead silence with low-level background hiss.
// Prevents the "is anyone there?" feeling during pauses.
// -------------------------------------------------------------------

func generateComfortNoise(length int) []byte {
	noise := make([]byte, length)
	// Low-level pseudo-random noise (simple LCG)
	seed := uint32(42)
	for i := 0; i < length/2; i++ {
		seed = seed*1103515245 + 12345
		sample := int16((seed >> 16) & 0x1F) - 16 // ±16 range (very quiet)
		binary.LittleEndian.PutUint16(noise[i*2:i*2+2], uint16(sample))
	}
	return noise
}

// -------------------------------------------------------------------
// Telecom Audio Pipeline — chains AGC + Noise Gate + CNG
//
// Call this on every incoming PCM frame before passing to STT.
// Designed for degraded 8kHz telephony audio.
// -------------------------------------------------------------------

type TelecomAudioPipeline struct {
	agc       *AGC
	noiseGate *NoiseGate
	enabled   bool
}

func NewTelecomAudioPipeline() *TelecomAudioPipeline {
	return &TelecomAudioPipeline{
		agc:       NewAGC(),
		noiseGate: NewNoiseGate(80),
		enabled:   true,
	}
}

// Process runs the full telecom audio pipeline on a PCM frame.
// Returns the processed frame and whether speech was detected.
func (tap *TelecomAudioPipeline) Process(pcm []byte) ([]byte, bool) {
	if !tap.enabled {
		return pcm, true
	}

	// Step 1: AGC — normalize volume
	normalized := tap.agc.Process(pcm)

	// Step 2: Noise gate — suppress background noise
	gated, isSpeech := tap.noiseGate.Process(normalized)

	return gated, isSpeech
}
