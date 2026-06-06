# VoiceAgent: Telecom-Native AI Media & Signaling Gateway

An open-core, high-performance **Back-to-Back User Agent (B2BUA)** and media proxy written in Go. VoiceAgent bridges legacy enterprise Session Border Controllers (SBCs) directly to multi-modal real-time AI endpoints with sub-50ms media processing latency, native SIPREC session forking, and deterministic SIP failover protection.

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-7_services-2496ED?logo=docker)](docker-compose.sip.yml)

---

### Core Architecture Capabilities

- **SIP Protocol Alignment:** Full support for standard SIP signaling (RFC 3261), native `SIPREC` session forking (RFC 7866), context-enriched call transfers (`SIP REFER`), and `ESL` channel control.
- **Low-Latency Media Transcoding:** In-memory streaming transcoding of G.711 µ-law/A-law (8kHz, 8-bit) directly into raw linear PCM via pre-computed 256-entry lookup tables — sub-1ms per 20ms frame, zero external dependencies.
- **Network-Level Signal Interception:** Native **RFC 2833 / RFC 4733** RTP event packet parsing for hardware DTMF capture. Goertzel algorithm fallback for inband tone detection on legacy PBX systems.
- **Telephony-Grade Audio Processing:** Automatic Gain Control (AGC), noise gate, and Comfort Noise Generation (CNG) — purpose-built for degraded 8kHz carrier lines, not clean WebRTC microphone input.
- **Deterministic Failover:** Circuit breaker per service with atomic state transitions (< 1ms detection). Graceful degradation chain: hold audio → auto-reconnect → SIP REFER to human queue.

---

## Network Deployment Topology

```
                            [ Public Carrier / PSTN ]
                                      │
                              (SIP Trunk / RTP G.711)
                                      │
                          [ Enterprise SBC ]
                          (Cisco CUBE, AudioCodes, Oracle)
                                      │
                    ┌─────────────────┼─────────────────┐
                    │                                     │
          (Direct SIP Trunk)                    (SIPREC Fork / RFC 7866)
                    │                                     │
                    ▼                                     ▼
        ┌───────────────────────┐              ┌───────────────────────┐
        │   VoiceAgent Gateway  │              │   VoiceAgent Gateway  │
        │   Mode: Interactive   │              │   Mode: Co-Pilot      │
        │                       │              │                       │
        │   ┌─ VAD ──────────┐  │              │   ┌─ callerSTT ────┐ │
        │   │ Whisper STT    │  │              │   │ agentSTT       │ │
        │   │ PII Masking    │  │              │   │ RAG (ChromaDB) │ │
        │   │ Claude/Gemini  │  │              │   │ Claude Coach   │ │
        │   │ Piper TTS      │  │              │   │ SSE → Dashboard│ │
        │   └────────────────┘  │              │   └────────────────┘ │
        │                       │              │                       │
        │   SIP REFER (failover)│              │   POST → CRM Webhook │
        └───────────┬───────────┘              └───────────┬───────────┘
                    │                                       │
                    ▼                                       ▼
        [ Human Agent Queue ]                  [ Agent Desktop / UI ]
        (Cisco/Avaya softphone                 (Next.js :3000)
         receives X-Transfer headers)
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

# Start all 7 services
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

### Kubernetes Deployment (KinD / On-Prem / Air-Gapped)

```bash
# Local KinD cluster
make all

# Enterprise on-premises (private SIP trunk to Cisco CUBE)
kubectl apply -k k8s/overlays/on-prem

# Air-gapped (zero internet — local Ollama LLM, local Whisper, local Piper)
kubectl apply -k k8s/overlays/air-gapped
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
| **PII Masking** | 7 patterns (credit card, SSN, CVV, DOB, account) — masks before LLM/recording |
| **Voice Biometrics** | 32-dim spectral fingerprint — fraud detection + identity verification |
| **Robocall Detection** | 3-layer: blocklist (< 1ms) + audio pattern + 28 keyword phrases |

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

### Enterprise SBC Profiles

