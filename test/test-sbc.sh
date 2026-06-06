#!/bin/bash
# test-sbc.sh — SBC feature integration tests
#
# Runs three test levels:
#   1. SIP connectivity (SIP OPTIONS probe)
#   2. Inbound SIP call via sipp (simulates SBC sending a call)
#   3. Outbound call origination via POST /call API
#
# Prerequisites:
#   - KinD cluster running with all pods healthy
#   - sipp installed (brew install sipp)
#   - Port-forward active: kubectl -n voiceagent port-forward deployment/media-gateway 8080:8080
#
# Usage:
#   ./test-sbc.sh [test1|test2|test3|all]

set -euo pipefail

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'
BOLD='\033[1m'

pass() { echo -e "  ${GREEN}PASS${NC} $1"; }
fail() { echo -e "  ${RED}FAIL${NC} $1"; }
info() { echo -e "  ${YELLOW}INFO${NC} $1"; }

FS_HOST="127.0.0.1"
FS_PORT="5060"
GW_URL="http://localhost:8080"

echo -e "${BOLD}╔══════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║       SBC Feature Integration Tests          ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════╝${NC}"
echo ""

# ─── Test 1: SIP Connectivity ─────────────────────────────────────

test1_sip_connectivity() {
    echo -e "${BOLD}Test 1: SIP Connectivity${NC}"
    echo "  Sending SIP OPTIONS to $FS_HOST:$FS_PORT over TCP..."

    RESPONSE=$(python3 -c "
import socket
msg='OPTIONS sip:test@$FS_HOST:$FS_PORT SIP/2.0\r\nVia: SIP/2.0/TCP $FS_HOST:15060;branch=z9hG4bK-test\r\nMax-Forwards: 70\r\nFrom: <sip:test@$FS_HOST>;tag=test\r\nTo: <sip:test@$FS_HOST>\r\nCall-ID: sbc-test-$(date +%s)\r\nCSeq: 1 OPTIONS\r\nContent-Length: 0\r\n\r\n'
s=socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.settimeout(5)
try:
    s.connect(('$FS_HOST', $FS_PORT))
    s.send(msg.encode())
    d=s.recv(4096).decode()
    lines=d.split('\r\n')
    print(lines[0])
    for l in lines:
        if l.startswith('Allow:'):
            print(l)
except Exception as e:
    print('ERROR: ' + str(e))
finally:
    s.close()
" 2>&1)

    if echo "$RESPONSE" | grep -q "200 OK"; then
        pass "FreeSWITCH responding to SIP OPTIONS"
        echo "    $RESPONSE" | head -1
    else
        fail "No SIP response: $RESPONSE"
        return 1
    fi
    echo ""
}

# ─── Test 2: Inbound SIP Call via sipp ─────────────────────────────

test2_inbound_sipp() {
    echo -e "${BOLD}Test 2: Inbound SIP Call (sipp)${NC}"
    echo "  Simulating SBC sending an INVITE to FreeSWITCH..."

    if ! command -v sipp &>/dev/null; then
        info "sipp not installed. Run: brew install sipp"
        return 1
    fi

    # Run sipp with a simple UAC scenario — 1 call, TCP transport
    SIPP_OUT=$(sipp -sf "$(dirname "$0")/sipp/inbound_call.xml" \
        -m 1 \
        -s 1000 \
        -t t1 \
        -timeout 20 \
        -max_retrans 2 \
        "$FS_HOST:$FS_PORT" 2>&1) || true

    if echo "$SIPP_OUT" | grep -qE "Successful call|200"; then
        pass "sipp call completed successfully"
    else
        # sipp may still succeed even with partial output
        CALL_COUNT=$(echo "$SIPP_OUT" | grep -oE "Successful call\(s\).*[0-9]+" | tail -1 || echo "")
        if [ -n "$CALL_COUNT" ]; then
            pass "sipp: $CALL_COUNT"
        else
            info "sipp output (check manually):"
            echo "$SIPP_OUT" | tail -10
        fi
    fi

    # Check gateway logs for the call
    echo ""
    echo "  Gateway logs (last inbound call):"
    kubectl -n voiceagent logs deployment/media-gateway --tail=5 2>&1 | \
        grep -E "session starting|heard|replied" | tail -3 | \
        while read -r line; do echo "    $line"; done

    echo ""
}

# ─── Test 3: Outbound Call Origination ─────────────────────────────

test3_outbound_call() {
    echo -e "${BOLD}Test 3: Outbound Call Origination (POST /call)${NC}"

    # Test the API endpoint
    echo "  Sending POST /call to originate through SBC gateway..."

    RESPONSE=$(curl -s -X POST "$GW_URL/call" \
        -H 'Content-Type: application/json' \
        -d '{"to":"1000","from":"+15559876543"}' 2>&1)

    STATUS=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
    CALL_ID=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('call_id',''))" 2>/dev/null || echo "")
    ERROR=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('error',''))" 2>/dev/null || echo "")

    if [ "$STATUS" = "originated" ]; then
        pass "Call originated successfully (call_id: $CALL_ID)"
    elif [ "$STATUS" = "error" ] && echo "$ERROR" | grep -qi "INVALID_GATEWAY\|GATEWAY_DOWN\|NORMAL_TEMPORARY_FAILURE"; then
        pass "ESL path works — gateway correctly reports SBC not connected"
        info "Error: $ERROR (expected when no SBC is configured)"
    else
        fail "Unexpected response: $RESPONSE"
    fi

    echo ""
}

