Output demos/gifs/rag.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set Padding 20

Type "# RAG Knowledge Base — Document upload + vector search"
Enter
Sleep 1s

Type "# Upload a document"
Enter
Sleep 500ms

Type@30ms 'curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/documents -H "Content-Type: application/json" -d "{\"name\":\"Insurance Policy\",\"category\":\"policy\",\"content\":\"Water damage from burst pipes is covered. Deductible is $500 for Tier 2. Claims must be filed within 30 days.\"}" | python3 -m json.tool'
Enter
Sleep 3s

Type "# Search the knowledge base"
Enter
Sleep 500ms

Type@30ms 'curl -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/api/documents/search -d "{\"query\":\"water damage coverage\"}" | python3 -m json.tool'
Enter
Sleep 3s

Type "# List all documents"
Enter
Sleep 500ms

Type@30ms 'curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/documents | python3 -c "import sys,json; [print(f\"  {d[\"name\"]} ({d[\"category\"]})\") for d in json.load(sys.stdin)]"'
Enter
Sleep 3s

Type "# ChromaDB vector search powers co-pilot RAG coaching"
Enter
Sleep 2s
