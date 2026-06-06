# AI Media Gateway — Call Center Platform

An AI-powered call center platform that handles inbound/outbound SIP voice calls, provides real-time agent assist with RAG-powered knowledge retrieval, detects and blocks robocalls, and generates automated post-call summaries with CRM integration. Built with Go, Next.js, FreeSWITCH, Whisper STT, Piper TTS, Claude/Gemini on Vertex AI, PostgreSQL, and ChromaDB.

## Platform Overview

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
│  Robocall Detection (3-layer)                                                │
│  ┌─────────────────────────────────────────────────────────────┐             │
│  │ Layer 1: Blocklist (< 1ms)  →  Layer 2: Audio Pattern (2s) │             │
│  │ → Layer 3: Transcript Keywords (after STT)                  │             │
│  └─────────────────────────────────────────────────────────────┘             │
│                                                                              │
│  Mode 1: Interactive AI Agent        Mode 2: Co-Pilot Agent Assist           │
│  ┌─────────────────────────┐         ┌─────────────────────────┐             │
│  │ /ws — WebSocket audio   │         │ /siprec — Dual-leg audio │            │
│  │ VAD → Whisper → Claude  │         │ callerSTT + agentSTT    │             │
│  │ → Piper TTS → playback  │         │ → RAG → coachWorker     │             │
│  └─────────────────────────┘         │ → SSE → Post-call → CRM │             │
│                                      └─────────────────────────┘             │
│  REST API                                                                    │
│  /api/agents  /api/calls  /api/documents  /api/llm  /api/stats              │
│  /api/blocklist  /api/robocall/stats  /call (ESL)  /healthz                 │
└──────────┬──────────┬──────────┬──────────┬──────────┬───────────────────────┘
           │          │          │          │          │
     FreeSWITCH  Whisper STT  Piper TTS  PostgreSQL  ChromaDB
     (:5060)     (:8000)      (:5000)    (:5432)     (:8200)
```

## Three Operating Modes

### Mode 1: Interactive AI Agent

The AI answers calls directly. Customer speaks, Claude responds with synthesized voice.

```
Customer → SIP → FreeSWITCH → mod_audio_fork → Gateway /ws
                                                    │
                              Robocall screening (3-layer)
                                                    │
                                              VAD → Whisper STT
                                                    │
                                              Claude (Vertex AI)
                                                    │
                                              Piper TTS → WAV
                                                    │
                              ESL uuid_broadcast → FreeSWITCH → Customer hears Claude
```

### Mode 2: Co-Pilot Agent Assist (SIPREC)

The AI silently observes a live call between a customer and human agent, providing real-time coaching suggestions grounded in the knowledge base.

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
  → UI live transcript              → POST webhook → CRM
```

### Smart Self-Service Actions

When the customer makes an actionable request through natural speech, the LLM parses the intent, the gateway executes a backend API call, and the AI confirms the action via voice.

```
Customer: "Hey, I need to reschedule my delivery for Thursday at 3 PM"
    │
    ▼
Claude parses intent → {"type":"api_call","intent":"reschedule","api_call":{"endpoint":"/deliveries","method":"PUT","payload":{"date":"2026-06-12","time":"15:00"}}}
    │
    ▼
Gateway executes API call to CRM/backend
    │
    ▼
AI speaks: "I've rescheduled your delivery to Thursday at 3 PM. Is there anything else?"
```

Supported action types:
- **api_call** — reschedule, cancel, check status, update info → calls configured CRM webhook
- **transfer** — escalation to human agent with full context (see below)
- **speak** — normal conversation (default)

### Intelligent Call Transfer with Context

When the AI detects anger, complexity, or an explicit request for a human, it transfers the call via SIP with custom headers containing the full conversation context.

