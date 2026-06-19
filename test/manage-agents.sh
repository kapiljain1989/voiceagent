#!/bin/bash
# manage-agents.sh — Create, list, and manage agents
#
# Usage:
#   ./test/manage-agents.sh setup          # Create admin + demo agents
#   ./test/manage-agents.sh list           # List all agents
#   ./test/manage-agents.sh status <name> <status>  # Set agent status
#   ./test/manage-agents.sh login <user> <pass>      # Login and get token
#
# Statuses: Available, Busy, On Break, Wrap-up, Offline

set -e

GW=${GATEWAY_URL:-http://localhost:8080}
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

ok()   { echo -e "  ${GREEN}✓ $1${NC}"; }
fail() { echo -e "  ${RED}✗ $1${NC}"; }
info() { echo -e "  ${CYAN}ℹ $1${NC}"; }

get_token() {
    local user=$1 pass=$2
    curl -s -X POST "$GW/api/auth/login" \
        -H 'Content-Type: application/json' \
        -d "{\"username\":\"$user\",\"password\":\"$pass\"}" | \
        python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null
}

api() {
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

cmd_setup() {
    echo -e "${CYAN}═══ VoiceAgent Agent Setup ═══${NC}"
    echo ""

    # 1. Login as admin
    info "Logging in as admin..."
    TOKEN=$(get_token admin admin)
    if [ -z "$TOKEN" ]; then
        fail "Admin login failed — admin user may not exist"
        info "Database may need to be re-seeded. Check gateway logs."
        exit 1
    fi
    ok "Admin logged in"

    # 2. Create agent users (username = firstname, password = firstname)
    info "Creating agent users..."
    for user in sarah alex priya; do
        result=$(api POST /api/auth/users "$TOKEN" "{\"username\":\"$user\",\"password\":\"$user\",\"role\":\"agent\"}")
        if echo "$result" | grep -q "error"; then
            info "$user already exists"
        else
            ok "User $user created"
        fi
    done

    # 3. Create agent profiles
    info "Creating agent profiles..."

    for agent_json in \
        '{"name":"Sarah Chen","email":"sarah@voiceagent.ai","phone":"+1-555-0101","extension":"2001","department":"Support","expertise":["billing","technical","returns"],"max_calls":3}' \
        '{"name":"Alex Rivera","email":"alex@voiceagent.ai","phone":"+1-555-0102","extension":"2002","department":"Sales","expertise":["pricing","enterprise","upsell"],"max_calls":3}' \
        '{"name":"Priya Sharma","email":"priya@voiceagent.ai","phone":"+1-555-0103","extension":"2003","department":"Billing","expertise":["payments","refunds","invoices"],"max_calls":3}'; do
        name=$(echo "$agent_json" | python3 -c "import sys,json; print(json.load(sys.stdin)['name'])" 2>/dev/null)
        result=$(api POST /api/agents "$TOKEN" "$agent_json")
        if echo "$result" | grep -q "already exists"; then
            info "$name already exists — skipping"
        elif echo "$result" | grep -q "created"; then
            ok "$name created"
        else
            info "$name: $result"
        fi
    done

    # 3b. Set specific statuses
    info "Setting agent statuses..."
    AGENTS=$(api GET /api/agents "$TOKEN")
    PRIYA_ID=$(echo "$AGENTS" | python3 -c "import sys,json; agents=json.load(sys.stdin); print(next((a['id'] for a in agents if a.get('name')=='Priya Sharma'),''))" 2>/dev/null)
    if [ -n "$PRIYA_ID" ]; then
        api PUT /api/agents "$TOKEN" "{\"id\":\"$PRIYA_ID\",\"status\":\"On Break\"}" > /dev/null
        ok "Priya Sharma → On Break"
    fi

    # 4. Link users to agents (requires user_id, not username)
    info "Linking users to agent profiles..."
    AGENTS=$(api GET /api/agents "$TOKEN")
    USERS=$(api GET /api/auth/users "$TOKEN")

    for pair in "sarah:Sarah Chen" "alex:Alex Rivera" "priya:Priya Sharma"; do
        user="${pair%%:*}"
        name="${pair##*:}"
        user_id=$(echo "$USERS" | python3 -c "import sys,json; print(next((u['id'] for u in json.load(sys.stdin) if u.get('username')=='$user'),''))" 2>/dev/null)
        agent_id=$(echo "$AGENTS" | python3 -c "import sys,json; print(next((a['id'] for a in json.load(sys.stdin) if a.get('name')=='$name'),''))" 2>/dev/null)
        if [ -n "$user_id" ] && [ -n "$agent_id" ]; then
            api POST /api/agents/link-user "$TOKEN" "{\"agent_id\":\"$agent_id\",\"user_id\":\"$user_id\"}" > /dev/null 2>&1
            ok "$user → $name"
        else
            info "$user or $name not found — skipping link"
        fi
    done

    echo ""
    echo -e "${GREEN}Setup complete!${NC}"
    echo ""
    echo "  Logins:"
    echo "    admin/admin  — Admin (manage agents, view all)"
    echo "    sarah/sarah  — Sarah Chen (Support, Available)"
    echo "    alex/alex    — Alex Rivera (Sales, Available)"
    echo "    priya/priya  — Priya Sharma (Billing, On Break)"
    echo ""
    echo "  Console: http://localhost:3000/console"
    echo "  Agents:  http://localhost:3000/agents"
}

cmd_list() {
    TOKEN=$(get_token admin admin)
    if [ -z "$TOKEN" ]; then fail "Admin login failed"; exit 1; fi

    echo -e "${CYAN}═══ Agents ═══${NC}"
    api GET /api/agents "$TOKEN" | python3 -c "
import sys, json
agents = json.load(sys.stdin) or []
if not agents:
    print('  No agents found. Run: ./test/manage-agents.sh setup')
for a in agents:
    status_icon = {'Available':'🟢','Busy':'🔴','On Break':'🟡','Wrap-up':'🟠','Offline':'⚫','On Call':'🔵'}.get(a.get('status',''),'⚪')
    print(f\"  {status_icon} {a.get('name','?'):20} ext:{a.get('extension','?'):6} {a.get('department','?'):12} {a.get('status','?')}\")
" 2>/dev/null
}

cmd_status() {
    local name=$1 status=$2
    if [ -z "$name" ] || [ -z "$status" ]; then
        echo "Usage: $0 status <agent-name> <Available|Busy|On Break|Wrap-up|Offline>"
        exit 1
    fi

    TOKEN=$(get_token admin admin)
    if [ -z "$TOKEN" ]; then fail "Admin login failed"; exit 1; fi

    AGENTS=$(api GET /api/agents "$TOKEN")
    AGENT_ID=$(echo "$AGENTS" | python3 -c "
import sys, json
agents = json.load(sys.stdin) or []
name = '$name'.lower()
for a in agents:
    if name in a.get('name','').lower():
        print(a['id'])
        break
" 2>/dev/null)

    if [ -z "$AGENT_ID" ]; then
        fail "Agent '$name' not found"
        exit 1
    fi

    api PUT /api/agents "$TOKEN" "{\"id\":\"$AGENT_ID\",\"status\":\"$status\"}" > /dev/null
    ok "$name → $status"
}

cmd_login() {
    local user=$1 pass=$2
    if [ -z "$user" ] || [ -z "$pass" ]; then
        echo "Usage: $0 login <username> <password>"
        exit 1
    fi
    TOKEN=$(get_token "$user" "$pass")
    if [ -z "$TOKEN" ]; then
        fail "Login failed for $user"
        exit 1
    fi
    ok "Logged in as $user"
    echo "  Token: $TOKEN"
}

# ─── Main ───────────────────────────────────────────────────────
case "${1:-help}" in
    setup)  cmd_setup ;;
    list)   cmd_list ;;
    status) cmd_status "$2" "$3" ;;
    login)  cmd_login "$2" "$3" ;;
    *)
        echo "Usage: $0 {setup|list|status|login}"
        echo ""
        echo "  setup                        Create admin + 3 demo agents"
        echo "  list                         List all agents with status"
        echo "  status <name> <status>       Set agent status"
        echo "  login <user> <pass>          Login and show token"
        echo ""
        echo "  Status values: Available, Busy, On Break, Wrap-up, Offline"
        ;;
esac
