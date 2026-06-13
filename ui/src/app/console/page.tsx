"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import {
  Phone,
  PhoneOff,
  PhoneOutgoing,
  Mic,
  MicOff,
  Pause,
  Play,
  ArrowRightLeft,
  Grid3x3,
  Hexagon,
  ChevronDown,
  ChevronUp,
  PenLine,
  Radio,
  Sparkles,
  Users,
} from "lucide-react";
import type {
  CallState,
  AgentStatus,
  QueueData,
  QueueCaller,
  TeamMember,
  VoiceSentimentData,
  TranscriptEntry,
  AIInsight,
  CopilotSuggestion,
  PostCallSummaryData,
} from "@/lib/console-types";
import { STATUS_COLORS } from "@/lib/console-types";
import {
  originateCall,
  holdCall,
  resumeCall,
  muteCall,
  unmuteCall,
  transferCall,
  conferenceCall,
  fetchQueues,
  pickCallFromQueue,
  updateAgentStatus,
  fetchAgentDirectory,
  fetchHealth,
  fetchActiveSessions,
  useSSEStream,
  usePolling,
} from "@/hooks/useConsoleApi";
import { StatusDot, ConsoleBadge, IconBtn } from "@/components/console/Primitives";
import { WebRTCHealth } from "@/components/console/WebRTCHealth";
import { DialPad } from "@/components/console/DialPad";
import { useWebRTC } from "@/hooks/useWebRTC";
import { authFetch } from "@/lib/auth";
import { TransferPanel } from "@/components/console/TransferPanel";
import { QueueMonitor } from "@/components/console/QueueMonitor";
import { AgentDirectory } from "@/components/console/AgentDirectory";
import { CallerMoodGauge } from "@/components/console/CallerMoodGauge";
import { AICopilotPanel } from "@/components/console/AICopilotPanel";
import { PostCallSummary } from "@/components/console/PostCallSummary";
import { TranscriptPanel } from "@/components/console/TranscriptPanel";

const AGENT_STATUSES: AgentStatus[] = ["Available", "Busy", "On Break", "Wrap-up"];
const IS_DEMO = process.env.NEXT_PUBLIC_DEMO_MODE === "true";

// ── Mock data for demo mode (used when NEXT_PUBLIC_DEMO_MODE=true) ──

const MOCK_TEAM: TeamMember[] = [
  { id: "1", name: "Sarah Chen", ext: "2001", status: "Available", department: "Support", activeCalls: 0 },
  { id: "2", name: "Alex Rivera", ext: "2002", status: "On Call", department: "Sales", activeCalls: 1 },
  { id: "3", name: "Priya Sharma", ext: "2003", status: "Available", department: "Support", activeCalls: 0 },
  { id: "4", name: "Marcus Johnson", ext: "2004", status: "On Break", department: "Billing", activeCalls: 0 },
  { id: "5", name: "Yuki Tanaka", ext: "2005", status: "Wrap-up", department: "Support", activeCalls: 0 },
  { id: "6", name: "Raj Patel", ext: "2006", status: "Available", department: "Sales", activeCalls: 0 },
  { id: "7", name: "Emma Wilson", ext: "2007", status: "Busy", department: "Escalation", activeCalls: 1 },
];

const MOCK_QUEUES: QueueData[] = [
  { name: "Support", avgHandle: "6:30", sla: 82, callers: [
    { id: "q1", call_id: "c1", number: "+1 (555) 234-8901", waitSec: 252, reason: "Order status inquiry", priority: "normal" },
    { id: "q2", call_id: "c2", number: "+1 (555) 876-5432", waitSec: 185, reason: "Technical support", priority: "normal" },
    { id: "q3", call_id: "c3", number: "+1 (555) 111-2233", waitSec: 48, reason: "Account access issue", priority: "high" },
  ]},
  { name: "Sales", avgHandle: "8:15", sla: 94, callers: [
    { id: "q4", call_id: "c4", number: "+1 (555) 444-7788", waitSec: 105, reason: "Pricing inquiry", priority: "normal" },
  ]},
  { name: "Billing", avgHandle: "4:45", sla: 68, callers: [
    { id: "q5", call_id: "c5", number: "+44 20 7946 0958", waitSec: 440, reason: "Disputed charge", priority: "high" },
    { id: "q6", call_id: "c6", number: "+1 (555) 999-0001", waitSec: 310, reason: "Payment plan request", priority: "normal" },
    { id: "q7", call_id: "c7", number: "+1 (555) 888-3344", waitSec: 220, reason: "Refund status", priority: "normal" },
    { id: "q8", call_id: "c8", number: "+1 (555) 777-6655", waitSec: 130, reason: "Invoice correction", priority: "normal" },
    { id: "q9", call_id: "c9", number: "+1 (555) 666-9900", waitSec: 65, reason: "Auto-pay setup", priority: "low" },
  ]},
];

