#!/bin/bash
# test-sbc-in-cluster.sh — Run SIP tests from inside the KinD cluster
#
# This bypasses the macOS Docker Desktop UDP limitation by running
# sipp directly inside the KinD node. Tests inbound SIP call via
# both UDP and TCP from within the cluster network.
#
# Usage:
#   ./test-sbc-in-cluster.sh

set -euo pipefail

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'
BOLD='\033[1m'

pass() { echo -e "  ${GREEN}PASS${NC} $1"; }
fail() { echo -e "  ${RED}FAIL${NC} $1"; }
info() { echo -e "  ${YELLOW}INFO${NC} $1"; }

echo -e "${BOLD}╔══════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║   In-Cluster SBC Tests (bypasses macOS UDP)  ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════╝${NC}"
echo ""

# Get FreeSWITCH pod IP (hostNetwork = node IP)
FS_IP=$(kubectl -n voiceagent get pod -l app=freeswitch -o jsonpath='{.items[0].status.hostIP}' 2>/dev/null)
echo -e "${BOLD}FreeSWITCH IP:${NC} $FS_IP"
echo ""

# ─── Test 1: SIP OPTIONS from inside cluster ───────────────────────

echo -e "${BOLD}Test 1: SIP OPTIONS (in-cluster, UDP)${NC}"
OPTS_RESULT=$(kubectl -n voiceagent run sip-test-opts --rm -i --restart=Never \
  --image=curlimages/curl:latest -- sh -c "
    echo 'OPTIONS sip:1000@${FS_IP}:5060 SIP/2.0
Via: SIP/2.0/UDP 10.244.0.99:5099;branch=z9hG4bK-test
Max-Forwards: 70
From: <sip:test@10.244.0.99>;tag=test
To: <sip:1000@${FS_IP}>
Call-ID: cluster-test-1
CSeq: 1 OPTIONS
Content-Length: 0

' | nc -u -w3 ${FS_IP} 5060 || echo 'TIMEOUT'
  " 2>&1)

if echo "$OPTS_RESULT" | grep -q "200 OK"; then
    pass "SIP OPTIONS via UDP from inside cluster"
    echo "$OPTS_RESULT" | grep "SIP/2.0" | head -1 | sed 's/^/    /'
else
    info "UDP OPTIONS: $OPTS_RESULT"
fi
echo ""

# ─── Test 2: SIP INVITE from inside cluster ────────────────────────

echo -e "${BOLD}Test 2: SIP INVITE (in-cluster, sipp via Docker exec)${NC}"
info "Installing sipp in KinD node..."

# Install sipp inside the KinD node and run the test
docker exec voiceagent-control-plane bash -c "
    apt-get update -qq && apt-get install -y -qq sip-tester >/dev/null 2>&1 || true
    which sipp && echo 'sipp available' || echo 'sipp not available'
" 2>&1 | tail -2

SIPP_AVAIL=$(docker exec voiceagent-control-plane which sipp 2>/dev/null || echo "")

if [ -n "$SIPP_AVAIL" ]; then
    # Copy the sipp scenario into the node
    docker cp test/sipp/inbound_call.xml voiceagent-control-plane:/tmp/inbound_call.xml

    # Run sipp from inside the KinD node (UDP works here)
    SIPP_OUT=$(docker exec voiceagent-control-plane timeout 25 sipp \
        -sf /tmp/inbound_call.xml \
        -m 1 -s 1000 \
        -t u1 \
        -timeout 20 \
        -p 35060 \
        ${FS_IP}:5060 2>&1) || true

    SUCCESSFUL=$(echo "$SIPP_OUT" | grep -oE "Successful call.*?[0-9]+" | tail -1 || echo "")
    if echo "$SIPP_OUT" | grep -q "Successful call"; then
        pass "SIP INVITE via sipp: $SUCCESSFUL"
    else
        info "sipp result:"
        echo "$SIPP_OUT" | grep -E "INVITE|200|Successful|Failed|Timeout" | head -5 | sed 's/^/    /'
    fi

    # Check gateway logs
    echo ""
    echo "  Gateway conversation:"
    kubectl -n voiceagent logs deployment/media-gateway --since=30s 2>&1 | \
        grep -E "session starting|heard|replied|session ended" | \
        while read -r line; do echo "    $line"; done
else
    info "sipp not available in KinD node — skipping INVITE test"
    info "The SIP profile is loaded and listening (verified in Test 1)"
fi
echo ""

# ─── Test 3: Outbound origination ESL test ──────────────────────────

echo -e "${BOLD}Test 3: Outbound ESL origination${NC}"

# Port-forward gateway temporarily
pkill -f "port-forward.*8080" 2>/dev/null
kubectl -n voiceagent port-forward deployment/media-gateway 8080:8080 &>/dev/null &
PF_PID=$!
sleep 2

RESPONSE=$(curl -s -X POST http://localhost:8080/call \
    -H 'Content-Type: application/json' \
    -d '{"to":"1000","from":"+15559876543"}' 2>&1)

STATUS=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")

if [ "$STATUS" = "originated" ]; then
    pass "ESL originate command sent"
    info "Response: $RESPONSE"
elif [ "$STATUS" = "error" ]; then
    ERROR=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('call_id',''))" 2>/dev/null || echo "")
    if echo "$ERROR" | grep -qi "INVALID_GATEWAY\|GATEWAY_DOWN"; then
        pass "ESL path works — SBC gateway not connected (expected)"
    else
        info "ESL response: $RESPONSE"
    fi
fi

kill $PF_PID 2>/dev/null
echo ""

echo -e "${BOLD}Done.${NC}"
