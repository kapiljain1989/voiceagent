# AI Media Gateway — Call Center Platform

An AI-powered call center platform that handles inbound/outbound SIP voice calls, provides real-time agent assist with RAG-powered knowledge retrieval, and generates automated post-call summaries. Built with Go, Next.js, FreeSWITCH, Whisper STT, Piper TTS, Claude/Gemini on Vertex AI, PostgreSQL, and ChromaDB.

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
│  Mode 1: Interactive AI Agent        Mode 2: Co-Pilot Agent Assist           │
│  ┌─────────────────────────┐         ┌─────────────────────────┐             │
│  │ /ws — WebSocket audio   │         │ /siprec — Dual-leg audio │            │
│  │ readFromFS → sttPipeline│         │ callerSTT + agentSTT    │             │
│  │ → claudeWorker → TTS   │         │ → coachWorker (RAG)     │             │
│  │ → writeToFS (playback) │         │ → SSE suggestions       │             │
│  └─────────────────────────┘         │ → Post-call summary     │             │
│                                      │ → CRM webhook           │             │
│  REST API                            └─────────────────────────┘             │
│  /api/agents  /api/calls  /api/documents  /api/llm  /api/stats              │
│  /call (ESL originate)  /healthz                                             │
└──────────┬──────────┬──────────┬──────────┬──────────┬───────────────────────┘
           │          │          │          │          │
     FreeSWITCH  Whisper STT  Piper TTS  PostgreSQL  ChromaDB
     (:5060)     (:8000)      (:5000)    (:5432)     (:8200)
```

## Two Operating Modes

### Mode 1: Interactive AI Agent

The AI answers calls directly — customer speaks, Claude responds with synthesized voice.

```
Customer → SIP → FreeSWITCH → mod_audio_fork → Gateway WebSocket
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
      │                    │
      └────────┬───────────┘
          transcripts (speaker-labeled)
               │
     RAG query (ChromaDB) → relevant policy docs
               │
         coachWorker (Claude with RAG context)
               │
      ┌────────┴────────┐
  SSE events         [on hangup]
  → Agent dashboard   summaryWorker
  → UI transcript     → call summary + sentiment
                      → POST webhook → CRM
```

## Services (7 containers)

| Service | Image | Port | Role |
|---------|-------|------|------|
| `gateway` | `voiceagent/gateway` | 8080 | Go WebSocket gateway, REST API, ESL client, RAG |
| `freeswitch` | `drachtio/drachtio-freeswitch-mrf` | 5070 | SIP/RTP engine, mod_audio_fork |
| `whisper` | `fedirz/faster-whisper-server` | 8000 | Local STT (faster-whisper-base.en) |
| `piper` | `artibex/piper-http` | 5000 | Local TTS (en_US-ryan-high, 16kHz) |
| `postgres` | `postgres:16-alpine` | 5432 | Agents, calls, documents, LLM configs |
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

Only Claude/Gemini on Vertex AI require cloud access. STT, TTS, and RAG run entirely locally.

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
| Call History | `/calls` | Paginated call log with sentiment and mode filters |
| Live Operations | `/calls/live` | Real-time transcript + co-pilot suggestions (SSE) |
| Knowledge Base | `/documents` | Document upload, RAG indexing, search testing |
| Configuration | `/settings` | LLM models (Claude/Gemini), system prompts, SBC trunk |

## Testing

### Live Call Center Demo (Co-Pilot Mode)

The full call center scenario — you speak as the customer, the co-pilot provides real-time suggestions on screen.

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
#    → Co-pilot shows: "Section 4.2.1 covers burst pipe damage. $500 deductible."
```

### Interactive AI Agent (Voice Call)

Call the AI directly — it responds with Claude's voice through your speaker.

```bash
# WebSocket mode (simplest — uses mic/speaker directly)
cd test && ./livecall ws://localhost:8080/ws

# SIP mode (via baresip softphone)
cd test && SIP_PORT=5070 ./test-sip-call.sh
# Type: /dial 1000
```

### Simulated Pipeline Test (No Microphone)

```bash
cd test && go run simcall.go
```

### Co-Pilot Simulation (No Microphone)

```bash
cd test && ./simcopilot localhost:8080
```

## API Reference

### Voice Endpoints

| Endpoint | Protocol | Description |
|----------|----------|-------------|
| `/ws` | WebSocket | Interactive AI agent — send PCM audio, receive TTS audio + transcript events |
| `/siprec` | WebSocket | Co-pilot — `?role=caller\|agent&call_id=xxx`, receive audio legs |
| `/siprec/events` | SSE | Agent dashboard — `?call_id=xxx`, streams transcript/suggestion/summary events |
| `/call` | POST | Originate outbound call via ESL (`{"to":"+15551234567","mode":"sbc\|loopback"}`) |

