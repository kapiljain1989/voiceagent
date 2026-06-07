# LinkedIn Posts

---

## Post 1: Launch Announcement (Primary)

I built a telecom-native AI gateway that enterprise SBCs actually respect.

Every week, a new "AI voice agent" startup wraps OpenAI's API in a WebSocket and calls it revolutionary. And every week, the enterprise telecom manager checks the compliance box and says: "This can't run inside our network."

So I built the system that can.

VoiceAgent is an open-source B2BUA media proxy written in Go that sits directly on the SIP/RTP media plane — where cloud wrappers can't reach.

What it does:

→ Answers customer calls with AI (Whisper STT → Claude/Gemini → Piper TTS)
→ Coaches human agents in real-time with RAG-powered suggestions via SIPREC
→ Parses RFC 2833 DTMF digits from raw RTP packets — 100% accurate
→ Transcodes G.711 μ-law at wire speed using pre-computed lookup tables
→ Masks credit cards and SSNs before they reach the LLM or call recording
→ Detects robocalls with 3-layer scoring (blocklist + audio + keywords)
→ Transfers angry callers to humans with full conversation context in SIP headers
→ Fails over in < 1ms when the AI drops — plays hold audio, sends SIP REFER

The stack: Go, FreeSWITCH, Whisper, Piper, Claude/Gemini on Vertex AI, PostgreSQL, ChromaDB, Redis, Prometheus, Next.js.

17 Go source files. 10 containerized services. Deploys on-prem, in private VPCs, or fully air-gapped.

Pre-configured SBC profiles for Cisco CUBE and AudioCodes included.

The open-source core is the bait. The enterprise extensions are the hook. The network-level integration is the moat.

GitHub: https://github.com/kapiljain1989/voiceagent

If you're building voice infrastructure — not voice wrappers — star it and tell me what breaks.

#VoIP #SIP #AI #Telecom #Go #Golang #OpenSource #Enterprise #CallCenter #FreeSWITCH #B2BUA

---

## Post 2: Technical Deep-Dive (Follow-up)

Why do enterprise contact centers reject every AI voice startup?

Because they all operate the same way: your customer's voice leaves your network, hits a third-party cloud, gets processed, and comes back. For a bank with PCI-DSS compliance or a hospital with HIPAA requirements — that's a non-starter.

I spent the last month building VoiceAgent — a Go-based media gateway that lives inside the enterprise network boundary.

Three things that make telecom teams say "finally":

1. RFC 2833 DTMF Parsing
When ASR fails on a noisy street, customers press buttons. Standard AI bots try to "hear" the tone through audio. My gateway intercepts the raw RTP event packets. Four bytes per digit. Zero ambiguity.

2. Sub-1ms G.711 Transcoding
Every phone call uses a 1972 codec. AI startups send it to a cloud transcoding service — adding 150ms of latency. My gateway decodes it with a 256-entry lookup table. One array index per sample.

3. Deterministic Failover
When the AI WebSocket drops mid-sentence, my circuit breaker trips in < 1ms (atomic compare-and-swap), plays a hold message, and sends a SIP REFER to the human queue — with the full conversation transcript in X-Transfer-Summary headers.

The result: the human agent picks up the phone and sees the context on their Cisco softphone screen. The customer never repeats their story.

Plus: SIPREC co-pilot, RAG knowledge base, PII masking, voice biometrics, robocall detection, self-service actions, Next.js dashboard, Python + TypeScript SDKs.

Open source: https://github.com/kapiljain1989/voiceagent
Technical blog: https://github.com/kapiljain1989/voiceagent/blob/main/docs/blog.md

Built for network engineers who ship infrastructure, not web developers who ship wrappers.

#Telecom #VoIP #SIP #AI #Enterprise #Go #OpenSource #B2BUA #ContactCenter

---

## Post 3: Short Format (High Engagement)

I open-sourced an AI call center platform that runs inside your private network.

Not a cloud wrapper. A real B2BUA media proxy.

→ 17 Go source files
→ 10 containerized services
→ RFC 2833 DTMF parsing from raw RTP packets
→ G.711 transcoding at < 1ms per frame
→ PCI/HIPAA PII masking before data leaves the box
→ Circuit breaker failover in < 1ms
→ Cisco CUBE + AudioCodes SBC profiles included
→ Deploys on-prem or fully air-gapped

GitHub: https://github.com/kapiljain1989/voiceagent

Star it. Break it. Ship it.

#AI #VoIP #Telecom #OpenSource #Go

---

## Post 4: Problem-Solution (For Non-Technical Audience)

Every AI voice startup has the same pitch: "We'll automate your call center."

Every enterprise CTO has the same response: "Can it run inside our network?"

The answer is always no. Until now.

I built VoiceAgent — an open-source AI gateway that deploys directly into your data center. Your customer's voice never leaves your network boundary.

What it does:
• AI answers routine calls automatically
• Coaches human agents in real-time during complex calls
• Detects spam calls before they reach your team
• Masks credit card numbers before they hit any recording
• Transfers angry callers to humans with full context — no "please repeat your story"
• Keeps working even when the AI service drops

The tech: Go, FreeSWITCH, Whisper, Claude, containers, one command to deploy.

The code: https://github.com/kapiljain1989/voiceagent

If your compliance team has ever blocked an AI vendor — this was built for you.

#AI #Enterprise #CallCenter #Compliance #OpenSource
