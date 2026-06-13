"use client";

import { useState, useEffect } from "react";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { authFetch } from "@/lib/auth";

const sentimentColor: Record<string, string> = {
  positive: "bg-emerald-500/20 text-emerald-400 border-emerald-500/30",
  neutral: "bg-slate-500/20 text-slate-400 border-slate-500/30",
  negative: "bg-rose-500/20 text-rose-400 border-rose-500/30",
};
const modeStyle: Record<string, string> = {
  copilot: "bg-violet-500/20 text-violet-400 border-violet-500/30",
  interactive: "bg-cyan-500/20 text-cyan-400 border-cyan-500/30",
};

function formatDuration(s: number) {
  if (!s) return "—";
  const m = Math.floor(s / 60);
  const sec = s % 60;
  return `${m}:${String(sec).padStart(2, "0")}`;
}

export default function Dashboard() {
  const [stats, setStats] = useState<any>(null);
  const [activeSessions, setActiveSessions] = useState<any[]>([]);
  const [recentCalls, setRecentCalls] = useState<any[]>([]);
  const [agents, setAgents] = useState<any[]>([]);
  const [services, setServices] = useState<any[]>([]);
  const [config, setConfig] = useState<any>(null);

  useEffect(() => {
    loadAll();
    const iv = setInterval(loadAll, 10000);
    return () => clearInterval(iv);
  }, []);

  async function loadAll() {
    try { const r = await authFetch("/api/stats"); if (r.ok) setStats(await r.json()); } catch {}
    try { const r = await authFetch("/api/copilot/active"); if (r.ok) setActiveSessions(await r.json()); } catch {}
    try { const r = await authFetch("/api/calls"); if (r.ok) { const d = await r.json(); setRecentCalls((d || []).slice(0, 8)); } } catch {}
    try { const r = await authFetch("/api/agents"); if (r.ok) setAgents(await r.json()); } catch {}
    try { const r = await authFetch("/api/services/status"); if (r.ok) setServices(await r.json()); } catch {}
    try { const r = await authFetch("/api/config"); if (r.ok) setConfig(await r.json()); } catch {}
  }

  const onlineAgents = agents.filter((a: any) => a.status === "Available" || a.status === "available");
  const onlineServices = services.filter((s: any) => s.status === "online");

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
        <div className="flex items-center gap-3">
          {config && (
            <Badge variant="outline" className="font-mono text-[10px] bg-cyan-500/10 text-cyan-400 border-cyan-500/20">
              {config.mode?.toUpperCase()} MODE
            </Badge>
          )}
          <div className={`flex items-center gap-2 px-3 py-1.5 rounded-md ${onlineServices.length === services.length && services.length > 0 ? "bg-emerald-500/10 border border-emerald-500/20" : "bg-amber-500/10 border border-amber-500/20"}`}>
            <span className={`w-2 h-2 rounded-full ${onlineServices.length === services.length && services.length > 0 ? "bg-emerald-500" : "bg-amber-500"} animate-pulse`} />
            <span className={`text-xs font-mono ${onlineServices.length === services.length && services.length > 0 ? "text-emerald-400" : "text-amber-400"}`}>
              {onlineServices.length}/{services.length} SERVICES
            </span>
          </div>
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-6 gap-3">
        <Card className="bg-[#0f1629] border-cyan-500/10 p-4 glow-border">
          <div className="text-[10px] font-mono text-slate-500 tracking-wider mb-2">ACTIVE CALLS</div>
          <div className="font-mono text-3xl font-semibold text-cyan-400">{activeSessions.length}</div>
        </Card>
        <Card className="bg-[#0f1629] border-cyan-500/10 p-4 glow-border">
          <div className="text-[10px] font-mono text-slate-500 tracking-wider mb-2">TODAY</div>
          <div className="font-mono text-3xl font-semibold text-slate-100">{stats?.totalToday || 0}</div>
        </Card>
        <Card className="bg-[#0f1629] border-cyan-500/10 p-4 glow-border">
          <div className="text-[10px] font-mono text-slate-500 tracking-wider mb-2">AVG DURATION</div>
          <div className="font-mono text-3xl font-semibold text-slate-100">{formatDuration(stats?.avgDuration || 0)}</div>
        </Card>
        <Card className="bg-[#0f1629] border-emerald-500/10 p-4">
          <div className="text-[10px] font-mono text-slate-500 tracking-wider mb-2">POSITIVE</div>
          <div className="font-mono text-3xl font-semibold text-emerald-400">{stats?.sentimentBreakdown?.positive || 0}</div>
        </Card>
        <Card className="bg-[#0f1629] border-rose-500/10 p-4">
          <div className="text-[10px] font-mono text-slate-500 tracking-wider mb-2">NEGATIVE</div>
          <div className="font-mono text-3xl font-semibold text-rose-400">{stats?.sentimentBreakdown?.negative || 0}</div>
        </Card>
        <Card className="bg-[#0f1629] border-violet-500/10 p-4">
          <div className="text-[10px] font-mono text-slate-500 tracking-wider mb-2">AGENTS ONLINE</div>
          <div className="font-mono text-3xl font-semibold text-violet-400">{onlineAgents.length}<span className="text-lg text-slate-600">/{agents.length}</span></div>
        </Card>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        {/* Active Sessions */}
        <Card className="lg:col-span-2 bg-[#0f1629] border-cyan-500/10 glow-border overflow-hidden">
          <div className="px-5 py-3.5 border-b border-cyan-500/10 flex items-center justify-between">
            <div className="flex items-center gap-2">
              <span className="w-2 h-2 rounded-full bg-cyan-400 animate-pulse-glow" />
              <h2 className="text-xs font-mono font-semibold text-slate-300 tracking-wider">ACTIVE SESSIONS</h2>
            </div>
            <span className="text-[10px] font-mono text-slate-500">{activeSessions.length} live</span>
          </div>
          {activeSessions.length > 0 ? (
            <div className="divide-y divide-white/[0.03]">
              {activeSessions.map((s: any, i: number) => {
                const vs = s.voice_sentiment;
                return (
                  <div key={i} className="px-5 py-3 flex items-center justify-between hover:bg-cyan-500/[0.02]">
                    <div className="flex items-center gap-3">
                      <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
                      <div>
                        <span className="font-mono text-sm text-cyan-400">{s.caller || s.call_id?.slice(0, 12) || "—"}</span>
                        {s.agent && <span className="text-xs text-slate-500 ml-2">→ {s.agent}</span>}
                      </div>
                    </div>
                    <div className="flex items-center gap-3">
                      <span className="font-mono text-xs text-slate-500">{s.duration}s</span>
                      {vs && (
                        <Badge variant="outline" className={`text-[9px] font-mono ${vs.frustration > 0.5 ? "bg-rose-500/15 text-rose-400" : vs.agitation > 0.4 ? "bg-amber-500/15 text-amber-400" : "bg-emerald-500/15 text-emerald-400"}`}>
                          {vs.frustration > 0.5 ? "FRUSTRATED" : vs.agitation > 0.4 ? "AGITATED" : "CALM"}
                        </Badge>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          ) : (
            <div className="px-5 py-8 text-center text-sm text-slate-600">No active calls</div>
          )}
        </Card>

        {/* Agent Status */}
        <Card className="bg-[#0f1629] border-violet-500/10 overflow-hidden">
          <div className="px-5 py-3.5 border-b border-violet-500/10">
            <h2 className="text-xs font-mono font-semibold text-violet-300 tracking-wider">AGENT STATUS</h2>
          </div>
          <div className="divide-y divide-white/[0.03]">
            {agents.slice(0, 8).map((a: any, i: number) => (
              <div key={i} className="px-5 py-2.5 flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span className={`w-2 h-2 rounded-full ${a.status === "Available" || a.status === "available" ? "bg-emerald-500 animate-pulse" : a.status === "Busy" ? "bg-amber-500" : "bg-slate-600"}`} />
                  <span className="text-sm text-slate-300">{a.name}</span>
                </div>
                <div className="flex items-center gap-2">
                  {a.extension && <span className="text-[10px] font-mono text-slate-600">x{a.extension}</span>}
                  <span className="text-[10px] font-mono text-slate-500">{a.status}</span>
                </div>
              </div>
            ))}
            {agents.length === 0 && (
              <div className="px-5 py-6 text-center text-sm text-slate-600">No agents configured</div>
            )}
          </div>
        </Card>
      </div>

      {/* Recent Calls + Services */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <Card className="lg:col-span-2 bg-[#0f1629] border-cyan-500/10 glow-border">
          <div className="px-5 py-3.5 border-b border-cyan-500/10">
            <h2 className="text-xs font-mono font-semibold text-slate-300 tracking-wider">RECENT CALLS</h2>
          </div>
          <div className="divide-y divide-white/[0.03]">
            {recentCalls.map((c: any, i: number) => (
              <div key={i} className="px-5 py-3 flex items-start gap-4 hover:bg-cyan-500/[0.02]">
                <span className="font-mono text-[10px] text-slate-500 mt-1 w-12 shrink-0">
                  {c.start_time ? new Date(c.start_time).toLocaleTimeString("en-US", { hour12: false, hour: "2-digit", minute: "2-digit" }) : "—"}
                </span>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-0.5">
                    <span className="font-mono text-sm text-slate-300">{c.caller_number || "—"}</span>
                    <Badge variant="outline" className={`text-[9px] font-mono ${sentimentColor[c.sentiment] || sentimentColor.neutral}`}>
                      {c.sentiment || "—"}
                    </Badge>
                    <Badge variant="outline" className={`text-[9px] font-mono ${modeStyle[c.mode] || ""}`}>
                      {c.mode || "—"}
                    </Badge>
                  </div>
                  <p className="text-xs text-slate-500 truncate">{c.summary || "No summary"}</p>
                </div>
                <span className="font-mono text-xs text-slate-600 shrink-0">{formatDuration(c.duration)}</span>
              </div>
            ))}
            {recentCalls.length === 0 && (
              <div className="px-5 py-6 text-center text-sm text-slate-600">No calls yet</div>
            )}
          </div>
        </Card>

        {/* Service Health */}
        <Card className="bg-[#0f1629] border-cyan-500/10 overflow-hidden">
          <div className="px-5 py-3.5 border-b border-cyan-500/10">
            <h2 className="text-xs font-mono font-semibold text-slate-300 tracking-wider">SERVICES</h2>
          </div>
          <div className="divide-y divide-white/[0.03]">
            {services.map((s: any, i: number) => (
              <div key={i} className="px-5 py-2.5 flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span className={`w-2 h-2 rounded-full ${s.status === "online" ? "bg-emerald-500" : "bg-rose-500"}`} />
                  <span className="text-sm text-slate-300">{s.name}</span>
                </div>
                <span className="text-[10px] font-mono text-slate-600">{s.port}</span>
              </div>
            ))}
            {services.length === 0 && (
              <div className="px-5 py-6 text-center text-sm text-slate-600">Loading services...</div>
            )}
          </div>
        </Card>
      </div>
    </div>
  );
}