### REST API

| Endpoint | Methods | Description |
|----------|---------|-------------|
| `/api/agents` | GET, POST | List/create agents with expertise and status |
| `/api/calls` | GET | Call history with transcript, summary, sentiment |
| `/api/calls/active` | GET | Currently active interactive + copilot sessions |
| `/api/documents` | GET, POST | Upload documents for RAG indexing |
| `/api/documents/search` | POST | RAG query — `{"query":"...","top_k":3}` |
| `/api/llm/configs` | GET, POST | Manage LLM model configurations |
| `/api/llm/test` | POST | Test a model — `{"provider":"anthropic-vertex","model":"...","prompt":"..."}` |
| `/api/stats` | GET | Dashboard stats (active calls, sentiment breakdown) |
| `/healthz` | GET | Health check with session count |

### SSE Event Types (`/siprec/events`)

```json
{"type":"transcript","speaker":"customer","text":"I need help with my claim"}
{"type":"suggestion","suggestion":"Policy 4.2.1 covers this...","category":"answer","confidence":0.95}
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

# Search the knowledge base
curl -X POST http://localhost:8080/api/documents/search \
  -d '{"query":"how long to return electronics","top_k":3}'

# Response:
[{"text":"Returns accepted within 30 days...","doc_name":"Returns Policy","score":0.95}]
```

When a customer asks a question during a co-pilot call, the system automatically:
1. Queries ChromaDB with the customer's utterance
2. Retrieves the top 3 matching document chunks
3. Injects them as context into Claude's system prompt
4. Claude generates suggestions grounded in actual policy documents

## Multi-LLM Support

Both Claude and Gemini on Vertex AI are supported. Configure and test via the API or UI Settings page.

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

Webhook payload:

```json
{
  "conversation_id": "uuid",
  "duration_seconds": 180,
  "transcript": [{"speaker":"customer","text":"...","timestamp":"..."}],
  "summary": "Customer called about billing dispute...",
  "action_items": ["Issue refund", "Send confirmation email"],
  "commitments_made": ["Callback within 24 hours"],
  "sentiment": "negative",
  "suggestions_given": [{"suggestion":"...","category":"empathy"}]
}
```

## Project Structure

```
voiceagent/
├── gateway/
│   ├── main.go             # Media gateway, WebSocket, VAD, ESL, interactive pipeline
│   ├── siprec.go           # Co-pilot session, dual-leg STT, coach worker, summary, webhook
│   ├── llm.go              # Multi-LLM abstraction (Claude + Gemini on Vertex AI)
│   ├── api.go              # REST API (agents, calls, documents, stats, LLM config)
│   ├── rag.go              # ChromaDB integration, document chunking, RAG query
│   ├── go.mod / go.sum
│   └── Dockerfile
├── freeswitch/
│   ├── Dockerfile
│   ├── entrypoint.sh
│   └── config/
│       ├── dialplan/
│       │   ├── public.xml          # Inbound routing (1xxx=AI, 2xxx=copilot)
│       │   ├── public-local.xml    # Docker Compose variant
│       │   └── outbound.xml        # SBC trunk routing
│       ├── sip_profiles/
│       │   ├── external.xml        # SBC trunk profile
│       │   └── external-local.xml  # Docker Compose variant
│       └── autoload_configs/
│           ├── acl.conf.xml        # IP ACL for SBC peers
│           ├── modules.conf.xml    # mod_audio_fork, mod_sofia, mod_event_socket
│           ├── sofia.conf.xml
│           ├── switch.conf.xml
│           └── event_socket.conf.xml
├── whisper/
│   └── Dockerfile
├── ui/
│   ├── src/app/
│   │   ├── page.tsx                # Command Center dashboard
│   │   ├── agents/page.tsx         # Agent management
│   │   ├── calls/page.tsx          # Call history
│   │   ├── calls/live/page.tsx     # Live Ops with SSE co-pilot
│   │   ├── documents/page.tsx      # Knowledge base + RAG
│   │   └── settings/page.tsx       # LLM config, prompts, SBC
│   ├── src/components/
│   │   └── layout/Sidebar.tsx      # Navigation sidebar
│   ├── src/lib/
│   │   ├── types.ts                # Shared TypeScript types
│   │   ├── gateway.ts              # Gateway API client
│   │   └── db.ts                   # Database client
│   ├── prisma/schema.prisma        # PostgreSQL schema
│   ├── Dockerfile
│   └── package.json
├── test/
│   ├── simcall.go                  # Synthetic pipeline test
│   ├── livecall.go                 # Real mic/speaker voice call
│   ├── simcopilot.go               # Two-party co-pilot simulation
│   ├── callcenter-live.sh          # Full call center demo with mic
│   ├── test-sip-call.sh            # SIP call via baresip
│   ├── test-sbc.sh                 # SBC integration tests
│   └── sipp/inbound_call.xml       # sipp UAC scenario
├── k8s/base/                       # Kustomize manifests for KinD deployment
├── docker-compose.sip.yml          # Full platform (7 services)
├── kind-config.yaml
├── Makefile
└── README.md
```

