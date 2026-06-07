Output demos/gifs/rag.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Catppuccin Mocha"
Set TypingSpeed 50ms
Set Padding 20

Type "# RAG Knowledge Base — upload + vector search"
Enter
Sleep 1s

Type "# Upload a policy document"
Enter
Sleep 500ms

Type "demos/run-demo.sh doc-upload 'Insurance Policy' 'policy' 'Water damage from burst pipes is covered. Deductible is 500 for Tier 2. Claims filed within 30 days.'"
Enter
Sleep 3s

Type "# Search the knowledge base"
Enter
Sleep 500ms

Type "demos/run-demo.sh doc-search 'water damage coverage'"
Enter
Sleep 3s

Type "# List all documents"
Enter
Sleep 500ms

Type "demos/run-demo.sh doc-list"
Enter
Sleep 3s
