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

The fastest path. Starts all 10 services with a single command.

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
# Check all 10 containers are running
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
voiceagent-redis-1        Up    :6379
voiceagent-prometheus-1   Up    :9090
voiceagent-grafana-1      Up    :3001
```

### 3. Login & Get Auth Token

Authentication is enabled by default. All API calls (except `/healthz`, `/metrics`, `/ws`, `/siprec`) require a JWT token.

```bash
# Login (default credentials: admin / admin)
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' | jq -r .token)

echo $TOKEN  # Should print a JWT token
```

### 4. Open the Dashboard

```
http://localhost:3000
```

Login with `admin` / `admin`.

### 5. Index Knowledge Base Documents

```bash
curl -X POST http://localhost:8080/api/documents \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Insurance Policy",
    "category": "policy",
    "content": "Section 4.2.1: Water damage from burst pipes is covered. Deductible is $500 for Tier 2. Claims must be filed within 30 days. Emergency repairs up to $1000 pre-approved."
  }'
```

### 6. Make Your First Call

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

## Option B: Kubernetes (Istio + Gateway API)

All 10 services deploy to K8s with Istio service mesh (STRICT mTLS) and Gateway API for HTTP/WebSocket ingress. Four overlays are available:

| Overlay | FreeSWITCH Networking | Istio | Use Case |
|---------|----------------------|-------|----------|
| `local` | hostNetwork | No | KinD local dev |
| `cloud` | LoadBalancer | Yes | GKE / EKS / AKS |
| `on-prem` | MetalLB L2 | Yes | Bare metal data center |
| `air-gapped` | hostNetwork | Yes | Zero internet (local Ollama) |

### Prerequisites (K8s)

```bash
brew install kind kubectl istioctl
```

### Local KinD Cluster

```bash
# Build all images, create cluster, deploy 10 services
make all

# Port-forward for local access
make port-forward-ui &          # http://localhost:3000
kubectl -n voiceagent port-forward svc/media-gateway 8080:8080 &

# Verify
curl http://localhost:8080/healthz
kubectl -n voiceagent get pods   # All 10 pods Running
```

### Cloud Deployment (GKE / EKS / AKS)

```bash
# Install Istio + deploy all services with Gateway API
make deploy-cloud

# Discover FreeSWITCH LoadBalancer IP and configure SDP
make freeswitch-ip

# Verify Istio mesh (9 services with sidecars, FreeSWITCH excluded)
make mesh-status

# Check Gateway API routes
kubectl -n voiceagent get httproutes
kubectl -n voiceagent get gateways
```

### On-Premises Deployment (MetalLB)

```bash
# Install MetalLB first
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.14.9/config/manifests/metallb-native.yaml

# Deploy with Istio + MetalLB
make deploy-on-prem

# Configure FreeSWITCH external IP
make freeswitch-ip
```

> Edit `k8s/overlays/on-prem/metallb-config.yaml` to set your site's IP range before deploying.

### Air-Gapped Deployment

```bash
kubectl apply -k k8s/overlays/air-gapped
```

### K8s Port Forwarding

```bash
make port-forward-ui            # Dashboard → localhost:3000
make port-forward-grafana       # Grafana → localhost:3001
make port-forward-prometheus    # Prometheus → localhost:9090
```

## Option C: Standalone Helper (No FreeSWITCH)

Plug-and-play SIPREC observer. Your SBC/PBX owns the call — VoiceAgent just observes and provides AI assist.

```bash
# Deploy (8 services — no FreeSWITCH, no Piper)
docker compose -f docker-compose.helper.yml up -d

# Or via Makefile
make helper
```

Expected services:
```
voiceagent-gateway-1      Up    :8080 + :5060 (SIP)
voiceagent-whisper-1      Up    :8000
voiceagent-postgres-1     Up    :5432
voiceagent-chromadb-1     Up    :8200
voiceagent-ui-1           Up    :3000
voiceagent-redis-1        Up    :6379
voiceagent-prometheus-1   Up    :9090
voiceagent-grafana-1      Up    :3001
```

### SBC Configuration (one line)

| SBC | Config |
|-----|--------|
| **Cisco CUBE** | `media-recording <VOICEAGENT_IP> port 5060` |
| **AudioCodes** | Recording Server = `<VOICEAGENT_IP>:5060` |
| **Oracle SBC** | destination = `sip:<VOICEAGENT_IP>:5060` |
| **Kamailio** | `siprec_start_recording("sip:<VOICEAGENT_IP>:5060")` |

All features work automatically: live transcript, co-pilot suggestions, robocall detection, PII masking, voice sentiment, post-call summary.

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
| `AUTH_ENABLED` | `true` | Enable JWT authentication |
| `JWT_SECRET` | `voiceagent-production-secret` | JWT signing key (change in production) |
| `VOICEAGENT_MODE` | `gateway` | `standalone` (SIPREC helper) or `gateway` (full B2BUA) |
| `REDIS_URL` | `redis://redis:6379/0` | Redis URL for distributed sessions |
| `CLAUDE_MODEL` | `claude-3-5-haiku@20241022` | LLM model |
| `CRM_WEBHOOK_URL` | *(empty)* | POST call summaries here |
| `CRM_WEBHOOK_TOKEN` | *(empty)* | Webhook auth token |
| `ACTION_RESCHEDULE_URL` | *(empty)* | Self-service reschedule API |
| `ACTION_CANCEL_URL` | *(empty)* | Self-service cancel API |

---

## Testing Each Feature

