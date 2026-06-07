# Demo 10: Co-Pilot Agent Assist — Live Session

**Duration:** ~3 minutes
**Format:** Screen recording (terminal + browser side-by-side)
**Audience:** Call center managers, customer experience leaders

---

## Scene 1: Setup (0:00 - 0:30)

**Terminal:**
```bash
# Index knowledge base documents
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")

curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/documents \
  -H 'Content-Type: application/json' \
  -d '{"name":"Insurance Policy","category":"policy","content":"Water damage from burst pipes is covered. Deductible is $500 for Tier 2. Claims must be filed within 30 days. Emergency repairs up to $1000 pre-approved."}'
```

**Narration:**
> "Co-pilot mode uses SIPREC to passively observe live calls between a customer and an agent. It transcribes both sides, queries a RAG knowledge base, and sends real-time coaching suggestions to the agent's dashboard."

---

## Scene 2: Start Co-Pilot Session (0:30 - 1:00)

**Terminal:**
```bash
cd test && go run simcopilot.go http://localhost:8080
```

**Browser:** Open http://localhost:3000/calls/live — paste the Call ID, click CONNECT

**Narration:**
> "We're simulating a two-party call. The SIPREC fork sends separate audio streams for the caller and the agent. The gateway labels each stream using the RFC 7866 metadata XML, so the transcript shows who said what."

---

## Scene 3: Live Coaching (1:00 - 2:00)

**Browser:** Watch the live transcript panel and co-pilot suggestions panel

**Narration:**
> "As the customer speaks, the transcript appears in real-time on the left. The co-pilot queries ChromaDB for relevant knowledge base articles and sends coaching suggestions to the agent via Server-Sent Events. The agent sees policy details, recommended responses, and action items — all without the customer knowing."

---

## Scene 4: Post-Call Summary (2:00 - 2:45)

**Terminal:** Show the post-call summary output from gateway logs
```bash
docker logs voiceagent-gateway-1 --tail=30 2>&1 | grep -A5 "summary"
```

**Narration:**
> "When the call ends, the gateway generates an AI-powered summary with key topics, action items, and customer sentiment. This gets pushed to the CRM webhook automatically. The summary includes both sides of the conversation — diarized and timestamped."

---

## Closing (2:45 - 3:00)

**Narration:**
> "Co-pilot agent assist runs entirely on-premises. No customer audio leaves your network. PII is masked before it reaches the LLM. Enterprise SBCs like Cisco CUBE and AudioCodes Mediant connect via standard SIPREC — no proprietary protocols."
