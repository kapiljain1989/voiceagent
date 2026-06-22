CLUSTER_NAME  := voiceagent
KIND_CONFIG   := kind-config.yaml
KUSTOMIZE_DIR := k8s/base
GW_IMAGE      := voiceagent/gateway:latest
FS_IMAGE      := voiceagent/freeswitch:latest
WHISPER_IMAGE := voiceagent/whisper:latest
PIPER_IMAGE   := waveoffire/piper-tts-server:latest
UI_IMAGE      := voiceagent/ui:latest
OVERLAY       ?= local

# Registry for K3s / remote cluster deployment (set via env vars)
REGISTRY      ?=
EXTERNAL_IP   ?=
REG_GW_IMAGE  := $(REGISTRY)/voiceagent-gateway:latest
REG_UI_IMAGE  := $(REGISTRY)/voiceagent-ui:latest

.PHONY: all kind-up kind-down build build-all load load-all deploy secret sbc-config undeploy \
        logs-gw logs-fs logs-whisper logs-piper logs-ui logs-postgres logs-redis logs-chromadb \
        clean status platform-deploy platform-undeploy deploy-local deploy-cloud deploy-on-prem \
        istio-install istio-uninstall mesh-status freeswitch-ip \
        port-forward-ui port-forward-grafana port-forward-prometheus \
        demos demos-gifs demos-clean \
        sbc-lab sbc-lab-down sbc-test \
        generate-k3s build-k3s push deploy-k3s undeploy-k3s registry-secret

all: kind-up build-all load-all secret deploy-local

## ─── Cluster lifecycle ───────────────────────────────────────────

kind-up:
	kind create cluster --name $(CLUSTER_NAME) --config $(KIND_CONFIG)
	@echo "Cluster $(CLUSTER_NAME) ready."

kind-down:
	kind delete cluster --name $(CLUSTER_NAME)

## ─── Istio lifecycle ─────────────────────────────────────────────

istio-install:
	istioctl install --set profile=minimal \
		--set values.pilot.env.PILOT_ENABLE_GATEWAY_API=true \
		-y
	@echo "Istio installed with Gateway API support."

istio-uninstall:
	istioctl uninstall --purge -y
	kubectl delete namespace istio-system --ignore-not-found

## ─── Container builds ────────────────────────────────────────────

build:
	docker build -t $(GW_IMAGE) gateway/
	docker build -t $(FS_IMAGE) freeswitch/
	docker build -t $(WHISPER_IMAGE) whisper/
	docker pull $(PIPER_IMAGE)

build-all: build
	docker build -t $(UI_IMAGE) ui/

## ─── Load images into KinD ───────────────────────────────────────

load:
	kind load docker-image $(GW_IMAGE) --name $(CLUSTER_NAME)
	kind load docker-image $(FS_IMAGE) --name $(CLUSTER_NAME)
	kind load docker-image $(WHISPER_IMAGE) --name $(CLUSTER_NAME)
	kind load docker-image $(PIPER_IMAGE) --name $(CLUSTER_NAME)

load-all: load
	kind load docker-image $(UI_IMAGE) --name $(CLUSTER_NAME)

## ─── Deploy / tear-down ──────────────────────────────────────────

# Prerequisites:
#   1. Run: gcloud auth application-default login
#   2. Set GCP_PROJECT_ID (or ANTHROPIC_VERTEX_PROJECT_ID) in your shell
#   3. Optionally set GCP_REGION / CLOUD_ML_REGION (defaults to us-east5)
secret:
	@kubectl create namespace voiceagent --dry-run=client -o yaml | kubectl apply -f -
	@kubectl create secret generic gcp-credentials \
		--namespace voiceagent \
		--from-file=key.json=$${GOOGLE_APPLICATION_CREDENTIALS:-$$HOME/.config/gcloud/application_default_credentials.json} \
		--dry-run=client -o yaml | kubectl apply -f -
	@kubectl create configmap gcp-config \
		--namespace voiceagent \
		--from-literal=project-id="$${GCP_PROJECT_ID:-$${ANTHROPIC_VERTEX_PROJECT_ID:-your-project-id}}" \
		--from-literal=region="$${GCP_REGION:-$${CLOUD_ML_REGION:-us-east5}}" \
		--dry-run=client -o yaml | kubectl apply -f -