const SENTIMENT_TIMELINE: VoiceSentimentData[] = [
  { agitation: 0.25, frustration: 0.15, engagement: 0.70, avg_pitch_hz: 185, speaking_rate_wpm: 140, avg_energy: 0.45, energy_trend: "stable", pitch_variance: 12, silence_ratio: 0.20, sentiment: "neutral", confidence: 0.82 },
  { agitation: 0.30, frustration: 0.20, engagement: 0.65, avg_pitch_hz: 190, speaking_rate_wpm: 145, avg_energy: 0.48, energy_trend: "rising", pitch_variance: 18, silence_ratio: 0.18, sentiment: "neutral", confidence: 0.85 },
  { agitation: 0.55, frustration: 0.50, engagement: 0.80, avg_pitch_hz: 220, speaking_rate_wpm: 168, avg_energy: 0.62, energy_trend: "rising", pitch_variance: 28, silence_ratio: 0.12, sentiment: "negative", confidence: 0.90 },
  { agitation: 0.70, frustration: 0.65, engagement: 0.85, avg_pitch_hz: 235, speaking_rate_wpm: 175, avg_energy: 0.70, energy_trend: "rising", pitch_variance: 35, silence_ratio: 0.08, sentiment: "negative", confidence: 0.93 },
  { agitation: 0.60, frustration: 0.55, engagement: 0.78, avg_pitch_hz: 215, speaking_rate_wpm: 160, avg_energy: 0.58, energy_trend: "falling", pitch_variance: 22, silence_ratio: 0.15, sentiment: "negative", confidence: 0.88 },
  { agitation: 0.35, frustration: 0.25, engagement: 0.72, avg_pitch_hz: 195, speaking_rate_wpm: 142, avg_energy: 0.50, energy_trend: "falling", pitch_variance: 15, silence_ratio: 0.18, sentiment: "neutral", confidence: 0.86 },
  { agitation: 0.15, frustration: 0.10, engagement: 0.82, avg_pitch_hz: 180, speaking_rate_wpm: 135, avg_energy: 0.42, energy_trend: "stable", pitch_variance: 10, silence_ratio: 0.22, sentiment: "positive", confidence: 0.91 },
  { agitation: 0.10, frustration: 0.05, engagement: 0.88, avg_pitch_hz: 175, speaking_rate_wpm: 130, avg_energy: 0.40, energy_trend: "stable", pitch_variance: 8, silence_ratio: 0.25, sentiment: "positive", confidence: 0.94 },
];

const MOCK_TRANSCRIPT: { speaker: "customer" | "agent"; text: string }[] = [
  { speaker: "customer", text: "Hi, I'm calling about my recent order. It still shows as processing after five days." },
  { speaker: "agent", text: "I understand your concern. Let me pull up your account right away. Can you confirm your order number?" },
  { speaker: "customer", text: "Sure, it's ORD-2024-78432." },
  { speaker: "agent", text: "Thank you. I can see the order here. It looks like there was a hold placed by our warehouse team. Let me check the details." },
  { speaker: "customer", text: "A hold? Nobody told me about that. I need this by Friday." },
  { speaker: "agent", text: "I completely understand the urgency. The hold was due to an address verification. I'm clearing it now and expediting the shipment at no extra cost." },
  { speaker: "customer", text: "Oh, that's great. Will I get a tracking number?" },
  { speaker: "agent", text: "Absolutely. You'll receive an email with tracking within the next 2 hours. Is there anything else I can help with?" },
];

const MOCK_COPILOT: CopilotSuggestion[] = [
  { category: "answer", text: "Order ORD-2024-78432 has a warehouse hold. Address verification required — cleared in system.", confidence: 0.92 },
  { category: "empathy", text: "Customer has been waiting 5 days. Acknowledge the delay and apologize for the inconvenience.", confidence: 0.88 },
  { category: "compliance", text: "Expedited shipping waiver requires manager approval for orders over $500. This order is $340 — approved.", confidence: 0.95 },
  { category: "answer", text: "Tracking number will be generated once shipment is scanned. Typically within 1-2 hours.", confidence: 0.90 },
  { category: "upsell", text: "Customer may benefit from Premium membership — free expedited shipping on all future orders.", confidence: 0.72 },
];

const MOCK_INSIGHTS: AIInsight[] = [
  { type: "summary", text: "Customer frustrated about delayed order ORD-2024-78432. Warehouse hold due to address verification." },
  { type: "action", text: "Clear warehouse hold and expedite shipment. Send tracking number within 2 hours." },
  { type: "sentiment", text: "Caller sentiment shifted from frustrated → satisfied after resolution offered." },
  { type: "compliance", text: "Agent correctly offered expedited shipping as goodwill. No policy violations detected." },
];

const MOCK_POST_CALL: PostCallSummaryData = {
  summary: "Customer called about delayed order ORD-2024-78432. Root cause was a warehouse hold due to address verification. Agent cleared the hold and expedited shipment at no extra cost. Customer satisfied with resolution.",
  action_items: [
    "Send tracking number to customer within 2 hours",
    "Flag address verification process for review — customer was not notified",
    "Follow up in 3 days to confirm delivery",
  ],
  sentiment: "positive",
  resolution: "resolved",
  duration: 0,
  csat_prediction: 4.2,
  topics: ["order-status", "shipping", "warehouse-hold"],
};

function formatTime(seconds: number) {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}

const STATE_DISPLAY: Record<CallState, { label: string; color: string }> = {
  idle: { label: "READY", color: "slate" },
  dialing: { label: "DIALING", color: "cyan" },
  connected: { label: "CONNECTED", color: "emerald" },
  hold: { label: "ON HOLD", color: "amber" },
  muted: { label: "MUTED", color: "amber" },
  disconnected: { label: "ENDED", color: "rose" },
};

interface AgentProfileData {
  id: string; name: string; extension: string; department: string;
  expertise: string[]; languages: string[]; priority: number;
  max_calls: number; current_calls: number; status: string; queues: string[];
}

interface IncomingCall {
  call_id: string; caller: string; queue: string;
}

