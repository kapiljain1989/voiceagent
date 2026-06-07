#!/bin/bash
# Helper script for VHS tape demos
# Usage: demos/run-demo.sh <demo-name> [args...]

TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")

case "$1" in
  healthz)
    curl -s http://localhost:8080/healthz | python3 -m json.tool
    ;;
  login)
    echo "Token: ${TOKEN:0:20}..."
    echo "Authenticated successfully"
    ;;
  services)
    docker compose -f docker-compose.sip.yml ps --format 'table {{.Name}}\t{{.Status}}'
    ;;
  robocall-test)
    curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/robocall/test \
      -d "{\"text\":\"$2\"}" | python3 -m json.tool
    ;;
  blocklist-add)
    curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/blocklist \
      -d "{\"number\":\"$2\",\"reason\":\"$3\"}" | python3 -m json.tool
    ;;
  blocklist-list)
    curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/blocklist | python3 -m json.tool
    ;;
  blocklist-remove)
    curl -s -H "Authorization: Bearer $TOKEN" -X DELETE http://localhost:8080/api/blocklist \
      -d "{\"number\":\"$2\"}"
    echo "Removed $2"
    ;;
  pii-test)
    curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/security/pii/test \
      -d "{\"text\":\"$2\"}" | python3 -m json.tool
    ;;
  doc-upload)
    curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/documents \
      -H 'Content-Type: application/json' \
      -d "{\"name\":\"$2\",\"category\":\"$3\",\"content\":\"$4\"}" | python3 -m json.tool
    ;;
  doc-search)
    curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/documents/search \
      -d "{\"query\":\"$2\"}" | python3 -m json.tool
    ;;
  doc-list)
    curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/documents | \
      python3 -c "import sys,json; [print(f'  {d[\"name\"]} ({d[\"category\"]})') for d in json.load(sys.stdin)]"
    ;;
  stats)
    curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/stats | python3 -m json.tool
    ;;
  agents)
    curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/agents | \
      python3 -c "import sys,json; print(f'{len(json.load(sys.stdin))} agents registered')"
    ;;
  failover)
    curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/failover/status | \
      python3 -c "import sys,json; d=json.load(sys.stdin); [print(f'  {k}: {v[\"state\"]} (failures: {v[\"failures\"]})') for k,v in d.items()]"
    ;;
  dtmf-test)
    curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/dtmf/test \
      -d "{\"text\":\"$2\"}" | python3 -m json.tool
    ;;
  metrics)
    curl -s http://localhost:8080/metrics | grep "voiceagent_" | head -10
    ;;
  scale)
    curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/scale/status | \
      python3 -c "import sys,json; d=json.load(sys.stdin); a=d['admission']; print(f'Sessions: {a[\"current\"]}/{a[\"max_sessions\"]} (available: {a[\"available\"]})')"
    ;;
  kustomize-count)
    for o in local cloud on-prem air-gapped; do
      echo -n "$o: "
      kubectl kustomize k8s/overlays/$o 2>/dev/null | grep -c '^kind:'
      echo " resources"
    done
    ;;
  kustomize-cloud)
    kubectl kustomize k8s/overlays/cloud 2>/dev/null | grep '^kind:' | sort | uniq -c | sort -rn | head -8
    ;;
  k8s-pods)
    kubectl -n voiceagent get pods -o wide 2>/dev/null || echo "No K8s cluster running"
    ;;
  config)
    curl -s http://localhost:8080/api/config | python3 -m json.tool
    ;;
  copilot-active)
    curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/copilot/active | python3 -c "
import sys,json
sessions = json.load(sys.stdin)
if not sessions:
    print('No active copilot sessions')
else:
    for s in sessions:
        vs = s.get('voice_sentiment', {})
        caller = s.get('caller', s['call_id'][:12])
        agent = s.get('agent', 'unknown')
        print(f'  {caller} -> {agent}  ({s[\"duration\"]}s)')
        if vs:
            print(f'    Agitation: {vs.get(\"agitation\",0):.0%}  Frustration: {vs.get(\"frustration\",0):.0%}  Engagement: {vs.get(\"engagement\",0):.0%}')
            print(f'    Pitch: {vs.get(\"avg_pitch_hz\",0):.0f}Hz  Speed: {vs.get(\"speaking_rate_wpm\",0):.0f}wpm  Energy: {vs.get(\"energy_trend\",\"stable\")}')
"
    ;;
  helper-services)
    docker compose -f docker-compose.helper.yml ps --format 'table {{.Name}}\t{{.Status}}' 2>/dev/null
    ;;
  sbc-lab-services)
    docker compose -f docker-compose.sip.yml -f docker-compose.sbc.yml ps --format 'table {{.Name}}\t{{.Status}}' 2>/dev/null
    ;;
  sip-options)
    python3 -c "
import socket, time
msg='OPTIONS sip:test@127.0.0.1:${2:-5061} SIP/2.0\r\nVia: SIP/2.0/TCP 127.0.0.1:15060;branch=z9hG4bK-demo\r\nMax-Forwards: 70\r\nFrom: <sip:test@127.0.0.1>;tag=d1\r\nTo: <sip:test@127.0.0.1>\r\nCall-ID: demo-001\r\nCSeq: 1 OPTIONS\r\nContent-Length: 0\r\n\r\n'
s=socket.socket(socket.AF_INET, socket.SOCK_STREAM); s.settimeout(5)
s.connect(('127.0.0.1', ${2:-5061})); s.send(msg.encode()); time.sleep(1)
print(s.recv(4096).decode().split('\r\n')[0]); s.close()
"
    ;;
  *)
    echo "Unknown demo: $1"
    exit 1
    ;;
esac
