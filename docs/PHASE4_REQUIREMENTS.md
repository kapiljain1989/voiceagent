# Phase 4: Enterprise Features — Requirements Document

## Objective

Production-grade enterprise capabilities: IVR for self-service, call recording for compliance, CRM integration for workflow automation, reporting for business intelligence, and multi-tenant isolation for SaaS deployment.

---

## 1. IVR Builder (AI-Powered)

### What
Configurable Interactive Voice Response system that greets callers, collects input (DTMF or speech), and routes based on rules or AI intent detection.

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| IV-1 | IVR flow editor — visual or JSON-based tree of nodes | Must |
| IV-2 | Node types: play prompt, collect DTMF, collect speech, route, transfer, hangup | Must |
| IV-3 | TTS prompts via Piper (existing TTS integration) | Must |
| IV-4 | DTMF collection with timeout and retry | Must |
| IV-5 | AI intent detection — send caller speech to Claude for intent classification | Should |
| IV-6 | Route to queue, agent, or external number based on IVR result | Must |
| IV-7 | IVR assigned per DID route (extend did_routes table) | Must |
| IV-8 | IVR variables (caller number, time of day) available in routing decisions | Should |
| IV-9 | IVR analytics — drop-off rates per node, avg time in IVR | Should |

### Data Model
```sql
CREATE TABLE ivr_flows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT,
    flow_data JSONB NOT NULL,  -- tree of IVR nodes
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Link IVR to DID routes
ALTER TABLE did_routes ADD COLUMN IF NOT EXISTS ivr_id UUID REFERENCES ivr_flows(id);
```

### IVR Flow JSON Structure
```json
{
  "entry": "welcome",
  "nodes": {
    "welcome": {
      "type": "play",
      "prompt": "Thank you for calling. Press 1 for Sales, 2 for Support, or say your request.",
      "next": "collect_input"
    },
    "collect_input": {
      "type": "collect",
      "mode": "dtmf_or_speech",
      "timeout_ms": 5000,
      "retries": 2,
      "dtmf_map": {
        "1": "route_sales",
        "2": "route_support"
      },
      "speech_intent": {
        "model": "claude",
        "intents": ["sales", "support", "billing", "cancel"],
        "fallback": "route_support"
      },
      "timeout_node": "retry",
      "error_node": "route_support"
    },
    "route_sales": {
      "type": "transfer",
      "destination_type": "queue",
      "destination_value": "Sales"
    },
    "route_support": {
      "type": "transfer",
      "destination_type": "queue",
      "destination_value": "Support"
    }
  }
}
```

### API
```
GET    /api/ivr              — list all IVR flows
POST   /api/ivr              — create IVR flow
PUT    /api/ivr/:id          — update IVR flow
DELETE /api/ivr/:id          — delete IVR flow
GET    /api/ivr/:id/test     — dry-run test with simulated input
```

### Call Flow
```
Inbound call → DID route match → ivr_id set
  → Gateway runs IVR flow engine
  → Play TTS prompt via RTP (reuse queue_announcements.go pattern)
  → Wait for DTMF (existing dtmf.go detector) or speech (Whisper STT)
  → Route based on input → transfer to queue/agent
  → If AI intent: send speech text to Claude → classify → route
```

---

## 2. Call Recording + Storage

### What
Record both sides of every call for compliance, training, and quality assurance. Store recordings with metadata for search and playback.

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| CR-1 | Record caller + agent audio as WAV or MP3 | Must |
| CR-2 | Recording starts automatically on call answer | Must |
| CR-3 | Store recordings on local filesystem or S3-compatible storage | Must |
| CR-4 | Metadata: call_id, caller, agent, duration, timestamp, file path | Must |
| CR-5 | Playback API — stream recorded audio | Must |
| CR-6 | Recording consent announcement (configurable) | Should |
| CR-7 | Pause/resume recording (for PCI compliance — skip credit card input) | Should |
| CR-8 | Recording retention policy (auto-delete after N days) | Should |
| CR-9 | UI: playback in call history, download button | Must |