Pre-configured profiles at `freeswitch/config/sip_profiles/enterprise/`:

| SBC | Profile | Codecs | Features |
|-----|---------|--------|----------|
| **Cisco CUBE** | `cisco-cube.xml` | G.711/G.722/G.729 | TLS, session timers, OPTIONS keepalive |
| **AudioCodes Mediant** | `audiocodes.xml` | G.711/G.722/G.729 | Registration mode, NAT traversal, rport |

```bash
# Configure trunk
SBC_ADDRESS=cube.internal make sbc-config

# Twilio SIP Trunk
SBC_ADDRESS=trunk.pstn.twilio.com SBC_REGISTER=true SBC_USERNAME=sid SBC_PASSWORD=token make sbc-config
```

### Dialplan Routing

| Destination | Mode | Behavior |
|-------------|------|----------|
| `1xxx` | Interactive | AI agent answers, self-service + transfer capable |
| `2xxx` | Co-Pilot | Passive observation, RAG coaching, post-call summary |
| `3xxx` | Human Queue | Direct-to-agent (failover target) |

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
| **Infrastructure** | `/api/failover/status`, `/api/dtmf/test`, `/api/stats`, `/healthz` |

---

## Project Structure (14 Go source files)

```
voiceagent/
├── gateway/
│   ├── main.go             # B2BUA core: WebSocket, VAD, ESL, session orchestration
│   ├── siprec.go           # SIPREC co-pilot: dual-leg STT, coach worker, summary
│   ├── siprec_meta.go      # RFC 7866 SIPREC metadata XML parser
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
│   └── Dockerfile          # Multi-stage: Go 1.25 → distroless (15MB binary)
├── freeswitch/             # SIP/RTP engine + enterprise SBC profiles
├── whisper/                # Local STT container (faster-whisper)
├── ui/                     # Next.js dashboard (6 pages, shadcn/ui)
├── sdk/
│   ├── python/             # pip-installable Python SDK
│   └── typescript/         # npm TypeScript SDK
├── k8s/
│   ├── base/               # Kustomize base manifests
│   └── overlays/           # on-prem, private-vpc, air-gapped
├── docs/
│   ├── features.md         # Complete feature reference (19 features, 857 lines)
│   ├── api-reference.md    # Full API documentation (25+ endpoints)
│   ├── quick-setup.md      # 5-minute setup guide
│   └── blog.md             # Technical deep-dive blog
├── test/                   # livecall, simcall, simcopilot, callcenter-live
├── docker-compose.sip.yml  # Full platform (7 services)
└── Makefile                # kind-up, build, load, deploy, sbc-config
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
| SIPREC RFC 7866 parser | | Yes |
| PII masking (PCI/HIPAA) | | Yes |
| Voice biometrics | | Yes |
| Self-service actions + transfers | | Yes |
| G.711 LUT transcoding | | Yes |
| RFC 2833 DTMF parsing | | Yes |
| Telecom AGC + noise gate | | Yes |
| Circuit breaker failover | | Yes |
| Cisco CUBE / AudioCodes profiles | | Yes |
| Air-gapped deployment | | Yes |

---

## Documentation

| Document | Description |
|----------|-------------|
| [`docs/features.md`](docs/features.md) | Complete feature reference — all 19 features with source code, schemas, and pipeline details (857 lines) |
| [`docs/api-reference.md`](docs/api-reference.md) | Full API documentation — 25+ endpoints with request/response examples |
| [`docs/quick-setup.md`](docs/quick-setup.md) | 5-minute setup guide — prerequisites, deployment, testing, troubleshooting |
| [`docs/blog.md`](docs/blog.md) | Technical deep-dive — architecture, protocol handling, enterprise positioning |
| [`sdk/python/README.md`](sdk/python/README.md) | Python SDK usage guide |
| [`sdk/typescript/README.md`](sdk/typescript/README.md) | TypeScript SDK usage guide with Next.js examples |

---

*Built for network engineers who ship infrastructure, not web developers who ship wrappers.*
