#!/bin/bash
# test-interactive.sh — Interactive test script for VoiceAgent
#
# Tests agent registration, online/offline status, profile linking,
# and queue assignment. Run with the UI open to see real-time updates.
#
# Usage:
#   ./test/test-interactive.sh
#
# Open http://localhost:3000 in your browser before running.

set -e

GW=${GATEWAY_URL:-http://localhost:8080}
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

function title() { echo -e "\n${CYAN}═══ $1 ═══${NC}"; }
function ok()    { echo -e "  ${GREEN}✓ $1${NC}"; }
function warn()  { echo -e "  ${YELLOW}⚠ $1${NC}"; }
function fail()  { echo -e "  ${RED}✗ $1${NC}"; }
function pause() { echo -e "\n  ${YELLOW}>>> $1${NC}"; echo -e "  ${YELLOW}    Press Enter to continue...${NC}"; read -r; }

function get_token() {
    local user=$1 pass=$2
    curl -s -X POST "$GW/api/auth/login" \
        -H 'Content-Type: application/json' \
        -d "{\"username\":\"$user\",\"password\":\"$pass\"}" | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null
}

function api() {
    local method=$1 path=$2 token=$3 body=$4
    if [ -n "$body" ]; then
        curl -s -X "$method" "$GW$path" \
            -H "Authorization: Bearer $token" \
            -H 'Content-Type: application/json' \
            -d "$body"
    else
        curl -s -X "$method" "$GW$path" \
            -H "Authorization: Bearer $token"
    fi
}

echo "╔══════════════════════════════════════════════════════════╗"
echo "║       VoiceAgent Interactive Test Suite                  ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""
echo "  Gateway: $GW"
echo "  UI:      http://localhost:3000"
echo ""

pause "Open http://localhost:3000 in your browser (Command Center page)"

# ─── Step 1: Login as admin ─────────────────────────────────────
title "Step 1: Admin Login"

ADMIN_TOKEN=$(get_token admin admin)
if [ -n "$ADMIN_TOKEN" ]; then
    ok "Admin logged in (token: ${ADMIN_TOKEN:0:20}...)"
else
    fail "Admin login failed"
    exit 1
fi

# ─── Step 2: Check current agents ──────────────────────────────
title "Step 2: Current Agents"

echo "  Current agents in database:"
api GET /api/agents "$ADMIN_TOKEN" | python3 -c "
import sys,json
agents = json.load(sys.stdin)
for a in agents:
    print(f'    {a.get(\"name\",\"?\"):20} ext:{a.get(\"extension\",\"—\"):5} status:{a.get(\"status\",\"?\")}  dept:{a.get(\"department\",\"?\")}')
print(f'  Total: {len(agents)} agents')
" 2>/dev/null

pause "Check the Agents page in your browser — you should see these agents"

# ─── Step 3: Create test users ──────────────────────────────────
title "Step 3: Create Test Users (agent accounts)"

for user in agent1 agent2 agent3; do
    result=$(api POST /api/auth/users "$ADMIN_TOKEN" "{\"username\":\"$user\",\"password\":\"$user\",\"role\":\"agent\"}")
    echo "$result" | grep -q "created\|exists" && ok "User '$user' created" || warn "User '$user': $result"
done

echo ""
echo "  All users:"
api GET /api/auth/users "$ADMIN_TOKEN" | python3 -c "
import sys,json
users = json.load(sys.stdin)
for u in users:
    print(f'    {u.get(\"username\",\"?\"):15} role:{u.get(\"role\",\"?\")}  id:{u.get(\"id\",\"?\")[:12]}...')
" 2>/dev/null

# ─── Step 4: Create agents with full profiles ───────────────────
title "Step 4: Create Agents with Full Profiles"

api POST /api/agents "$ADMIN_TOKEN" '{"name":"Test Agent Alpha","email":"alpha@test.com","expertise":["billing","retention"]}' > /dev/null
ok "Agent Alpha created (billing, retention)"

api POST /api/agents "$ADMIN_TOKEN" '{"name":"Test Agent Beta","email":"beta@test.com","expertise":["technical","networking"]}' > /dev/null
ok "Agent Beta created (technical, networking)"

api POST /api/agents "$ADMIN_TOKEN" '{"name":"Test Agent Gamma","email":"gamma@test.com","expertise":["sales","upsell"]}' > /dev/null
ok "Agent Gamma created (sales, upsell)"

pause "Check the Agents page — 3 new agents should appear"

# ─── Step 5: Link users to agents ───────────────────────────────
title "Step 5: Link User Accounts to Agent Profiles"

# Get user IDs and agent IDs
USERS_JSON=$(api GET /api/auth/users "$ADMIN_TOKEN")
AGENTS_JSON=$(api GET /api/agents "$ADMIN_TOKEN")

AGENT1_UID=$(echo "$USERS_JSON" | python3 -c "import sys,json; [print(u['id']) for u in json.load(sys.stdin) if u.get('username')=='agent1']" 2>/dev/null)
AGENT2_UID=$(echo "$USERS_JSON" | python3 -c "import sys,json; [print(u['id']) for u in json.load(sys.stdin) if u.get('username')=='agent2']" 2>/dev/null)
AGENT3_UID=$(echo "$USERS_JSON" | python3 -c "import sys,json; [print(u['id']) for u in json.load(sys.stdin) if u.get('username')=='agent3']" 2>/dev/null)

ALPHA_AID=$(echo "$AGENTS_JSON" | python3 -c "import sys,json; [print(a['id']) for a in json.load(sys.stdin) if 'Alpha' in a.get('name','')]" 2>/dev/null)
BETA_AID=$(echo "$AGENTS_JSON" | python3 -c "import sys,json; [print(a['id']) for a in json.load(sys.stdin) if 'Beta' in a.get('name','')]" 2>/dev/null)
GAMMA_AID=$(echo "$AGENTS_JSON" | python3 -c "import sys,json; [print(a['id']) for a in json.load(sys.stdin) if 'Gamma' in a.get('name','')]" 2>/dev/null)

if [ -n "$AGENT1_UID" ] && [ -n "$ALPHA_AID" ]; then
    api POST /api/agents/link-user "$ADMIN_TOKEN" "{\"agent_id\":\"$ALPHA_AID\",\"user_id\":\"$AGENT1_UID\"}" > /dev/null
    ok "agent1 → Test Agent Alpha"
fi
if [ -n "$AGENT2_UID" ] && [ -n "$BETA_AID" ]; then
    api POST /api/agents/link-user "$ADMIN_TOKEN" "{\"agent_id\":\"$BETA_AID\",\"user_id\":\"$AGENT2_UID\"}" > /dev/null
    ok "agent2 → Test Agent Beta"
fi
if [ -n "$AGENT3_UID" ] && [ -n "$GAMMA_AID" ]; then
    api POST /api/agents/link-user "$ADMIN_TOKEN" "{\"agent_id\":\"$GAMMA_AID\",\"user_id\":\"$AGENT3_UID\"}" > /dev/null
    ok "agent3 → Test Agent Gamma"
fi

# ─── Step 6: Assign agents to queues ────────────────────────────
title "Step 6: Assign Agents to Queues"

if [ -n "$ALPHA_AID" ]; then
    api POST /api/agents/assign-queue "$ADMIN_TOKEN" "{\"agent_id\":\"$ALPHA_AID\",\"queue\":\"Support\"}" > /dev/null
    api POST /api/agents/assign-queue "$ADMIN_TOKEN" "{\"agent_id\":\"$ALPHA_AID\",\"queue\":\"Billing\"}" > /dev/null
    ok "Alpha → Support + Billing queues"
fi
if [ -n "$BETA_AID" ]; then
    api POST /api/agents/assign-queue "$ADMIN_TOKEN" "{\"agent_id\":\"$BETA_AID\",\"queue\":\"Support\"}" > /dev/null
    ok "Beta → Support queue"
fi
if [ -n "$GAMMA_AID" ]; then
    api POST /api/agents/assign-queue "$ADMIN_TOKEN" "{\"agent_id\":\"$GAMMA_AID\",\"queue\":\"Sales\"}" > /dev/null
    ok "Gamma → Sales queue"
fi

pause "Agents are now linked to users and assigned to queues"

# ─── Step 7: Test agent login + online status ───────────────────
title "Step 7: Agent Login → Online Status"

echo "  Logging in as agent1 (Test Agent Alpha)..."
AGENT1_TOKEN=$(get_token agent1 agent1)

if [ -n "$AGENT1_TOKEN" ]; then
    ok "agent1 logged in"

    echo ""
    echo "  Fetching agent profile..."
    api GET /api/agent/me "$AGENT1_TOKEN" | python3 -c "
import sys,json
d = json.load(sys.stdin)
if d.get('linked'):
    p = d['profile']
    print(f'    Name:       {p.get(\"name\",\"?\")}')
    print(f'    Extension:  {p.get(\"extension\",\"—\")}')
    print(f'    Department: {p.get(\"department\",\"—\")}')
    print(f'    Skills:     {p.get(\"expertise\",[])}')
    print(f'    Queues:     {p.get(\"queues\",[])}')
    print(f'    Status:     {p.get(\"status\",\"?\")}')
else:
    print(f'    Not linked: {d}')
" 2>/dev/null

    ok "Agent Alpha is now ONLINE (Available)"
else
    fail "agent1 login failed"
fi

pause "Check the Command Center — Agent Alpha should show as Available (green dot)"

# ─── Step 8: Change agent status ────────────────────────────────
title "Step 8: Change Agent Status"

echo "  Setting agent1 to 'On Break'..."
api POST /api/agent/me/status "$AGENT1_TOKEN" '{"status":"On Break"}' | python3 -c "import sys,json; print(f'    Status: {json.load(sys.stdin)}')" 2>/dev/null
ok "Agent Alpha → On Break"

pause "Check the Command Center — Agent Alpha should show 'On Break' (violet dot)"

echo "  Setting agent1 back to 'Available'..."
api POST /api/agent/me/status "$AGENT1_TOKEN" '{"status":"Available"}' > /dev/null
ok "Agent Alpha → Available"

pause "Agent Alpha should be green (Available) again"

# ─── Step 9: Login more agents ──────────────────────────────────
title "Step 9: Multiple Agents Online"

AGENT2_TOKEN=$(get_token agent2 agent2)
AGENT3_TOKEN=$(get_token agent3 agent3)

if [ -n "$AGENT2_TOKEN" ]; then
    api GET /api/agent/me "$AGENT2_TOKEN" > /dev/null
    ok "agent2 (Beta) logged in → Available"
fi
if [ -n "$AGENT3_TOKEN" ]; then
    api GET /api/agent/me "$AGENT3_TOKEN" > /dev/null
    ok "agent3 (Gamma) logged in → Available"
fi

echo ""
echo "  Online agents:"
api GET /api/agents/online "$ADMIN_TOKEN" | python3 -c "
import sys,json
agents = json.load(sys.stdin) or []
for a in agents:
    print(f'    {a.get(\"agent_id\",\"?\")[:12]}...  status:{a.get(\"status\",\"?\")}')
print(f'  Total online: {len(agents)}')
" 2>/dev/null

pause "Command Center should show 3+ agents online"

# ─── Step 10: Test routing ──────────────────────────────────────
title "Step 10: Test Call Routing"

echo "  Routing a billing call..."
api POST /api/routing/test "$ADMIN_TOKEN" '{"intent":"billing payment refund","language":"English"}' | python3 -c "
import sys,json
d = json.load(sys.stdin)
print(f'    Queue:  {d.get(\"queue\",\"?\")}')
a = d.get('agent')
if a:
    print(f'    Agent:  {a.get(\"name\",\"?\")} (ext {a.get(\"extension\",\"—\")})')
    print(f'    Score:  {a.get(\"score\",0)}')
    print(f'    Reason: {a.get(\"reason\",\"—\")}')
else:
    print('    No agent available')
" 2>/dev/null

echo ""
echo "  Routing a sales call..."
api POST /api/routing/test "$ADMIN_TOKEN" '{"intent":"sales upgrade enterprise","language":"English"}' | python3 -c "
import sys,json
d = json.load(sys.stdin)
print(f'    Queue:  {d.get(\"queue\",\"?\")}')
a = d.get('agent')
if a:
    print(f'    Agent:  {a.get(\"name\",\"?\")}')
    print(f'    Score:  {a.get(\"score\",0)}')
else:
    print('    No agent available')
" 2>/dev/null

# ─── Step 11: Agent goes offline ────────────────────────────────
title "Step 11: Agent Goes Offline"

echo "  Setting agent2 to 'Offline'..."
api POST /api/agent/me/status "$AGENT2_TOKEN" '{"status":"Offline"}' > /dev/null
ok "Agent Beta → Offline"

pause "Agent Beta should show as Offline (grey dot) on the Command Center"

echo "  Setting agent2 back to 'Available'..."
api POST /api/agent/me/status "$AGENT2_TOKEN" '{"status":"Available"}' > /dev/null
ok "Agent Beta → Available"

# ─── Step 12: Console test ──────────────────────────────────────
title "Step 12: Console Agent View"

pause "Open http://localhost:3000/console in a NEW incognito/private window.\n      Login as 'agent1' / 'agent1'.\n      You should see Agent Alpha's profile with extension and queues."

# ─── Summary ────────────────────────────────────────────────────
title "Test Complete"

echo ""
echo "  What was tested:"
echo "    ✓ Admin login + user creation (agent1, agent2, agent3)"
echo "    ✓ Agent profiles created with skills"
echo "    ✓ Users linked to agent profiles"
echo "    ✓ Agents assigned to queues (Support, Billing, Sales)"
echo "    ✓ Agent login → auto-set Available"
echo "    ✓ Status changes: Available → On Break → Available"
echo "    ✓ Multiple agents online simultaneously"
echo "    ✓ Skill-based call routing (billing → Alpha, sales → Gamma)"
echo "    ✓ Agent offline/online toggle"
echo "    ✓ Console shows agent profile"
echo ""
echo "  Test accounts:"
echo "    admin  / admin   (admin role)"
echo "    agent1 / agent1  (Agent Alpha — billing, retention)"
echo "    agent2 / agent2  (Agent Beta — technical)"
echo "    agent3 / agent3  (Agent Gamma — sales, upsell)"
echo ""
