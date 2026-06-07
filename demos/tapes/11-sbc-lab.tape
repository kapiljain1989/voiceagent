Output demos/gifs/sbc-lab.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Catppuccin Mocha"
Set TypingSpeed 50ms
Set Padding 20

Type "# SBC Lab — Kamailio + mobile softphone testing"
Enter
Sleep 1s

Type "demos/run-demo.sh sbc-lab-services"
Enter
Sleep 3s

Type "# SIP OPTIONS to Kamailio SBC"
Enter
Sleep 500ms

Type "demos/run-demo.sh sip-options 5080"
Enter
Sleep 3s

Type "# Dial 1000 (AI agent) or 2001 (co-pilot bridge) from mobile"
Enter
Sleep 2s
