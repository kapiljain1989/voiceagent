# Phase 2: Call Control — Requirements Document

## Objective

Give agents full call control capabilities from the Console — outbound dialing, call transfers, conference calls, and hold/resume. These are the core productivity features that make the Console a real agent desktop.

---

## 1. Outbound Calling

### What
Agent initiates a call to an external number through a configured SIP trunk.

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| OB-1 | Agent dials a number from Console dial pad | Must |
| OB-2 | Gateway sends SIP INVITE through outbound trunk | Must |
| OB-3 | Trunk selection — use the trunk configured for outbound | Must |
| OB-4 | Caller ID — use trunk's caller_id or agent's extension | Must |
| OB-5 | Call progress tones — agent hears ringback via WebRTC | Should |
| OB-6 | Called party answers → bidirectional audio (RTP ↔ WebRTC) | Must |
| OB-7 | Called party busy/no answer → appropriate error on Console | Must |
| OB-8 | Outbound call gets copilot (transcription, coaching, sentiment) | Must |
| OB-9 | Call history records outbound calls with direction=outbound | Must |
| OB-10 | Click-to-call from call history or contact | Should |

### Call Flow
```
Agent clicks Dial on Console
  → Console sends POST /api/call/outbound {number, trunk_id}
  → Gateway selects outbound trunk
  → Gateway sends SIP INVITE to trunk SBC
    → INVITE: From=agent_caller_id, To=dialed_number
  → SBC routes to PSTN/destination
  → Called party rings → agent hears ringback via WebRTC
  → Called party answers → 200 OK → RTP flows
  → Gateway bridges: RTP (called) ↔ WebRTC (agent)
  → Copilot pipeline runs on both legs
  → Either party hangs up → BYE → cleanup
```

### API
```
POST /api/call/outbound
{
  "number": "+15551234567",
  "trunk_id": "optional — uses default outbound trunk",
  "caller_id": "optional — overrides trunk default",
  "agent_id": "agent UUID"
}

Response:
{
  "call_id": "outbound-12345",
  "status": "dialing"
}
```

### Data Model
```sql
-- Mark trunk as inbound/outbound/both
ALTER TABLE sip_trunks ADD COLUMN IF NOT EXISTS direction TEXT DEFAULT 'both';
-- Values: inbound, outbound, both

-- Call history direction
ALTER TABLE calls ADD COLUMN IF NOT EXISTS direction TEXT DEFAULT 'inbound';
-- Values: inbound, outbound
```

---

## 2. Call Transfer

### What
Agent transfers an active call to another agent, queue, or external number.

### Types

| Type | Description | Use Case |
|------|-------------|----------|
| **Blind** | Caller moved directly, agent drops | "Let me transfer you to Billing" |
| **Warm** | Agent stays on, introduces caller to target, then drops | "I have a customer who needs help with..." |
| **Queue** | Caller moved to a different queue | "I'll transfer you to our Sales team" |

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| TR-1 | Blind transfer to agent extension | Must |
| TR-2 | Blind transfer to external number (via trunk) | Must |
| TR-3 | Blind transfer to queue | Must |
| TR-4 | Warm transfer — agent conferences with target before dropping | Should |
| TR-5 | Transfer UI — search agents/queues, enter external number | Must |
| TR-6 | Transfer preserves call history and copilot context | Must |
| TR-7 | Original agent's call ends after transfer completes | Must |
| TR-8 | If target agent unavailable → return to original agent or queue | Should |

### Call Flow — Blind Transfer
```
Agent clicks Transfer → selects target
  → POST /api/call/transfer {call_id, target_type, target_value}
  → Gateway:
    If target=agent:
      → Ring target agent via SSE
      → Target accepts → bridge caller RTP to target WebRTC
      → Original agent disconnected
    If target=queue:
      → Move caller to target queue
      → ACD assigns new agent
      → Original agent disconnected
    If target=external:
      → Send new INVITE via trunk to external number
      → External answers → bridge caller RTP to external RTP
      → Original agent disconnected
```

### API
```
POST /api/call/transfer
{
  "call_id": "current call ID",
  "transfer_type": "blind",        // blind, warm
  "target_type": "agent",          // agent, queue, external
  "target_value": "agent-uuid or queue-name or +15551234567",
  "agent_id": "current agent UUID"
}

Response:
{
  "status": "transferring",
  "transfer_call_id": "transfer-12345"
}
```

---

## 3. Conference (3-Way Calling)

### What
Agent adds a third party to the active call — all three can hear each other.

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| CF-1 | Agent adds another agent to the call | Must |
| CF-2 | Agent adds an external number to the call | Should |
| CF-3 | All parties hear each other (audio mixing) | Must |
| CF-4 | Agent can drop the third party | Must |
| CF-5 | Agent can leave, leaving the other two connected | Should |
| CF-6 | Conference UI shows all participants | Must |
| CF-7 | Copilot runs on the full conference audio | Should |

