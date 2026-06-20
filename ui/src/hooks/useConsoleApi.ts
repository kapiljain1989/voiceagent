import { useRef, useEffect, useState, useCallback } from "react";
import { authFetch } from "@/lib/auth";
import type {
  ActiveSession,
  CopilotSuggestion,
  TranscriptEntry,
  AIInsight,
  PostCallSummaryData,
  VoiceSentimentData,
  QueueData,
  TeamMember,
  HealthStatus,
} from "@/lib/console-types";

const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";

// ── Call Origination ──

export async function originateCall(to: string, mode = "loopback") {
  const res = await authFetch("/call", {
    method: "POST",
    body: JSON.stringify({ to, mode }),
  });
  return res.json() as Promise<{ status: string; call_id: string }>;
}

// ── Call Control ──

export async function holdCall(callId: string) {
  return authFetch("/api/call/hold", {
    method: "POST",
    body: JSON.stringify({ call_id: callId }),
  });
}

export async function resumeCall(callId: string) {
  return authFetch("/api/call/resume", {
    method: "POST",
    body: JSON.stringify({ call_id: callId }),
  });
}

export async function muteCall(callId: string) {
  return authFetch("/api/call/mute", {
    method: "POST",
    body: JSON.stringify({ call_id: callId }),
  });
}

export async function unmuteCall(callId: string) {
  return authFetch("/api/call/unmute", {
    method: "POST",
    body: JSON.stringify({ call_id: callId }),
  });
}

export async function transferCall(
  callId: string,
  target: string,
  mode: "blind" | "attended",
  department?: string,
) {
  return authFetch("/api/call/transfer", {
    method: "POST",
    body: JSON.stringify({ call_id: callId, target, mode, department }),
  });
}

export async function conferenceCall(callId: string, target: string, targetType: string = "agent") {
  return authFetch("/api/call/conference", {
    method: "POST",
    body: JSON.stringify({ call_id: callId, target, target_type: targetType }),
  });
}

export async function conferenceCallDrop(callId: string, who: "third" | "self") {
  return authFetch("/api/call/conference/drop", {
    method: "POST",
    body: JSON.stringify({ call_id: callId, who }),
  });
}

// ── Queue ──

export async function fetchQueues(): Promise<QueueData[]> {
  const res = await authFetch("/api/queues");
  if (!res.ok) return [];
  return res.json();
}

export async function pickCallFromQueue(
  entryId: string,
  agentId: string,
): Promise<{ call_id: string }> {
  const res = await authFetch("/api/queue/pick", {
    method: "POST",
    body: JSON.stringify({ queue_entry_id: entryId, agent_id: agentId }),
  });
  return res.json();
}

// ── Agent Status ──

export async function updateAgentStatus(agentId: string, status: string) {
  return authFetch("/api/agent/status", {
    method: "POST",
    body: JSON.stringify({ agent_id: agentId, status }),
  });
}

export async function fetchAgentDirectory(): Promise<TeamMember[]> {
  const res = await authFetch("/api/agents/directory");
  if (!res.ok) return [];
  return res.json();
}

// ── Health ──

export async function fetchHealth(): Promise<HealthStatus> {
  const res = await fetch(`${GATEWAY}/healthz`);
  return res.json();
}

// ── Active Sessions ──

export async function fetchActiveSessions(): Promise<ActiveSession[]> {
  const res = await authFetch("/api/copilot/active");
  if (!res.ok) return [];
  return res.json();
}

// ── SSE Hook ──

export function useSSEStream(callId: string | null) {
  const [transcripts, setTranscripts] = useState<TranscriptEntry[]>([]);
  const [suggestions, setSuggestions] = useState<CopilotSuggestion[]>([]);
  const [summary, setSummary] = useState<PostCallSummaryData | null>(null);
  const [callStateFromSSE, setCallStateFromSSE] = useState<string | null>(null);
  const [voiceSentiment, setVoiceSentiment] = useState<VoiceSentimentData | null>(null);
  const [conferenceState, setConferenceState] = useState<{ active: boolean; thirdParty?: string; thirdType?: string } | null>(null);
  const esRef = useRef<EventSource | null>(null);

  const reset = useCallback(() => {
    setTranscripts([]);
    setSuggestions([]);
    setSummary(null);
    setCallStateFromSSE(null);
    setConferenceState(null);
    setVoiceSentiment(null);
  }, []);

  useEffect(() => {
    if (!callId) {
      if (esRef.current) {
        esRef.current.close();
        esRef.current = null;
      }
      return;
    }

    const es = new EventSource(`${GATEWAY}/siprec/events?call_id=${callId}`);
    esRef.current = es;

    es.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        const now = new Date().toLocaleTimeString("en-US", {
          hour12: false,
          hour: "2-digit",
          minute: "2-digit",
          second: "2-digit",
        });

        if (data.type === "transcript") {
          setTranscripts((prev) => [
            ...prev,
            { speaker: data.speaker, text: data.text, time: now },
          ]);
        } else if (data.type === "suggestion") {
          setSuggestions((prev) => [
            ...prev,
            {
              text: data.suggestion,
              category: data.category || "answer",
              confidence: data.confidence || 0.8,
              time: now,
            },
          ]);
        } else if (data.type === "summary") {
          setSummary({
            summary: data.summary || "",
            action_items: data.action_items || [],
            sentiment: data.sentiment || "neutral",
            duration: data.duration || 0,
            voice_sentiment: data.voice_sentiment,
          });
        } else if (data.type === "voice_sentiment") {
          setVoiceSentiment({
            agitation: data.agitation ?? 0,
            frustration: data.frustration ?? 0,
            engagement: data.engagement ?? 0,
            avg_pitch_hz: data.avg_pitch_hz ?? 0,
            speaking_rate_wpm: data.speaking_rate_wpm ?? 0,
            avg_energy: data.avg_energy ?? 0,
            energy_trend: data.energy_trend ?? "stable",
            pitch_variance: data.pitch_variance ?? 0,
            silence_ratio: data.silence_ratio ?? 0,
            sentiment: data.sentiment ?? "neutral",
            confidence: data.confidence ?? 0,
          });
        } else if (data.type === "conference_started") {
          setConferenceState({ active: true, thirdParty: data.third_party, thirdType: data.third_type });
        } else if (data.type === "conference_ended" || data.type === "conference_failed") {
          setConferenceState(null);
        } else if (data.type === "call_state") {
          setCallStateFromSSE(data.state);
        }
      } catch {
        // ignore parse errors
      }
    };

    es.onerror = () => {
      setTimeout(() => {
        if (esRef.current === es && callId) {
          es.close();
          // reconnect handled by re-render
        }
      }, 2000);
    };

    return () => {
      es.close();
      esRef.current = null;
    };
  }, [callId]);

  return { transcripts, suggestions, summary, callStateFromSSE, voiceSentiment, conferenceState, reset };
}

// ── Polling Hooks ──

export function usePolling<T>(
  fetcher: () => Promise<T>,
  intervalMs: number,
  enabled = true,
) {
  const [data, setData] = useState<T | null>(null);

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;

    async function poll() {
      try {
        const result = await fetcher();
        if (!cancelled) setData(result);
      } catch {
        // silent
      }
    }

    poll();
    const iv = setInterval(poll, intervalMs);
    return () => {
      cancelled = true;
      clearInterval(iv);
    };
  }, [fetcher, intervalMs, enabled]);

  return data;
}
