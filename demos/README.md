# VoiceAgent Demo Videos

## Quick GIFs (GitHub/README)

Automated terminal recordings via [VHS](https://github.com/charmbracelet/vhs). Each `.tape` file scripts a reproducible terminal session.

| # | Demo | Tape | Duration |
|---|------|------|----------|
| 1 | Quick Start — deploy 10 services | [01-quickstart.tape](tapes/01-quickstart.tape) | ~25s |
| 2 | AI Voice Call — STT→Claude→TTS | [02-voice-call.tape](tapes/02-voice-call.tape) | ~25s |
| 3 | Co-Pilot Agent Assist — SIPREC | [03-copilot.tape](tapes/03-copilot.tape) | ~25s |
| 4 | Robocall Detection — 3 layers | [04-robocall.tape](tapes/04-robocall.tape) | ~25s |
| 5 | PII Masking — 7 patterns | [05-pii-masking.tape](tapes/05-pii-masking.tape) | ~25s |
| 6 | RAG Knowledge Base — upload + search | [06-rag.tape](tapes/06-rag.tape) | ~20s |
| 7 | K8s Deploy — Istio + Gateway API | [07-k8s-deploy.tape](tapes/07-k8s-deploy.tape) | ~20s |
| 8 | API & SDK — Python SDK demo | [08-api-sdk.tape](tapes/08-api-sdk.tape) | ~25s |

### Generate GIFs

```bash
brew install vhs ffmpeg
make demos
```

## Narrated Video Scripts (YouTube/LinkedIn)

Step-by-step recording guides with terminal commands, browser actions, and narration text.

| # | Demo | Script | Duration |
|---|------|--------|----------|
| 9 | Platform Overview | [09-platform-overview.md](scripts/09-platform-overview.md) | ~3 min |
| 10 | Co-Pilot Live Session | [10-copilot-live.md](scripts/10-copilot-live.md) | ~3 min |
| 11 | Security Suite | [11-security-suite.md](scripts/11-security-suite.md) | ~2.5 min |
| 12 | Infrastructure | [12-infrastructure.md](scripts/12-infrastructure.md) | ~2 min |
| 13 | K8s + Istio | [13-k8s-istio.md](scripts/13-k8s-istio.md) | ~3 min |
| 14 | SBC Integration | [14-sbc-integration.md](scripts/14-sbc-integration.md) | ~2.5 min |

### Recording Tips

1. Start Docker Compose services before recording
2. Run `make demos` for terminal GIFs (automated)
3. For narrated videos, follow each script scene-by-scene with OBS/QuickTime
4. Use split-screen: terminal on left, browser on right
5. Resolution: 1920x1080 minimum
