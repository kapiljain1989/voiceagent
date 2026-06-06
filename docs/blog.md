# How I Built a Telecom-Native AI Gateway That Enterprise SBCs Actually Respect

*A deep technical walkthrough of building an enterprise-grade AI call center platform from scratch with Go, FreeSWITCH, and real SIP infrastructure — not another cloud wrapper.*

---

There's a gap in the market that most AI voice startups don't see because they've never sat in an enterprise Network Operations Center.

Every week, a new "AI voice agent" startup launches. They wrap OpenAI's API in a WebSocket, point a Twilio number at it, and call it revolutionary. And every week, the enterprise telecom manager at a bank, insurer, or healthcare system looks at these products, checks the compliance box, and says: **"This can't run inside our network."**

I built the system that can.

## The Problem Nobody Talks About

Standard AI voice platforms (Vapi, Retell, Bland) operate as cloud services. Your customer's voice leaves your network, hits a third-party server, gets processed, and comes back. For a consumer app, that's fine. For an enterprise with PCI-DSS compliance, HIPAA requirements, or a government security clearance — it's a non-starter.

But the technical barrier goes deeper than compliance. Enterprise voice infrastructure runs on protocols that web developers have never heard of:

- **SIP** — Session Initiation Protocol (the HTTP of telephony)
- **RTP** — Real-time Transport Protocol (the actual audio packets)
- **SIPREC** — Session Recording Protocol (RFC 7866 — how SBCs fork call audio)
- **RFC 2833** — How DTMF button presses travel over RTP
- **G.711 μ-law/A-law** — The 1972 codec that still carries 90% of the world's phone calls

If you don't speak these protocols natively, enterprise telecom teams won't let you near their Cisco CUBE or AudioCodes SBC.

## The Architecture

I chose Go for a specific reason: it's the only modern language that gives you concurrent network I/O, zero-copy buffer management, and sub-millisecond latency — all without a garbage collection pause that kills a real-time audio stream.

Here's what the system looks like:

```
                     Enterprise Network Boundary
                              │
  PSTN ──▶ SBC (Cisco/AudioCodes) ──▶ FreeSWITCH ──▶ Go Media Gateway
           │                                              │
           │ Private SIP trunk                    ┌───────┴───────┐
           │ (never leaves the DC)                │  14 Go files  │
           │                                      │  doing the    │
           │                                      │  impossible   │
           ▼                                      └───────┬───────┘
  Customer hears Claude's voice                           │
  Agent sees co-pilot suggestions                   ┌─────┴─────┐
  CRM gets auto-generated summary               Whisper  Piper  Claude
                                                 (local)  (local) (Vertex AI)
```

The Go gateway sits at the media plane — the actual audio bytes flow through it. That position gives access to data that cloud wrappers simply cannot touch.

## The Three Things That Make Enterprise Telecom Teams Say "Finally"

### 1. Native RFC 2833 DTMF Parsing

When a customer is on a noisy street, ASR models hallucinate. The customer gets frustrated and starts pressing buttons on their keypad. Standard AI voice bots try to "hear" the DTMF tone through the audio — which breaks constantly.

My gateway intercepts the RFC 2833 RTP event packets directly. Four bytes per digit. Zero ambiguity:

```go
// RFC 2833 payload: [event_code, E|R|volume, duration_hi, duration_lo]
func ParseRFC2833(payload []byte) *DTMFEvent {
    eventCode := payload[0]      // 0-9, *, #, A-D
    endBit := (payload[1] & 0x80) != 0
    duration := int(payload[2])<<8 | int(payload[3])
    // Only process end-of-event to avoid duplicates
    if !endBit { return nil }
    return &DTMFEvent{Digit: dtmfEventToChar(eventCode), Duration: duration}
}
```

When the customer types their account number `482910` on the keypad, the LLM receives `"User typed: 482910"` — clean, accurate, no audio processing.

For legacy PBX systems that don't support RFC 2833, the gateway falls back to Goertzel algorithm inband DTMF detection — detecting the frequency pairs (697+1209 Hz = digit "1") directly from the PCM samples.

### 2. G.711 Codec Transcoding at Wire Speed

Enterprise phone calls use G.711 — a codec from 1972 that compresses audio to 64 kbps using μ-law (North America) or A-law (Europe) companding. Every AI voice startup sends this compressed audio to a cloud transcoding service, adding 100-200ms of latency.

My gateway decodes G.711 in pure Go with a pre-computed 256-entry lookup table:

```go
var ulawToLinear [256]int16  // computed once at init()

func DecodeG711Ulaw(ulaw []byte) []byte {
    pcm := make([]byte, len(ulaw)*2)
    for i, b := range ulaw {
        sample := ulawToLinear[b]  // O(1) table lookup
        binary.LittleEndian.PutUint16(pcm[i*2:i*2+2], uint16(sample))
    }
    return pcm
}
```

One table lookup per sample. No FFmpeg. No cloud service. Sub-millisecond for a 20ms frame. The audio goes from G.711 → L16 PCM → 16kHz resample in a single pass, entirely in memory.

### 3. Deterministic Failover That Saves Calls

This is the one that makes enterprise buyers sign contracts.

In a contact center, a dropped call means lost revenue. AI models fail, hallucinate, or drop WebSocket connections mid-sentence. My gateway has a circuit breaker per service with atomic state transitions:

```go
type CircuitBreaker struct {
    state     atomic.Int32   // closed → open → half-open
    failures  atomic.Int64
    threshold int64
}
```

