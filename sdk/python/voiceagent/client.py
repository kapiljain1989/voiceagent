"""VoiceAgent API client — Python SDK for the Telecom-Native AI Gateway."""

import json
from typing import AsyncIterator, Callable, Optional
from dataclasses import asdict

import httpx

from voiceagent.models import (
    Agent, Call, Document, RAGResult, LLMConfig, LLMTestResult,
    RobocallResult, PIIResult, VoicePrint, FailoverStatus,
    DashboardStats, SSEEvent,
)


class VoiceAgentClient:
    """Client for the VoiceAgent Gateway API.

    Usage:
        client = VoiceAgentClient("http://localhost:8080")

        # Health check
        health = client.health()

        # Create an agent
        agent = client.create_agent(name="Priya", email="priya@co.com", expertise=["billing"])

        # Originate a call
        result = client.originate_call(to="+15551234567")

        # Search knowledge base
        results = client.rag_search("water damage coverage")

        # Test robocall detection
        result = client.test_robocall(text="Press 1 for your auto warranty")

        # Stream co-pilot events
        async for event in client.stream_events(call_id="xxx"):
            print(event.type, event.text)
    """

    def __init__(self, base_url: str = "http://localhost:8080", timeout: float = 30.0):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self._client = httpx.Client(base_url=self.base_url, timeout=timeout)

    def close(self):
        self._client.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    # ── Health ──────────────────────────────────────────────────

    def health(self) -> dict:
        """Gateway health check."""
        return self._get("/healthz")

    def stats(self) -> DashboardStats:
        """Dashboard statistics."""
        data = self._get("/api/stats")
        return DashboardStats(
            active_calls=data.get("activeCalls", 0),
            total_today=data.get("totalToday", 0),
            avg_duration=data.get("avgDuration", 0),
            sentiment_breakdown=data.get("sentimentBreakdown", {}),
        )

    def failover_status(self) -> FailoverStatus:
        """Circuit breaker health for all services."""
        data = self._get("/api/failover/status")
        return FailoverStatus(**data)

    # ── Agents ──────────────────────────────────────────────────

    def list_agents(self) -> list[Agent]:
        """List all agents."""
        data = self._get("/api/agents")
        return [Agent(**{_snake(k): v for k, v in a.items()}) for a in data]

    def create_agent(
        self, name: str, email: str, phone: str = "", expertise: list[str] = None
    ) -> dict:
        """Create a new agent."""
        return self._post("/api/agents", {
            "name": name, "email": email, "phone": phone,
            "expertise": expertise or [],
        })

    # ── Calls ───────────────────────────────────────────────────

    def list_calls(self) -> list[Call]:
        """List recent calls."""
        data = self._get("/api/calls")
        return [Call(**{_snake(k): v for k, v in c.items() if _snake(k) in Call.__dataclass_fields__}) for c in data]

    def active_calls(self) -> dict:
        """Active session counts."""
        return self._get("/api/calls/active")

    def originate_call(
        self, to: str, from_: str = "", mode: str = "sbc"
    ) -> dict:
        """Originate an outbound call."""
        return self._post("/call", {"to": to, "from": from_, "mode": mode})

    # ── Documents & RAG ─────────────────────────────────────────

    def list_documents(self) -> list[Document]:
        """List indexed documents."""
        data = self._get("/api/documents")
        return [Document(**{_snake(k): v for k, v in d.items() if _snake(k) in Document.__dataclass_fields__}) for d in data]

    def index_document(self, name: str, category: str, content: str) -> dict:
        """Upload and index a document for RAG."""
        return self._post("/api/documents", {
            "name": name, "category": category, "content": content,
        })

    def rag_search(self, query: str, top_k: int = 3) -> list[RAGResult]:
        """Search the knowledge base."""
        data = self._post("/api/documents/search", {"query": query, "top_k": top_k})
        if isinstance(data, list):
            return [RAGResult(**{_snake(k): v for k, v in r.items()}) for r in data]
        return []

    # ── LLM ─────────────────────────────────────────────────────

    def list_llm_configs(self) -> list[LLMConfig]:
        """List LLM model configurations."""
        data = self._get("/api/llm/configs")
        return [LLMConfig(**{_snake(k): v for k, v in c.items() if _snake(k) in LLMConfig.__dataclass_fields__}) for c in data]

    def add_llm_config(
        self, name: str, provider: str, model: str, region: str
    ) -> dict:
        """Add a new LLM configuration."""
        return self._post("/api/llm/configs", {
            "name": name, "provider": provider, "model": model, "region": region,
        })

    def test_llm(
        self, provider: str, model: str, prompt: str = "Hello"
    ) -> LLMTestResult:
        """Test an LLM model."""
        data = self._post("/api/llm/test", {
            "provider": provider, "model": model, "prompt": prompt,
        })
        return LLMTestResult(**{k: data.get(k, "") for k in LLMTestResult.__dataclass_fields__})

    # ── Robocall Detection ──────────────────────────────────────

    def list_blocklist(self) -> list[dict]:
        """List blocked numbers."""
        return self._get("/api/blocklist")

    def add_to_blocklist(self, number: str, reason: str = "") -> dict:
        """Add a number to the blocklist."""
        return self._post("/api/blocklist", {"number": number, "reason": reason})

    def remove_from_blocklist(self, number: str) -> dict:
        """Remove a number from the blocklist."""
        return self._delete("/api/blocklist", {"number": number})

    def robocall_stats(self) -> dict:
        """Robocall detection metrics."""
        return self._get("/api/robocall/stats")

    def test_robocall(self, text: str = "", number: str = "") -> dict:
        """Test robocall classification."""
        return self._post("/api/robocall/test", {"text": text, "number": number})

    # ── Security ────────────────────────────────────────────────

    def list_voiceprints(self) -> list[VoicePrint]:
        """List enrolled voice prints."""
        data = self._get("/api/security/voiceprints")
        return [VoicePrint(**{_snake(k): v for k, v in vp.items() if _snake(k) in VoicePrint.__dataclass_fields__}) for vp in data]

    def enroll_voiceprint(self, label: str, type: str) -> dict:
        """Enroll a new voice print."""
        return self._post("/api/security/voiceprints", {"label": label, "type": type})

    def test_pii(self, text: str) -> PIIResult:
        """Test PII masking."""
        data = self._post("/api/security/pii/test", {"text": text})
        return PIIResult(**{_snake(k): v for k, v in data.items() if _snake(k) in PIIResult.__dataclass_fields__})

    def pii_config(self) -> dict:
        """Get PII masking configuration."""
        return self._get("/api/security/pii/config")

    def set_pii_enabled(self, enabled: bool) -> dict:
        """Enable/disable PII masking."""
        return self._post("/api/security/pii/config", {"enabled": enabled})

    # ── Actions ─────────────────────────────────────────────────

    def list_webhooks(self) -> dict:
        """List configured action webhook URLs."""
        return self._get("/api/actions/webhooks")

    def set_webhooks(self, webhooks: dict) -> dict:
        """Configure action webhook URLs."""
        return self._post("/api/actions/webhooks", webhooks)

    # ── DTMF ────────────────────────────────────────────────────

    def test_dtmf(self, digits: str) -> dict:
        """Test DTMF digit parsing."""
        return self._post("/api/dtmf/test", {"text": digits})

    # ── SSE Streaming ───────────────────────────────────────────

    def stream_events(
        self, call_id: str, callback: Callable[[SSEEvent], None] = None
    ):
        """Stream real-time co-pilot events for a call.

        Usage:
            def on_event(event):
                if event.type == "transcript":
                    print(f"[{event.speaker}] {event.text}")
                elif event.type == "suggestion":
                    print(f">>> {event.suggestion}")

            client.stream_events("call-id-123", callback=on_event)
        """
        url = f"{self.base_url}/siprec/events?call_id={call_id}"
        with httpx.stream("GET", url, timeout=None) as response:
            for line in response.iter_lines():
                if line.startswith("data: "):
                    try:
                        data = json.loads(line[6:])
                        event = SSEEvent(**{k: data.get(k) for k in SSEEvent.__dataclass_fields__})
                        if callback:
                            callback(event)
                        else:
                            yield event
                    except (json.JSONDecodeError, TypeError):
                        continue

    # ── Internal ────────────────────────────────────────────────

    def _get(self, path: str) -> dict | list:
        resp = self._client.get(path)
        resp.raise_for_status()
        return resp.json()

    def _post(self, path: str, data: dict) -> dict:
        resp = self._client.post(path, json=data)
        resp.raise_for_status()
        return resp.json()

    def _delete(self, path: str, data: dict) -> dict:
        resp = self._client.request("DELETE", path, json=data)
        resp.raise_for_status()
        return resp.json()


def _snake(name: str) -> str:
    """Convert camelCase to snake_case."""
    result = []
    for i, c in enumerate(name):
        if c.isupper() and i > 0:
            result.append("_")
        result.append(c.lower())
    return "".join(result)
