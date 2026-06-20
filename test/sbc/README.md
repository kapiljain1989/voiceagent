# Test SBC — External SBC Simulator

**This is NOT part of VoiceAgent.** It simulates an enterprise PBX/SBC for testing.

Softphones register with this SBC. Calls are routed to the VoiceAgent gateway.

## Quick Start

```bash
# Start (gateway on same machine)
./test/sbc/start-sbc.sh

# Start (gateway on another machine)
./test/sbc/start-sbc.sh 192.168.1.100

# Stop
./test/sbc/start-sbc.sh stop
```

## Softphone Setup

| Setting   | Value                          |
|-----------|--------------------------------|
| Server    | localhost (or this machine's IP)|
| Port      | 5060                           |
| Username  | caller1 (any name)             |
| Password  | (empty — open registration)    |
| Transport | UDP                            |

Dial any number (e.g., `2001`) → call routes to VoiceAgent gateway.

## Without a Softphone

```bash
# Send a test call directly to the gateway (bypasses SBC)
python3 test/sip_test_call.py 127.0.0.1:5062 60
```

## Architecture

```
Softphone → Test SBC (:5060) → VoiceAgent Gateway (:5062)
                                     ↓
                              Console Queue → Agent picks → WebRTC
```

## For Two-Machine Setup

```
Machine A (VoiceAgent)           Machine B (Test SBC + Softphone)
┌───────────────────┐            ┌───────────────────┐
│ KinD Cluster      │            │ Test SBC (:5060)  │
│   Gateway         │  SIP/RTP  │                   │
│   Whisper, DB     │ ◄──────── │ Softphone (Zoiper)│
│   Console (:3000) │            │                   │
└───────────────────┘            └───────────────────┘

# On Machine A: run gateway natively for SIP
go run ./gateway LISTEN_ADDR=:8080 SIP_LISTEN_ADDR=:5062 ...

# On Machine B:
GATEWAY_HOST=<machine-a-ip> ./test/sbc/start-sbc.sh
```