export default function ConsolePage() {
  // WebRTC hook for real calling
  const webrtc = useWebRTC();

  // Agent profile (fetched on mount)
  const [agentProfile, setAgentProfile] = useState<AgentProfileData | null>(null);
  const [profileLinked, setProfileLinked] = useState(true);

  // Incoming call ring
  const [incomingCall, setIncomingCall] = useState<IncomingCall | null>(null);

  // Core state — synced with WebRTC when active
  const [callState, setCallState] = useState<CallState>("idle");
  const [callId, setCallId] = useState<string | null>(null);
  const [agentStatus, setAgentStatus] = useState<AgentStatus>("Available");
  const [showStatusMenu, setShowStatusMenu] = useState(false);
  const [phoneNumber, setPhoneNumber] = useState("");
  const [showDialpad, setShowDialpad] = useState(false);
  const [showTransfer, setShowTransfer] = useState(false);
  const [callDuration, setCallDuration] = useState(0);
  const [notes, setNotes] = useState("");
  const [showNotesDrawer, setShowNotesDrawer] = useState(false);

  // Simulation refs (used when no real backend)
  const [transcriptEntries, setTranscriptEntries] = useState<TranscriptEntry[]>([]);
  const [aiInsights, setAiInsights] = useState<AIInsight[]>([]);
  const [copilotSuggestions, setCopilotSuggestions] = useState<CopilotSuggestion[]>([]);
  const [voiceSentiment, setVoiceSentiment] = useState<VoiceSentimentData | null>(null);
  const transcriptIdx = useRef(0);
  const insightIdx = useRef(0);
  const sentimentIdx = useRef(0);
  const suggestionIdx = useRef(0);

  // Post-call summary
  const [showPostCallSummary, setShowPostCallSummary] = useState(false);
  const [finalCallDuration, setFinalCallDuration] = useState(0);

  // Conference / Transfer
  const [conferenceActive, setConferenceActive] = useState(false);
  const [conferenceParty, setConferenceParty] = useState<string | null>(null);
  const [transferTarget, setTransferTarget] = useState<string | null>(null);
  const [notification, setNotification] = useState<{ msg: string; color: string } | null>(null);

  // Queue state — seed with mock data in demo mode, empty in production
  const [queues, setQueues] = useState<QueueData[]>(IS_DEMO ? MOCK_QUEUES : []);

  const isCallActive = callState === "connected" || callState === "hold" || callState === "muted";
  const cs = STATE_DISPLAY[callState];

  // ── Fetch agent profile on mount ──
  useEffect(() => {
    if (IS_DEMO) return;
    async function loadProfile() {
      try {
        const res = await authFetch("/api/agent/me");
        if (res.ok) {
          const data = await res.json();
          if (data.linked && data.profile) {
            setAgentProfile(data.profile);
            setProfileLinked(true);
            if (data.profile.status) setAgentStatus(data.profile.status as AgentStatus);
          } else {
            setProfileLinked(false);
          }
        }
      } catch {}
    }
    loadProfile();
  }, []);

  // ── Fetch real queues ──
  useEffect(() => {
    if (IS_DEMO) return;
    async function loadQueues() {
      try {
        const res = await authFetch("/api/queues/list");
        if (res.ok) {
          const data = await res.json();
          if (data?.length > 0) {
            setQueues(data.map((q: any) => ({
              name: q.name,
              avgHandle: "—",
              sla: 0,
              callers: [],
            })));
          }
        }
      } catch {}
    }
    loadQueues();
    const iv = setInterval(loadQueues, 15000);
    return () => clearInterval(iv);
  }, []);

  // ── SSE for incoming call ring events ──
  useEffect(() => {
    if (IS_DEMO || !agentProfile) return;
    const gw = process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";
    const es = new EventSource(`${gw}/api/agent/me/events`);
    es.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data.type === "ring") {
          setIncomingCall({ call_id: data.call_id, caller: data.caller, queue: data.queue });
        }
      } catch {}
    };
    return () => es.close();
  }, [agentProfile]);

  // ── Timer ──
  useEffect(() => {
    if (!isCallActive) return;
    const iv = setInterval(() => setCallDuration((d) => d + 1), 1000);
    return () => clearInterval(iv);
  }, [isCallActive]);

  // ── Simulate transcript streaming (demo mode only) ──
  useEffect(() => {
    if (!IS_DEMO || callState !== "connected") return;
    const iv = setInterval(() => {
      if (transcriptIdx.current < MOCK_TRANSCRIPT.length) {
        const entry = MOCK_TRANSCRIPT[transcriptIdx.current];
        const now = new Date().toLocaleTimeString("en-US", {
          hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit",
        });
        setTranscriptEntries((prev) => [...prev, { ...entry, time: now }]);
        transcriptIdx.current++;
      } else if (insightIdx.current < MOCK_INSIGHTS.length) {
        setAiInsights((prev) => [...prev, MOCK_INSIGHTS[insightIdx.current]]);
        insightIdx.current++;
      }
    }, 3500);
    return () => clearInterval(iv);
  }, [callState]);

  // ── Simulate voice sentiment (demo mode only) ──
  useEffect(() => {
    if (!IS_DEMO || callState !== "connected") return;
    if (sentimentIdx.current < SENTIMENT_TIMELINE.length) {
      setVoiceSentiment(SENTIMENT_TIMELINE[sentimentIdx.current]);
      sentimentIdx.current++;
    }
  }, [transcriptEntries.length, callState]);

  // ── Simulate copilot suggestions (demo mode only) ──
  useEffect(() => {
    if (!IS_DEMO || callState !== "connected") return;
    const iv = setInterval(() => {
      if (suggestionIdx.current < MOCK_COPILOT.length) {
        const s = MOCK_COPILOT[suggestionIdx.current];
        const now = new Date().toLocaleTimeString("en-US", {
          hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit",
        });
        setCopilotSuggestions((prev) => [...prev, { ...s, time: now }]);
        suggestionIdx.current++;
      }
    }, 5000);
    return () => clearInterval(iv);
  }, [callState]);

  // ── Tick queue wait times (demo mode) ──
  useEffect(() => {
    if (!IS_DEMO) return;
    const iv = setInterval(() => {
      setQueues((prev) =>
        prev.map((q) => ({
          ...q,
          callers: q.callers.map((c) => ({ ...c, waitSec: c.waitSec + 1 })),
        })),
      );
    }, 1000);
    return () => clearInterval(iv);
  }, []);

  // ── Queue SLA fluctuation (demo mode) ──
  useEffect(() => {
    if (!IS_DEMO) return;
    const iv = setInterval(() => {
      setQueues((prev) =>
        prev.map((q) => ({
          ...q,
          sla: Math.min(100, Math.max(50, q.sla + (Math.random() > 0.4 ? 1 : -2))),
        })),
      );
    }, 8000);
    return () => clearInterval(iv);
  }, []);

  // ── Notifications ──
  function showNotif(msg: string, color = "cyan") {
    setNotification({ msg, color });
    setTimeout(() => setNotification(null), 3000);
  }

  // ── Reset call state ──
  function resetCallData() {
    setTranscriptEntries([]);
    setAiInsights([]);
    setVoiceSentiment(null);
    setCopilotSuggestions([]);
    setShowPostCallSummary(false);
    transcriptIdx.current = 0;
    insightIdx.current = 0;
    sentimentIdx.current = 0;
    suggestionIdx.current = 0;
    setConferenceActive(false);
    setConferenceParty(null);
  }

  // ── Sync WebRTC state → local state ──
  useEffect(() => {
    setCallState(webrtc.callState);
    if (webrtc.callId) setCallId(webrtc.callId);
  }, [webrtc.callState, webrtc.callId]);

  // ── SSE stream for real transcripts/copilot when WebRTC call is active ──
  const sseStream = useSSEStream(webrtc.callState === "connected" ? webrtc.callId : null);
  useEffect(() => {
    if (sseStream.transcripts.length > 0) {
      setTranscriptEntries(sseStream.transcripts);
    }
    if (sseStream.suggestions.length > 0) {
      setCopilotSuggestions(sseStream.suggestions);
    }
    if (sseStream.summary?.voice_sentiment) {
      setVoiceSentiment(sseStream.summary.voice_sentiment);
    }
    if (sseStream.summary) {
      setShowPostCallSummary(true);
    }
  }, [sseStream.transcripts, sseStream.suggestions, sseStream.summary]);

  // ── Call actions — use WebRTC for real calls, fallback to mock ──
  async function handleDial() {
    if (!phoneNumber) return;
    setCallDuration(0);
    resetCallData();
    if (IS_DEMO) {
      setCallState("dialing");
      setTimeout(() => setCallState("connected"), 2200);
    } else {
      try {
        await webrtc.dial(phoneNumber, "console-agent");
      } catch {
        setCallState("idle");
      }
    }
  }

  function handleEndCall() {
    setFinalCallDuration(callDuration);
    webrtc.hangup();
    setCallState("disconnected");
    setConferenceActive(false);
    setConferenceParty(null);
    setShowTransfer(false);
    setTimeout(() => {
      setCallState("idle");
      if (transcriptEntries.length > 0) {
        setShowPostCallSummary(true);
      }
    }, 1500);
  }

  function handleMute() {
    if (callState === "muted") {
      webrtc.unmute();
      setCallState("connected");
    } else if (callState === "connected") {
      webrtc.mute();
      setCallState("muted");
    }
  }

  function handleHold() {
    if (callState === "hold") {
      if (callId) resumeCall(callId);
      setCallState("connected");
    } else if (callState === "connected" || callState === "muted") {
      if (callId) holdCall(callId);
      setCallState("hold");
    }
  }

  function handleDigit(d: string) {
    setPhoneNumber((prev) => prev + d);
    if (isCallActive) {
      webrtc.sendDTMF(d);
    }
  }

  function handleBlindTransfer(target: string) {
    showNotif(`Transferred to ${target}`, "cyan");
    setShowTransfer(false);
    handleEndCall();
  }

  function handleAttendedTransfer(target: string) {
    setTransferTarget(target);
    setCallState("hold");
    showNotif(`Consulting ${target}... Customer on hold`, "amber");
    setShowTransfer(false);
    setTimeout(() => {
      showNotif(`Connected with ${target}. Merge or complete transfer.`, "emerald");
    }, 3000);
  }

  function handleConference(agent: TeamMember) {
    setConferenceActive(true);
    setConferenceParty(agent.name);
    showNotif(`${agent.name} joined the conference`, "violet");
  }

  function handleMergeTransfer() {
    if (transferTarget) {
      showNotif(`Call merged with ${transferTarget}`, "emerald");
      setConferenceActive(true);
      setConferenceParty(transferTarget);
      setCallState("connected");
      setTransferTarget(null);
    }
  }

  function handleCompleteTransfer() {
    if (transferTarget) {
      showNotif(`Call transferred to ${transferTarget}`, "cyan");
      setTransferTarget(null);
      handleEndCall();
    }
  }

  function handlePickCall(caller: QueueCaller, queueName: string) {
    if (isCallActive) return;
    setQueues((prev) =>
      prev.map((q) =>
        q.name === queueName
          ? { ...q, callers: q.callers.filter((c) => c.id !== caller.id) }
          : q,
      ),
    );
    setPhoneNumber(caller.number);
    showNotif(`Picked up ${caller.number} from ${queueName} queue — ${caller.reason}`, "emerald");
    setCallState("dialing");
    setCallDuration(0);
    resetCallData();
    setTimeout(() => setCallState("connected"), 1200);
  }

  return (
    <div className="h-screen flex flex-col overflow-hidden bg-[#0a0e1a]">
      {/* ── TOP BAR ── */}
      <header className="h-12 shrink-0 flex items-center justify-between px-4 border-b border-white/[0.05] bg-[#070b14]">
        <div className="flex items-center gap-3">
          <div className="w-7 h-7 rounded-md bg-cyan-500/15 flex items-center justify-center">
            <Hexagon size={14} className="text-cyan-500" strokeWidth={2.5} />
          </div>
          <span className="font-mono text-[11px] font-semibold text-cyan-400 tracking-[0.15em]">
            VOICEAGENT
          </span>
          <div className="w-px h-5 bg-white/[0.06] mx-1" />
          <span className="text-[11px] font-mono text-slate-500">
            {agentProfile ? `${agentProfile.name} — EXT ${agentProfile.extension || "?"}` : "CONSOLE"}
          </span>
        </div>

        <div className="flex items-center gap-3">
          <WebRTCHealth callState={callState} />

          <div className="relative">
            <button
              onClick={() => setShowStatusMenu(!showStatusMenu)}
              className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-[#0f1629] border border-white/[0.06] hover:border-white/[0.1] transition-colors"
            >
              <StatusDot status={agentStatus} size={7} pulse={agentStatus === "Available"} />
              <span className="text-xs font-medium text-slate-300">{agentStatus}</span>
              <ChevronDown size={12} className="text-slate-600" />
            </button>
            {showStatusMenu && (
              <div className="absolute right-0 top-full mt-1 w-40 py-1 rounded-lg bg-[#0f1629] border border-white/[0.08] shadow-xl z-50 animate-[slide-up_0.3s_ease-out]">
                {AGENT_STATUSES.map((s) => (
                  <button
                    key={s}
                    onClick={async () => {
                      setAgentStatus(s); setShowStatusMenu(false);
                      if (!IS_DEMO) await authFetch("/api/agent/me/status", { method: "POST", body: JSON.stringify({ status: s }) });
                    }}
                    className={`w-full flex items-center gap-2 px-3 py-2 text-xs hover:bg-white/[0.04] transition-colors ${agentStatus === s ? "text-cyan-400" : "text-slate-400"}`}
                  >
                    <StatusDot status={s} size={6} />
                    {s}
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className="w-8 h-8 rounded-full bg-gradient-to-br from-cyan-500/20 to-violet-500/20 border border-white/[0.08] flex items-center justify-center">
            <span className="text-[10px] font-mono text-slate-300">KJ</span>
          </div>
        </div>
      </header>

      {/* ── AGENT PROFILE BAR ── */}
      {agentProfile && !IS_DEMO && (
        <div className="h-8 shrink-0 flex items-center justify-between px-4 bg-cyan-500/[0.03] border-b border-cyan-500/[0.06]">
          <div className="flex items-center gap-3 text-[10px] font-mono">
            <span className="text-cyan-400">{agentProfile.name}</span>
            <span className="text-slate-600">EXT {agentProfile.extension || "—"}</span>
            <span className="text-slate-600">{agentProfile.department}</span>
            {agentProfile.queues?.map(q => (
              <span key={q} className="px-1.5 py-0.5 rounded bg-cyan-500/10 text-cyan-400/70 text-[9px]">{q}</span>
            ))}
          </div>
          <div className="flex items-center gap-2 text-[10px] font-mono text-slate-600">
            <span>Priority: {agentProfile.priority || 1}</span>
            <span>Calls: {agentProfile.current_calls}/{agentProfile.max_calls}</span>
          </div>
        </div>
      )}

      {!profileLinked && !IS_DEMO && (
        <div className="h-10 shrink-0 flex items-center justify-center bg-amber-500/10 text-amber-400 text-xs font-mono">
          No agent profile linked to your account. Ask an admin to link your user to an agent profile.
        </div>
      )}

      {/* ── INCOMING CALL RING ── */}
      {incomingCall && callState === "idle" && (
        <div className="shrink-0 flex items-center justify-between px-6 py-3 bg-emerald-500/[0.08] border-b border-emerald-500/20 animate-pulse">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-full bg-emerald-500/20 flex items-center justify-center animate-bounce">
              <Phone size={20} className="text-emerald-400" />
            </div>
            <div>
              <div className="text-sm font-medium text-emerald-300">Incoming Call</div>
              <div className="text-xs font-mono text-slate-400">
                {incomingCall.caller} — {incomingCall.queue} queue
              </div>
            </div>
          </div>
          <div className="flex gap-2">
            <button
              onClick={async () => {
                setCallId(incomingCall.call_id);
                setCallDuration(0);
                resetCallData();
                setIncomingCall(null);
                try {
                  await webrtc.dial(incomingCall.caller, agentProfile?.id);
                } catch {
                  setCallState("connected");
                }
              }}
              className="px-6 py-2 rounded-md bg-emerald-600 hover:bg-emerald-500 text-white font-mono text-xs"
            >
              ACCEPT
            </button>
            <button
              onClick={() => setIncomingCall(null)}
              className="px-6 py-2 rounded-md border border-rose-500/30 text-rose-400 hover:bg-rose-500/10 font-mono text-xs"
            >
              REJECT
            </button>
          </div>
        </div>
      )}

      {/* ── NOTIFICATION BAR ── */}
      {notification && (
        <div
          className={`h-8 shrink-0 flex items-center justify-center text-[11px] font-mono tracking-wide animate-[slide-up_0.3s_ease-out] ${
            notification.color === "emerald" ? "bg-emerald-500/10 text-emerald-400" :
            notification.color === "amber" ? "bg-amber-500/10 text-amber-400" :
            notification.color === "violet" ? "bg-violet-500/10 text-violet-400" :
            notification.color === "rose" ? "bg-rose-500/10 text-rose-400" :
            "bg-cyan-500/10 text-cyan-400"
          }`}
        >
          {notification.msg}
        </div>
      )}

      {/* ── MAIN LAYOUT ── */}
      <div className="flex-1 flex overflow-hidden">
        {/* ═══ LEFT: Dialer + Directory ═══ */}
        <div className="w-72 shrink-0 border-r border-white/[0.05] flex flex-col bg-[#070b14]/50">
          {/* Call state + dialer */}
          <div className="px-4 py-5 border-b border-white/[0.05]">
            <div className="flex items-center justify-between mb-3">
              <ConsoleBadge color={cs.color}>{cs.label}</ConsoleBadge>
              {isCallActive && (
                <span className="text-lg font-mono font-semibold text-slate-200 tabular-nums">
                  {formatTime(callDuration)}
                </span>
              )}
            </div>

            <div className="flex items-center gap-2 mb-3">
              <input
                value={phoneNumber}
                onChange={(e) => setPhoneNumber(e.target.value)}
                placeholder="+1 (555) 000-0000"
                className="flex-1 px-3 py-2.5 rounded-lg bg-[#070b14] border border-white/[0.06] text-base font-mono text-slate-200 placeholder:text-slate-700 tabular-nums focus:outline-none focus:ring-1 focus:ring-cyan-500/30"
                onKeyDown={(e) => e.key === "Enter" && handleDial()}
                disabled={isCallActive}
              />
              <button
                onClick={() => setShowDialpad(!showDialpad)}
                className={`w-10 h-10 rounded-lg border flex items-center justify-center transition-colors ${showDialpad ? "bg-cyan-500/15 border-cyan-500/20 text-cyan-400" : "bg-[#070b14] border-white/[0.06] text-slate-500 hover:text-slate-400"}`}
              >
                <Grid3x3 size={16} />
              </button>
            </div>

            <DialPad visible={showDialpad} onDigit={handleDigit} />

            {conferenceActive && (
              <div className="mb-3 px-2.5 py-2 rounded-lg bg-violet-500/[0.06] border border-violet-500/15 animate-[slide-up_0.3s_ease-out]">
                <div className="flex items-center gap-1.5 mb-1">
                  <Users size={12} strokeWidth={2} className="text-violet-400" />
                  <span className="text-[10px] font-mono text-violet-400 tracking-wider">3-WAY CONFERENCE</span>
                </div>
                <span className="text-xs text-slate-400">{conferenceParty} connected</span>
              </div>
            )}

            {transferTarget && (
              <div className="mb-3 px-2.5 py-2 rounded-lg bg-amber-500/[0.06] border border-amber-500/15 animate-[slide-up_0.3s_ease-out]">
                <div className="text-[10px] font-mono text-amber-400 tracking-wider mb-2">
                  CONSULTING: {transferTarget}
                </div>
                <div className="flex gap-2">
                  <button
                    onClick={handleMergeTransfer}
                    className="flex-1 py-1.5 rounded-md bg-emerald-500/15 text-emerald-400 text-[10px] font-mono hover:bg-emerald-500/20 transition-colors"
                  >
                    MERGE
                  </button>
                  <button
                    onClick={handleCompleteTransfer}
                    className="flex-1 py-1.5 rounded-md bg-cyan-500/15 text-cyan-400 text-[10px] font-mono hover:bg-cyan-500/20 transition-colors"
                  >
                    COMPLETE
                  </button>
                </div>
              </div>
            )}

            {/* Action buttons */}
            <div className="flex items-center justify-between">
              {callState === "idle" || callState === "disconnected" ? (
                <button
                  onClick={handleDial}
                  disabled={!phoneNumber}
                  className={`w-full h-12 rounded-xl font-mono text-sm tracking-wider flex items-center justify-center gap-2 border transition-all active:scale-95 cursor-pointer ${phoneNumber ? "bg-emerald-500/15 text-emerald-400 border-emerald-500/20 hover:bg-emerald-500/25 shadow-[0_0_20px_rgba(16,185,129,0.12)]" : "bg-white/[0.03] text-slate-600 border-white/[0.04] cursor-not-allowed"}`}
                >
                  <Phone size={18} strokeWidth={2} />
                  DIAL
                </button>
              ) : callState === "dialing" ? (
                <button
                  onClick={handleEndCall}
                  className="w-full h-12 rounded-xl bg-rose-500/15 text-rose-400 border border-rose-500/20 font-mono text-sm tracking-wider flex items-center justify-center gap-2 hover:bg-rose-500/25 shadow-[0_0_20px_rgba(244,63,94,0.12)] transition-all active:scale-95 cursor-pointer"
                >
                  <PhoneOff size={18} strokeWidth={2} />
                  CANCEL
                </button>
              ) : (
                <div className="flex gap-2 w-full">
                  <IconBtn
                    Icon={callState === "muted" ? MicOff : Mic}
                    label="MUTE"
                    onClick={handleMute}
                    active={callState === "muted"}
                    color="amber"
                  />
                  <IconBtn
                    Icon={callState === "hold" ? Play : Pause}
                    label={callState === "hold" ? "RESUME" : "HOLD"}
                    onClick={handleHold}
                    active={callState === "hold"}
                    color="amber"
                  />
                  <IconBtn
                    Icon={ArrowRightLeft}
                    label="XFER"
                    onClick={() => setShowTransfer(!showTransfer)}
                    active={showTransfer}
                    color="cyan"
                  />
                  <button
                    onClick={handleEndCall}
                    className="flex-1 h-12 rounded-xl bg-rose-500/15 text-rose-400 border border-rose-500/20 font-mono text-[10px] tracking-wider flex flex-col items-center justify-center gap-0.5 hover:bg-rose-500/25 shadow-[0_0_20px_rgba(244,63,94,0.12)] transition-all active:scale-95 cursor-pointer"
                  >
                    <PhoneOff size={18} strokeWidth={2} />
                    <span>END</span>
                  </button>
                </div>
              )}
            </div>
          </div>

          {/* Transfer panel */}
          <div className="px-3 pt-2">
            <TransferPanel
              visible={showTransfer}
              agents={MOCK_TEAM}
              onBlindTransfer={handleBlindTransfer}
              onAttendedTransfer={handleAttendedTransfer}
              onClose={() => setShowTransfer(false)}
            />
          </div>

          {/* Dialing animation */}
          {callState === "dialing" && (
            <div className="flex-1 flex items-center justify-center">
              <div className="relative">
                <div className="w-16 h-16 rounded-full bg-cyan-500/20 flex items-center justify-center">
                  <PhoneOutgoing size={24} strokeWidth={1.8} className="text-cyan-400" />
                </div>
                <div className="absolute inset-0 rounded-full border-2 border-cyan-400/40 animate-[ring-pulse_1.5s_ease-out_infinite]" />
                <div className="absolute inset-0 rounded-full border-2 border-cyan-400/20 animate-[ring-pulse_1.5s_ease-out_infinite]" style={{ animationDelay: "0.5s" }} />
              </div>
            </div>
          )}

          {/* Agent directory */}
          {callState !== "dialing" && (
            <div className="flex-1 flex flex-col min-h-0 border-t border-white/[0.05]">
              <div className="px-4 py-2.5 flex items-center justify-between">
                <span className="text-[10px] font-mono text-slate-500 tracking-wider">TEAM DIRECTORY</span>
                <span className="text-[10px] font-mono text-slate-700">
                  {MOCK_TEAM.filter((a) => a.status === "Available").length} online
                </span>
              </div>
              <div className="flex-1 overflow-y-auto px-2 pb-2">
                <AgentDirectory
                  agents={MOCK_TEAM}
                  callActive={isCallActive}
                  onTransfer={(a) => handleBlindTransfer(a.name)}
                  onConference={handleConference}
                />
              </div>
            </div>
          )}
        </div>

        {/* ═══ CENTER: Transcript + Copilot ═══ */}
        <div className="flex-1 flex flex-col min-w-0 relative">
          <PostCallSummary
            summary={{ ...MOCK_POST_CALL, duration: finalCallDuration }}
            duration={finalCallDuration}
            visible={showPostCallSummary}
            onDismiss={() => setShowPostCallSummary(false)}
          />

          {/* Header bar */}
          <div className="h-10 shrink-0 flex items-center border-b border-white/[0.05] px-4">
            <div className="flex items-center gap-2">
              <Radio size={13} strokeWidth={1.8} className="text-cyan-400" />
              <span className="text-[10px] font-mono text-cyan-400 tracking-wider">LIVE TRANSCRIPT</span>
            </div>
            <div className="flex items-center gap-2 ml-auto">
              {isCallActive && voiceSentiment && (() => {
                const mood = voiceSentiment.frustration > 0.6 ? "FRUSTRATED" :
                             voiceSentiment.agitation > 0.5 ? "AGITATED" :
                             voiceSentiment.engagement > 0.7 ? "ENGAGED" : "CALM";
                const moodColor = voiceSentiment.frustration > 0.6 ? "rose" :
                                  voiceSentiment.agitation > 0.5 ? "amber" :
                                  voiceSentiment.engagement > 0.7 ? "emerald" : "cyan";
                return <ConsoleBadge color={moodColor}>{mood}</ConsoleBadge>;
              })()}
              {isCallActive && (
                <div className="flex items-center gap-1.5 ml-1">
                  <span className="w-1.5 h-1.5 rounded-full bg-rose-500 animate-pulse" />
                  <span className="text-[9px] font-mono text-slate-500">REC</span>
                </div>
              )}
              <span className="text-[10px] font-mono text-slate-700 ml-1">{transcriptEntries.length} entries</span>
            </div>
          </div>

          {/* Split: Transcript | Copilot */}
          <div className="flex-1 flex overflow-hidden" style={{ minHeight: 0 }}>
            <div className="flex-[3] overflow-hidden p-4 border-r border-white/[0.05]">
              <TranscriptPanel entries={transcriptEntries} insights={aiInsights} />
            </div>
            <div className="flex-[2] overflow-hidden flex flex-col">
              <div className="h-8 shrink-0 flex items-center gap-2 px-4 border-b border-white/[0.05]">
                <Sparkles size={12} strokeWidth={2} className="text-violet-400" />
                <span className="text-[10px] font-mono text-violet-400 tracking-wider">AI COPILOT</span>
                {copilotSuggestions.length > 0 && (
                  <span className="ml-auto text-[9px] font-mono text-slate-600">{copilotSuggestions.length} suggestions</span>
                )}
              </div>
              <div className="flex-1 overflow-hidden p-3">
                <AICopilotPanel suggestions={copilotSuggestions} isActive={isCallActive || copilotSuggestions.length > 0} />
              </div>
            </div>
          </div>

          {/* Notes drawer */}
          <div className={`shrink-0 border-t border-white/[0.05] transition-all duration-300 ${showNotesDrawer ? "h-36" : "h-8"}`}>
            <button
              onClick={() => setShowNotesDrawer(!showNotesDrawer)}
              className="w-full h-8 flex items-center gap-2 px-4 text-[10px] font-mono text-slate-500 hover:text-slate-400 transition-colors"
            >
              <PenLine size={12} strokeWidth={1.8} />
              <span className="tracking-wider">CALL NOTES</span>
              {notes && !showNotesDrawer && <span className="w-1.5 h-1.5 rounded-full bg-cyan-400 ml-1" />}
              <ChevronUp size={12} className={`ml-auto transition-transform duration-200 ${showNotesDrawer ? "rotate-180" : ""}`} />
            </button>
            {showNotesDrawer && (
              <div className="h-[calc(100%-2rem)] animate-[fade-in_0.2s_ease-out]">
                <textarea
                  value={notes}
                  onChange={(e) => setNotes(e.target.value)}
                  placeholder="Type call notes here..."
                  className="w-full h-full px-3 py-2.5 bg-transparent text-sm text-slate-300 placeholder:text-slate-700 leading-relaxed resize-none focus:outline-none"
                />
              </div>
            )}
          </div>
        </div>

        {/* ═══ RIGHT: Mood + Queues + Info ═══ */}
        <div className="w-64 shrink-0 border-l border-white/[0.05] flex flex-col bg-[#070b14]/50">
          <CallerMoodGauge sentiment={voiceSentiment} isActive={isCallActive} />

          <div className="px-3 py-2.5 border-b border-white/[0.05]">
            <div className="flex items-center justify-between">
              <span className="text-[10px] font-mono text-slate-500 tracking-wider">QUEUE MONITOR</span>
              <span className={`text-[10px] font-mono ${queues.reduce((s, q) => s + q.callers.length, 0) > 6 ? "text-rose-400" : "text-emerald-400"}`}>
                {queues.reduce((s, q) => s + q.callers.length, 0)} total
              </span>
            </div>
          </div>
          <div className="px-3 py-3 border-b border-white/[0.05]">
            <QueueMonitor queues={queues} onPickCall={handlePickCall} callActive={isCallActive} />
          </div>

          {isCallActive && (
            <div className="px-3 py-3 border-b border-white/[0.05] animate-[fade-in_0.2s_ease-out]">
              <span className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">CALL INFO</span>
              <div className="space-y-2">
                <div className="flex justify-between text-[11px]">
                  <span className="text-slate-600 font-mono">Number</span>
                  <span className="text-slate-300 font-mono">{phoneNumber}</span>
                </div>
                <div className="flex justify-between text-[11px]">
                  <span className="text-slate-600 font-mono">Duration</span>
                  <span className="text-cyan-400 font-mono">{formatTime(callDuration)}</span>
                </div>
                <div className="flex justify-between text-[11px]">
                  <span className="text-slate-600 font-mono">State</span>
                  <ConsoleBadge color={cs.color}>{cs.label}</ConsoleBadge>
                </div>
                {conferenceActive && (
                  <div className="flex justify-between text-[11px]">
                    <span className="text-slate-600 font-mono">Conference</span>
                    <ConsoleBadge color="violet">{conferenceParty}</ConsoleBadge>
                  </div>
                )}
              </div>
            </div>
          )}

          {aiInsights.length > 0 && (
            <div className="flex-1 px-3 py-3 overflow-y-auto">
              <span className="text-[10px] font-mono text-violet-400 tracking-wider block mb-2">AI SUMMARY</span>
              <div className="space-y-2">
                {aiInsights.map((ins, i) => {
                  const typeLabel: Record<string, string> = { summary: "SUMMARY", action: "ACTION", sentiment: "SENTIMENT", compliance: "COMPLIANCE" };
                  const typeColor: Record<string, string> = { summary: "cyan", action: "emerald", sentiment: "violet", compliance: "amber" };
                  return (
                    <div key={i} className="p-2 rounded-md bg-[#070b14] border border-white/[0.04] animate-[slide-up_0.3s_ease-out]">
                      <ConsoleBadge color={typeColor[ins.type]} className="mb-1">{typeLabel[ins.type]}</ConsoleBadge>
                      <p className="text-[11px] text-slate-400 leading-relaxed mt-1">{ins.text}</p>
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          {!isCallActive && aiInsights.length === 0 && (
            <div className="flex-1 px-3 py-3 flex flex-col justify-end">
              <span className="text-[10px] font-mono text-slate-700 tracking-wider block mb-2">SHORTCUTS</span>
              <div className="space-y-1 text-[10px] font-mono text-slate-700">
                {[
                  ["Enter", "Dial number"],
                  ["M", "Mute / Unmute"],
                  ["H", "Hold / Resume"],
                  ["T", "Transfer panel"],
                  ["E", "End call"],
                ].map(([key, desc]) => (
                  <div key={key} className="flex items-center gap-2">
                    <span className="w-8 text-center py-0.5 rounded bg-white/[0.03] border border-white/[0.05] text-slate-500">
                      {key}
                    </span>
                    <span>{desc}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
