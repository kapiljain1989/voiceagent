# Demo 17: Local SBC Lab with Mobile Softphone

**Duration:** ~3 minutes
**Format:** Screen recording (terminal + mobile phone camera)
**Audience:** VoIP engineers, solution architects

---

## Scene 1: Deploy (0:00 - 0:30)

**Terminal:**
```bash
export EXT_IP=$(ipconfig getifaddr en0)
make sbc-lab
docker compose -f docker-compose.sip.yml -f docker-compose.sbc.yml ps
```

**Narration:**
> "The SBC lab adds Kamailio as a local SBC simulator on top of the 10-service platform — 11 containers total. FreeSWITCH has a softphone profile on port 5090 that accepts mobile SIP registrations."

---

## Scene 2: Mobile Registration (0:30 - 1:00)

**Mobile phone camera:** Show Zoiper app settings

**Narration:**
> "On your mobile, install any SIP app — Zoiper, opal, anything. SIP server is your Mac's LAN IP, port 5090, transport TCP, username customer1, password 1234. Hit register — you're connected to the call center lab."

**Terminal:**
```bash
docker exec voiceagent-freeswitch-1 fs_cli -P 8022 -x "sofia status profile softphone reg"
```

---

## Scene 3: AI Agent Call (1:00 - 1:45)

**Mobile:** Dial 1000, put on speaker

**Narration:**
> "Dial 1000 from the mobile. FreeSWITCH answers, forks the audio to the Go gateway, Whisper transcribes, Claude generates a response, Piper synthesizes speech, and the audio plays back through your phone speaker. The full pipeline in under 3 seconds."

---

## Scene 4: Co-Pilot Bridge (1:45 - 2:30)

**Terminal:** Start baresip as agent1
**Browser:** Open http://192.168.x.x:3000/calls/live
**Mobile:** Dial 2001

**Narration:**
> "For co-pilot testing, register baresip on your laptop as agent1. Dial 2001 from the mobile — Kamailio routes to FreeSWITCH which bridges the customer to the agent and forks audio to the copilot pipeline. The dashboard auto-discovers the session, shows the caller number, live transcript, and coaching suggestions."

---

## Scene 5: All Scenarios (2:30 - 3:00)

**Terminal:**
```bash
make sbc-test
```

**Narration:**
> "Four dial patterns. 1000 for AI agent, 2001 for co-pilot bridge with a human agent, 2100 for co-pilot auto-answer without an agent, and 3000 for the hold queue. The automated test suite validates SIP OPTIONS, registration, INVITE routing, and trunk API."
