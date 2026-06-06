"use client";

import { useState } from "react";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";

const mockAgents = [
  { id: "1", name: "Priya Sharma", email: "priya@company.com", phone: "+1 555-0101", expertise: ["billing", "retention"], status: "available", activeCalls: 1, maxCalls: 3 },
  { id: "2", name: "Raj Patel", email: "raj@company.com", phone: "+1 555-0102", expertise: ["technical", "networking"], status: "busy", activeCalls: 3, maxCalls: 3 },
  { id: "3", name: "Anita Desai", email: "anita@company.com", phone: "+1 555-0103", expertise: ["sales", "upsell"], status: "available", activeCalls: 0, maxCalls: 5 },
  { id: "4", name: "Vikram Singh", email: "vikram@company.com", phone: "+1 555-0104", expertise: ["compliance", "insurance"], status: "offline", activeCalls: 0, maxCalls: 3 },
  { id: "5", name: "Meera Joshi", email: "meera@company.com", phone: "+1 555-0105", expertise: ["billing", "technical", "sales"], status: "available", activeCalls: 2, maxCalls: 4 },
];

const statusDot: Record<string, string> = {
  available: "bg-emerald-500",
  busy: "bg-amber-500",
  offline: "bg-slate-600",
};

const expertiseColor: Record<string, string> = {
  billing: "bg-cyan-500/15 text-cyan-400 border-cyan-500/25",
  technical: "bg-violet-500/15 text-violet-400 border-violet-500/25",
  sales: "bg-amber-500/15 text-amber-400 border-amber-500/25",
  retention: "bg-rose-500/15 text-rose-400 border-rose-500/25",
  compliance: "bg-emerald-500/15 text-emerald-400 border-emerald-500/25",
  insurance: "bg-blue-500/15 text-blue-400 border-blue-500/25",
  networking: "bg-orange-500/15 text-orange-400 border-orange-500/25",
  upsell: "bg-pink-500/15 text-pink-400 border-pink-500/25",
};

export default function AgentsPage() {
  const [search, setSearch] = useState("");
  const filtered = mockAgents.filter((a) =>
    a.name.toLowerCase().includes(search.toLowerCase()) ||
    a.expertise.some((e) => e.includes(search.toLowerCase()))
  );

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-slate-100">Agent Roster</h1>
          <p className="text-sm text-slate-500 font-mono mt-1">{mockAgents.length} agents configured</p>
        </div>
        <Dialog>
          <DialogTrigger>
            <span className="inline-flex items-center justify-center px-4 py-2 rounded-md bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider cursor-pointer transition-colors">
              + ADD AGENT
            </span>
          </DialogTrigger>
          <DialogContent className="bg-[#111827] border-cyan-500/20">
            <DialogHeader>
              <DialogTitle className="text-slate-200 font-mono">NEW AGENT</DialogTitle>
            </DialogHeader>
            <div className="space-y-4 pt-2">
              <Input placeholder="Full name" className="bg-[#0f1629] border-cyan-500/15 text-slate-200" />
              <Input placeholder="Email" type="email" className="bg-[#0f1629] border-cyan-500/15 text-slate-200" />
              <Input placeholder="Phone" className="bg-[#0f1629] border-cyan-500/15 text-slate-200" />
              <Input placeholder="Expertise (comma-separated)" className="bg-[#0f1629] border-cyan-500/15 text-slate-200" />
              <Button className="w-full bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider">
                CREATE AGENT
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      </div>

      <Input
        placeholder="Search agents or expertise..."
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="max-w-md bg-[#0f1629] border-cyan-500/15 text-slate-200 placeholder:text-slate-600 font-mono text-sm"
      />

      <Card className="bg-[#0f1629] border-cyan-500/10 glow-border overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-[11px] font-mono text-slate-500 uppercase tracking-wider border-b border-white/5">
              <th className="text-left px-5 py-3 font-medium">Status</th>
              <th className="text-left px-5 py-3 font-medium">Agent</th>
              <th className="text-left px-5 py-3 font-medium">Expertise</th>
              <th className="text-left px-5 py-3 font-medium">Calls</th>
              <th className="text-left px-5 py-3 font-medium">Contact</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((a) => (
              <tr key={a.id} className="border-b border-white/[0.03] hover:bg-cyan-500/[0.03] transition-colors">
                <td className="px-5 py-4">
                  <div className="flex items-center gap-2">
                    <span className={`w-2.5 h-2.5 rounded-full ${statusDot[a.status]} ${a.status === "available" ? "animate-pulse" : ""}`} />
                    <span className="text-[10px] font-mono text-slate-500 uppercase">{a.status}</span>
                  </div>
                </td>
                <td className="px-5 py-4">
                  <div className="font-medium text-slate-200">{a.name}</div>
                  <div className="text-xs text-slate-500 font-mono">{a.email}</div>
                </td>
                <td className="px-5 py-4">
                  <div className="flex flex-wrap gap-1.5">
                    {a.expertise.map((e) => (
                      <Badge key={e} variant="outline" className={`text-[10px] font-mono ${expertiseColor[e] || "bg-slate-500/15 text-slate-400"}`}>
                        {e}
                      </Badge>
                    ))}
                  </div>
                </td>
                <td className="px-5 py-4">
                  <span className="font-mono text-sm">
                    <span className={a.activeCalls >= a.maxCalls ? "text-rose-400" : "text-cyan-400"}>{a.activeCalls}</span>
                    <span className="text-slate-600">/{a.maxCalls}</span>
                  </span>
                </td>
                <td className="px-5 py-4 font-mono text-xs text-slate-500">{a.phone}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>
    </div>
  );
}
