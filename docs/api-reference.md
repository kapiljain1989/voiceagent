# API Reference

Complete API documentation for the VoiceAgent Telecom-Native AI Gateway.

**Base URL:** `http://<HOST>:8080`

**Authentication:** Most endpoints require a JWT token or API key. See [Authentication](#authentication).

---

## Table of Contents

- [Authentication](#authentication)
- [Voice & Real-Time](#voice--real-time)
- [Call Control](#call-control)
- [Agent Management](#agent-management)
- [Queue Management](#queue-management)
- [Call Routing & DIDs](#call-routing--dids)
- [WebRTC](#webrtc)
- [Call History & Reports](#call-history--reports)
- [Recordings](#recordings)
- [Documents & RAG](#documents--rag)
- [LLM Configuration](#llm-configuration)
- [Supervisor](#supervisor)
- [Robocall Detection](#robocall-detection)
- [Security & PII](#security--pii)
- [Self-Service Actions](#self-service-actions)
- [Webhooks](#webhooks)
- [SIP Trunks](#sip-trunks)
- [IVR](#ivr)
- [Tenants](#tenants)
- [Settings & Configuration](#settings--configuration)
- [Health & Infrastructure](#health--infrastructure)
- [Error Responses](#error-responses)

---

## Authentication

Auth is enabled by default (`AUTH_ENABLED=true`). Two methods are supported:

| Method | Header | Format |
|--------|--------|--------|
| JWT Bearer | `Authorization` | `Bearer <token>` |
| API Key | `X-API-Key` | `<api-key>` |

### `POST /api/auth/login`

Get a JWT token. No auth required.

**Request:**
```json
{
  "username": "admin",
  "password": "admin"
}
```

**Response (200):**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "username": "admin",
  "role": "admin"
}
```

### `GET /api/auth/me`

Get authenticated user profile.

**Response:**
```json
{
  "user_id": "uuid",
  "username": "admin",
  "role": "admin",
  "exp": 1719014400
}
```

### `GET /api/auth/apikey`

Retrieve API key for the authenticated user.

**Response:**
```json
{"api_key": "vag_abc123..."}
```

### `GET /api/auth/users`

List all users (admin only).

**Response:**
```json
[
  {
    "id": "uuid",
    "username": "admin",
    "role": "admin",
    "created_at": "2026-06-06T12:00:00Z"
  }
]
```

### `POST /api/auth/users`

Create a new user (admin only).

**Request:**
```json
{
  "username": "agent1",
  "password": "secure-password",
  "role": "agent"
}
```

**Response (201):**
```json
{"status": "created", "api_key": "vag_..."}
```

---

## Voice & Real-Time

### `WebSocket /ws` — Interactive AI Agent

Bidirectional audio streaming. FreeSWITCH connects here via `mod_audio_fork`. Accepts raw L16 PCM audio, returns TTS audio + JSON events.

**Connection:**
```
ws://gateway:8080/ws?uuid=<freeswitch-channel-uuid>
Subprotocol: audio.drachtio.org
```

**Inbound frames (client → gateway):**
| Type | Format | Description |
|------|--------|-------------|
| Binary | L16 PCM 16-bit LE, mono, 16kHz | 20ms audio frames (640 bytes each) |
| Text | `{"type":"stop","callId":"..."}` | Graceful hangup signal |

**Outbound frames (gateway → client):**
| Type | Format | Description |
|------|--------|-------------|
| Binary | L16 PCM 16-bit LE, mono, 16kHz | TTS playback audio (20ms frames) |
| Text | `{"event":"transcript","text":"..."}` | Whisper transcription of caller speech |
| Text | `{"event":"response","text":"..."}` | LLM response text |
| Text | `{"event":"robocall","text":"score=80%..."}` | Robocall detection alert |
| Text | `{"event":"pii_masked","text":"Detected 2..."}` | PII masking notification |
| Text | `{"event":"action","intent":"..."}` | Self-service action executed |
| Text | `{"event":"transfer","reason":"..."}` | Call transfer initiated |

**Pipeline:** Audio → VAD (500ms silence) → Whisper STT → PII masking → robocall check → LLM (action-aware) → Piper TTS → playback

---

### `WebSocket /siprec` — Co-Pilot Agent Assist

Passive observation of a two-party call via SIPREC. The SBC/PBX sends two separate WebSocket connections (one per audio leg).

**Connection:**
```
ws://gateway:8080/siprec?role=caller&call_id=<uuid>
ws://gateway:8080/siprec?role=agent&call_id=<uuid>
```

| Parameter | Required | Values | Description |
|-----------|----------|--------|-------------|
| `call_id` | Yes | UUID string | Unique call identifier |
| `role` | Yes | `caller` or `agent` | Which party's audio this stream carries |

**Inbound:** Same binary PCM frames as `/ws`.

**Behavior:** Both legs are transcribed independently with speaker labels. LLM coach mode generates real-time suggestions. On hangup, a call summary is generated and POSTed to the CRM webhook (if configured).

---

### `GET /siprec/events` — SSE Event Stream

Server-Sent Events for the agent dashboard. Subscribe to receive real-time transcripts, suggestions, and call state changes.

**Connection:**
```
curl -N http://localhost:8080/siprec/events?call_id=<uuid>
```

**Event types:**

```json
{"type":"transcript","speaker":"customer","text":"I need help with my claim"}

{"type":"transcript","speaker":"agent","text":"Let me look that up for you"}

{"type":"suggestion","suggestion":"Policy 4.2.1 covers burst pipe damage.","category":"answer","confidence":0.95,"context":"water damage"}

{"type":"call_state","state":"ringing|connected|hold|ended","call_id":"uuid"}

{"type":"robocall","text":"score=80% keywords=[press 1, auto warranty]"}

{"type":"pii_masked","text":"Detected 2 PII items: credit_card, cvv"}

{"type":"action","intent":"reschedule","status":"success"}

{"type":"transfer","reason":"angry","department":"retention","priority":"urgent","summary":"Customer upset about billing"}

{"type":"summary","summary":"Customer called about water damage claim.","action_items":["File claim within 30 days"],"commitments":["Callback within 24 hours"],"sentiment":"neutral","duration":180}
```

---

### `GET /siprec/summary`

Query the post-call summary for a completed SIPREC session.

**Query parameters:** `?call_id=<uuid>`

**Response:**
```json
{
  "summary": "Customer called about billing dispute...",
  "action_items": ["Issue credit for $45.00"],
  "commitments": ["Follow-up email within 24h"],
  "sentiment": "neutral",
  "duration": 245
}
```

---

## Call Control

### `POST /call`

Originate an outbound call via FreeSWITCH ESL. No auth required.

**Request:**
```json
{
  "to": "+15551234567",
  "from": "+15559876543",
  "mode": "sbc"
}
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `to` | Yes | | Destination number |
| `from` | No | `0000000000` | Caller ID |
| `mode` | No | `sbc` | `sbc` (via SBC trunk) or `loopback` (local test) |

**Response (200):**
```json
{"status": "originated", "call_id": "a1b2c3d4-..."}
```

### `POST /api/call/outbound`

Originate an outbound call via SIP server (standalone mode).

**Request:**
```json
{
  "to": "+15551234567",
  "from": "+15559876543",
  "agent_id": "uuid"
}
```

### `POST /api/call/hold`

Place a call on hold with hold music.

**Request:**
```json
{"call_id": "uuid", "agent_id": "uuid"}
```

**Response:**
```json
{"status": "ok", "state": "hold"}
```

### `POST /api/call/resume`

Resume a held call.

**Request:**
```json
{"call_id": "uuid", "agent_id": "uuid"}
```

**Response:**
```json
{"status": "ok", "state": "connected"}
```

### `POST /api/call/mute`

Mute the agent's microphone.

**Request:**
```json
{"call_id": "uuid", "agent_id": "uuid"}
```

**Response:**
```json
{"status": "ok", "state": "muted"}
```

### `POST /api/call/unmute`

Unmute the agent's microphone.

**Request:**
```json
{"call_id": "uuid", "agent_id": "uuid"}
```

**Response:**
```json
{"status": "ok", "state": "connected"}
```

### `POST /api/call/transfer`

Transfer a call (blind or warm) to a queue, agent, or external number.

**Request:**
```json
{
  "call_id": "uuid",
  "transfer_type": "blind",
  "target_type": "queue",
  "target_value": "billing",
  "agent_id": "uuid"
}
```

| Field | Values | Description |
|-------|--------|-------------|
| `transfer_type` | `blind`, `warm` | Blind (immediate) or warm (announced) transfer |
| `target_type` | `queue`, `agent`, `external` | Where to transfer |
| `target_value` | string | Queue name, agent ID, or phone number |

**Response:**
```json
{"status": "transferred", "target": "billing"}
```

### `POST /api/call/conference`

Start a three-way conference with a third party.

**Request:**
```json
{
  "call_id": "uuid",
  "target": "+15551234567",
  "target_type": "external",
  "agent_id": "uuid"
}
```

**Response:**
```json
{
  "status": "conference_started",
  "call_id": "uuid",
  "third_party": "+15551234567"
}
```

### `POST /api/call/conference/drop`

Drop a party from the conference.

**Request:**
```json
{
  "call_id": "uuid",
  "who": "third"
}
```

| `who` | Description |
|-------|-------------|
| `third` | Drop the third party, keep agent + caller |
| `self` | Agent leaves, caller stays with third party |

**Response:**
```json
{"status": "third_party_dropped"}
```

---

## Agent Management

### `GET /api/agents`

List all agents.

**Response:**
```json
[
  {
    "id": "uuid",
    "name": "Priya Sharma",
    "email": "priya@company.com",
    "phone": "+1555101",
    "expertise": ["billing", "retention"],
    "status": "Available",
    "maxCalls": 3,
    "activeCalls": 1
  }
]
```

### `POST /api/agents`

Create a new agent.

**Request:**
```json
{
  "name": "Priya Sharma",
  "email": "priya@company.com",
  "phone": "+1555101",
  "expertise": ["billing", "retention"]
}
```

**Response (201):**
```json
{"id": "uuid", "status": "created"}
```

### `PUT /api/agents`

Update an agent.

**Request:**
```json
{
  "id": "uuid",
  "name": "Priya Sharma",
  "expertise": ["billing", "retention", "claims"],
  "maxCalls": 5
}
```

### `DELETE /api/agents`

Delete an agent.

**Request:**
```json
{"id": "uuid"}
```

### `GET /api/agent/me`

Current authenticated agent's profile. Creates an in-memory session on first call.

**Response:**
```json
{
  "linked": true,
  "profile": {
    "id": "uuid",
    "user_id": "uuid",
    "name": "Priya Sharma",
    "email": "priya@company.com",
    "extension": "1001",
    "department": "billing",
    "expertise": ["billing", "retention"],
    "languages": ["en", "hi"],
    "priority": 1,
    "max_calls": 3,
    "current_calls": 0,
    "status": "Available",
    "customer_tiers": ["premium"],
    "queues": ["billing", "general"]
  }
}
```

### `POST /api/agent/me/status`

Update agent availability status.

**Request:**
```json
{"status": "Available"}
```

| Status Values | Description |
|---------------|-------------|
| `Available` | Ready to take calls |
| `Busy` | On a call |
| `On Break` | Temporarily unavailable |
| `Wrap-up` | Post-call work |
| `Offline` | Signed out |

### `GET /api/agent/me/queues`

Queues assigned to the current agent.

**Response:**
```json
["billing", "general", "retention"]
```

### `GET /api/agent/me/events` — Agent SSE Stream

Server-Sent Events stream for ring notifications and state changes.

**Event types:**
```json
{"type": "ring", "call_id": "uuid", "caller": "+15551234567", "queue": "billing", "wait_seconds": 12}
{"type": "state", "status": "Available"}
```

### `GET /api/agents/online`

List all currently online agents.

**Response:**
```json
[
  {"agent_id": "uuid", "status": "Available", "since": "2026-06-06T14:00:00Z"}
]
```

### `GET /api/agents/directory`

Full agent directory with extensions and departments.

**Response:**
```json
[
  {
    "id": "uuid",
    "name": "Priya Sharma",
    "ext": "1001",
    "status": "Available",
    "department": "billing",
    "activeCalls": 0
  }
]
```

### `POST /api/agents/assign-queue`

Assign an agent to a queue.

**Request:**
```json
{"agent_id": "uuid", "queue": "billing"}
```

**Response:**
```json
{"status": "assigned"}
```

### `POST /api/agents/link-user`

Link an authenticated user account to an agent profile.

**Request:**
```json
{"agent_id": "uuid", "user_id": "uuid"}
```

**Response:**
```json
{"status": "linked"}
```

### `GET /api/agent/status`

Query agent status by ID.

**Query parameters:** `?agent_id=<uuid>`

### `POST /api/agent/status`

Update agent status (admin).

**Request:**
```json
{"agent_id": "uuid", "status": "Available"}
```

---

## Queue Management

### `GET /api/queues`

List all queues with callers waiting.

**Response:**
```json
[
  {
    "name": "billing",
    "avgHandle": "4m30s",
    "sla": 80,
    "callers": [
      {
        "id": "uuid",
        "call_id": "uuid",
        "number": "+15551234567",
        "waitSec": 45,
        "reason": "billing inquiry",
        "priority": "normal",
        "queue_name": "billing"
      }
    ]
  }
]
```

### `POST /api/queue/pick`

Agent picks a call from the queue.

**Request:**
```json
{
  "queue_entry_id": "uuid",
  "call_id": "uuid",
  "agent_id": "uuid",
  "webrtc_bridge": true
}
```

**Response:**
```json
{"status": "ok", "call_id": "uuid"}
```

### `POST /api/queue/add`

Manually add a caller to a queue.

**Request:**
```json
{
  "queue_name": "billing",
  "call_id": "uuid",
  "caller_number": "+15551234567",
  "reason": "billing dispute",
  "priority": "high"
}
```

| Priority | Description |
|----------|-------------|
| `low` | Standard queue position |
| `normal` | Default |
| `high` | Prioritized in queue |

---

## Call Routing & DIDs

### `GET /api/queues/list`

List queue definitions with routing configuration.

**Response:**
```json
[
  {
    "id": "uuid",
    "name": "billing",
    "description": "Billing and payments",
    "skills_required": ["billing"],
    "max_wait_seconds": 300,
    "overflow_queue": "general",
    "caller_count": 3
  }
]
```

### `POST /api/queues/create`

Create a new queue.

**Request:**
```json
{
  "name": "retention",
  "description": "Customer retention",
  "skills_required": ["retention", "billing"],
  "max_wait_seconds": 120,
  "overflow_queue": "general"
}
```

### `GET /api/queues/agents`

List agents assigned to a queue with match scoring.

**Query parameters:** `?queue=billing`

**Response:**
```json
[
  {
    "agent_id": "uuid",
    "name": "Priya Sharma",
    "extension": "1001",
    "score": 0.95,
    "reason": "skill_match + availability"
  }
]
```

### `POST /api/routing/test`

Test routing rules — given an intent and language, find the best queue and agent.

**Request:**
```json
{"intent": "billing", "language": "en"}
```

**Response:**
```json
{
  "queue": "billing",
  "agent": {
    "agent_id": "uuid",
    "name": "Priya Sharma",
    "extension": "1001",
    "score": 0.95,
    "reason": "skill_match"
  }
}
```

### `GET /api/routing/dids`

List DID (Direct Inward Dial) routing rules.

**Response:**
```json
[
  {
    "id": "uuid",
    "did_pattern": "+1555100*",
    "match_type": "prefix",
    "trunk_id": "uuid",
    "destination_type": "queue",
    "destination_value": "billing",
    "priority": 10,
    "overflow_destination": "general",
    "ivr_id": "uuid",
    "enabled": true,
    "created_at": "2026-06-06T12:00:00Z"
  }
]
```

### `POST /api/routing/dids`

Create a DID routing rule.

**Request:**
```json
{
  "did_pattern": "+1555200*",
  "match_type": "prefix",
  "destination_type": "queue",
  "destination_value": "support",
  "priority": 10,
  "enabled": true
}
```

| `destination_type` | Description |
|--------------------|-------------|
| `queue` | Route to a queue |
| `agent` | Route to a specific agent |
| `ivr` | Route to IVR flow |
| `external` | Forward to external number |

---

## WebRTC

Browser-based agent audio via WebRTC (Opus codec).

### `POST /api/webrtc/offer`

Send an SDP offer to establish a WebRTC session.

**Request:**
```json
{
  "sdp": "v=0\r\no=...",
  "type": "offer",
  "agent_id": "uuid",
  "call_id": "uuid"
}
```

**Response:**
```json
{
  "sdp": "v=0\r\no=...",
  "type": "answer",
  "call_id": "uuid"
}
```

### `POST /api/webrtc/candidate`

Exchange ICE candidates.

**Request:**
```json
{
  "call_id": "uuid",
  "candidate": "candidate:1 1 udp ...",
  "sdpMid": "0",
  "sdpMLineIndex": 0
}
```

### `POST /api/webrtc/hangup`

End a WebRTC session.

**Request:**
```json
{"call_id": "uuid"}
```

### `GET /api/webrtc/sessions`

List active WebRTC sessions.

**Response:**
```json
[
  {"call_id": "uuid", "agent_id": "uuid", "duration": 125}
]
```

### `POST /api/webrtc/bridge`

Bridge a WebRTC session to a SIPREC call (agent joins an observed call).

**Request:**
```json
{
  "sdp": "v=0\r\no=...",
  "type": "offer",
  "agent_id": "uuid",
  "siprec_call_id": "uuid"
}
```

**Response:**
```json
{
  "sdp": "v=0\r\no=...",
  "type": "answer",
  "call_id": "uuid"
}
```

---

## Call History & Reports

### `GET /api/calls`

List recent calls (last 50).

**Response:**
```json
[
  {
    "id": "uuid",
    "callerNumber": "+15559990001",
    "calledNumber": "1000",
    "mode": "interactive",
    "status": "completed",
    "duration": 312,
    "sentiment": "positive",
    "summary": "Billing inquiry resolved",
    "startTime": "2026-06-06T14:32:00Z"
  }
]
```

### `GET /api/calls/active`

Count of active sessions by type.

**Response:**
```json
{
  "interactive": 3,
  "copilot": 2,
  "total": 5
}
```

### `GET /api/reports/calls`

Call volume analytics.

**Query parameters:** `?days=7`

**Response:** Array of call volume data grouped by hour/day.

### `GET /api/reports/agents`

Agent performance metrics.

**Query parameters:** `?days=7`

**Response:** Array of per-agent metrics (calls handled, avg duration, sentiment scores).

### `GET /api/reports/sentiment`

Sentiment trend data.

**Query parameters:** `?days=7`

**Response:** Array of sentiment distribution over time.

### `GET /api/stats`

Dashboard summary statistics.

**Response:**
```json
{
  "activeCalls": 5,
  "totalToday": 142,
  "avgDuration": 245,
  "sentimentBreakdown": {"positive": 65, "neutral": 52, "negative": 25}
}
```

---

## Recordings

### `GET /api/recordings`

List recorded call files.

**Response:**
```json
[
  {
    "id": "uuid",
    "call_id": "uuid",
    "duration": 180,
    "size": 1440000,
    "created_at": "2026-06-06T14:32:00Z",
    "url": "/api/recordings/uuid"
  }
]
```

### `GET /api/recordings/{id}`

Download a recording file. Returns binary audio (`audio/wav`).

---

## Documents & RAG

### `GET /api/documents`

List indexed knowledge base documents.

**Response:**
```json
[
  {
    "id": "doc_123",
    "name": "Insurance Policy v4.2",
    "type": "txt",
    "size": 2400,
    "category": "policy",
    "chunks": 5,
    "status": "indexed"
  }
]
```

### `POST /api/documents`

Upload and index a document in ChromaDB.

**Request:**
```json
{
  "name": "Insurance Policy",
  "category": "policy",
  "content": "Section 4.2.1: Water damage from burst pipes is covered..."
}
```

| Field | Required | Values | Description |
|-------|----------|--------|-------------|
| `name` | Yes | | Document name |
| `category` | Yes | `policy`, `faq`, `product`, `procedure` | Category for filtering |
| `content` | Yes | | Full text content |

**Response (201):**
```json
{"id": "doc_123", "name": "Insurance Policy", "chunks": 5, "status": "indexed"}
```

### `POST /api/documents/search`

RAG vector search against indexed documents.

**Request:**
```json
{
  "query": "water damage coverage deductible",
  "top_k": 3
}
```

**Response:**
```json
[
  {
    "text": "Section 4.2.1: Water damage from burst pipes is covered...",
    "doc_name": "Insurance Policy",
    "score": 0.95,
    "chunk_index": 0
  }
]
```

---

## LLM Configuration

### `GET /api/llm/configs`

List configured LLM models.

**Response:**
```json
[
  {
    "id": "l1",
    "name": "Claude Haiku",
    "provider": "anthropic-vertex",
    "model": "claude-3-5-haiku@20241022",
    "region": "us-east5",
    "isDefault": true,
    "maxTokens": 512
  },
  {
    "id": "l2",
    "name": "Gemini Flash",
    "provider": "gemini-vertex",
    "model": "gemini-2.0-flash",
    "region": "us-central1",
    "isDefault": false,
    "maxTokens": 1024
  }
]
```

### `POST /api/llm/configs`

Add a new LLM configuration.

**Request:**
```json
{
  "name": "Gemini Pro",
  "provider": "gemini-vertex",
  "model": "gemini-1.5-pro",
  "region": "us-central1"
}
```

### `POST /api/llm/test`

Test an LLM model with a sample prompt.

**Request:**
```json
{
  "provider": "anthropic-vertex",
  "model": "claude-3-5-haiku@20241022",
  "prompt": "What is SIP in telecom?"
}
```

**Response:**
```json
{
  "response": "SIP (Session Initiation Protocol) is a signaling protocol...",
  "latency": 1451,
  "model": "claude:claude-3-5-haiku@20241022"
}
```

---

## Supervisor

### `POST /api/supervisor/monitor`

Start supervisor monitoring of an active call.

**Request:**
```json
{
  "call_id": "uuid",
  "mode": "listen"
}
```

| Mode | Description |
|------|-------------|
| `listen` | Silent monitoring (supervisor hears both parties) |
| `whisper` | Supervisor speaks to agent only |
| `barge` | Supervisor joins the conversation |

### `POST /api/supervisor/stop`

Stop supervisor monitoring session.

**Request:**
```json
{"call_id": "uuid"}
```

### `GET /api/supervisor/calls`

List calls available for supervisor monitoring.

**Response:** Array of active calls with agent, caller, duration, and queue info.

---

## Robocall Detection

### `GET /api/blocklist`

List all blocked phone numbers.

**Response:**
```json
[{"number": "15551234567", "reason": "known_robocaller"}]
```

### `POST /api/blocklist`

Add a number to the blocklist.

**Request:**
```json
{"number": "+15551234567", "reason": "known_robocaller"}
```

### `DELETE /api/blocklist`

Remove a number from the blocklist.

**Request:**
```json
{"number": "+15551234567"}
```

### `GET /api/robocall/stats`

Robocall detection metrics.

**Response:**
```json
{
  "blocklist_size": 12,
  "threshold": 0.7,
  "auto_block": false,
  "total_calls_today": 142,
  "robocalls_detected": 23,
  "blocked": 8,
  "uncertain": 15
}
```

### `POST /api/robocall/test`

Test robocall classification on sample text.

**Request:**
```json
{
  "text": "We have been trying to reach you about your auto warranty. Press 1.",
  "number": "+15559999999"
}
```

**Response:**
```json
{
  "blocklist": {"status": "not_found"},
  "keyword": {
    "score": 1.0,
    "category": "robocall",
    "reason": "keyword_match",
    "keywords": ["press 1", "auto warranty", "we have been trying to reach you"]
  }
}
```

---

## Security & PII

### `POST /api/security/pii/test`

Test PII masking on text. Detects: `credit_card`, `ssn`, `ssn_spoken`, `credit_card_spoken`, `cvv`, `dob`, `dob_compact`, `dob_spoken`, `account_number`.

**Request:**
```json
{"text": "My credit card is 4111 1111 1111 1111 and SSN is 123-45-6789"}
```

**Response:**
```json
{
  "original": "My credit card is 4111 1111 1111 1111 and SSN is 123-45-6789",
  "masked": "My credit card is XXXX-XXXX-XXXX-#### and SSN is XXX-XX-####",
  "pii_found": true,
  "detections": [
    {"type": "credit_card", "level": "critical", "masked": "XXXX-XXXX-XXXX-####"},
    {"type": "ssn", "level": "critical", "masked": "XXX-XX-####"}
  ]
}
```

### `GET /api/security/pii/config`

View PII masking configuration.

### `POST /api/security/pii/config`

Enable/disable PII masking.

**Request:**
```json
{"enabled": true}
```

### `GET /api/security/rules/pii`

List PII redaction rules.

**Response:**
```json
[
  {
    "id": "uuid",
    "name": "credit_card",
    "regex": "\\b\\d{4}[- ]?\\d{4}[- ]?\\d{4}[- ]?\\d{4}\\b",
    "mask": "XXXX-XXXX-XXXX-####",
    "level": "critical",
    "enabled": true,
    "is_default": true
  }
]
```

### `POST /api/security/rules/pii`

Add a custom PII redaction rule.

**Request:**
```json
{
  "name": "employee_id",
  "regex": "\\bEMP-\\d{6}\\b",
  "mask": "EMP-XXXXXX",
  "level": "high"
}
```

### `DELETE /api/security/rules/pii`

Remove a custom PII rule (default rules cannot be deleted).

### `GET /api/security/rules/robocall`

List robocall detection rules (keyword patterns and weights).

**Response:**
```json
[
  {
    "id": "uuid",
    "phrase": "press 1",
    "weight": 0.8,
    "category": "robocall",
    "enabled": true,
    "is_default": true
  }
]
```

### `POST /api/security/rules/robocall`

Add a custom robocall detection phrase.

**Request:**
```json
{"phrase": "your account has been compromised", "weight": 0.9, "category": "scam"}
```

### `GET /api/security/rules/biometric`

Get biometric verification configuration.

### `POST /api/security/rules/biometric`

Update biometric config key-value pair.

**Request:**
```json
{"key": "threshold", "value": "0.85"}
```

### `GET /api/security/voiceprints`

List enrolled voice prints.

**Response:**
```json
[
  {
    "id": "vp_abc123",
    "label": "fraud_profile_001",
    "type": "fraud",
    "features_dim": 32,
    "created_at": "2026-06-06T12:00:00Z"
  }
]
```

### `POST /api/security/voiceprints`

Enroll a new voice print.

**Request:**
```json
{"label": "fraud_profile_001", "type": "fraud"}
```

| `type` | Description |
|--------|-------------|
| `fraud` | Known fraud voice profile |
| `verified` | Verified account holder |

---

## Self-Service Actions

### `GET /api/actions/webhooks`

List configured action webhook URLs.

**Response:**
```json
{
  "reschedule": "https://crm.example.com/api",
  "cancel": "",
  "check_status": "",
  "update_info": ""
}
```

### `POST /api/actions/webhooks`

Configure action webhook URLs.

**Request:**
```json
{
  "reschedule": "https://crm.example.com/api/deliveries",
  "cancel": "https://crm.example.com/api/subscriptions"
}
```

### `POST /api/actions/test`

Test action parsing from LLM response text.

**Request:**
```json
{"text": "{\"type\":\"api_call\",\"intent\":\"reschedule\",\"text\":\"Done.\",\"api_call\":{\"endpoint\":\"/deliveries\",\"method\":\"PUT\",\"payload\":{\"date\":\"2026-06-12\"}}}"}
```

**Response:**
```json
{
  "type": "api_call",
  "text": "Done.",
  "intent": "reschedule",
  "api_call": {"endpoint": "/deliveries", "method": "PUT", "payload": {"date": "2026-06-12"}},
  "confidence": 0.9
}
```

---

## Webhooks

### `GET /api/webhooks`

List configured event webhooks.

**Response:**
```json
[
  {
    "id": "uuid",
    "name": "CRM Sync",
    "url": "https://crm.example.com/hook",
    "method": "POST",
    "auth_type": "bearer",
    "events": ["call.ended", "summary.generated"],
    "enabled": true,
    "retry_count": 3,
    "created_at": "2026-06-06T12:00:00Z"
  }
]
```

### `POST /api/webhooks`

Create a new webhook.

**Request:**
```json
{
  "name": "CRM Sync",
  "url": "https://crm.example.com/hook",
  "method": "POST",
  "auth_type": "bearer",
  "auth_value": "secret-token",
  "events": ["call.ended", "summary.generated"],
  "enabled": true,
  "retry_count": 3
}
```

### `DELETE /api/webhooks`

Delete a webhook.

**Request:**
```json
{"id": "uuid"}
```

### `POST /api/webhooks/test`

Send a test event to a webhook.

**Request:**
```json
{"webhook_id": "uuid"}
```

### `GET /api/webhooks/logs`

View webhook execution logs.

**Response:**
```json
[
  {
    "id": "uuid",
    "event_type": "call.ended",
    "call_id": "uuid",
    "status_code": 200,
    "error": "",
    "sent_at": "2026-06-06T14:32:00Z",
    "webhook_name": "CRM Sync"
  }
]
```

---

## SIP Trunks

### `GET /api/trunks`

List SIP trunk configurations.

**Response:**
```json
[
  {
    "id": "uuid",
    "name": "Primary SBC",
    "provider": "twilio",
    "address": "sbc.example.com",
    "port": 5060,
    "transport": "udp",
    "register": false,
    "caller_id": "+15559876543",
    "codecs": "PCMU,PCMA,opus",
    "status": "active",
    "trunk_type": "sip",
    "tls_enabled": false,
    "srtp_enabled": false,
    "created_at": "2026-06-06T12:00:00Z"
  }
]
```

### `POST /api/trunks`

Create a SIP trunk.

**Request:**
```json
{
  "name": "Primary SBC",
  "provider": "twilio",
  "address": "sbc.example.com",
  "port": 5060,
  "transport": "udp",
  "register": false,
  "username": "user",
  "password": "pass",
  "caller_id": "+15559876543",
  "codecs": "PCMU,PCMA",
  "status": "active"
}
```

### `DELETE /api/trunks`

Delete a SIP trunk.

**Request:**
```json
{"id": "uuid"}
```

### `POST /api/trunks/test`

Test trunk connectivity (SIP OPTIONS ping).

**Request:**
```json
{"address": "sbc.example.com", "port": 5060}
```

**Response:**
```json
{
  "address": "sbc.example.com",
  "reachable": true,
  "response": "200 OK"
}
```

### `POST /api/trunks/apply`

Apply a trunk configuration (activate it for routing).

**Request:**
```json
{"id": "uuid"}
```

### `GET /api/trunks/acl`

List trunk access control lists.

**Response:**
```json
[
  {
    "id": "uuid",
    "trunk_id": "uuid",
    "ip_address": "203.0.113.10",
    "cidr_bits": 32,
    "description": "SBC primary"
  }
]
```

### `POST /api/trunks/acl`

Add an ACL entry.

**Request:**
```json
{
  "trunk_id": "uuid",
  "ip_address": "203.0.113.10",
  "cidr_bits": 32,
  "description": "SBC primary"
}
```

### `DELETE /api/trunks/acl`

Remove an ACL entry.

**Request:**
```json
{"id": "uuid"}
```

### `GET /api/trunks/security-log`

Trunk security audit log.

**Response:**
```json
[
  {
    "id": 1,
    "event_type": "auth_failure",
    "trunk_name": "Primary SBC",
    "source_ip": "203.0.113.99",
    "call_id": "uuid",
    "details": "Invalid credentials",
    "created_at": "2026-06-06T14:32:00Z"
  }
]
```

---

## IVR

### `GET /api/ivr`

List IVR flows.

**Response:**
```json
[
  {
    "id": "uuid",
    "name": "Main Menu",
    "description": "Primary call tree",
    "enabled": true,
    "created_at": "2026-06-06T12:00:00Z"
  }
]
```

### `GET /api/ivr?id=<uuid>`

Get a single IVR flow with node graph.

**Response:**
```json
{
  "id": "uuid",
  "name": "Main Menu",
  "description": "Primary call tree",
  "entry": "welcome",
  "nodes": {
    "welcome": {
      "type": "prompt",
      "prompt": "Welcome to Acme Corp. Press 1 for billing, 2 for support.",
      "timeout_ms": 5000,
      "retries": 2,
      "dtmf_map": {"1": "billing", "2": "support"},
      "timeout_node": "repeat"
    },
    "billing": {
      "type": "route",
      "destination_type": "queue",
      "destination_value": "billing"
    }
  },
  "enabled": true
}
```

### `POST /api/ivr`

Create a new IVR flow.

**Request:**
```json
{
  "name": "After Hours",
  "description": "After-hours menu",
  "flow_data": {}
}
```

---

## Tenants

Multi-tenant management for hosted deployments.

### `GET /api/tenants`

List tenants.

**Response:**
```json
[
  {
    "id": "uuid",
    "name": "Acme Corp",
    "domain": "acme.example.com",
    "max_agents": 50,
    "max_concurrent_calls": 20,
    "enabled": true,
    "agents": 12,
    "active_calls": 3,
    "created_at": "2026-06-06T12:00:00Z"
  }
]
```

### `POST /api/tenants`

Create a tenant.

**Request:**
```json
{
  "name": "Acme Corp",
  "domain": "acme.example.com",
  "max_agents": 50,
  "max_concurrent_calls": 20
}
```

### `POST /api/tenants/users`

Assign a user to a tenant.

**Request:**
```json
{"user_id": "uuid", "tenant_id": "uuid"}
```

---

## Settings & Configuration

### `GET /api/settings`

Get all system settings.

**Response:**
```json
{
  "llm": {
    "provider": "anthropic-vertex",
    "model": "claude-3-5-haiku@20241022",
    "region": "us-east5",
    "max_tokens": 512
  },
  "prompts": {
    "interactive": "You are a helpful call center agent...",
    "copilot": "You are observing a live call..."
  },
  "trunks": [
    {
      "name": "Primary SBC",
      "address": "sbc.example.com",
      "port": 5060,
      "transport": "udp",
      "status": "active"
    }
  ]
}
```

### `GET /api/settings/llm` | `POST /api/settings/llm`

Get or update LLM provider settings.

**POST Request:**
```json
{
  "provider": "anthropic-vertex",
  "model": "claude-3-5-haiku@20241022",
  "region": "us-east5",
  "max_tokens": 512
}
```

### `GET /api/settings/prompts` | `POST /api/settings/prompts`

Get or update system prompt templates.

**POST Request:**
```json
{
  "interactive": "You are a helpful call center agent...",
  "copilot": "You are observing a live call..."
}
```

### `GET /api/settings/trunks` | `POST /api/settings/trunks`

Get or update trunk settings (via config store, separate from `/api/trunks` CRUD).

### `GET /api/config`

Gateway configuration and deployment mode. No auth required.

**Response:**
```json
{
  "mode": "standalone",
  "sip_listen": ":5062",
  "stt_url": "http://whisper:8000/v1/audio/transcriptions",
  "tts_url": "http://piper:5000",
  "claude_model": "claude-3-5-haiku@20241022"
}
```

| `mode` | Description |
|--------|-------------|
| `standalone` | Gateway acts as SIP B2BUA directly |
| `gateway` | Full stack with FreeSWITCH |

---

## Health & Infrastructure

### `GET /healthz`

Health check. No auth required.

**Response:**
```json
{"status": "ok", "sessions": 3, "mode": "standalone"}
```

### `GET /metrics`

Prometheus metrics endpoint. No auth required.

### `GET /api/services/status`

Health probes for all dependent services. No auth required.

**Response:** Status of Whisper, Piper, PostgreSQL, Redis, ChromaDB, and FreeSWITCH (if applicable).

### `GET /api/scale/status`

STT/TTS worker pool and admission controller metrics. No auth required.

### `GET /api/failover/status`

Circuit breaker health for all upstream services.

**Response:**
```json
{
  "llm": {"state": "closed", "failures": 0},
  "stt": {"state": "closed", "failures": 0},
  "tts": {"state": "closed", "failures": 0},
  "esl": {"state": "closed", "failures": 0}
}
```

Circuit states: `closed` (healthy), `open` (tripped — failing fast), `half-open` (probing recovery).

### `GET /api/copilot/active`

Count of active SIPREC co-pilot sessions.

### `POST /api/dtmf/test`

Test DTMF digit parsing.

**Request:**
```json
{"text": "482910"}
```

**Response:**
```json
{"input": "482910", "parsed": "User typed: 482910"}
```

---

## Error Responses

All endpoints return errors in a consistent format:

```json
{"error": "description of the problem"}
```

| HTTP Code | Meaning |
|-----------|---------|
| 400 | Bad request — invalid JSON or missing required fields |
| 401 | Unauthorized — missing or invalid token/API key |
| 403 | Forbidden — insufficient role permissions |
| 404 | Not found — session or resource doesn't exist |
| 405 | Method not allowed |
| 409 | Conflict — resource already exists |
| 500 | Internal server error |
| 502 | Bad gateway — upstream service (LLM, STT, TTS) failed |
| 503 | Service unavailable — circuit breaker open |
