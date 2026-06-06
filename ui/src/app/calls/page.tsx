"use client";

import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import Link from "next/link";

const mockCalls = [
  { id: "c1", time: "14:32", date: "2026-06-06", caller: "+1 (555) 999-0001", agent: "Priya Sharma", duration: 312, sentiment: "positive", mode: "copilot", summary: "Billing inquiry resolved — waived late fee" },
  { id: "c2", time: "14:18", date: "2026-06-06", caller: "+1 (555) 888-0002", agent: "AI Agent", duration: 525, sentiment: "neutral", mode: "interactive", summary: "Technical support — router reset walkthrough" },
  { id: "c3", time: "13:55", date: "2026-06-06", caller: "+1 (555) 777-0003", agent: "Raj Patel", duration: 750, sentiment: "negative", mode: "copilot", summary: "Service cancellation request — retained with offer" },
  { id: "c4", time: "13:41", date: "2026-06-06", caller: "+1 (555) 666-0004", agent: "AI Agent", duration: 200, sentiment: "positive", mode: "interactive", summary: "Account upgrade — premium plan activated" },
  { id: "c5", time: "13:28", date: "2026-06-06", caller: "+1 (555) 555-0005", agent: "Meera Joshi", duration: 255, sentiment: "neutral", mode: "copilot", summary: "Insurance claim status check — pending review" },
  { id: "c6", time: "12:45", date: "2026-06-06", caller: "+1 (555) 444-0006", agent: "AI Agent", duration: 180, sentiment: "positive", mode: "interactive", summary: "Password reset and account verification" },
  { id: "c7", time: "12:12", date: "2026-06-06", caller: "+44 20 7946 0123", agent: "Anita Desai", duration: 420, sentiment: "positive", mode: "copilot", summary: "International plan upgrade with roaming add-on" },
  { id: "c8", time: "11:50", date: "2026-06-06", caller: "+1 (555) 333-0008", agent: "Vikram Singh", duration: 890, sentiment: "negative", mode: "copilot", summary: "Disputed charge escalation — supervisor callback scheduled" },
];

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
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

export default function CallsPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-slate-100">Call History</h1>
          <p className="text-sm text-slate-500 font-mono mt-1">{mockCalls.length} calls today</p>
        </div>
        <Link href="/calls/live">
          <button className="flex items-center gap-2 px-4 py-2 rounded-md bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-mono tracking-wider transition-colors">
            <span className="w-2 h-2 rounded-full bg-white animate-pulse" />
            LIVE OPS
          </button>
        </Link>
      </div>

      <Input
        placeholder="Search calls..."
        className="max-w-md bg-[#0f1629] border-cyan-500/15 text-slate-200 placeholder:text-slate-600 font-mono text-sm"
      />

      <Card className="bg-[#0f1629] border-cyan-500/10 glow-border overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-[11px] font-mono text-slate-500 uppercase tracking-wider border-b border-white/5">
              <th className="text-left px-5 py-3 font-medium">Time</th>
              <th className="text-left px-5 py-3 font-medium">Caller</th>
              <th className="text-left px-5 py-3 font-medium">Agent</th>
              <th className="text-left px-5 py-3 font-medium">Duration</th>
              <th className="text-left px-5 py-3 font-medium">Mode</th>
              <th className="text-left px-5 py-3 font-medium">Sentiment</th>
              <th className="text-left px-5 py-3 font-medium">Summary</th>
            </tr>
          </thead>
          <tbody>
            {mockCalls.map((c) => (
              <tr key={c.id} className="border-b border-white/[0.03] hover:bg-cyan-500/[0.03] transition-colors cursor-pointer">
                <td className="px-5 py-3.5 font-mono text-xs text-slate-500">{c.time}</td>
                <td className="px-5 py-3.5 font-mono text-sm text-slate-300">{c.caller}</td>
                <td className="px-5 py-3.5 text-slate-300">{c.agent}</td>
                <td className="px-5 py-3.5 font-mono text-cyan-400">{formatDuration(c.duration)}</td>
                <td className="px-5 py-3.5">
                  <Badge variant="outline" className={`text-[10px] font-mono uppercase ${modeBadge[c.mode]}`}>
                    {c.mode}
                  </Badge>
                </td>
                <td className="px-5 py-3.5">
                  <Badge variant="outline" className={`text-[10px] font-mono uppercase ${sentimentBadge[c.sentiment]}`}>
                    {c.sentiment}
                  </Badge>
                </td>
                <td className="px-5 py-3.5 text-sm text-slate-500 max-w-[300px] truncate">{c.summary}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>
    </div>
  );
}