### Data Model
```sql
CREATE TABLE call_recordings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id TEXT NOT NULL,
    caller TEXT,
    agent TEXT,
    duration_secs INT,
    file_path TEXT NOT NULL,
    file_size_bytes BIGINT,
    format TEXT DEFAULT 'wav',
    sample_rate INT DEFAULT 16000,
    channels INT DEFAULT 2,         -- stereo: left=caller, right=agent
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Architecture
```
During call:
  Caller PCM (16kHz) → recording buffer (left channel)
  Agent PCM (16kHz)  → recording buffer (right channel)
  On call end → encode WAV (stereo) → write to disk → insert DB row

Storage path: /data/recordings/YYYY/MM/DD/{call_id}.wav
```

### API
```
GET  /api/recordings                    — list recordings with filters
GET  /api/recordings/:id               — get recording metadata
GET  /api/recordings/:id/audio         — stream audio file
POST /api/recordings/:id/pause         — pause recording (PCI)
POST /api/recordings/:id/resume        — resume recording
```

---

## 3. CRM Webhooks

### What
Send call events to external CRM systems (Salesforce, HubSpot, custom) via configurable webhooks. Enable workflow automation based on call outcomes.

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| WH-1 | Configurable webhook endpoints with URL, headers, auth | Must |
| WH-2 | Events: call_started, call_answered, call_ended, call_transferred | Must |
| WH-3 | Payload includes: caller, agent, duration, sentiment, transcript summary | Must |
| WH-4 | Retry with exponential backoff on failure | Must |
| WH-5 | Webhook test button (send sample payload) | Should |
| WH-6 | Event filtering — choose which events trigger webhook | Should |
| WH-7 | Webhook execution log with status codes | Should |
| WH-8 | Template variables in payload (customize JSON structure) | Should |

### Data Model
```sql
CREATE TABLE webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    method TEXT DEFAULT 'POST',
    headers JSONB DEFAULT '{}',
    auth_type TEXT DEFAULT 'none',     -- none, bearer, basic, api_key
    auth_value TEXT,
    events TEXT[] DEFAULT '{}',        -- call_started, call_ended, etc.
    payload_template JSONB,            -- custom payload mapping
    enabled BOOLEAN DEFAULT TRUE,
    retry_count INT DEFAULT 3,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE webhook_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID REFERENCES webhooks(id),
    event_type TEXT NOT NULL,
    call_id TEXT,
    status_code INT,
    response_body TEXT,
    error TEXT,
    sent_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Default Payload
```json
{
  "event": "call_ended",
  "call_id": "abc-123",
  "timestamp": "2026-06-20T16:00:00Z",
  "caller": "+15551234567",
  "agent": "Sarah",
  "duration_secs": 245,
  "queue": "Support",
  "sentiment": "positive",
  "frustration": 0.12,
  "summary": "Customer called about billing. Issue resolved.",
  "action_items": ["Follow up in 3 days"],
  "transcript_url": "/api/calls/abc-123/transcript"
}
```

### API
```
GET    /api/webhooks           — list webhooks
POST   /api/webhooks           — create webhook
PUT    /api/webhooks/:id       — update webhook
DELETE /api/webhooks/:id       — delete webhook
POST   /api/webhooks/:id/test  — send test payload
GET    /api/webhooks/logs      — recent webhook execution log
```

---

## 4. Reporting (Call Analytics)

### What
Analytics dashboard with call volume, agent performance, queue metrics, sentiment trends, and SLA compliance.

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| RP-1 | Call volume chart: calls per hour/day/week | Must |
| RP-2 | Agent performance: calls handled, avg duration, sentiment scores | Must |
| RP-3 | Queue metrics: avg wait time, abandonment rate, SLA % | Must |
| RP-4 | Sentiment trends: positive/negative/neutral over time | Must |
| RP-5 | Date range filter | Must |
| RP-6 | Export to CSV | Should |
| RP-7 | Scheduled email reports | Should |
| RP-8 | Custom KPI definitions | Should |

