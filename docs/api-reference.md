# API Reference

Complete API documentation for the VoiceAgent Telecom-Native AI Gateway.

**Base URL:** `http://localhost:8080`

---

## Voice Endpoints

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
| Text | `{"event":"response","text":"..."}` | Claude's response text |
| Text | `{"event":"robocall","text":"score=80%..."}` | Robocall detection alert |
| Text | `{"event":"pii_masked","text":"Detected 2..."}` | PII masking notification |
| Text | `{"event":"action","intent":"..."}` | Self-service action executed |
| Text | `{"event":"transfer","reason":"..."}` | Call transfer initiated |

**Pipeline:** Audio → VAD (500ms silence) → Whisper STT → PII masking → robocall check → Claude (action-aware) → Piper TTS → playback via ESL

---

### `WebSocket /siprec` — Co-Pilot Agent Assist

Passive observation of a two-party call. FreeSWITCH sends two separate WebSocket connections (one per audio leg).

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

**Behavior:** Both legs are transcribed independently with speaker labels. Claude coach mode generates real-time suggestions. On hangup, a call summary is generated and POSTed to the CRM webhook.

---

### `GET /siprec/events` — SSE Event Stream

Server-Sent Events endpoint for the agent dashboard. Subscribe to receive real-time transcripts, suggestions, and call summaries.

**Connection:**
```
curl -N http://localhost:8080/siprec/events?call_id=<uuid>
```

| Parameter | Required | Description |
|-----------|----------|-------------|
| `call_id` | Yes | Call to monitor |

**Event types:**

```json
{"type":"transcript","speaker":"customer","text":"I need help with my claim"}

{"type":"transcript","speaker":"agent","text":"Let me look that up for you"}

{"type":"suggestion","suggestion":"Policy 4.2.1 covers burst pipe damage. $500 deductible.","category":"answer","confidence":0.95,"context":"water damage"}

{"type":"robocall","text":"score=80% keywords=[press 1, auto warranty]"}

{"type":"pii_masked","text":"Detected 2 PII items: credit_card, cvv"}

{"type":"action","intent":"reschedule","status":"success"}

{"type":"transfer","reason":"angry","department":"retention","priority":"urgent","summary":"Customer upset about billing"}

{"type":"summary","summary":"Customer called about water damage claim.","action_items":["File claim within 30 days","Schedule adjuster visit"],"commitments":["Callback within 24 hours"],"sentiment":"neutral","duration":180,"voice_sentiment":{"avg_energy":245.3,"energy_trend":"rising","avg_pitch_hz":185.0,"pitch_variance":32.5,"speaking_rate_wpm":142.0,"silence_ratio":0.35,"agitation":0.45,"engagement":0.65,"frustration":0.38,"sentiment":"neutral","confidence":0.6}}
```

---

### `POST /call` — Outbound Call Origination

Originates an outbound call via FreeSWITCH ESL.

**Request:**
```json
POST /call
Content-Type: application/json

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

**Response (502):**
```json
{"status": "error", "error": "GATEWAY_DOWN"}
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
    "status": "available",
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

---

## Call History

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

Active session counts.

**Response:**
```json
{
  "interactive": 3,
  "copilot": 2,
  "total": 5
}
```

---

## Documents & RAG

### `GET /api/documents`

List indexed documents.

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

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Document name |
| `category` | Yes | `policy`, `faq`, `product`, `procedure` |
| `content` | Yes | Full text content |

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

## Robocall Detection

### `GET /api/blocklist`

List all blocked numbers.

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

Test robocall classification on text.

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

## Security

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

| Field | Values | Description |
|-------|--------|-------------|
| `type` | `fraud` or `verified` | Fraud profile or verified account holder |

### `POST /api/security/pii/test`

Test PII masking on text. Supports 9 detection patterns: `credit_card`, `ssn`, `ssn_spoken`, `credit_card_spoken`, `cvv`, `dob`, `dob_compact`, `dob_spoken`, `account_number`.

**Request:**
```json
{"text": "My credit card is 4111 1111 1111 1111 and SSN is 123-45-6789 dob 16121968"}
```

**Response:**
```json
{
  "original": "My credit card is 4111 1111 1111 1111 and SSN is 123-45-6789 dob 16121968",
  "masked": "My credit card is XXXX-XXXX-XXXX-#### and SSN is XXX-XX-#### DOB [REDACTED]",
  "pii_found": true,
  "detections": [
    {"type": "credit_card", "level": "critical", "masked": "XXXX-XXXX-XXXX-####"},
    {"type": "ssn", "level": "critical", "masked": "XXX-XX-####"},
    {"type": "dob_compact", "level": "high", "masked": "DOB [REDACTED]"}
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

## DTMF

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

## Infrastructure

### `GET /api/failover/status`

Circuit breaker health for all services.

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

### `GET /api/stats`

Dashboard statistics.

**Response:**
```json
{
  "activeCalls": 5,
  "totalToday": 142,
  "avgDuration": 245,
  "sentimentBreakdown": {"positive": 65, "neutral": 52, "negative": 25}
}
```

### `GET /healthz`

Health check.

**Response:**
```json
{"status": "ok", "sessions": 3, "mode": "gateway"}
```

### `GET /api/copilot/active`

Returns active co-pilot (SIPREC) sessions with real-time voice sentiment and caller/agent identity.

**Response:**
```json
[
  {
    "call_id": "7d698d99-84d8-4c6c-93bc-86aed21ed91e",
    "started_at": "2026-06-07T21:33:30Z",
    "duration": 45,
    "caller": "+15551234567",
    "agent": "agent1",
    "voice_sentiment": {
      "avg_energy": 245.3,
      "energy_trend": "rising",
      "avg_pitch_hz": 185.0,
      "pitch_variance": 32.5,
      "speaking_rate_wpm": 142.0,
      "silence_ratio": 0.35,
      "agitation": 0.45,
      "engagement": 0.65,
      "frustration": 0.38,
      "sentiment": "neutral",
      "confidence": 0.6
    }
  }
]
```

### `GET /api/config`

Returns current gateway configuration and deployment mode.

**Response:**
```json
{
  "mode": "standalone",
  "sip_listen": ":5060",
  "stt_url": "http://whisper:8000/v1/audio/transcriptions",
  "tts_url": "http://piper:5000",
  "claude_model": "claude-3-5-haiku@20241022"
}
```

**Mode values:**
- `standalone` — Native SIP SIPREC helper (no FreeSWITCH)
- `gateway` — Full B2BUA with FreeSWITCH (default)

---

## Error Responses

All endpoints return errors in a consistent format:

```json
{"status": "error", "error": "description of the problem"}
```

| HTTP Code | Meaning |
|-----------|---------|
| 400 | Bad request — invalid JSON or missing required fields |
| 404 | Not found — session or resource doesn't exist |
| 405 | Method not allowed |
| 500 | Internal server error |
| 502 | Bad gateway — upstream service (LLM, STT, TTS, ESL) failed |
