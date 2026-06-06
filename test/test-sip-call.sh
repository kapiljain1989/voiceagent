#!/bin/bash
# test-sip-call.sh — Interactive SIP voice call via baresip
set -euo pipefail

BARESIP_DIR="$HOME/.baresip"
MODULE_PATH="/opt/homebrew/lib/baresip/modules"
LOCAL_IP=$(ipconfig getifaddr en0 2>/dev/null || echo "192.168.1.156")

mkdir -p "$BARESIP_DIR"

cat > "$BARESIP_DIR/config" << EOF
module_path             ${MODULE_PATH}

audio_player            coreaudio
audio_source            coreaudio
audio_srate             8000
audio_channels          1

sip_trans_def           udp
net_interface           en0

module                  g711.so
module                  coreaudio.so
module                  audiounit.so
module                  auconv.so
module                  auresamp.so
module                  account.so
module                  stdio.so
module                  menu.so
EOF

# Account points directly at FreeSWITCH — dialing "1000" sends
# INVITE sip:1000@${LOCAL_IP}:5060 via TCP
SIP_PORT="${SIP_PORT:-5070}"

cat > "$BARESIP_DIR/accounts" << EOF
<sip:caller@${LOCAL_IP}:${SIP_PORT}>;regint=0;answermode=manual
EOF

echo "╔══════════════════════════════════════════════╗"
echo "║       SIP Voice Call (baresip → FreeSWITCH)  ║"
echo "╚══════════════════════════════════════════════╝"
echo ""
echo "  Local IP: ${LOCAL_IP}"
echo "  FreeSWITCH: ${LOCAL_IP}:${SIP_PORT} (TCP)"
echo ""
echo "  Commands:"
echo "    /dial 1000          — place call"
echo "    /hangup             — hang up"
echo "    /quit               — exit"
echo ""

exec baresip
