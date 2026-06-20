Output demos/gifs/standalone-helper.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Catppuccin Mocha"
Set TypingSpeed 50ms
Set Padding 20

Type "# Standalone Helper — No FreeSWITCH, plug-and-play SIPREC"
Enter
Sleep 1s

Type "demos/run-demo.sh config"
Enter
Sleep 3s

Type "demos/run-demo.sh helper-services"
Enter
Sleep 3s

Type "# SIP endpoint accepts SIPREC INVITEs"
Enter
Sleep 500ms

Type "demos/run-demo.sh sip-options"
Enter
Sleep 3s

Type "# SBC config: point recording server to this IP:5060"
Enter
Sleep 2s
