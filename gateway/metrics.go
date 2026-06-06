package main

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// -------------------------------------------------------------------
// Prometheus-compatible Metrics — /metrics endpoint
//
// Exposes call volume, latency histograms, error rates, circuit
// breaker state, and pipeline stage durations in Prometheus text format.
// No external dependency — implements the text exposition format directly.
// -------------------------------------------------------------------

type Metrics struct {
	// Counters
	callsTotal        atomic.Int64
	callsActive       atomic.Int64
	callsCompleted    atomic.Int64
	callsFailed       atomic.Int64
	robocallsDetected atomic.Int64
	robocallsBlocked  atomic.Int64
	piiDetections     atomic.Int64
	transfersExecuted atomic.Int64
	actionsExecuted   atomic.Int64
	dtmfDigitsCaptured atomic.Int64
	webhooksSent      atomic.Int64
	webhooksFailed    atomic.Int64

	// STT metrics
	sttRequests   atomic.Int64
	sttErrors     atomic.Int64
	sttTotalMs    atomic.Int64

	// TTS metrics
	ttsRequests   atomic.Int64
	ttsErrors     atomic.Int64
	ttsTotalMs    atomic.Int64

	// LLM metrics
	llmRequests   atomic.Int64
	llmErrors     atomic.Int64
	llmTotalMs    atomic.Int64

	// RAG metrics
	ragQueries    atomic.Int64
	ragTotalMs    atomic.Int64

	// Latency histograms (simple bucket counters)
	sttLatencyBuckets  [8]atomic.Int64 // 0-50, 50-100, 100-200, 200-500, 500-1000, 1000-2000, 2000-5000, 5000+
	llmLatencyBuckets  [8]atomic.Int64
	ttsLatencyBuckets  [8]atomic.Int64

	// Copilot
	copilotSessions    atomic.Int64
	suggestionsGenerated atomic.Int64

	// Failover
	failoverTriggered  atomic.Int64

	startTime time.Time
	mu        sync.RWMutex
}

func NewMetrics() *Metrics {
	return &Metrics{startTime: time.Now()}
}

// RecordSTT records an STT request latency.
func (m *Metrics) RecordSTT(duration time.Duration, err error) {
	m.sttRequests.Add(1)
	ms := duration.Milliseconds()
	m.sttTotalMs.Add(ms)
	m.recordBucket(&m.sttLatencyBuckets, ms)
	if err != nil {
		m.sttErrors.Add(1)
	}
}

// RecordLLM records an LLM request latency.
func (m *Metrics) RecordLLM(duration time.Duration, err error) {
	m.llmRequests.Add(1)
	ms := duration.Milliseconds()
	m.llmTotalMs.Add(ms)
	m.recordBucket(&m.llmLatencyBuckets, ms)
	if err != nil {
		m.llmErrors.Add(1)
	}
}

// RecordTTS records a TTS request latency.
func (m *Metrics) RecordTTS(duration time.Duration, err error) {
	m.ttsRequests.Add(1)
	ms := duration.Milliseconds()
	m.ttsTotalMs.Add(ms)
	m.recordBucket(&m.ttsLatencyBuckets, ms)
	if err != nil {
		m.ttsErrors.Add(1)
	}
}

func (m *Metrics) recordBucket(buckets *[8]atomic.Int64, ms int64) {
	switch {
	case ms < 50:
		buckets[0].Add(1)
	case ms < 100:
		buckets[1].Add(1)
	case ms < 200:
		buckets[2].Add(1)
	case ms < 500:
		buckets[3].Add(1)
	case ms < 1000:
		buckets[4].Add(1)
	case ms < 2000:
		buckets[5].Add(1)
	case ms < 5000:
		buckets[6].Add(1)
	default:
		buckets[7].Add(1)
	}
}

