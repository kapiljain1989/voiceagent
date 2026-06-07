# VoiceAgent: Telecom-Native AI Call Center Platform

A plug-and-play **AI co-pilot and call intelligence platform** for enterprise contact centers. VoiceAgent observes live calls via SIPREC, transcribes in real time, provides agent coaching suggestions, detects robocalls and PII, analyzes voice sentiment, and generates post-call summaries — all running on-premises with zero cloud dependency except the LLM.

**Two deployment modes:** Standalone helper (plug into any SBC, no FreeSWITCH) or full B2BUA gateway (interactive AI agent answers calls).

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-10_services-2496ED?logo=docker)](docker-compose.sip.yml)
[![Kubernetes](https://img.shields.io/badge/K8s-Istio_+_Gateway_API-326CE5?logo=kubernetes)](k8s/)

---

### Core Architecture Capabilities

- **SIP Protocol Alignment:** Full support for standard SIP signaling (RFC 3261), native `SIPREC` session forking (RFC 7866), context-enriched call transfers (`SIP REFER`), and `ESL` channel control.
- **Low-Latency Media Transcoding:** In-memory streaming transcoding of G.711 µ-law/A-law (8kHz, 8-bit) directly into raw linear PCM via pre-computed 256-entry lookup tables — sub-1ms per 20ms frame, zero external dependencies.
- **Network-Level Signal Interception:** Native **RFC 2833 / RFC 4733** RTP event packet parsing for hardware DTMF capture. Goertzel algorithm fallback for inband tone detection on legacy PBX systems.
- **Telephony-Grade Audio Processing:** Automatic Gain Control (AGC), noise gate, and Comfort Noise Generation (CNG) — purpose-built for degraded 8kHz carrier lines, not clean WebRTC microphone input.
- **Deterministic Failover:** Circuit breaker per service with atomic state transitions (< 1ms detection). Graceful degradation chain: hold audio → auto-reconnect → SIP REFER to human queue.

---

## Deployment Modes

### Mode 1: Standalone Helper (Recommended for Production)

Plug-and-play — point your SBC's SIPREC recording server to VoiceAgent. No FreeSWITCH, no dialplan, no trunk config. The SBC/PBX owns the call; VoiceAgent is a read-only observer.

```
Customer ──► SBC/PBX ──► Human Agent
                │
                └── SIPREC (RFC 7866) ──► VoiceAgent :5060
                                              │
                                              ├── Live transcript (Whisper STT)
                                              ├── Co-pilot suggestions (RAG + Claude)
                                              ├── Robocall detection (3-layer)
                                              ├── PII masking (9 patterns)
                                              ├── Voice sentiment (pitch/energy/rate)
                                              └── SSE → Agent dashboard :3000
```

```bash
# Deploy (8 services, no FreeSWITCH)
docker compose -f docker-compose.helper.yml up -d

# SBC config: point SIPREC recording server to <voiceagent-ip>:5061
# That's it. Zero VoiceAgent-side configuration.
```

### Mode 2: Full B2BUA Gateway (Demo / Interactive AI)

VoiceAgent answers calls directly — AI agent speaks to the customer via Piper TTS.

```
Customer ──► FreeSWITCH ──► Gateway AI Pipeline ──► TTS response
                                  │
                            Whisper → Claude → Piper
```

```bash
# Deploy (10 services + optional Kamailio SBC lab)
docker compose -f docker-compose.sip.yml up -d
```

### Port Binding & Network Prerequisites

| Port | Protocol | Binding | Service | Exposure |
|------|----------|---------|---------|----------|
| 5060/5070 | SIP (TCP+UDP) | Host network | FreeSWITCH SIP signaling | Required: inbound/outbound SIP |
| 20000-20020 | RTP (UDP) | 1:1 mapped | FreeSWITCH RTP media | Required: bidirectional audio |
| 8080 | HTTP/WS | Container | Go gateway API + WebSocket | Internal: service mesh |
| 8000 | HTTP | Container | Whisper STT API | Internal |
| 5000 | HTTP | Container | Piper TTS API | Internal |
| 5432 | TCP | Container | PostgreSQL | Internal |
| 8200 | HTTP | Container | ChromaDB vector store | Internal |
| 3000 | HTTP | Container | Next.js dashboard | User-facing |
| 8022 | TCP | Container | FreeSWITCH ESL | Internal: gateway only |
| 6379 | TCP | Container | Redis (session store) | Internal |
| 9090 | HTTP | Container | Prometheus metrics | Monitoring |
| 3001 | HTTP | Container | Grafana dashboards | Monitoring |

---

## Why VoiceAgent? (vs. Cloud Voice Wrappers)

| Capability | Cloud AI Voice Bots | VoiceAgent |
|------------|:-------------------:|:----------:|
| Deploys inside enterprise private network | No | **Yes** |
| Air-gapped operation (zero internet) | No | **Yes** |
| Native SIP/RTP protocol handling | No (WebRTC bridge) | **Yes** (B2BUA) |
| SIPREC dual-stream session recording | No | **Yes** (RFC 7866) |
| G.711 codec transcoding (no FFmpeg) | No (cloud transcode) | **Yes** (< 1ms LUT) |
| RFC 2833 hardware DTMF capture | No (audio-based) | **Yes** (100% accuracy) |
| Telephony-grade AGC/noise gate | No (WebRTC VAD) | **Yes** (8kHz optimized) |
| PII masking before cloud/recording | No | **Yes** (PCI/HIPAA) |
| Deterministic SIP failover | No (call drops) | **Yes** (< 1ms detection) |
| Call transfer with context headers | No | **Yes** (X-Transfer-Summary) |
| Voice biometric fraud detection | No | **Yes** (spectral fingerprint) |

---

## Local Lab Quickstart

Spin up the complete telecom laboratory — FreeSWITCH SIP engine, Go media gateway, STT, TTS, database, vector store, and dashboard — in a single command.

### Prerequisites

```bash
# macOS
brew install docker go node portaudio baresip

# GCP credentials (for Claude/Gemini on Vertex AI only — all other services run locally)
gcloud auth application-default login
export ANTHROPIC_VERTEX_PROJECT_ID="your-project-id"
export CLOUD_ML_REGION="us-east5"
```

### Deploy

```bash
git clone https://github.com/kapiljain1989/voiceagent.git
cd voiceagent

# Set your host IP for SIP RTP address advertisement in SDP
export EXT_IP=$(ipconfig getifaddr en0)

# Start all 10 services
docker compose -f docker-compose.sip.yml up -d

# Verify SIP signaling binding
nc -z localhost 5070 && echo "SIP: OK"

# Verify gateway
curl -s http://localhost:8080/healthz
# → {"status":"ok","sessions":0}

# Open the operations dashboard
open http://localhost:3000
```

Your local environment is now accepting inbound SIP INVITEs and routing media through the AI pipeline.

### Kubernetes Deployment (Istio + Gateway API)

All 10 services deploy to K8s with Istio service mesh (STRICT mTLS) and Gateway API for HTTP/WebSocket ingress. FreeSWITCH is excluded from the mesh — SIP/RTP requires direct UDP access.

```bash
# Local KinD cluster (hostNetwork FreeSWITCH)
make all                    # kind-up → build-all → load-all → secret → deploy-local

# Cloud (GKE/EKS/AKS — LoadBalancer for SIP/RTP)
make deploy-cloud           # installs Istio, applies cloud overlay
make freeswitch-ip          # discovers LB IP, configures FreeSWITCH ext-rtp-ip

# Enterprise on-premises (MetalLB L2 for FreeSWITCH LB IP)
make deploy-on-prem         # installs Istio + MetalLB, applies on-prem overlay

# Air-gapped (zero internet — local Ollama LLM, no egress ServiceEntries)
kubectl apply -k k8s/overlays/air-gapped
```

```bash
# Verify mesh status
make mesh-status            # istioctl proxy-status + mTLS check

# Port-forward services for local access
make port-forward-ui        # localhost:3000
make port-forward-grafana   # localhost:3001
make port-forward-prometheus # localhost:9090
```

---

## Go Concurrency Architecture

The gateway uses Go's goroutine/channel model to manage concurrent media buffers without locks on the hot path. Each call session spawns a pipeline of single-responsibility goroutines connected by typed channels:

### Interactive Mode — 5 goroutines per session

```
readFromFS ──[chan]──▶ sttPipeline ──[chan]──▶ claudeWorker ──[chan]──▶ ttsWorker ──[chan]──▶ writeToFS
  (WS reader)          (VAD+Whisper)           (LLM+actions)           (Piper TTS)           (ESL play)
                       + PII masking           + self-service
                       + robocall L3           + SIP transfer
                       + biometrics
```

### Co-Pilot Mode — 5 goroutines per session

```
readCaller ──[chan]──▶ callerSTT ──┐
                                   ├──[chan]──▶ coachWorker ──▶ SSE broadcast
readAgent  ──[chan]──▶ agentSTT  ──┘             (RAG+Claude)   → agent dashboard
                                    + PII mask                  → post-call summary
                                    + robocall                  → CRM webhook
```

Shutdown propagates via channel-close cascade. Circuit breakers use `sync/atomic` for lock-free state transitions.

---

## Feature Matrix

### Call Processing

| Feature | Description |
|---------|-------------|
| **Interactive AI Agent** | Customer speaks → Whisper STT → Claude → Piper TTS → voice response |
| **Co-Pilot Agent Assist** | SIPREC dual-leg observation → RAG-grounded coaching → SSE to agent |
| **Self-Service Actions** | Natural language → intent parsing → CRM API execution → voice confirmation |
| **Intelligent Transfer** | Anger/complexity detection → SIP REFER with X-Transfer-Summary headers |
| **Post-Call Summary** | Auto-generated summary + action items + sentiment → CRM webhook |

### Security & Compliance

| Feature | Description |
|---------|-------------|
| **PII Masking** | 9 patterns (credit card, SSN, CVV, DOB, account, spoken digits) — masks before LLM/recording |
| **Voice Biometrics** | 32-dim spectral fingerprint — fraud detection + identity verification |
| **Robocall Detection** | 3-layer: blocklist (< 1ms) + audio pattern + 28 keyword phrases |
| **Voice Sentiment** | Acoustic emotion detection — pitch, energy, speaking rate, agitation, frustration scoring |

### Telecom Infrastructure

| Feature | Description |
|---------|-------------|
| **G.711 Transcoding** | Pre-computed 256-entry LUT — O(1) per sample, < 1ms per frame |
| **RFC 2833 DTMF** | Hardware digit capture from RTP event packets — 100% accurate |
| **Telecom AGC** | Automatic Gain Control + noise gate + comfort noise (CNG) |
| **SIPREC Parser** | RFC 7866 metadata XML — participant/stream resolution, diarization labels |
| **Failover Machine** | Circuit breaker per service — hold audio → reconnect → SIP REFER fallback |
| **SBC Profiles** | Pre-configured for Cisco CUBE, AudioCodes Mediant |

### Multi-LLM Support

| Provider | Models | Streaming |
|----------|--------|:---------:|
| **Claude (Vertex AI)** | claude-3-5-haiku, sonnet, opus | Yes |
| **Gemini (Vertex AI)** | gemini-2.0-flash, gemini-1.5-pro | Yes |
| **Ollama (air-gapped)** | llama, mistral, phi | Planned |

---

## SBC Integration

### Standalone Helper (1-line SBC config)

Point your SBC's SIPREC recording server to VoiceAgent — nothing else needed on the VoiceAgent side:

| SBC | Configuration |
|-----|--------------|
| **Cisco CUBE** | `media-recording <VOICEAGENT_IP> port 5060` |
| **AudioCodes** | SIP Recording → Recording Server = `<VOICEAGENT_IP>:5060` |
| **Oracle SBC** | session-recording → destination = `sip:<VOICEAGENT_IP>:5060` |
| **Kamailio** | `siprec_start_recording("sip:<VOICEAGENT_IP>:5060")` |

Full guide: [`docs/sbc-configuration.md`](docs/sbc-configuration.md)

### Full Gateway Mode (Enterprise SBC Profiles)

Pre-configured profiles at `freeswitch/config/sip_profiles/enterprise/`:

| SBC | Profile | Codecs | Features |
|-----|---------|--------|----------|
| **Cisco CUBE** | `cisco-cube.xml` | G.711/G.722/G.729 | TLS, session timers, OPTIONS keepalive |
| **AudioCodes Mediant** | `audiocodes.xml` | G.711/G.722/G.729 | Registration mode, NAT traversal, rport |

### Local SBC Lab (Mobile Softphone Testing)

Test with real voice calls from your mobile phone on your home LAN:

```bash
export EXT_IP=$(ipconfig getifaddr en0)
make sbc-lab    # 11 services: 10 + Kamailio SBC
```

Softphone config: SIP server `<LAN_IP>`, port `5090`, TCP, username `customer1`, password `1234`.
Dial `1000` (AI agent) or `2001` (co-pilot with agent). Guide: [`docs/softphone-setup.md`](docs/softphone-setup.md)

---

## Production Scale Infrastructure

### Istio Service Mesh + Gateway API

All 10 services deploy to K8s with Istio for zero-trust networking:

| Layer | Implementation | Purpose |
|-------|----------------|---------|
| **mTLS** | Istio PeerAuthentication (STRICT) | Encrypted service-to-service traffic |
| **Circuit Breaking** | Istio DestinationRules | Connection pool limits, outlier ejection per service |
| **Authorization** | Istio AuthorizationPolicies | Deny-by-default, per-service allow rules |
| **Ingress** | Gateway API HTTPRoutes | HTTP/WebSocket routing to gateway API, UI, Grafana |
| **Egress** | Istio ServiceEntries | Controlled access to Vertex AI + HuggingFace |
| **Network Policies** | K8s NetworkPolicy | CNI-level defense-in-depth under Istio |
| **Observability** | Istio Telemetry → Prometheus | Envoy sidecar metrics + access logging |

FreeSWITCH is excluded from the mesh (`sidecar.istio.io/inject: "false"`) — SIP/RTP requires raw UDP access. In cloud environments, a dedicated LoadBalancer with `externalTrafficPolicy: Local` exposes SIP (5060) and RTP (16000-16020) directly. On-prem uses MetalLB L2 for the same purpose.

### Horizontal Gateway Scaling (Redis)

The gateway is stateless when `REDIS_URL` is set. Session state, pub/sub for SSE event routing, and distributed counters all run through Redis. Deploy N gateway replicas behind a load balancer.

```bash
# Enable distributed state
REDIS_URL=redis://redis:6379/0 docker compose up --scale gateway=3
```

Falls back to local in-memory store when Redis is not configured — zero-config for development.

### STT/TTS Worker Pools

Scale Whisper and Piper horizontally with comma-separated URLs. The gateway round-robins requests across the pool with automatic health checking and recovery.

```bash
# 3 Whisper workers
STT_URL=http://whisper-1:8000/v1/audio/transcriptions,http://whisper-2:8000/v1/audio/transcriptions,http://whisper-3:8000/v1/audio/transcriptions

# 2 Piper workers
TTS_URL=http://piper-1:5000,http://piper-2:5000
```

Health checks run every 30s. Unhealthy workers are removed from rotation and auto-recover after cooldown. Pool status: `GET /api/scale/status`

### Rate Limiting & Admission Control

| Control | Default | Description |
|---------|---------|-------------|
| **Rate limiter** | 100 req/s per IP, burst 200 | Token bucket with `X-Forwarded-For` support |
| **Admission controller** | 500 max concurrent sessions | Atomic CAS — rejects new calls when at capacity |

### Prometheus Metrics & Grafana

`GET /metrics` exposes 30+ metrics in Prometheus text format:

| Category | Metrics |
|----------|---------|
| **Calls** | total, active, completed, failed |
| **STT** | requests, errors, avg latency, latency histogram (8 buckets) |
| **LLM** | requests, errors, avg latency, latency histogram |
| **TTS** | requests, errors, avg latency, latency histogram |
| **Security** | robocalls detected/blocked, PII detections |
| **Actions** | transfers executed, self-service actions, DTMF digits |
| **Infrastructure** | failover events, webhooks sent/failed, co-pilot suggestions |

- **Prometheus:** http://localhost:9090 (5s scrape interval)
- **Grafana:** http://localhost:3001 (admin / `voiceagent`)

---

## Operational Resilience

### B2BUA Survival State

| Failure Scenario | Standard AI Bot | VoiceAgent |
|------------------|----------------|------------|
| Cloud AI API drops | Call goes silent, drops | Plays hold audio, SIP REFER, saves the call |
| User enters credit card | Audio captured to cloud logs | PII masked, audio frames silenced |
| Heavy background static | AI constantly interrupts | AGC + noise gate enables natural turn-taking |
| DTMF input needed | Tries to "hear" tone via ASR | Intercepts RFC 2833 packets — 100% accurate |
| Internet connection lost | Complete failure | Local emergency dialplan + SIP REFER |

### Latency Budget

| Stage | Duration |
|-------|----------|
| G.711 → L16 transcoding | < 1ms |
| RFC 2833 DTMF parsing | < 1ms |
| Blocklist + PII masking | < 1ms |
| Circuit breaker detection | < 1ms |
| AGC + noise gate | < 1ms |
| VAD silence detection | 500ms |
| Whisper STT | 200-400ms |
| RAG query (ChromaDB) | 50-100ms |
| Claude/Gemini response | 1-2s |
| Piper TTS synthesis | 200-400ms |
| **Total: Co-pilot** | **~2s** |
| **Total: Interactive** | **~3s** |

---

## SDKs

| Language | Package | Install |
|----------|---------|---------|
| **Python** | `voiceagent` | `pip install -e sdk/python` |
| **TypeScript** | `@voiceagent/sdk` | `cd sdk/typescript && npm install && npm run build` |

```python
# Python
from voiceagent import VoiceAgentClient
client = VoiceAgentClient("http://localhost:8080")
results = client.rag_search("water damage coverage")
pii = client.test_pii("My SSN is 123-45-6789")
```

```typescript
// TypeScript
import { VoiceAgentClient } from "@voiceagent/sdk";
const client = new VoiceAgentClient("http://localhost:8080");
const results = await client.ragSearch("water damage");
const cleanup = client.streamEvents("call-id", (e) => console.log(e));
```

---

## API Surface (25+ endpoints)

Full documentation: [`docs/api-reference.md`](docs/api-reference.md)

| Category | Endpoints |
|----------|-----------|
| **Voice** | `/ws` (WebSocket), `/siprec` (WebSocket), `/siprec/events` (SSE), `/call` (POST) |
| **Agents** | `/api/agents` (CRUD) |
| **Calls** | `/api/calls`, `/api/calls/active` |
| **Documents** | `/api/documents`, `/api/documents/search` |
| **LLM** | `/api/llm/configs`, `/api/llm/test` |
| **Security** | `/api/blocklist`, `/api/robocall/*`, `/api/security/voiceprints`, `/api/security/pii/*` |
| **Actions** | `/api/actions/webhooks`, `/api/actions/test` |
| **Co-Pilot** | `/api/copilot/active` (live sessions with voice sentiment + caller/agent identity) |
| **Infrastructure** | `/api/failover/status`, `/api/scale/status`, `/api/dtmf/test`, `/api/stats`, `/healthz`, `/metrics` |

---

## Project Structure (20 Go source files, 8-11 services)

```
voiceagent/
├── gateway/
│   ├── main.go             # B2BUA core: WebSocket, VAD, ESL, session orchestration
│   ├── sipserver.go         # Native SIP UAS (sipgo) — accepts SIPREC INVITEs directly
│   ├── rtplistener.go       # RTP receiver (pion/rtp) — G.711 decode, feeds pipeline
│   ├── siprec.go           # SIPREC co-pilot: dual-leg STT, coach worker, summary
│   ├── siprec_meta.go      # RFC 7866 SIPREC metadata XML parser
│   ├── sentiment.go         # Voice sentiment: pitch, energy, speaking rate, frustration
│   ├── codec.go            # G.711 μ-law/A-law LUT transcoding + resampler
│   ├── dtmf.go             # RFC 2833/4733 DTMF parsing + Goertzel inband
│   ├── agc.go              # AGC + noise gate + comfort noise generation
│   ├── failover.go         # Circuit breakers + deterministic failover state machine
│   ├── robocall.go         # 3-layer robocall detection (blocklist, audio, keywords)
│   ├── security.go         # Voice biometrics (spectral fingerprint) + PII masking
│   ├── actions.go          # Self-service intent execution + SIP REFER transfer
│   ├── llm.go              # Multi-LLM abstraction (Claude + Gemini streaming)
│   ├── api.go              # REST API + PostgreSQL (agents, calls, documents)
│   ├── rag.go              # ChromaDB document chunking + vector RAG query
│   ├── store.go            # Redis distributed session store + local fallback
│   ├── metrics.go          # Prometheus /metrics endpoint (30+ metrics)
│   ├── scale.go            # Worker pool, rate limiter, admission controller
│   └── Dockerfile          # Multi-stage: Go 1.25 → distroless (15MB binary)
├── freeswitch/             # SIP/RTP engine + enterprise SBC profiles
├── whisper/                # Local STT container (faster-whisper)
├── ui/                     # Next.js dashboard (6 pages, shadcn/ui)
├── sdk/
│   ├── python/             # pip-installable Python SDK
│   └── typescript/         # npm TypeScript SDK
├── k8s/
│   ├── base/               # All 10 services + NetworkPolicies + secrets
│   ├── istio/              # PeerAuth, DestinationRules, AuthZ, ServiceEntries
│   ├── gateway-api/        # Gateway + HTTPRoutes (gateway, UI, Grafana)
│   └── overlays/           # local (KinD), cloud (LB), on-prem (MetalLB), air-gapped
├── docs/
│   ├── features.md         # Complete feature reference (21 features, 857 lines)
│   ├── api-reference.md    # Full API documentation (25+ endpoints)
│   ├── quick-setup.md      # 5-minute setup guide
│   ├── test-plan.md        # Comprehensive test plan (28 sections, 80+ tests)
│   └── blog.md             # Technical deep-dive blog
├── sbc/                    # Kamailio SBC simulator for local testing
├── test/                   # livecall, simcall, simcopilot, callcenter-live
├── demos/                  # VHS tape files (GIF recordings) + narrated scripts
├── docker-compose.helper.yml  # Standalone helper mode (8 services, no FreeSWITCH)
├── docker-compose.sip.yml     # Full B2BUA platform (10 services)
├── docker-compose.sbc.yml     # SBC lab overlay (adds Kamailio + softphone profile)
├── prometheus.yml          # Prometheus scrape config
└── Makefile                # helper, sbc-lab, deploy-local/cloud/on-prem, demos
```

---

## Open-Core Licensing

| Layer | Open Source (GitHub) | Enterprise License |
|-------|:--------------------:|:------------------:|
| SIP-to-WebSocket B2BUA | Yes | |
| Interactive AI agent | Yes | |
| Co-pilot agent assist | Yes | |
| Multi-LLM (Claude/Gemini) | Yes | |
| RAG knowledge base | Yes | |
| Robocall detection | Yes | |
| Standalone SIPREC helper (no FreeSWITCH) | | Yes |
| Voice sentiment (acoustic emotion) | | Yes |
| SIPREC RFC 7866 parser | | Yes |
| PII masking (PCI/HIPAA) | | Yes |
| Voice biometrics | | Yes |
| Self-service actions + transfers | | Yes |
| G.711 LUT transcoding | | Yes |
| RFC 2833 DTMF parsing | | Yes |
| Telecom AGC + noise gate | | Yes |
| Circuit breaker failover | | Yes |
| Cisco CUBE / AudioCodes profiles | | Yes |
| Redis distributed sessions | | Yes |
| Worker pool load balancing | | Yes |
| Prometheus metrics + Grafana | | Yes |
| Rate limiting + admission control | | Yes |
| Istio mesh + Gateway API | | Yes |
| K8s overlays (cloud/on-prem/air-gapped) | | Yes |
| Air-gapped deployment | | Yes |

---

## Documentation

| Document | Description |
|----------|-------------|
| [`docs/features.md`](docs/features.md) | Complete feature reference — all 21 features with source code, schemas, and pipeline details (857 lines) |
| [`docs/api-reference.md`](docs/api-reference.md) | Full API documentation — 25+ endpoints with request/response examples |
| [`docs/quick-setup.md`](docs/quick-setup.md) | 5-minute setup guide — prerequisites, deployment, testing, troubleshooting |
| [`docs/test-plan.md`](docs/test-plan.md) | Comprehensive test plan — 28 sections, 80+ test cases, load tests, E2E scenarios |
| [`docs/blog.md`](docs/blog.md) | Technical deep-dive — architecture, protocol handling, enterprise positioning |
| [`docs/sbc-configuration.md`](docs/sbc-configuration.md) | SBC integration guide — Cisco CUBE, AudioCodes, Oracle, Kamailio, Twilio + SIPREC setup |
| [`docs/softphone-setup.md`](docs/softphone-setup.md) | Local SBC lab — mobile softphone + Kamailio SBC + co-pilot testing on home LAN |
| [`sdk/python/README.md`](sdk/python/README.md) | Python SDK usage guide |
| [`sdk/typescript/README.md`](sdk/typescript/README.md) | TypeScript SDK usage guide with Next.js examples |

---

*Built for network engineers who ship infrastructure, not web developers who ship wrappers.*