### Call Flow
```
Agent on call with Caller
  → Agent clicks Conference → selects target
  → Gateway:
    If target=agent:
      → Ring target agent → target accepts
      → Mix audio: Caller RTP + Agent1 WebRTC + Agent2 WebRTC
    If target=external:
      → INVITE via trunk → external answers
      → Mix audio: Caller RTP + Agent WebRTC + External RTP
  → Agent clicks "Drop 3rd" → disconnect third party
  → Or Agent clicks "Leave" → disconnect self, other two stay
```

### Audio Mixing
```
Three audio sources:
  A: Caller (RTP)
  B: Agent (WebRTC)
  C: Third party (WebRTC or RTP)

Each party hears the other two mixed:
  A hears: mix(B, C)
  B hears: mix(A, C)
  C hears: mix(A, B)

Mixing: add PCM samples, divide by 2, clamp to int16 range
```

### API
```
POST /api/call/conference
{
  "call_id": "current call ID",
  "target_type": "agent",          // agent, external
  "target_value": "agent-uuid or +15551234567",
  "agent_id": "current agent UUID"
}

POST /api/call/conference/drop
{
  "call_id": "current call ID",
  "participant": "third-party-id"
}
```

---

## 4. Hold / Resume

### What
Agent puts the caller on hold (hears music/silence) and resumes.

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| HD-1 | Agent clicks Hold → caller hears hold music/silence | Must |
| HD-2 | Agent clicks Resume → audio bridge resumes | Must |
| HD-3 | Hold/Resume UI button toggles state | Must |
| HD-4 | Hold music via TTS or pre-recorded audio | Should |
| HD-5 | Agent can still see copilot/transcript while on hold | Must |
| HD-6 | Multiple holds per call supported | Should |
| HD-7 | Hold duration tracked in call record | Should |

### Call Flow
```
Agent clicks Hold:
  → Stop forwarding caller audio to agent WebRTC
  → Start sending hold music to caller via RTP
  → Console shows "ON HOLD" state

Agent clicks Resume:
  → Stop hold music
  → Resume audio bridge (caller ↔ agent)
  → Console shows "CONNECTED" state
```

### API
```
POST /api/call/hold
{
  "call_id": "current call ID",
  "agent_id": "agent UUID"
}

POST /api/call/resume
{
  "call_id": "current call ID",
  "agent_id": "agent UUID"
}
```

---

## 5. Implementation Priority

| Priority | Feature | Complexity | Depends On |
|----------|---------|-----------|------------|
| 1 | Hold / Resume | Low | Existing RTP bridge |
| 2 | Blind Transfer to Queue | Medium | Existing queue + ACD |
| 3 | Blind Transfer to Agent | Medium | ACD ring notification |
| 4 | Outbound Calling | High | SIP UAC (new), trunk outbound |
| 5 | Blind Transfer to External | High | Outbound calling |
| 6 | Warm Transfer | High | Conference mixing |
| 7 | Conference (3-Way) | High | Audio mixing engine |

---

## 6. Integration Points

### Existing Code to Extend

| File | Extension |
|------|-----------|
| `gateway/webrtc.go` | Hold/resume: pause/resume audio tap. Transfer: disconnect + reconnect |
| `gateway/sipserver.go` | Outbound: SIP UAC sending INVITE. Transfer: re-INVITE or new INVITE |
| `gateway/acd.go` | Transfer-to-queue: re-enqueue caller. Transfer-to-agent: direct ring |
| `gateway/queue.go` | Transfer: move caller between queues |
| `gateway/rtplistener.go` | Hold music playback. Conference: audio mixing |
| `gateway/queue_announcements.go` | Hold music reuse (TTS or tone generation) |
| `ui/src/app/console/page.tsx` | Transfer panel, conference UI, hold button |

### New Files

| File | Purpose |
|------|---------|
| `gateway/call_control.go` | Hold/resume, transfer, conference API handlers |
| `gateway/sip_uac.go` | SIP User Agent Client — sends outbound INVITE |
| `gateway/audio_mixer.go` | PCM audio mixing for conference calls |

---

## 7. Acceptance Criteria

### Outbound
- [ ] Agent dials +15551234567 from Console
- [ ] Called party rings, agent hears ringback
- [ ] Called party answers → two-way audio
- [ ] Copilot runs on outbound call
- [ ] Call recorded in history as direction=outbound

### Transfer
- [ ] Agent transfers to Billing queue → caller hears Billing welcome
- [ ] Agent transfers to another agent → target agent gets ring
- [ ] Original agent's call ends after transfer

### Conference
- [ ] Agent adds third party → all three hear each other
- [ ] Agent drops third party → back to two-party call

### Hold
- [ ] Agent holds → caller hears silence/music
- [ ] Agent resumes → audio flows again
- [ ] Multiple hold/resume cycles work

---

## 8. Test Plan

```bash
# Hold/Resume
./test-local.sh call    # accept on Console
# Click HOLD → verify caller stops hearing agent
# Click RESUME → verify audio resumes

# Transfer to Queue
# Accept a Support call → click Transfer → select "Billing"
# Verify call moves to Billing queue, new announcement plays

# Outbound
# Console dial pad → enter +15551234567 → DIAL
# Verify SIP INVITE goes through trunk to test SBC
# Test SBC answers → audio flows

# Conference
# On active call → click Conference → select another agent
# Verify 3-way audio mixing
```
