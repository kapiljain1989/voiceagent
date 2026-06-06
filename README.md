# AI Media Gateway — Enterprise Call Center Platform

An enterprise-grade, AI-powered call center platform with telecom-grade reliability. Handles inbound/outbound SIP voice calls, provides real-time agent coaching with RAG-powered knowledge retrieval, detects robocalls, executes self-service actions via natural language, performs intelligent call transfers with context, masks PII for PCI/HIPAA compliance, runs voice biometric fraud detection, and generates automated post-call summaries. Deployable on-premises, in private VPCs, or fully air-gapped with zero internet dependency.

Built with Go, Next.js, FreeSWITCH, Whisper STT, Piper TTS, Claude/Gemini on Vertex AI, PostgreSQL, and ChromaDB.

## Platform Architecture

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                        Next.js Dashboard (:3000)                             │
│  ┌──────────┐ ┌──────────┐ ┌──────────────┐ ┌──────────┐ ┌──────────┐     │
│  │ Command  │ │  Agent   │ │  Live Ops    │ │Knowledge │ │  Config  │     │
│  │ Center   │ │  Roster  │ │  Co-Pilot    │ │  Base    │ │  Panel   │     │
│  └──────────┘ └──────────┘ └──────────────┘ └──────────┘ └──────────┘     │
└───────────────────────────────┬──────────────────────────────────────────────┘
                                │
┌───────────────────────────────┼──────────────────────────────────────────────┐
│                     Go Media Gateway (:8080)                                 │
│                                                                              │
│  ┌─── Security & Compliance ─────────────────────────────────────────┐      │
│  │ Robocall Detection (3-layer)  │  Live PII Masking (PCI/HIPAA)     │      │
│  │ Voice Biometrics & Fraud      │  G.711/A-law Codec Transcoding    │      │
│  └───────────────────────────────────────────────────────────────────┘      │
│                                                                              │
│  ┌─── Call Handling ─────────────────────────────────────────────────┐      │
│  │ Interactive AI Agent          │  Co-Pilot Agent Assist (SIPREC)   │      │
│  │ VAD → Whisper → Claude → TTS │  Dual-leg STT → RAG → Coach      │      │
│  │ Self-Service Actions (CRM)    │  SIPREC Metadata Parser (RFC 7866)│      │
│  │ Intelligent SIP Transfer      │  Post-Call Summary → CRM Webhook  │      │
│  └───────────────────────────────────────────────────────────────────┘      │
│                                                                              │
│  ┌─── Reliability ───────────────────────────────────────────────────┐      │
│  │ Circuit Breakers (LLM/STT/TTS/ESL)  │  Failover State Machine    │      │
│  │ Sub-ms Failure Detection             │  Auto SIP REFER to Human  │      │
│  └───────────────────────────────────────────────────────────────────┘      │
│                                                                              │
│  REST API: /api/agents /api/calls /api/documents /api/llm /api/stats        │
│  /api/blocklist /api/robocall /api/security /api/actions /api/failover      │
└──────────┬──────────┬──────────┬──────────┬──────────┬───────────────────────┘
           │          │          │          │          │
     FreeSWITCH  Whisper STT  Piper TTS  PostgreSQL  ChromaDB
     (:5060)     (:8000)      (:5000)    (:5432)     (:8200)
```

## Core Capabilities

### Interactive AI Agent

The AI answers calls directly. Customer speaks naturally, Claude responds with synthesized voice. Supports self-service actions and intelligent escalation.

```
Customer → SIP → FreeSWITCH → mod_audio_fork → Gateway /ws
                                                    │
                              Robocall screening (3-layer)
                              PII masking (PCI/HIPAA)
                              Voice biometric check
                                                    │
                                              VAD → Whisper STT
                                                    │
                                    Claude (Vertex AI) → action parsing
                                         │                    │
                                    [speak]            [api_call / transfer]
                                         │                    │
                                  Piper TTS → WAV      CRM API / SIP REFER
                                         │
                              ESL uuid_broadcast → FreeSWITCH → caller
