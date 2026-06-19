# Phase 1: Core Routing — Requirements Document

## Objective

Transform VoiceAgent from a basic SIP-to-WebRTC bridge into a functional call center with intelligent call routing. Callers reach the right agent based on the number they dialed, their needs, and agent availability.

---

## 1. DID Routing

### What
Map incoming phone numbers (DIDs) to destinations (queues, agents, IVR).

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| DID-1 | DID routing table in database — maps DID pattern to destination | Must |
| DID-2 | Destination types: queue, agent (direct extension), IVR, external forward | Must |
| DID-3 | Pattern matching — exact (`+18005550100`), prefix (`+1800*`), regex | Must |
| DID-4 | Time-based routing — business hours vs after-hours vs holidays | Should |
| DID-5 | Trunk-scoped DIDs — same DID can route differently per trunk | Should |
| DID-6 | Default route — calls with no matching DID go to a fallback queue | Must |
| DID-7 | UI to manage DID routes (Settings page) | Must |
| DID-8 | API: CRUD `/api/routing/dids` | Must |

### Example DID Routes
```
+18005550100  →  queue:Sales         (Mon-Fri 9am-5pm)
+18005550100  →  queue:After-Hours   (outside business hours)
+18005550200  →  queue:Support
+18005550300  →  queue:Billing
+15551234567  →  agent:sarah.chen    (direct line)
*             →  queue:Support       (default fallback)
```

### Data Model
```sql
CREATE TABLE did_routes (
    id UUID PRIMARY KEY,
    did_pattern TEXT NOT NULL,          -- "+18005550100" or "+1800*" or "*"
    match_type TEXT DEFAULT 'exact',    -- exact, prefix, regex
    trunk_id UUID REFERENCES sip_trunks(id),  -- NULL = any trunk
    destination_type TEXT NOT NULL,     -- queue, agent, ivr, forward
    destination_value TEXT NOT NULL,    -- queue name, agent ID, or phone number
    priority INT DEFAULT 0,            -- higher = checked first
    time_condition JSONB,              -- {"days":"mon-fri","start":"09:00","end":"17:00"}
    overflow_destination TEXT,          -- fallback if primary full/unavailable
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Call Flow
```
INVITE arrives → extract To/Request-URI → match against did_routes
  → Found: route to destination (queue/agent/IVR)
  → Not found: use default route
  → No default: reject with 404
```

---

## 2. Enhanced ACD (Automatic Call Distributor)

### What
Intelligently assign queued calls to the best available agent.

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| ACD-1 | Routing strategies: skills-based, round-robin, longest-idle, least-calls | Must |
| ACD-2 | Skills matching — call's required skills vs agent's expertise | Must |
| ACD-3 | Agent availability check — only route to Available agents | Must |
| ACD-4 | Load balancing — prefer agent with fewer active calls | Must |
| ACD-5 | Priority queuing — VIP callers jump the queue | Should |
| ACD-6 | Queue-to-agent assignment — agents assigned to specific queues | Must |
| ACD-7 | Configurable per queue — each queue can have different strategy | Should |
| ACD-8 | Wrap-up time — agent gets N seconds after call before next one | Should |
| ACD-9 | Max concurrent calls per agent — respect agent.max_calls | Must |
| ACD-10 | Overflow — if no agent available after N seconds, route to overflow | Should |

### Routing Algorithm
```
1. Call enters queue with required skills
2. Find agents assigned to this queue
3. Filter: status=Available AND active_calls < max_calls
4. Score each agent:
   - Skill match:     +10 per matching skill
   - Language match:   +20
   - Priority tier:    +5 × agent.priority
   - Load penalty:     -10 × active_calls
   - Idle bonus:       +1 per second idle
5. Highest score wins → ring agent
6. If no agent: caller stays in queue
7. After overflow_timeout: route to overflow destination
```

### Data Model Updates
```sql
-- Queue configuration
ALTER TABLE queues ADD COLUMN routing_strategy TEXT DEFAULT 'skills';
ALTER TABLE queues ADD COLUMN overflow_queue TEXT;
ALTER TABLE queues ADD COLUMN overflow_timeout_sec INT DEFAULT 300;
ALTER TABLE queues ADD COLUMN wrap_up_sec INT DEFAULT 15;
ALTER TABLE queues ADD COLUMN max_wait_sec INT DEFAULT 600;

