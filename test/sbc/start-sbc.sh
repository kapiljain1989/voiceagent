#!/bin/bash
# start-sbc.sh — Start the test SBC and show softphone config
#
# Usage:
#   ./test/sbc/start-sbc.sh          # Start SBC
#   ./test/sbc/start-sbc.sh stop     # Stop SBC

set -e

GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

DIR="$(cd "$(dirname "$0")" && pwd)"

if [ "${1:-}" = "stop" ]; then
    cd "$DIR" && docker compose down 2>/dev/null
    echo -e "${GREEN}✓ Test SBC stopped${NC}"
    exit 0
fi

echo -e "${CYAN}Starting Test SBC...${NC}"
cd "$DIR" && docker compose up -d --build 2>&1 | tail -3

sleep 2

echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║              Test SBC Running                           ║${NC}"
echo -e "${GREEN}╠══════════════════════════════════════════════════════════╣${NC}"
echo -e "${GREEN}║                                                         ║${NC}"
echo -e "${GREEN}║  SIP Address: localhost:5080                             ║${NC}"
echo -e "${GREEN}║                                                         ║${NC}"
echo -e "${GREEN}║  Softphone Setup (Zoiper/Opal/Opal.app):                ║${NC}"
echo -e "${GREEN}║    Server:   localhost                                   ║${NC}"
echo -e "${GREEN}║    Port:     5080                                        ║${NC}"
echo -e "${GREEN}║    Username: caller1  (or any name)                      ║${NC}"
echo -e "${GREEN}║    Password: (leave empty or any value)                  ║${NC}"
echo -e "${GREEN}║    Transport: UDP                                        ║${NC}"
echo -e "${GREEN}║                                                         ║${NC}"
echo -e "${GREEN}║  Call any number (e.g., 2001) → routes to gateway       ║${NC}"
echo -e "${GREEN}║                                                         ║${NC}"
echo -e "${GREEN}║  Or use the test script:                                 ║${NC}"
echo -e "${GREEN}║    python3 test/sip_test_call.py 127.0.0.1:5062 60      ║${NC}"
echo -e "${GREEN}║                                                         ║${NC}"
echo -e "${GREEN}║  Stop:  ./test/sbc/start-sbc.sh stop                    ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════╝${NC}"
