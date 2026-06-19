# Call Center Trunks & Routing — How It Works in Production

## How a Real Call Center Works

```
                    PSTN / Internet
                         │
                    ┌─────┴─────┐
                    │  SIP Trunk │  (Twilio, Vonage, Telnyx, Cisco CUBE)
                    │  Provider  │  Provides phone numbers (DIDs)
                    └─────┬─────┘
                         │
                    ┌─────┴─────┐
                    │    SBC     │  Session Border Controller
                    │            │  Security, NAT, codec transcoding
                    └─────┬─────┘
                         │
              ┌──────────┼──────────┐
              │          │          │
         DID Pool    IVR/Auto     Direct
         Routing     Attendant    Inward
              │          │        Dial (DID)
              └──────────┼──────────┘
                         │
                    ┌─────┴─────┐
                    │   ACD     │  Automatic Call Distributor
                    │  (Router) │  Skills, priority, availability
                    └─────┬─────┘
                         │
              ┌──────────┼──────────┐
              │          │          │
         Queue A    Queue B    Queue C
         (Sales)   (Support)  (Billing)
              │          │          │
         Agent Pool  Agent Pool  Agent Pool
```

## Key Concepts

### 1. Trunks (Inbound & Outbound)

**Inbound Trunk**: SBC/provider sends calls TO the gateway
- Provider gives you phone numbers (DIDs like +1-800-555-0100)
- Multiple DIDs can map to different queues
- The trunk is the pipe — DIDs are the addresses on it

**Outbound Trunk**: Gateway sends calls OUT through the SBC
- Agent makes outbound call → gateway sends INVITE through trunk
- Caller ID, codec, routing rules configured per trunk
- Used for: callbacks, outbound campaigns, transfers to external numbers

**SIPREC Trunk**: SBC sends a COPY of the call (passive)
- Gateway doesn't handle the call — just observes
- Provides transcription, copilot, sentiment analysis
- The SBC manages the actual call routing

### 2. DID (Direct Inward Dial) Routing

Each phone number maps to a destination:
```
+1-800-555-0100 → Sales queue
+1-800-555-0200 → Support queue
+1-800-555-0300 → Billing queue
+1-555-123-4567 → Agent Sarah Chen (direct line)
```

The gateway needs a DID routing table:
- DID number → queue name OR agent extension
- Time-based routing (business hours → queue, after hours → voicemail/IVR)
- Overflow rules (if queue full → different queue)

### 3. ACD (Automatic Call Distributor)

Routes calls to the best available agent based on:
- **Skills**: Agent has billing+retention skills, call needs billing
- **Priority**: VIP callers get priority agents
- **Availability**: Agent must be Available (not on break/busy)
- **Load**: Agent with fewest active calls
- **Round-robin**: Distribute evenly
- **Longest idle**: Agent waiting longest gets next call

### 4. IVR (Interactive Voice Response)

Before reaching an agent:
```
"Press 1 for Sales, 2 for Support, 3 for Billing"
"Please say your account number"
"Your estimated wait time is 5 minutes"
```

In VoiceAgent's architecture:
- Gateway could handle IVR with AI (Claude generates responses)
- Or IVR happens on the SBC side (before SIPREC/direct trunk)
- The gateway's interactive mode already does AI conversation

### 5. Call Flows

```
Inbound Call Flow:
─────────────────
1. Call arrives on trunk (SIP INVITE from SBC)
2. Gateway authenticates trunk (IP + auth)
3. DID routing: which queue? (+1-800-555-0100 → Sales)
4. Queue the call (caller hears hold music/position announcement)
5. ACD finds best agent (skills, availability, load)
6. Ring agent on Console (SSE notification)
7. Agent picks → WebRTC bridge → two-way audio
8. Copilot runs: transcription, coaching, sentiment
9. Call ends → summary, action items → CRM webhook

Outbound Call Flow:
──────────────────
1. Agent clicks "Dial" on Console
2. Gateway sends INVITE through outbound trunk
3. Caller answers → WebRTC bridge
4. Same copilot pipeline

Transfer Flow:
─────────────
1. Agent clicks "Transfer" → enters target (queue/agent/external)
2. Warm transfer: agent stays on, introduces caller
3. Cold transfer: caller moved directly
4. Blind transfer: caller goes to queue

SIPREC Observer Flow:
────────────────────
1. SBC handles the call normally (its own IVR, routing, agents)
2. SBC sends SIPREC fork to gateway
3. Gateway provides: transcription, copilot, sentiment, summary
4. No call control — gateway is passive
```

## What's Built vs What's Needed

### Built ✓
- SIP trunk management (CRUD, security, ACL)
- Trunk types: Direct (B2BUA) + SIPREC (Observer)
- IP whitelist + digest auth
- Basic queue manager (in-memory + DB)
- Agent profiles, skills, status
- WebRTC bridge (agent hears/speaks through Console)
- Copilot pipeline (STT → Claude → suggestions)
- Voice sentiment analysis
- Post-call summary
- Console UI with queue monitor
- Role-based access (admin/supervisor/agent)
- Test SBC (Kamailio) + test caller script
- Docker Compose test environment

### Needed for Production Call Center

#### Phase 1: Core Routing (make it a real call center)
- DID routing table + API
- Enhanced ACD (skills + availability + load balancing)
- Queue announcements (position/wait time via TTS)
- Agent ring notification (SSE to specific agent)

#### Phase 2: Call Control
- Outbound calling through trunk
- Call transfer (warm/cold/blind)
- Conference (3-way calling)
- Hold/resume with music

#### Phase 3: Supervisor Tools
- Live call monitoring (listen-only)
- Whisper (coach agent, caller can't hear)
- Barge (join the call)
- Real-time dashboard (calls in progress, queue depths)

#### Phase 4: Enterprise Features
- IVR builder (AI-powered, configurable)
- Call recording + storage
- CRM webhooks (Salesforce, HubSpot)
- Reporting (call analytics, agent performance)
- Multi-tenant support