## ─── SBC configuration ───────────────────────────────────────────

# Configure SBC trunk peering. Set env vars before running:
#   SBC_ADDRESS=sbc.example.com SBC_USERNAME=user SBC_PASSWORD=pass make sbc-config
sbc-config:
	@kubectl -n voiceagent create configmap sbc-config \
		--from-literal=sbc-address="$${SBC_ADDRESS:-127.0.0.1}" \
		--from-literal=sbc-register="$${SBC_REGISTER:-false}" \
		--from-literal=sbc-username="$${SBC_USERNAME:-voiceagent}" \
		--from-literal=sbc-password="$${SBC_PASSWORD:-changeme}" \
		--from-literal=sbc-auth-calls="$${SBC_AUTH_CALLS:-false}" \
		--dry-run=client -o yaml | kubectl apply -f -
	@kubectl -n voiceagent rollout restart deployment/freeswitch
	@echo "SBC config updated. FreeSWITCH restarting."

## ─── Outbound call ───────────────────────────────────────────────

# Originate an outbound call via the gateway API.
#   make call TO=+15551234567
call:
	@curl -s -X POST http://localhost:8080/call \
		-H 'Content-Type: application/json' \
		-d "{\"to\":\"$${TO}\",\"from\":\"$${FROM:-0000000000}\"}" | python3 -m json.tool

## ─── Legacy deploy (base only, no Istio) ─────────────────────────

deploy:
	kubectl apply -k $(KUSTOMIZE_DIR)
	kubectl -n voiceagent rollout status deployment/postgres --timeout=60s
	kubectl -n voiceagent rollout status deployment/redis --timeout=30s
	kubectl -n voiceagent rollout status deployment/chromadb --timeout=60s
	kubectl -n voiceagent rollout status deployment/whisper --timeout=120s
	kubectl -n voiceagent rollout status deployment/piper --timeout=120s
	kubectl -n voiceagent rollout status deployment/media-gateway --timeout=60s
	kubectl -n voiceagent rollout status deployment/freeswitch --timeout=60s
	kubectl -n voiceagent rollout status deployment/ui --timeout=60s
	kubectl -n voiceagent rollout status deployment/prometheus --timeout=30s
	kubectl -n voiceagent rollout status deployment/grafana --timeout=30s

undeploy:
	kubectl delete -k $(KUSTOMIZE_DIR) --ignore-not-found

## ─── Platform deploy (Istio + Gateway API + all services) ────────

platform-deploy: secret sbc-config
	kubectl apply -k k8s/overlays/$(OVERLAY)
	kubectl -n voiceagent rollout status deployment/postgres --timeout=60s
	kubectl -n voiceagent rollout status deployment/redis --timeout=30s
	kubectl -n voiceagent rollout status deployment/chromadb --timeout=60s
	kubectl -n voiceagent rollout status deployment/whisper --timeout=120s
	kubectl -n voiceagent rollout status deployment/piper --timeout=120s
	kubectl -n voiceagent rollout status deployment/media-gateway --timeout=60s
	kubectl -n voiceagent rollout status deployment/freeswitch --timeout=60s
	kubectl -n voiceagent rollout status deployment/ui --timeout=60s
	kubectl -n voiceagent rollout status deployment/prometheus --timeout=30s
	kubectl -n voiceagent rollout status deployment/grafana --timeout=30s

platform-undeploy:
	kubectl delete -k k8s/overlays/$(OVERLAY) --ignore-not-found

## ─── Overlay-specific targets ────────────────────────────────────

deploy-local:
	OVERLAY=local $(MAKE) platform-deploy

