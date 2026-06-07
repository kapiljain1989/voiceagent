Output demos/gifs/copilot.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set Padding 20

Type "# Co-Pilot Agent Assist — SIPREC dual-leg observation"
Enter
Sleep 1s

Type "# Simulate a two-party call with co-pilot coaching"
Enter
Sleep 500ms

Type "cd test && timeout 20 go run simcopilot.go http://localhost:8080"
Enter
Sleep 15s

Type "# Check co-pilot output"
Enter
Sleep 500ms

Type "docker logs voiceagent-gateway-1 --tail=20 2>&1 | grep -E 'suggestion|coaching|summary|copilot'"
Enter
Sleep 3s

Type "# Co-pilot: live transcript + RAG coaching + post-call summary"
Enter
Sleep 2s
