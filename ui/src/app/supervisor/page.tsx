"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { authFetch } from "@/lib/auth";
import { useSSEStream } from "@/hooks/useConsoleApi";

const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";

interface ActiveCall {
  call_id: string;
  started_at: string;
  duration: number;
  caller: string;
  agent: string;
  monitored: boolean;
  utterance_count: number;
  last_utterance?: { speaker: string; text: string };
  voice_sentiment?: {
    agitation: number;
    frustration: number;
    engagement: number;
    sentiment: string;
    confidence: number;
    avg_pitch_hz: number;
    speaking_rate_wpm: number;
  };
}

interface QueueInfo {
  name: string;
  callers: { id: string; call_id: string; number: string }[];
}

type MonitorMode = "listen" | "whisper" | "barge";

export default function SupervisorPage() {
  const [calls, setCalls] = useState<ActiveCall[]>([]);
  const [queues, setQueues] = useState<QueueInfo[]>([]);
  const [monitoringCallId, setMonitoringCallId] = useState<string | null>(null);
  const [monitorMode, setMonitorMode] = useState<MonitorMode | null>(null);
  const pcRef = useRef<RTCPeerConnection | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const remoteAudioRef = useRef<HTMLAudioElement | null>(null);

  const sseStream = useSSEStream(monitoringCallId);

  // Fetch active calls
  useEffect(() => {
    const load = async () => {
      try {
        const res = await authFetch("/api/supervisor/calls");
        if (res.ok) setCalls(await res.json());
      } catch {}
    };
    load();
    const interval = setInterval(load, 3000);
    return () => clearInterval(interval);
  }, []);

  // Fetch queues
  useEffect(() => {
    const load = async () => {
      try {
        const res = await authFetch("/api/queues");
        if (res.ok) setQueues(await res.json());
      } catch {}
    };
    load();
    const interval = setInterval(load, 5000);
    return () => clearInterval(interval);
  }, []);

  const startMonitor = useCallback(async (callId: string, mode: MonitorMode) => {
    // Stop existing session
    if (pcRef.current) {
      pcRef.current.close();
      pcRef.current = null;
    }
    if (streamRef.current) {
      streamRef.current.getTracks().forEach(t => t.stop());
      streamRef.current = null;
    }

    if (mode === "barge") {
      try {
        const res = await authFetch("/api/supervisor/monitor", {
          method: "POST",
          body: JSON.stringify({ call_id: callId, mode: "barge" }),
        });
        if (res.ok) {
          setMonitoringCallId(callId);
          setMonitorMode("barge");
        }
      } catch {}
      return;
    }

    // Listen or Whisper — need WebRTC
    try {
      const stream = mode === "whisper"
        ? await navigator.mediaDevices.getUserMedia({ audio: true })
        : undefined;
      if (stream) streamRef.current = stream;

      const pc = new RTCPeerConnection({
        iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
      });
      pcRef.current = pc;

      if (stream) {
        stream.getAudioTracks().forEach(t => pc.addTrack(t, stream));
      }

      pc.ontrack = (event) => {
        if (remoteAudioRef.current && event.streams[0]) {
          remoteAudioRef.current.srcObject = event.streams[0];
        }
      };

      pc.oniceconnectionstatechange = () => {
        if (pc.iceConnectionState === "failed" || pc.iceConnectionState === "closed") {
          stopMonitor();
        }
      };

      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);
      await new Promise<void>(resolve => {
        if (pc.iceGatheringState === "complete") { resolve(); return; }
        pc.onicegatheringstatechange = () => {
          if (pc.iceGatheringState === "complete") resolve();
        };
      });

      const res = await authFetch("/api/supervisor/monitor", {
        method: "POST",
        body: JSON.stringify({
          call_id: callId,
          mode,
          sdp: pc.localDescription?.sdp,
        }),
      });

      if (!res.ok) {
        pc.close();
        return;
      }

      const data = await res.json();
      await pc.setRemoteDescription({ type: "answer", sdp: data.sdp });

      setMonitoringCallId(callId);
      setMonitorMode(mode);
    } catch (e) {
      console.error("monitor failed", e);
    }
  }, []);

  const stopMonitor = useCallback(async () => {
    if (monitoringCallId) {
      try {
        await authFetch("/api/supervisor/stop", {
          method: "POST",
          body: JSON.stringify({ call_id: monitoringCallId }),
        });
      } catch {}
    }
    if (pcRef.current) {
      pcRef.current.close();
      pcRef.current = null;
    }
    if (streamRef.current) {
      streamRef.current.getTracks().forEach(t => t.stop());
      streamRef.current = null;
    }
    setMonitoringCallId(null);
    setMonitorMode(null);
  }, [monitoringCallId]);

  const totalWaiting = queues.reduce((s, q) => s + (q.callers?.length || 0), 0);
  const monitoredCall = calls.find(c => c.call_id === monitoringCallId);

  function sentimentBadge(s?: ActiveCall["voice_sentiment"]) {
    if (!s) return <span className="text-[10px] font-mono text-slate-600">--</span>;
    const color = s.frustration > 0.6 ? "rose" : s.agitation > 0.5 ? "amber" : s.engagement > 0.7 ? "emerald" : "cyan";
    const label = s.frustration > 0.6 ? "FRUSTRATED" : s.agitation > 0.5 ? "AGITATED" : s.engagement > 0.7 ? "ENGAGED" : "CALM";
    const colorClass: Record<string, string> = {
      rose: "bg-rose-500/10 text-rose-400 border-rose-500/20",
      amber: "bg-amber-500/10 text-amber-400 border-amber-500/20",
      emerald: "bg-emerald-500/10 text-emerald-400 border-emerald-500/20",
      cyan: "bg-cyan-500/10 text-cyan-400 border-cyan-500/20",
    };
    return (
      <span className={`text-[9px] font-mono px-1.5 py-0.5 rounded border ${colorClass[color]}`}>
        {label}
      </span>
    );
  }

  function formatDuration(sec: number) {
    const m = Math.floor(sec / 60);
    const s = sec % 60;
    return `${m}:${s.toString().padStart(2, "0")}`;
  }

  return (
    <div className="flex h-full gap-4 p-6">
      {/* Hidden audio element */}
      <audio ref={remoteAudioRef} autoPlay />

      {/* ═══ LEFT: Queue + Agent Summary ═══ */}
      <div className="w-64 shrink-0 space-y-4">
        {/* Queue overview */}
        <div className="rounded-lg bg-[#0a0f1a] border border-white/[0.06] p-4">
          <h3 className="text-[10px] font-mono text-slate-500 tracking-wider mb-3">QUEUE OVERVIEW</h3>
          <div className="space-y-2">
            {queues.map(q => (
              <div key={q.name} className="flex items-center justify-between">
                <span className="text-xs font-mono text-slate-400">{q.name}</span>
                <span className={`text-xs font-mono font-semibold ${(q.callers?.length || 0) > 3 ? "text-rose-400" : (q.callers?.length || 0) > 0 ? "text-amber-400" : "text-emerald-400"}`}>
                  {q.callers?.length || 0}
                </span>
              </div>
            ))}
          </div>
          <div className="mt-3 pt-3 border-t border-white/[0.04] flex items-center justify-between">
            <span className="text-[10px] font-mono text-slate-600">Total Waiting</span>
            <span className="text-xs font-mono font-semibold text-slate-300">{totalWaiting}</span>
          </div>
        </div>

        {/* Stats */}
        <div className="rounded-lg bg-[#0a0f1a] border border-white/[0.06] p-4">
          <h3 className="text-[10px] font-mono text-slate-500 tracking-wider mb-3">LIVE STATS</h3>
          <div className="space-y-2">
            <div className="flex justify-between">
              <span className="text-xs font-mono text-slate-500">Active Calls</span>
              <span className="text-xs font-mono font-semibold text-cyan-400">{calls.length}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-xs font-mono text-slate-500">Frustrated</span>
              <span className="text-xs font-mono font-semibold text-rose-400">
                {calls.filter(c => c.voice_sentiment && c.voice_sentiment.frustration > 0.6).length}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-xs font-mono text-slate-500">Avg Duration</span>
              <span className="text-xs font-mono text-slate-300">
                {calls.length > 0 ? formatDuration(Math.round(calls.reduce((s, c) => s + c.duration, 0) / calls.length)) : "--"}
              </span>
            </div>
          </div>
        </div>

        {/* Monitor panel */}
        {monitoredCall && (
          <div className="rounded-lg bg-violet-500/[0.06] border border-violet-500/15 p-4">
            <div className="flex items-center gap-2 mb-3">
              <span className="w-2 h-2 rounded-full bg-violet-500 animate-pulse" />
              <span className="text-[10px] font-mono text-violet-400 tracking-wider">
                {monitorMode === "listen" ? "LISTENING" : monitorMode === "whisper" ? "WHISPERING" : "BARGED IN"}
              </span>
            </div>
            <div className="text-xs font-mono text-slate-300 mb-1">{monitoredCall.caller}</div>
            <div className="text-[10px] font-mono text-slate-500 mb-3">Agent: {monitoredCall.agent}</div>

            <div className="flex gap-2 mb-3">
              {monitorMode !== "listen" && (
                <button onClick={() => startMonitor(monitoredCall.call_id, "listen")} className="flex-1 px-2 py-1.5 text-[9px] font-mono rounded bg-cyan-500/10 text-cyan-400 border border-cyan-500/20 hover:bg-cyan-500/20 transition-colors">
                  LISTEN
                </button>
              )}
              {monitorMode !== "whisper" && (
                <button onClick={() => startMonitor(monitoredCall.call_id, "whisper")} className="flex-1 px-2 py-1.5 text-[9px] font-mono rounded bg-amber-500/10 text-amber-400 border border-amber-500/20 hover:bg-amber-500/20 transition-colors">
                  WHISPER
                </button>
              )}
              {monitorMode !== "barge" && (
                <button onClick={() => startMonitor(monitoredCall.call_id, "barge")} className="flex-1 px-2 py-1.5 text-[9px] font-mono rounded bg-rose-500/10 text-rose-400 border border-rose-500/20 hover:bg-rose-500/20 transition-colors">
                  BARGE
                </button>
              )}
            </div>
            <button onClick={stopMonitor} className="w-full px-2 py-1.5 text-[9px] font-mono rounded bg-slate-500/10 text-slate-400 border border-slate-500/20 hover:bg-slate-500/20 transition-colors">
              STOP MONITORING
            </button>
          </div>
        )}
      </div>

      {/* ═══ CENTER: Active Calls Grid ═══ */}
      <div className="flex-1 flex flex-col min-w-0">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-sm font-mono text-slate-300 tracking-wider">ACTIVE CALLS</h2>
          <span className="text-[10px] font-mono text-slate-600">{calls.length} calls in progress</span>
        </div>

        {calls.length === 0 ? (
          <div className="flex-1 flex items-center justify-center">
            <div className="text-center">
              <div className="text-lg font-mono text-slate-700 mb-2">No active calls</div>
              <div className="text-xs font-mono text-slate-600">Calls will appear here when agents are on calls</div>
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
            {calls.map(call => {
              const isMonitored = call.call_id === monitoringCallId;
              return (
                <div
                  key={call.call_id}
                  className={`rounded-lg p-4 border transition-colors ${
                    isMonitored
                      ? "bg-violet-500/[0.06] border-violet-500/20"
                      : "bg-[#0a0f1a] border-white/[0.06] hover:border-white/[0.12]"
                  }`}
                >
                  {/* Header */}
                  <div className="flex items-center justify-between mb-3">
                    <div className="flex items-center gap-2">
                      <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
                      <span className="text-xs font-mono text-slate-300 font-semibold">
                        {call.caller || "Unknown"}
                      </span>
                    </div>
                    <span className="text-[10px] font-mono text-slate-500">{formatDuration(call.duration)}</span>
                  </div>

                  {/* Agent + Sentiment */}
                  <div className="flex items-center justify-between mb-3">
                    <span className="text-[10px] font-mono text-slate-500">Agent: {call.agent || "--"}</span>
                    {sentimentBadge(call.voice_sentiment)}
                  </div>

                  {/* Voice metrics */}
                  {call.voice_sentiment && (
                    <div className="grid grid-cols-3 gap-2 mb-3 text-[9px] font-mono">
                      <div>
                        <div className="text-slate-600">FRS</div>
                        <div className={call.voice_sentiment.frustration > 0.5 ? "text-rose-400" : "text-slate-400"}>
                          {(call.voice_sentiment.frustration * 100).toFixed(0)}%
                        </div>
                      </div>
                      <div>
                        <div className="text-slate-600">AGT</div>
                        <div className={call.voice_sentiment.agitation > 0.5 ? "text-amber-400" : "text-slate-400"}>
                          {(call.voice_sentiment.agitation * 100).toFixed(0)}%
                        </div>
                      </div>
                      <div>
                        <div className="text-slate-600">ENG</div>
                        <div className={call.voice_sentiment.engagement > 0.6 ? "text-emerald-400" : "text-slate-400"}>
                          {(call.voice_sentiment.engagement * 100).toFixed(0)}%
                        </div>
                      </div>
                    </div>
                  )}

                  {/* Last utterance */}
                  {call.last_utterance && (
                    <div className="mb-3 px-2 py-1.5 rounded bg-white/[0.02] border border-white/[0.04]">
                      <span className={`text-[9px] font-mono ${call.last_utterance.speaker === "customer" ? "text-cyan-500" : "text-emerald-500"}`}>
                        {call.last_utterance.speaker}:
                      </span>
                      <span className="text-[10px] text-slate-400 ml-1">
                        {call.last_utterance.text.length > 60 ? call.last_utterance.text.slice(0, 60) + "..." : call.last_utterance.text}
                      </span>
                    </div>
                  )}

                  {/* Action buttons */}
                  <div className="flex gap-1.5">
                    <button
                      onClick={() => startMonitor(call.call_id, "listen")}
                      disabled={isMonitored && monitorMode === "listen"}
                      title="Listen"
                      className="flex-1 py-1.5 text-[9px] font-mono rounded bg-cyan-500/10 text-cyan-400 border border-cyan-500/20 hover:bg-cyan-500/20 disabled:opacity-30 transition-colors"
                    >
                      LISTEN
                    </button>
                    <button
                      onClick={() => startMonitor(call.call_id, "whisper")}
                      disabled={isMonitored && monitorMode === "whisper"}
                      title="Whisper"
                      className="flex-1 py-1.5 text-[9px] font-mono rounded bg-amber-500/10 text-amber-400 border border-amber-500/20 hover:bg-amber-500/20 disabled:opacity-30 transition-colors"
                    >
                      WHISPER
                    </button>
                    <button
                      onClick={() => startMonitor(call.call_id, "barge")}
                      disabled={isMonitored && monitorMode === "barge"}
                      title="Barge"
                      className="flex-1 py-1.5 text-[9px] font-mono rounded bg-rose-500/10 text-rose-400 border border-rose-500/20 hover:bg-rose-500/20 disabled:opacity-30 transition-colors"
                    >
                      BARGE
                    </button>
                  </div>

                  {/* Monitoring indicator */}
                  {isMonitored && (
                    <div className="mt-2 flex items-center gap-1.5">
                      <span className="w-1.5 h-1.5 rounded-full bg-violet-500 animate-pulse" />
                      <span className="text-[9px] font-mono text-violet-400">
                        {monitorMode === "listen" ? "Listening" : monitorMode === "whisper" ? "Whispering" : "Barged"}
                      </span>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* ═══ RIGHT: Live Transcript (when monitoring) ═══ */}
      {monitoringCallId && (
        <div className="w-80 shrink-0 rounded-lg bg-[#0a0f1a] border border-white/[0.06] flex flex-col">
          <div className="px-4 py-3 border-b border-white/[0.05]">
            <span className="text-[10px] font-mono text-slate-500 tracking-wider">LIVE TRANSCRIPT</span>
          </div>
          <div className="flex-1 overflow-y-auto px-4 py-2 space-y-2">
            {sseStream.transcripts.length === 0 ? (
              <div className="text-[10px] font-mono text-slate-700 text-center py-8">Waiting for speech...</div>
            ) : (
              sseStream.transcripts.map((t, i) => (
                <div key={i} className="text-xs">
                  <span className={`text-[9px] font-mono font-semibold ${t.speaker === "customer" ? "text-cyan-500" : "text-emerald-500"}`}>
                    {t.speaker}
                  </span>
                  <span className="text-slate-400 ml-1">{t.text}</span>
                  <span className="text-[9px] text-slate-700 ml-1">{t.time}</span>
                </div>
              ))
            )}
          </div>
          {sseStream.suggestions.length > 0 && (
            <div className="px-4 py-2 border-t border-white/[0.05]">
              <span className="text-[9px] font-mono text-amber-500">AI SUGGESTION:</span>
              <p className="text-[10px] text-slate-400 mt-1">
                {sseStream.suggestions[sseStream.suggestions.length - 1].text}
              </p>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
