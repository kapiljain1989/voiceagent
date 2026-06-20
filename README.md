# VoiceAgent: AI-Powered Call Center Platform

A production-grade **AI call center platform** with standalone SIP gateway, real-time agent copilot, supervisor tools, IVR, call recording, and multi-tenant support. Connects directly to any SBC via SIP — no FreeSWITCH or external PBX required.

---

## Architecture

```
                    ┌─────────────────────────────────────────────────┐
                    │              VoiceAgent Gateway                  │
                    │                                                  │
  Caller ──SIP──►   │  SIP B2BUA ──► RTP Listener ──► STT (Whisper)   │
  (Phone)           │      │              │                 │          │
                    │      │              ▼                 ▼          │
                    │      │        Audio Mixer      Claude Copilot    │
                    │      │              │                 │          │
                    │      ▼              ▼                 ▼          │
  Agent  ◄─WebRTC─  │  WebRTC Bridge ◄──────────── SSE Events        │
  (Browser)         │                                                  │
                    │  IVR Engine │ ACD │ Queue │ Recording │ Webhook  │
                    └─────────────────────────────────────────────────┘
                                          │
                    ┌─────────┬───────────┼───────────┬────────────┐
                    │         │           │           │            │
                Postgres   Whisper     Piper TTS    Redis      ChromaDB
                  (DB)     (STT)       (Voice)    (Sessions)    (RAG)
```

---

## Features

### Call Handling
- **Standalone SIP B2BUA** — accepts SIP INVITE, handles RTP, sends BYE. No FreeSWITCH dependency.
- **SIPREC Observer** — plug into any SBC's SIPREC recording output for passive call monitoring.
- **WebRTC Agent Console** — browser-based agent desktop with bidirectional audio (PCMU codec).
- **Outbound Calling** — agent-initiated calls through configured SIP trunks.
- **DID Routing** — route inbound calls to queues/agents based on dialed number (exact, prefix, regex match).
- **IVR System** — configurable flow engine with TTS prompts, DTMF collection, timeout/retry, and AI intent routing.

### Agent Tools
- **ACD (Automatic Call Distribution)** — skills-based routing, priority scoring, idle-time weighting.
- **Hold / Resume** — hold music via TTS, audio bridge pause/resume.
- **Call Transfer** — blind transfer to queue or agent with re-routing.
- **3-Way Conference** — audio mixer with per-participant exclusion. Add another agent or external number.
- **Queue Announcements** — position and wait time via Piper TTS every 30 seconds.

### AI Copilot
- **Live Transcription** — Whisper STT with time-based chunking and hallucination filtering.
- **Agent Coaching** — Claude analyzes conversation in real-time, suggests responses via SSE.
- **Voice Sentiment** — acoustic analysis of pitch, energy, speaking rate, agitation, frustration, engagement.
- **Post-Call Summary** — AI-generated summary with action items, commitments, and sentiment.
- **RAG Knowledge Base** — document upload, ChromaDB vector search, context-grounded suggestions.

### Supervisor Tools
- **Live Dashboard** — real-time view of all active calls with sentiment, queue depths, agent status.
- **Monitor (Listen-Only)** — supervisor hears both sides silently via WebRTC.
- **Whisper** — supervisor coaches agent in real-time; caller cannot hear.
- **Barge** — supervisor joins call as third party via conference mixer.

### Enterprise
- **Call Recording** — stereo WAV (caller left, agent right), automatic on every call, playback API.
- **Reporting** — call volume by hour, agent performance, sentiment trends. All calls persisted to database.
- **CRM Webhooks** — configurable event dispatch (call_started, call_ended) with retry, auth, and execution log.
- **Multi-Tenant** — tenant isolation with per-tenant agents, trunks, DIDs, queues, and usage limits.
- **Role-Based Access** — admin, supervisor, agent roles with page-level access control.

### Security
- **SIP Trunk Authentication** — IP whitelist + SIP digest auth with configurable policies.
- **PII Masking** — credit card, SSN, phone number patterns masked before LLM processing.
- **Robocall Detection** — blocklist + audio pattern + keyword analysis.
- **Voice Biometrics** — spectral fingerprint for caller identity verification.

---

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Go 1.22+ (for native gateway development)
- Node.js 22+ (for UI development)

### Deploy with Docker Compose

```bash
git clone https://github.com/kapiljain1989/voiceagent.git
cd voiceagent

# Start all services (Postgres, Whisper, Piper, Redis, ChromaDB, UI)
docker compose -f docker-compose.test.yml up -d

# Run gateway natively (for SIP/RTP on host network)
cd gateway
go build -o /tmp/voiceagent-gateway .
DATABASE_URL="postgres://voiceagent:voiceagent@localhost:5432/voiceagent?sslmode=disable" \
SIP_LISTEN_ADDR=":5060" \
MODE=standalone \
STT_URL="http://localhost:8000/v1/audio/transcriptions" \
TTS_URL="http://localhost:5050" \
/tmp/voiceagent-gateway

# Open the dashboard
open http://localhost:3000
```