## Configuration

### Gateway Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LISTEN_ADDR` | `:8080` | Gateway listen address |
| `STT_URL` | `http://whisper:8000/v1/audio/transcriptions` | Whisper API endpoint |
| `TTS_URL` | `http://piper:5000` | Piper TTS endpoint |
| `GCP_PROJECT_ID` | from `ANTHROPIC_VERTEX_PROJECT_ID` | GCP project for Vertex AI |
| `GCP_REGION` | from `CLOUD_ML_REGION` or `us-east5` | Vertex AI region |
| `CLAUDE_MODEL` | `claude-3-5-haiku@20241022` | Default Claude model |
| `SYSTEM_PROMPT` | *(conversational voice assistant)* | Interactive mode system prompt |
| `ESL_HOST` | `freeswitch` | FreeSWITCH ESL host |
| `ESL_PORT` | `8022` | FreeSWITCH ESL port |
| `TTS_AUDIO_DIR` | *(empty)* | Shared volume for ESL WAV playback |
| `DATABASE_URL` | *(empty)* | PostgreSQL connection string |
| `CHROMA_URL` | *(empty)* | ChromaDB endpoint for RAG |
| `CRM_WEBHOOK_URL` | *(empty)* | POST call summaries here on hangup |
| `CRM_WEBHOOK_TOKEN` | *(empty)* | Bearer token for webhook auth |

### TTS Voice

Default: `en_US-ryan-high` (Piper). Change via `MODEL_DOWNLOAD_LINK` in docker-compose:

```yaml
piper:
  environment:
    - MODEL_DOWNLOAD_LINK=https://huggingface.co/rhasspy/piper-voices/resolve/v1.0.0/en/en_US/ryan/high/en_US-ryan-high.onnx?download=true
```

Available voices at [rhasspy/piper-voices](https://huggingface.co/rhasspy/piper-voices).

## Pipeline Internals

### Interactive Mode (5 goroutines)

```
readFromFS ──pcmIn──▶ sttPipeline ──transcripts──▶ claudeWorker ──sentences──▶ ttsWorker ──pcmOut──▶ writeToFS
                      (VAD + Whisper)               (streaming SSE)             (Piper HTTP)          (ESL broadcast)
```

### Co-Pilot Mode (5 goroutines)

```
readCaller ──pcmCaller──▶ callerSTT ──┐
                                      ├──transcripts──▶ coachWorker ──▶ SSE broadcast
readAgent  ──pcmAgent───▶ agentSTT  ──┘                (RAG + Claude)   → agent dashboard
                                                                        → post-call summary
                                                                        → CRM webhook
```

### Latency Profile

| Stage | Duration |
|-------|----------|
| VAD silence detection | 500ms |
| Whisper transcription (base.en) | 200-400ms |
| RAG query (ChromaDB) | 50-100ms |
| Claude/Gemini response | 1-2s |
| Piper TTS synthesis | 200-400ms |
| **Co-pilot (speech → suggestion)** | **~2s** |
| **Interactive (speech → voice response)** | **~3s** |

## Observability

```bash
# Docker Compose logs
docker compose -f docker-compose.sip.yml logs -f gateway
docker compose -f docker-compose.sip.yml logs -f freeswitch
docker compose -f docker-compose.sip.yml logs -f whisper

# KinD logs
make logs-gw
make logs-fs
make status
```

## Cleanup

```bash
# Docker Compose
docker compose -f docker-compose.sip.yml down -v

# KinD
make clean
```

## Future Work

- **Streaming STT** — Replace batch Whisper with WebSocket streaming for sub-second transcription
- **Barge-in** — Cancel in-flight TTS when customer interrupts
- **Neural VAD** — Replace energy-based VAD with Silero for better accuracy
- **Knowledge Base RAG with Vertex AI Embeddings** — Replace n-gram hashing with `text-embedding-004`
- **Agent Desktop UI** — Full React dashboard with WebRTC softphone built-in
- **Call Recording** — Store audio files with playback in call detail view
- **Horizontal Scaling** — Multiple gateway replicas with Redis session affinity
- **Metrics** — Prometheus endpoints for call volume, latency percentiles, pipeline health
- **Multi-language** — Configure STT/TTS language per call via SIP headers