```
Customer: "This is ridiculous! I've been overcharged $142 for three months! Let me speak to a manager!"
    │
    ▼
Claude detects: anger + billing dispute + escalation request
    │
    ▼
AI speaks: "I understand this is frustrating. Let me connect you with a billing specialist."
    │
    ▼
Gateway sends ESL transfer with custom SIP headers:
    X-Transfer-Summary: "Customer upset about $142 billing error over 3 months. Wants refund."
    X-Transfer-Reason: angry
    X-Transfer-Department: retention
    X-Transfer-Priority: urgent
    X-Transfer-Transcript: "[user] overcharged $142 | [assistant] checking account..."
    │
    ▼
Human agent's Cisco/Avaya softphone displays the summary instantly.
No more "Please repeat your story."
```

### Mode 3: Robocall Detection

Three-layer spam filtering runs on every inbound call before it reaches the agent or AI pipeline.

| Layer | Method | Speed | Accuracy |
|-------|--------|-------|----------|
| **Layer 1** | Blocklist lookup | < 1ms | 100% (known numbers) |
| **Layer 2** | Audio pattern analysis | ~2s | Detects monotone/pre-recorded audio |
| **Layer 3** | Transcript keyword matching | After first STT | 28 robocall phrase patterns |

**Detection signals:**
- Blocklist hit (number previously flagged)
- Low RMS variance (monotone/pre-recorded voice)
- High silence ratio (dead air padding)
- Keyword matches: "press 1", "auto warranty", "IRS", "your Amazon account", etc.
- Combined weighted score with configurable threshold (default 0.7)

## Services (7 containers)

| Service | Image | Port | Role |
|---------|-------|------|------|
| `gateway` | `voiceagent/gateway` | 8080 | Go media gateway, REST API, ESL client, RAG, robocall detection |
| `freeswitch` | `drachtio/drachtio-freeswitch-mrf` | 5070 | SIP/RTP engine, mod_audio_fork |
| `whisper` | `fedirz/faster-whisper-server` | 8000 | Local STT (faster-whisper-base.en) |
| `piper` | `artibex/piper-http` | 5000 | Local TTS (en_US-ryan-high, 16kHz) |
| `postgres` | `postgres:16-alpine` | 5432 | Agents, calls, documents, blocklist, LLM configs |
| `chromadb` | `chromadb/chroma` | 8200 | RAG vector store for document retrieval |
| `ui` | `voiceagent/ui` | 3000 | Next.js call center dashboard |

## Prerequisites

- Docker Desktop
- Go 1.22+
- Node.js 22+
- gcloud CLI with Application Default Credentials
- PortAudio for live mic calls (`brew install portaudio`)
- baresip for SIP calls (`brew install baresip`)

### GCP Requirements

Claude and Gemini on Vertex AI require cloud access. STT, TTS, RAG, and robocall detection run entirely locally.

```bash
export ANTHROPIC_VERTEX_PROJECT_ID="your-gcp-project-id"
export CLOUD_ML_REGION="us-east5"
```

## Quick Start

```bash
# Set your LAN IP for SIP RTP addressing
export EXT_IP=$(ipconfig getifaddr en0)

# Start all 7 services
docker compose -f docker-compose.sip.yml up -d

# Open the dashboard
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
| Configuration | `/settings` | LLM models, system prompts, SBC trunk, blocklist management |

## Testing

### Live Call Center Demo (Co-Pilot Mode)

Speak as the customer while the co-pilot provides real-time suggestions on screen.

```bash
# 1. Index knowledge base documents
curl -X POST http://localhost:8080/api/documents \
  -H 'Content-Type: application/json' \
  -d '{"name":"Insurance Policy","category":"policy","content":"Section 4.2.1: Water damage from burst pipes is covered. Deductible is $500. Claims within 30 days."}'

# 2. Open the Live Ops dashboard
open http://localhost:3000/calls/live

# 3. Start a live call with your microphone
cd test && ./callcenter-live.sh

# 4. Copy the Call ID from terminal, paste in the UI, click CONNECT

# 5. Speak: "I had a burst pipe and water damaged my floor"
#    → Co-pilot: "Section 4.2.1 covers burst pipe damage. $500 deductible."
```

### Interactive AI Agent (Voice Call)

```bash
# WebSocket mode (mic/speaker, no SIP)
cd test && ./livecall ws://localhost:8080/ws

