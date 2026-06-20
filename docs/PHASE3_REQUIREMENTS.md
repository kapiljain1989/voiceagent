# Phase 3: Supervisor Tools — Requirements Document

## Objective

Give supervisors real-time visibility and intervention capabilities on live calls. Supervisors can monitor calls silently, coach agents via whisper, or join calls directly via barge — all from a dedicated dashboard.

---

## 1. Live Call Monitoring (Listen-Only)

### What
Supervisor silently listens to both sides of an active call without the caller or agent knowing.

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| MN-1 | Supervisor sees all active calls on a live dashboard | Must |
| MN-2 | Supervisor clicks "Monitor" to listen to a specific call | Must |
| MN-3 | Supervisor hears both caller and agent audio mixed | Must |
| MN-4 | Neither caller nor agent is notified of monitoring | Must |
| MN-5 | Supervisor can stop monitoring at any time | Must |
| MN-6 | Multiple supervisors can monitor the same call | Should |
| MN-7 | Dashboard shows caller number, agent, duration, queue, sentiment | Must |
| MN-8 | Dashboard auto-refreshes in real-time | Must |

### Call Flow
```
Supervisor opens /supervisor dashboard
  → Sees list of active calls with live metrics
  → Clicks "Monitor" on a call
  → POST /api/supervisor/monitor {call_id, mode: "listen"}
  → Gateway creates WebRTC session for supervisor (receive-only)
  → Audio tap on caller + agent channels → mix → supervisor browser speaker
  → Supervisor hears both sides, can't speak
  → Clicks "Stop" → POST /api/supervisor/stop → WebRTC closed
```

### API
```
POST /api/supervisor/monitor
{
  "call_id": "siprec-call-id",
  "mode": "listen"           // listen, whisper, barge
}

Response:
{
  "status": "monitoring",
  "mode": "listen",
  "sdp": "WebRTC answer SDP"
}
```

---

## 2. Whisper (Coach Agent)

### What
Supervisor speaks to the agent in real-time. The caller cannot hear the supervisor — only the agent can.

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| WH-1 | Supervisor clicks "Whisper" to start coaching | Must |
| WH-2 | Supervisor's voice is heard only by the agent | Must |
| WH-3 | Caller cannot hear the supervisor | Must |
| WH-4 | Supervisor still hears both caller and agent | Must |
| WH-5 | Agent sees a visual indicator that supervisor is whispering | Should |
| WH-6 | Supervisor can switch between listen and whisper modes | Should |
| WH-7 | Whisper audio does not affect copilot transcription | Should |

### Call Flow
```
Supervisor clicks "Whisper" (or upgrades from listen)
  → POST /api/supervisor/monitor {call_id, mode: "whisper"}
  → Gateway creates bidirectional WebRTC session
  → Supervisor hears: mix(caller, agent) — same as listen
  → Supervisor speaks: audio → whisper channel → mixed into agent's speaker only
  → Caller hears: only agent voice (no supervisor)
  → Agent hears: caller + supervisor whisper mixed
```

### Audio Architecture
```
                          ┌─────────────────┐
Caller audio ────────────►│                 │──► Supervisor speaker
Agent audio  ────────────►│  Mix for        │    (hears both sides)
                          │  Supervisor     │
                          └─────────────────┘

Supervisor mic ──► whisperCh ──► Mixed into agent's
                                 WebRTC outTrack
                                 (only agent hears)
```

---

## 3. Barge (Join Call)

### What
Supervisor joins the active call as a third participant. All three parties (caller, agent, supervisor) can hear each other.

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| BG-1 | Supervisor clicks "Barge" to join the call | Must |
| BG-2 | All three parties hear each other | Must |
| BG-3 | Reuse existing 3-way conference mixer | Must |
| BG-4 | Supervisor can exit barge without ending the call | Must |
| BG-5 | Caller and agent are notified when supervisor barges | Should |
| BG-6 | Copilot continues running during barge | Should |

### Call Flow
```
Supervisor clicks "Barge"
  → POST /api/supervisor/monitor {call_id, mode: "barge"}
  → Gateway calls startConference(callID, "agent", supervisorID)
  → AudioMixer activated: caller, agent, supervisor all mixed
  → All three hear each other
  → Supervisor clicks "Stop" → dropThirdParty() → back to 2-way call
```

---

## 4. Real-Time Supervisor Dashboard

### What
A dedicated page showing all active calls with live metrics, queue depths, and agent statuses.

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| DB-1 | Dashboard page at /supervisor, accessible to supervisor+admin roles | Must |
| DB-2 | Live call cards: caller, agent, duration, queue, sentiment indicator | Must |
| DB-3 | Queue depth summary: calls waiting per queue | Must |
| DB-4 | Agent status overview: available, on-call, break counts | Must |
| DB-5 | Click-to-action on each call: Monitor, Whisper, Barge buttons | Must |
| DB-6 | Auto-refresh every 3 seconds | Must |
| DB-7 | Live transcript preview when monitoring a call | Should |
| DB-8 | Alert when caller frustration exceeds threshold | Should |
| DB-9 | Filter calls by queue or agent | Should |

