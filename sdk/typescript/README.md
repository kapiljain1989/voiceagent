# @voiceagent/sdk — TypeScript

TypeScript/Node.js client for the VoiceAgent Telecom-Native AI Gateway.

## Install

```bash
npm install @voiceagent/sdk
# or link locally
cd sdk/typescript && npm install && npm run build
```

## Usage

```typescript
import { VoiceAgentClient } from "@voiceagent/sdk";

const client = new VoiceAgentClient("http://localhost:8080");

// Health check
const health = await client.health();
console.log(health);

// Create an agent
await client.createAgent({
  name: "Priya Sharma",
  email: "priya@company.com",
  expertise: ["billing", "retention"],
});

// Index a document for RAG
await client.indexDocument({
  name: "Insurance Policy",
  category: "policy",
  content: "Water damage from burst pipes is covered. Deductible is $500.",
});

// Search knowledge base
const results = await client.ragSearch("water damage coverage");
results.forEach((r) => console.log(`[${(r.score * 100).toFixed(0)}%] ${r.doc_name}: ${r.text}`));

// Test robocall detection
const robocall = await client.testRobocall({ text: "Press 1 for your auto warranty" });
console.log(robocall);

// Test PII masking
const pii = await client.testPII("My SSN is 123-45-6789");
console.log(`Masked: ${pii.masked}`);

// Originate a call
const call = await client.originateCall({ to: "+15551234567" });
console.log(`Call ID: ${call.call_id}`);

// Stream co-pilot events (browser or Node.js with EventSource polyfill)
const cleanup = client.streamEvents("call-id-123", (event) => {
  switch (event.type) {
    case "transcript":
      console.log(`[${event.speaker}] ${event.text}`);
      break;
    case "suggestion":
      console.log(`>>> ${event.suggestion}`);
      break;
    case "summary":
      console.log(`Summary: ${event.summary}`);
      break;
  }
});

// Later: cleanup() to stop streaming
```

## Use in Next.js

```typescript
// app/api/calls/route.ts
import { VoiceAgentClient } from "@voiceagent/sdk";

const client = new VoiceAgentClient(process.env.GATEWAY_URL);

export async function GET() {
  const calls = await client.listCalls();
  return Response.json(calls);
}
```

```typescript
// components/LiveTranscript.tsx
"use client";
import { useEffect, useState } from "react";
import { VoiceAgentClient, SSEEvent } from "@voiceagent/sdk";

export function LiveTranscript({ callId }: { callId: string }) {
  const [events, setEvents] = useState<SSEEvent[]>([]);

  useEffect(() => {
    const client = new VoiceAgentClient();
    const cleanup = client.streamEvents(callId, (event) => {
      setEvents((prev) => [...prev, event]);
    });
    return cleanup;
  }, [callId]);

  return (
    <div>
      {events.map((e, i) => (
        <div key={i}>
          {e.type === "transcript" && <p>[{e.speaker}] {e.text}</p>}
          {e.type === "suggestion" && <p>>>> {e.suggestion}</p>}
        </div>
      ))}
    </div>
  );
}
```
