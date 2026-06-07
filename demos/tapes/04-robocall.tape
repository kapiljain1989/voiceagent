Output demos/gifs/robocall.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set Padding 20

Type "# Robocall Detection — 3-layer defense"
Enter
Sleep 1s

Type "# Layer 3: Keyword detection"
Enter
Sleep 500ms

Type@30ms 'curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/robocall/test -d "{\"text\":\"Press 1 for your auto warranty\"}" | python3 -m json.tool'
Enter
Sleep 3s

Type "# Add to blocklist (Layer 1)"
Enter
Sleep 500ms

Type@30ms 'curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/blocklist -d "{\"number\":\"+15550000001\",\"reason\":\"spam caller\"}" | python3 -m json.tool'
Enter
Sleep 2s

Type "# Verify blocklist"
Enter
Sleep 500ms

Type@30ms 'curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/blocklist | python3 -m json.tool'
Enter
Sleep 3s

Type "# Remove from blocklist"
Enter
Sleep 500ms

Type@30ms 'curl -s -H "Authorization: Bearer $TOKEN" -X DELETE http://localhost:8080/api/blocklist -d "{\"number\":\"+15550000001\"}"'
Enter
Sleep 2s

Type "# 3 layers: blocklist (<1ms) → audio pattern → 28 keyword phrases"
Enter
Sleep 2s