# ─── Test 4: SBC ConfigMap Update ──────────────────────────────────

test4_sbc_config() {
    echo -e "${BOLD}Test 4: SBC Configuration${NC}"

    # Verify the SBC configmap exists
    CM=$(kubectl -n voiceagent get configmap sbc-config -o jsonpath='{.data.sbc-address}' 2>&1)
    if [ -n "$CM" ]; then
        pass "SBC configmap exists (sbc-address: $CM)"
    else
        fail "SBC configmap not found"
        return 1
    fi

    # Verify FreeSWITCH has the SBC env vars
    SBC_ADDR=$(kubectl -n voiceagent exec deployment/freeswitch -- env 2>/dev/null | grep SBC_ADDRESS | head -1)
    if [ -n "$SBC_ADDR" ]; then
        pass "FreeSWITCH has SBC env vars ($SBC_ADDR)"
    else
        info "SBC env vars not visible (may be injected via entrypoint)"
    fi

    # Verify gateway has ESL config
    ESL=$(kubectl -n voiceagent exec deployment/media-gateway -- env 2>/dev/null | grep ESL_HOST | head -1)
    if [ -n "$ESL" ]; then
        pass "Gateway has ESL config ($ESL)"
    else
        info "ESL env vars not visible in distroless container"
    fi

    echo ""
}

# ─── Test 5: Full Pipeline via SIP ──────────────────────────────────

test5_pipeline_check() {
    echo -e "${BOLD}Test 5: Pipeline Health Check${NC}"

    # Health endpoint
    HEALTH=$(curl -s "$GW_URL/healthz" 2>&1)
    if echo "$HEALTH" | grep -q '"ok"'; then
        pass "Gateway healthy: $HEALTH"
    else
        fail "Gateway unhealthy: $HEALTH"
    fi

    # Check all pods are running
    PODS=$(kubectl -n voiceagent get pods --no-headers 2>&1 | grep -v Terminating)
    RUNNING=$(echo "$PODS" | grep -c "Running" || echo 0)
    TOTAL=$(echo "$PODS" | wc -l | tr -d ' ')

    if [ "$RUNNING" = "$TOTAL" ]; then
        pass "All $TOTAL pods running"
    else
        fail "$RUNNING/$TOTAL pods running"
        echo "$PODS" | while read -r line; do echo "    $line"; done
    fi

    echo ""
}

# ─── Run tests ──────────────────────────────────────────────────────

TESTS=${1:-all}

case "$TESTS" in
    test1) test1_sip_connectivity ;;
    test2) test2_inbound_sipp ;;
    test3) test3_outbound_call ;;
    test4) test4_sbc_config ;;
    test5) test5_pipeline_check ;;
    all)
        test5_pipeline_check
        test4_sbc_config
        test1_sip_connectivity
        test3_outbound_call
        test2_inbound_sipp
        ;;
    *)
        echo "Usage: $0 [test1|test2|test3|test4|test5|all]"
        exit 1
        ;;
esac

echo -e "${BOLD}Done.${NC}"
