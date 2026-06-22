# Quick Setup Guide

Get the AI Call Center Platform running in under 10 minutes. Three deployment options:

| Mode | Best For | Services | SIP Mode |
|------|----------|----------|----------|
| **Docker Compose** | Local dev, demo | All-in-one on host | FreeSWITCH B2BUA or Standalone |
| **KinD** | Local K8s dev | KinD cluster | Standalone (hostNetwork) |
| **K3s** | Production | Remote cluster | Standalone (hostNetwork) |

---

## Prerequisites

```bash
# macOS
brew install docker kind kubectl go node

# Verify
docker --version        # 24+
go version              # 1.22+
node --version          # 22+
kind version            # 0.20+
kubectl version --client
```

### GCP Credentials (Optional)

Required only for Claude/Gemini LLM features (co-pilot suggestions, post-call summaries). All other features (STT, call routing, queue, WebRTC, supervisor) work without GCP.

```bash
gcloud auth application-default login
export ANTHROPIC_VERTEX_PROJECT_ID="your-project-id"
export CLOUD_ML_REGION="us-east5"
```

---

## Option A: Docker Compose

Three variants available depending on your use case.

### A1. Full Platform (FreeSWITCH B2BUA)

10 services including FreeSWITCH for SIP trunk termination.

```bash
git clone https://github.com/kapiljain1989/voiceagent.git
cd voiceagent

# Set your LAN IP (required for SIP RTP addressing)
export EXT_IP=$(ipconfig getifaddr en0)    # macOS
# export EXT_IP=$(hostname -I | awk '{print $1}')  # Linux

# Start everything
docker compose -f docker-compose.sip.yml up -d
```

Services started:
```
gateway       :8080    HTTP/WebSocket API
freeswitch    :5070    SIP signaling
whisper       :8000    STT (faster-whisper)
piper         :5000    TTS
postgres      :5432    Database
chromadb      :8200    Vector store
ui            :3000    Next.js dashboard
redis         :6379    Session store
prometheus    :9090    Metrics
grafana       :3001    Dashboards
```

### A2. Standalone B2BUA (No FreeSWITCH)

Gateway acts as SIP B2BUA directly. Uses host networking for SIP+RTP.

```bash
export EXT_IP=$(ipconfig getifaddr en0)

docker compose -f docker-compose.standalone.yml up -d
```

Services: Gateway (:5060 SIP + :8080 HTTP), Whisper, PostgreSQL, ChromaDB, UI, Redis.

### A3. SIPREC Observer (Helper Mode)

Your SBC/PBX owns the call. VoiceAgent observes via SIPREC and provides AI assist.

```bash
docker compose -f docker-compose.helper.yml up -d
# or
make helper
```

Point your SBC's SIPREC recording server to `<HOST_IP>:5060`.

| SBC | Config |
|-----|--------|
| **Cisco CUBE** | `media-recording <HOST_IP> port 5060` |
| **AudioCodes** | Recording Server = `<HOST_IP>:5060` |
| **Oracle SBC** | destination = `sip:<HOST_IP>:5060` |
| **Kamailio** | `siprec_start_recording("sip:<HOST_IP>:5060")` |

### Verify (all Docker Compose variants)

```bash
# Health check
curl http://localhost:8080/healthz

# Login (default: admin / admin)
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' | jq -r .token)

# Open dashboard
open http://localhost:3000
```

### Cleanup

```bash
docker compose -f <compose-file> down      # Stop services
docker compose -f <compose-file> down -v    # Stop + delete data
```

---

## Option B: KinD (Local Kubernetes)

Local K8s development cluster using KinD. Gateway runs with `hostNetwork` for SIP/RTP.

### 1. Create Cluster & Build Images

```bash
# One command: create cluster, build images, load into KinD, deploy
make all
```

Or step by step:

```bash
make kind-up       # Create KinD cluster from kind-config.yaml
make build-all     # Build gateway, freeswitch, whisper, UI images
make load-all      # Load images into KinD
make secret        # Create GCP credentials (optional, warn if missing)
make deploy-local  # Deploy with local overlay
```

