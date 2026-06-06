#!/bin/bash
# callcenter-live.sh — Live call center demo with YOUR microphone
#
# You play BOTH roles:
#   - Speak as the customer (your mic is the customer leg)
#   - Read the co-pilot suggestions on screen (you're the agent)
#
# The co-pilot uses RAG to pull answers from the knowledge base.
#
# Usage:
#   ./callcenter-live.sh

set -eo pipefail

GATEWAY="${GATEWAY_URL:-localhost:8080}"
CALL_ID="live-cc-$(date +%s)"

cat << BANNER

  ╔════════════════════════════════════════════════════════════╗
  ║        AI CALL CENTER — LIVE MIC DEMO                     ║
  ║                                                           ║
  ║  You speak as the CUSTOMER into your microphone.           ║
  ║  Watch the CO-PILOT suggestions appear in real-time.       ║
  ║                                                           ║
  ║  Try saying things like:                                   ║
  ║    "I had a burst pipe and water damaged my floor"         ║
  ║    "Can you waive the late fee on my bill?"                ║
  ║    "I want to cancel my policy"                            ║
  ║                                                           ║
  ║  Call ID: ${CALL_ID}
  ║  Press Ctrl+C to hang up.                                  ║
  ╚════════════════════════════════════════════════════════════╝

BANNER

echo "  Starting co-pilot SSE listener..."
echo ""

# Start SSE listener that prints co-pilot events in real-time
(
  sleep 2
  curl -s -N "http://${GATEWAY}/siprec/events?call_id=${CALL_ID}" 2>/dev/null | while IFS= read -r line; do
    if [[ "$line" == data:* ]]; then
      data="${line#data: }"
      python3 -c "
import json
try:
    e = json.loads('''$data''')
    t = e.get('type','')
    if t == 'transcript':
        speaker = e['speaker'].upper()
        color = '\033[36m' if speaker == 'CUSTOMER' else '\033[32m'
        print(f'{color}  [{speaker:8}]\033[0m {e[\"text\"]}')
    elif t == 'suggestion':
        cat = e.get('category','')
        conf = e.get('confidence',0)
        print(f'\033[35m  >>> CO-PILOT ({cat}, {conf*100:.0f}%): {e.get(\"suggestion\",\"\")}\033[0m')
    elif t == 'summary':
        print()
        print('\033[33m  ═══ POST-CALL SUMMARY ═══\033[0m')
        print(f'\033[33m  {e.get(\"summary\",\"\")}\033[0m')
        items = e.get('action_items',[])
        if items:
            for i in (items if isinstance(items,list) else []):
                print(f'\033[33m    - {i}\033[0m')
        print(f'\033[33m  Sentiment: {e.get(\"sentiment\",\"?\")} | Duration: {e.get(\"duration\",0)}s\033[0m')
except Exception as ex:
    pass
" 2>/dev/null
    fi
  done
) &
SSE_PID=$!

# Connect silent agent leg in background
python3 -c "
import websocket, time, json
ws = websocket.WebSocket()
ws.connect('ws://${GATEWAY}/siprec?role=agent&call_id=${CALL_ID}')
silence = b'\x00' * 640
try:
    while True:
        ws.send(silence, opcode=websocket.ABNF.OPCODE_BINARY)
        time.sleep(0.02)
except:
    pass
ws.close()
" 2>/dev/null &
AGENT_PID=$!

sleep 1

# Run livecall connecting to /siprec as customer (your mic)
cd /Users/kapjain/LLMD/voiceagent/test
./livecall "ws://${GATEWAY}/siprec?role=caller&call_id=${CALL_ID}" 2>&1

# Cleanup
kill $SSE_PID 2>/dev/null
kill $AGENT_PID 2>/dev/null
wait 2>/dev/null

echo ""
echo "  Call ended. Check full logs:"
echo "  docker logs voiceagent-gateway-1 | grep '${CALL_ID}'"
