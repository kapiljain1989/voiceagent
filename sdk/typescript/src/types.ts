export interface Agent {
  id: string;
  name: string;
  email: string;
  phone: string;
  expertise: string[];
  status: "available" | "busy" | "offline";
  maxCalls: number;
  activeCalls: number;
}

export interface Call {
  id: string;
  callerNumber: string;
  calledNumber: string;
  mode: "interactive" | "copilot";
  status: "ringing" | "active" | "completed" | "failed";
  duration: number;
  summary: string;
  sentiment: "positive" | "neutral" | "negative";
  startTime: string;
}

export interface Document {
  id: string;
  name: string;
  type: string;
  size: number;
  category: string;
  chunks: number;
  status: "processing" | "indexed" | "failed";
}

export interface RAGResult {
  text: string;
  doc_name: string;
  score: number;
  chunk_index: number;
}

export interface LLMConfig {
  id: string;
  name: string;
  provider: "anthropic-vertex" | "gemini-vertex";
  model: string;
  region: string;
  isDefault: boolean;
  maxTokens: number;
}

export interface LLMTestResult {
  response: string;
  latency: number;
  model: string;
}

export interface RobocallResult {
  score: number;
  category: "human" | "robocall" | "uncertain";
  reason: string;
  keywords: string[];
  blocked: boolean;
}

export interface PIIResult {
  original: string;
  masked: string;
  pii_found: boolean;
  detections: Array<{ type: string; level: string; masked: string }>;
}

export interface VoicePrint {
  id: string;
  label: string;
  type: "fraud" | "verified";
  features_dim: number;
}

export interface FailoverStatus {
  llm: { state: string; failures: number };
  stt: { state: string; failures: number };
  tts: { state: string; failures: number };
  esl: { state: string; failures: number };
}

export interface DashboardStats {
  activeCalls: number;
  totalToday: number;
  avgDuration: number;
  sentimentBreakdown: { positive: number; neutral: number; negative: number };
}

export interface SSEEvent {
  type: "transcript" | "suggestion" | "summary" | "robocall" | "pii_masked" | "action" | "transfer";
  speaker?: string;
  text?: string;
  suggestion?: string;
  category?: string;
  confidence?: number;
  summary?: string;
  action_items?: string[];
  sentiment?: string;
  duration?: number;
}

export interface OriginateResult {
  status: string;
  call_id?: string;
  error?: string;
}