When the LLM WebSocket drops:
1. Circuit breaker trips in < 1ms (atomic compare-and-swap)
2. Gateway plays a pre-cached audio file: *"One moment please..."*
3. Simultaneously sends SIP REFER to transfer the caller to a human queue
4. Custom SIP headers carry the conversation context:

```
X-Failover-Reason: llm_timeout
X-Failover-Summary: "Customer was asking about water damage claim. Policy 4.2.1 applies."
X-Failover-Transcript: "[customer] my pipe burst | [ai] checking coverage..."
```

The human agent picks up the phone and sees the summary on their Cisco softphone screen. The customer never has to repeat their story.

## The Co-Pilot: Where RAG Meets Real-Time Telephony

The interactive AI agent is the flashy demo. The co-pilot is where the money is.

Enterprise call centers have human agents. They don't want to replace them — they want to make them faster. The co-pilot mode silently observes live calls using SIPREC (the same protocol the SBC uses for call recording) and provides real-time coaching:

```
Customer: "Does my insurance cover water damage from a burst pipe?"
    │
    ▼
Gateway receives both audio legs (caller + agent) via SIPREC
    │
    ▼
Whisper transcribes both speakers with diarization labels
    │
    ▼
RAG queries ChromaDB → finds Policy Section 4.2.1
    │
    ▼
Claude generates suggestion with the policy context injected
    │
    ▼
Agent's screen shows:
  "Section 4.2.1 covers burst pipe damage.
   $500 deductible for Tier 2. Claims within 30 days.
   Emergency repairs up to $1000 pre-approved."
```

The agent doesn't fumble through a 200-page policy manual. They read the answer off their screen and sound like an expert. Call handle time drops. Customer satisfaction rises. The enterprise renews the contract.

## Live PII Masking: The Compliance Feature That Closes Deals

When a customer reads their credit card number over the phone, that audio gets recorded. If that recording is stored without PCI compliance, the enterprise faces massive fines.

My gateway masks PII in real-time — after Whisper transcribes the audio but before the text reaches Claude or gets stored:

```
Input:  "My card number is 4111 1111 1111 1111 and CVV is 123"
Output: "My card number is XXXX-XXXX-XXXX-#### and [CVV REDACTED]"
```

Seven detection patterns run on every transcript: credit cards, SSNs, CVVs, dates of birth, account numbers, and spoken variants like "my social security number is...". The corresponding audio frames can be silenced before they reach the call recording system.

This single feature — which takes about 200 lines of regex in Go — is often the reason an enterprise chooses this platform over a cloud alternative. Not because the AI is better. Because the compliance team signs off.

## The Numbers

| Metric | Value |
|--------|-------|
| Gateway binary size | 15 MB (distroless container) |
| G.711 decode latency | < 1ms per 20ms frame |
| DTMF detection accuracy | 100% (RFC 2833 packet parsing) |
| Circuit breaker trip time | < 1ms (atomic state transition) |
| Speech-to-suggestion (co-pilot) | ~2 seconds |
| Speech-to-voice response | ~3 seconds |
| PII detection patterns | 7 (credit card, SSN, CVV, DOB, account, spoken variants) |
| Robocall keywords | 28 weighted phrases |
| Voice fingerprint dimensions | 32 spectral features |
| Total Go source files | 14 |
| Total lines of Go | ~5,000 |
| Docker services | 7 (gateway, FreeSWITCH, Whisper, Piper, PostgreSQL, ChromaDB, UI) |

## The Deployment Story

The platform ships as Kustomize overlays. Three deployment modes:

**Docker Compose** — for demos and local development:
```bash
docker compose up  # 7 containers, fully functional in 30 seconds
```

**On-Premises** — for enterprise data centers:
```bash
kubectl apply -k k8s/overlays/on-prem
```

**Air-Gapped** — for environments with zero internet access:
```bash
kubectl apply -k k8s/overlays/air-gapped  # uses local Ollama instead of Vertex AI
```

Once this system is inside an enterprise's private network, talking directly to their Cisco CUBE via a private SIP trunk, the compliance barrier to replace it is massive. Every competitive evaluation requires a new security audit, a new network penetration test, and a new vendor risk assessment. The first mover advantage in enterprise telecom infrastructure is measured in years, not months.

## The Open-Core Model

The GitHub repo is public. The core gateway — SIP-to-WebSocket bridging, basic VAD, interactive AI agent, co-pilot, RAG — is open source. Enterprise teams download it, run it on KinD, test it locally, and confirm the engineering is solid.

Then they need production features: SIPREC RFC 7866 parsing, PII masking, voice biometrics, DTMF handling, telecom AGC, failover state machine, Cisco/AudioCodes profiles. These require an enterprise license.

The open-source core is the bait. The enterprise extensions are the hook. The network-level integration is the moat.

## What's Next

- **Streaming STT** — WebSocket-based Whisper for sub-second transcription
- **STIR/SHAKEN** — SIP attestation headers for cryptographic caller verification
- **ML Robocall Model** — Neural network trained on audio features, replacing keyword heuristics
- **Vertex AI Embeddings** — Replace the n-gram hashing with `text-embedding-004` for production RAG
- **WebRTC Softphone** — Built into the Next.js dashboard for browser-based agent calls

---

*The entire platform is open source at [github.com/kapiljain1989/voiceagent](https://github.com/kapiljain1989/voiceagent). Star it, fork it, deploy it into your KinD cluster, and tell me what breaks.*

*If you're an enterprise telecom team evaluating AI voice solutions and need something that runs inside your network boundary — let's talk.*
