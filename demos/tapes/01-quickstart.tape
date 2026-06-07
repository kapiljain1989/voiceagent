Output demos/gifs/quickstart.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set Padding 20

Type "# VoiceAgent — Quick Start (10 services in one command)"
Enter
Sleep 1s

Type "export EXT_IP=$(ipconfig getifaddr en0)"
Enter
Sleep 500ms

Type "docker compose -f docker-compose.sip.yml up -d"
Enter
Sleep 6s

Type "# Verify all services"
Enter
Sleep 500ms

Type "docker compose -f docker-compose.sip.yml ps --format 'table {{.Name}}\t{{.Status}}'"
Enter
Sleep 3s

Type "# Health check"
Enter
Sleep 500ms

Type "curl -s http://localhost:8080/healthz | python3 -m json.tool"
Enter
Sleep 2s

Type "# Login"
Enter
Sleep 500ms

Type@50ms 'TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"admin\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)[\"token\"])")'
Enter
Sleep 1s

Type "echo 'Authenticated ✓'"
Enter
Sleep 2s

Type "# Dashboard: http://localhost:3000"
Enter
Sleep 2s
