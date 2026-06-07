# Demo 9: Platform Overview

**Duration:** ~3 minutes
**Format:** Screen recording (terminal + browser)
**Audience:** VoIP architects, CTOs evaluating AI call center platforms

---

## Scene 1: Deploy (0:00 - 0:30)

**Terminal:**
```bash
export EXT_IP=$(ipconfig getifaddr en0)
docker compose -f docker-compose.sip.yml up -d
docker compose -f docker-compose.sip.yml ps
curl -s http://localhost:8080/healthz | python3 -m json.tool
```

**Narration:**
> "VoiceAgent is a telecom-native AI media gateway — 10 services, one command. You get a Go B2BUA, FreeSWITCH SIP engine, local Whisper STT, local Piper TTS, PostgreSQL, ChromaDB vector store, Redis, Prometheus, Grafana, and a Next.js operations dashboard. Everything runs locally — no cloud dependencies except the LLM."

---

## Scene 2: Dashboard Tour (0:30 - 1:30)

**Browser:** http://localhost:3000

**Actions:**
1. Login with admin/admin
2. **Command Center** — point out stats cards (Active Calls, Total Today, Avg Duration, Sentiment)
3. **Agents** — show agent table, add an agent
4. **Calls** — show call history, filters
5. **Documents** — show uploaded knowledge base docs, RAG search
6. **Settings** — show LLM config (Claude/Gemini), system prompt editor, SBC settings
7. **Security** — robocall detection, PII masking, voice biometrics tabs
8. **Infrastructure** — failover circuit status, worker pools, DTMF parser, Prometheus metrics

**Narration:**
> "The command center gives you real-time visibility into every call. You can manage agents, browse call history with sentiment analysis, upload knowledge base documents for RAG-powered coaching, and configure LLM providers. The security panel covers robocall detection, PII masking, and voice biometrics. Infrastructure shows failover circuits, scaling status, and raw Prometheus metrics."

---

## Scene 3: Live AI Call (1:30 - 2:30)

**Terminal:**
```bash
cd test && timeout 25 go run simcall.go ws://localhost:8080/ws
```

**Browser:** Show http://localhost:3000/calls (refresh to see new call appear)

**Narration:**
> "When a call comes in through the SBC, FreeSWITCH forks the audio via WebSocket to the Go gateway. The gateway runs a 5-goroutine pipeline: VAD detects speech, Whisper transcribes locally, PII gets masked, Claude generates a response, and Piper synthesizes speech — all in under 3 seconds. The audio gets transcoded from G.711 to linear PCM using a pre-computed lookup table — sub-millisecond, no FFmpeg."

---

## Scene 4: Metrics (2:30 - 3:00)

**Browser:** http://localhost:9090 (Prometheus) and http://localhost:3001 (Grafana)

**Terminal:**
```bash
curl -s http://localhost:8080/metrics | grep "voiceagent_" | head -10
```

**Narration:**
> "30+ Prometheus metrics track every stage of the pipeline — call counts, STT/LLM/TTS latencies with histogram buckets, robocall detections, PII events, and failover state. Grafana dashboards visualize it all in real time."

---

## Closing

**Narration:**
> "VoiceAgent deploys anywhere — Docker Compose for dev, Kubernetes with Istio service mesh for production. Cloud, on-prem, or air-gapped. Built for network engineers who ship infrastructure, not web developers who ship wrappers. Link in the description."
