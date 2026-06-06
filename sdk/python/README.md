# VoiceAgent Python SDK

Python client for the VoiceAgent Telecom-Native AI Gateway.

## Install

```bash
pip install -e sdk/python
```

## Usage

```python
from voiceagent import VoiceAgentClient

client = VoiceAgentClient("http://localhost:8080")

# Health check
print(client.health())

# Create an agent
client.create_agent(name="Priya Sharma", email="priya@co.com", expertise=["billing"])

# Index a document for RAG
client.index_document(
    name="Insurance Policy",
    category="policy",
    content="Water damage from burst pipes is covered. Deductible is $500."
)

# Search knowledge base
results = client.rag_search("water damage coverage")
for r in results:
    print(f"[{r.score:.0%}] {r.doc_name}: {r.text[:80]}")

# Test robocall detection
result = client.test_robocall(text="Press 1 for your auto warranty")
print(f"Score: {result['keyword']['score']}, Category: {result['keyword']['category']}")

# Test PII masking
pii = client.test_pii("My SSN is 123-45-6789")
print(f"Masked: {pii.masked}")

# Originate a call
call = client.originate_call(to="+15551234567")
print(f"Call ID: {call['call_id']}")

# Stream co-pilot events
def on_event(event):
    if event.type == "transcript":
        print(f"[{event.speaker}] {event.text}")
    elif event.type == "suggestion":
        print(f">>> {event.suggestion}")
    elif event.type == "summary":
        print(f"Summary: {event.summary}")

client.stream_events("call-id-123", callback=on_event)
```
