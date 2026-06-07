Output demos/gifs/api-sdk.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set Padding 20

Type "# Python SDK — 25+ API endpoints"
Enter
Sleep 1s

Type 'python3 -c "'
Enter

Type "from voiceagent import VoiceAgentClient"
Enter
Type "client = VoiceAgentClient('http://localhost:8080')"
Enter
Type "client.login('admin', 'admin')"
Enter
Sleep 500ms

Type "# Health check"
Enter
Type "print('Health:', client.health())"
Enter
Sleep 1s

Type "# PII masking"
Enter
Type "pii = client.test_pii('My SSN is 123-45-6789')"
Enter
Type "print('Masked:', pii['masked'])"
Enter
Sleep 1s

Type "# Robocall detection"
Enter
Type "robo = client.test_robocall('Press 1 for warranty')"
Enter
Type "print('Score:', robo['keyword']['score'])"
Enter
Sleep 1s

Type "# RAG search"
Enter
Type "results = client.rag_search('water damage')"
Enter
Type "print('Results:', len(results))"
Enter

Type '"'
Enter
Sleep 4s

Type "# Python + TypeScript SDKs available"
Enter
Sleep 2s
