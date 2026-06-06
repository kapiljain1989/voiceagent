export interface Agent {
  id: string;
  name: string;
  email: string;
  phone?: string;
  expertise: string[];
  status: "available" | "busy" | "offline";
  maxCalls: number;
  activeCalls: number;
  createdAt: string;
}

export interface Call {
  id: string;
  callerNumber: string;
  calledNumber: string;
  agentId?: string;
  agent?: Agent;
  mode: "interactive" | "copilot";
  status: "ringing" | "active" | "completed" | "failed";
  startTime: string;
  endTime?: string;
  duration?: number;
  summary?: string;
  sentiment?: "positive" | "neutral" | "negative";
  actionItems: string[];
  commitments: string[];
  transcript?: Utterance[];
  suggestions?: Suggestion[];
  llmModel: string;
}

export interface Utterance {
  speaker: "customer" | "agent";
  text: string;
  timestamp: string;
}

export interface Suggestion {
  suggestion: string;
  context: string;
  category: "answer" | "upsell" | "compliance" | "empathy" | "none";
  confidence: number;
}

export interface CallSummary {
  conversation_id: string;
  duration_seconds: number;
  transcript: Utterance[];
  summary: string;
  action_items: string[];
  commitments_made: string[];
  sentiment: string;
  suggestions_given: Suggestion[];
}

export interface Document {
  id: string;
  name: string;
  type: string;
  size: number;
  category: string;
  chromaId?: string;
  chunks: number;
  status: "processing" | "indexed" | "failed";
  uploadedAt: string;
}

export interface LLMConfig {
  id: string;
  name: string;
  provider: "anthropic-vertex" | "gemini-vertex";
  model: string;
  region: string;
  isDefault: boolean;
  systemPrompt?: string;
  maxTokens: number;
}

export interface DashboardStats {
  activeCalls: number;
  totalToday: number;
  avgDuration: number;
  sentimentBreakdown: {
    positive: number;
    neutral: number;
    negative: number;
  };
}

export interface SSEEvent {
  type: "transcript" | "suggestion" | "summary";
  speaker?: string;
  text?: string;
  suggestion?: string;
  category?: string;
  confidence?: number;
  context?: string;
  summary?: string;
  action_items?: string[];
  commitments?: string[];
  sentiment?: string;
  duration?: number;
}