deploy-cloud: istio-install
	OVERLAY=cloud $(MAKE) platform-deploy
	@echo "Run 'make freeswitch-ip' to configure FreeSWITCH external IP."

deploy-on-prem: istio-install
	OVERLAY=on-prem $(MAKE) platform-deploy
	@echo "Run 'make freeswitch-ip' to configure FreeSWITCH external IP."

## ─── FreeSWITCH LB IP discovery (cloud / on-prem) ───────────────

freeswitch-ip:
	@echo "Waiting for LoadBalancer IP..."
	@kubectl -n voiceagent wait --for=jsonpath='{.status.loadBalancer.ingress[0].ip}' \
		service/freeswitch-lb --timeout=120s
	@FS_IP=$$(kubectl -n voiceagent get svc freeswitch-lb \
		-o jsonpath='{.status.loadBalancer.ingress[0].ip}'); \
	echo "FreeSWITCH external IP: $$FS_IP"; \
	kubectl -n voiceagent create configmap freeswitch-external-ip \
		--from-literal=external-rtp-ip="$$FS_IP" \
		--from-literal=external-sip-ip="$$FS_IP" \
		--dry-run=client -o yaml | kubectl apply -f -; \
	kubectl -n voiceagent rollout restart deployment/freeswitch
	@echo "FreeSWITCH restarting with correct external IP."

## ─── Observability ───────────────────────────────────────────────

logs-gw:
	kubectl -n voiceagent logs -f deployment/media-gateway

logs-fs:
	kubectl -n voiceagent logs -f deployment/freeswitch

logs-whisper:
	kubectl -n voiceagent logs -f deployment/whisper

logs-piper:
	kubectl -n voiceagent logs -f deployment/piper

logs-ui:
	kubectl -n voiceagent logs -f deployment/ui

logs-postgres:
	kubectl -n voiceagent logs -f deployment/postgres

logs-redis:
	kubectl -n voiceagent logs -f deployment/redis

logs-chromadb:
	kubectl -n voiceagent logs -f deployment/chromadb

port-forward-ui:
	kubectl -n voiceagent port-forward svc/ui 3000:3000

port-forward-grafana:
	kubectl -n voiceagent port-forward svc/grafana 3001:3000

port-forward-prometheus:
	kubectl -n voiceagent port-forward svc/prometheus 9090:9090

status:
	@echo "=== Pods ==="
	@kubectl -n voiceagent get pods -o wide
	@echo ""
	@echo "=== Services ==="
	@kubectl -n voiceagent get svc
	@echo ""
	@echo "=== Gateway health ==="
	@kubectl -n voiceagent exec deployment/media-gateway -- \
		wget -q -O- http://localhost:8080/healthz 2>/dev/null || true

## ─── Mesh status ─────────────────────────────────────────────────

mesh-status:
	@echo "=== Istio Proxy Status ==="
	@istioctl proxy-status -n voiceagent
	@echo ""
	@echo "=== mTLS Status ==="
	@istioctl authn tls-check -n voiceagent

## ─── SBC Local Lab ───────────────────────────────────────────────

sbc-lab:
	docker compose -f docker-compose.sip.yml -f docker-compose.sbc.yml up -d
	@echo "SBC lab running (11 services). Kamailio SBC at $${EXT_IP}:5080"
	@echo "Mobile softphone: register to $${EXT_IP}:5080, dial 1000 (AI) or 2000 (co-pilot)"

sbc-lab-down:
	docker compose -f docker-compose.sip.yml -f docker-compose.sbc.yml down

sbc-test:
	test/test-sbc-local.sh all

## ─── Standalone Helper (No FreeSWITCH) ──────────────────────────

helper:
	docker compose -f docker-compose.helper.yml up -d
	@echo "Helper mode running (8 services). SIPREC endpoint at :5060"
	@echo "Point your SBC SIPREC recording server to this IP:5060"

