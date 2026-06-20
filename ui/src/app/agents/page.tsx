"use client";

import { useState, useEffect, useCallback } from "react";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { authFetch } from "@/lib/auth";
import {
  Users, UserCheck, Coffee, PhoneOff, Phone, Clock, RefreshCw,
} from "lucide-react";

const statusConfig: Record<string, { dot: string; icon: typeof Users; label: string }> = {
  Available: { dot: "bg-emerald-500", icon: UserCheck, label: "Available" },
  Busy: { dot: "bg-amber-500", icon: Phone, label: "Busy" },
  "On Call": { dot: "bg-blue-500", icon: Phone, label: "On Call" },
  "On Break": { dot: "bg-violet-500", icon: Coffee, label: "On Break" },
  "Wrap-up": { dot: "bg-cyan-500", icon: Clock, label: "Wrap-up" },
  Offline: { dot: "bg-slate-600", icon: PhoneOff, label: "Offline" },
};

const expertiseColor: Record<string, string> = {
  billing: "bg-cyan-500/15 text-cyan-400 border-cyan-500/25",
  technical: "bg-violet-500/15 text-violet-400 border-violet-500/25",
  sales: "bg-amber-500/15 text-amber-400 border-amber-500/25",
  retention: "bg-rose-500/15 text-rose-400 border-rose-500/25",
  compliance: "bg-emerald-500/15 text-emerald-400 border-emerald-500/25",
  general: "bg-blue-500/15 text-blue-400 border-blue-500/25",
  escalation: "bg-orange-500/15 text-orange-400 border-orange-500/25",
  upsell: "bg-pink-500/15 text-pink-400 border-pink-500/25",
  enterprise: "bg-indigo-500/15 text-indigo-400 border-indigo-500/25",
  payments: "bg-teal-500/15 text-teal-400 border-teal-500/25",
  refunds: "bg-red-500/15 text-red-400 border-red-500/25",
  returns: "bg-orange-500/15 text-orange-400 border-orange-500/25",
  invoices: "bg-sky-500/15 text-sky-400 border-sky-500/25",
  pricing: "bg-yellow-500/15 text-yellow-400 border-yellow-500/25",
};

const ALL_STATUSES = ["Available", "Busy", "On Break", "Wrap-up", "Offline"];

interface Agent {
  id: string; name: string; email: string; phone?: string; extension?: string;
  department?: string; expertise: string[]; status: string;
  active_calls: number; max_calls?: number; languages?: string[];
  priority?: number; queues?: string[];
}

