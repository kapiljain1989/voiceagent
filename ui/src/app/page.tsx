"use client";

import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

const stats = [
  { label: "ACTIVE CALLS", value: "7", delta: "+2", trend: "up", color: "cyan" },
  { label: "TOTAL TODAY", value: "142", delta: "+18%", trend: "up", color: "emerald" },
  { label: "AVG DURATION", value: "4:32", delta: "-0:12", trend: "down", color: "amber" },
  { label: "CSAT SCORE", value: "78%", delta: "+3%", trend: "up", color: "emerald" },
];

const activeCalls = [
  { id: "a1", caller: "+1 (555) 234-8901", agent: "Priya Sharma", duration: "3:45", mode: "copilot", sentiment: "positive" },
  { id: "a2", caller: "+1 (555) 876-5432", agent: "AI Agent", duration: "1:22", mode: "interactive", sentiment: "neutral" },
  { id: "a3", caller: "+1 (555) 111-2233", agent: "Raj Patel", duration: "7:18", mode: "copilot", sentiment: "negative" },
  { id: "a4", caller: "+44 20 7946 0958", agent: "AI Agent", duration: "0:48", mode: "interactive", sentiment: "neutral" },
];

const recentCalls = [
  { id: "r1", time: "14:32", caller: "+1 (555) 999-0001", summary: "Billing inquiry resolved — waived late fee", sentiment: "positive", duration: "5:12" },
  { id: "r2", time: "14:18", caller: "+1 (555) 888-0002", summary: "Technical support — router reset walkthrough", sentiment: "neutral", duration: "8:45" },
  { id: "r3", time: "13:55", caller: "+1 (555) 777-0003", summary: "Service cancellation request — retained with offer", sentiment: "negative", duration: "12:30" },
  { id: "r4", time: "13:41", caller: "+1 (555) 666-0004", summary: "Account upgrade — premium plan activated", sentiment: "positive", duration: "3:20" },
  { id: "r5", time: "13:28", caller: "+1 (555) 555-0005", summary: "Insurance claim status check — pending review", sentiment: "neutral", duration: "4:15" },
];

const sentimentColor: Record<string, string> = {
  positive: "bg-emerald-500/20 text-emerald-400 border-emerald-500/30",
  neutral: "bg-slate-500/20 text-slate-400 border-slate-500/30",
  negative: "bg-rose-500/20 text-rose-400 border-rose-500/30",
};

const modeStyle: Record<string, string> = {
  copilot: "bg-violet-500/20 text-violet-400 border-violet-500/30",
  interactive: "bg-cyan-500/20 text-cyan-400 border-cyan-500/30",
};

export default function Dashboard() {
  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-slate-100">Command Center</h1>
          <p className="text-sm text-slate-500 font-mono mt-1">
            {new Date().toLocaleDateString("en-US", { weekday: "long", year: "numeric", month: "long", day: "numeric" })}
          </p>
        </div>
        <div className="flex items-center gap-2 px-3 py-1.5 rounded-md bg-emerald-500/10 border border-emerald-500/20">
          <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
          <span className="text-xs font-mono text-emerald-400">ALL SYSTEMS NOMINAL</span>
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
        {stats.map((s) => (
          <Card key={s.label} className="bg-[#0f1629] border-cyan-500/10 p-5 glow-border">
            <div className="flex items-center justify-between mb-3">
              <span className="text-[11px] font-mono font-medium tracking-widest text-slate-500">{s.label}</span>
              <span className={`text-xs font-mono ${s.trend === "up" ? "text-emerald-400" : "text-amber-400"}`}>
                {s.delta}
              </span>
            </div>
            <div className="stat-number text-3xl font-mono font-semibold text-slate-100">{s.value}</div>
          </Card>
        ))}
      </div>

      {/* Active Calls */}
      <Card className="bg-[#0f1629] border-cyan-500/10 glow-border overflow-hidden">
        <div className="px-5 py-4 border-b border-cyan-500/10 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <span className="w-2 h-2 rounded-full bg-cyan-400 animate-pulse-glow" />
            <h2 className="text-sm font-semibold text-slate-200 tracking-wide">ACTIVE CALLS</h2>
          </div>
          <span className="text-xs font-mono text-slate-500">{activeCalls.length} channels</span>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-[11px] font-mono text-slate-500 uppercase tracking-wider border-b border-white/5">
                <th className="text-left px-5 py-3 font-medium">Caller</th>
                <th className="text-left px-5 py-3 font-medium">Agent</th>
                <th className="text-left px-5 py-3 font-medium">Duration</th>
                <th className="text-left px-5 py-3 font-medium">Mode</th>
                <th className="text-left px-5 py-3 font-medium">Sentiment</th>
              </tr>
            </thead>
            <tbody>
              {activeCalls.map((c) => (
                <tr key={c.id} className="border-b border-white/[0.03] hover:bg-cyan-500/[0.03] transition-colors">
                  <td className="px-5 py-3 font-mono text-sm text-slate-300">{c.caller}</td>
                  <td className="px-5 py-3 text-slate-300">{c.agent}</td>
                  <td className="px-5 py-3 font-mono text-cyan-400">{c.duration}</td>
                  <td className="px-5 py-3">
                    <Badge variant="outline" className={`text-[10px] font-mono uppercase ${modeStyle[c.mode]}`}>
                      {c.mode}
                    </Badge>
                  </td>
                  <td className="px-5 py-3">
                    <Badge variant="outline" className={`text-[10px] font-mono uppercase ${sentimentColor[c.sentiment]}`}>
                      {c.sentiment}
                    </Badge>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>

      {/* Recent Calls */}
      <Card className="bg-[#0f1629] border-cyan-500/10 glow-border">
        <div className="px-5 py-4 border-b border-cyan-500/10">
          <h2 className="text-sm font-semibold text-slate-200 tracking-wide">RECENT COMPLETIONS</h2>
        </div>
        <div className="divide-y divide-white/[0.03]">
          {recentCalls.map((c) => (
            <div key={c.id} className="px-5 py-3.5 flex items-start gap-4 hover:bg-cyan-500/[0.02] transition-colors cursor-pointer">
              <span className="font-mono text-xs text-slate-500 mt-0.5 w-12 shrink-0">{c.time}</span>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1">
                  <span className="font-mono text-sm text-slate-300">{c.caller}</span>
                  <Badge variant="outline" className={`text-[10px] font-mono ${sentimentColor[c.sentiment]}`}>
                    {c.sentiment}
                  </Badge>
                </div>
                <p className="text-sm text-slate-500 truncate">{c.summary}</p>
              </div>
              <span className="font-mono text-xs text-slate-600 shrink-0">{c.duration}</span>
            </div>
          ))}
        </div>
      </Card>
    </div>
  );
}
