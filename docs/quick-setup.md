# Quick Setup Guide

Get the AI Call Center Platform running in under 5 minutes.

---

## Prerequisites

Install these before starting:

```bash
# macOS
brew install docker kind kubectl go node portaudio baresip sipp

# Verify
docker --version        # 24+
go version              # 1.22+
node --version          # 22+
kind version            # 0.20+
```

### GCP Credentials

Claude and Gemini run on Vertex AI. STT, TTS, and all other services run locally.

```bash
# Login to GCP
gcloud auth application-default login

# Set your project (must have Vertex AI API enabled)
export ANTHROPIC_VERTEX_PROJECT_ID="your-project-id"
export CLOUD_ML_REGION="us-east5"
```

---

## Option A: Docker Compose (Recommended)

The fastest path. Starts all 7 services with a single command.

### 1. Clone and Start

```bash
git clone https://github.com/kapiljain1989/voiceagent.git
cd voiceagent

# Set your LAN IP (required for SIP RTP addressing)
export EXT_IP=$(ipconfig getifaddr en0)  # macOS
# export EXT_IP=$(hostname -I | awk '{print $1}')  # Linux

# Start everything
docker compose -f docker-compose.sip.yml up -d
```

### 2. Verify Services

```bash
# Check all 7 containers are running
docker compose -f docker-compose.sip.yml ps

# Health check
curl http://localhost:8080/healthz
# → {"status":"ok","sessions":0}
```

Expected services:
```
voiceagent-gateway-1      Up    :8080
voiceagent-freeswitch-1   Up    :5070
voiceagent-whisper-1      Up    :8000
voiceagent-piper-1        Up    :5000
voiceagent-postgres-1     Up    :5432
voiceagent-chromadb-1     Up    :8200
voiceagent-ui-1           Up    :3000
```

### 3. Open the Dashboard

```
http://localhost:3000
```

### 4. Index Knowledge Base Documents

```bash
curl -X POST http://localhost:8080/api/documents \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Insurance Policy",
    "category": "policy",
    "content": "Section 4.2.1: Water damage from burst pipes is covered. Deductible is $500 for Tier 2. Claims must be filed within 30 days. Emergency repairs up to $1000 pre-approved."
  }'
```

### 5. Make Your First Call

**WebSocket voice call (simplest — uses your mic/speaker):**
```bash
cd test && ./livecall ws://localhost:8080/ws
# Speak into your microphone — Claude responds through your speaker
# Ctrl+C to hang up
```

**SIP call via baresip:**
```bash
cd test && SIP_PORT=5070 ./test-sip-call.sh
# Type: /dial 1000
```

**Co-pilot call center demo:**
```bash
cd test && ./callcenter-live.sh
# Open http://localhost:3000/calls/live
# Paste the Call ID → speak → see co-pilot suggestions
```

---

## Option B: KinD Cluster (Kubernetes)

For testing the Kustomize deployment locally.

```bash
# Deploy everything
make all

# Port-forward the gateway
kubectl -n voiceagent port-forward deployment/media-gateway 8080:8080 &

# Test
cd test && ./livecall ws://localhost:8080/ws
```

### On-Premises Deployment

```bash
kubectl apply -k k8s/overlays/on-prem
```

### Air-Gapped Deployment

```bash
kubectl apply -k k8s/overlays/air-gapped
```

---

## Configuration Checklist

### Required

| Variable | Where to Set | Description |
|----------|-------------|-------------|
| `ANTHROPIC_VERTEX_PROJECT_ID` | Shell env | GCP project with Vertex AI enabled |
| `CLOUD_ML_REGION` | Shell env | Vertex AI region (default: `us-east5`) |
| `EXT_IP` | Shell env | Your LAN IP (for SIP RTP) |

### Optional

| Variable | Default | Description |
|----------|---------|-------------|
| `CLAUDE_MODEL` | `claude-3-5-haiku@20241022` | LLM model |
| `CRM_WEBHOOK_URL` | *(empty)* | POST call summaries here |
| `CRM_WEBHOOK_TOKEN` | *(empty)* | Webhook auth token |
| `ACTION_RESCHEDULE_URL` | *(empty)* | Self-service reschedule API |
| `ACTION_CANCEL_URL` | *(empty)* | Self-service cancel API |