helper-down:
	docker compose -f docker-compose.helper.yml down

## ─── Demo recordings ─────────────────────────────────────────────

demos: demos-gifs

demos-gifs:
	@for tape in demos/tapes/*.tape; do \
		echo "Recording $$tape..."; \
		vhs "$$tape"; \
	done
	@echo "All GIFs generated in demos/gifs/"

demos-clean:
	rm -f demos/gifs/*.gif demos/gifs/*.mp4 demos/gifs/*.webm

## ─── K3s / Remote Cluster ─────────────────────────────────────────

# Generate K3s overlay files from templates. Requires EXTERNAL_IP and REGISTRY env vars.
#   EXTERNAL_IP=1.2.3.4 REGISTRY=quay.io/myorg make generate-k3s
generate-k3s:
	@if [ -z "$(EXTERNAL_IP)" ]; then echo "ERROR: EXTERNAL_IP not set. Usage: EXTERNAL_IP=x.x.x.x REGISTRY=quay.io/org make generate-k3s"; exit 1; fi
	@if [ -z "$(REGISTRY)" ]; then echo "ERROR: REGISTRY not set. Usage: EXTERNAL_IP=x.x.x.x REGISTRY=quay.io/org make generate-k3s"; exit 1; fi
	@sed 's|__EXTERNAL_IP__|$(EXTERNAL_IP)|g' k8s/overlays/k3s/external-ip-configmap.yaml.tpl > k8s/overlays/k3s/external-ip-configmap.yaml
	@sed -e 's|__EXTERNAL_IP__|$(EXTERNAL_IP)|g' -e 's|__REGISTRY__|$(REGISTRY)|g' k8s/overlays/k3s/kustomization.yaml.tpl > k8s/overlays/k3s/kustomization.yaml
	@echo "K3s overlay generated for $(EXTERNAL_IP) / $(REGISTRY)"

build-k3s: generate-k3s
	docker buildx build --platform linux/amd64 -t $(REG_GW_IMAGE) --load gateway/
	docker buildx build --platform linux/amd64 \
		--build-arg NEXT_PUBLIC_GATEWAY_URL=http://$(EXTERNAL_IP):30080 \
		-t $(REG_UI_IMAGE) --load ui/

push: build-k3s
	docker push $(REG_GW_IMAGE)
	docker push $(REG_UI_IMAGE)

registry-secret:
	@kubectl create namespace voiceagent --dry-run=client -o yaml | kubectl apply -f -
	@kubectl create secret docker-registry registry-credentials \
		--namespace voiceagent \
		--docker-server=$(REGISTRY) \
		--docker-username="$${REGISTRY_USER}" \
		--docker-password="$${REGISTRY_PASSWORD}" \
		--dry-run=client -o yaml | kubectl apply -f -

deploy-k3s: generate-k3s
	@kubectl create namespace voiceagent --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -k k8s/overlays/k3s
	kubectl -n voiceagent rollout status deployment/postgres --timeout=60s
	kubectl -n voiceagent rollout status deployment/redis --timeout=30s
	kubectl -n voiceagent rollout status deployment/chromadb --timeout=60s
	kubectl -n voiceagent rollout status deployment/whisper --timeout=120s
	kubectl -n voiceagent rollout status deployment/piper --timeout=120s
	kubectl -n voiceagent rollout status deployment/media-gateway --timeout=60s
	kubectl -n voiceagent rollout status deployment/ui --timeout=60s
	kubectl -n voiceagent rollout status deployment/prometheus --timeout=30s
	kubectl -n voiceagent rollout status deployment/grafana --timeout=30s

undeploy-k3s:
	kubectl delete -k k8s/overlays/k3s --ignore-not-found

## ─── Housekeeping ────────────────────────────────────────────────

clean: undeploy kind-down
	docker rmi $(GW_IMAGE) $(FS_IMAGE) $(WHISPER_IMAGE) $(UI_IMAGE) 2>/dev/null || true