### Data Sources
```
GET /api/supervisor/calls     → active calls with sentiment + queue + agent
GET /api/queues               → queue depths (already exists)
GET /api/agents               → agent statuses (already exists)
GET /siprec/events?call_id=X  → live transcript SSE (already exists)
```

### Dashboard Layout
```
┌─────────────────────────────────────────────────────────┐
│ SUPERVISOR DASHBOARD                     Queues: 3 | 0 │
├─────────────┬───────────────────────────────────────────┤
│ QUEUE       │ ACTIVE CALLS                              │
│ OVERVIEW    │                                           │
│             │ ┌─────────────────┐ ┌─────────────────┐   │
│ Support: 2  │ │ +1-555-0100     │ │ +1-555-0200     │   │
│ Sales: 0    │ │ Agent: Sarah    │ │ Agent: Alex     │   │
│ Billing: 1  │ │ 3:45 | Support  │ │ 1:20 | Sales    │   │
│             │ │ 😊 CALM         │ │ 😤 FRUSTRATED   │   │
│ AGENTS      │ │ [👂] [🎤] [📞] │ │ [👂] [🎤] [📞] │   │
│ Online: 4   │ └─────────────────┘ └─────────────────┘   │
│ On Call: 2  │                                           │
│ Break: 1    │ ┌─────────────────────────────────────┐   │
│ Available:1 │ │ MONITORING: +1-555-0100              │   │
│             │ │ Live transcript...                    │   │
│             │ │ [STOP] [WHISPER ▲] [BARGE ▲]         │   │
│             │ └─────────────────────────────────────┘   │
└─────────────┴───────────────────────────────────────────┘
```

---

## 5. Implementation Priority

| Priority | Feature | Complexity | Depends On |
|----------|---------|-----------|------------|
| 1 | Supervisor API + listen-only | Medium | Existing audio taps |
| 2 | Supervisor dashboard UI | Medium | Supervisor API |
| 3 | Whisper mode | Medium | Supervisor API + WebRTC mod |
| 4 | Barge mode | Low | Existing conference mixer |

---

## 6. Integration Points

### Existing Code to Reuse

| Component | File | Reuse |
|-----------|------|-------|
| Audio taps | `gateway/siprec.go` | `AddAudioTap()` for listen-only caller audio |
| Conference mixer | `gateway/conference.go` | `startConference()` for barge mode |
| WebRTC bridge | `gateway/webrtc.go` | Modified `handleBridge()` for supervisor sessions |
| Active sessions | `gateway/siprec.go` | `/api/copilot/active` endpoint with sentiment |
| SSE events | `gateway/siprec.go` | Live transcript via `/siprec/events` |
| Role auth | `ui/src/lib/auth.ts` | Supervisor role already in `PAGE_ACCESS` |
| Agent states | `gateway/agent_session.go` | Agent online/status tracking |

### New Files

| File | Purpose |
|------|---------|
| `gateway/supervisor.go` | Supervisor session, monitor/whisper/barge handlers |
| `ui/src/app/supervisor/page.tsx` | Supervisor live dashboard |

### Modified Files

| File | Change |
|------|--------|
| `gateway/siprec.go` | Add `whisperCh` field to siprecSession |
| `gateway/webrtc.go` | Mix whisper audio into agent's callerTap output |
| `gateway/main.go` | Register supervisor routes |
| `ui/src/lib/auth.ts` | Add `/supervisor` to PAGE_ACCESS |
| `ui/src/components/layout/Sidebar.tsx` | Add Supervisor nav link |

---

## 7. Acceptance Criteria

### Monitor
- [ ] Supervisor sees active calls with real-time data
- [ ] Click Monitor → hears both caller and agent
- [ ] Neither party knows supervisor is listening
- [ ] Click Stop → monitoring ends, call continues

### Whisper
- [ ] Supervisor speaks → only agent hears
- [ ] Caller does not hear supervisor
- [ ] Supervisor hears both sides
- [ ] Agent sees "supervisor coaching" indicator

### Barge
- [ ] Supervisor joins → all three hear each other
- [ ] Supervisor exits → call continues as 2-way
- [ ] Reuses existing conference mixer

### Dashboard
- [ ] Shows all active calls with sentiment
- [ ] Shows queue depths and agent statuses
- [ ] Auto-refreshes in real time
- [ ] Only accessible to supervisor/admin roles

---

## 8. Test Plan

```bash
# Setup
# Login as supervisor role user
# Have a test call active (baresip → agent picks on Console)

# Monitor (Listen-Only)
# Open /supervisor dashboard
# Verify active call appears with correct data
# Click Monitor → verify audio heard in browser
# Click Stop → verify clean disconnect

# Whisper
# Click Whisper on active call
# Speak → verify agent hears in Console
# Verify caller does NOT hear supervisor
# Click Stop

# Barge
# Click Barge on active call
# Verify all three hear each other
# Click Stop → verify 2-way call continues

# Dashboard
# Start multiple calls
# Verify all appear on dashboard
# Verify sentiment badges update
# Verify queue depth summary updates
```
