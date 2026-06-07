# Demo 14: SBC Integration

**Duration:** ~2.5 minutes
**Format:** Screen recording (terminal + browser)
**Audience:** VoIP engineers, telecom architects

---

## Scene 1: Trunk Management API (0:00 - 0:45)

**Terminal:**
```bash
# Add a Cisco CUBE trunk
curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/trunks \
  -d '{"name":"Cisco CUBE DC1","address":"cube-dc1.internal","port":5060,"transport":"tcp","register":false,"codecs":"PCMU,PCMA,G729"}' | python3 -m json.tool

# Add a Twilio trunk
curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/trunks \
  -d '{"name":"Twilio Prod","address":"trunk.pstn.twilio.com","register":true,"username":"ACsid","password":"token","caller_id":"+15559876543"}' | python3 -m json.tool

# List trunks (passwords never returned)
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/trunks | python3 -m json.tool
```

**Narration:**
> "SIP trunks are managed via a secure REST API. Auto-detection identifies the provider — Twilio, Cisco CUBE, AudioCodes, or custom. Credentials are stored in PostgreSQL but never returned in GET responses. The Apply endpoint pushes configuration to FreeSWITCH via ESL without restart."

---

## Scene 2: SBC Configuration (0:45 - 1:30)

**Browser:** http://localhost:3000/settings — SBC tab

**Terminal:**
```bash
# Pre-configured enterprise SBC profiles
ls freeswitch/config/sip_profiles/enterprise/

# Configure via Makefile
SBC_ADDRESS=cube-dc1.internal make sbc-config
```

**Narration:**
> "Pre-configured profiles for Cisco CUBE and AudioCodes Mediant handle the vendor-specific quirks — codec negotiation, session timers, NAT traversal, TLS. The Makefile target injects the SBC address into FreeSWITCH vars.xml and restarts the SIP profile. For complex setups, the full SBC configuration guide covers Cisco, AudioCodes, Oracle SBC, Kamailio, and Twilio."

---

## Scene 3: Dialplan Routing (1:30 - 2:00)

**Terminal:**
```bash
# Show dialplan routing
cat freeswitch/config/dialplan/public.xml | grep -E "condition|application" | head -12
```

**Narration:**
> "Dialplan routing is convention-based. 1xxx goes to interactive AI agent — the AI answers the call. 2xxx goes to co-pilot mode — passive SIPREC observation with RAG coaching. 3xxx goes to the human agent queue — the failover target. When all circuits open, the gateway issues a SIP REFER to 3000 with X-Failover headers."

---

## Scene 4: SIP Call Flow (2:00 - 2:30)

**Terminal:**
```bash
# SIP OPTIONS probe
python3 -c "
import socket
msg='OPTIONS sip:test@127.0.0.1:5070 SIP/2.0\r\nVia: SIP/2.0/TCP 127.0.0.1:5099;branch=z9hG4bK-test\r\nMax-Forwards: 70\r\nFrom: <sip:t@127.0.0.1>;tag=t\r\nTo: <sip:t@127.0.0.1>\r\nCall-ID: test\r\nCSeq: 1 OPTIONS\r\nContent-Length: 0\r\n\r\n'
s=socket.socket(socket.AF_INET,socket.SOCK_STREAM); s.settimeout(5)
s.connect(('127.0.0.1',5070)); s.send(msg.encode())
import time; time.sleep(1)
print(s.recv(4096).decode().split('\r\n')[0]); s.close()
"

# FreeSWITCH sofia status
docker exec voiceagent-freeswitch-1 fs_cli -P 8022 -x "sofia status" 2>/dev/null | grep RUNNING
```

**Narration:**
> "FreeSWITCH confirms SIP/2.0 200 OK on the OPTIONS probe. The external SIP profile is running with G.711 codec preference, RFC 2833 DTMF, and session timers enabled. From here, any enterprise SBC can trunk into VoiceAgent and route calls to the AI pipeline."