export default function AgentsPage() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [queues, setQueues] = useState<string[]>([]);
  const [search, setSearch] = useState("");
  const [showAdd, setShowAdd] = useState(false);
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);
  const [statusFilter, setStatusFilter] = useState<string>("all");

  const [newAgent, setNewAgent] = useState({
    name: "", email: "", phone: "", extension: "", department: "Support",
    expertise: "", languages: "English", priority: 1, max_calls: 3, queue: "Support",
  });

  const [editAgent, setEditAgent] = useState<Agent | null>(null);

  const loadAgents = useCallback(async () => {
    try {
      const res = await authFetch("/api/agents");
      if (res.ok) {
        const data = await res.json();
        setAgents(data || []);
        setLastUpdated(new Date());
      }
    } catch {}
  }, []);

  async function loadQueues() {
    try {
      const res = await authFetch("/api/queues/list");
      if (res.ok) {
        const data = await res.json();
        setQueues((data || []).map((q: any) => q.name));
      }
    } catch {}
  }

  useEffect(() => {
    loadAgents();
    loadQueues();
    const iv = setInterval(loadAgents, 5000);
    return () => clearInterval(iv);
  }, [loadAgents]);

  async function changeStatus(agentId: string, newStatus: string) {
    await authFetch("/api/agents", {
      method: "PUT",
      body: JSON.stringify({ id: agentId, status: newStatus }),
    });
    loadAgents();
  }

  async function createAgent() {
    if (!newAgent.name) return;
    const body = {
      name: newAgent.name, email: newAgent.email, phone: newAgent.phone,
      extension: newAgent.extension, department: newAgent.department,
      expertise: newAgent.expertise.split(",").map(s => s.trim()).filter(Boolean),
      languages: newAgent.languages.split(",").map(s => s.trim()).filter(Boolean),
      priority: newAgent.priority, max_calls: newAgent.max_calls,
    };
    const res = await authFetch("/api/agents", { method: "POST", body: JSON.stringify(body) });
    if (res.ok) {
      setShowAdd(false);
      setNewAgent({ name: "", email: "", phone: "", extension: "", department: "Support", expertise: "", languages: "English", priority: 1, max_calls: 3, queue: "Support" });
      loadAgents();
    }
  }

  async function saveEditAgent() {
    if (!editAgent) return;
    await authFetch("/api/agents", {
      method: "PUT",
      body: JSON.stringify({
        id: editAgent.id, name: editAgent.name, email: editAgent.email,
        phone: editAgent.phone || "", extension: editAgent.extension || "",
        department: editAgent.department || "", expertise: editAgent.expertise || [],
        languages: editAgent.languages || [], priority: editAgent.priority || 1,
        max_calls: editAgent.max_calls || 3, status: editAgent.status || "Available",
      }),
    });
    setEditAgent(null);
    loadAgents();
  }

  async function deleteAgent(id: string) {
    if (!confirm("Delete this agent?")) return;
    await authFetch("/api/agents", { method: "DELETE", body: JSON.stringify({ id }) });
    loadAgents();
  }

  // Stats
  const available = agents.filter(a => a.status === "Available").length;
  const onCall = agents.filter(a => a.status === "On Call" || a.status === "Busy").length;
  const onBreak = agents.filter(a => a.status === "On Break" || a.status === "Wrap-up").length;
  const offline = agents.filter(a => a.status === "Offline").length;

  const filtered = agents.filter((a) => {
    const matchesSearch =
      a.name?.toLowerCase().includes(search.toLowerCase()) ||
      a.expertise?.some((e) => e.includes(search.toLowerCase())) ||
      a.department?.toLowerCase().includes(search.toLowerCase());
    const matchesFilter =
      statusFilter === "all" ||
      (statusFilter === "active" && (a.status === "Available" || a.status === "On Call" || a.status === "Busy")) ||
      (statusFilter === "away" && (a.status === "On Break" || a.status === "Wrap-up")) ||
      (statusFilter === "offline" && a.status === "Offline") ||
      a.status === statusFilter;
    return matchesSearch && matchesFilter;
  });

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-slate-100">Agent Roster</h1>
          <div className="flex items-center gap-3 mt-1">
            <span className="text-sm text-slate-500 font-mono">{agents.length} agents</span>
            {lastUpdated && (
              <span className="text-[10px] text-slate-600 font-mono flex items-center gap-1">
                <RefreshCw size={9} className="animate-spin" style={{ animationDuration: "5s" }} />
                {lastUpdated.toLocaleTimeString()}
              </span>
            )}
          </div>
        </div>
        <Button onClick={() => setShowAdd(!showAdd)}
          className="bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider">
          {showAdd ? "CANCEL" : "+ ADD AGENT"}
        </Button>
      </div>

      {/* Stats Cards */}
      <div className="grid grid-cols-4 gap-3">
        {[
          { label: "Available", value: available, color: "emerald", icon: UserCheck, filter: "Available" },
          { label: "On Call", value: onCall, color: "blue", icon: Phone, filter: "active" },
          { label: "Away", value: onBreak, color: "violet", icon: Coffee, filter: "away" },
          { label: "Offline", value: offline, color: "slate", icon: PhoneOff, filter: "offline" },
        ].map(({ label, value, color, icon: Icon, filter }) => (
          <button key={label} onClick={() => setStatusFilter(statusFilter === filter ? "all" : filter)}
            className={`rounded-lg border transition-all ${statusFilter === filter ? `border-${color}-500/40 bg-${color}-500/10` : "border-white/[0.04] bg-[#070b14] hover:bg-white/[0.02]"} p-3 text-left`}>
            <div className="flex items-center justify-between mb-1">
              <Icon size={14} className={`text-${color}-400`} strokeWidth={2} />
              <span className={`text-xl font-mono font-bold text-${color}-400`}>{value}</span>
            </div>
            <span className="text-[10px] font-mono text-slate-500 uppercase tracking-wider">{label}</span>
          </button>
        ))}
      </div>

      {/* Add Agent Form */}
      {showAdd && (
        <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
          <h3 className="text-sm font-semibold text-slate-200 tracking-wide mb-4">NEW AGENT</h3>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">NAME</label>
              <Input value={newAgent.name} onChange={(e) => setNewAgent({...newAgent, name: e.target.value})}
                placeholder="Sarah Chen" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
            </div>
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">EMAIL</label>
              <Input value={newAgent.email} onChange={(e) => setNewAgent({...newAgent, email: e.target.value})}
                placeholder="sarah@company.com" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
            </div>
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">PHONE</label>
              <Input value={newAgent.phone} onChange={(e) => setNewAgent({...newAgent, phone: e.target.value})}
                placeholder="+15550101" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
            </div>
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">EXTENSION</label>
              <Input value={newAgent.extension} onChange={(e) => setNewAgent({...newAgent, extension: e.target.value})}
                placeholder="2001" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
            </div>
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">DEPARTMENT</label>
              <select value={newAgent.department} onChange={(e) => setNewAgent({...newAgent, department: e.target.value})}
                className="w-full bg-[#070b14] border border-cyan-500/15 rounded-md text-slate-200 font-mono text-sm px-3 py-2">
                <option value="Support">Support</option>
                <option value="Sales">Sales</option>
                <option value="Billing">Billing</option>
                <option value="Escalation">Escalation</option>
              </select>
            </div>
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">SKILLS (comma-separated)</label>
              <Input value={newAgent.expertise} onChange={(e) => setNewAgent({...newAgent, expertise: e.target.value})}
                placeholder="billing, retention, technical" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
            </div>
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">LANGUAGES</label>
              <Input value={newAgent.languages} onChange={(e) => setNewAgent({...newAgent, languages: e.target.value})}
                placeholder="English, Spanish" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
            </div>
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">PRIORITY</label>
              <select value={newAgent.priority} onChange={(e) => setNewAgent({...newAgent, priority: parseInt(e.target.value)})}
                className="w-full bg-[#070b14] border border-cyan-500/15 rounded-md text-slate-200 font-mono text-sm px-3 py-2">
                <option value={1}>1 — Junior</option>
                <option value={2}>2 — Senior</option>
                <option value={3}>3 — Specialist</option>
              </select>
            </div>
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">MAX CONCURRENT CALLS</label>
              <Input type="number" value={newAgent.max_calls} onChange={(e) => setNewAgent({...newAgent, max_calls: parseInt(e.target.value) || 3})}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
            </div>
          </div>
          <div className="flex justify-end mt-4">
            <Button onClick={createAgent} className="bg-emerald-600 hover:bg-emerald-500 text-white font-mono text-xs tracking-wider px-6">
              CREATE AGENT
            </Button>
          </div>
        </Card>
      )}

      {/* Search */}
      <Input
        placeholder="Search agents, skills, or department..."
        value={search} onChange={(e) => setSearch(e.target.value)}
        className="max-w-md bg-[#0f1629] border-cyan-500/15 text-slate-200 placeholder:text-slate-600 font-mono text-sm"
      />

      {/* Agent Table */}
      <Card className="bg-[#0f1629] border-cyan-500/10 glow-border overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-[11px] font-mono text-slate-500 uppercase tracking-wider border-b border-white/5">
              <th className="text-left px-5 py-3 font-medium">Status</th>
              <th className="text-left px-5 py-3 font-medium">Agent</th>
              <th className="text-left px-5 py-3 font-medium">Dept</th>
              <th className="text-left px-5 py-3 font-medium">Skills</th>
              <th className="text-left px-5 py-3 font-medium">Calls</th>
              <th className="text-left px-5 py-3 font-medium">Ext</th>
              <th className="text-left px-5 py-3 font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((a) => {
              const cfg = statusConfig[a.status] || statusConfig.Offline;
              return (
                <tr key={a.id} className="border-b border-white/[0.03] hover:bg-cyan-500/[0.03] transition-colors">
                  <td className="px-5 py-4">
                    <div className="relative group">
                      <div className="flex items-center gap-2 cursor-pointer">
                        <span className={`w-2.5 h-2.5 rounded-full ${cfg.dot} ${a.status === "Available" ? "animate-pulse" : ""} ${a.status === "On Call" ? "animate-pulse" : ""}`} />
                        <span className="text-[10px] font-mono text-slate-500 uppercase group-hover:text-slate-300 transition-colors">{a.status}</span>
                      </div>
                      <div className="absolute left-0 top-full mt-1 z-20 hidden group-hover:block">
                        <div className="bg-[#0a0e1a] border border-white/10 rounded-lg shadow-xl py-1 min-w-[130px]">
                          {ALL_STATUSES.map(s => (
                            <button key={s} onClick={() => changeStatus(a.id, s)}
                              className={`w-full text-left px-3 py-1.5 text-[10px] font-mono flex items-center gap-2 hover:bg-white/5 transition-colors ${a.status === s ? "text-cyan-400" : "text-slate-400"}`}>
                              <span className={`w-2 h-2 rounded-full ${(statusConfig[s] || statusConfig.Offline).dot}`} />
                              {s}
                            </button>
                          ))}
                        </div>
                      </div>
                    </div>
                  </td>
                  <td className="px-5 py-4">
                    <div className="font-medium text-slate-200">{a.name}</div>
                    <div className="text-xs text-slate-500 font-mono">{a.email}</div>
                  </td>
                  <td className="px-5 py-4 text-xs text-slate-400">{a.department || "—"}</td>
                  <td className="px-5 py-4">
                    <div className="flex flex-wrap gap-1.5">
                      {(a.expertise || []).map((e) => (
                        <Badge key={e} variant="outline" className={`text-[10px] font-mono ${expertiseColor[e] || "bg-slate-500/15 text-slate-400"}`}>
                          {e}
                        </Badge>
                      ))}
                    </div>
                  </td>
                  <td className="px-5 py-4">
                    <span className="font-mono text-sm">
                      <span className={(a.active_calls || 0) >= (a.max_calls || 3) ? "text-rose-400" : "text-cyan-400"}>{a.active_calls || 0}</span>
                      <span className="text-slate-600">/{a.max_calls || 3}</span>
                    </span>
                  </td>
                  <td className="px-5 py-4 font-mono text-xs text-slate-500">{a.extension || "—"}</td>
                  <td className="px-5 py-4">
                    <div className="flex gap-1">
                      <button onClick={() => setEditAgent({...a})} className="text-[10px] font-mono text-cyan-400 hover:text-cyan-300 px-2 py-1 rounded hover:bg-cyan-500/10">EDIT</button>
                      <button onClick={() => deleteAgent(a.id)} className="text-[10px] font-mono text-rose-400 hover:text-rose-300 px-2 py-1 rounded hover:bg-rose-500/10">DEL</button>
                    </div>
                  </td>
                </tr>
              );
            })}
            {filtered.length === 0 && (
              <tr>
                <td colSpan={7} className="px-5 py-8 text-center text-sm text-slate-600">
                  {agents.length === 0 ? "No agents configured. Click + ADD AGENT." : "No agents match your search."}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </Card>

      {/* Edit Agent Panel */}
      {editAgent && (
        <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-sm font-semibold text-slate-200 tracking-wide">EDIT AGENT — {editAgent.name}</h3>
            <button onClick={() => setEditAgent(null)} className="text-xs font-mono text-slate-500 hover:text-slate-300">CANCEL</button>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">NAME</label>
              <Input value={editAgent.name} onChange={(e) => setEditAgent({...editAgent, name: e.target.value})}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
            </div>
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">EMAIL</label>
              <Input value={editAgent.email} onChange={(e) => setEditAgent({...editAgent, email: e.target.value})}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
            </div>
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">EXTENSION</label>
              <Input value={editAgent.extension || ""} onChange={(e) => setEditAgent({...editAgent, extension: e.target.value})}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
            </div>
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">DEPARTMENT</label>
              <select value={editAgent.department || "Support"} onChange={(e) => setEditAgent({...editAgent, department: e.target.value})}
                className="w-full bg-[#070b14] border border-cyan-500/15 rounded-md text-slate-200 font-mono text-sm px-3 py-2">
                <option value="Support">Support</option><option value="Sales">Sales</option>
                <option value="Billing">Billing</option><option value="Escalation">Escalation</option>
              </select>
            </div>
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">SKILLS</label>
              <Input value={(editAgent.expertise || []).join(", ")}
                onChange={(e) => setEditAgent({...editAgent, expertise: e.target.value.split(",").map(s => s.trim()).filter(Boolean)})}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
            </div>
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">LANGUAGES</label>
              <Input value={(editAgent.languages || []).join(", ")}
                onChange={(e) => setEditAgent({...editAgent, languages: e.target.value.split(",").map(s => s.trim()).filter(Boolean)})}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
            </div>
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">PRIORITY</label>
              <select value={editAgent.priority || 1} onChange={(e) => setEditAgent({...editAgent, priority: parseInt(e.target.value)})}
                className="w-full bg-[#070b14] border border-cyan-500/15 rounded-md text-slate-200 font-mono text-sm px-3 py-2">
                <option value={1}>1 — Junior</option><option value={2}>2 — Senior</option><option value={3}>3 — Specialist</option>
              </select>
            </div>
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">MAX CALLS</label>
              <Input type="number" value={editAgent.max_calls || 3} onChange={(e) => setEditAgent({...editAgent, max_calls: parseInt(e.target.value) || 3})}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
            </div>
            <div>
              <label className="text-[10px] font-mono text-slate-500 block mb-1">STATUS</label>
              <select value={editAgent.status || "Available"} onChange={(e) => setEditAgent({...editAgent, status: e.target.value})}
                className="w-full bg-[#070b14] border border-cyan-500/15 rounded-md text-slate-200 font-mono text-sm px-3 py-2">
                {ALL_STATUSES.map(s => <option key={s} value={s}>{s}</option>)}
              </select>
            </div>
          </div>
          <div className="flex justify-end mt-4">
            <Button onClick={saveEditAgent} className="bg-emerald-600 hover:bg-emerald-500 text-white font-mono text-xs tracking-wider px-6">
              SAVE CHANGES
            </Button>
          </div>
        </Card>
      )}
    </div>
  );
}
