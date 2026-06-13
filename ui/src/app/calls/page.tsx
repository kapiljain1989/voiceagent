"use client";

import { useState, useEffect } from "react";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { authFetch } from "@/lib/auth";

const sentimentBadge: Record<string, string> = {
  positive: "bg-emerald-500/20 text-emerald-400 border-emerald-500/30",
  neutral: "bg-slate-500/20 text-slate-400 border-slate-500/30",
  negative: "bg-rose-500/20 text-rose-400 border-rose-500/30",
};

const modeBadge: Record<string, string> = {
  copilot: "bg-violet-500/20 text-violet-400 border-violet-500/30",
  interactive: "bg-cyan-500/20 text-cyan-400 border-cyan-500/30",
};

function formatDuration(seconds: number) {
  if (!seconds) return "—";
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function formatTime(ts: string) {
  if (!ts) return "—";
  try {
    const d = new Date(ts);
    return d.toLocaleTimeString("en-US", { hour12: false, hour: "2-digit", minute: "2-digit" });
  } catch { return "—"; }
}

function formatDate(ts: string) {
  if (!ts) return "";
  try {
    return new Date(ts).toLocaleDateString("en-US", { month: "short", day: "numeric" });
  } catch { return ""; }
}

interface CallRecord {
  id: string; caller_number: string; called_number: string; mode: string;
  status: string; start_time: string; end_time: string; duration: number;
  summary: string; sentiment: string; llm_model: string;
  action_items?: string[]; commitments?: string[];
  transcript?: Array<{ speaker: string; text: string; timestamp: string }>;
  voice_sentiment?: any;
  robocall_score?: number; robocall_category?: string; pii_detected?: boolean;
}

export default function CallsPage() {
  const [calls, setCalls] = useState<CallRecord[]>([]);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [selectedCall, setSelectedCall] = useState<CallRecord | null>(null);
  const [stats, setStats] = useState<any>(null);

  useEffect(() => {
    loadCalls();
    loadStats();
  }, []);

  async function loadCalls() {
    setLoading(true);
    try {
      const res = await authFetch("/api/calls");
      if (res.ok) {
        const data = await res.json();
        setCalls(data || []);
      }
    } catch {}
    setLoading(false);
  }

  async function loadStats() {
    try {
      const res = await authFetch("/api/stats");
      if (res.ok) setStats(await res.json());
    } catch {}
  }

  async function loadCallDetail(callId: string) {
    try {
      const res = await authFetch(`/api/calls?id=${callId}`);
      if (res.ok) {
        const data = await res.json();
        if (Array.isArray(data) && data.length > 0) {
          setSelectedCall(data[0]);
        } else {
          setSelectedCall(data);
        }
      }
    } catch {}
  }

  const filtered = calls.filter((c) =>
    c.caller_number?.toLowerCase().includes(search.toLowerCase()) ||
    c.summary?.toLowerCase().includes(search.toLowerCase()) ||
    c.sentiment?.toLowerCase().includes(search.toLowerCase()) ||
    c.mode?.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-slate-100">Call History</h1>
          <p className="text-sm text-slate-500 font-mono mt-1">
            {calls.length} calls {stats?.totalToday ? `(${stats.totalToday} today)` : ""}
          </p>
        </div>
        <Button onClick={loadCalls} variant="outline" className="font-mono text-xs text-slate-400 border-cyan-500/20">
          REFRESH
        </Button>
      </div>

      {/* Stats row */}
      {stats && (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
          <Card className="bg-[#0f1629] border-cyan-500/10 p-4 text-center">
            <div className="text-[10px] font-mono text-slate-500 mb-1">TODAY</div>
            <div className="font-mono text-2xl text-cyan-400">{stats.totalToday || 0}</div>
          </Card>
          <Card className="bg-[#0f1629] border-cyan-500/10 p-4 text-center">
            <div className="text-[10px] font-mono text-slate-500 mb-1">AVG DURATION</div>
            <div className="font-mono text-2xl text-cyan-400">{formatDuration(stats.avgDuration || 0)}</div>
          </Card>
          <Card className="bg-[#0f1629] border-emerald-500/10 p-4 text-center">
            <div className="text-[10px] font-mono text-slate-500 mb-1">POSITIVE</div>
            <div className="font-mono text-2xl text-emerald-400">{stats.sentimentBreakdown?.positive || 0}</div>
          </Card>
          <Card className="bg-[#0f1629] border-rose-500/10 p-4 text-center">
            <div className="text-[10px] font-mono text-slate-500 mb-1">NEGATIVE</div>
            <div className="font-mono text-2xl text-rose-400">{stats.sentimentBreakdown?.negative || 0}</div>
          </Card>
        </div>
      )}

      <Input placeholder="Search by caller, summary, sentiment, mode..."
        value={search} onChange={(e) => setSearch(e.target.value)}
        className="max-w-md bg-[#0f1629] border-cyan-500/15 text-slate-200 placeholder:text-slate-600 font-mono text-sm" />

      <div className="flex gap-4">
        {/* Call list */}
        <Card className={`bg-[#0f1629] border-cyan-500/10 glow-border overflow-hidden ${selectedCall ? "flex-1" : "w-full"}`}>
          <table className="w-full text-sm">
            <thead>
              <tr className="text-[11px] font-mono text-slate-500 uppercase tracking-wider border-b border-white/5">
                <th className="text-left px-5 py-3 font-medium">Time</th>
                <th className="text-left px-5 py-3 font-medium">Caller</th>
                <th className="text-left px-5 py-3 font-medium">Duration</th>
                <th className="text-left px-5 py-3 font-medium">Mode</th>
                <th className="text-left px-5 py-3 font-medium">Sentiment</th>
                <th className="text-left px-5 py-3 font-medium">Summary</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((c) => (
                <tr key={c.id}
                  onClick={() => { setSelectedCall(c); loadCallDetail(c.id); }}
                  className={`border-b border-white/[0.03] hover:bg-cyan-500/[0.03] transition-colors cursor-pointer ${selectedCall?.id === c.id ? "bg-cyan-500/[0.06]" : ""}`}>
                  <td className="px-5 py-3.5">
                    <div className="font-mono text-xs text-slate-400">{formatTime(c.start_time)}</div>
                    <div className="font-mono text-[10px] text-slate-600">{formatDate(c.start_time)}</div>
                  </td>
                  <td className="px-5 py-3.5 font-mono text-sm text-slate-300">{c.caller_number || "—"}</td>
                  <td className="px-5 py-3.5 font-mono text-cyan-400">{formatDuration(c.duration)}</td>
                  <td className="px-5 py-3.5">
                    <Badge variant="outline" className={`text-[10px] font-mono uppercase ${modeBadge[c.mode] || "bg-slate-500/20 text-slate-400"}`}>
                      {c.mode}
                    </Badge>
                  </td>
                  <td className="px-5 py-3.5">
                    <Badge variant="outline" className={`text-[10px] font-mono uppercase ${sentimentBadge[c.sentiment] || sentimentBadge.neutral}`}>
                      {c.sentiment || "—"}
                    </Badge>
                  </td>
                  <td className="px-5 py-3.5 text-sm text-slate-500 max-w-[250px] truncate">{c.summary || "—"}</td>
                </tr>
              ))}
              {filtered.length === 0 && (
                <tr>
                  <td colSpan={6} className="px-5 py-8 text-center text-sm text-slate-600">
                    {loading ? "Loading calls..." : calls.length === 0 ? "No call history yet." : "No calls match your search."}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </Card>

        {/* Call detail panel */}
        {selectedCall && (
          <Card className="w-[420px] shrink-0 bg-[#0f1629] border-cyan-500/10 overflow-y-auto max-h-[calc(100vh-16rem)]">
            <div className="px-5 py-4 border-b border-cyan-500/10 flex items-center justify-between">
              <h3 className="text-sm font-semibold text-slate-200 tracking-wide">CALL DETAILS</h3>
              <button onClick={() => setSelectedCall(null)} className="text-slate-600 hover:text-slate-400 text-xs font-mono">CLOSE</button>
            </div>

            <div className="p-5 space-y-4">
              {/* Call info */}
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <div className="text-[10px] font-mono text-slate-500">CALLER</div>
                  <div className="font-mono text-sm text-slate-200">{selectedCall.caller_number || "—"}</div>
                </div>
                <div>
                  <div className="text-[10px] font-mono text-slate-500">DURATION</div>
                  <div className="font-mono text-sm text-cyan-400">{formatDuration(selectedCall.duration)}</div>
                </div>
                <div>
                  <div className="text-[10px] font-mono text-slate-500">MODE</div>
                  <Badge variant="outline" className={`text-[10px] font-mono ${modeBadge[selectedCall.mode] || ""}`}>
                    {selectedCall.mode}
                  </Badge>
                </div>
                <div>
                  <div className="text-[10px] font-mono text-slate-500">SENTIMENT</div>
                  <Badge variant="outline" className={`text-[10px] font-mono ${sentimentBadge[selectedCall.sentiment] || sentimentBadge.neutral}`}>
                    {selectedCall.sentiment || "neutral"}
                  </Badge>
                </div>
                <div>
                  <div className="text-[10px] font-mono text-slate-500">START</div>
                  <div className="font-mono text-xs text-slate-400">
                    {selectedCall.start_time ? new Date(selectedCall.start_time).toLocaleString() : "—"}
                  </div>
                </div>
                <div>
                  <div className="text-[10px] font-mono text-slate-500">LLM</div>
                  <div className="font-mono text-xs text-slate-400">{selectedCall.llm_model || "—"}</div>
                </div>
              </div>

              {/* Security flags */}
              {(selectedCall.robocall_score! > 0 || selectedCall.pii_detected) && (
                <div className="flex gap-2">
                  {selectedCall.robocall_score! > 0 && (
                    <Badge variant="outline" className="text-[10px] font-mono bg-rose-500/15 text-rose-400 border-rose-500/25">
                      ROBOCALL {((selectedCall.robocall_score || 0) * 100).toFixed(0)}%
                    </Badge>
                  )}
                  {selectedCall.pii_detected && (
                    <Badge variant="outline" className="text-[10px] font-mono bg-amber-500/15 text-amber-400 border-amber-500/25">
                      PII DETECTED
                    </Badge>
                  )}
                </div>
              )}

              {/* Summary */}
              {selectedCall.summary && (
                <div className="p-3 rounded bg-amber-500/[0.05] border border-amber-500/15">
                  <div className="text-[10px] font-mono text-amber-400 mb-1">SUMMARY</div>
                  <p className="text-sm text-slate-300">{selectedCall.summary}</p>
                </div>
              )}

              {/* Action items */}
              {selectedCall.action_items && selectedCall.action_items.length > 0 && (
                <div>
                  <div className="text-[10px] font-mono text-slate-500 mb-2">ACTION ITEMS</div>
                  {selectedCall.action_items.map((item, i) => (
                    <div key={i} className="text-sm text-slate-400 pl-3 border-l border-amber-500/30 mb-1">{item}</div>
                  ))}
                </div>
              )}

              {/* Commitments */}
              {selectedCall.commitments && selectedCall.commitments.length > 0 && (
                <div>
                  <div className="text-[10px] font-mono text-slate-500 mb-2">COMMITMENTS</div>
                  {selectedCall.commitments.map((item, i) => (
                    <div key={i} className="text-sm text-slate-400 pl-3 border-l border-cyan-500/30 mb-1">{item}</div>
                  ))}
                </div>
              )}

              {/* Voice sentiment */}
              {selectedCall.voice_sentiment && (
                <div className="p-3 rounded bg-[#070b14] border border-slate-700/50">
                  <div className="text-[10px] font-mono text-cyan-400 tracking-wider mb-2">VOICE SENTIMENT</div>
                  <div className="grid grid-cols-3 gap-2">
                    {[
                      { label: "AGITATION", value: selectedCall.voice_sentiment.agitation, color: selectedCall.voice_sentiment.agitation > 0.5 ? "text-rose-400" : "text-emerald-400" },
                      { label: "FRUSTRATION", value: selectedCall.voice_sentiment.frustration, color: selectedCall.voice_sentiment.frustration > 0.5 ? "text-rose-400" : "text-emerald-400" },
                      { label: "ENGAGEMENT", value: selectedCall.voice_sentiment.engagement, color: selectedCall.voice_sentiment.engagement > 0.5 ? "text-emerald-400" : "text-amber-400" },
                    ].map((s) => (
                      <div key={s.label} className="text-center">
                        <div className="text-[9px] font-mono text-slate-500">{s.label}</div>
                        <div className={`font-mono text-lg ${s.color}`}>{((s.value || 0) * 100).toFixed(0)}%</div>
                      </div>
                    ))}
                  </div>
                  <div className="grid grid-cols-3 gap-2 mt-2 text-[10px] font-mono text-slate-500">
                    <div>Pitch: {selectedCall.voice_sentiment.avg_pitch_hz?.toFixed(0) || "—"} Hz</div>
                    <div>Speed: {selectedCall.voice_sentiment.speaking_rate_wpm?.toFixed(0) || "—"} wpm</div>
                    <div>Energy: {selectedCall.voice_sentiment.energy_trend || "—"}</div>
                  </div>
                </div>
              )}

              {/* Transcript */}
              {selectedCall.transcript && Array.isArray(selectedCall.transcript) && selectedCall.transcript.length > 0 && (
                <div>
                  <div className="text-[10px] font-mono text-slate-500 mb-2">TRANSCRIPT ({selectedCall.transcript.length} utterances)</div>
                  <div className="space-y-2 max-h-60 overflow-y-auto">
                    {selectedCall.transcript.map((t: any, i: number) => (
                      <div key={i} className="text-xs">
                        <span className={`font-mono font-semibold ${t.speaker === "customer" ? "text-cyan-400" : "text-emerald-400"}`}>
                          {t.speaker === "customer" ? "CUSTOMER" : "AGENT"}
                        </span>
                        <p className="text-slate-400 mt-0.5">{t.text}</p>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Call ID */}
              <div className="pt-2 border-t border-slate-800">
                <div className="text-[10px] font-mono text-slate-600">ID: {selectedCall.id}</div>
              </div>
            </div>
          </Card>
        )}
      </div>
    </div>
  );
}