```

### Co-Pilot Agent Assist (SIPREC)

Silently observes live calls between customers and human agents. Provides real-time coaching suggestions grounded in the knowledge base via RAG.

```
Customer ↔ Human Agent (live SIP call)
                │
          FreeSWITCH forks audio (caller + agent legs)
                │
          Gateway /siprec endpoint
                │
      ┌─────────┴─────────┐
  callerSTT           agentSTT
  (VAD + Whisper)     (VAD + Whisper)
      └────────┬───────────┘
          transcripts (speaker-labeled)
               │
     RAG query (ChromaDB) → relevant policy docs injected
               │
         coachWorker (Claude with RAG context)
               │
      ┌────────┴────────┐
  SSE events         [on hangup]
  → Agent dashboard   summaryWorker → call summary + sentiment
  → UI live ops                    → POST webhook → CRM
```

### Smart Self-Service Actions

Natural language intent parsing with automatic backend API execution and voice confirmation.

```
Customer: "Hey, I need to reschedule my delivery for Thursday at 3 PM"
    │
    ▼
Claude parses → {"type":"api_call","intent":"reschedule",
                  "api_call":{"endpoint":"/deliveries","method":"PUT",
                  "payload":{"date":"2026-06-12","time":"15:00"}}}
    │
    ▼
Gateway executes CRM API call
    │
    ▼
AI speaks: "Done! Your delivery is now set for Thursday at 3 PM."
```

| Customer Says | Intent | Action |
|---------------|--------|--------|
| "Reschedule my delivery for Thursday" | `reschedule` | PUT /deliveries |
| "Cancel my subscription" | `cancel` | POST /subscriptions/cancel |
| "What's my account balance?" | `check_status` | GET /accounts/balance |

### Intelligent Call Transfer with Context

When the AI detects anger, complexity, or an explicit request for a human, it transfers the call via SIP with custom headers containing the full conversation context. The receiving agent's Cisco/Avaya softphone displays the summary instantly.

```
Gateway sends ESL transfer with custom SIP headers:
    X-Transfer-Summary: "Customer upset about $142 billing error. Wants refund."
    X-Transfer-Reason: angry
    X-Transfer-Department: retention
    X-Transfer-Priority: urgent
    X-Transfer-Transcript: "[customer] overcharged $142 | [agent] checking..."
