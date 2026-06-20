#!/bin/bash
# deploy-local.sh — One-click VoiceAgent deployment on local KinD cluster
#
# Usage:
#   ./deploy-local.sh
#
# Prerequisites:
#   - Docker Desktop running
#   - kind, kubectl installed
#   - GCP credentials (for Claude/Vertex AI)
#
# Services deployed:
#   Gateway  → http://localhost:8080  (SIP on :5062)
#   Console  → http://localhost:3000
#   Grafana  → http://localhost:3001

set -e

GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info()  { echo -e "${CYAN}► $1${NC}"; }
ok()    { echo -e "${GREEN}✓ $1${NC}"; }
warn()  { echo -e "${YELLOW}⚠ $1${NC}"; }
fail()  { echo -e "${RED}✗ $1${NC}"; exit 1; }

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

# ─── Preflight ──────────────────────────────────────────────────
info "Preflight checks"

command -v docker >/dev/null || fail "Docker not found"
command -v kind >/dev/null || fail "kind not found (brew install kind)"
command -v kubectl >/dev/null || fail "kubectl not found"
docker info >/dev/null 2>&1 || fail "Docker not running"

GCP_CREDS="${GOOGLE_APPLICATION_CREDENTIALS:-$HOME/.config/gcloud/application_default_credentials.json}"
if [ ! -f "$GCP_CREDS" ]; then
  warn "GCP credentials not found at $GCP_CREDS"
  warn "Run: gcloud auth application-default login"
  warn "Or set GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json"
fi

# GCP project ID
GCP_PROJECT="${GCP_PROJECT_ID:-${ANTHROPIC_VERTEX_PROJECT_ID:-}}"
GCP_REGION="${GCP_REGION:-${CLOUD_ML_REGION:-us-east5}}"
if [ -z "$GCP_PROJECT" ]; then
  warn "GCP_PROJECT_ID not set — Claude/Vertex AI will not work"
  warn "Set: export GCP_PROJECT_ID=your-project-id"
  GCP_PROJECT="REPLACE_WITH_GCP_PROJECT_ID"
fi

ok "Prerequisites OK"

# ─── Create KinD cluster ────────────────────────────────────────
info "Creating KinD cluster"

if kind get clusters 2>/dev/null | grep -q "^voiceagent$"; then
  warn "Cluster 'voiceagent' already exists — deleting"
  kind delete cluster --name voiceagent
fi

kind create cluster --name voiceagent --config kind-config.yaml
ok "KinD cluster created"

kubectl config use-context kind-voiceagent >/dev/null 2>&1
kubectl cluster-info --context kind-voiceagent >/dev/null
ok "kubectl connected"

# ─── Build Docker images ────────────────────────────────────────
info "Building Docker images"

docker build -t voiceagent/gateway:latest gateway/
ok "Gateway image built"

docker build -t voiceagent/ui:latest ui/
ok "UI image built"

# ─── Load images into KinD ──────────────────────────────────────
info "Loading images into KinD"

kind load docker-image voiceagent/gateway:latest --name voiceagent
kind load docker-image voiceagent/ui:latest --name voiceagent

# Whisper is a large amd64 image — pull directly inside KinD node
info "Pulling whisper image inside KinD node (may take a minute)"
docker exec voiceagent-control-plane crictl pull fedirz/faster-whisper-server:latest-cpu 2>/dev/null || \
  docker exec voiceagent-control-plane ctr --namespace=k8s.io images pull docker.io/fedirz/faster-whisper-server:latest-cpu 2>/dev/null || \
  warn "Whisper image pull failed — STT will not work until image is available"

ok "Images loaded"

# ─── Create namespace + secrets ──────────────────────────────────
info "Creating namespace and secrets"

kubectl create namespace voiceagent 2>/dev/null || true

# GCP credentials
if [ -f "$GCP_CREDS" ]; then
  kubectl create secret generic gcp-credentials -n voiceagent \
    --from-file=key.json="$GCP_CREDS" \
    --dry-run=client -o yaml | kubectl apply -f -
  ok "GCP credentials secret created"
else
  # Create empty secret so deployment doesn't fail
  kubectl create secret generic gcp-credentials -n voiceagent \
    --from-literal=key.json="{}" \
    --dry-run=client -o yaml | kubectl apply -f -
  warn "Empty GCP credentials — Claude will not work"
fi

# Update GCP config
sed -i.bak "s/REPLACE_WITH_GCP_PROJECT_ID/$GCP_PROJECT/" k8s/base/gcp-configmap.yaml
sed -i.bak "s/us-east5/$GCP_REGION/" k8s/base/gcp-configmap.yaml

# ─── Deploy with Kustomize ──────────────────────────────────────
info "Deploying with Kustomize (local overlay)"

kubectl apply -k k8s/overlays/local

# Restore configmap template
mv k8s/base/gcp-configmap.yaml.bak k8s/base/gcp-configmap.yaml 2>/dev/null || true

ok "All resources applied"

# ─── Wait for pods ──────────────────────────────────────────────
info "Waiting for pods to be ready (this may take 1-2 minutes)"

kubectl -n voiceagent wait --for=condition=ready pod -l app=postgres --timeout=120s 2>/dev/null || true
kubectl -n voiceagent wait --for=condition=ready pod -l app=redis --timeout=60s 2>/dev/null || true
kubectl -n voiceagent wait --for=condition=ready pod -l app=media-gateway --timeout=120s 2>/dev/null || true
kubectl -n voiceagent wait --for=condition=ready pod -l app=ui --timeout=120s 2>/dev/null || true

echo ""
info "Pod status:"
kubectl get pods -n voiceagent -o wide
echo ""

# ─── Verify ─────────────────────────────────────────────────────
info "Verifying services"

sleep 3

if curl -s http://localhost:8080/healthz 2>/dev/null | grep -q "ok"; then
  ok "Gateway: http://localhost:8080"
else
  warn "Gateway not responding yet — may still be starting"
fi

if curl -s http://localhost:3000 >/dev/null 2>&1; then
  ok "Console: http://localhost:3000"
else
  warn "UI not responding yet — may still be starting"
fi

# ─── Done ───────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║          VoiceAgent Deployed Successfully!              ║${NC}"
echo -e "${GREEN}╠══════════════════════════════════════════════════════════╣${NC}"
echo -e "${GREEN}║  Console:  http://localhost:3000                        ║${NC}"
echo -e "${GREEN}║  Gateway:  http://localhost:8080                        ║${NC}"
echo -e "${GREEN}║  Grafana:  http://localhost:3001                        ║${NC}"
echo -e "${GREEN}║  SIP:      localhost:5062 (UDP)                         ║${NC}"
echo -e "${GREEN}╠══════════════════════════════════════════════════════════╣${NC}"
echo -e "${GREEN}║  Test call:                                             ║${NC}"
echo -e "${GREEN}║    python3 test/sip_test_call.py 127.0.0.1:5062 60     ║${NC}"
echo -e "${GREEN}║    Then open Console and click PICK                     ║${NC}"
echo -e "${GREEN}╠══════════════════════════════════════════════════════════╣${NC}"
echo -e "${GREEN}║  Teardown:                                              ║${NC}"
echo -e "${GREEN}║    kind delete cluster --name voiceagent                ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════╝${NC}"