### Data Model
```sql
CREATE TABLE call_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id TEXT UNIQUE NOT NULL,
    direction TEXT DEFAULT 'inbound',    -- inbound, outbound
    caller TEXT,
    agent_id UUID REFERENCES agents(id),
    agent_name TEXT,
    queue TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    answered_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    duration_secs INT,
    wait_secs INT,
    hold_secs INT DEFAULT 0,
    sentiment TEXT,
    frustration REAL,
    summary TEXT,
    disposition TEXT,                    -- resolved, escalated, abandoned, transferred
    recording_id UUID REFERENCES call_recordings(id),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_call_records_started ON call_records(started_at);
CREATE INDEX idx_call_records_agent ON call_records(agent_id);
```

### API
```
GET /api/reports/calls       — call volume by time period
GET /api/reports/agents      — agent performance metrics
GET /api/reports/queues      — queue SLA and wait times
GET /api/reports/sentiment   — sentiment distribution over time
GET /api/reports/export      — CSV export with filters
```

---

## 5. Multi-Tenant Support

### What
Isolate data and configuration per tenant (organization) for SaaS deployment. Each tenant has their own agents, queues, trunks, DIDs, and recordings.

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| MT-1 | Tenant table with name, domain, settings | Must |
| MT-2 | All tables get tenant_id foreign key | Must |
| MT-3 | API middleware injects tenant_id from auth token | Must |
| MT-4 | Query filters enforce tenant isolation | Must |
| MT-5 | Tenant-specific SIP trunk configuration | Must |
| MT-6 | Tenant admin can manage their own agents/queues | Must |
| MT-7 | Super-admin can manage all tenants | Must |
| MT-8 | Tenant usage metering (call minutes, recordings) | Should |

### Data Model
```sql
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    domain TEXT UNIQUE,
    settings JSONB DEFAULT '{}',
    max_agents INT DEFAULT 50,
    max_concurrent_calls INT DEFAULT 20,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Add tenant_id to existing tables
ALTER TABLE users ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id);
ALTER TABLE agents ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id);
ALTER TABLE sip_trunks ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id);
ALTER TABLE did_routes ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id);
ALTER TABLE queues ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id);
```

---

## 6. Implementation Priority

| Priority | Feature | Complexity | Value |
|----------|---------|-----------|-------|
| 1 | Call Recording | Medium | High — compliance requirement |
| 2 | Reporting | Medium | High — business intelligence |
| 3 | CRM Webhooks | Medium | High — workflow automation |
| 4 | IVR Builder | High | High — self-service deflection |
| 5 | Multi-Tenant | High | Medium — needed for SaaS only |

---

## 7. Acceptance Criteria

### Call Recording
- [ ] Calls automatically recorded as stereo WAV
- [ ] Playback from call history UI
- [ ] Recording metadata searchable

### Reporting
- [ ] Call volume chart with date range picker
- [ ] Agent leaderboard with performance metrics
- [ ] Queue SLA dashboard
- [ ] CSV export

### CRM Webhooks
- [ ] Configure webhook → call ends → webhook fires with payload
- [ ] Retry on failure with backoff
- [ ] Test button sends sample payload

### IVR
- [ ] Create IVR flow → assign to DID → caller hears prompts
- [ ] DTMF routing works (press 1 for Sales)
- [ ] AI intent detection routes correctly

### Multi-Tenant
- [ ] Two tenants see only their own data
- [ ] Tenant admin can manage their agents
- [ ] Super-admin sees all

---

## 8. Test Plan

```bash
# Call Recording
# Make a call → end → check /data/recordings/ for WAV file
# GET /api/recordings → verify metadata
# GET /api/recordings/:id/audio → play back in browser

# Reporting
# Make 10+ calls → open /reports page
# Verify charts show correct data
# Export CSV and verify contents

# CRM Webhooks
# POST /api/webhooks with https://webhook.site URL
# Make a call → end → check webhook.site for payload
# Verify retry on simulated failure

# IVR
# Create IVR: "Press 1 for Sales, 2 for Support"
# Assign to DID route
# Call in → hear prompt → press 1 → land in Sales queue
# Test speech: say "I want to buy" → AI routes to Sales

# Multi-Tenant
# Create tenant A and B
# Login as tenant A admin → create agents → verify B's agents not visible
```