# SIP mode (via baresip softphone)
cd test && SIP_PORT=5070 ./test-sip-call.sh
# Type: /dial 1000
```

### Robocall Detection Test

```bash
# Add a number to the blocklist
curl -X POST http://localhost:8080/api/blocklist \
  -d '{"number":"+15551234567","reason":"known_robocaller"}'

# Test keyword detection
curl -X POST http://localhost:8080/api/robocall/test \
  -d '{"text":"We have been trying to reach you about your auto warranty. Press 1 to speak to a representative."}'
# → {"score":1.0, "category":"robocall", "keywords":["press 1","auto warranty","we have been trying to reach you"]}

# Test clean speech
curl -X POST http://localhost:8080/api/robocall/test \
  -d '{"text":"Hi, I had a burst pipe and my floor is damaged. Can you help me?"}'
# → {"score":0.0, "category":"human"}

# View robocall stats
curl http://localhost:8080/api/robocall/stats

# List blocklist
curl http://localhost:8080/api/blocklist
```

### Simulated Pipeline Tests (No Microphone)

```bash
cd test && go run simcall.go        # Interactive pipeline test
cd test && ./simcopilot localhost:8080  # Co-pilot simulation
```

## API Reference

### Voice Endpoints

| Endpoint | Protocol | Description |
|----------|----------|-------------|
| `/ws` | WebSocket | Interactive AI agent — PCM audio in, TTS audio + events out |
| `/siprec` | WebSocket | Co-pilot — `?role=caller\|agent&call_id=xxx` |
| `/siprec/events` | SSE | Agent dashboard — `?call_id=xxx`, streams transcript/suggestion/summary |
| `/call` | POST | Originate outbound call via ESL |

### REST API

| Endpoint | Methods | Description |
|----------|---------|-------------|
| `/api/agents` | GET, POST | Agent management with expertise and status |
| `/api/calls` | GET | Call history with transcript, summary, sentiment |
| `/api/calls/active` | GET | Currently active sessions |
| `/api/documents` | GET, POST | Document upload and RAG indexing |
| `/api/documents/search` | POST | RAG vector query |
| `/api/llm/configs` | GET, POST | LLM model configurations |
| `/api/llm/test` | POST | Test a model with sample prompt |
| `/api/blocklist` | GET, POST, DELETE | Robocall blocklist management |
| `/api/robocall/stats` | GET | Robocall detection metrics |
| `/api/robocall/test` | POST | Test robocall classification |
| `/api/actions/test` | POST | Parse action from text |
| `/api/actions/webhooks` | GET, POST | Manage self-service action webhook URLs |
| `/api/stats` | GET | Dashboard stats |
| `/healthz` | GET | Health check |

### SSE Event Types (`/siprec/events`)

```json
{"type":"transcript","speaker":"customer","text":"I need help with my claim"}
{"type":"suggestion","suggestion":"Policy 4.2.1 covers this...","category":"answer","confidence":0.95}
{"type":"robocall","text":"score=80% keywords=[press 1, auto warranty]"}
{"type":"summary","summary":"Customer called about...","action_items":[...],"sentiment":"neutral"}
```

## SBC Connectivity

FreeSWITCH supports trunk peering with any SBC for PSTN integration.

```bash
# Configure SBC trunk
SBC_ADDRESS=sbc.example.com SBC_USERNAME=trunk1 SBC_PASSWORD=s3cret make sbc-config

# Twilio SIP Trunk
SBC_ADDRESS=your-trunk.pstn.twilio.com SBC_REGISTER=true SBC_USERNAME=sid SBC_PASSWORD=token make sbc-config

# Originate outbound call
curl -X POST http://localhost:8080/call -d '{"to":"+15551234567"}'
```

### Dialplan Routing

| Destination | Mode | Behavior |
|-------------|------|----------|
| `1xxx` | Interactive AI Agent | Claude answers the call directly |
| `2xxx` | Co-Pilot Agent Assist | Passive observation with suggestions |

## RAG Knowledge Base

Upload documents to power the co-pilot with real policy/product knowledge.

```bash
# Index a document
curl -X POST http://localhost:8080/api/documents \
  -H 'Content-Type: application/json' \
  -d '{"name":"Returns Policy","category":"policy","content":"Returns accepted within 30 days..."}'