> All API calls require `Authorization: Bearer $TOKEN`. Get a token first:
> ```bash
> TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
>   -H 'Content-Type: application/json' \
>   -d '{"username":"admin","password":"admin"}' | jq -r .token)
> ```

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
curl -H "Authorization: Bearer $TOKEN" \
  -X POST http://localhost:8080/api/robocall/test \
  -d '{"text":"Press 1 for your auto warranty"}'
```

### PII Masking
```bash
curl -H "Authorization: Bearer $TOKEN" \
  -X POST http://localhost:8080/api/security/pii/test \
  -d '{"text":"My SSN is 123-45-6789"}'
```

### RAG Search
```bash
curl -H "Authorization: Bearer $TOKEN" \
  -X POST http://localhost:8080/api/documents/search \
  -d '{"query":"water damage coverage"}'
```

### LLM Test
```bash
curl -H "Authorization: Bearer $TOKEN" \
  -X POST http://localhost:8080/api/llm/test \
  -d '{"provider":"anthropic-vertex","model":"claude-3-5-haiku@20241022","prompt":"Hello"}'
```

### Failover Status
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/failover/status
```

### DTMF Parsing
```bash
curl -H "Authorization: Bearer $TOKEN" \
  -X POST http://localhost:8080/api/dtmf/test \
  -d '{"text":"482910"}'
```

### Blocklist
```bash
# Add
curl -H "Authorization: Bearer $TOKEN" \
  -X POST http://localhost:8080/api/blocklist \
  -d '{"number":"+15551234567","reason":"spam"}'

# List
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/blocklist
```

### SIP Trunk Management
```bash
# Add trunk
curl -H "Authorization: Bearer $TOKEN" \
  -X POST http://localhost:8080/api/trunks \
  -d '{"name":"My SBC","address":"sbc.example.com","register":false}'

# List trunks
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/trunks
```

### Self-Service Webhooks
```bash
# Configure
curl -H "Authorization: Bearer $TOKEN" \
  -X POST http://localhost:8080/api/actions/webhooks \
  -d '{"reschedule":"https://crm.example.com/api"}'

# View
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/actions/webhooks
```

### Prometheus & Grafana
```bash
# Raw metrics (no auth required)
curl http://localhost:8080/metrics

# Prometheus UI
open http://localhost:9090

# Grafana dashboards
open http://localhost:3001
```

### Voice Sentiment & Config
```bash
# Check deployment mode
curl http://localhost:8080/api/config | python3 -m json.tool

# Active copilot sessions with live voice sentiment
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/copilot/active | python3 -m json.tool
```

---

## SBC Configuration

> Full SBC integration guide: [`docs/sbc-configuration.md`](sbc-configuration.md) — covers Cisco CUBE, AudioCodes Mediant, Oracle SBC, Kamailio, and Twilio with SIPREC setup.

### Via API (Recommended)
```bash
curl -H "Authorization: Bearer $TOKEN" \
  -X POST http://localhost:8080/api/trunks \
  -d '{"name":"My SBC","address":"sbc.example.com","register":false}'
```

### Twilio SIP Trunk
```bash
curl -H "Authorization: Bearer $TOKEN" \
  -X POST http://localhost:8080/api/trunks \
  -d '{
    "name": "Twilio Production",
    "address": "your-trunk.pstn.twilio.com",
    "register": true,
    "username": "your-trunk-sid",
    "password": "your-auth-token",
    "caller_id": "+15559876543"
  }'
```

### Cisco CUBE / AudioCodes
Copy the pre-configured profile from `freeswitch/config/sip_profiles/enterprise/` to `sip_profiles/external.xml` and set `SBC_ADDRESS`, or use the trunk API above.

---

## Logs & Debugging

### Docker Compose
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

### Kubernetes
```bash
# Pod status
make status

# Per-service logs
make logs-gw        # Gateway
make logs-fs        # FreeSWITCH
make logs-whisper   # Whisper STT
make logs-piper     # Piper TTS
make logs-ui        # Next.js UI
make logs-postgres  # PostgreSQL
make logs-redis     # Redis
make logs-chromadb  # ChromaDB

# Istio mesh diagnostics (cloud/on-prem)
make mesh-status
istioctl analyze -n voiceagent
```

---

## Cleanup

### Docker Compose
```bash
# Stop all services
docker compose -f docker-compose.sip.yml down

# Stop and remove all data (PostgreSQL, ChromaDB volumes)
docker compose -f docker-compose.sip.yml down -v
```

### Kubernetes
```bash
# Tear down services (keep cluster)
make platform-undeploy

# Tear down everything (cluster + images)
make clean
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
| API: 401 Unauthorized | Missing or expired token | Re-run login to get a fresh `$TOKEN` |
| UI: "Cannot connect to gateway" | CORS or gateway down | Check gateway is running and `NEXT_PUBLIC_GATEWAY_URL` is set |
| K8s: Pod `ImagePullBackOff` | Image not in KinD | Run `make load-all` to load images into KinD |
| K8s: Pod `Pending` (PVC) | No storage provisioner | KinD uses local-path by default; cloud needs storageClassName |
| K8s: Istio sidecar not injecting | Namespace label missing | Check `kubectl get ns voiceagent --show-labels` for `istio-injection=enabled` |
| K8s: FreeSWITCH no SIP | hostNetwork not set | Local overlay uses hostNetwork; cloud uses LoadBalancer (`make freeswitch-ip`) |
| K8s: Service unreachable | NetworkPolicy blocking | Check `kubectl -n voiceagent get networkpolicies` and verify source pod is allowed |
| SIP: no SIPREC sessions | Wrong mode | Set `VOICEAGENT_MODE=standalone` in docker-compose |
