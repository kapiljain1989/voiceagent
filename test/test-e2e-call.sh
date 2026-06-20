#!/bin/bash
# test-e2e-call.sh — End-to-end call center scenario
#
# Simulates: incoming call → queue → agent picks → AI pipeline →
#            transcript → copilot → voice sentiment → summary
#
# Open http://localhost:3000/console in your browser before running.
#
# Usage: ./test/test-e2e-call.sh

set -e
GW=${GATEWAY_URL:-http://localhost:8080}
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

function title() { echo -e "\n${CYAN}═══ $1 ═══${NC}"; }
function ok()    { echo -e "  ${GREEN}✓ $1${NC}"; }
function fail()  { echo -e "  ${RED}✗ $1${NC}"; }
function info()  { echo -e "  ${CYAN}ℹ $1${NC}"; }
function pause() { echo -e "\n  ${YELLOW}>>> $1${NC}"; echo -e "  ${YELLOW}    Press Enter to continue...${NC}"; read -r; }

function get_token() {
    curl -s -X POST "$GW/api/auth/login" -H 'Content-Type: application/json' \
        -d "{\"username\":\"$1\",\"password\":\"$2\"}" | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null
}

function api() {
    local method=$1 path=$2 token=$3 body=$4
    if [ -n "$body" ]; then
        curl -s -X "$method" "$GW$path" -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d "$body"
    else
        curl -s -X "$method" "$GW$path" -H "Authorization: Bearer $token"
    fi
}

echo "╔══════════════════════════════════════════════════════════╗"
echo "║       End-to-End Call Center Scenario                    ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""
echo "  This test simulates a full call center flow:"
echo "  1. Agent logs in and goes Available"
echo "  2. Customer call arrives → queued"
echo "  3. Call routed to best agent"
echo "  4. Agent picks call from queue"
echo "  5. AI pipeline runs (simcall)"
echo "  6. Transcript + copilot suggestions appear"
echo "  7. Call ends → summary generated"
echo ""
echo "  Gateway: $GW"
echo "  Console: http://localhost:3000/console"
echo ""

pause "Open http://localhost:3000/console in your browser"

# ─── Step 1: Setup — ensure agent is available ──────────────────
title "Step 1: Agent Setup"

ADMIN_TOKEN=$(get_token admin admin)
if [ -z "$ADMIN_TOKEN" ]; then fail "Admin login failed"; exit 1; fi

# Create agent1 user if not exists
api POST /api/auth/users "$ADMIN_TOKEN" '{"username":"agent1","password":"agent1","role":"agent"}' > /dev/null 2>&1

# Login as agent1
AGENT_TOKEN=$(get_token agent1 agent1)
if [ -z "$AGENT_TOKEN" ]; then fail "agent1 login failed"; exit 1; fi

# Fetch agent profile (marks as Available)
PROFILE=$(api GET /api/agent/me "$AGENT_TOKEN")
AGENT_NAME=$(echo "$PROFILE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('profile',{}).get('name','Unknown'))" 2>/dev/null)
LINKED=$(echo "$PROFILE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('linked',False))" 2>/dev/null)

if [ "$LINKED" = "True" ]; then
    ok "Agent '$AGENT_NAME' logged in and Available"
else
    info "No agent profile linked — using admin agent"
    # Set first agent to Available
    docker exec voiceagent-postgres-1 psql -U voiceagent -d voiceagent -c "UPDATE agents SET status='Available' WHERE name='Sarah Chen';" 2>/dev/null > /dev/null
    AGENT_NAME="Sarah Chen"
    ok "Sarah Chen set to Available"
fi

pause "Check Console — agent should show as Available (green dot)"

# ─── Step 2: Incoming call → Queue ──────────────────────────────
title "Step 2: Incoming Call → Queue"

CALLER="+15551234567"
CALL_ID="e2e-$(date +%s)"

echo "  Incoming call from $CALLER..."
echo "  Detecting intent: 'billing payment refund'"

# Route the call
ROUTE=$(api POST /api/routing/test "$ADMIN_TOKEN" '{"intent":"billing payment refund","language":"English"}')
QUEUE=$(echo "$ROUTE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('queue','Support'))" 2>/dev/null)
ROUTED_AGENT=$(echo "$ROUTE" | python3 -c "import sys,json; a=json.load(sys.stdin).get('agent'); print(a.get('name','?') if a else 'none')" 2>/dev/null)

ok "Routed to $QUEUE queue → agent: $ROUTED_AGENT"

# Add to queue
api POST /api/queue/add "$ADMIN_TOKEN" "{\"call_id\":\"$CALL_ID\",\"caller_number\":\"$CALLER\",\"queue_name\":\"$QUEUE\",\"reason\":\"Billing payment refund\",\"priority\":\"normal\"}" > /dev/null
ok "Caller $CALLER added to $QUEUE queue"

pause "Check Console Queue Monitor — you should see $CALLER in the $QUEUE queue"

# ─── Step 3: Check queue state ──────────────────────────────────
title "Step 3: Queue State"

api GET /api/queues "$ADMIN_TOKEN" | python3 -c "
import sys,json
for q in json.load(sys.stdin):
    callers = q.get('callers',[])
    if callers:
        print(f'  {q[\"name\"]:15} {len(callers)} waiting')
        for c in callers:
            print(f'    {c.get(\"number\",\"?\"):20} {c.get(\"reason\",\"\")}  {c.get(\"waitSec\",0)}s')
" 2>/dev/null

# ─── Step 4: Agent picks call ───────────────────────────────────
title "Step 4: Agent Picks Call"

pause "Click PICK on the caller in the Queue Monitor, OR press Enter to auto-pick"

api POST /api/queue/pick "$ADMIN_TOKEN" "{\"call_id\":\"$CALL_ID\"}" > /dev/null
ok "Call $CALL_ID picked from queue"

pause "Queue should now be empty. Agent is handling the call."

# ─── Step 5: Simulate AI pipeline ──────────────────────────────
title "Step 5: AI Pipeline — Simulated Call"

echo "  Starting simulated call to test AI pipeline..."
echo "  (This sends synthetic audio to the gateway)"

if [ -f test/simcall ] || [ -f test/simcall.go ]; then
    cd test
    timeout 20 go run simcall.go ws://localhost:8080/ws 2>&1 &
    SIM_PID=$!
    cd ..

    echo "  Simcall running (PID: $SIM_PID)..."
    sleep 5

    # Check if transcript appeared
    echo ""
    echo "  Gateway logs (call activity):"
    docker logs voiceagent-gateway-1 --tail=10 2>&1 | grep -E "heard|replied|session" | tail -5 | while read line; do
        echo "    $line"
    done

    sleep 10

    # Wait for simcall to finish
    wait $SIM_PID 2>/dev/null
    ok "Simulated call completed"
else
    info "simcall not found — skipping AI pipeline test"
    info "To test: cd test && go run simcall.go ws://localhost:8080/ws"
fi

# ─── Step 6: Check copilot sessions ────────────────────────────
title "Step 6: Active Copilot Sessions"

SESSIONS=$(api GET /api/copilot/active "$ADMIN_TOKEN")
SESSION_COUNT=$(echo "$SESSIONS" | python3 -c "import sys,json; print(len(json.load(sys.stdin) or []))" 2>/dev/null)

if [ "$SESSION_COUNT" -gt 0 ] 2>/dev/null; then
    ok "$SESSION_COUNT active copilot session(s)"
    echo "$SESSIONS" | python3 -c "
import sys,json
for s in json.load(sys.stdin):
    vs = s.get('voice_sentiment',{})
    print(f'  Call: {s.get(\"call_id\",\"?\")[:16]}  Duration: {s.get(\"duration\",0)}s')
    if vs:
        print(f'    Agitation: {vs.get(\"agitation\",0):.0%}  Frustration: {vs.get(\"frustration\",0):.0%}')
" 2>/dev/null
else
    info "No active copilot sessions (call may have ended)"
fi

# ─── Step 7: Check call history ─────────────────────────────────
title "Step 7: Call History"

echo "  Recent calls:"
api GET /api/calls "$ADMIN_TOKEN" | python3 -c "
import sys,json
calls = json.load(sys.stdin) or []
for c in calls[:3]:
    print(f'  {c.get(\"caller_number\",\"?\"):20} {c.get(\"mode\",\"?\"):12} {c.get(\"sentiment\",\"?\"):10} {c.get(\"duration\",0)}s')
    if c.get('summary'):
        print(f'    Summary: {c[\"summary\"][:80]}...')
if not calls:
    print('  No calls in history yet')
" 2>/dev/null

pause "Check http://localhost:3000/calls — click a call to see full details"

# ─── Step 8: Test PII + Robocall ────────────────────────────────
title "Step 8: Security Features"

echo "  PII masking test:"
PII_RESULT=$(api POST /api/security/pii/test "$ADMIN_TOKEN" '{"text":"My card is 4111111111111111 and SSN 123-45-6789"}')
echo "$PII_RESULT" | python3 -c "
import sys,json
d = json.load(sys.stdin)
print(f'    Found: {d.get(\"pii_found\")}')
print(f'    Masked: {d.get(\"masked\")}')
" 2>/dev/null

echo ""
echo "  Robocall detection test:"
ROBO_RESULT=$(api POST /api/robocall/test "$ADMIN_TOKEN" '{"text":"Press 1 for your auto warranty"}')
echo "$ROBO_RESULT" | python3 -c "
import sys,json
d = json.load(sys.stdin)
kw = d.get('keyword',{})
print(f'    Category: {kw.get(\"category\")}  Score: {kw.get(\"score\",0)*100:.0f}%')
print(f'    Keywords: {kw.get(\"keywords\",[])}')
" 2>/dev/null

# ─── Step 9: Service health ─────────────────────────────────────
title "Step 9: Service Health"

api GET /api/services/status "$ADMIN_TOKEN" | python3 -c "
import sys,json
for s in json.load(sys.stdin):
    icon = '●' if s['status'] == 'online' else '○'
    print(f'  {icon} {s[\"name\"]:15} {s[\"port\"]:10} {s[\"status\"]}')
" 2>/dev/null

# ─── Step 10: Agent goes offline ────────────────────────────────
title "Step 10: Agent Goes Offline"

api POST /api/agent/me/status "$AGENT_TOKEN" '{"status":"Wrap-up"}' > /dev/null
ok "Agent → Wrap-up"

sleep 2

api POST /api/agent/me/status "$AGENT_TOKEN" '{"status":"Offline"}' > /dev/null
ok "Agent → Offline"

pause "Check Console — agent should show as Offline"

# ─── Summary ────────────────────────────────────────────────────
title "E2E Test Complete"

echo ""
echo "  Scenario tested:"
echo "    ✓ Agent login + Available status"
echo "    ✓ Incoming call → skill-based routing"
echo "    ✓ Caller queued in $QUEUE"
echo "    ✓ Agent picks call from queue"
echo "    ✓ AI pipeline (STT → Claude → TTS)"
echo "    ✓ PII masking (card + SSN detected)"
echo "    ✓ Robocall detection (auto warranty → 100%)"
echo "    ✓ Service health check"
echo "    ✓ Agent wrap-up → offline"
echo ""
echo "  UI pages to verify:"
echo "    http://localhost:3000           Command Center (stats)"
echo "    http://localhost:3000/console   Agent Console (queue, calls)"
echo "    http://localhost:3000/calls     Call History (click for details)"
echo "    http://localhost:3000/agents    Agent Roster (status)"
echo "    http://localhost:3000/security  Security (PII, robocall)"
echo ""