### 2. Verify

```bash
# Check pods
kubectl -n voiceagent get pods

# Port-forward
make port-forward-ui &                                              # :3000
kubectl -n voiceagent port-forward svc/media-gateway 8080:8080 &    # :8080

# Health check
curl http://localhost:8080/healthz
```

### 3. Access

| Service | URL |
|---------|-----|
| Dashboard | http://localhost:3000 |
| Gateway API | http://localhost:8080 |
| Grafana | http://localhost:3001 (`make port-forward-grafana`) |
| SIP | localhost:5062 (via KinD nodePort) |

### Cleanup

```bash
make platform-undeploy   # Remove services, keep cluster
make clean               # Delete cluster + images
```

---

## Option C: K3s Production

Production deployment on a remote K3s/K8s cluster with a public IP. Uses env vars for all deployment-specific values (no hardcoded IPs or registries).

### 1. Prerequisites

- A running K3s cluster with `kubectl` access
- A container registry (quay.io, Docker Hub, ECR, etc.)
- The cluster node's public IP address

```bash
# Required env vars
export EXTERNAL_IP="1.2.3.4"                  # Cluster node public IP
export REGISTRY="quay.io/your-org"            # Container registry
export REGISTRY_USER="your-username"          # Registry login
export REGISTRY_PASSWORD="your-token"         # Registry password
```

### 2. Registry Secret

```bash
make registry-secret
```

### 3. Build & Push Images

```bash
make push
```

This runs `generate-k3s` (creates k8s overlay from templates) → `build-k3s` (builds linux/amd64 gateway + UI images with `NEXT_PUBLIC_GATEWAY_URL=http://$EXTERNAL_IP:30080` baked in) → pushes to registry.

### 4. Deploy

```bash
make deploy-k3s
```

Deploys all services and waits for rollout:
- Gateway (hostNetwork, SIP :5062, HTTP :8080)
- Whisper (NodePort 30085 for hostNetwork gateway)
- UI (NodePort 30030)
- PostgreSQL, Redis, ChromaDB, Prometheus, Grafana

### 5. GCP Credentials (Optional)

If you want Claude/Gemini LLM features:

```bash
export GCP_PROJECT_ID="your-project-id"
export GCP_REGION="us-east5"
export GOOGLE_APPLICATION_CREDENTIALS="$HOME/.config/gcloud/application_default_credentials.json"
make secret
```

If GCP is not configured, the gateway starts with a warning but all non-LLM features work.

### 6. Verify

```bash
# Pod status
make status

# or
kubectl -n voiceagent get pods -o wide
```

### 7. Access

| Service | URL |
|---------|-----|
| Dashboard | `http://<EXTERNAL_IP>:30030` |
| Gateway API | `http://<EXTERNAL_IP>:30080` |
| SIP | `<EXTERNAL_IP>:5062` (UDP/TCP) |
| RTP | `<EXTERNAL_IP>:30000-30100` (UDP) |
| Grafana | `http://<EXTERNAL_IP>:30090` |

### 8. Update & Redeploy

After code changes:

```bash
make push                  # Rebuild + push images
kubectl -n voiceagent rollout restart deployment/media-gateway deployment/ui
```

### Cleanup

```bash
make undeploy-k3s
```

---

## Login & Authentication

Authentication is enabled by default. Default credentials: `admin` / `admin`.

```bash
# Get JWT token
TOKEN=$(curl -s -X POST http://<HOST>:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' | jq -r .token)

# Use token for API calls
curl -H "Authorization: Bearer $TOKEN" http://<HOST>:8080/api/agents
```

Dashboard login: same `admin` / `admin` at `http://<HOST>:3000`.

---

## Environment Variables

### Required