```

Department routing: billing→3001, technical→3002, sales→3003, retention→3004, supervisor→3005.

### Robocall Detection (3-Layer)

| Layer | Method | Speed |
|-------|--------|-------|
| **Blocklist** | In-memory hash map lookup | < 1ms |
| **Audio Pattern** | RMS variance, silence ratio, monotone detection | ~2s |
| **Transcript Keywords** | 28 weighted phrases ("press 1", "auto warranty", "IRS") | After first STT |

Combined weighted scoring with configurable threshold (default 0.7). Auto-block option for high-confidence detections.

### Voice Biometrics & Fraud Detection

Concurrent voice fingerprinting runs alongside the STT pipeline on raw PCM audio. Extracts 32-dimensional spectral features and compares against enrolled profiles using cosine similarity.

- **Fraud detection** — match caller voice against known fraud profiles
- **Identity verification** — confirm caller matches the account holder's voiceprint
- **Enrollment API** — register new voiceprints from call audio

### Live PII Masking (PCI/HIPAA Compliance)

Real-time detection and masking of sensitive data in transcripts before they reach the LLM or call recording storage. Audio frames containing PII can be silenced before recording.

```
Input:  "My card number is 4111 1111 1111 1111 and CVV is 123"
Output: "My card number is XXXX-XXXX-XXXX-#### and [CVV REDACTED]"
```

7 detection patterns: credit cards, SSNs, CVVs, dates of birth, account numbers, and spoken variants ("my social security number is...").

### Post-Call Summary & CRM Webhook

On hangup, Claude generates a structured summary automatically POSTed to your CRM:

```json
{
  "summary": "Customer called about billing dispute. Agent offered 20% discount.",
  "action_items": ["Issue refund for $42.50", "Send confirmation email"],
  "commitments_made": ["Callback within 24 hours"],
  "sentiment": "negative"
}
```

## 4-Layer Enterprise Infrastructure

### Layer 1: Network Isolation (Security)

Network-topology agnostic. Deploys into public cloud, private VPC, on-premises data center, or fully air-gapped environment with zero internet dependency.

```bash
kubectl apply -k k8s/overlays/on-prem       # On-premises
kubectl apply -k k8s/overlays/air-gapped     # Air-gapped (local Ollama LLM)
```

Pre-configured SBC profiles for Cisco CUBE and AudioCodes Mediant at `freeswitch/config/sip_profiles/enterprise/`.

### Layer 2: SIPREC Protocol Mastery

Native RFC 7866 SIPREC metadata XML parser. Parses binary session metadata from SBC SIPREC INVITE, anchors dual RTP streams, and resolves participant/stream mappings for diarized transcription. Handles session groups, participant AORs, and stream labels.

### Layer 3: Sub-50ms Codec Transcoding

Pure Go G.711 μ-law/A-law decoder with pre-computed 256-entry lookup tables. Zero-copy, O(1) per-sample transcoding with 8kHz → 16kHz linear interpolation resampling. No FFmpeg, no cloud transcoding service. Includes G.711 encoder for return-path audio and SNR estimation for quality monitoring.

### Layer 4: Deterministic Failover (99.999% Target)

Circuit breaker per service (LLM, STT, TTS, ESL) with atomic state transitions and sub-millisecond failure detection. Background health monitoring with automatic recovery probing.

```
LLM drops    → "One moment please..." → auto-reconnect → resume
STT fails    → buffer audio → retry on recovery
TTS fails    → static tone via ESL fallback
All services → SIP REFER to human queue with X-Failover headers
```

Real-time status: `GET /api/failover/status`

## Services (7 containers)

| Service | Image | Port | Role |
|---------|-------|------|------|
| `gateway` | `voiceagent/gateway` | 8080 | Go media gateway with all AI, security, and telecom features |
| `freeswitch` | `drachtio/drachtio-freeswitch-mrf` | 5070 | SIP/RTP engine, mod_audio_fork |
| `whisper` | `fedirz/faster-whisper-server` | 8000 | Local STT (faster-whisper-base.en) |
| `piper` | `artibex/piper-http` | 5000 | Local TTS (en_US-ryan-high) |
| `postgres` | `postgres:16-alpine` | 5432 | Agents, calls, documents, blocklist, voice prints |
| `chromadb` | `chromadb/chroma` | 8200 | RAG vector store |
| `ui` | `voiceagent/ui` | 3000 | Next.js call center dashboard |

## Prerequisites

- Docker Desktop
- Go 1.22+
- Node.js 22+
- gcloud CLI with Application Default Credentials
- PortAudio for live mic calls (`brew install portaudio`)
- baresip for SIP calls (`brew install baresip`)

### GCP Requirements

Claude and Gemini on Vertex AI require cloud access. STT, TTS, RAG, robocall detection, PII masking, voice biometrics, and codec transcoding run entirely locally.

```bash
export ANTHROPIC_VERTEX_PROJECT_ID="your-gcp-project-id"
export CLOUD_ML_REGION="us-east5"
```

## Quick Start

```bash
export EXT_IP=$(ipconfig getifaddr en0)
docker compose -f docker-compose.sip.yml up -d
open http://localhost:3000
```

## Dashboard Pages

| Page | URL | Description |
|------|-----|-------------|
| Command Center | `/` | Stats cards, active calls, recent completions |
| Agent Roster | `/agents` | Agent CRUD with expertise badges and status |
| Call History | `/calls` | Paginated call log with sentiment, mode, and robocall badges |
| Live Operations | `/calls/live` | Real-time transcript + co-pilot suggestions via SSE |
| Knowledge Base | `/documents` | Document upload, RAG indexing, vector search testing |
| Configuration | `/settings` | LLM models, system prompts, SBC trunk, blocklist |

## Testing

### Live Call Center Demo

```bash
cd test && ./callcenter-live.sh
# Open http://localhost:3000/calls/live → paste Call ID → CONNECT
# Speak: "I had a burst pipe and water damaged my floor"
# → Co-pilot: "Section 4.2.1 covers burst pipe damage. $500 deductible."
```

### Interactive AI Agent

```bash
cd test && ./livecall ws://localhost:8080/ws              # WebSocket (mic/speaker)
cd test && SIP_PORT=5070 ./test-sip-call.sh               # SIP via baresip
```

### Robocall Detection

```bash
curl -X POST http://localhost:8080/api/robocall/test \
  -d '{"text":"We have been trying to reach you about your auto warranty. Press 1."}'