---

## Testing Each Feature

### Interactive AI Agent
```bash
cd test && ./livecall ws://localhost:8080/ws
```

### Co-Pilot Agent Assist
```bash
cd test && ./callcenter-live.sh
```

### Robocall Detection
```bash
curl -X POST http://localhost:8080/api/robocall/test \
  -d '{"text":"Press 1 for your auto warranty"}'
```

### PII Masking
```bash
curl -X POST http://localhost:8080/api/security/pii/test \
  -d '{"text":"My SSN is 123-45-6789"}'
```

### RAG Search
```bash
curl -X POST http://localhost:8080/api/documents/search \
  -d '{"query":"water damage coverage"}'
```

### LLM Test
```bash
curl -X POST http://localhost:8080/api/llm/test \
  -d '{"provider":"anthropic-vertex","model":"claude-3-5-haiku@20241022","prompt":"Hello"}'
```

### Failover Status
```bash
curl http://localhost:8080/api/failover/status
```

### DTMF Parsing
```bash
curl -X POST http://localhost:8080/api/dtmf/test \
  -d '{"text":"482910"}'
```

### Blocklist
```bash
# Add
curl -X POST http://localhost:8080/api/blocklist \
  -d '{"number":"+15551234567","reason":"spam"}'

# List
curl http://localhost:8080/api/blocklist
```

### Self-Service Webhooks
```bash
# Configure
curl -X POST http://localhost:8080/api/actions/webhooks \
  -d '{"reschedule":"https://crm.example.com/api"}'

# View
curl http://localhost:8080/api/actions/webhooks
```

---

## SBC Configuration

### Twilio SIP Trunk
```bash
SBC_ADDRESS=your-trunk.pstn.twilio.com \
SBC_REGISTER=true \
SBC_USERNAME=your-sid \
SBC_PASSWORD=your-token \
make sbc-config
```

### Cisco CUBE
Copy `freeswitch/config/sip_profiles/enterprise/cisco-cube.xml` to `sip_profiles/external.xml` and set `SBC_ADDRESS`.

### AudioCodes Mediant
Copy `freeswitch/config/sip_profiles/enterprise/audiocodes.xml` to `sip_profiles/external.xml` and set `SBC_ADDRESS`.

---

## Logs & Debugging

```bash
# All services
docker compose -f docker-compose.sip.yml logs -f

# Specific service
docker compose -f docker-compose.sip.yml logs -f gateway
docker compose -f docker-compose.sip.yml logs -f freeswitch
docker compose -f docker-compose.sip.yml logs -f whisper

# Gateway call activity
docker logs voiceagent-gateway-1 | grep -E 'heard|replied|suggestion|summary'

# FreeSWITCH SIP/media
docker logs voiceagent-freeswitch-1 | grep -v event_socket | grep -v "Ping failed"
```

---

## Cleanup

```bash
# Stop all services
docker compose -f docker-compose.sip.yml down

# Stop and remove all data (PostgreSQL, ChromaDB volumes)
docker compose -f docker-compose.sip.yml down -v
```

---

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Gateway: `GCP_PROJECT_ID is required` | Missing env var | `export ANTHROPIC_VERTEX_PROJECT_ID=your-project` |
| FreeSWITCH: `Error Creating SIP UA` | Port conflict from previous pod | Delete pod and wait 30s: `kubectl delete pod -l app=freeswitch` |
| Whisper: slow first request | Model downloading | Wait 30-60s for `faster-whisper-base.en` to download |
| Piper: German voice | Wrong env var | Set `MODEL_DOWNLOAD_LINK` in docker-compose |
| SIP call: no audio | RTP port mismatch | Ensure `EXT_IP` matches your LAN IP |
| Co-pilot: no suggestions | RAG empty | Index documents first via `POST /api/documents` |
| UI: 404 on pages | Old build | Rebuild UI: `docker compose up -d --build ui` |