# Search
curl -X POST http://localhost:8080/api/documents/search \
  -d '{"query":"how long to return electronics","top_k":3}'
```

When a customer asks a question during a co-pilot call, the system automatically queries ChromaDB, retrieves matching document chunks, injects them as context into Claude's system prompt, and generates suggestions grounded in actual policy documents.

## Multi-LLM Support

Both Claude and Gemini on Vertex AI are supported with a unified `LLMClient` interface.

```bash
# Test Claude
curl -X POST http://localhost:8080/api/llm/test \
  -d '{"provider":"anthropic-vertex","model":"claude-3-5-haiku@20241022","prompt":"Hello"}'

# Test Gemini
curl -X POST http://localhost:8080/api/llm/test \
  -d '{"provider":"gemini-vertex","model":"gemini-2.0-flash","prompt":"Hello"}'
```

## CRM Webhook

On call end, the gateway POSTs a structured summary to your CRM.

```bash
export CRM_WEBHOOK_URL=https://hooks.salesforce.com/services/...
export CRM_WEBHOOK_TOKEN=Bearer_xxx
docker compose -f docker-compose.sip.yml up -d
```

Payload includes conversation ID, duration, full transcript, summary, action items, commitments made, sentiment analysis, and suggestions given during the call.

## Project Structure

```
voiceagent/
├── gateway/
│   ├── main.go             # Media gateway, WebSocket, VAD, ESL, interactive pipeline
│   ├── siprec.go           # Co-pilot: dual-leg STT, coach worker, summary, webhook
│   ├── robocall.go         # 3-layer robocall detection: blocklist, audio, keywords
│   ├── actions.go          # Self-service actions + intelligent call transfer with SIP headers
│   ├── llm.go              # Multi-LLM abstraction (Claude + Gemini on Vertex AI)
│   ├── api.go              # REST API: agents, calls, documents, stats, LLM config
│   ├── rag.go              # ChromaDB: document chunking, vector storage, RAG query
│   ├── go.mod / go.sum
│   └── Dockerfile
├── freeswitch/
│   ├── Dockerfile
│   ├── entrypoint.sh
│   └── config/
│       ├── dialplan/           # public.xml (1xxx=AI, 2xxx=copilot), outbound.xml
│       ├── sip_profiles/       # SBC trunk profile with gateway
│       └── autoload_configs/   # mod_audio_fork, sofia, ACL, ESL
├── whisper/
│   └── Dockerfile
├── ui/
│   ├── src/app/
│   │   ├── page.tsx            # Command Center dashboard
│   │   ├── agents/page.tsx     # Agent management
│   │   ├── calls/page.tsx      # Call history with robocall badges
│   │   ├── calls/live/page.tsx # Live Ops with SSE co-pilot
│   │   ├── documents/page.tsx  # Knowledge base + RAG search
│   │   └── settings/page.tsx   # LLM config, prompts, SBC, blocklist
│   ├── src/components/         # Sidebar, dashboard cards, transcript viewer
│   ├── src/lib/                # TypeScript types, gateway client, DB client
│   ├── prisma/schema.prisma    # PostgreSQL schema
│   ├── Dockerfile
│   └── package.json
├── test/
│   ├── simcall.go              # Synthetic pipeline test
│   ├── livecall.go             # Real mic/speaker voice call
│   ├── simcopilot.go           # Two-party co-pilot simulation
│   ├── callcenter-live.sh      # Full call center demo with mic
│   ├── test-sip-call.sh        # SIP call via baresip
│   └── test-sbc.sh             # SBC integration tests
├── k8s/base/                   # Kustomize manifests for KinD deployment
├── docker-compose.sip.yml      # Full platform (7 services)
├── kind-config.yaml
├── Makefile
└── README.md
```

## Configuration

### Gateway Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LISTEN_ADDR` | `:8080` | Gateway listen address |
| `STT_URL` | `http://whisper:8000/v1/audio/transcriptions` | Whisper STT endpoint |
| `TTS_URL` | `http://piper:5000` | Piper TTS endpoint |
| `GCP_PROJECT_ID` | from `ANTHROPIC_VERTEX_PROJECT_ID` | GCP project for Vertex AI |
| `GCP_REGION` | from `CLOUD_ML_REGION` or `us-east5` | Vertex AI region |
| `CLAUDE_MODEL` | `claude-3-5-haiku@20241022` | Default Claude model |
| `SYSTEM_PROMPT` | *(conversational assistant)* | Interactive mode system prompt |
| `ESL_HOST` | `freeswitch` | FreeSWITCH ESL host |
| `ESL_PORT` | `8022` | FreeSWITCH ESL port |
| `TTS_AUDIO_DIR` | *(empty)* | Shared volume for ESL WAV playback |
| `DATABASE_URL` | *(empty)* | PostgreSQL connection string |
| `CHROMA_URL` | *(empty)* | ChromaDB endpoint for RAG |
| `CRM_WEBHOOK_URL` | *(empty)* | POST call summaries on hangup |
| `CRM_WEBHOOK_TOKEN` | *(empty)* | Bearer token for webhook auth |

