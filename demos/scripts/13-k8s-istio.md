# Demo 13: Kubernetes with Istio + Gateway API

**Duration:** ~3 minutes
**Format:** Screen recording (terminal)
**Audience:** Platform engineers, DevOps, SREs

---

## Scene 1: Overlay Architecture (0:00 - 0:45)

**Terminal:**
```bash
# Show the 4 deployment overlays
ls k8s/overlays/

# Count resources per overlay
for o in local cloud on-prem air-gapped; do
  echo -n "$o: "
  kubectl kustomize k8s/overlays/$o 2>/dev/null | grep -c '^kind:'
done

# Cloud overlay includes Istio mesh resources
kubectl kustomize k8s/overlays/cloud | grep '^kind:' | sort | uniq -c | sort -rn | head -10
```

**Narration:**
> "Four Kustomize overlays cover every deployment target. Local uses KinD with FreeSWITCH on hostNetwork. Cloud deploys to GKE, EKS, or AKS with a LoadBalancer for SIP/RTP. On-prem uses MetalLB L2 for bare metal. Air-gapped runs everything offline with local Ollama. The cloud overlay adds Istio service mesh, Gateway API resources, and FreeSWITCH LoadBalancer — 69 total K8s resources."

---

## Scene 2: Deploy to KinD (0:45 - 1:30)

**Terminal:**
```bash
# Full deploy: cluster → build → load → deploy
make all

# Check pods
kubectl -n voiceagent get pods -o wide

# Check services
kubectl -n voiceagent get svc
```

**Narration:**
> "One command deploys the full platform to a local KinD cluster. 10 deployments, 10 services, PersistentVolumeClaims for Postgres, ChromaDB, Prometheus, and Grafana. FreeSWITCH runs on hostNetwork with SIP port 5060 and RTP ports 16000 through 16020 mapped through to the host."

---

## Scene 3: Istio Mesh (1:30 - 2:15)

**Terminal:**
```bash
# Install Istio with Gateway API support
make istio-install

# Deploy cloud overlay
make deploy-cloud

# Verify mesh: 9 services with sidecars, FreeSWITCH excluded
make mesh-status

# Verify mTLS
istioctl authn tls-check -n voiceagent | head -12
```

**Narration:**
> "Istio injects Envoy sidecars into 9 services with STRICT mutual TLS. FreeSWITCH is excluded from the mesh — it handles raw UDP for SIP signaling and RTP media, which is incompatible with Envoy's TCP proxy. DestinationRules add circuit breaking per service — Whisper and Piper get max 10 connections since they're CPU-bound. AuthorizationPolicies enforce deny-by-default with explicit allow rules."

---

## Scene 4: Gateway API + Network Policies (2:15 - 3:00)

**Terminal:**
```bash
# HTTP routes
kubectl -n voiceagent get httproutes

# Gateway
kubectl -n voiceagent get gateways

# Network policies (defense-in-depth)
kubectl -n voiceagent get networkpolicies

# Verify: whisper cannot reach redis
kubectl -n voiceagent exec deployment/whisper -- wget -qO- --timeout=2 http://redis:6379 2>&1 | head -1 || echo "Blocked by NetworkPolicy"
```

**Narration:**
> "Gateway API routes HTTP and WebSocket traffic through Istio's ingress gateway. Three HTTPRoutes handle the API, dashboard, and Grafana. NetworkPolicies add CNI-level enforcement under Istio — Postgres only accepts connections from the gateway and UI. Redis is gateway-only. Whisper and Piper are gateway-only. Default deny blocks everything else."
