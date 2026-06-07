Output demos/gifs/pii-masking.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Catppuccin Mocha"
Set TypingSpeed 50ms
Set Padding 20

Type "# PII Masking — 7 patterns, masks before LLM"
Enter
Sleep 1s

Type "# Credit card number"
Enter
Sleep 500ms

Type "demos/run-demo.sh pii-test 'My credit card is 4111 1111 1111 1111'"
Enter
Sleep 3s

Type "# Social Security Number"
Enter
Sleep 500ms

Type "demos/run-demo.sh pii-test 'My SSN is 123-45-6789'"
Enter
Sleep 3s

Type "# Multiple PII types"
Enter
Sleep 500ms

Type "demos/run-demo.sh pii-test 'DOB 03/15/1990 account 98765432 CVV 123'"
Enter
Sleep 3s
