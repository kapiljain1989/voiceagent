# VoiceAgent Feature Documentation

Complete technical reference for every feature in the VoiceAgent Telecom-Native AI Gateway. 14 Go source files, 5,533 lines of code, 25+ API endpoints, 7 containerized services.

---

## Table of Contents

1. [Interactive AI Agent](#1-interactive-ai-agent)
2. [Co-Pilot Agent Assist (SIPREC)](#2-co-pilot-agent-assist-siprec)
3. [Smart Self-Service Actions](#3-smart-self-service-actions)
4. [Intelligent Call Transfer](#4-intelligent-call-transfer)
5. [Robocall Detection](#5-robocall-detection)
6. [Voice Biometrics & Fraud Detection](#6-voice-biometrics--fraud-detection)
7. [Live PII Masking](#7-live-pii-masking)
8. [Post-Call Summary & CRM Webhook](#8-post-call-summary--crm-webhook)
9. [G.711 Codec Transcoding](#9-g711-codec-transcoding)
10. [RFC 2833 DTMF Parsing](#10-rfc-2833-dtmf-parsing)
11. [Telecom Audio Pipeline (AGC)](#11-telecom-audio-pipeline-agc)
12. [SIPREC Metadata Parser (RFC 7866)](#12-siprec-metadata-parser-rfc-7866)
13. [Deterministic Failover State Machine](#13-deterministic-failover-state-machine)
14. [Multi-LLM Support](#14-multi-llm-support)
15. [RAG Knowledge Base](#15-rag-knowledge-base)
16. [Next.js Operations Dashboard](#16-nextjs-operations-dashboard)
17. [SBC Connectivity & Enterprise Profiles](#17-sbc-connectivity--enterprise-profiles)
18. [Deployment Modes](#18-deployment-modes)
19. [SDKs (Python & TypeScript)](#19-sdks-python--typescript)
20. [Voice Sentiment Analysis](#20-voice-sentiment-analysis)
21. [Standalone SIPREC Helper](#21-standalone-siprec-helper)

---

## 1. Interactive AI Agent

**Source:** `gateway/main.go` (1,173 lines)
**Endpoint:** `WebSocket /ws`
**Pipeline:** 5 goroutines per call session

### How It Works

A customer calls in via SIP. FreeSWITCH answers, establishes the RTP media session, and forks the audio stream over a WebSocket connection to the Go gateway using `mod_audio_fork`. The gateway runs a pipeline of 5 concurrent goroutines connected by buffered Go channels:

```
readFromFS ──[pcmIn]──▶ sttPipeline ──[transcripts]──▶ claudeWorker ──[sentences]──▶ ttsWorker ──[pcmOut]──▶ writeToFS
```

| Goroutine | Responsibility | Channel In | Channel Out |
|-----------|---------------|------------|-------------|
| `readFromFS` | Reads binary L16 PCM from FreeSWITCH WebSocket | WebSocket | `pcmIn` (50 frames) |
| `sttPipeline` | Energy-based VAD + Whisper batch transcription + PII masking + robocall keyword check | `pcmIn` | `transcripts` (4 slots) |
| `claudeWorker` | Claude/Gemini streaming with action parsing (speak/api_call/transfer) | `transcripts` | `sentences` (8 slots) |
| `ttsWorker` | Piper HTTP synthesis, WAV header stripping, optional resampling | `sentences` | `pcmOut` (20 slots) |
| `writeToFS` | ESL `uuid_broadcast` playback with 20ms frame pacing, JSON event frames | `pcmOut` | WebSocket |

### VAD Parameters

| Parameter | Value | Description |
|-----------|-------|-------------|
| `vadRMSThreshold` | 50 | RMS energy above this triggers speech detection |
| `vadSilenceMs` | 500 | Milliseconds of silence before flushing to Whisper |
| `vadMaxBufferSecs` | 5 | Maximum audio accumulation (prevents unbounded growth) |
| `vadFrameMs` | 20 | Expected frame duration from FreeSWITCH |
| `warmupFrames` | 25 | Frames skipped at start (media path stabilization) |

### Session Lifecycle

1. FreeSWITCH connects WebSocket to `/ws?uuid=<channel-uuid>`
2. Gateway reads first frame — JSON metadata or raw PCM (auto-detected)
3. 5 goroutines spawn with buffered channels
4. Audio flows: FS → VAD → Whisper → Claude → Piper → ESL → FS → caller
5. On WebSocket close: channel-close cascade shuts down all goroutines
6. Session counter decremented atomically

### Whisper Hallucination Filter

Whisper generates false positives from silence/noise ("Thank you", "Goodbye", "Okay"). The gateway filters these using exact-match and repetition detection:

- **Exact match:** "thank you", "thanks", "bye", "goodbye", "you", "okay", "oh", "hmm"
- **Repetition:** 3+ occurrences of "thank you", "all right", "okay", "good" in a single transcript
- Phrases with 4+ words pass through (e.g., "Thank you very much" is not filtered)

### API

```
WebSocket /ws?uuid=<freeswitch-channel-uuid>
Subprotocol: audio.drachtio.org
```

**Events sent to client:**
- `{"event":"transcript","text":"..."}`
- `{"event":"response","text":"..."}`
- `{"event":"robocall","text":"score=..."}`
- `{"event":"pii_masked","text":"Detected..."}`
- `{"event":"action","intent":"..."}`
- `{"event":"transfer","reason":"..."}`

---

## 2. Co-Pilot Agent Assist (SIPREC)

**Source:** `gateway/siprec.go` (772 lines)
**Endpoint:** `WebSocket /siprec`, `SSE /siprec/events`
**Pipeline:** 5 goroutines (2 STT + coach + 2 readers)

### How It Works

For calls routed to 2xxx extensions, FreeSWITCH forks both the caller and agent audio legs to the gateway as separate WebSocket connections. The gateway pairs them into a single `siprecSession` and runs independent STT pipelines per speaker.

```
FreeSWITCH ──[caller WS]──▶ /siprec?role=caller&call_id=xxx
FreeSWITCH ──[agent WS]───▶ /siprec?role=agent&call_id=xxx
```

### Session Pairing

The first WebSocket connection creates the session. The second connection joins it via the shared `call_id`. Sessions are stored in a concurrent map with mutex protection:

```go
siprecSessions map[string]*siprecSession  // protected by siprecSessionsMu
```

### Coach Worker

When a customer utterance is transcribed:

1. **RAG query** — searches ChromaDB for matching document chunks
2. **Context injection** — relevant policy docs prepended to Claude's system prompt
3. **Claude generates suggestion** — JSON format: `{"suggestion":"...","category":"...","confidence":0.9}`
4. **SSE broadcast** — suggestion pushed to all subscribed agent dashboards

### Coach System Prompt

```
You are a real-time agent coach on a live call. Provide ONE brief suggestion.
- Output ONLY a single valid JSON object
- Quote specific facts from the knowledge base context
- If the customer's words are unclear, interpret the most likely intent
- Categories: "answer", "compliance", "empathy", "upsell"
```

### Post-Call Summary

On WebSocket close (BYE detected):

1. Grace period: 3 seconds for in-flight requests to complete
2. Full transcript sent to Claude with summary system prompt
3. Structured JSON generated: summary, action_items, commitments_made, sentiment
4. Summary broadcast via SSE to all connected dashboards
5. POST to CRM webhook (if configured)
6. Session stays in map for 30 seconds (so SSE clients receive the summary)

### API

```
WebSocket /siprec?role=caller|agent&call_id=<uuid>
GET /siprec/events?call_id=<uuid>          (SSE stream)
GET /siprec/summary                         (placeholder for DB query)
```

---

## 3. Smart Self-Service Actions

**Source:** `gateway/actions.go` (349 lines)
**Endpoint:** `/api/actions/test`, `/api/actions/webhooks`

### How It Works

The `claudeWorker` uses an action-aware system prompt that instructs Claude to return structured JSON instead of plain text when it detects an actionable customer request:

```json
{"type":"api_call","text":"Done! Rescheduled to Thursday.","intent":"reschedule",
 "api_call":{"endpoint":"/deliveries","method":"PUT","payload":{"date":"2026-06-12","time":"15:00"}},
 "confidence":0.9}
```

### Action Types

| Type | Trigger | Gateway Behavior |
|------|---------|-----------------|
| `speak` | Normal conversation | Speaks text via TTS (default) |
| `api_call` | Customer requests: reschedule, cancel, check status | Executes HTTP request to configured CRM webhook, speaks confirmation |
| `transfer` | Anger detected, complexity, explicit escalation request | Sets SIP X-headers, ESL transfers to target department |

### Action Execution

1. `ParseAction()` extracts the JSON action from Claude's response
2. For `api_call`: POST/PUT/GET to the configured webhook URL with the payload
3. Custom headers added: `X-Call-ID`, `X-Intent`
4. On success: speaks the confirmation text via TTS
5. On failure: speaks a fallback message and offers to connect to a human

### Webhook Configuration

```bash
curl -X POST http://localhost:8080/api/actions/webhooks \
  -d '{"reschedule":"https://crm.example.com/api/deliveries",
       "cancel":"https://crm.example.com/api/subscriptions",
       "check_status":"https://crm.example.com/api/accounts"}'
```

---

## 4. Intelligent Call Transfer

**Source:** `gateway/actions.go` (within `executeTransfer()`)

### How It Works

When Claude returns a `transfer` action, the gateway:

1. Pauses the audio fork
2. Sets custom SIP headers on the FreeSWITCH channel via ESL:

```
X-Transfer-Summary:    "Customer upset about $142 billing error. Wants refund."
X-Transfer-Reason:     angry
X-Transfer-Department: retention
X-Transfer-Priority:   urgent
X-Transfer-Transcript: "[customer] overcharged $142 | [agent] checking..."
X-Transfer-CallID:     <session-uuid>
```

3. Executes `uuid_transfer` via ESL to the target department extension
4. The receiving agent's Cisco/Avaya softphone displays these X-headers instantly

### Department Routing Table

| Department | Extension | Trigger Signals |
|------------|-----------|----------------|
| Billing | 3001 | Billing disputes, charges, refunds |
| Technical | 3002 | Product issues, troubleshooting |
| Sales | 3003 | Upgrades, new services |
| Retention | 3004 | Cancellation threats, unhappy customers |
| Supervisor | 3005 | Explicit "let me speak to a manager" |
| Legal | 3006 | Legal terms, regulatory mentions |
| Claims | 3007 | Insurance claims, damage reports |
| General | 3000 | Default / failover queue |

---

## 5. Robocall Detection

**Source:** `gateway/robocall.go` (488 lines)
**Endpoints:** `/api/blocklist`, `/api/robocall/stats`, `/api/robocall/test`

### Three Detection Layers

**Layer 1 — Blocklist (< 1ms)**
- In-memory `map[string]string` (number → reason)
- Loaded from PostgreSQL on startup
- O(1) lookup per call
- Numbers normalized to digits only
- Auto-increments call count on each hit

**Layer 2 — Audio Pattern Analysis (~2 seconds)**
- Receives first 2 seconds of PCM audio (100 frames)
- Computes per-frame RMS energy
- Metrics: average energy, standard deviation, coefficient of variation, silence ratio
- Scoring: low CV (monotone) +0.4, high silence +0.3, dead channel +0.2
- Low variance + consistent energy = pre-recorded robocall

**Layer 3 — Transcript Keywords (after first STT)**
- 28 weighted phrases, each with a score from 0.3 to 0.8:

| Phrase | Weight | Category |
|--------|--------|----------|
| "auto warranty" | 0.8 | Scam |
| "extended warranty" | 0.8 | Scam |
| "do not hang up" | 0.7 | Pressure |
| "you have won" | 0.7 | Scam |
| "arrest warrant" | 0.7 | Threat |
| "press 1" | 0.6 | IVR |
| "IRS" | 0.6 | Government impersonation |
| "social security" | 0.6 | PII harvesting |
| "your amazon" | 0.6 | Brand impersonation |
| "we have been trying to reach you" | 0.6 | Pressure |
| ...and 18 more | 0.3-0.6 | Various |

### Combined Scoring

```
finalScore = (audioScore × 0.4) + (keywordScore × 0.6)
```

- Score > threshold (default 0.7) → category: `robocall`
- Score > threshold × 0.6 → category: `uncertain`
- Otherwise → category: `human`
- Blocklist match → score: 1.0, category: `robocall`, blocked: true

### Database Schema

```sql
CREATE TABLE blocklist (
    id UUID PRIMARY KEY,
    number TEXT UNIQUE NOT NULL,
    reason TEXT,
    source TEXT DEFAULT 'manual',     -- manual, auto_detected, reported
    call_count INT DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Added to calls table:
ALTER TABLE calls ADD COLUMN robocall_score FLOAT DEFAULT 0;
ALTER TABLE calls ADD COLUMN robocall_category TEXT DEFAULT 'human';
ALTER TABLE calls ADD COLUMN blocked BOOLEAN DEFAULT FALSE;
```

---

## 6. Voice Biometrics & Fraud Detection

**Source:** `gateway/security.go` (voice biometrics section, ~200 lines)
**Endpoint:** `/api/security/voiceprints`

### How It Works

Concurrent voice fingerprinting runs alongside the STT pipeline. The gateway extracts a 32-dimensional spectral feature vector from raw PCM audio and compares it against enrolled voice prints.

### Feature Extraction

1. Divide PCM audio into 20ms frames (320 samples at 16kHz)
2. Split each frame into 32 frequency bands
3. Compute band energy (sum of squared samples) per band
4. Average across all frames
5. L2-normalize the feature vector

### Voice Print Matching

- **Similarity metric:** Cosine similarity between feature vectors
- **Match threshold:** 0.85 (configurable)
- **Two profile types:**
  - `fraud` — known bad actor voice profiles
  - `verified` — confirmed account holder voiceprints

### Database Schema

```sql
CREATE TABLE voice_prints (
    id UUID PRIMARY KEY,
    label TEXT NOT NULL,
    type TEXT NOT NULL,        -- 'fraud' or 'verified'
    features JSONB NOT NULL,   -- 32-dim float array
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### API

```bash
# List all enrolled prints
GET /api/security/voiceprints

# Enroll a new print
POST /api/security/voiceprints
{"label": "fraud_profile_001", "type": "fraud"}
```

---

## 7. Live PII Masking

**Source:** `gateway/security.go` (PII masking section, ~200 lines)
**Endpoint:** `/api/security/pii/test`, `/api/security/pii/config`

### Detection Patterns

9 regex patterns run on every Whisper transcript before the text reaches Claude or gets stored:

| Pattern | Example Input | Masked Output | Level |
|---------|---------------|---------------|-------|
| **Credit card** | `4111 1111 1111 1111` | `XXXX-XXXX-XXXX-####` | Critical |
| **SSN** | `123-45-6789` | `XXX-XX-####` | Critical |
| **SSN spoken** | `"social security number is 1..."` | `[SSN REDACTED]` | Critical |
| **Card spoken** | `"card number is 4..."` | `[CARD REDACTED]` | Critical |
| **CVV** | `"CVV is 123"` | `[CVV REDACTED]` | Critical |
| **Date of birth** | `"date of birth is Jan..."` | `[DOB REDACTED]` | High |
| **Account number** | `"account number is 1234567..."` | `[ACCOUNT REDACTED]` | High |

### Audio Frame Silencing

When PII is detected, the corresponding PCM audio frames can be zeroed before they reach the call recording system:

```go
func SilenceAudioFrames(frames [][]byte) {
    for i := range frames {
        for j := range frames[i] {
            frames[i][j] = 0
        }
    }
}
```

### Pipeline Integration

PII masking runs in `transcribeAndSend()` after Whisper returns the transcript and before the text is passed to `claudeWorker` or logged:

```
Whisper → transcript text → PII masking → masked text → Claude / logs / recording
```

### Configuration

```bash
# Test PII detection
POST /api/security/pii/test
{"text": "My card number is 4111 1111 1111 1111 and CVV is 123"}

# Enable/disable
POST /api/security/pii/config
{"enabled": true}
```

---

## 8. Post-Call Summary & CRM Webhook

**Source:** `gateway/siprec.go` (`onCallEnd()`, `generateSummary()`, `postWebhook()`)

### Summary Generation

On call end, Claude receives the full conversation transcript with a structured system prompt:

```
Generate a structured JSON summary of this customer-agent phone call.
Output: {"summary":"...","action_items":[...],"commitments_made":[...],"sentiment":"positive|neutral|negative"}
```

### Webhook Payload

```json
{
  "conversation_id": "uuid",
  "duration_seconds": 180,
  "transcript": [
    {"speaker": "customer", "text": "I had a burst pipe...", "timestamp": "2026-06-06T14:32:05Z"},
    {"speaker": "agent", "text": "Let me check your policy...", "timestamp": "2026-06-06T14:32:12Z"}
  ],
  "summary": "Customer called about water damage claim from burst pipe. Agent confirmed coverage under Section 4.2.1.",
  "action_items": ["File claim within 30 days", "Schedule adjuster visit"],
  "commitments_made": ["Callback within 24 hours"],
  "sentiment": "neutral",
  "suggestions_given": [
    {"suggestion": "Section 4.2.1 covers burst pipe damage. $500 deductible.", "category": "answer", "confidence": 0.95}
  ]
}
```

### Configuration

```bash
export CRM_WEBHOOK_URL=https://hooks.salesforce.com/services/...
export CRM_WEBHOOK_TOKEN=Bearer_xxx
```

---

## 9. G.711 Codec Transcoding

**Source:** `gateway/codec.go` (218 lines)

### Pre-Computed Lookup Tables

Two 256-entry lookup tables computed at `init()` for O(1) per-sample decode:

- **μ-law table:** `ulawToLinear[256]int16` — North American G.711
- **A-law table:** `alawToLinear[256]int16` — European/international G.711

### Decode Performance

```go
func DecodeG711Ulaw(ulaw []byte) []byte {
    pcm := make([]byte, len(ulaw)*2)
    for i, b := range ulaw {
        sample := ulawToLinear[b]  // single array index — O(1)
        binary.LittleEndian.PutUint16(pcm[i*2:i*2+2], uint16(sample))
    }
    return pcm
}
```

**Benchmark:** < 1ms per 20ms frame (160 bytes G.711 → 640 bytes L16 @ 16kHz)

### Resampling

G.711 operates at 8kHz. The STT pipeline requires 16kHz. The gateway resamples using 2x linear interpolation in a single pass:

```
8kHz input: [s0, s1, s2, ...]
16kHz output: [s0, (s0+s1)/2, s1, (s1+s2)/2, s2, ...]
```

### Encode (Return Path)

`EncodeG711Ulaw()` converts L16 PCM back to G.711 for TTS playback over the RTP path, using the standard μ-law compression algorithm with bias and soft clipping.

### Utilities

| Function | Description |
|----------|-------------|
| `DecodeG711Ulaw([]byte)` | μ-law → L16 PCM |
| `DecodeG711Alaw([]byte)` | A-law → L16 PCM |
| `EncodeG711Ulaw([]byte)` | L16 PCM → μ-law |
| `TranscodeFrame([]byte, codec)` | Auto-detect and transcode |
| `ResampleG711toL16([]byte, codec)` | Decode + 8kHz→16kHz resample |
| `estimateSNR([]byte)` | Signal-to-noise ratio estimation |

---

## 10. RFC 2833 DTMF Parsing

**Source:** `gateway/dtmf.go` (270 lines)
**Endpoint:** `/api/dtmf/test`

### RFC 2833 RTP Event Packet Format

```
Byte 0:   Event code (0-15 → 0-9, *, #, A-D)
Byte 1:   E(1 bit) | R(1 bit) | Volume(6 bits)
Byte 2-3: Duration (16-bit, in timestamp units)
```

### Parsing

```go
func ParseRFC2833(payload []byte) *DTMFEvent {
    eventCode := payload[0]
    endBit := (payload[1] & 0x80) != 0  // only process end-of-event
    duration := int(payload[2])<<8 | int(payload[3])
    if !endBit { return nil }           // avoid duplicates
    return &DTMFEvent{Digit: dtmfEventToChar(eventCode), Duration: duration * 1000 / 8000}
}
```

### DTMF Collector

Accumulates digits with inter-digit timeout (default 2 seconds). When the timeout fires without a new digit, the sequence is flushed and delivered to the LLM as `"User typed: 482910"`.

### Goertzel Algorithm Fallback

For legacy PBX systems that don't support RFC 2833, the gateway detects DTMF tones directly from PCM audio using the Goertzel algorithm — computing the power of the 8 DTMF frequencies (697, 770, 852, 941, 1209, 1336, 1477, 1633 Hz) per frame.

---

## 11. Telecom Audio Pipeline (AGC)

**Source:** `gateway/agc.go` (194 lines)

### Three Processing Stages

Every incoming PCM frame passes through:

**Stage 1 — Automatic Gain Control (AGC)**
- Computes current RMS energy of the frame
- Calculates desired gain: `targetRMS / currentRMS`
- Clamps gain between `minGain` (0.1) and `maxGain` (10.0)
- Smooths gain transitions: `attackRate` (0.01) for increasing, `releaseRate` (0.05) for decreasing
- Applies gain with soft clipping at ±32000 to prevent distortion

**Stage 2 — Noise Gate**
- If frame RMS < threshold (default 80): gate closes, frame replaced with comfort noise
- Hold-open mechanism: keeps gate open for 5 frames after last speech detection
- Prevents choppy audio during natural pauses

**Stage 3 — Comfort Noise Generation (CNG)**
- Replaces dead silence with low-level pseudo-random noise (±16 sample range)
- Uses LCG (Linear Congruential Generator) for deterministic noise
- Prevents the "is anyone there?" feeling during pauses

### API

```go
pipeline := NewTelecomAudioPipeline()
processedFrame, isSpeech := pipeline.Process(rawPCM)
```

---

## 12. SIPREC Metadata Parser (RFC 7866)

**Source:** `gateway/siprec_meta.go` (152 lines)

### XML Structure

When an enterprise SBC forks a call via SIPREC, it sends a SIP INVITE containing an XML metadata body that describes the recording session:

```xml
<recording>
  <session session-id="abc123" start-time="2026-06-06T14:32:00Z"/>
  <participant participant-id="p1">
    <nameID><aor>sip:customer@carrier.com</aor></nameID>
  </participant>
  <participant participant-id="p2">
    <nameID><aor>sip:agent@enterprise.com</aor></nameID>
  </participant>
  <stream stream-id="s1" session-id="abc123" participant-id="p1" label="caller"/>
  <stream stream-id="s2" session-id="abc123" participant-id="p2" label="agent"/>
</recording>
```

### Parsed Output

`ParseSIPRECMetadata()` → `RecordingSession` → `BuildContext()` → `SIPRECSessionContext`:

```go
type SIPRECSessionContext struct {
    SessionID    string
    StartTime    time.Time
    Participants map[string]string  // participantID → name/AOR
    Streams      []StreamInfo
    CallerStream *StreamInfo
    AgentStream  *StreamInfo
}
```

This resolves which RTP stream belongs to the caller and which belongs to the agent — required for diarized transcription.

---

## 13. Deterministic Failover State Machine

**Source:** `gateway/failover.go` (264 lines)
**Endpoint:** `/api/failover/status`

### Circuit Breaker Per Service

Four independent circuit breakers monitor LLM, STT, TTS, and ESL services:

| State | Behavior |
|-------|----------|
| **Closed** | Requests pass through normally. Failures increment counter. |
| **Open** | Requests fail fast (no attempt). Timer starts for `resetTimeout`. |
| **Half-Open** | One probe request allowed. Success → Closed. Failure → Open. |

### State Transitions

```go
type CircuitBreaker struct {
    state     atomic.Int32   // lock-free state
    failures  atomic.Int64   // lock-free counter
    threshold int64          // failures before opening (default 3)
    resetTimeout time.Duration  // time in Open before Half-Open (default 30s)
}
```

All transitions use `atomic.CompareAndSwap` — no mutex on the hot path.

### Failover Chain

| Failure | Detection | Response |
|---------|-----------|----------|
| LLM drops | < 1ms (atomic) | Play "One moment please..." → reconnect |
| LLM circuit opens (3 failures) | Immediate | SIP REFER to human queue with X-Failover headers |
| STT fails | < 1ms | Buffer audio frames → retry on Half-Open probe |
| TTS fails | < 1ms | Play static tone via ESL fallback |
| ESL fails | < 1ms | Log error — call will be parked |
| All circuits open | Immediate | SIP REFER to extension 3000 (human queue) |

### Health Monitor

Background goroutine checks all services every 30 seconds and logs circuit states.

---

## 14. Multi-LLM Support

**Source:** `gateway/llm.go` (352 lines)

### LLMClient Interface

```go
type LLMClient interface {
    Chat(ctx, messages, systemPrompt, maxTokens) (string, error)
    StreamChat(ctx, messages, systemPrompt, maxTokens) (<-chan string, error)
    Name() string
}
```

### Implementations

| Provider | Client | Endpoint | Streaming |
|----------|--------|----------|:---------:|
| **Claude (Vertex AI)** | `ClaudeClient` | `publishers/anthropic/models/{model}:streamRawPredict` | SSE |
| **Gemini (Vertex AI)** | `GeminiClient` | `publishers/google/models/{model}:streamGenerateContent?alt=sse` | SSE |

Both use the same GCP Application Default Credentials. Authentication is handled via `google.FindDefaultCredentials()` → `TokenSource.Token()`.

### API

```bash
# Test Claude
POST /api/llm/test
{"provider":"anthropic-vertex","model":"claude-3-5-haiku@20241022","prompt":"Hello"}

# Test Gemini
POST /api/llm/test
{"provider":"gemini-vertex","model":"gemini-2.0-flash","prompt":"Hello"}

# Response
{"response":"Hi there!","latency":1451,"model":"claude:claude-3-5-haiku@20241022"}
```

---

## 15. RAG Knowledge Base

**Source:** `gateway/rag.go` (314 lines)
**Endpoints:** `/api/documents`, `/api/documents/search`
**Vector Store:** ChromaDB

### Document Processing Pipeline

```
Upload text → chunkText() (500 chars, 50 char overlap)
    → simpleEmbed() (384-dim n-gram hash vector)
    → ChromaDB add (ids, documents, embeddings, metadata)
```

### Chunk Strategy

- Target chunk size: 500 characters
- Overlap: 50 characters (context continuity)
- Sentence boundary detection: breaks at `.` or `\n` when possible
- Each chunk stored with metadata: `doc_id`, `doc_name`, `chunk_index`

### Query Flow

1. Customer utterance arrives in co-pilot `coachWorker`
2. `BuildRAGContext()` called with the utterance text
3. ChromaDB queried with the embedding of the utterance
4. Top 3 matching chunks returned with relevance scores
5. Chunks formatted and prepended to Claude's system prompt:

```
Relevant knowledge base context:
[1] (Insurance Claims, 100%): Section 4.2.1: Water damage covered. $500 deductible.
[2] (Billing FAQ, 85%): Late fees can be waived once per year...

Using the knowledge base context above, provide coaching to the agent...
```

### Embedding

Current: n-gram character hash → 384-dimensional vector (MVP). Production should use Vertex AI `text-embedding-004`.

---

## 16. Next.js Operations Dashboard

**Source:** `ui/` (Next.js 16, TypeScript, Tailwind CSS, shadcn/ui)
**Port:** 3000

### Pages

| Page | Route | Features |
|------|-------|----------|
| **Command Center** | `/` | 4 stat cards, active calls table, recent completions |
| **Agent Roster** | `/agents` | Agent CRUD, expertise badges, status indicators, add agent dialog |
| **Call History** | `/calls` | Paginated table, sentiment/mode badges, search |
| **Live Operations** | `/calls/live` | SSE-connected live transcript, co-pilot suggestions panel, dial pad |
| **Knowledge Base** | `/documents` | Upload dropzone, document list, RAG search test panel |
| **Configuration** | `/settings` | LLM model management, system prompt editor, SBC settings, service status |

### Design

- Dark theme: deep navy (`#0a0e1a`), cyan accents (`#06b6d4`)
- JetBrains Mono for data readouts, DM Sans for text
- Emerald (positive), amber (warning), rose (negative) sentiment colors
- Violet co-pilot suggestion panel
- Pulse/glow animations for active indicators

### Live Ops (SSE Integration)

The Live Operations page connects to the gateway's SSE endpoint via `EventSource`. When a `call_id` is provided, the page streams:

- **Left panel:** Live transcript with speaker labels (cyan = customer, emerald = agent)
- **Right panel:** Co-pilot suggestions with category badges and confidence scores
- **Bottom:** Dial pad for outbound call origination via `POST /call`

On call end, the summary appears in the transcript panel with action items and sentiment.

---

## 17. SBC Connectivity & Enterprise Profiles

**Source:** `freeswitch/config/sip_profiles/enterprise/`

### Pre-Configured Profiles

| SBC | File | Codecs | Features |
|-----|------|--------|----------|
| **Cisco CUBE** | `cisco-cube.xml` | PCMU, PCMA, G722, G729 | TLS, session timers, OPTIONS keepalive |
| **AudioCodes Mediant** | `audiocodes.xml` | PCMU, PCMA, G722, G729 | Registration mode, NAT traversal, rport |

### Configuration

```bash
# IP-based peering
SBC_ADDRESS=cube.internal make sbc-config

# Registration mode
SBC_ADDRESS=audiocodes.internal SBC_REGISTER=true SBC_USERNAME=trunk1 SBC_PASSWORD=s3cret make sbc-config

# Twilio SIP Trunk
SBC_ADDRESS=trunk.pstn.twilio.com SBC_REGISTER=true SBC_USERNAME=sid SBC_PASSWORD=token make sbc-config
```

### IP ACL

Trusted SBC IPs configured in `freeswitch/config/autoload_configs/acl.conf.xml`. Default: RFC1918 + Docker NAT ranges. Production: restrict to SBC IPs only.

### Entrypoint Script

`freeswitch/entrypoint.sh` injects SBC environment variables (`SBC_ADDRESS`, `SBC_USERNAME`, `SBC_PASSWORD`, etc.) into FreeSWITCH vars.xml at container startup via `sed`.

---

## 18. Deployment Modes

### Kustomize Overlays

| Mode | Overlay | Network | LLM | Command |
|------|---------|---------|-----|---------|
| **Docker Compose** | N/A | Local | Vertex AI | `docker compose up` |
| **KinD** | `k8s/base` | Local K8s | Vertex AI | `make all` |
| **On-Premises** | `k8s/overlays/on-prem` | Enterprise DC | Vertex AI | `kubectl apply -k k8s/overlays/on-prem` |
| **Air-Gapped** | `k8s/overlays/air-gapped` | No internet | Local Ollama | `kubectl apply -k k8s/overlays/air-gapped` |

### Air-Gapped Mode

Zero internet dependency. All AI services run locally:
- **LLM:** Ollama (Llama, Mistral, Phi) instead of Vertex AI
- **STT:** Whisper (already local)
- **TTS:** Piper (already local)
- **RAG:** ChromaDB (already local)

---

## 19. SDKs (Python & TypeScript)

### Python SDK

**Location:** `sdk/python/`
**Install:** `pip install -e sdk/python`
**Dependency:** `httpx`

Covers all 25+ API endpoints with typed dataclass models. SSE streaming via `httpx.stream()` with callback support.

### TypeScript SDK

**Location:** `sdk/typescript/`
**Install:** `cd sdk/typescript && npm install && npm run build`
**Dependency:** None (uses native `fetch` and `EventSource`)

Full TypeScript types for all models. `EventSource`-based SSE streaming works in both browser and Node.js. Includes Next.js usage examples (API routes + React components).

### Shared Coverage

Both SDKs cover: agents, calls, documents/RAG, LLM config/test, robocall detection/blocklist, PII masking, voice biometrics, self-service actions/webhooks, DTMF, failover status, dashboard stats, health check, and SSE event streaming.

---

## 20. Voice Sentiment Analysis

**Source:** `gateway/sentiment.go`

Acoustic emotion detection from raw PCM audio — no ML model, pure signal processing. Runs on every audio frame in both co-pilot and interactive pipelines.

### Acoustic Features

| Feature | Method | What it measures |
|---------|--------|-----------------|
| **Pitch (F0)** | Autocorrelation | Fundamental frequency — rising pitch indicates frustration |
| **RMS Energy** | Per-frame calculation | Volume level — increasing energy indicates agitation |
| **Energy Trend** | First-half vs second-half average | Rising, falling, or stable over call duration |
| **Speaking Rate** | Words per minute from Whisper | Fast (>150 wpm) = urgency, slow (<100 wpm) = confusion |
| **Silence Ratio** | Speech frames vs total frames | High silence = disengagement or hesitation |
| **Pitch Variance** | Standard deviation of F0 estimates | High jitter = emotional instability |

### Derived Signals

| Signal | Formula | Threshold |
|--------|---------|-----------|
| **Agitation** | 0.3 x energy + 0.4 x pitch_jitter + 0.3 x speed | > 0.5 = agitated |
| **Frustration** | 0.4 x agitation + 0.3 x rising_energy + 0.3 x pitch_jitter | > 0.6 = frustrated |
| **Engagement** | 1.0 - silence_ratio | > 0.7 = engaged |

### Multi-Modal Sentiment

Combines Claude's text-based sentiment with acoustic voice sentiment. If voice frustration > 0.6 but text says "neutral", overrides to "negative". Catches sarcasm and controlled anger that text analysis misses.

### API

- `GET /api/copilot/active` — per-session real-time voice sentiment
- `/siprec/events` SSE — `summary` event includes `voice_sentiment` object
- Gateway logs — agitation, frustration, pitch, speaking rate per call

### SentimentResult Schema

```json
{
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
```

---

## 21. Standalone SIPREC Helper

**Source:** `gateway/sipserver.go`, `gateway/rtplistener.go`

Native SIP+RTP endpoint that accepts SIPREC INVITEs directly — no FreeSWITCH dependency. The SBC/PBX owns the call; VoiceAgent is a read-only audio observer.

### Architecture

```
SBC/PBX (owns the call)
    |
    +-- SIPREC INVITE (SIP + RTP) --> VoiceAgent Gateway :5060
                                           |
                                           +-- Parse RFC 7866 XML metadata
                                           +-- Receive RTP (G.711) on port 30000-30050
                                           +-- Decode G.711 -> PCM via LUT (< 1ms)
                                           +-- VAD -> Whisper STT -> Claude coaching
                                           +-- Voice sentiment analysis
                                           +-- SSE -> Agent dashboard
```

### Configuration

| Variable | Value | Effect |
|----------|-------|--------|
| `VOICEAGENT_MODE=standalone` | Enables SIP server | No FreeSWITCH needed |
| `VOICEAGENT_MODE=gateway` | Default | FreeSWITCH + WebSocket mode |

### SBC Configuration (one line)

| SBC | Config |
|-----|--------|
| **Cisco CUBE** | `media-recording <VOICEAGENT_IP> port 5060` |
| **AudioCodes** | Recording Server = `<VOICEAGENT_IP>:5060` |
| **Oracle SBC** | destination = `sip:<VOICEAGENT_IP>:5060` |
| **Kamailio** | `siprec_start_recording("sip:<VOICEAGENT_IP>:5060")` |

### Deployment

```bash
docker compose -f docker-compose.helper.yml up -d   # 8 services, no FreeSWITCH
```

### SIP Stack

- **SIP UAS:** github.com/emiago/sipgo
- **RTP:** github.com/pion/rtp
- **SDP:** github.com/pion/sdp
- **Codec:** Existing G.711 LUT decoder

### Network Requirements

| Port | Protocol | Direction | Purpose |
|------|----------|-----------|---------|
| 5060 | UDP+TCP | SBC -> VoiceAgent | SIP SIPREC INVITE/BYE |
| 30000-30050 | UDP | SBC -> VoiceAgent | RTP audio streams |
| 8080 | TCP | Browser -> VoiceAgent | HTTP API + SSE |
