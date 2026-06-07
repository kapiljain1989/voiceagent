Output demos/gifs/voice-call.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Catppuccin Mocha"
Set TypingSpeed 50ms
Set Padding 20

Type "# Interactive AI Voice Call — STT -> Claude -> TTS"
Enter
Sleep 1s

Type "# Simulate FreeSWITCH audio_fork WebSocket"
Enter
Sleep 500ms

Type "cd test && timeout 15 go run simcall.go ws://localhost:8080/ws 2>&1; cd .."
Enter
Sleep 16s

Type "# Gateway pipeline: Audio -> Whisper STT -> Claude -> Piper TTS -> Audio"
Enter
Sleep 2s