### Deploy on Kubernetes (KinD)

```bash
./deploy-local.sh
```

---

## Connecting to Your SBC

VoiceAgent works with any SBC that supports standard SIP. The gateway acts as a SIP User Agent Server (UAS) that accepts INVITEs and handles RTP directly.

### Direct SIP Trunk (B2BUA Mode)

Configure your SBC to send calls to VoiceAgent as a SIP trunk peer.

**Gateway configuration:**
```bash
SIP_LISTEN_ADDR=":5060"    # SIP signaling port (UDP + TCP)
MODE=standalone            # No FreeSWITCH, gateway handles everything
```

**SBC configuration** — point a SIP trunk to `<VOICEAGENT_IP>:5060`:

| SBC | Configuration |
|-----|--------------|
| **Cisco CUBE** | `dial-peer voice 100 voip` → `session target ipv4:<VOICEAGENT_IP>` |
| **AudioCodes** | IP Group → Proxy Set → `<VOICEAGENT_IP>:5060` |
| **Oracle SBC** | session-agent → `ip-address <VOICEAGENT_IP>`, `port 5060` |
| **Kamailio** | `$du = "sip:<VOICEAGENT_IP>:5060";` in route block |
| **Asterisk** | `[voiceagent]` trunk → `host=<VOICEAGENT_IP>`, `port=5060` |
| **FreeSWITCH** | Gateway profile → `<param name="proxy" value="<VOICEAGENT_IP>:5060"/>` |

### SIPREC Observer Mode

For passive monitoring without routing calls through VoiceAgent, configure your SBC's SIPREC recording server:

| SBC | Configuration |
|-----|--------------|
| **Cisco CUBE** | `media-recording <VOICEAGENT_IP> port 5060` |
| **AudioCodes** | SIP Recording → Recording Server = `<VOICEAGENT_IP>:5060` |
| **Oracle SBC** | session-recording → destination = `sip:<VOICEAGENT_IP>:5060` |
| **Kamailio** | `siprec_start_recording("sip:<VOICEAGENT_IP>:5060")` |

### SIP Trunk Security

Configure trunk authentication in the Settings UI or via API:

```bash
# Add trunk with IP whitelist + digest auth
curl -X POST http://localhost:8080/api/sip/trunks \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Production SBC",
    "address": "10.0.1.100",
    "port": 5060,
    "security_policy": "strict",
    "allowed_ips": ["10.0.1.100", "10.0.1.101"],
    "auth_username": "voiceagent",
    "auth_password": "secure-password"
  }'
```

Security policies:
- **strict** — IP whitelist + SIP digest authentication required
- **permissive** — IP whitelist only (no digest auth)
- **disabled** — accept from any source (development only)

### DID Routing

Map dialed numbers to queues, agents, or IVR flows:

```bash
# Route +18005550100 to Sales queue
curl -X POST http://localhost:8080/api/routing/dids \
  -d '{"did_pattern": "+18005550100", "match_type": "exact", "destination_type": "queue", "destination_value": "Sales"}'

# Route all 1-800 numbers to IVR
curl -X POST http://localhost:8080/api/routing/dids \
  -d '{"did_pattern": "1800", "match_type": "prefix", "destination_type": "queue", "destination_value": "Support", "ivr_id": "<ivr-flow-uuid>"}'

# Default route (catch-all)
curl -X POST http://localhost:8080/api/routing/dids \
  -d '{"did_pattern": "*", "match_type": "prefix", "destination_type": "queue", "destination_value": "Support"}'
```

---

## IVR Configuration

Create an IVR flow and assign it to a DID route:

```bash
curl -X POST http://localhost:8080/api/ivr \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Main Menu",
    "flow_data": {
      "entry": "welcome",
      "nodes": {
        "welcome": {
          "type": "play",
          "prompt": "Thank you for calling. Press 1 for Sales, 2 for Support.",
          "next": "collect"
        },
        "collect": {
          "type": "collect",
          "timeout_ms": 5000,
          "retries": 2,
          "dtmf_map": {"1": "sales", "2": "support"},
          "timeout_node": "support"
        },
        "sales": {
          "type": "transfer",
          "destination_type": "queue",
          "destination_value": "Sales"
        },
        "support": {
          "type": "transfer",
          "destination_type": "queue",
          "destination_value": "Support"
        }
      }
    }
  }'
```

Node types: `play` (TTS prompt), `collect` (DTMF with timeout/retry), `transfer` (route to queue/agent), `hangup`.

---

## CRM Webhooks

Receive call events in your CRM or ticketing system:

```bash
curl -X POST http://localhost:8080/api/webhooks \
  -d '{
    "name": "Salesforce",
    "url": "https://your-crm.com/webhook",
    "events": ["call_started", "call_ended"],
    "auth_type": "bearer",
    "auth_value": "your-api-token",
    "retry_count": 3
  }'
```

