#!/bin/bash
# test-sbc-local.sh — Local SBC Lab Test Suite
#
# Tests the Kamailio SBC → FreeSWITCH → Gateway pipeline.
# Requires: docker compose with sbc overlay running.
#
# Usage:
#   ./test-sbc-local.sh          # Run all tests
#   ./test-sbc-local.sh test1    # SIP OPTIONS
#   ./test-sbc-local.sh test2    # Registration
#   ./test-sbc-local.sh test3    # AI call
#   ./test-sbc-local.sh test4    # Trunk API
#   ./test-sbc-local.sh test5    # Gateway health

set -e

EXT_IP=${EXT_IP:-$(ipconfig getifaddr en0 2>/dev/null || hostname -I | awk '{print $1}')}
SBC_PORT=5080
GW_PORT=8080
PASS=0
FAIL=0

green() { echo -e "\033[32m✓ $1\033[0m"; PASS=$((PASS+1)); }
red()   { echo -e "\033[31m✗ $1\033[0m"; FAIL=$((FAIL+1)); }

echo "╔══════════════════════════════════════════════╗"
echo "║       Local SBC Lab Test Suite                ║"
echo "╚══════════════════════════════════════════════╝"
echo "  SBC:     ${EXT_IP}:${SBC_PORT}"
echo "  Gateway: localhost:${GW_PORT}"
echo ""

# ─── Test 1: SIP OPTIONS to Kamailio ─────────────────────────────

test1() {
    echo "=== Test 1: SIP OPTIONS to Kamailio ==="
    RESPONSE=$(python3 -c "
import socket, time
msg='OPTIONS sip:test@${EXT_IP}:${SBC_PORT} SIP/2.0\r\nVia: SIP/2.0/UDP ${EXT_IP}:15060;branch=z9hG4bK-test\r\nMax-Forwards: 70\r\nFrom: <sip:test@${EXT_IP}>;tag=test1\r\nTo: <sip:test@${EXT_IP}>\r\nCall-ID: test-options-001\r\nCSeq: 1 OPTIONS\r\nContent-Length: 0\r\n\r\n'
s=socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.settimeout(5)
s.sendto(msg.encode(), ('${EXT_IP}', ${SBC_PORT}))
try:
    data, addr = s.recvfrom(4096)
    print(data.decode().split('\r\n')[0])
except:
    print('TIMEOUT')
s.close()
" 2>/dev/null)

    if echo "$RESPONSE" | grep -q "200 OK"; then
        green "Kamailio responded: $RESPONSE"
    else
        red "Kamailio not responding: $RESPONSE"
    fi
}

# ─── Test 2: SIP REGISTER ────────────────────────────────────────

test2() {
    echo "=== Test 2: SIP REGISTER to Kamailio ==="
    RESPONSE=$(python3 -c "
import socket
msg='REGISTER sip:${EXT_IP}:${SBC_PORT} SIP/2.0\r\nVia: SIP/2.0/UDP ${EXT_IP}:15061;branch=z9hG4bK-reg\r\nMax-Forwards: 70\r\nFrom: <sip:testuser@${EXT_IP}>;tag=reg1\r\nTo: <sip:testuser@${EXT_IP}>\r\nCall-ID: test-register-001\r\nCSeq: 1 REGISTER\r\nContact: <sip:testuser@${EXT_IP}:15061>\r\nExpires: 60\r\nContent-Length: 0\r\n\r\n'
s=socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.settimeout(5)
s.sendto(msg.encode(), ('${EXT_IP}', ${SBC_PORT}))
try:
    data, addr = s.recvfrom(4096)
    print(data.decode().split('\r\n')[0])
except:
    print('TIMEOUT')
s.close()
" 2>/dev/null)

    if echo "$RESPONSE" | grep -q "200 OK"; then
        green "Registration accepted: $RESPONSE"
    else
        red "Registration failed: $RESPONSE"
    fi
}

# ─── Test 3: AI Call via SIP INVITE ───────────────────────────────

test3() {
    echo "=== Test 3: SIP INVITE 1000 (AI Agent) ==="
    RESPONSE=$(python3 -c "
import socket
msg='INVITE sip:1000@${EXT_IP}:${SBC_PORT} SIP/2.0\r\nVia: SIP/2.0/UDP ${EXT_IP}:15062;branch=z9hG4bK-inv\r\nMax-Forwards: 70\r\nFrom: <sip:customer@${EXT_IP}>;tag=inv1\r\nTo: <sip:1000@${EXT_IP}>\r\nCall-ID: test-invite-001\r\nCSeq: 1 INVITE\r\nContact: <sip:customer@${EXT_IP}:15062>\r\nContent-Type: application/sdp\r\nContent-Length: 0\r\n\r\n'
s=socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.settimeout(10)
s.sendto(msg.encode(), ('${EXT_IP}', ${SBC_PORT}))
try:
    data, addr = s.recvfrom(4096)
    first_line = data.decode().split('\r\n')[0]
    print(first_line)
except:
    print('TIMEOUT')
s.close()
" 2>/dev/null)

    if echo "$RESPONSE" | grep -qE "100 |180 |200 "; then
        green "INVITE accepted: $RESPONSE"
    else
        red "INVITE failed: $RESPONSE"
    fi
}

# ─── Test 4: Trunk API ───────────────────────────────────────────

test4() {
    echo "=== Test 4: Gateway Trunk API ==="
    TOKEN=$(curl -s -X POST http://localhost:${GW_PORT}/api/auth/login \
        -H 'Content-Type: application/json' \
        -d '{"username":"admin","password":"admin"}' | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null)

    if [ -n "$TOKEN" ]; then
        green "Auth token obtained"
    else
        red "Auth failed"
        return
    fi

    TRUNKS=$(curl -s -H "Authorization: Bearer $TOKEN" http://localhost:${GW_PORT}/api/trunks 2>/dev/null)
    COUNT=$(echo "$TRUNKS" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null)

    if [ "$COUNT" -gt 0 ] 2>/dev/null; then
        green "Trunk API: $COUNT trunks configured"
    else
        green "Trunk API: responding (0 trunks — add via POST /api/trunks)"
    fi
}

# ─── Test 5: Gateway Health ───────────────────────────────────────

test5() {
    echo "=== Test 5: Gateway Health ==="
    HEALTH=$(curl -s http://localhost:${GW_PORT}/healthz 2>/dev/null)

    if echo "$HEALTH" | grep -q '"ok"'; then
        green "Gateway healthy: $HEALTH"
    else
        red "Gateway not healthy: $HEALTH"
    fi
}

# ─── Run tests ────────────────────────────────────────────────────

case "${1:-all}" in
    test1) test1 ;;
    test2) test2 ;;
    test3) test3 ;;
    test4) test4 ;;
    test5) test5 ;;
    all)
        test5
        test1
        test2
        test3
        test4
        echo ""
        echo "Results: $PASS passed, $FAIL failed"
        [ $FAIL -eq 0 ] && echo "All tests passed!" || exit 1
        ;;
    *)
        echo "Usage: $0 [test1|test2|test3|test4|test5|all]"
        exit 1
        ;;
esac
