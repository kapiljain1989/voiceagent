import type {
  Agent, Call, Document, RAGResult, LLMConfig, LLMTestResult,
  RobocallResult, PIIResult, VoicePrint, FailoverStatus,
  DashboardStats, SSEEvent, OriginateResult,
} from "./types";

/**
 * VoiceAgent SDK — TypeScript client for the Telecom-Native AI Gateway.
 *
 * @example
 * ```ts
 * const client = new VoiceAgentClient("http://localhost:8080");
 *
 * // Health check
 * const health = await client.health();
 *
 * // Create an agent
 * await client.createAgent({ name: "Priya", email: "priya@co.com", expertise: ["billing"] });
 *
 * // Test robocall detection
 * const result = await client.testRobocall({ text: "Press 1 for your warranty" });
 *
 * // Stream co-pilot events
 * client.streamEvents("call-id", (event) => {
 *   if (event.type === "transcript") console.log(`[${event.speaker}] ${event.text}`);
 *   if (event.type === "suggestion") console.log(`>>> ${event.suggestion}`);
 * });
 * ```
 */
export class VoiceAgentClient {
  private baseUrl: string;

  constructor(baseUrl: string = "http://localhost:8080") {
    this.baseUrl = baseUrl.replace(/\/$/, "");
  }

  // ── Health ──────────────────────────────────────────────────

  async health(): Promise<{ status: string; sessions: number }> {
    return this.get("/healthz");
  }

  async stats(): Promise<DashboardStats> {
    return this.get("/api/stats");
  }

  async failoverStatus(): Promise<FailoverStatus> {
    return this.get("/api/failover/status");
  }

  // ── Agents ──────────────────────────────────────────────────

  async listAgents(): Promise<Agent[]> {
    return this.get("/api/agents");
  }

  async createAgent(agent: {
    name: string;
    email: string;
    phone?: string;
    expertise?: string[];
  }): Promise<{ id: string; status: string }> {
    return this.post("/api/agents", agent);
  }

  // ── Calls ───────────────────────────────────────────────────

  async listCalls(): Promise<Call[]> {
    return this.get("/api/calls");
  }

  async activeCalls(): Promise<{ interactive: number; copilot: number; total: number }> {
    return this.get("/api/calls/active");
  }

  async originateCall(params: {
    to: string;
    from?: string;
    mode?: "sbc" | "loopback";
  }): Promise<OriginateResult> {
    return this.post("/call", params);
  }

  // ── Documents & RAG ─────────────────────────────────────────

  async listDocuments(): Promise<Document[]> {
    return this.get("/api/documents");
  }

  async indexDocument(doc: {
    name: string;
    category: string;
    content: string;
  }): Promise<{ id: string; chunks: number; status: string }> {
    return this.post("/api/documents", doc);
  }

  async ragSearch(query: string, topK: number = 3): Promise<RAGResult[]> {
    return this.post("/api/documents/search", { query, top_k: topK });
  }

  // ── LLM ─────────────────────────────────────────────────────

  async listLLMConfigs(): Promise<LLMConfig[]> {
    return this.get("/api/llm/configs");
  }

  async addLLMConfig(config: {
    name: string;
    provider: string;
    model: string;
    region: string;
  }): Promise<{ status: string }> {
    return this.post("/api/llm/configs", config);
  }

  async testLLM(params: {
    provider: string;
    model: string;
    prompt?: string;
  }): Promise<LLMTestResult> {
    return this.post("/api/llm/test", params);
  }

  // ── Robocall Detection ──────────────────────────────────────

  async listBlocklist(): Promise<Array<{ number: string; reason: string }>> {
    return this.get("/api/blocklist");
  }

  async addToBlocklist(number: string, reason: string = ""): Promise<{ status: string }> {
    return this.post("/api/blocklist", { number, reason });
  }

  async removeFromBlocklist(number: string): Promise<{ status: string }> {
    return this.delete("/api/blocklist", { number });
  }

  async robocallStats(): Promise<Record<string, any>> {
    return this.get("/api/robocall/stats");
  }

  async testRobocall(params: { text?: string; number?: string }): Promise<Record<string, any>> {
    return this.post("/api/robocall/test", params);
  }

  // ── Security ────────────────────────────────────────────────

  async listVoiceprints(): Promise<VoicePrint[]> {
    return this.get("/api/security/voiceprints");
  }

  async enrollVoiceprint(label: string, type: "fraud" | "verified"): Promise<VoicePrint> {
    return this.post("/api/security/voiceprints", { label, type });
  }

  async testPII(text: string): Promise<PIIResult> {
    return this.post("/api/security/pii/test", { text });
  }

  async piiConfig(): Promise<{ enabled: boolean; patterns: any[] }> {
    return this.get("/api/security/pii/config");
  }

  async setPIIEnabled(enabled: boolean): Promise<{ status: string }> {
    return this.post("/api/security/pii/config", { enabled });
  }

  // ── Actions ─────────────────────────────────────────────────

  async listWebhooks(): Promise<Record<string, string>> {
    return this.get("/api/actions/webhooks");
  }

  async setWebhooks(webhooks: Record<string, string>): Promise<{ status: string }> {
    return this.post("/api/actions/webhooks", webhooks);
  }

  // ── DTMF ────────────────────────────────────────────────────

  async testDTMF(digits: string): Promise<{ input: string; parsed: string }> {
    return this.post("/api/dtmf/test", { text: digits });
  }

  // ── SSE Streaming ───────────────────────────────────────────

  /**
   * Stream real-time co-pilot events for a call.
   * Uses Server-Sent Events (works in browser and Node.js).
   *
   * @example
   * ```ts
   * const cleanup = client.streamEvents("call-123", (event) => {
   *   if (event.type === "transcript") console.log(`[${event.speaker}] ${event.text}`);
   *   if (event.type === "suggestion") console.log(`>>> ${event.suggestion}`);
   *   if (event.type === "summary") console.log(`Summary: ${event.summary}`);
   * });
   *
   * // Later: cleanup() to stop streaming
   * ```
   */
  streamEvents(callId: string, onEvent: (event: SSEEvent) => void): () => void {
    const url = `${this.baseUrl}/siprec/events?call_id=${callId}`;
    const eventSource = new EventSource(url);

    eventSource.onmessage = (msg) => {
      try {
        const event: SSEEvent = JSON.parse(msg.data);
        onEvent(event);
      } catch {}
    };

    return () => eventSource.close();
  }

  // ── Internal ────────────────────────────────────────────────

  private async get<T>(path: string): Promise<T> {
    const res = await fetch(`${this.baseUrl}${path}`);
    if (!res.ok) throw new Error(`${res.status}: ${await res.text()}`);
    return res.json();
  }

  private async post<T>(path: string, body: any): Promise<T> {
    const res = await fetch(`${this.baseUrl}${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(`${res.status}: ${await res.text()}`);
    return res.json();
  }

  private async delete<T>(path: string, body: any): Promise<T> {
    const res = await fetch(`${this.baseUrl}${path}`, {
      method: "DELETE",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(`${res.status}: ${await res.text()}`);
    return res.json();
  }
}