Webhook payload (call_ended):
```json
{
  "event": "call_ended",
  "call_id": "abc-123",
  "timestamp": "2026-06-20T16:00:00Z",
  "caller": "+15551234567",
  "agent": "Sarah",
  "duration": 245,
  "sentiment": "positive",
  "summary": "Customer called about billing. Issue resolved."
}
```

---

## Multi-Tenant Setup

Create isolated tenants for SaaS deployment:

```bash
# Create tenant
curl -X POST http://localhost:8080/api/tenants \
  -d '{"name": "Acme Corp", "domain": "acme.com", "max_agents": 50}'

# Assign user to tenant
curl -X POST http://localhost:8080/api/tenants/users \
  -d '{"user_id": "<user-uuid>", "tenant_id": "<tenant-uuid>"}'
```

Each tenant gets isolated agents, queues, trunks, DIDs, recordings, and webhooks.

---

## API Reference

| Category | Endpoints |
|----------|-----------|
| **SIP Trunks** | `GET/POST/PUT/DELETE /api/sip/trunks` |
| **DID Routing** | `GET/POST/DELETE /api/routing/dids` |
| **IVR** | `GET/POST/PUT/DELETE /api/ivr` |
| **Agents** | `GET/POST /api/agents`, `/api/agent/me` |
| **Queues** | `GET /api/queues`, `POST /api/queue/pick` |
| **Call Control** | `POST /api/call/hold`, `/resume`, `/transfer`, `/conference`, `/conference/drop` |
| **Outbound** | `POST /api/call/outbound` |
| **WebRTC** | `POST /api/webrtc/bridge` |
| **Supervisor** | `GET /api/supervisor/calls`, `POST /api/supervisor/monitor`, `/stop` |
| **Recordings** | `GET /api/recordings`, `GET /api/recordings/:id` |
| **Reports** | `GET /api/reports/calls`, `/agents`, `/sentiment` |
| **Webhooks** | `GET/POST/DELETE /api/webhooks`, `POST /api/webhooks/test`, `GET /api/webhooks/logs` |
| **Tenants** | `GET/POST/PUT/DELETE /api/tenants`, `POST /api/tenants/users` |
| **Copilot** | `GET /api/copilot/active`, `GET /siprec/events` (SSE) |
| **Stats** | `GET /api/stats`, `GET /healthz`, `GET /metrics` |

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SIP_LISTEN_ADDR` | (empty) | SIP signaling address (e.g. `:5060`) |
| `MODE` | `standalone` | `standalone` (B2BUA) or `gateway` (with FreeSWITCH) |
| `DATABASE_URL` | (empty) | PostgreSQL connection string |
| `STT_URL` | `http://whisper:8000/v1/audio/transcriptions` | Whisper STT endpoint |
| `TTS_URL` | `http://piper:5000` | Piper TTS endpoint |
| `RECORDING_DIR` | `/tmp/recordings` | Call recording storage path |
| `REDIS_URL` | (empty) | Redis for distributed sessions |
| `GCP_PROJECT_ID` | (empty) | GCP project for Vertex AI (Claude/Gemini) |
| `GCP_REGION` | `us-east5` | Vertex AI region |
| `CLAUDE_MODEL` | `claude-3-5-haiku@20241022` | Claude model for copilot |
| `AUTH_ENABLED` | `false` | Enable JWT authentication |
| `CHROMA_URL` | (empty) | ChromaDB URL for RAG |

---

## UI Pages

| Page | URL | Access | Description |
|------|-----|--------|-------------|
| Dashboard | `/` | admin, supervisor | Active calls, stats, sentiment overview |
| Agents | `/agents` | admin, supervisor | Agent management, status, skills |
| Calls | `/calls` | admin, supervisor, agent | Call history with transcript and recording playback |
| Console | `/console` | admin, supervisor, agent | Agent desktop: accept calls, dial, transfer, conference |
| Supervisor | `/supervisor` | admin, supervisor | Live call monitoring, whisper, barge |
| Documents | `/documents` | admin | Knowledge base upload for RAG |
| Settings | `/settings` | admin | SIP trunks, DID routing, LLM config, system prompts |

---

## Tech Stack

| Component | Technology |
|-----------|------------|
| Gateway | Go 1.25, sipgo (SIP), pion/webrtc (WebRTC), pion/rtp (RTP) |
| UI | Next.js 16, React, Tailwind CSS, shadcn/ui |
| STT | faster-whisper (small.en model) |
| TTS | Piper (neural voice synthesis) |
| LLM | Claude via Vertex AI |
| Database | PostgreSQL |
| Vector DB | ChromaDB |
| Sessions | Redis (optional, falls back to in-memory) |
| Codec | G.711 u-law/A-law with pre-computed lookup tables |

---

## License

Apache 2.0
