# Demo 15: Standalone Helper Mode

**Duration:** ~2.5 minutes
**Format:** Screen recording (terminal + browser)
**Audience:** Contact center architects, IT operations

---

## Scene 1: Deploy (0:00 - 0:30)

**Terminal:**
```bash
docker compose -f docker-compose.helper.yml up -d
docker compose -f docker-compose.helper.yml ps
curl -s http://localhost:8080/api/config | python3 -m json.tool
```

**Narration:**
> "Standalone helper mode runs 8 services — no FreeSWITCH, no Piper. The gateway has a native SIP endpoint built in Go that accepts SIPREC INVITEs directly. Your SBC owns the call; VoiceAgent just observes."

---

## Scene 2: SBC Configuration (0:30 - 1:00)

**Terminal:**
```bash
# SIP endpoint is live
python3 -c "
import socket, time
msg='OPTIONS sip:test@127.0.0.1:5061 SIP/2.0\r\n...'
s=socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.connect(('127.0.0.1', 5061)); s.send(msg.encode()); time.sleep(1)
print(s.recv(4096).decode().split('\r\n')[0]); s.close()
"
```

**Narration:**
> "The SIP server responds to OPTIONS. On your SBC, you add one line — point the SIPREC recording server to this IP. Cisco CUBE: media-recording IP port 5060. AudioCodes: Recording Server equals IP colon 5060. That's the entire integration."

---

## Scene 3: Live Features (1:00 - 2:00)

**Terminal:**
```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login ...)

# All features work without FreeSWITCH
curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/robocall/test \
  -d '{"text":"Press 1 for your auto warranty"}'

curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/security/pii/test \
  -d '{"text":"My SSN is 123-45-6789 dob 16121968"}'

curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/copilot/active
```

**Browser:** Show dashboard at http://localhost:3000

**Narration:**
> "Every feature works in standalone mode — robocall detection, PII masking with 9 patterns including spoken digits and compact dates, voice sentiment analysis, RAG knowledge base, Prometheus metrics. The only things missing are TTS response and SIP REFER transfer — because the helper doesn't speak or control calls."

---

## Scene 4: VOICEAGENT_MODE (2:00 - 2:30)

**Terminal:**
```bash
curl -s http://localhost:8080/healthz
# → {"status":"ok","sessions":0,"mode":"standalone"}
```

**Narration:**
> "One environment variable controls the mode. VOICEAGENT_MODE equals standalone for the helper, or gateway for the full B2BUA with FreeSWITCH. Same binary, same image, same config — just flip the mode."
