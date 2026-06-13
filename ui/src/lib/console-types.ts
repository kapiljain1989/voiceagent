export type CallState = "idle" | "dialing" | "connected" | "hold" | "muted" | "disconnected";
export type AgentStatus = "Available" | "Busy" | "On Break" | "Wrap-up";

export interface QueueCaller {
  id: string;
  call_id: string;
  number: string;
  waitSec: number;
  reason: string;
  priority: "low" | "normal" | "high";
}

export interface QueueData {
  name: string;
  avgHandle: string;
  sla: number;
  callers: QueueCaller[];
}

export interface TeamMember {
  id: string;
  name: string;
  ext: string;
  status: string;
  department: string;
  activeCalls: number;
}

export interface VoiceSentimentData {
  avg_energy: number;
  energy_trend: "rising" | "falling" | "stable";
  avg_pitch_hz: number;
  pitch_variance: number;
  speaking_rate_wpm: number;
  silence_ratio: number;
  agitation: number;
  engagement: number;
  frustration: number;
  sentiment: string;
  confidence: number;
}

export interface CopilotSuggestion {
  text: string;
  category: "answer" | "empathy" | "compliance" | "upsell";
  confidence: number;
  time?: string;
}

export interface TranscriptEntry {
  speaker: "customer" | "agent";
  text: string;
  time: string;
}

export interface AIInsight {
  type: "summary" | "action" | "sentiment" | "compliance";
  text: string;
}

export interface PostCallSummaryData {
  summary: string;
  action_items: string[];
  sentiment: string;
  resolution?: string;
  duration: number;
  csat_prediction?: number;
  topics?: string[];
  voice_sentiment?: VoiceSentimentData;
}

export interface ActiveSession {
  call_id: string;
  duration: number;
  caller?: string;
  agent?: string;
  voice_sentiment?: VoiceSentimentData;
}

export interface HealthStatus {
  status: string;
  sessions: number;
  mode: string;
}

export const STATUS_COLORS: Record<string, string> = {
  Available: "emerald",
  Busy: "rose",
  "On Break": "amber",
  "Wrap-up": "violet",
  "On Call": "cyan",
};

export const DEPARTMENTS = ["Support", "Sales", "Billing", "Escalation"];