# → {"score":1.0, "category":"robocall", "keywords":["press 1","auto warranty",...]}
```

### PII Masking

```bash
curl -X POST http://localhost:8080/api/security/pii/test \
  -d '{"text":"My social security number is 123-45-6789"}'
# → {"masked":"My social security number is XXX-XX-####", "pii_found":true}
```

### Voice Biometrics

```bash
curl http://localhost:8080/api/security/voiceprints                    # List prints
curl -X POST http://localhost:8080/api/security/voiceprints \
  -d '{"label":"fraud_profile_001","type":"fraud"}'                    # Enroll
```

### Failover Status

```bash
curl http://localhost:8080/api/failover/status
# → {"llm":{"state":"closed","failures":0}, "stt":{...}, "tts":{...}, "esl":{...}}
```

### Self-Service Actions

```bash
curl -X POST http://localhost:8080/api/actions/test \
  -d '{"text":"{\"type\":\"api_call\",\"intent\":\"reschedule\",\"text\":\"Done.\",\"api_call\":{\"endpoint\":\"/deliveries\",\"method\":\"PUT\",\"payload\":{\"date\":\"2026-06-12\"}}}"}'

curl http://localhost:8080/api/actions/webhooks                        # List webhooks
curl -X POST http://localhost:8080/api/actions/webhooks \
  -d '{"reschedule":"https://crm.example.com/api"}'                    # Configure
```

### Simulated Tests (No Microphone)

```bash
cd test && go run simcall.go                    # Interactive pipeline
cd test && ./simcopilot localhost:8080           # Co-pilot simulation
```

## API Reference

### Voice Endpoints

| Endpoint | Protocol | Description |
|----------|----------|-------------|
| `/ws` | WebSocket | Interactive AI agent — PCM audio in, TTS audio + events out |
| `/siprec` | WebSocket | Co-pilot — `?role=caller\|agent&call_id=xxx` |
| `/siprec/events` | SSE | Agent dashboard — streams transcript/suggestion/summary |
| `/call` | POST | Outbound call origination via ESL |

### REST API

| Endpoint | Description |
|----------|-------------|
| `/api/agents` | Agent CRUD with expertise and status |
| `/api/calls` | Call history with transcripts and summaries |
| `/api/calls/active` | Active session counts (interactive + copilot) |
| `/api/documents` | Document upload and RAG indexing |
| `/api/documents/search` | RAG vector query |
| `/api/llm/configs` | LLM model management (Claude/Gemini) |
| `/api/llm/test` | Test a model with sample prompt |
| `/api/blocklist` | Robocall blocklist CRUD |
| `/api/robocall/stats` | Robocall detection metrics |
| `/api/robocall/test` | Test robocall classification |
| `/api/security/voiceprints` | Voice biometric enrollment and listing |
| `/api/security/pii/test` | Test PII masking |
| `/api/security/pii/config` | PII masking configuration |
| `/api/actions/webhooks` | Self-service action webhook URLs |
| `/api/actions/test` | Test action parsing |
| `/api/failover/status` | Circuit breaker health (all services) |
| `/api/stats` | Dashboard stats |
| `/healthz` | Health check |

### SSE Event Types

```json
{"type":"transcript","speaker":"customer","text":"I need help with my claim"}
{"type":"suggestion","suggestion":"Policy 4.2.1 covers...","category":"answer","confidence":0.95}
{"type":"robocall","text":"score=80% keywords=[press 1, auto warranty]"}
{"type":"pii_masked","text":"Detected 2 PII items: credit_card, cvv"}
{"type":"action","intent":"reschedule","status":"success"}
{"type":"transfer","reason":"angry","department":"retention","priority":"urgent"}
{"type":"summary","summary":"Customer called about...","action_items":[...],"sentiment":"neutral"}
```

## SBC Connectivity

```bash
SBC_ADDRESS=sbc.example.com SBC_USERNAME=trunk1 SBC_PASSWORD=s3cret make sbc-config
```

Enterprise SBC profiles at `freeswitch/config/sip_profiles/enterprise/`:
- **Cisco CUBE** — G.711/G.722/G.729 codecs, TLS support, session timers
- **AudioCodes Mediant** — Registration mode, NAT traversal, forced rport

### Dialplan Routing

| Destination | Mode |
|-------------|------|
| `1xxx` | Interactive AI Agent — Claude answers directly |
| `2xxx` | Co-Pilot Agent Assist — passive observation with suggestions |

## RAG Knowledge Base

```bash
curl -X POST http://localhost:8080/api/documents \
  -d '{"name":"Policy v4.2","category":"policy","content":"Water damage from burst pipes is covered..."}'

