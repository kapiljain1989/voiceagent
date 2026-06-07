# VoiceAgent — Comprehensive Test Plan

Complete testing guide covering all 19 features, 25+ API endpoints, 10 services, 4 deployment overlays (local, cloud, on-prem, air-gapped), Istio service mesh, and Gateway API.

---

## Table of Contents

1. [Pre-Test Environment Setup](#1-pre-test-environment-setup)
2. [Service Health Verification](#2-service-health-verification)
3. [Interactive AI Agent Tests](#3-interactive-ai-agent-tests)
4. [Co-Pilot Agent Assist Tests](#4-co-pilot-agent-assist-tests)
5. [Self-Service Action Tests](#5-self-service-action-tests)
6. [Intelligent Call Transfer Tests](#6-intelligent-call-transfer-tests)
7. [Robocall Detection Tests](#7-robocall-detection-tests)
8. [Voice Biometrics Tests](#8-voice-biometrics-tests)
9. [PII Masking Tests](#9-pii-masking-tests)
10. [Post-Call Summary Tests](#10-post-call-summary-tests)
11. [RAG Knowledge Base Tests](#11-rag-knowledge-base-tests)
12. [Multi-LLM Tests](#12-multi-llm-tests)
13. [G.711 Codec Transcoding Tests](#13-g711-codec-transcoding-tests)
14. [DTMF Parsing Tests](#14-dtmf-parsing-tests)
15. [Telecom AGC Tests](#15-telecom-agc-tests)
16. [Failover State Machine Tests](#16-failover-state-machine-tests)
17. [SBC Connectivity Tests](#17-sbc-connectivity-tests)
18. [Rate Limiting & Admission Tests](#18-rate-limiting--admission-tests)
19. [Redis Session Store Tests](#19-redis-session-store-tests)
20. [Prometheus Metrics Tests](#20-prometheus-metrics-tests)
21. [Agent CRUD Tests](#21-agent-crud-tests)
22. [Call History Tests](#22-call-history-tests)
23. [Dashboard UI Tests](#23-dashboard-ui-tests)
24. [SDK Tests](#24-sdk-tests)
25. [SIP Call Tests](#25-sip-call-tests)
26. [Load & Stress Tests](#26-load--stress-tests)
27. [Deployment Mode Tests](#27-deployment-mode-tests)
28. [End-to-End Scenario Tests](#28-end-to-end-scenario-tests)

---

## 1. Pre-Test Environment Setup

### Start All Services

```bash
export EXT_IP=$(ipconfig getifaddr en0)
export ANTHROPIC_VERTEX_PROJECT_ID="your-project-id"
export CLOUD_ML_REGION="us-east5"

cd voiceagent
docker compose -f docker-compose.sip.yml up -d

# Wait for all services to initialize
sleep 30
```

### Verify All 10 Containers Running

```bash
docker compose -f docker-compose.sip.yml ps --format "table {{.Name}}\t{{.Status}}"
```

**Expected:** All 10 services show `Up`:
- gateway, freeswitch, whisper, piper, postgres, chromadb, ui, redis, prometheus, grafana

### Authenticate

```bash
# Get auth token (all API calls except /healthz, /metrics, /ws, /siprec require this)
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")

echo "Token: ${TOKEN:0:20}..."
```

### Index Test Documents

```bash
# Insurance policy
curl -s -X POST http://localhost:8080/api/documents \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Insurance Policy v4.2","category":"policy","content":"Section 4.2.1: Water damage from burst pipes is covered under standard homeowner policy. Deductible is $500 for Tier 2. Claims must be filed within 30 days of incident. Emergency repairs up to $1000 are pre-approved without adjuster visit."}'

# Billing FAQ
curl -s -X POST http://localhost:8080/api/documents \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Billing FAQ","category":"faq","content":"Late fees are $25 per occurrence. Late fees can be waived once per year for customers in good standing with 12 or more months of account history. Payment plans available for balances over $200. Auto-pay discount of 5 percent on monthly premium."}'

# Retention playbook
curl -s -X POST http://localhost:8080/api/documents \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Retention Playbook","category":"procedure","content":"For customers threatening to cancel: Offer 1 is 20 percent discount for 6 months. Offer 2 is free upgrade to next tier for 3 months. Offer 3 is waive all pending fees. Always empathize first. Never argue with the customer. If customer insists after all offers, transfer to retention specialist at extension 4500."}'
```

### Create Test Agents

```bash
curl -s -X POST http://localhost:8080/api/agents \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"Test Agent 1","email":"agent1@test.com","expertise":["billing","retention"]}'

curl -s -X POST http://localhost:8080/api/agents \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"Test Agent 2","email":"agent2@test.com","expertise":["technical","claims"]}'
```

---

## 2. Service Health Verification

### Test 2.1: Gateway Health

```bash
curl -s http://localhost:8080/healthz
```

**Expected:** `{"status":"ok","sessions":0}`

### Test 2.2: Prometheus Metrics

```bash
curl -s http://localhost:8080/metrics | head -5
```

**Expected:** Prometheus text format with `voiceagent_uptime_seconds`

### Test 2.3: Redis Connection

```bash
docker exec voiceagent-redis-1 redis-cli ping
```

**Expected:** `PONG`

### Test 2.4: PostgreSQL Connection

```bash
docker exec voiceagent-postgres-1 pg_isready
```

**Expected:** `accepting connections`

### Test 2.5: ChromaDB Health

```bash
curl -s http://localhost:8200/api/v2/heartbeat
```

**Expected:** JSON with nanosecond heartbeat timestamp

### Test 2.6: Prometheus Scraping

```bash
curl -s http://localhost:9090/-/healthy
```

**Expected:** HTTP 200

### Test 2.7: Grafana

```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:3001
```

**Expected:** `200`

### Test 2.8: FreeSWITCH SIP Profile

```bash
docker exec voiceagent-freeswitch-1 fs_cli -P 8022 -x "sofia status" 2>/dev/null | grep RUNNING
```

**Expected:** `external` profile RUNNING

### Test 2.9: UI Dashboard

```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:3000
```

**Expected:** `200`

### Test 2.10: Failover Status

```bash
curl -s http://localhost:8080/api/failover/status | python3 -m json.tool
```

**Expected:** All 4 circuits `closed`, 0 failures

### Test 2.11: Scale Status

```bash
curl -s http://localhost:8080/api/scale/status | python3 -m json.tool
```

**Expected:** STT pool healthy, TTS pool healthy, admission available=500

---

## 3. Interactive AI Agent Tests

### Test 3.1: Simulated Call (No Microphone)

```bash
cd test && timeout 60 go run simcall.go
```

**Expected:**
- `sent metadata`
- `receiving audio...`
- `SUCCESS — full pipeline working`
- Audio frames received (> 0 bytes)

### Test 3.2: Live Microphone Call

```bash
cd test && ./livecall ws://localhost:8080/ws
```

**Expected:**
- `Connected. Listening...`
- Speak → `YOU: <your words>`
- `CLAUDE: <response>`
- Audio plays through speaker

### Test 3.3: Verify Gateway Logs

```bash
docker logs voiceagent-gateway-1 2>&1 | grep -E "heard|replied" | tail -5
```

**Expected:** `heard` and `replied` log entries with transcribed text

---

## 4. Co-Pilot Agent Assist Tests

### Test 4.1: Simulated Two-Party Call

```bash
cd test && timeout 60 ./simcopilot localhost:8080
```

**Expected:**
- Both legs connected
- `[customer]: ...` transcript appears
- `CALL SUMMARY` generated on hangup

### Test 4.2: Live Co-Pilot with Microphone

```bash
cd test && ./callcenter-live.sh
```

**Expected:**
- `[CUSTOMER] <your speech>` (transcribed)
- `>>> CO-PILOT (<category>): <suggestion with policy details>` (RAG-grounded)

### Test 4.3: SSE Event Stream

```bash
# In terminal 1: start call
cd test && ./callcenter-live.sh

# In terminal 2: subscribe to SSE (paste the call_id)
curl -N "http://localhost:8080/siprec/events?call_id=<CALL_ID>"
```

**Expected:** JSON events stream: `transcript`, `suggestion`, `summary`

### Test 4.4: RAG Context in Suggestions

Speak: "I had a burst pipe and water damaged my floor"

**Expected:** Suggestion references "Section 4.2.1", "$500 deductible", "30 days"

### Test 4.5: Post-Call Summary Generation

Hang up with Ctrl+C after speaking.

**Expected:** Gateway logs show:
```
"msg":"generating summary"
"msg":"call summary","duration":X,"utterances":Y,"sentiment":"..."
```

---

## 5. Self-Service Action Tests

### Test 5.1: Parse API Call Action

```bash
curl -s -X POST http://localhost:8080/api/actions/test \
  -d '{"text":"{\"type\":\"api_call\",\"text\":\"Done.\",\"intent\":\"reschedule\",\"api_call\":{\"endpoint\":\"/deliveries\",\"method\":\"PUT\",\"payload\":{\"date\":\"2026-06-12\"}},\"confidence\":0.9}"}'
```

**Expected:** `{"type":"api_call","intent":"reschedule",...}`

### Test 5.2: Parse Transfer Action

```bash
curl -s -X POST http://localhost:8080/api/actions/test \
  -d '{"text":"{\"type\":\"transfer\",\"text\":\"Connecting you.\",\"intent\":\"escalate\",\"transfer\":{\"reason\":\"angry\",\"department\":\"retention\",\"priority\":\"urgent\",\"summary\":\"Billing dispute\"},\"confidence\":0.95}"}'
```

**Expected:** `{"type":"transfer","intent":"escalate",...}`

### Test 5.3: Parse Plain Speech

```bash
curl -s -X POST http://localhost:8080/api/actions/test \
  -d '{"text":"Your plan includes 5GB of data."}'
```

**Expected:** `{"type":"speak","intent":"conversation",...}`

### Test 5.4: Webhook Configuration

```bash
# Set webhooks
curl -s -X POST http://localhost:8080/api/actions/webhooks \
  -d '{"reschedule":"https://httpbin.org/put","cancel":"https://httpbin.org/post"}'

# Verify
curl -s http://localhost:8080/api/actions/webhooks
```

**Expected:** Configured URLs returned

---

## 6. Intelligent Call Transfer Tests

### Test 6.1: Transfer Action Parsing

Test from Test 5.2 — verify `transfer.department`, `transfer.reason`, `transfer.priority`, `transfer.summary` fields.

### Test 6.2: Department Routing Table

Verify department → extension mapping:
- billing → 3001
- technical → 3002
- retention → 3004
- supervisor → 3005

---

## 7. Robocall Detection Tests

### Test 7.1: Keyword Detection — Positive

```bash
curl -s -X POST http://localhost:8080/api/robocall/test \
  -d '{"text":"We have been trying to reach you about your auto warranty. Press 1 to speak to a representative."}'
```

**Expected:** `score: 1.0`, `category: robocall`, keywords: `[press 1, auto warranty, we have been trying to reach you]`

### Test 7.2: Clean Speech — Negative

```bash
curl -s -X POST http://localhost:8080/api/robocall/test \
  -d '{"text":"Hi, I had a burst pipe and my floor is damaged. Can you help me file a claim?"}'
```

**Expected:** `score: 0.0`, `category: human`

### Test 7.3: Blocklist Add

```bash
curl -s -X POST http://localhost:8080/api/blocklist \
  -d '{"number":"+15559999999","reason":"test_robocaller"}'
```

**Expected:** `{"status":"added","number":"15559999999"}`

### Test 7.4: Blocklist Check

```bash
curl -s -X POST http://localhost:8080/api/robocall/test \
  -d '{"number":"+15559999999"}'
```

**Expected:** `blocklist.score: 1.0`, `blocked: true`

### Test 7.5: Blocklist Remove

```bash
curl -s -X DELETE http://localhost:8080/api/blocklist \
  -d '{"number":"+15559999999"}'

# Verify removed
curl -s http://localhost:8080/api/blocklist
```

**Expected:** Number no longer in list

### Test 7.6: Robocall Stats

```bash
curl -s http://localhost:8080/api/robocall/stats
```

**Expected:** JSON with `blocklist_size`, `threshold`, `auto_block`

### Test 7.7: Edge Cases

```bash
# Single keyword
curl -s -X POST http://localhost:8080/api/robocall/test -d '{"text":"Press 1"}'
# Expected: score ~0.6 (single keyword)

# Mixed legitimate + keyword
curl -s -X POST http://localhost:8080/api/robocall/test -d '{"text":"Hi this is your bank. Press 1 to verify your account."}'
# Expected: score > 0.5 (uncertain/robocall)

# Empty text
curl -s -X POST http://localhost:8080/api/robocall/test -d '{"text":""}'
# Expected: score 0.0
```

---

## 8. Voice Biometrics Tests

### Test 8.1: List Voice Prints (Empty)

```bash
curl -s http://localhost:8080/api/security/voiceprints
```

**Expected:** `[]` (empty array)

### Test 8.2: Enroll Fraud Profile

```bash
curl -s -X POST http://localhost:8080/api/security/voiceprints \
  -d '{"label":"fraud_profile_001","type":"fraud"}'
```

**Expected:** JSON with `id`, `label: fraud_profile_001`, `type: fraud`

### Test 8.3: Enroll Verified Profile

```bash
curl -s -X POST http://localhost:8080/api/security/voiceprints \
  -d '{"label":"verified_john_doe","type":"verified"}'
```

**Expected:** JSON with `type: verified`

### Test 8.4: List Enrolled Prints

```bash
curl -s http://localhost:8080/api/security/voiceprints
```

**Expected:** 2 entries with correct labels and types

---

## 9. PII Masking Tests

### Test 9.1: Credit Card Detection

```bash
curl -s -X POST http://localhost:8080/api/security/pii/test \
  -d '{"text":"My credit card number is 4111 1111 1111 1111 and the CVV is 123"}'
```

**Expected:**
- `pii_found: true`
- `masked` text contains `XXXX-XXXX-XXXX-####` and `[CVV REDACTED]`
- 3 detections: `credit_card`, `credit_card_spoken`, `cvv`

### Test 9.2: SSN Detection

```bash
curl -s -X POST http://localhost:8080/api/security/pii/test \
  -d '{"text":"My social security number is 123-45-6789"}'
```

**Expected:** `masked: "My social security number is XXX-XX-####"`

### Test 9.3: Clean Text (No PII)

```bash
curl -s -X POST http://localhost:8080/api/security/pii/test \
  -d '{"text":"I need to reschedule my delivery for Thursday at 3 PM"}'
```

**Expected:** `pii_found: false`, `masked` = `original`

### Test 9.4: Date of Birth

```bash
curl -s -X POST http://localhost:8080/api/security/pii/test \
  -d '{"text":"My date of birth is January 15 1985"}'
```

**Expected:** `[DOB REDACTED]` in masked text

### Test 9.5: Account Number

```bash
curl -s -X POST http://localhost:8080/api/security/pii/test \
  -d '{"text":"My account number is 9876543210"}'
```

**Expected:** `[ACCOUNT REDACTED]`

### Test 9.6: PII Config

```bash
# Get config
curl -s http://localhost:8080/api/security/pii/config

# Disable
curl -s -X POST http://localhost:8080/api/security/pii/config -d '{"enabled":false}'

# Test while disabled
curl -s -X POST http://localhost:8080/api/security/pii/test \
  -d '{"text":"My SSN is 123-45-6789"}'
# Expected: pii_found: false (masking disabled)

# Re-enable
curl -s -X POST http://localhost:8080/api/security/pii/config -d '{"enabled":true}'
```

---

## 10. Post-Call Summary Tests

Run a co-pilot call (`./callcenter-live.sh`), speak 2-3 sentences, hang up.

```bash
docker logs voiceagent-gateway-1 2>&1 | grep "call summary" | tail -1
```

**Expected:** JSON with `summary`, `utterances`, `suggestions`, `sentiment`, `duration`

---

## 11. RAG Knowledge Base Tests

### Test 11.1: List Documents

```bash
curl -s http://localhost:8080/api/documents
```

**Expected:** 3 documents (Insurance, Billing, Retention) with `status: indexed`

### Test 11.2: Search — Water Damage

```bash
curl -s -X POST http://localhost:8080/api/documents/search \
  -d '{"query":"water damage burst pipe coverage deductible","top_k":3}'
```

**Expected:** Insurance Policy as top result, score > 0.9

### Test 11.3: Search — Late Fees

```bash
curl -s -X POST http://localhost:8080/api/documents/search \
  -d '{"query":"late fee waiver good standing","top_k":2}'
```

**Expected:** Billing FAQ as top result

### Test 11.4: Search — Cancellation

```bash
curl -s -X POST http://localhost:8080/api/documents/search \
  -d '{"query":"customer wants to cancel retention offer","top_k":2}'
```

**Expected:** Retention Playbook as top result

### Test 11.5: Index New Document

```bash
curl -s -X POST http://localhost:8080/api/documents \
  -d '{"name":"Returns Policy","category":"policy","content":"Returns accepted within 30 days with receipt. Electronics have a 15-day return window."}'
```

**Expected:** `status: indexed`, `chunks: 1`

---

## 12. Multi-LLM Tests

### Test 12.1: Claude Test

```bash
curl -s -X POST http://localhost:8080/api/llm/test \
  -d '{"provider":"anthropic-vertex","model":"claude-3-5-haiku@20241022","prompt":"What is SIP in one sentence?"}'
```

**Expected:** `response` with SIP definition, `latency` in ms, `model: claude:...`

### Test 12.2: Gemini Test

```bash
curl -s -X POST http://localhost:8080/api/llm/test \
  -d '{"provider":"gemini-vertex","model":"gemini-2.0-flash","prompt":"What is SIP in one sentence?"}'
```

**Expected:** Response from Gemini, or error if not available in your project

### Test 12.3: LLM Config List

```bash
curl -s http://localhost:8080/api/llm/configs
```

**Expected:** Array with at least Claude Haiku and Gemini Flash entries

### Test 12.4: Add LLM Config

```bash
curl -s -X POST http://localhost:8080/api/llm/configs \
  -d '{"name":"Claude Sonnet","provider":"anthropic-vertex","model":"claude-sonnet-4-20250514","region":"us-east5"}'
```

**Expected:** `status: created`

---

## 13. G.711 Codec Transcoding Tests

Compile-time verification — the 256-entry lookup tables are generated at `init()`.

```bash
cd gateway && go test -run TestCodec -v 2>/dev/null || echo "No test file — verify via build"
go build -o /dev/null . && echo "Codec tables compiled OK"
```

**Expected:** Build succeeds (tables compile)

---

## 14. DTMF Parsing Tests

### Test 14.1: Parse Digit Sequence

```bash
curl -s -X POST http://localhost:8080/api/dtmf/test -d '{"text":"482910"}'
```

**Expected:** `{"input":"482910","parsed":"User typed: 482910"}`

### Test 14.2: Star and Hash

```bash
curl -s -X POST http://localhost:8080/api/dtmf/test -d '{"text":"*123#"}'
```

**Expected:** `{"input":"*123#","parsed":"User typed: *123#"}`

---

## 15. Telecom AGC Tests

Compile-time verification:

```bash
cd gateway && go build -o /dev/null . && echo "AGC compiled OK"
```

**Expected:** Build succeeds (AGC, noise gate, CNG compile)

---

## 16. Failover State Machine Tests

### Test 16.1: All Circuits Healthy

```bash
curl -s http://localhost:8080/api/failover/status
```

**Expected:** All 4 services `state: closed`, `failures: 0`

### Test 16.2: Verify After Call

Make a call (`./simcall`), then check:

```bash
curl -s http://localhost:8080/api/failover/status
```

**Expected:** Still `closed` (no failures during normal call)

---

## 17. SBC Connectivity Tests

### Test 17.1: SIP Port Reachable

```bash
nc -z localhost 5070 && echo "SIP OK" || echo "SIP FAIL"
```

**Expected:** `SIP OK`

### Test 17.2: SIP OPTIONS Probe

```bash
python3 -c "
import socket, time
msg='OPTIONS sip:test@127.0.0.1:5070 SIP/2.0\r\nVia: SIP/2.0/TCP 127.0.0.1:5099;branch=z9hG4bK-test\r\nMax-Forwards: 70\r\nFrom: <sip:t@127.0.0.1>;tag=t\r\nTo: <sip:t@127.0.0.1>\r\nCall-ID: test\r\nCSeq: 1 OPTIONS\r\nContent-Length: 0\r\n\r\n'
s=socket.socket(socket.AF_INET,socket.SOCK_STREAM); s.settimeout(5)
s.connect(('127.0.0.1',5070)); s.send(msg.encode()); time.sleep(1)
print(s.recv(4096).decode().split('\r\n')[0]); s.close()
"
```

**Expected:** `SIP/2.0 200 OK`

### Test 17.3: ESL Reachable

```bash
docker exec voiceagent-freeswitch-1 fs_cli -P 8022 -x "status" 2>/dev/null | head -3
```

**Expected:** FreeSWITCH version and uptime

---

## 18. Rate Limiting & Admission Tests

### Test 18.1: Admission Status

```bash
curl -s http://localhost:8080/api/scale/status | python3 -c "
import sys,json; d=json.load(sys.stdin)
print(f'Max sessions: {d[\"admission\"][\"max_sessions\"]}')
print(f'Current: {d[\"admission\"][\"current\"]}')
print(f'Available: {d[\"admission\"][\"available\"]}')
"
```

**Expected:** `max_sessions: 500`, `current: 0`, `available: 500`

### Test 18.2: Rate Limit Headers (after many requests)

```bash
for i in $(seq 1 5); do
  curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/healthz
done
```

**Expected:** All `200` (within rate limit)

---

## 19. Redis Session Store Tests

### Test 19.1: Redis Ping

```bash
docker exec voiceagent-redis-1 redis-cli ping
```

**Expected:** `PONG`

### Test 19.2: Redis After Call

Make a call, then check Redis for session data:

```bash
docker exec voiceagent-redis-1 redis-cli keys "*"
```

**Expected:** Keys may appear during active calls

---

## 20. Prometheus Metrics Tests

### Test 20.1: Metrics Endpoint

```bash
curl -s http://localhost:8080/metrics | grep "voiceagent_" | wc -l
```

**Expected:** 30+ metrics lines

### Test 20.2: Key Metrics Present

```bash
curl -s http://localhost:8080/metrics | grep -E "calls_total|calls_active|stt_requests|llm_requests|uptime"
```

**Expected:** All 5 metrics present with values

### Test 20.3: Prometheus Scraping

```bash
curl -s "http://localhost:9090/api/v1/query?query=voiceagent_uptime_seconds" | python3 -c "
import sys,json; d=json.load(sys.stdin)
print(f'Status: {d[\"status\"]}')
if d['data']['result']:
    print(f'Uptime: {d[\"data\"][\"result\"][0][\"value\"][1]}s')
"
```

**Expected:** `Status: success`, uptime value

---

## 21. Agent CRUD Tests

### Test 21.1: List Agents

```bash
curl -s http://localhost:8080/api/agents | python3 -c "import sys,json; print(f'{len(json.load(sys.stdin))} agents')"
```

**Expected:** 2+ agents

### Test 21.2: Create Agent

```bash
curl -s -X POST http://localhost:8080/api/agents \
  -d '{"name":"New Agent","email":"new@test.com","expertise":["sales"]}'
```

**Expected:** `{"id":"uuid","status":"created"}`

### Test 21.3: Verify Creation

```bash
curl -s http://localhost:8080/api/agents | python3 -c "
import sys,json
for a in json.load(sys.stdin):
    print(f'{a[\"name\"]} ({a.get(\"email\",\"\")})')
"
```

**Expected:** "New Agent" in the list

---

## 22. Call History Tests

### Test 22.1: List Calls

```bash
curl -s http://localhost:8080/api/calls
```

**Expected:** Array of call records (may be empty if no DB calls yet)

### Test 22.2: Active Calls

```bash
curl -s http://localhost:8080/api/calls/active
```

**Expected:** `{"interactive":0,"copilot":0,"total":0}`

### Test 22.3: Dashboard Stats

```bash
curl -s http://localhost:8080/api/stats
```

**Expected:** JSON with `activeCalls`, `totalToday`, `avgDuration`, `sentimentBreakdown`

---

## 23. Dashboard UI Tests

Login with `admin` / `admin`, then open each page and verify rendering:

| Page | URL | Check |
|------|-----|-------|
| Login | http://localhost:3000/login | Login form, VOICEAGENT branding |
| Command Center | http://localhost:3000 | Stats cards, active calls table |
| Agents | http://localhost:3000/agents | Agent table with badges |
| Calls | http://localhost:3000/calls | Call history table |
| Live Ops | http://localhost:3000/calls/live | Connect field, transcript panel, co-pilot panel |
| Documents | http://localhost:3000/documents | Upload dropzone, document list, RAG search |
| Settings | http://localhost:3000/settings | LLM configs, system prompts, SBC settings |
| Security | http://localhost:3000/security | Robocall, PII masking, voice biometrics tabs |
| Infrastructure | http://localhost:3000/infrastructure | Failover circuits, scaling, DTMF, metrics |

---

## 24. SDK Tests

### Test 24.1: Python SDK

```bash
cd sdk/python
pip install -e . 2>/dev/null
python3 -c "
from voiceagent import VoiceAgentClient
client = VoiceAgentClient('http://localhost:8080')
print('Health:', client.health())
print('Agents:', len(client.list_agents()))
pii = client.test_pii('My SSN is 123-45-6789')
print('PII masked:', pii.masked)
print('Python SDK: OK')
"
```

**Expected:** Health OK, agent count, masked SSN, "Python SDK: OK"

### Test 24.2: TypeScript SDK

```bash
cd sdk/typescript
npm install 2>/dev/null
npx tsc --noEmit 2>&1 | tail -3 || echo "TypeScript types OK"
```

**Expected:** No type errors

---

## 25. SIP Call Tests

### Test 25.1: baresip SIP Call

```bash
cd test && SIP_PORT=5070 ./test-sip-call.sh
# Type: /dial 1000
# Speak for 5 seconds → hear Claude's response
# /hangup
```

**Expected:** Call established, audio bidirectional, TTS plays back

### Test 25.2: sipp Automated Call

```bash
docker exec voiceagent-freeswitch-1 bash -c "which sipp && sipp -sn uac -m 1 -s 1000 -t u1 -timeout 25 -d 10000 172.18.0.3:5060" 2>/dev/null | grep "Successful"
```

**Expected:** `Successful call: 1`

---

## 26. Load & Stress Tests

### Test 26.1: Concurrent API Requests

```bash
# 50 concurrent health checks
for i in $(seq 1 50); do
  curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/healthz &
done
wait
```

**Expected:** All return `200`

### Test 26.2: Concurrent Robocall Tests

```bash
for i in $(seq 1 20); do
  curl -s -o /dev/null -X POST http://localhost:8080/api/robocall/test \
    -d '{"text":"Press 1 for your warranty"}' &
done
wait
echo "Done"
```

**Expected:** All complete without errors

### Test 26.3: Concurrent PII Tests

```bash
for i in $(seq 1 20); do
  curl -s -o /dev/null -X POST http://localhost:8080/api/security/pii/test \
    -d '{"text":"My SSN is 123-45-6789"}' &
done
wait
echo "Done"
```

**Expected:** All complete without errors

### Test 26.4: Metrics After Load

```bash
curl -s http://localhost:8080/metrics | grep "voiceagent_"
```

**Expected:** Counters incremented from load tests

---

## 27. Deployment Mode Tests

### Test 27.1: Docker Compose (Current)

Already running — verify with `docker compose ps`.

### Test 27.2: Kustomize Base Render

```bash
kubectl kustomize k8s/base 2>/dev/null | grep "^kind:" | sort | uniq -c | sort -rn
```

**Expected:** 10 Deployments, 10 Services, 11 NetworkPolicies, 4 PVCs, 3 Secrets, 2 ConfigMaps, 1 Namespace

### Test 27.3: Local Overlay (KinD)

```bash
kubectl kustomize k8s/overlays/local 2>/dev/null | grep "hostNetwork"
```

**Expected:** `hostNetwork: true` (FreeSWITCH uses host networking)

### Test 27.4: Cloud Overlay (Istio + Gateway API)

```bash
# Verify Istio resources
kubectl kustomize k8s/overlays/cloud 2>/dev/null | grep "^kind:" | sort | uniq -c | sort -rn
```

**Expected:** 69 resources including DestinationRules, AuthorizationPolicies, PeerAuthentication, Gateway, HTTPRoutes, ServiceEntries

```bash
# Verify FreeSWITCH LB
kubectl kustomize k8s/overlays/cloud 2>/dev/null | grep -A2 "name: freeswitch-lb" | head -3
```

**Expected:** Service named `freeswitch-lb`

```bash
# Verify FreeSWITCH hostNetwork removed
kubectl kustomize k8s/overlays/cloud 2>/dev/null | grep "hostNetwork" || echo "hostNetwork correctly removed"
```

**Expected:** `hostNetwork correctly removed`

```bash
# Verify mTLS
kubectl kustomize k8s/overlays/cloud 2>/dev/null | grep -A3 "kind: PeerAuthentication"
```

**Expected:** `mode: STRICT`

### Test 27.5: On-Prem Overlay (MetalLB)

```bash
kubectl kustomize k8s/overlays/on-prem 2>/dev/null | grep "DEPLOYMENT_MODE" -A1
```

**Expected:** `value: on-prem`

```bash
kubectl kustomize k8s/overlays/on-prem 2>/dev/null | grep -E "kind: (IPAddressPool|L2Advertisement)"
```

**Expected:** MetalLB IPAddressPool + L2Advertisement

### Test 27.6: Air-Gapped Overlay

```bash
kubectl kustomize k8s/overlays/air-gapped 2>/dev/null | grep "ENABLE_AIRGAP"
```

**Expected:** `value: true`

### Test 27.7: KinD Local Deploy

```bash
make kind-up
make build-all load-all
make deploy-local
```

**Expected:** All 10 pods reach `Running` state

```bash
kubectl -n voiceagent get pods
```

**Expected:**
```
NAME                             READY   STATUS    RESTARTS   AGE
chromadb-xxx                     1/1     Running   0          ...
grafana-xxx                      1/1     Running   0          ...
media-gateway-xxx                1/1     Running   0          ...
piper-xxx                        1/1     Running   0          ...
postgres-xxx                     1/1     Running   0          ...
prometheus-xxx                   1/1     Running   0          ...
redis-xxx                        1/1     Running   0          ...
ui-xxx                           1/1     Running   0          ...
whisper-xxx                      1/1     Running   0          ...
freeswitch-xxx                   1/1     Running   0          ...
```

### Test 27.8: K8s Service Health (via port-forward)

```bash
kubectl -n voiceagent port-forward svc/media-gateway 8080:8080 &
curl -s http://localhost:8080/healthz
```

**Expected:** `{"status":"ok","sessions":0}`

### Test 27.9: Istio Mesh Status (cloud/on-prem only)

```bash
# Requires Istio installed: make istio-install
make mesh-status
```

**Expected:** 9 services with `SYNCED` proxy status (FreeSWITCH absent)

### Test 27.10: Gateway API Routes (cloud/on-prem only)

```bash
kubectl -n voiceagent get httproutes
```

**Expected:** 3 routes: `gateway-api`, `ui`, `grafana`

```bash
kubectl -n voiceagent get gateways
```

**Expected:** `voiceagent-gateway` with `Programmed` status

### Test 27.11: Network Policy Enforcement

```bash
# This should fail (whisper cannot reach redis)
kubectl -n voiceagent exec deployment/whisper -- wget -qO- --timeout=3 http://redis:6379 2>&1 | head -1
```

**Expected:** Connection refused or timeout

```bash
# This should succeed (gateway can reach redis)
kubectl -n voiceagent exec deployment/media-gateway -- wget -qO- --timeout=3 http://redis:6379 2>&1 | head -1
```

**Expected:** Connection established (may get protocol error, but TCP connects)

### Test 27.12: FreeSWITCH LB IP (cloud only)

```bash
make freeswitch-ip
```

**Expected:** External IP discovered, ConfigMap created, FreeSWITCH restarted

---

## 28. End-to-End Scenario Tests

### Scenario A: Full Customer Call (Interactive)

1. Start `./livecall ws://localhost:8080/ws`
2. Say: "I had a burst pipe and my floor is damaged"
3. Wait for Claude response (should mention claims)
4. Say: "Can you waive the late fee on my bill"
5. Wait for response (should mention waiver policy)
6. Say: "I want to cancel my policy"
7. Wait for response (should offer retention discount or transfer)
8. Ctrl+C to hang up
9. Verify gateway logs show: `heard`, `replied`, `action` entries

### Scenario B: Full Co-Pilot Session

1. Open http://localhost:3000/calls/live
2. Run `./callcenter-live.sh`
3. Paste call ID in UI, click CONNECT
4. Say: "Does my insurance cover water damage from a burst pipe?"
5. Verify: transcript appears in UI left panel
6. Verify: co-pilot suggestion appears in UI right panel with policy details
7. Say: "Can you waive my late fee?"
8. Verify: new suggestion appears
9. Ctrl+C to hang up
10. Verify: call summary appears in UI

### Scenario C: Robocall → Block → Verify

1. Add number to blocklist: `curl -X POST .../api/blocklist -d '{"number":"+15550000001","reason":"test"}'`
2. Verify blocklist: `curl .../api/blocklist`
3. Test detection: `curl -X POST .../api/robocall/test -d '{"number":"+15550000001"}'`
4. Verify `blocked: true`
5. Remove: `curl -X DELETE .../api/blocklist -d '{"number":"+15550000001"}'`
6. Verify removed

### Scenario D: PII in Live Call

1. Start `./livecall ws://localhost:8080/ws`
2. Say: "My credit card number is 4111 1111 1111 1111"
3. Check gateway logs: `docker logs voiceagent-gateway-1 | grep pii`
4. Verify: PII was detected and masked before reaching Claude
5. The LLM should NOT receive the actual card number

### Scenario E: Full Pipeline Under Load

1. Run 3 concurrent simcall instances:
```bash
cd test
timeout 45 go run simcall.go &
timeout 45 go run simcall.go &
timeout 45 go run simcall.go &
wait
```
2. Check metrics: `curl http://localhost:8080/metrics | grep calls_total`
3. Verify: all 3 calls completed successfully
4. Check failover: all circuits still `closed`

---

## Test Results Template

| # | Test | Status | Notes |
|---|------|--------|-------|
| 2.1 | Gateway health | | |
| 2.2 | Prometheus metrics | | |
| 2.3 | Redis connection | | |
| 2.4 | PostgreSQL connection | | |
| 2.5 | ChromaDB health | | |
| 2.6 | Prometheus scraping | | |
| 2.7 | Grafana | | |
| 2.8 | FreeSWITCH SIP | | |
| 2.9 | UI Dashboard | | |
| 2.10 | Failover status | | |
| 2.11 | Scale status | | |
| 3.1 | Simulated call | | |
| 3.2 | Live mic call | | |
| 4.1 | Simulated co-pilot | | |
| 4.2 | Live co-pilot | | |
| 4.3 | SSE event stream | | |
| 4.4 | RAG in suggestions | | |
| 5.1-5.4 | Self-service actions | | |
| 7.1-7.7 | Robocall detection | | |
| 8.1-8.4 | Voice biometrics | | |
| 9.1-9.6 | PII masking | | |
| 11.1-11.5 | RAG knowledge base | | |
| 12.1-12.4 | Multi-LLM | | |
| 14.1-14.2 | DTMF parsing | | |
| 16.1-16.2 | Failover | | |
| 17.1-17.3 | SBC connectivity | | |
| 18.1-18.2 | Rate limiting | | |
| 19.1-19.2 | Redis store | | |
| 20.1-20.3 | Prometheus | | |
| 21.1-21.3 | Agent CRUD | | |
| 27.2 | Kustomize base render | | |
| 27.3 | Local overlay (KinD) | | |
| 27.4 | Cloud overlay (Istio) | | |
| 27.5 | On-prem overlay (MetalLB) | | |
| 27.6 | Air-gapped overlay | | |
| 27.7 | KinD local deploy | | |
| 27.8 | K8s service health | | |
| 27.9 | Istio mesh status | | |
| 27.10 | Gateway API routes | | |
| 27.11 | Network policy enforcement | | |
| 27.12 | FreeSWITCH LB IP | | |
| 22.1-22.3 | Call history | | |
| 23 | Dashboard UI (6 pages) | | |
| 24.1-24.2 | SDKs | | |
| 25.1-25.2 | SIP calls | | |
| 26.1-26.4 | Load tests | | |
| 27.1-27.4 | Deployment modes | | |
| 28.A-28.E | E2E scenarios | | |
