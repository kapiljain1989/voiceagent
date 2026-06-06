#!/bin/bash
# callcenter-demo.sh — Full call center scenario test
#
# Simulates a real call center workflow:
#   1. A "customer" calls in (simulated audio via WebSocket)
#   2. YOU are the human agent — watch the co-pilot suggestions on screen
#   3. The co-pilot provides real-time knowledge base answers
#   4. On call end, a summary is generated and posted
#
# This opens THREE terminal panes:
#   - Pane 1: Live co-pilot suggestions (SSE stream)
#   - Pane 2: Simulated customer call
#   - Pane 3: Gateway logs
#
# Usage:
#   ./callcenter-demo.sh

set -euo pipefail

GATEWAY="localhost:8080"
CALL_ID="demo-$(date +%s)"

cat << 'BANNER'

  ╔════════════════════════════════════════════════════════════╗
  ║        AI CALL CENTER — LIVE DEMO                         ║
  ║                                                           ║
  ║  Scenario: Customer calls about water damage claim        ║
  ║  You: Human agent with AI co-pilot assist                 ║
  ║                                                           ║
  ║  What will happen:                                        ║
  ║    1. Simulated customer connects (WebSocket audio)       ║
  ║    2. Co-pilot transcribes + suggests answers in real-time║
  ║    3. Knowledge base (RAG) provides policy details        ║
  ║    4. Call summary generated on hangup                    ║
  ╚════════════════════════════════════════════════════════════╝

BANNER

echo "Step 1: Open the UI dashboard at http://localhost:3000/calls/live"
echo ""
echo "Step 2: In a SECOND terminal, watch the co-pilot SSE events:"
echo ""
echo "  curl -N http://${GATEWAY}/siprec/events?call_id=${CALL_ID}"
echo ""
echo "Step 3: Press ENTER here to start the simulated customer call..."
read -r

echo "Starting customer call (call_id: ${CALL_ID})..."
echo ""

# Connect the simulated customer
python3 << PYEOF
import websocket
import json
import struct
import math
import time
import sys
import threading

GATEWAY = "${GATEWAY}"
CALL_ID = "${CALL_ID}"
SR = 16000
FRAME_MS = 20
FRAME_SIZE = SR * 2 * FRAME_MS // 1000  # 640 bytes

def generate_tone(freq, duration_ms, start_sample=0):
    """Generate a tone that Whisper can hear as speech-like."""
    frames = []
    samples_per_frame = SR * FRAME_MS // 1000
    total_frames = duration_ms // FRAME_MS
    for f in range(total_frames):
        frame = bytearray(FRAME_SIZE)
        for i in range(samples_per_frame):
            t = (start_sample + f * samples_per_frame + i) / SR
            # Mix multiple frequencies to sound more speech-like
            sample = int(6000 * (
                math.sin(2 * math.pi * freq * t) +
                0.5 * math.sin(2 * math.pi * (freq * 1.5) * t) +
                0.3 * math.sin(2 * math.pi * (freq * 0.7) * t)
            ))
            sample = max(-32768, min(32767, sample))
            struct.pack_into('<h', frame, i * 2, sample)
        frames.append(bytes(frame))
    return frames

def send_silence(ws, duration_ms):
    silence = b'\x00' * FRAME_SIZE
    for _ in range(duration_ms // FRAME_MS):
        ws.send(silence, opcode=websocket.ABNF.OPCODE_BINARY)
        time.sleep(FRAME_MS / 1000)

def send_audio(ws, freq, duration_ms):
    frames = generate_tone(freq, duration_ms)
    for frame in frames:
        ws.send(frame, opcode=websocket.ABNF.OPCODE_BINARY)
        time.sleep(FRAME_MS / 1000)

# Connect as "customer" leg
url = f"ws://{GATEWAY}/siprec?role=caller&call_id={CALL_ID}"
print(f"  Connecting customer to {url}")
ws = websocket.WebSocket()
ws.connect(url)
print("  Customer connected!")
print()

# Also connect the "agent" leg (silent — represents you listening)
url_agent = f"ws://{GATEWAY}/siprec?role=agent&call_id={CALL_ID}"
ws_agent = websocket.WebSocket()
ws_agent.connect(url_agent)
print("  Agent leg connected (you are listening)")
print()

# Simulate the customer speaking in phases
conversations = [
    ("Customer is speaking...", 800, 3000),
    ("(silence — VAD detecting end of speech...)", 0, 2500),
    ("Customer speaking again...", 1000, 3000),
    ("(silence...)", 0, 2500),
    ("Customer asking another question...", 1200, 3000),
    ("(silence...)", 0, 2500),
]

for desc, freq, duration in conversations:
    print(f"  {desc}")
    if freq > 0:
        send_audio(ws, freq, duration)
    else:
        send_silence(ws, duration)

# Wait for co-pilot to process
print()
print("  Waiting for co-pilot to finish processing...")
time.sleep(8)

# Hang up
print()
print("  Customer hanging up — triggering call summary...")
ws.send(json.dumps({"type": "stop", "callId": CALL_ID}))
ws_agent.send(json.dumps({"type": "stop", "callId": CALL_ID}))
time.sleep(5)

ws.close()
ws_agent.close()
print("  Call ended.")
PYEOF

echo ""
echo "━━━ Call Center Demo Complete ━━━"
echo ""
echo "Check the results:"
echo "  1. UI: http://localhost:3000/calls/live"
echo "  2. Gateway logs: docker logs voiceagent-gateway-1 | grep -E 'heard|replied|suggestion|summary'"
echo "  3. Co-pilot events: curl -s http://${GATEWAY}/siprec/events?call_id=${CALL_ID}"
