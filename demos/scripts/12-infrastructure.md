# Demo 12: Infrastructure — Failover, DTMF, Metrics

**Duration:** ~2 minutes
**Format:** Screen recording (terminal + browser)
**Audience:** SREs, infrastructure engineers, NOC operators

---

## Scene 1: Failover Circuit Breakers (0:00 - 0:40)

**Browser:** http://localhost:3000/infrastructure — Failover tab

**Terminal:**
```bash
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/failover/status | python3 -m json.tool
```

**Narration:**
> "Four circuit breakers monitor STT, TTS, LLM, and ESL. Each uses atomic CAS for sub-millisecond state transitions — closed, open, half-open. When all circuits open, the gateway issues a SIP REFER to transfer the caller to a human agent queue at extension 3000, with X-Failover headers documenting what went wrong."

---

## Scene 2: DTMF Parsing (0:40 - 1:10)

**Browser:** http://localhost:3000/infrastructure — DTMF tab

**Terminal:**
```bash
curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/dtmf/test \
  -d '{"text":"482910"}' | python3 -m json.tool
```

**Narration:**
> "DTMF capture intercepts RFC 2833 RTP event packets directly — 100% accurate, no audio processing needed. The Goertzel algorithm runs as a fallback for legacy PBX systems that send inband tones. The LLM receives 'User typed: 482910' instead of trying to hear button presses."

---

## Scene 3: Prometheus + Grafana (1:10 - 1:45)

**Browser:** http://localhost:9090 (Prometheus), then http://localhost:3001 (Grafana)

**Terminal:**
```bash
curl -s http://localhost:8080/metrics | grep -E "voiceagent_(calls|stt|llm|tts)" | head -12
```

**Narration:**
> "30+ metrics in Prometheus text format. Call counts, STT/LLM/TTS latency histograms with 8 buckets, robocall detections, PII events, failover triggers, and infrastructure health. Prometheus scrapes every 5 seconds. Grafana visualizes the pipeline latency budget in real time."

---

## Scene 4: Scale Status (1:45 - 2:00)

**Terminal:**
```bash
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/scale/status | python3 -m json.tool
```

**Narration:**
> "The admission controller caps concurrent sessions at 500. Token bucket rate limiter at 100 requests per second per IP. STT and TTS worker pools round-robin across replicas with automatic health recovery. Scale horizontally by adding Redis and pointing multiple gateways at the same session store."
