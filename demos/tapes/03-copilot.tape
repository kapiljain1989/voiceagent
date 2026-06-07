Output demos/gifs/copilot.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Catppuccin Mocha"
Set TypingSpeed 50ms
Set Padding 20

Type "# Co-Pilot Agent Assist — SIPREC dual-leg observation"
Enter
Sleep 1s

Type "# Two-party call: caller + agent legs via WebSocket"
Enter
Sleep 500ms

Type "cd test && timeout 25 go run simcopilot.go localhost:8080 2>&1; cd .."
Enter
Sleep 26s

Type "# Live transcript + RAG coaching + post-call summary"
Enter
Sleep 2s
