# Softphone Setup Guide — Local SBC Lab

Test VoiceAgent with real voice calls from your mobile phone or laptop on your home network.

---

## Architecture

```
Mobile (Customer)  ──► Kamailio SBC (:5060) ──► FreeSWITCH ──► Gateway AI
Laptop (Agent)     ──►       ↕ bridge + SIPREC fork              ↓
                        ◄── RTP audio ──►                    Whisper → Claude → Piper
```

## Prerequisites

```bash
# Start the SBC lab (11 services)
export EXT_IP=$(ipconfig getifaddr en0)
make sbc-lab

# Verify Kamailio is running
docker compose -f docker-compose.sip.yml -f docker-compose.sbc.yml ps | grep kamailio
```

Your Mac's LAN IP (e.g., `192.168.1.156`) is the SIP server address for all softphones.

> **macOS Docker Desktop Note:** Docker Desktop on macOS cannot reliably forward inbound UDP from the LAN to containers. For real voice calls with audio, connect your softphone directly to FreeSWITCH on port **5070**. Use Kamailio on port 5080 for SBC routing/registration feature testing.

---

## Mobile Softphone Setup

### iOS

**Recommended:** Opal SIP, Opal, or any SIP-compatible app from the App Store.

**Option A: Direct to FreeSWITCH (audio works)**

| Setting | Value |
|---------|-------|
| SIP Server | `192.168.x.x` (your Mac's LAN IP) |
| Port | **`5090`** |
| Transport | **TCP** |
| Username | `customer1` |
| Password | **`1234`** |
| Display Name | `Customer` |
| STUN | Disabled |
| ICE | Disabled |
| Codec | G.711 u-law (PCMU) |

Available users: `customer1`, `customer2`, `agent1` — all with password `1234`.

> Use TCP transport — Docker Desktop on macOS forwards TCP reliably but drops inbound UDP from the LAN.

**Option B: Via Kamailio SBC (SBC feature testing)**

| Setting | Value |
|---------|-------|
| SIP Server | `192.168.x.x` |
| Port | `5080` |
| Transport | UDP |
| Username | `customer1` |
| Password | *(leave empty)* |

> Signaling works for registration and call setup testing. Audio requires Linux host or rtpengine relay.

### Android

Same settings as iOS above — use Option A (port 5070, TCP) for audio.

### Verify Registration

After configuring, the softphone should show "Registered" or a green status. Verify on the server:

```bash
docker exec voiceagent-kamailio-1 kamcmd ul.dump 2>/dev/null || \
  docker logs voiceagent-kamailio-1 2>&1 | grep REGISTER | tail -5
```

---

## Laptop Agent Setup (baresip)

For co-pilot testing, register your laptop as the agent:

```bash
# Install baresip
brew install baresip

# Configure
mkdir -p ~/.baresip
cat > ~/.baresip/accounts <<'EOF'
<sip:agent1@192.168.x.x:5090>;regint=60;transport=tcp;auth_pass=1234
EOF

cat > ~/.baresip/config <<'EOF'
module_path /opt/homebrew/lib/baresip/modules
module stdio.so
module menu.so
module g711.so
module aubridge.so
module coreaudio.so

audio_player coreaudio,default
audio_source coreaudio,default
audio_srate 8000
audio_channels 1
EOF

# Start (replace IP)
baresip
# Type: /dial 1000   ← calls AI agent
# Type: /accept      ← answer incoming call from customer
```

---

## Test Calls

### Scenario A: AI Agent (from mobile)

1. Open softphone on mobile
2. Dial **`1000`**
3. Speak — Claude AI responds through your phone speaker
4. Hang up

**What happens:** Mobile → Kamailio → FreeSWITCH → Gateway (Whisper STT → Claude → Piper TTS) → audio back to mobile

### Scenario B: Co-Pilot with Laptop Agent

1. Start baresip on laptop (registered as `agent1`)
2. Open dashboard: `http://192.168.x.x:3000/calls/live`
3. From mobile, dial **`2000`**
4. Baresip rings on laptop — type `/accept`
5. Talk between mobile (customer) and laptop (agent)
6. Watch co-pilot suggestions appear in the dashboard
7. Hang up — post-call summary appears

**What happens:** Mobile → Kamailio bridges to laptop + SIPREC forks to Gateway → dual-leg STT → Claude coaching → SSE to dashboard

### Scenario C: Co-Pilot with 2nd Mobile

Same as Scenario B, but register a second phone as `agent1` instead of using baresip.

### Scenario D: Co-Pilot Auto-Answer (no agent needed)

1. From mobile, dial **`2100`**
2. FreeSWITCH auto-answers the agent leg
3. Speak — SIPREC pipeline runs, co-pilot suggestions appear in dashboard
4. Hang up

### Scenario E: Human Queue

1. From mobile, dial **`3000`**
2. FreeSWITCH plays hold audio
3. This is the failover target when all AI circuits are open

---

## Dial Plan Reference

| Number | Mode | Agent Leg | SIPREC |
|--------|------|-----------|--------|
| `1000`-`1999` | Interactive AI | AI answers | No |
| `2000`-`2099` | Co-Pilot Bridge | Rings registered `agent1` | Yes |
| `2100`-`2199` | Co-Pilot Auto | FreeSWITCH auto-answer | Yes |
| `3000`-`3999` | Human Queue | Hold audio | No |

---

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Softphone shows "Unregistered" | Wrong SIP server IP | Use your Mac's LAN IP (`ipconfig getifaddr en0`) |
| No audio (one-way or both) | EXT_IP not set | `export EXT_IP=$(ipconfig getifaddr en0)` and restart |
| Call connects but no AI response | Gateway not running | Check `curl http://localhost:8080/healthz` |
| "404 No Route" on dial | Wrong destination number | Use 1xxx, 2xxx, or 3xxx patterns |
| Laptop baresip doesn't ring | Not registered as `agent1` | Check baresip account config, verify registration |
| Audio choppy on mobile | WiFi congestion | Move closer to router, disable Bluetooth audio |
| Kamailio not starting | Port 5080 in use | Check `lsof -i :5080` and stop conflicting process |

---

## Automated Tests

```bash
# Run the full SBC test suite
make sbc-test

# Or individual tests
test/test-sbc-local.sh test1   # SIP OPTIONS
test/test-sbc-local.sh test2   # Registration
test/test-sbc-local.sh test3   # INVITE
test/test-sbc-local.sh test4   # Trunk API
test/test-sbc-local.sh test5   # Gateway health
```
