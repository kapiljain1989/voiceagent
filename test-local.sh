#!/bin/bash
# test-local.sh — Start test environment and run tests
#
# Usage:
#   ./test-local.sh up       # Start all services
#   ./test-local.sh call     # Make a test SIP call (60s)
#   ./test-local.sh setup    # Create agents and users
#   ./test-local.sh down     # Teardown everything
#   ./test-local.sh logs     # Follow gateway logs
#   ./test-local.sh status   # Check all services

set -e

COMPOSE="docker compose -f docker-compose.test.yml"
GW_PORT="${GW_PORT:-8080}"
UI_PORT="${UI_PORT:-3000}"
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
NC='\033[0m'

case "${1:-help}" in
  up)
    echo -e "${CYAN}Starting test environment...${NC}"
    $COMPOSE up -d --build 2>&1 | tail -10
    echo ""
    echo -e "${CYAN}Waiting for services...${NC}"
    sleep 15
    echo ""
    echo -e "${GREEN}╔═══════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║     Test Environment Ready                ║${NC}"
    echo -e "${GREEN}╠═══════════════════════════════════════════╣${NC}"
    echo -e "${GREEN}║  Console:  http://localhost:${UI_PORT}           ║${NC}"
    echo -e "${GREEN}║  Gateway:  http://localhost:${GW_PORT}           ║${NC}"
    echo -e "${GREEN}║                                           ║${NC}"
    echo -e "${GREEN}║  Next steps:                              ║${NC}"
    echo -e "${GREEN}║    ./test-local.sh setup  (create agents) ║${NC}"
    echo -e "${GREEN}║    ./test-local.sh call   (test SIP call) ║${NC}"
    echo -e "${GREEN}╚═══════════════════════════════════════════╝${NC}"
    ;;

  call)
    echo -e "${CYAN}Making test SIP call (60s)...${NC}"
    echo -e "${YELLOW}Open http://localhost:3000/console (login as sarah)${NC}"
    echo -e "${YELLOW}Click PICK when the call appears${NC}"
    echo ""
    $COMPOSE run --rm test-call
    ;;

  setup)
    echo -e "${CYAN}Setting up agents...${NC}"
    GATEWAY_URL=http://localhost:${GW_PORT} ./test/manage-agents.sh setup
    ;;

  down)
    echo -e "${CYAN}Tearing down...${NC}"
    $COMPOSE down -v
    echo -e "${GREEN}✓ Done${NC}"
    ;;

  logs)
    $COMPOSE logs -f gateway
    ;;

  status)
    echo -e "${CYAN}=== Containers ===${NC}"
    $COMPOSE ps
    echo ""
    echo -e "${CYAN}=== Gateway ===${NC}"
    curl -s http://localhost:${GW_PORT}/healthz 2>&1 || echo "Not reachable"
    echo ""
    echo -e "${CYAN}=== UI ===${NC}"
    curl -s -o /dev/null -w "HTTP %{http_code}" http://localhost:${UI_PORT} 2>&1 || echo "Not reachable"
    echo ""
    ;;

  *)
    echo "Usage: $0 {up|call|setup|down|logs|status}"
    echo ""
    echo "  up     — Start all services (gateway, DB, whisper, UI, SBC)"
    echo "  call   — Run a test SIP call (60s, 440Hz tone)"
    echo "  setup  — Create admin + agent users"
    echo "  down   — Teardown everything (including data)"
    echo "  logs   — Follow gateway logs"
    echo "  status — Check all services"
    ;;
esac
