#!/bin/bash
# start-sbc.sh — Start/stop the test SBC
#
# Usage:
#   ./test/sbc/start-sbc.sh                    # Start (gateway on same machine)
#   ./test/sbc/start-sbc.sh 192.168.1.100      # Start (gateway on another IP)
#   ./test/sbc/start-sbc.sh stop               # Stop
#   ./test/sbc/start-sbc.sh logs               # View logs
#   ./test/sbc/start-sbc.sh status             # Check status

set -e

DIR="$(cd "$(dirname "$0")" && pwd)"
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
NC='\033[0m'

case "${1:-start}" in
    stop)
        cd "$DIR" && docker compose down 2>/dev/null
        echo -e "${GREEN}✓ Test SBC stopped${NC}"
        exit 0
        ;;
    logs)
        docker logs test-sbc -f 2>&1
        exit 0
        ;;
    status)
        if docker ps --format '{{.Names}}' | grep -q test-sbc; then
            echo -e "${GREEN}✓ Test SBC running${NC}"
            docker logs test-sbc 2>&1 | head -1
        else
            echo -e "${YELLOW}⚠ Test SBC not running${NC}"
        fi
        exit 0
        ;;
esac

GATEWAY_HOST="${1:-host.docker.internal}"

echo -e "${CYAN}Starting Test SBC...${NC}"
cd "$DIR" && GATEWAY_HOST="$GATEWAY_HOST" docker compose up -d --build 2>&1 | tail -3

sleep 2

# Verify it's running
if ! docker ps --format '{{.Names}}' | grep -q test-sbc; then
    echo -e "${YELLOW}⚠ SBC failed to start. Check: docker logs test-sbc${NC}"
    exit 1
fi

SBC_IP=$(ifconfig 2>/dev/null | grep "inet " | grep -v 127.0.0.1 | head -1 | awk '{print $2}')

echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║           Test SBC Running                              ║${NC}"
echo -e "${GREEN}╠══════════════════════════════════════════════════════════╣${NC}"
echo -e "${GREEN}║                                                         ║${NC}"
echo -e "${GREEN}║  SBC Address:  localhost:5060                            ║${NC}"
echo -e "${GREEN}║  LAN Address:  ${SBC_IP:-?}:5060                         ${NC}"
echo -e "${GREEN}║  Gateway:      ${GATEWAY_HOST}:5062                      ${NC}"
echo -e "${GREEN}║                                                         ║${NC}"
echo -e "${GREEN}║  Softphone Setup:                                       ║${NC}"
echo -e "${GREEN}║    Server:    localhost (or ${SBC_IP:-this IP})           ${NC}"
echo -e "${GREEN}║    Port:      5060                                       ║${NC}"
echo -e "${GREEN}║    Username:  caller1 (any name works)                   ║${NC}"
echo -e "${GREEN}║    Password:  (leave empty)                              ║${NC}"
echo -e "${GREEN}║    Transport: UDP                                        ║${NC}"
echo -e "${GREEN}║                                                         ║${NC}"
echo -e "${GREEN}║  Test call (no softphone needed):                        ║${NC}"
echo -e "${GREEN}║    python3 test/sip_test_call.py 127.0.0.1:5062 60      ║${NC}"
echo -e "${GREEN}║                                                         ║${NC}"
echo -e "${GREEN}║  Commands:                                               ║${NC}"
echo -e "${GREEN}║    ./test/sbc/start-sbc.sh stop    — Stop SBC            ║${NC}"
echo -e "${GREEN}║    ./test/sbc/start-sbc.sh logs    — View SBC logs       ║${NC}"
echo -e "${GREEN}║    ./test/sbc/start-sbc.sh status  — Check status        ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════╝${NC}"