curl -X POST http://localhost:8080/api/documents/search \
  -d '{"query":"water damage coverage","top_k":3}'
```

During co-pilot calls, the system queries ChromaDB with each customer utterance, retrieves matching document chunks, and injects them as context into Claude's coaching prompt.

## Multi-LLM Support

Unified `LLMClient` interface supporting both Claude and Gemini on Vertex AI with streaming and non-streaming modes.

```bash
curl -X POST http://localhost:8080/api/llm/test \
  -d '{"provider":"anthropic-vertex","model":"claude-3-5-haiku@20241022","prompt":"Hello"}'

curl -X POST http://localhost:8080/api/llm/test \
  -d '{"provider":"gemini-vertex","model":"gemini-2.0-flash","prompt":"Hello"}'
```

## CRM Webhook

```bash
export CRM_WEBHOOK_URL=https://hooks.salesforce.com/services/...
export CRM_WEBHOOK_TOKEN=Bearer_xxx
```

On call end, POSTs a structured JSON with conversation ID, duration, full transcript, summary, action items, commitments made, sentiment, and suggestions given.

## Project Structure

```
voiceagent/
├── gateway/
│   ├── main.go             # Media gateway, WebSocket, VAD, ESL, interactive pipeline
│   ├── siprec.go           # Co-pilot: dual-leg STT, coach worker, summary, webhook
│   ├── siprec_meta.go      # RFC 7866 SIPREC metadata XML parser
│   ├── robocall.go         # 3-layer robocall detection (blocklist, audio, keywords)
│   ├── security.go         # Voice biometrics + PII masking (PCI/HIPAA)
│   ├── actions.go          # Self-service actions + intelligent call transfer
│   ├── failover.go         # Circuit breakers + deterministic failover state machine
│   ├── codec.go            # G.711 μ-law/A-law transcoding with lookup tables
│   ├── llm.go              # Multi-LLM abstraction (Claude + Gemini on Vertex AI)
│   ├── api.go              # REST API (agents, calls, documents, stats, LLM config)
│   ├── rag.go              # ChromaDB document chunking + RAG query
│   ├── go.mod / go.sum
│   └── Dockerfile
├── freeswitch/
│   ├── Dockerfile
│   ├── entrypoint.sh
│   └── config/
│       ├── dialplan/               # 1xxx=AI agent, 2xxx=co-pilot, outbound routing
│       ├── sip_profiles/           # Standard + enterprise (Cisco CUBE, AudioCodes)
│       └── autoload_configs/       # mod_audio_fork, sofia, ACL, ESL
├── whisper/
│   └── Dockerfile
├── ui/
│   ├── src/app/                    # Next.js pages (dashboard, agents, calls, docs, settings)
│   ├── src/components/             # Sidebar, cards, transcript viewer
│   ├── prisma/schema.prisma        # PostgreSQL schema
│   └── Dockerfile
├── test/
│   ├── livecall.go                 # Real mic/speaker voice call
│   ├── callcenter-live.sh          # Full call center demo with mic
│   ├── simcall.go / simcopilot.go  # Automated pipeline tests
│   └── test-sip-call.sh            # SIP call via baresip
├── k8s/
│   ├── base/                       # Kustomize base manifests
│   └── overlays/
│       ├── on-prem/                # Enterprise on-premises deployment
│       ├── private-vpc/            # Private VPC deployment
│       └── air-gapped/            # Zero-internet deployment (local Ollama LLM)
├── docker-compose.sip.yml          # Full platform (7 services)
├── kind-config.yaml
├── Makefile
└── README.md
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `LISTEN_ADDR` | `:8080` | Gateway listen address |
| `STT_URL` | `http://whisper:8000/v1/audio/transcriptions` | Whisper STT endpoint |
| `TTS_URL` | `http://piper:5000` | Piper TTS endpoint |
| `GCP_PROJECT_ID` | from `ANTHROPIC_VERTEX_PROJECT_ID` | Vertex AI project |
| `GCP_REGION` | from `CLOUD_ML_REGION` or `us-east5` | Vertex AI region |
| `CLAUDE_MODEL` | `claude-3-5-haiku@20241022` | Default LLM model |
| `DATABASE_URL` | | PostgreSQL connection |
| `CHROMA_URL` | | ChromaDB endpoint for RAG |
| `CRM_WEBHOOK_URL` | | POST call summaries on hangup |
| `CRM_WEBHOOK_TOKEN` | | Bearer token for webhook auth |
| `ACTION_RESCHEDULE_URL` | | Self-service reschedule webhook |
| `ACTION_CANCEL_URL` | | Self-service cancel webhook |
| `ACTION_STATUS_URL` | | Self-service status check webhook |

