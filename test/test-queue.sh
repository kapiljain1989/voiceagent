#!/bin/bash
# test-queue.sh — Interactive queue testing
#
# Tests: add callers to queue, route calls, agent picks from queue
# Open http://localhost:3000/console to watch queue monitor update
#
# Usage: ./test/test-queue.sh

set -e
GW=${GATEWAY_URL:-http://localhost:8080}
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
NC='\033[0m'

function title() { echo -e "\n${CYAN}═══ $1 ═══${NC}"; }
function ok()    { echo -e "  ${GREEN}✓ $1${NC}"; }
function pause() { echo -e "\n  ${YELLOW}>>> $1${NC}"; echo -e "  ${YELLOW}    Press Enter to continue...${NC}"; read -r; }

TOKEN=$(curl -s -X POST "$GW/api/auth/login" -H 'Content-Type: application/json' -d '{"username":"admin","password":"admin"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")

echo "╔══════════════════════════════════════════════════════════╗"
echo "║       Queue Feature Test                                 ║"
echo "╚══════════════════════════════════════════════════════════╝"

pause "Open http://localhost:3000/console — watch the Queue Monitor panel"

# ─── Step 1: Show current queues ──────────────────────────────
title "Step 1: Current Queues"

curl -s "$GW/api/queues/list" -H "Authorization: Bearer $TOKEN" | python3 -c "
import sys,json
for q in json.load(sys.stdin):
    print(f'  {q[\"name\"]:15} skills:{q.get(\"skills_required\",[])}  waiting:{q.get(\"caller_count\",0)}')
"

# ─── Step 2: Set agents to Available ─────────────────────────
title "Step 2: Set agents to Available"

# Login as agent1 to make them available
AGENT1_TOKEN=$(curl -s -X POST "$GW/api/auth/login" -H 'Content-Type: application/json' -d '{"username":"agent1","password":"agent1"}' | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null)

if [ -n "$AGENT1_TOKEN" ]; then
    curl -s "$GW/api/agent/me" -H "Authorization: Bearer $AGENT1_TOKEN" > /dev/null
    ok "agent1 online (Available)"
fi

# Set Sarah Chen to Available via DB
docker exec voiceagent-postgres-1 psql -U voiceagent -d voiceagent -c "UPDATE agents SET status='Available' WHERE name='Sarah Chen';" 2>/dev/null > /dev/null
ok "Sarah Chen → Available"

docker exec voiceagent-postgres-1 psql -U voiceagent -d voiceagent -c "UPDATE agents SET status='Available' WHERE name='Marcus Johnson';" 2>/dev/null > /dev/null
ok "Marcus Johnson → Available"

pause "Check Console — agents should show as Available"

# ─── Step 3: Add callers to queues ────────────────────────────
title "Step 3: Add Callers to Queues"

echo "  Adding 3 callers to Support queue..."
curl -s -X POST "$GW/api/queue/add" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"call_id":"call-001","caller_number":"+15551110001","queue_name":"Support","reason":"Billing question","priority":"normal"}' > /dev/null
ok "Caller +15551110001 → Support (billing question)"

curl -s -X POST "$GW/api/queue/add" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"call_id":"call-002","caller_number":"+15551110002","queue_name":"Support","reason":"Technical issue","priority":"normal"}' > /dev/null
ok "Caller +15551110002 → Support (technical issue)"

curl -s -X POST "$GW/api/queue/add" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"call_id":"call-003","caller_number":"+15551110003","queue_name":"Support","reason":"Urgent complaint","priority":"high"}' > /dev/null
ok "Caller +15551110003 → Support (URGENT complaint)"

echo ""
echo "  Adding 2 callers to Sales queue..."
curl -s -X POST "$GW/api/queue/add" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"call_id":"call-004","caller_number":"+15552220004","queue_name":"Sales","reason":"Product inquiry","priority":"normal"}' > /dev/null
ok "Caller +15552220004 → Sales (product inquiry)"

curl -s -X POST "$GW/api/queue/add" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"call_id":"call-005","caller_number":"+15552220005","queue_name":"Sales","reason":"Enterprise plan","priority":"high"}' > /dev/null
ok "Caller +15552220005 → Sales (ENTERPRISE plan)"

echo ""
echo "  Adding 1 caller to Billing queue..."
curl -s -X POST "$GW/api/queue/add" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"call_id":"call-006","caller_number":"+15553330006","queue_name":"Billing","reason":"Refund request","priority":"normal"}' > /dev/null
ok "Caller +15553330006 → Billing (refund request)"

pause "Check Console Queue Monitor — you should see 6 callers across 3 queues"

# ─── Step 4: Check queue state ────────────────────────────────
title "Step 4: Queue State"

curl -s "$GW/api/queues/list" -H "Authorization: Bearer $TOKEN" | python3 -c "
import sys,json
for q in json.load(sys.stdin):
    print(f'  {q[\"name\"]:15} waiting:{q.get(\"caller_count\",0)}')
"

# ─── Step 5: Test routing ─────────────────────────────────────
title "Step 5: Route a Billing Call"

echo "  Intent: 'billing refund dispute'"
curl -s -X POST "$GW/api/routing/test" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"intent":"billing refund dispute","language":"English"}' | python3 -c "
import sys,json
d = json.load(sys.stdin)
a = d.get('agent')
print(f'  Queue:  {d.get(\"queue\")}')
if a:
    print(f'  Agent:  {a.get(\"name\")} (ext {a.get(\"extension\",\"—\")})')
    print(f'  Score:  {a.get(\"score\")}')
    print(f'  Reason: {a.get(\"reason\",\"—\")}')
else:
    print('  No agent available')
"

# ─── Step 6: Pick a call from queue ──────────────────────────
title "Step 6: Agent Picks Call from Queue"

echo "  Agent picking call-001 from Support queue..."
curl -s -X POST "$GW/api/queue/pick" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"call_id":"call-001"}' | python3 -m json.tool 2>/dev/null

pause "Check Console — Support queue should now show 2 callers (was 3)"

# ─── Step 7: Check updated queue ─────────────────────────────
title "Step 7: Updated Queue State"

curl -s "$GW/api/queues/list" -H "Authorization: Bearer $TOKEN" | python3 -c "
import sys,json
for q in json.load(sys.stdin):
    print(f'  {q[\"name\"]:15} waiting:{q.get(\"caller_count\",0)}')
"

# ─── Step 8: Clear all queues ─────────────────────────────────
title "Step 8: Clear Queues"

echo "  Clearing all queue entries..."
docker exec voiceagent-postgres-1 psql -U voiceagent -d voiceagent -c "DELETE FROM queue_entries;" 2>/dev/null > /dev/null
ok "All queues cleared"

pause "Check Console — all queues should show 0 callers"

# ─── Summary ──────────────────────────────────────────────────
title "Queue Test Complete"
echo ""
echo "  What was tested:"
echo "    ✓ Queues listed with skills and caller counts"
echo "    ✓ Callers added to Support, Sales, Billing queues"
echo "    ✓ Priority queueing (high priority callers)"
echo "    ✓ Skill-based routing (billing intent → best agent)"
echo "    ✓ Agent picks call from queue (caller removed)"
echo "    ✓ Queue cleared"
echo ""
