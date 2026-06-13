"use client";

import { useState, useEffect } from "react";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { authFetch } from "@/lib/auth";

const statusDot: Record<string, string> = {
  Available: "bg-emerald-500",
  Busy: "bg-amber-500",
  "On Break": "bg-violet-500",
  "Wrap-up": "bg-cyan-500",
  Offline: "bg-slate-600",
  available: "bg-emerald-500",
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
};

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

  // New agent form
  const [newAgent, setNewAgent] = useState({
    name: "", email: "", phone: "", extension: "", department: "Support",
    expertise: "", languages: "English", priority: 1, max_calls: 3, queue: "Support",
  });

  // Edit agent
  const [editAgent, setEditAgent] = useState<Agent | null>(null);

  useEffect(() => {
    loadAgents();
    loadQueues();
  }, []);

  async function loadAgents() {
    try {
      const res = await authFetch("/api/agents");
      if (res.ok) {
        const data = await res.json();
        setAgents(data || []);
      }
    } catch {}
  }

  async function loadQueues() {
    try {
      const res = await authFetch("/api/queues/list");
      if (res.ok) {
        const data = await res.json();
        setQueues((data || []).map((q: any) => q.name));
      }
    } catch {}
  }

  async function createAgent() {
    if (!newAgent.name) return;
    const body = {
      name: newAgent.name,
      email: newAgent.email,
      phone: newAgent.phone,
      expertise: newAgent.expertise.split(",").map(s => s.trim()).filter(Boolean),
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
        id: editAgent.id,
        name: editAgent.name,
        email: editAgent.email,
        phone: editAgent.phone || "",
        extension: editAgent.extension || "",
        department: editAgent.department || "",
        expertise: editAgent.expertise || [],
        languages: editAgent.languages || [],
        priority: editAgent.priority || 1,
        max_calls: editAgent.max_calls || 3,
        status: editAgent.status || "Available",
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

  const filtered = agents.filter((a) =>
    a.name?.toLowerCase().includes(search.toLowerCase()) ||
    a.expertise?.some((e) => e.includes(search.toLowerCase())) ||
    a.department?.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-slate-100">Agent Roster</h1>
          <p className="text-sm text-slate-500 font-mono mt-1">{agents.length} agents configured</p>
        </div>
        <Button onClick={() => setShowAdd(!showAdd)}
          className="bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider">
          {showAdd ? "CANCEL" : "+ ADD AGENT"}
        </Button>
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
              <label className="text-[10px] font-mono text-slate-500 block mb-1">QUEUE</label>
              <select value={newAgent.queue} onChange={(e) => setNewAgent({...newAgent, queue: e.target.value})}
                className="w-full bg-[#070b14] border border-cyan-500/15 rounded-md text-slate-200 font-mono text-sm px-3 py-2">
                {queues.map(q => <option key={q} value={q}>{q}</option>)}
                {queues.length === 0 && <option>Support</option>}
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
          </div>
          <div className="flex justify-end mt-4">
            <Button onClick={createAgent} className="bg-emerald-600 hover:bg-emerald-500 text-white font-mono text-xs tracking-wider px-6">
              CREATE AGENT
            </Button>
          </div>
        </Card>
      )}

      <Input
        placeholder="Search agents, skills, or department..."
        value={search} onChange={(e) => setSearch(e.target.value)}
        className="max-w-md bg-[#0f1629] border-cyan-500/15 text-slate-200 placeholder:text-slate-600 font-mono text-sm"
      />

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
            {filtered.map((a) => (
              <tr key={a.id} className="border-b border-white/[0.03] hover:bg-cyan-500/[0.03] transition-colors">
                <td className="px-5 py-4">
                  <div className="flex items-center gap-2">
                    <span className={`w-2.5 h-2.5 rounded-full ${statusDot[a.status] || "bg-slate-600"} ${a.status === "Available" || a.status === "available" ? "animate-pulse" : ""}`} />
                    <span className="text-[10px] font-mono text-slate-500 uppercase">{a.status}</span>
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
            ))}
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
                <option value="Available">Available</option><option value="Busy">Busy</option>
                <option value="On Break">On Break</option><option value="Offline">Offline</option>
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