TTS voice: `en_US-ryan-high` (Piper). Full list at [rhasspy/piper-voices](https://huggingface.co/rhasspy/piper-voices).

## Pipeline Internals

### Interactive Mode (5 goroutines)

```
readFromFS ──pcmIn──▶ sttPipeline ──transcripts──▶ claudeWorker ──sentences──▶ ttsWorker ──pcmOut──▶ writeToFS
                      (VAD+Whisper)  + PII mask     (action parse)             (Piper)               (ESL)
                      + robocall L3  + biometrics   + self-service
                                                    + transfer
```

### Co-Pilot Mode (5 goroutines)

```
readCaller ──pcmCaller──▶ callerSTT ──┐
                                      ├──transcripts──▶ coachWorker ──▶ SSE → agent dashboard
readAgent  ──pcmAgent───▶ agentSTT  ──┘  + PII mask     (RAG+Claude)   → post-call summary → CRM
                                         + robocall L3
```

### Failover Chain

```
LLM circuit open     → "One moment please..." → reconnect → resume
STT circuit open     → buffer audio → retry on half-open probe
TTS circuit open     → static tone via ESL
All circuits open    → SIP REFER to human queue (ext 3000)
                       with X-Failover-Summary headers
```

### Latency Profile

| Stage | Duration |
|-------|----------|
| Blocklist check | < 1ms |
| G.711 → L16 transcoding | < 1ms |
| VAD silence detection | 500ms |
| Whisper transcription | 200-400ms |
| PII masking | < 1ms |
| Robocall keyword check | < 1ms |
| RAG query (ChromaDB) | 50-100ms |
| Claude/Gemini response | 1-2s |
| Piper TTS synthesis | 200-400ms |
| Circuit breaker failover | < 1ms |
| **Co-pilot (speech → suggestion)** | **~2s** |
| **Interactive (speech → voice)** | **~3s** |

## Deployment Modes

| Mode | Network | LLM | Command |
|------|---------|-----|---------|
| Docker Compose | Local/dev | Vertex AI | `docker compose up` |
| KinD | Kubernetes local | Vertex AI | `make all` |
| On-Premises | Enterprise DC | Vertex AI | `kubectl apply -k k8s/overlays/on-prem` |
| Air-Gapped | No internet | Local Ollama | `kubectl apply -k k8s/overlays/air-gapped` |

## Open-Core Model

| Capability | Open Source | Enterprise |
|------------|:----------:|:----------:|
| SIP-to-WebSocket gateway | Yes | |
| Interactive AI agent | Yes | |
| Co-pilot agent assist | Yes | |
| Multi-LLM (Claude/Gemini) | Yes | |
| RAG knowledge base | Yes | |
| Robocall detection | Yes | |
| SIPREC RFC 7866 parser | | Yes |
| PII masking (PCI/HIPAA) | | Yes |
| Voice biometrics & fraud | | Yes |
| Self-service actions | | Yes |
| Intelligent call transfer | | Yes |
| G.711 codec transcoding | | Yes |
| Failover state machine | | Yes |
| Cisco CUBE / AudioCodes profiles | | Yes |
| Air-gapped deployment | | Yes |

## Observability

```bash
docker compose -f docker-compose.sip.yml logs -f gateway
docker compose -f docker-compose.sip.yml logs -f freeswitch
docker compose -f docker-compose.sip.yml logs -f whisper
```

## Cleanup

```bash
docker compose -f docker-compose.sip.yml down -v
```
