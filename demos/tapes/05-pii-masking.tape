Output demos/gifs/pii-masking.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set Padding 20

Type "# PII Masking — 7 patterns, masks before LLM/recording"
Enter
Sleep 1s

Type "# Credit card number"
Enter
Sleep 500ms

Type@30ms 'curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/security/pii/test -d "{\"text\":\"My credit card is 4111 1111 1111 1111\"}" | python3 -m json.tool'
Enter
Sleep 3s

Type "# Social Security Number"
Enter
Sleep 500ms

Type@30ms 'curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/security/pii/test -d "{\"text\":\"My SSN is 123-45-6789\"}" | python3 -m json.tool'
Enter
Sleep 3s

Type "# Multiple PII in one sentence"
Enter
Sleep 500ms

Type@30ms 'curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/security/pii/test -d "{\"text\":\"DOB 03/15/1990 account 98765432 CVV 123\"}" | python3 -m json.tool'
Enter
Sleep 3s

Type "# PII masked before reaching Claude or call recording"
Enter
Sleep 2s