### TTS Voice

Default: `en_US-ryan-high` (Piper). Available voices at [rhasspy/piper-voices](https://huggingface.co/rhasspy/piper-voices).

## Pipeline Internals

### Interactive Mode (5 goroutines)

```
readFromFS ──pcmIn──▶ sttPipeline ──transcripts──▶ claudeWorker ──sentences──▶ ttsWorker ──pcmOut──▶ writeToFS
                      (VAD+Whisper)  + robocall L3  (streaming SSE)             (Piper HTTP)          (ESL broadcast)
```

### Co-Pilot Mode (5 goroutines)

```
readCaller ──pcmCaller──▶ callerSTT ──┐
                                      ├──transcripts──▶ coachWorker ──▶ SSE → agent dashboard
readAgent  ──pcmAgent───▶ agentSTT  ──┘  + robocall L3  (RAG+Claude)   → post-call summary → CRM
```

### Robocall Detection Pipeline

```
Inbound call
    │
    ├── Layer 1: Blocklist hash map (< 1ms)
    │   MATCH → log + reject
    │
    ├── Layer 2: Audio pattern (first 2s)
    │   RMS variance, silence ratio, monotone detection
    │   score > 0.8 → flag as likely robocall
    │
    └── Layer 3: Transcript keywords (after Whisper)
        28 weighted phrases, combined scoring
        score > threshold → flag + optional auto-block
```

### Latency Profile

| Stage | Duration |
|-------|----------|
| Blocklist check | < 1ms |
| VAD silence detection | 500ms |
| Whisper transcription | 200-400ms |
| Robocall keyword check | < 1ms |
| RAG query (ChromaDB) | 50-100ms |
| Claude/Gemini response | 1-2s |
| Piper TTS synthesis | 200-400ms |
| **Co-pilot (speech → suggestion)** | **~2s** |
| **Interactive (speech → voice)** | **~3s** |

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

## Future Work

- **Streaming STT** — WebSocket streaming Whisper for sub-second transcription
- **Barge-in** — Cancel in-flight TTS when customer interrupts
- **Neural VAD** — Silero VAD for better speech boundary detection
- **Vertex AI Embeddings** — Replace n-gram hashing with `text-embedding-004` for RAG
- **ML Robocall Model** — Train a classifier on audio features for higher accuracy
- **STIR/SHAKEN** — Integrate SIP attestation headers for caller verification
- **Agent Desktop UI** — WebRTC softphone built into the dashboard
- **Call Recording** — Audio storage with playback in call detail view
- **Horizontal Scaling** — Redis session affinity for multi-replica gateway
- **Prometheus Metrics** — Call volume, latency percentiles, robocall detection rate
- **Multi-language** — STT/TTS language selection per call via SIP headers
