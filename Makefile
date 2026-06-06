CLUSTER_NAME  := voiceagent
KIND_CONFIG   := kind-config.yaml
KUSTOMIZE_DIR := k8s/base
GW_IMAGE      := voiceagent/gateway:latest
FS_IMAGE      := voiceagent/freeswitch:latest
WHISPER_IMAGE := voiceagent/whisper:latest
PIPER_IMAGE   := waveoffire/piper-tts-server:latest

.PHONY: all kind-up kind-down build load deploy secret sbc-config undeploy logs-gw logs-fs clean

all: kind-up build load secret deploy

## ─── Cluster lifecycle ───────────────────────────────────────────

kind-up:
	kind create cluster --name $(CLUSTER_NAME) --config $(KIND_CONFIG)
	@echo "Cluster $(CLUSTER_NAME) ready."

kind-down:
	kind delete cluster --name $(CLUSTER_NAME)

## ─── Container builds ────────────────────────────────────────────

build:
	docker build -t $(GW_IMAGE) gateway/
	docker build -t $(FS_IMAGE) freeswitch/
	docker build -t $(WHISPER_IMAGE) whisper/
	docker pull $(PIPER_IMAGE)

## ─── Load images into KinD ───────────────────────────────────────

load:
	kind load docker-image $(GW_IMAGE) --name $(CLUSTER_NAME)
	kind load docker-image $(FS_IMAGE) --name $(CLUSTER_NAME)
	kind load docker-image $(WHISPER_IMAGE) --name $(CLUSTER_NAME)
	kind load docker-image $(PIPER_IMAGE) --name $(CLUSTER_NAME)

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

deploy:
	kubectl apply -k $(KUSTOMIZE_DIR)
	kubectl -n voiceagent rollout status deployment/whisper --timeout=120s
	kubectl -n voiceagent rollout status deployment/piper --timeout=120s
	kubectl -n voiceagent rollout status deployment/media-gateway --timeout=60s
	kubectl -n voiceagent rollout status deployment/freeswitch --timeout=60s

undeploy:
	kubectl delete -k $(KUSTOMIZE_DIR) --ignore-not-found

## ─── Observability ───────────────────────────────────────────────

logs-gw:
	kubectl -n voiceagent logs -f deployment/media-gateway

logs-fs:
	kubectl -n voiceagent logs -f deployment/freeswitch

logs-whisper:
	kubectl -n voiceagent logs -f deployment/whisper

logs-piper:
	kubectl -n voiceagent logs -f deployment/piper

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

## ─── Housekeeping ────────────────────────────────────────────────

clean: undeploy kind-down
	docker rmi $(GW_IMAGE) $(FS_IMAGE) $(WHISPER_IMAGE) 2>/dev/null || true