// Handler returns the /metrics HTTP handler.
func (m *Metrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")

		uptime := time.Since(m.startTime).Seconds()

		// Gauge
		fmt.Fprintf(w, "# HELP voiceagent_uptime_seconds Gateway uptime\n")
		fmt.Fprintf(w, "voiceagent_uptime_seconds %.1f\n\n", uptime)

		fmt.Fprintf(w, "# HELP voiceagent_calls_active Currently active call sessions\n")
		fmt.Fprintf(w, "voiceagent_calls_active %d\n\n", m.callsActive.Load())

		fmt.Fprintf(w, "# HELP voiceagent_copilot_sessions Active co-pilot sessions\n")
		fmt.Fprintf(w, "voiceagent_copilot_sessions %d\n\n", m.copilotSessions.Load())

		// Counters
		fmt.Fprintf(w, "# HELP voiceagent_calls_total Total calls processed\n")
		fmt.Fprintf(w, "voiceagent_calls_total %d\n\n", m.callsTotal.Load())

		fmt.Fprintf(w, "# HELP voiceagent_calls_completed Total completed calls\n")
		fmt.Fprintf(w, "voiceagent_calls_completed %d\n\n", m.callsCompleted.Load())

		fmt.Fprintf(w, "# HELP voiceagent_calls_failed Total failed calls\n")
		fmt.Fprintf(w, "voiceagent_calls_failed %d\n\n", m.callsFailed.Load())

		fmt.Fprintf(w, "# HELP voiceagent_robocalls_detected Total robocalls detected\n")
		fmt.Fprintf(w, "voiceagent_robocalls_detected %d\n\n", m.robocallsDetected.Load())

		fmt.Fprintf(w, "# HELP voiceagent_robocalls_blocked Total robocalls blocked\n")
		fmt.Fprintf(w, "voiceagent_robocalls_blocked %d\n\n", m.robocallsBlocked.Load())

		fmt.Fprintf(w, "# HELP voiceagent_pii_detections Total PII detections masked\n")
		fmt.Fprintf(w, "voiceagent_pii_detections %d\n\n", m.piiDetections.Load())

		fmt.Fprintf(w, "# HELP voiceagent_transfers_executed Total call transfers\n")
		fmt.Fprintf(w, "voiceagent_transfers_executed %d\n\n", m.transfersExecuted.Load())

		fmt.Fprintf(w, "# HELP voiceagent_actions_executed Total self-service actions\n")
		fmt.Fprintf(w, "voiceagent_actions_executed %d\n\n", m.actionsExecuted.Load())

		fmt.Fprintf(w, "# HELP voiceagent_dtmf_digits Total DTMF digits captured\n")
		fmt.Fprintf(w, "voiceagent_dtmf_digits %d\n\n", m.dtmfDigitsCaptured.Load())

		fmt.Fprintf(w, "# HELP voiceagent_suggestions_generated Total co-pilot suggestions\n")
		fmt.Fprintf(w, "voiceagent_suggestions_generated %d\n\n", m.suggestionsGenerated.Load())

		fmt.Fprintf(w, "# HELP voiceagent_failover_triggered Total failover events\n")
		fmt.Fprintf(w, "voiceagent_failover_triggered %d\n\n", m.failoverTriggered.Load())

		// STT
		fmt.Fprintf(w, "# HELP voiceagent_stt_requests_total Total STT requests\n")
		fmt.Fprintf(w, "voiceagent_stt_requests_total %d\n", m.sttRequests.Load())
		fmt.Fprintf(w, "voiceagent_stt_errors_total %d\n", m.sttErrors.Load())
		sttAvg := int64(0)
		if m.sttRequests.Load() > 0 {
			sttAvg = m.sttTotalMs.Load() / m.sttRequests.Load()
		}
		fmt.Fprintf(w, "voiceagent_stt_avg_latency_ms %d\n\n", sttAvg)

		// LLM
		fmt.Fprintf(w, "# HELP voiceagent_llm_requests_total Total LLM requests\n")
		fmt.Fprintf(w, "voiceagent_llm_requests_total %d\n", m.llmRequests.Load())
		fmt.Fprintf(w, "voiceagent_llm_errors_total %d\n", m.llmErrors.Load())
		llmAvg := int64(0)
		if m.llmRequests.Load() > 0 {
			llmAvg = m.llmTotalMs.Load() / m.llmRequests.Load()
		}
		fmt.Fprintf(w, "voiceagent_llm_avg_latency_ms %d\n\n", llmAvg)

		// TTS
		fmt.Fprintf(w, "# HELP voiceagent_tts_requests_total Total TTS requests\n")
		fmt.Fprintf(w, "voiceagent_tts_requests_total %d\n", m.ttsRequests.Load())
		fmt.Fprintf(w, "voiceagent_tts_errors_total %d\n", m.ttsErrors.Load())
		ttsAvg := int64(0)
		if m.ttsRequests.Load() > 0 {
			ttsAvg = m.ttsTotalMs.Load() / m.ttsRequests.Load()
		}
		fmt.Fprintf(w, "voiceagent_tts_avg_latency_ms %d\n\n", ttsAvg)

		// RAG
		fmt.Fprintf(w, "# HELP voiceagent_rag_queries_total Total RAG queries\n")
		fmt.Fprintf(w, "voiceagent_rag_queries_total %d\n\n", m.ragQueries.Load())

		// Webhooks
		fmt.Fprintf(w, "# HELP voiceagent_webhooks_sent Total webhooks sent\n")
		fmt.Fprintf(w, "voiceagent_webhooks_sent %d\n", m.webhooksSent.Load())
		fmt.Fprintf(w, "voiceagent_webhooks_failed %d\n\n", m.webhooksFailed.Load())

		// Latency histograms
		bucketLabels := []string{"0-50ms", "50-100ms", "100-200ms", "200-500ms", "500-1000ms", "1-2s", "2-5s", "5s+"}

		fmt.Fprintf(w, "# HELP voiceagent_stt_latency_bucket STT latency distribution\n")
		for i, label := range bucketLabels {
			fmt.Fprintf(w, "voiceagent_stt_latency_bucket{le=\"%s\"} %d\n", label, m.sttLatencyBuckets[i].Load())
		}

		fmt.Fprintf(w, "\n# HELP voiceagent_llm_latency_bucket LLM latency distribution\n")
		for i, label := range bucketLabels {
			fmt.Fprintf(w, "voiceagent_llm_latency_bucket{le=\"%s\"} %d\n", label, m.llmLatencyBuckets[i].Load())
		}

		fmt.Fprintf(w, "\n# HELP voiceagent_tts_latency_bucket TTS latency distribution\n")
		for i, label := range bucketLabels {
			fmt.Fprintf(w, "voiceagent_tts_latency_bucket{le=\"%s\"} %d\n", label, m.ttsLatencyBuckets[i].Load())
		}
	}
}
