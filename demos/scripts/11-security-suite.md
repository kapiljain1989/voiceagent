# Demo 11: Security Suite

**Duration:** ~2.5 minutes
**Format:** Screen recording (terminal + browser)
**Audience:** Security officers, compliance teams, enterprise architects

---

## Scene 1: Robocall Detection (0:00 - 0:45)

**Terminal:**
```bash
# Test keyword detection (Layer 3)
curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/robocall/test \
  -d '{"text":"Press 1 for your auto warranty"}' | python3 -m json.tool

# Test clean speech
curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/robocall/test \
  -d '{"text":"Hi, I am calling about my insurance claim"}' | python3 -m json.tool

# Blocklist management
curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/blocklist \
  -d '{"number":"+15550000001","reason":"spam"}'

curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/blocklist | python3 -m json.tool
```

**Narration:**
> "Three detection layers run in sequence. Layer 1 is an in-memory blocklist — sub-millisecond hash lookup. Layer 2 analyzes the first 2 seconds of audio for robocall patterns: monotone RMS, low variance, high energy consistency. Layer 3 matches against 28 known robocall phrases. Scores above 0.7 get flagged. High-confidence detections auto-block via ESL hangup."

---

## Scene 2: PII Masking (0:45 - 1:30)

**Terminal:**
```bash
# Credit card
curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/security/pii/test \
  -d '{"text":"My card is 4111 1111 1111 1111"}' | python3 -m json.tool

# SSN
curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/security/pii/test \
  -d '{"text":"SSN 123-45-6789"}' | python3 -m json.tool

# Multiple PII types
curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/security/pii/test \
  -d '{"text":"DOB 03/15/1990 CVV 123 account 98765432"}' | python3 -m json.tool
```

**Narration:**
> "Seven regex patterns catch credit cards, SSNs, CVVs, dates of birth, and account numbers. PII gets masked before reaching the LLM or call recording. The original audio frames containing PII are silenced. This runs in the STT pipeline — before transcription hits Claude."

---

## Scene 3: Voice Biometrics (1:30 - 2:00)

**Browser:** http://localhost:3000/security — Voice Biometrics tab

**Terminal:**
```bash
# Enroll a voiceprint
curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/security/voiceprints \
  -d '{"caller_id":"+15551234567","name":"John Smith"}' | python3 -m json.tool

# List enrolled voiceprints
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/security/voiceprints | python3 -m json.tool
```

**Narration:**
> "Voice biometrics compute a 32-dimensional spectral fingerprint from the caller's audio. Cosine similarity matching detects fraud — if the voice doesn't match the registered caller, it flags the call. Enrollment happens automatically after the first verified call."

---

## Closing (2:00 - 2:30)

**Browser:** Show Security page with all three tabs

**Narration:**
> "Robocall detection, PII masking, and voice biometrics — all running locally, sub-millisecond per check. No customer data leaves your network. PCI and HIPAA compliance built into the audio pipeline."
