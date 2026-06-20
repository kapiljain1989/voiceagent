# Demo 16: Voice Sentiment Analysis

**Duration:** ~2 minutes
**Format:** Screen recording (terminal + browser)
**Audience:** CX leaders, quality assurance, workforce management

---

## Scene 1: What It Measures (0:00 - 0:30)

**Browser:** Show Live Ops page during an active co-pilot call

**Narration:**
> "Voice sentiment analyzes the raw audio signal — not the words, but how they're said. Pitch, volume, speaking rate, silence patterns. An angry caller sounds different even if they say polite words. This runs on every audio frame with zero latency — pure signal processing, no ML model."

---

## Scene 2: Live Indicators (0:30 - 1:00)

**Browser:** Point out the active session list with sentiment badges

**Narration:**
> "Each active call shows real-time agitation and frustration percentages. Green means calm, amber means agitated, red means frustrated. The mood badge in the header updates every 3 seconds — CALM, ENGAGED, AGITATED, or FRUSTRATED. The agent sees this while talking to the customer."

---

## Scene 3: Post-Call Analysis (1:00 - 1:30)

**Browser:** Show a completed call summary with the voice sentiment panel

**Narration:**
> "When the call ends, the summary includes full acoustic analysis. Three gauges — agitation, frustration, engagement — each as a percentage. Below that: average pitch in hertz, energy trend — rising means the caller was getting louder — speaking rate in words per minute, and silence ratio."

---

## Scene 4: Multi-Modal (1:30 - 2:00)

**Terminal:**
```bash
docker logs voiceagent-gateway-1 --tail=5 2>&1 | grep "text_sentiment.*voice_sentiment"
```

**Narration:**
> "The real power is multi-modal sentiment. If Claude analyzes the transcript as neutral but the voice shows frustration above 60 percent, the system overrides to negative. This catches sarcasm — 'Oh sure, that's just great' sounds neutral in text but the voice tells the truth."
