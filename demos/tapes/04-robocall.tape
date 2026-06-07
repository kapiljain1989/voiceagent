Output demos/gifs/robocall.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Catppuccin Mocha"
Set TypingSpeed 50ms
Set Padding 20

Type "# Robocall Detection — 3-layer defense"
Enter
Sleep 1s

Type "# Layer 3: Keyword detection"
Enter
Sleep 500ms

Type "demos/run-demo.sh robocall-test 'Press 1 for your auto warranty'"
Enter
Sleep 3s

Type "# Add to blocklist (Layer 1: <1ms lookup)"
Enter
Sleep 500ms

Type "demos/run-demo.sh blocklist-add '+15550000001' 'spam caller'"
Enter
Sleep 2s

Type "# View blocklist"
Enter
Sleep 500ms

Type "demos/run-demo.sh blocklist-list"
Enter
Sleep 3s

Type "demos/run-demo.sh blocklist-remove '+15550000001'"
Enter
Sleep 2s