| Variable | Modes | Description |
|----------|-------|-------------|
| `EXT_IP` | Docker Compose | LAN IP for SIP RTP addressing |
| `EXTERNAL_IP` | K3s | Cluster node public IP |
| `REGISTRY` | K3s | Container registry (e.g. `quay.io/org`) |

### Optional

| Variable | Default | Description |
|----------|---------|-------------|
| `ANTHROPIC_VERTEX_PROJECT_ID` | *(none)* | GCP project for Claude/Gemini |
| `CLOUD_ML_REGION` | `us-east5` | Vertex AI region |
| `VOICEAGENT_MODE` | `gateway` | `standalone` (SIP B2BUA) or `gateway` (FreeSWITCH) |
| `AUTH_ENABLED` | `true` | JWT authentication |
| `JWT_SECRET` | `voiceagent-production-secret` | JWT signing key (change in production!) |
| `CLAUDE_MODEL` | `claude-3-5-haiku@20241022` | LLM model |
| `REDIS_URL` | `redis://redis:6379/0` | Redis URL |
| `CRM_WEBHOOK_URL` | *(empty)* | POST call summaries to external CRM |

---

## Port Reference

| Service | Docker Compose | KinD NodePort | K3s NodePort |
|---------|---------------|---------------|--------------|
| Gateway HTTP | 8080 | 30080 | 30080 |
| Gateway SIP | 5060 or 5070 | 30062 | 5062 (hostNetwork) |
| RTP pool | 30000-30050 | 30000-30020 | 30000-30100 (hostNetwork) |
| UI | 3000 | 30030 | 30030 |
| Whisper STT | 8000 | internal | 30085 |
| PostgreSQL | 5432 | internal | internal |
| Redis | 6379 | internal | internal |
| ChromaDB | 8200 | internal | internal |
| Prometheus | 9090 | 30090 | 30090 |
| Grafana | 3001 | 30090 | 30090 |

---

## Logs & Debugging

### Docker Compose

```bash
docker compose -f <file> logs -f              # All services
docker compose -f <file> logs -f gateway       # Gateway only
docker compose -f <file> logs -f whisper       # Whisper STT
```

### Kubernetes (KinD & K3s)

```bash
make status          # Pod + service overview
make logs-gw         # Gateway logs
make logs-whisper    # Whisper STT logs
make logs-ui         # UI logs
make logs-postgres   # PostgreSQL logs
make logs-redis      # Redis logs
```

---

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `GCP_PROJECT_ID is required` | Missing env var | `export ANTHROPIC_VERTEX_PROJECT_ID=your-project` or ignore (non-LLM features still work) |
| Whisper slow on first request | Model downloading | Wait 30-60s for model download |
| SIP call: no audio | Wrong IP for RTP | Set `EXT_IP` (Compose) or `EXTERNAL_IP` (K3s) to correct IP |
| UI: "Cannot connect to gateway" | Gateway URL mismatch | Check `NEXT_PUBLIC_GATEWAY_URL` matches gateway address |
| API: 401 Unauthorized | Missing/expired token | Re-run login to get fresh `$TOKEN` |
| K8s: `ImagePullBackOff` | Image not loaded/pushed | KinD: `make load-all`. K3s: `make push` + `make registry-secret` |
| K8s: Pod `Pending` (PVC) | No storage provisioner | KinD: uses local-path. K3s/Civo: set `storageClassName: civo-volume` |
| K3s: `generate-k3s` fails | Missing env vars | Set `EXTERNAL_IP` and `REGISTRY` before running make targets |
| K3s: Agent stuck "On Call" | Stale DB state from restart | `kubectl -n voiceagent exec deployment/postgres -- psql -U voiceagent -d voiceagent -c "UPDATE agents SET status='Available', active_calls=0;"` |
| K3s: Whisper unreachable | DNS from hostNetwork | Whisper uses headless service (`whisper-headless.voiceagent.svc.cluster.local`) |
| Agent STT hallucinations | PCMU codec quality | Gateway uses Opus for WebRTC agent audio; ensure browser supports it |
