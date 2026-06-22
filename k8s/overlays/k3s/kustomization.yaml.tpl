apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: voiceagent

resources:
  - ../../base
  - external-ip-configmap.yaml
  - gcp-credentials-placeholder.yaml

images:
  - name: voiceagent/gateway
    newName: __REGISTRY__/voiceagent-gateway
    newTag: latest
  - name: voiceagent/ui
    newName: __REGISTRY__/voiceagent-ui
    newTag: latest

patches:
  # Whisper: NodePort so hostNetwork gateway can reach it reliably
  - target:
      kind: Service
      name: whisper
    patch: |
      - op: replace
        path: /spec/type
        value: NodePort
      - op: replace
        path: /spec/ports/0
        value:
          name: http
          port: 8000
          targetPort: 8000
          nodePort: 30085
          protocol: TCP

  # Gateway: hostNetwork for SIP+RTP, Recreate strategy, external IP from ConfigMap
  - target:
      kind: Deployment
      name: media-gateway
    patch: |
      - op: add
        path: /spec/strategy
        value:
          type: Recreate
      - op: add
        path: /spec/template/spec/hostNetwork
        value: true
      - op: add
        path: /spec/template/spec/dnsPolicy
        value: ClusterFirstWithHostNet
      - op: replace
        path: /spec/template/spec/containers/0/env/3
        value:
          name: EXTERNAL_IP
          valueFrom:
            configMapKeyRef:
              name: external-ip
              key: external-ip
      - op: replace
        path: /spec/template/spec/containers/0/env/4
        value:
          name: STT_URL
          value: "http://whisper-headless.voiceagent.svc.cluster.local:8000/v1/audio/transcriptions"

  # UI: pull from registry, set public gateway URL to external IP
  - target:
      kind: Deployment
      name: ui
    patch: |
      - op: replace
        path: /spec/template/spec/containers/0/imagePullPolicy
        value: Always
      - op: replace
        path: /spec/template/spec/containers/0/env/4
        value:
          name: NEXT_PUBLIC_GATEWAY_URL
          value: "http://__EXTERNAL_IP__:30080"

  # Postgres: use subdir for PGDATA (Civo volumes have lost+found)
  - target:
      kind: Deployment
      name: postgres
    patch: |
      - op: add
        path: /spec/template/spec/containers/0/env/-
        value:
          name: PGDATA
          value: /var/lib/postgresql/data/pgdata

  # Prometheus: run as nobody (65534) with fsGroup for volume permissions
  - target:
      kind: Deployment
      name: prometheus
    patch: |
      - op: add
        path: /spec/template/spec/securityContext
        value:
          fsGroup: 65534
          runAsUser: 65534
          runAsNonRoot: true

  # Grafana: fsGroup for volume permissions (grafana runs as uid 472)
  - target:
      kind: Deployment
      name: grafana
    patch: |
      - op: add
        path: /spec/template/spec/securityContext
        value:
          fsGroup: 472
          runAsUser: 472
          runAsNonRoot: true

  # PVCs: use K3s civo-volume provisioner
  - target:
      kind: PersistentVolumeClaim
      name: postgres-data
    patch: |
      - op: add
        path: /spec/storageClassName
        value: civo-volume

  - target:
      kind: PersistentVolumeClaim
      name: chromadb-data
    patch: |
      - op: add
        path: /spec/storageClassName
        value: civo-volume

  - target:
      kind: PersistentVolumeClaim
      name: prometheus-data
    patch: |
      - op: add
        path: /spec/storageClassName
        value: civo-volume

  - target:
      kind: PersistentVolumeClaim
      name: grafana-data
    patch: |
      - op: add
        path: /spec/storageClassName
        value: civo-volume
