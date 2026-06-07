Output demos/gifs/voice-call.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set Padding 20

Type "# Interactive AI Voice Call — STT → Claude → TTS pipeline"
Enter
Sleep 1s

Type "# Simulate a FreeSWITCH audio_fork WebSocket connection"
Enter
Sleep 500ms

Type "cd test && timeout 20 go run simcall.go ws://localhost:8080/ws"
Enter
Sleep 15s

Type "# Check gateway logs for the call flow"
Enter
Sleep 500ms

Type "docker logs voiceagent-gateway-1 --tail=15 2>&1 | grep -E 'heard|replied|session|audio'"
Enter
Sleep 3s

Type "# Call completed: Audio → Whisper STT → Claude → Piper TTS → Audio"
Enter
Sleep 2s