-- Agent-queue assignment (already exists in agent_queues table)
-- Agent last_call_end for idle time tracking
ALTER TABLE agents ADD COLUMN last_call_end TIMESTAMPTZ;
```

---

## 3. Queue Announcements

### What
Callers hear their position and estimated wait time while in queue.

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| QA-1 | Position announcement — "You are caller number 3 in the queue" | Must |
| QA-2 | Wait time estimate — "Estimated wait time is 5 minutes" | Should |
| QA-3 | Periodic announcements — repeat every 30-60 seconds | Must |
| QA-4 | Welcome message — played once when entering queue | Should |
| QA-5 | Hold music between announcements | Should |
| QA-6 | Generate announcements via TTS (Piper or cloud) | Must |
| QA-7 | Configurable per queue — different messages per queue | Should |

### Implementation
```
Caller enters queue:
  1. Play welcome: "Thank you for calling Sales. Please hold."
  2. Every 30s:
     a. Generate TTS: "You are caller {N}. Estimated wait: {M} minutes."
     b. Send audio via RTP to caller
     c. Play hold music between announcements
  3. Agent picks → stop announcements → bridge audio
```

---

## 4. Agent Ring Notification

### What
When ACD selects an agent, notify them on the Console with call details.

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| AR-1 | SSE push to specific agent when call is assigned | Must |
| AR-2 | Ring notification shows: caller number, queue, wait time, reason | Must |
| AR-3 | Accept/Reject buttons on Console | Must |
| AR-4 | Ring timeout — if agent doesn't pick in N seconds, try next agent | Must |
| AR-5 | Agent status changes to "Ringing" during ring | Should |
| AR-6 | Browser notification (if tab not focused) | Should |
| AR-7 | Ring tone audio in browser | Should |
| AR-8 | If rejected, route to next best agent | Must |

### SSE Event Format
```json
{
  "type": "ring",
  "call_id": "abc-123",
  "caller": "+18005551234",
  "queue": "Sales",
  "wait_seconds": 45,
  "reason": "Billing inquiry",
  "skills_matched": ["billing", "retention"],
  "ring_timeout_sec": 20
}
```

### Console Flow
```
Agent Console (status: Available)
  → SSE: ring event received
  → UI: incoming call modal with caller info
  → Agent clicks Accept → WebRTC bridge starts
  → Agent clicks Reject → ACD routes to next agent
  → Timeout (20s) → ACD routes to next agent, agent status → "Missed"
```

---

## 5. Integration Points

### How DID Routing Connects to Existing Code

```
sipserver.go handleInvite()
  → authenticate trunk (existing)
  → NEW: resolve DID route (extract To header → match did_routes)
  → NEW: if destination=queue → enhanced ACD places in queue
  → NEW: ACD finds agent → ring notification via SSE
  → agent picks → existing WebRTC bridge
  → existing copilot pipeline runs
```

### Files to Create/Modify

| File | Change |
|------|--------|
| `gateway/did_routing.go` | NEW: DID route matching, time conditions, API handlers |
| `gateway/acd.go` | NEW: Enhanced ACD with scoring, assignment, ring management |
| `gateway/migrations/006_did_routing.up.sql` | NEW: did_routes table, queue config columns |
| `gateway/sipserver.go` | Call handleInvite to use DID routing instead of hardcoded "Support" queue |
| `gateway/queue.go` | Add announcement support, wrap-up time, overflow |
| `gateway/agent_session.go` | Ring notification, accept/reject, timeout handling |
| `gateway/rtplistener.go` | Send TTS announcements via RTP to waiting callers |
| `ui/src/app/settings/page.tsx` | DID routing management UI |
| `ui/src/app/console/page.tsx` | Incoming ring modal, accept/reject |

---

## 6. Acceptance Criteria

### DID Routing
- [ ] Call to +18005550100 routes to Sales queue
- [ ] Call to +18005550200 routes to Support queue  
- [ ] Call to unknown number routes to default queue
- [ ] Time-based route changes destination outside business hours
- [ ] DID routes manageable via API and UI

### ACD
- [ ] Call in Sales queue assigned to agent with sales skills
- [ ] Agent with fewer active calls preferred
- [ ] Agent on break NOT assigned calls
- [ ] Agent at max_calls NOT assigned calls
- [ ] If no agent available, caller waits in queue

### Queue Announcements
- [ ] Caller hears position announcement every 30 seconds
- [ ] Announcement delivered via RTP (TTS generated)

### Agent Ring
- [ ] Agent Console shows incoming call notification
- [ ] Accept → WebRTC bridge starts
- [ ] Reject → next agent gets the call
- [ ] Timeout (20s) → next agent gets the call

---

## 7. Test Plan

```bash
# 1. Create DID routes
curl -X POST /api/routing/dids -d '{"did_pattern":"+18005550100","destination_type":"queue","destination_value":"Sales"}'
curl -X POST /api/routing/dids -d '{"did_pattern":"*","destination_type":"queue","destination_value":"Support"}'

# 2. Make test call with specific DID
# Modify sip_test_call.py to set To header to +18005550100
python3 test/sip_test_call.py gateway:5062 60

# 3. Verify call lands in Sales queue (not Support)
# Console should show "Sales" queue

# 4. Verify ACD selects correct agent
# Sarah (sales skills) gets the ring, not Priya (billing skills)

# 5. Agent picks → audio flows → copilot runs
```
