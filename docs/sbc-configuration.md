# SBC Configuration Guide

Complete SBC-side configuration for integrating with the VoiceAgent Telecom-Native AI Gateway. Covers Cisco CUBE, AudioCodes Mediant, Oracle SBC, Kamailio, and Twilio.

---

## Table of Contents

1. [General Requirements](#1-general-requirements)
2. [Cisco CUBE](#2-cisco-cube)
3. [AudioCodes Mediant](#3-audiocodes-mediant)
4. [Oracle (Acme Packet) SBC](#4-oracle-acme-packet-sbc)
5. [Kamailio / OpenSIPs](#5-kamailio--opensips)
6. [Twilio Elastic SIP Trunking](#6-twilio-elastic-sip-trunking)
7. [SIPREC Configuration (Co-Pilot)](#7-siprec-configuration-co-pilot)
8. [Firewall & Network Requirements](#8-firewall--network-requirements)
9. [Common Integration Issues](#9-common-integration-issues)

---

## 1. General Requirements

Every SBC needs a SIP trunk pointing to the VoiceAgent FreeSWITCH instance:

| Parameter | Value | Notes |
|-----------|-------|-------|
| **Remote IP** | VoiceAgent host IP | `EXT_IP` from your deployment |
| **Remote Port** | 5060 (KinD) or 5070 (Docker Compose) | Match your deployment mode |
| **Transport** | UDP or TCP | TCP recommended for reliability |
| **Codec Profile** | PCMU, PCMA (G.711) | Gateway transcodes to L16 internally via LUT |
| **DTMF Mode** | RFC 2833 | Gateway intercepts RTP event packets natively |
| **Session Timers** | Enabled (RFC 4028) | 1800s timeout, 120s minimum |
| **Registration** | Optional | IP-based peering or SIP REGISTER |
| **VAD / Silence Suppression** | Disabled | VoiceAgent runs its own telecom AGC pipeline |
| **SIP REFER** | Enabled | Required for intelligent call transfers |

### VoiceAgent-Side Trunk Setup

```bash
# Via API (secure — requires auth token)
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -d '{"username":"admin","password":"admin"}' | jq -r .token)

curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/trunks \
  -d '{"name":"My SBC","address":"sbc.example.com","register":false}'

# Via Makefile
SBC_ADDRESS=sbc.example.com make sbc-config

# Via config file
# Edit freeswitch/config/sip_profiles/external.xml
```

### Dialplan Routing Convention

| Destination Pattern | Mode | Behavior |
|---------------------|------|----------|
| `1xxx` | Interactive AI Agent | AI answers the call, self-service + transfer |
| `2xxx` | Co-Pilot Agent Assist | Passive observation, RAG coaching, post-call summary |
| `3xxx` | Human Queue | Direct-to-agent (failover target) |

---

## 2. Cisco CUBE

### IOS Configuration

```ios
! ========================================
! SIP Trunk to VoiceAgent AI Gateway
! ========================================

voice service voip
 ip address trusted list
  ipv4 <VOICEAGENT_IP>
 allow-connections sip to sip
 sip
  session transport udp

! ========================================
! Codec Preference — G.711 μ-law first
! (Lowest transcoding latency in VoiceAgent)
! ========================================
voice class codec 1
 codec preference 1 g711ulaw
 codec preference 2 g711alaw
 codec preference 3 g722-64

! ========================================
! Dial-peer: PSTN → VoiceAgent AI Agent
! Pattern: 1xxx → Interactive mode
! ========================================
dial-peer voice 100 voip
 description "To VoiceAgent AI Agent"
 destination-pattern 1...
 session target ipv4:<VOICEAGENT_IP>:<PORT>
 session protocol sipv2
 voice-class codec 1
 dtmf-relay rfc2833
 no vad

! ========================================
! Dial-peer: PSTN → VoiceAgent Co-Pilot
! Pattern: 2xxx → Co-Pilot mode
! ========================================
dial-peer voice 200 voip
 description "To VoiceAgent Co-Pilot"
 destination-pattern 2...
 session target ipv4:<VOICEAGENT_IP>:<PORT>
 session protocol sipv2
 voice-class codec 1
 dtmf-relay rfc2833
 no vad

! ========================================
! Inbound dial-peer from PSTN
! ========================================
dial-peer voice 10 pots
 description "From PSTN"
 incoming called-number .
 direct-inward-dial
 forward-digits all

! ========================================
! SIP REFER support
! (Required for VoiceAgent intelligent transfers)
! ========================================
sip-ua
 handle-replaces

! ========================================
! SIPREC for Co-Pilot recording (optional)
! ========================================
voice class media-profile 1
 recording-profile 1
  media-recording <VOICEAGENT_IP> port 5060
```

### Key CUBE Settings

| Setting | Value | Why |
|---------|-------|-----|
| `dtmf-relay rfc2833` | Required | VoiceAgent parses RFC 2833 RTP event packets |
| `no vad` | Required | CUBE's VAD conflicts with VoiceAgent's telecom AGC |
| `handle-replaces` | Required | Enables SIP REFER for call transfers |
| Codec order: G.711 μ-law first | Recommended | Fastest transcoding path (< 1ms LUT decode) |

---

## 3. AudioCodes Mediant

### Configuration

```
; ========================================
; IP Group — VoiceAgent AI Gateway
; ========================================
[IP Group - VoiceAgent]
  Name = VoiceAgent-AI
  Proxy Address = <VOICEAGENT_IP>
  Proxy Port = 5060
  Transport Type = UDP
  Codec List = G.711U-law, G.711A-law, G.722
  DTMF Transport Type = RFC 2833
  SBC Operation Mode = B2BUA
  Enable Early Media = Enable
  Session Timer = Enable
  Session Timer Interval = 1800
  Minimum Session Timer = 120
  SIP REFER Support = Enable

; ========================================
; IP Group — PSTN Carrier
; ========================================
[IP Group - PSTN]
  Name = PSTN-Carrier
  Proxy Address = <CARRIER_IP>
  Transport Type = UDP

; ========================================
; Routing Rules
; ========================================
[IP-to-IP Routing]
  Rule 1:
    Source IP Group = PSTN
    Destination IP Group = VoiceAgent-AI
    Destination Pattern = 1*           ; 1xxx → AI Agent

  Rule 2:
    Source IP Group = PSTN
    Destination IP Group = VoiceAgent-AI
    Destination Pattern = 2*           ; 2xxx → Co-Pilot

  Rule 3:
    Source IP Group = VoiceAgent-AI
    Destination IP Group = PSTN        ; Return path for outbound calls

; ========================================
; Media Settings
; ========================================
[Media]
  DTMF Transport Type = RFC 2833
  Silence Suppression = Disable        ; VoiceAgent handles its own AGC
  Echo Cancellation = Enable
  Jitter Buffer = Adaptive
  RTP Redundancy Depth = 0

; ========================================
; SIPREC Recording (for Co-Pilot)
; ========================================
[SIP Recording]
  Recording Server = <VOICEAGENT_IP>:5060
  Recording Mode = Selective           ; Only 2xxx calls
  Recording Pattern = 2*
```

---

## 4. Oracle (Acme Packet) SBC

```
; ========================================
; Session Agent — VoiceAgent Gateway
; ========================================
session-router
  session-agent
    hostname            voiceagent-gw
    ip-address          <VOICEAGENT_IP>
    port                5060
    transport-method    UDP
    realm-id            VoiceAgent

; ========================================
; Routing Policies
; ========================================
  local-policy
    from-address        *
    to-address          1*              ; 1xxx → AI Agent
    next-hop            voiceagent-gw

  local-policy
    from-address        *
    to-address          2*              ; 2xxx → Co-Pilot
    next-hop            voiceagent-gw

; ========================================
; Codec Policy
; ========================================
media-manager
  codec-policy
    name                voiceagent-codecs
    codec               PCMU
    codec               PCMA
    codec               G722
    dtmf-in-band        rfc2833

; ========================================
; SIPREC Recording
; ========================================
session-recording
  name                  voiceagent-siprec
  destination           sip:<VOICEAGENT_IP>:5060
  mode                  selective
  pattern               2*
```

---

## 5. Kamailio / OpenSIPs

### Kamailio Configuration

```kamailio
# ========================================
# Load modules
# ========================================
loadmodule "dispatcher.so"
loadmodule "siprec.so"

# ========================================
# Route to VoiceAgent
# ========================================
route[VOICEAGENT] {
    # 1xxx → Interactive AI Agent
    if ($rU =~ "^1[0-9]{3}$") {
        $du = "sip:<VOICEAGENT_IP>:5060;transport=udp";
        route(RELAY);
        exit;
    }

    # 2xxx → Co-Pilot mode
    if ($rU =~ "^2[0-9]{3}$") {
        $du = "sip:<VOICEAGENT_IP>:5060;transport=udp";
        # Start SIPREC for co-pilot calls
        siprec_start_recording("sip:<VOICEAGENT_IP>:5060");
        route(RELAY);
        exit;
    }
}

# ========================================
# Load balancing (multiple VoiceAgent replicas)
# ========================================
modparam("dispatcher", "list_file", "/etc/kamailio/dispatcher.list")

# /etc/kamailio/dispatcher.list:
# 1 sip:<VOICEAGENT_IP_1>:5060
# 1 sip:<VOICEAGENT_IP_2>:5060
# 1 sip:<VOICEAGENT_IP_3>:5060

route[VOICEAGENT_LB] {
    if (!ds_select_dst("1", "4")) {  # Round-robin
        send_reply("503", "Service Unavailable");
        exit;
    }
    route(RELAY);
}
```

---

## 6. Twilio Elastic SIP Trunking

### Twilio Console Setup

**Step 1: Create a Trunk**
- Navigate to: Console → Elastic SIP Trunking → Trunks → Create
- Name: `VoiceAgent`

**Step 2: Origination (Twilio → VoiceAgent)**
- Add Origination URI:
  ```
  sip:<YOUR_PUBLIC_IP>:5070;transport=tcp
  ```
- Priority: 10
- Weight: 100

**Step 3: Termination (VoiceAgent → Twilio → PSTN)**
- SIP URI: `voiceagent.pstn.twilio.com`
- Authentication:
  - IP ACL: Add your VoiceAgent public IP
  - Credential List: Create username/password

**Step 4: Assign Phone Number**
- Numbers → Buy or port a number → Assign to trunk

**Step 5: VoiceAgent Configuration**
```bash
# Via API
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/trunks \
  -d '{
    "name": "Twilio Production",
    "address": "your-trunk.pstn.twilio.com",
    "register": true,
    "username": "your-trunk-sid",
    "password": "your-auth-token",
    "caller_id": "+15559876543"
  }'

# Via Makefile
SBC_ADDRESS=your-trunk.pstn.twilio.com \
SBC_REGISTER=true \
SBC_USERNAME=your-trunk-sid \
SBC_PASSWORD=your-auth-token \
make sbc-config
```

---

## 7. SIPREC Configuration (Co-Pilot)

SIPREC (RFC 7866) enables the SBC to fork a copy of the live call audio to VoiceAgent for passive observation. The SBC sends:

- A SIP INVITE with XML metadata describing the recording session
- Two separate RTP streams (caller leg + agent leg)

VoiceAgent's `siprec_meta.go` parses the RFC 7866 XML metadata automatically and labels each stream as `caller` or `agent` for diarized transcription.

### SIPREC Setup Per SBC

| SBC | Configuration |
|-----|--------------|
| **Cisco CUBE** | `media-recording <IP> port 5060` under `voice class media-profile` |
| **AudioCodes** | SIP Recording → Recording Server = `<IP>:5060`, Mode = Selective |
| **Oracle** | Session Recording Server → destination = `sip:<IP>:5060` |
| **Kamailio** | `siprec_start_recording("sip:<IP>:5060")` in route block |
| **Twilio** | Not supported natively (use Twilio Media Streams instead) |

### SIPREC Metadata (RFC 7866)

The SBC sends XML like this in the SIPREC INVITE body:

```xml
<recording>
  <session session-id="abc123" start-time="2026-06-07T14:00:00Z"/>
  <participant participant-id="p1">
    <nameID><aor>sip:customer@carrier.com</aor></nameID>
  </participant>
  <participant participant-id="p2">
    <nameID><aor>sip:agent@enterprise.com</aor></nameID>
  </participant>
  <stream stream-id="s1" participant-id="p1" label="caller"/>
  <stream stream-id="s2" participant-id="p2" label="agent"/>
</recording>
```

VoiceAgent parses this to determine which RTP stream belongs to the customer and which belongs to the agent.

---

## 8. Firewall & Network Requirements

### Inbound (SBC → VoiceAgent)

| Port | Protocol | Purpose |
|------|----------|---------|
| 5060 or 5070 | TCP + UDP | SIP signaling |
| 16000-16020 (KinD) or 20000-20020 (Compose) | UDP | RTP media |

### Outbound (VoiceAgent → SBC)

| Port | Protocol | Purpose |
|------|----------|---------|
| 5060 | TCP + UDP | SIP responses, outbound INVITE, SIP REFER |
| Dynamic (ephemeral) | UDP | RTP return path |

### TLS (Optional)

For encrypted SIP signaling:

| Port | Protocol | Purpose |
|------|----------|---------|
| 5061 | TLS | Encrypted SIP (requires certificate on both sides) |

Enable in the SBC profile:
```xml
<param name="tls" value="true"/>
<param name="tls-sip-port" value="5061"/>
```

---

## 9. Common Integration Issues

| Symptom | Cause | Fix |
|---------|-------|-----|
| **No audio (one-way or both)** | RTP IP mismatch — SDP advertises wrong IP | Set `EXT_IP` to your public/LAN IP. Check SBC NAT settings. |
| **DTMF not detected** | SBC using inband DTMF instead of RFC 2833 | Set `dtmf-relay rfc2833` on the SBC dial-peer |
| **AI constantly interrupts** | SBC's VAD/silence suppression enabled | Disable VAD on the SBC — VoiceAgent runs its own AGC |
| **Calls drop after 30 minutes** | No session timers | Enable RFC 4028 session timers on both sides |
| **Transfer fails (SIP REFER)** | SBC doesn't support REFER | Enable `handle-replaces` (CUBE) or SIP REFER support |
| **SIPREC not working** | Wrong recording server IP/port | Verify SIPREC destination matches VoiceAgent's SIP port |
| **403 Forbidden on inbound** | IP not in VoiceAgent ACL | Add SBC IP to `freeswitch/config/autoload_configs/acl.conf.xml` |
| **Codec negotiation fails** | G.729 offered but not supported | Ensure G.711 (PCMU/PCMA) is in the codec list |
| **Echo on calls** | No echo cancellation on SBC | Enable echo cancellation on the SBC side |
| **High latency** | Cloud transcoding in the path | VoiceAgent transcodes G.711 locally (< 1ms) — ensure no middlebox transcoding |
| **Registration fails** | Wrong credentials or realm | Verify username/password match on both sides. Check SIP realm. |
