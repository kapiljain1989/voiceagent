"use client";

import { useState, useEffect, useCallback } from "react";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { authFetch } from "@/lib/auth";
import {
  Shield, ShieldCheck, ShieldX, Plus, Trash2, Eye, Lock, Unlock,
  Network, Phone, Radio, RefreshCw, AlertTriangle, CheckCircle, XCircle,
} from "lucide-react";

const providerBadge: Record<string, string> = {
  "anthropic-vertex": "bg-orange-500/15 text-orange-400 border-orange-500/25",
  "google-vertex": "bg-blue-500/15 text-blue-400 border-blue-500/25",
  "ollama": "bg-emerald-500/15 text-emerald-400 border-emerald-500/25",
  custom: "bg-slate-500/15 text-slate-400 border-slate-500/25",
};

interface LLMConfig { provider: string; model: string; region: string; max_tokens: number; }

interface Trunk {
  id: string; name: string; provider: string; address: string; port: number;
  transport: string; register: boolean; username: string; caller_id: string;
  codecs: string; status: string; created_at: string;
  // Security fields
  auth_realm?: string; auth_user?: string; tls_enabled?: boolean;
  srtp_enabled?: boolean; security_policy?: string;
}

interface ACLEntry {
  id: string; trunk_id: string; ip_address: string; cidr_bits: number;
  description: string; created_at: string;
}

interface SecurityLog {
  id: number; event_type: string; trunk_name: string; source_ip: string;
  call_id: string; details: string; created_at: string;
}

const TRUNK_TYPES = [
  { value: "direct", label: "Direct (B2BUA)", desc: "Gateway handles the call — answers, queues, bridges to agent" },
  { value: "siprec", label: "SIPREC Observer", desc: "Gateway observes the call — transcription, copilot, sentiment only" },
];

const SECURITY_POLICIES = [
  { value: "strict", label: "Strict", desc: "IP whitelist + digest auth required", icon: ShieldCheck, color: "emerald" },
  { value: "permissive", label: "Permissive", desc: "IP whitelist only, no auth", icon: Shield, color: "amber" },
  { value: "disabled", label: "Disabled", desc: "Accept all (testing only)", icon: ShieldX, color: "rose" },
];

export default function SettingsPage() {
  const [llm, setLlm] = useState<LLMConfig>({ provider: "", model: "", region: "", max_tokens: 512 });
  const [llmSaved, setLlmSaved] = useState(false);
  const [interactivePrompt, setInteractivePrompt] = useState("");
  const [copilotPrompt, setCopilotPrompt] = useState("");
  const [promptSaved, setPromptSaved] = useState(false);
  const [testPrompt, setTestPrompt] = useState("Hello, how are you?");
  const [testResult, setTestResult] = useState("");
  const [testing, setTesting] = useState(false);
  const [services, setServices] = useState<Array<{ name: string; status: string; port: string }>>([]);

  // Trunks
  const [trunks, setTrunks] = useState<Trunk[]>([]);
  const [showAddTrunk, setShowAddTrunk] = useState(false);
  const [selectedTrunk, setSelectedTrunk] = useState<Trunk | null>(null);
  const [trunkACLs, setTrunkACLs] = useState<ACLEntry[]>([]);
  const [newACL, setNewACL] = useState({ ip_address: "", cidr_bits: 32, description: "" });
  const [securityLog, setSecurityLog] = useState<SecurityLog[]>([]);
  const [newTrunk, setNewTrunk] = useState({
    name: "", address: "", port: 5060, transport: "udp", codecs: "PCMU,PCMA",
    caller_id: "", type: "direct", security_policy: "strict",
    auth_realm: "voiceagent", auth_user: "", auth_password: "",
    tls_enabled: false, srtp_enabled: false,
  });

  useEffect(() => { loadSettings(); loadServices(); loadTrunks(); }, []);

  async function loadSettings() {
    try {
      const res = await authFetch("/api/settings");
      if (res.ok) {
        const data = await res.json();
        setLlm(data.llm || {});
        setInteractivePrompt(data.prompts?.interactive || "");
        setCopilotPrompt(data.prompts?.copilot || "");
      }
    } catch {}
  }

  async function loadServices() {
    try {
      const res = await authFetch("/api/services/status");
      if (res.ok) setServices(await res.json());
    } catch {}
  }

  const loadTrunks = useCallback(async () => {
    try {
      const res = await authFetch("/api/trunks");
      if (res.ok) setTrunks(await res.json() || []);
    } catch {}
  }, []);

  async function loadTrunkACLs(trunkId: string) {
    try {
      const res = await authFetch(`/api/trunks/acl?trunk_id=${trunkId}`);
      if (res.ok) setTrunkACLs(await res.json() || []);
    } catch {}
  }

  async function loadSecurityLog() {
    try {
      const res = await authFetch("/api/trunks/security-log");
      if (res.ok) setSecurityLog(await res.json() || []);
    } catch {}
  }

  async function saveLLM() {
    const res = await authFetch("/api/settings/llm", { method: "POST", body: JSON.stringify(llm) });
    if (res.ok) { setLlmSaved(true); setTimeout(() => setLlmSaved(false), 2000); }
  }

  async function savePrompts() {
    const res = await authFetch("/api/settings/prompts", { method: "POST", body: JSON.stringify({ interactive: interactivePrompt, copilot: copilotPrompt }) });
    if (res.ok) { setPromptSaved(true); setTimeout(() => setPromptSaved(false), 2000); }
  }

  async function createTrunk() {
    if (!newTrunk.name || !newTrunk.address) return;
    const res = await authFetch("/api/trunks", { method: "POST", body: JSON.stringify(newTrunk) });
    if (res.ok) {
      setShowAddTrunk(false);
      setNewTrunk({ name: "", address: "", port: 5060, transport: "udp", codecs: "PCMU,PCMA", caller_id: "", type: "direct", security_policy: "strict", auth_realm: "voiceagent", auth_user: "", auth_password: "", tls_enabled: false, srtp_enabled: false });
      loadTrunks();
    }
  }

  async function deleteTrunk(id: string) {
    if (!confirm("Delete this trunk and all its ACL entries?")) return;
    await authFetch("/api/trunks", { method: "DELETE", body: JSON.stringify({ id }) });
    if (selectedTrunk?.id === id) setSelectedTrunk(null);
    loadTrunks();
  }

  async function addACL() {
    if (!selectedTrunk || !newACL.ip_address) return;
    await authFetch("/api/trunks/acl", { method: "POST", body: JSON.stringify({ trunk_id: selectedTrunk.id, ...newACL }) });
    setNewACL({ ip_address: "", cidr_bits: 32, description: "" });
    loadTrunkACLs(selectedTrunk.id);
  }

  async function deleteACL(aclId: string) {
    await authFetch("/api/trunks/acl", { method: "DELETE", body: JSON.stringify({ id: aclId }) });
    if (selectedTrunk) loadTrunkACLs(selectedTrunk.id);
  }

  async function testLLM() {
    setTesting(true); setTestResult("");
    try {
      const res = await authFetch("/api/llm/test", { method: "POST", body: JSON.stringify({ provider: llm.provider, model: llm.model, prompt: testPrompt }) });
      const data = await res.json();
      setTestResult(data.response || data.error || JSON.stringify(data));
    } catch (e: any) { setTestResult(`Failed: ${e.message}`); }
    setTesting(false);
  }

  function selectTrunk(trunk: Trunk) {
    setSelectedTrunk(trunk);
    loadTrunkACLs(trunk.id);
  }

  const policyConfig = SECURITY_POLICIES.find(p => p.value === (selectedTrunk?.security_policy || "strict")) || SECURITY_POLICIES[0];

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-slate-100">Configuration</h1>
        <p className="text-sm text-slate-500 font-mono mt-1">LLM, prompts, SIP trunks, security</p>
      </div>

      <Tabs defaultValue="trunks" className="space-y-4">
        <TabsList className="bg-[#0f1629] border border-cyan-500/10">
          <TabsTrigger value="trunks" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">SIP TRUNKS</TabsTrigger>
          <TabsTrigger value="security" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">SECURITY LOG</TabsTrigger>
          <TabsTrigger value="llm" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">LLM MODEL</TabsTrigger>
          <TabsTrigger value="prompts" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">PROMPTS</TabsTrigger>
          <TabsTrigger value="status" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">STATUS</TabsTrigger>
        </TabsList>

        {/* ── SIP Trunks ── */}
        <TabsContent value="trunks" className="space-y-4">
          <div className="flex gap-4">
            {/* Trunk List */}
            <div className={`${selectedTrunk ? "w-1/2" : "w-full"} space-y-3`}>
              <div className="flex items-center justify-between">
                <span className="text-xs font-mono text-slate-500">{trunks.length} trunk(s) configured</span>
                <Button onClick={() => setShowAddTrunk(!showAddTrunk)} className="bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-[10px] tracking-wider h-7 px-3">
                  <Plus size={12} className="mr-1" /> {showAddTrunk ? "CANCEL" : "ADD TRUNK"}
                </Button>
              </div>

              {/* Add Trunk Form */}
              {showAddTrunk && (
                <Card className="bg-[#0f1629] border-cyan-500/10 p-4 space-y-3">
                  <h3 className="text-xs font-mono text-slate-300 font-semibold">NEW SIP TRUNK</h3>
                  <div className="grid grid-cols-2 gap-2">
                    <div className="col-span-2">
                      <label className="text-[10px] font-mono text-slate-500 block mb-1">TRUNK TYPE</label>
                      <div className="grid grid-cols-2 gap-2">
                        {TRUNK_TYPES.map(t => (
                          <button key={t.value} onClick={() => setNewTrunk({ ...newTrunk, type: t.value })}
                            className={`p-2 rounded border text-left transition-all ${newTrunk.type === t.value ? "border-cyan-500/40 bg-cyan-500/10" : "border-white/5 bg-[#070b14] hover:bg-white/[0.02]"}`}>
                            <div className="flex items-center gap-1.5">
                              {t.value === "direct" ? <Phone size={12} className="text-cyan-400" /> : <Radio size={12} className="text-violet-400" />}
                              <span className="text-[10px] font-mono text-slate-300">{t.label}</span>
                            </div>
                            <p className="text-[9px] text-slate-600 mt-1">{t.desc}</p>
                          </button>
                        ))}
                      </div>
                    </div>
                    <div>
                      <label className="text-[10px] font-mono text-slate-500 block mb-1">NAME</label>
                      <Input value={newTrunk.name} onChange={e => setNewTrunk({ ...newTrunk, name: e.target.value })}
                        placeholder="Cisco CUBE DC1" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-xs h-8" />
                    </div>
                    <div>
                      <label className="text-[10px] font-mono text-slate-500 block mb-1">ADDRESS</label>
                      <Input value={newTrunk.address} onChange={e => setNewTrunk({ ...newTrunk, address: e.target.value })}
                        placeholder="10.0.1.50" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-xs h-8" />
                    </div>
                    <div>
                      <label className="text-[10px] font-mono text-slate-500 block mb-1">PORT</label>
                      <Input type="number" value={newTrunk.port} onChange={e => setNewTrunk({ ...newTrunk, port: parseInt(e.target.value) || 5060 })}
                        className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-xs h-8" />
                    </div>
                    <div>
                      <label className="text-[10px] font-mono text-slate-500 block mb-1">TRANSPORT</label>
                      <select value={newTrunk.transport} onChange={e => setNewTrunk({ ...newTrunk, transport: e.target.value })}
                        className="w-full bg-[#070b14] border border-cyan-500/15 rounded-md text-slate-200 font-mono text-xs px-2 h-8">
                        <option value="udp">UDP</option><option value="tcp">TCP</option><option value="tls">TLS</option>
                      </select>
                    </div>
                    <div>
                      <label className="text-[10px] font-mono text-slate-500 block mb-1">SECURITY POLICY</label>
                      <select value={newTrunk.security_policy} onChange={e => setNewTrunk({ ...newTrunk, security_policy: e.target.value })}
                        className="w-full bg-[#070b14] border border-cyan-500/15 rounded-md text-slate-200 font-mono text-xs px-2 h-8">
                        {SECURITY_POLICIES.map(p => <option key={p.value} value={p.value}>{p.label} — {p.desc}</option>)}
                      </select>
                    </div>
                    <div>
                      <label className="text-[10px] font-mono text-slate-500 block mb-1">CODECS</label>
                      <Input value={newTrunk.codecs} onChange={e => setNewTrunk({ ...newTrunk, codecs: e.target.value })}
                        className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-xs h-8" />
                    </div>
                    {newTrunk.security_policy === "strict" && (
                      <>
                        <div>
                          <label className="text-[10px] font-mono text-slate-500 block mb-1">AUTH USER</label>
                          <Input value={newTrunk.auth_user} onChange={e => setNewTrunk({ ...newTrunk, auth_user: e.target.value })}
                            placeholder="sbc-user" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-xs h-8" />
                        </div>
                        <div>
                          <label className="text-[10px] font-mono text-slate-500 block mb-1">AUTH PASSWORD</label>
                          <Input type="password" value={newTrunk.auth_password} onChange={e => setNewTrunk({ ...newTrunk, auth_password: e.target.value })}
                            placeholder="••••••••" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-xs h-8" />
                        </div>
                      </>
                    )}
                    <div className="col-span-2 flex items-center gap-4">
                      <label className="flex items-center gap-2 cursor-pointer">
                        <input type="checkbox" checked={newTrunk.tls_enabled} onChange={e => setNewTrunk({ ...newTrunk, tls_enabled: e.target.checked })}
                          className="rounded border-cyan-500/30 bg-[#070b14]" />
                        <span className="text-[10px] font-mono text-slate-400">TLS Signaling</span>
                      </label>
                      <label className="flex items-center gap-2 cursor-pointer">
                        <input type="checkbox" checked={newTrunk.srtp_enabled} onChange={e => setNewTrunk({ ...newTrunk, srtp_enabled: e.target.checked })}
                          className="rounded border-cyan-500/30 bg-[#070b14]" />
                        <span className="text-[10px] font-mono text-slate-400">SRTP Media</span>
                      </label>
                    </div>
                  </div>
                  <div className="flex justify-end">
                    <Button onClick={createTrunk} className="bg-emerald-600 hover:bg-emerald-500 text-white font-mono text-[10px] tracking-wider h-7 px-4">CREATE TRUNK</Button>
                  </div>
                </Card>
              )}

              {/* Trunk Cards */}
              {trunks.map(trunk => {
                const policy = SECURITY_POLICIES.find(p => p.value === (trunk.security_policy || "strict")) || SECURITY_POLICIES[0];
                const PolicyIcon = policy.icon;
                const isSelected = selectedTrunk?.id === trunk.id;
                return (
                  <Card key={trunk.id}
                    className={`bg-[#0f1629] border transition-all cursor-pointer ${isSelected ? "border-cyan-500/30 bg-cyan-500/[0.03]" : "border-white/[0.04] hover:border-white/10"}`}
                    onClick={() => selectTrunk(trunk)}>
                    <div className="p-4">
                      <div className="flex items-center justify-between mb-2">
                        <div className="flex items-center gap-2">
                          <Network size={14} className="text-cyan-400" />
                          <span className="text-sm font-medium text-slate-200">{trunk.name}</span>
                          <Badge variant="outline" className={`text-[9px] font-mono ${trunk.provider === "siprec" ? "bg-violet-500/15 text-violet-400 border-violet-500/25" : "bg-cyan-500/15 text-cyan-400 border-cyan-500/25"}`}>
                            {trunk.provider === "siprec" ? "SIPREC" : "DIRECT"}
                          </Badge>
                        </div>
                        <div className="flex items-center gap-2">
                          <PolicyIcon size={12} className={`text-${policy.color}-400`} />
                          <Badge variant="outline" className={`text-[9px] font-mono bg-${policy.color}-500/15 text-${policy.color}-400`}>{policy.label}</Badge>
                        </div>
                      </div>
                      <div className="flex items-center gap-4 text-[10px] font-mono text-slate-500">
                        <span>{trunk.address}:{trunk.port}</span>
                        <span>{trunk.transport?.toUpperCase()}</span>
                        {trunk.tls_enabled && <Badge variant="outline" className="text-[8px] bg-emerald-500/15 text-emerald-400 border-emerald-500/25"><Lock size={8} className="mr-0.5" />TLS</Badge>}
                        {trunk.srtp_enabled && <Badge variant="outline" className="text-[8px] bg-blue-500/15 text-blue-400 border-blue-500/25">SRTP</Badge>}
                        <span className={`ml-auto ${trunk.status === "active" ? "text-emerald-400" : "text-slate-600"}`}>{trunk.status}</span>
                      </div>
                    </div>
                  </Card>
                );
              })}

              {trunks.length === 0 && !showAddTrunk && (
                <Card className="bg-[#0f1629] border-white/[0.04] p-8 text-center">
                  <ShieldX size={24} className="text-slate-600 mx-auto mb-2" />
                  <p className="text-sm text-slate-500">No trunks configured</p>
                  <p className="text-[10px] text-slate-600 font-mono mt-1">Gateway rejects all SIP by default</p>
                </Card>
              )}
            </div>

            {/* Trunk Detail Panel */}
            {selectedTrunk && (
              <div className="w-1/2 space-y-3">
                <Card className="bg-[#0f1629] border-cyan-500/10 p-4">
                  <div className="flex items-center justify-between mb-3">
                    <h3 className="text-xs font-mono text-slate-300 font-semibold">{selectedTrunk.name} — IP WHITELIST</h3>
                    <button onClick={() => deleteTrunk(selectedTrunk.id)} className="text-[10px] font-mono text-rose-400 hover:text-rose-300 flex items-center gap-1">
                      <Trash2 size={10} /> DELETE TRUNK
                    </button>
                  </div>

                  {/* ACL entries */}
                  <div className="space-y-1.5 mb-3">
                    {trunkACLs.map(acl => (
                      <div key={acl.id} className="flex items-center justify-between bg-[#070b14] rounded px-3 py-2">
                        <div>
                          <span className="text-xs font-mono text-slate-300">{acl.ip_address}/{acl.cidr_bits}</span>
                          {acl.description && <span className="text-[10px] text-slate-600 ml-2">— {acl.description}</span>}
                        </div>
                        <button onClick={() => deleteACL(acl.id)} className="text-rose-500 hover:text-rose-400"><Trash2 size={11} /></button>
                      </div>
                    ))}
                    {trunkACLs.length === 0 && (
                      <div className="text-center py-3">
                        <AlertTriangle size={14} className="text-amber-400 mx-auto mb-1" />
                        <p className="text-[10px] font-mono text-amber-400">No IPs whitelisted — trunk will reject all calls</p>
                      </div>
                    )}
                  </div>

                  {/* Add ACL */}
                  <div className="flex gap-2">
                    <Input value={newACL.ip_address} onChange={e => setNewACL({ ...newACL, ip_address: e.target.value })}
                      placeholder="192.168.1.0" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-xs h-7 flex-1" />
                    <Input type="number" value={newACL.cidr_bits} onChange={e => setNewACL({ ...newACL, cidr_bits: parseInt(e.target.value) || 32 })}
                      className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-xs h-7 w-16" />
                    <Input value={newACL.description} onChange={e => setNewACL({ ...newACL, description: e.target.value })}
                      placeholder="Description" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-xs h-7 flex-1" />
                    <Button onClick={addACL} className="bg-emerald-600 hover:bg-emerald-500 text-white font-mono text-[10px] h-7 px-3">
                      <Plus size={12} />
                    </Button>
                  </div>
                </Card>

                {/* Trunk Security Details */}
                <Card className="bg-[#0f1629] border-cyan-500/10 p-4">
                  <h3 className="text-xs font-mono text-slate-300 font-semibold mb-3">SECURITY</h3>
                  <div className="grid grid-cols-2 gap-3 text-[10px] font-mono">
                    <div>
                      <span className="text-slate-500">Policy</span>
                      <div className="flex items-center gap-1 mt-1">
                        <policyConfig.icon size={12} className={`text-${policyConfig.color}-400`} />
                        <span className={`text-${policyConfig.color}-400`}>{policyConfig.label}</span>
                      </div>
                    </div>
                    <div>
                      <span className="text-slate-500">Transport</span>
                      <p className="text-slate-300 mt-1">{selectedTrunk.transport?.toUpperCase()}</p>
                    </div>
                    <div>
                      <span className="text-slate-500">TLS</span>
                      <p className={`mt-1 ${selectedTrunk.tls_enabled ? "text-emerald-400" : "text-slate-600"}`}>
                        {selectedTrunk.tls_enabled ? "Enabled" : "Disabled"}
                      </p>
                    </div>
                    <div>
                      <span className="text-slate-500">SRTP</span>
                      <p className={`mt-1 ${selectedTrunk.srtp_enabled ? "text-emerald-400" : "text-slate-600"}`}>
                        {selectedTrunk.srtp_enabled ? "Enabled" : "Disabled"}
                      </p>
                    </div>
                    {selectedTrunk.auth_user && (
                      <div className="col-span-2">
                        <span className="text-slate-500">Auth</span>
                        <p className="text-slate-300 mt-1">{selectedTrunk.auth_user}@{selectedTrunk.auth_realm || "voiceagent"}</p>
                      </div>
                    )}
                  </div>
                </Card>
              </div>
            )}
          </div>
        </TabsContent>

        {/* ── Security Log ── */}
        <TabsContent value="security" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 p-4">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-xs font-mono text-slate-300 font-semibold">SIP SECURITY AUDIT LOG</h3>
              <Button onClick={loadSecurityLog} variant="outline" className="font-mono text-[10px] text-slate-400 border-cyan-500/20 h-7">
                <RefreshCw size={10} className="mr-1" /> REFRESH
              </Button>
            </div>
            {securityLog.length === 0 ? (
              <p className="text-center text-sm text-slate-600 py-6">No security events. Click Refresh to load.</p>
            ) : (
              <div className="space-y-1">
                {securityLog.map(log => {
                  const isSuccess = log.event_type.includes("success");
                  const isBlock = log.event_type.includes("blocked") || log.event_type.includes("failure");
                  return (
                    <div key={log.id} className="flex items-center gap-3 px-3 py-2 rounded bg-[#070b14] text-[10px] font-mono">
                      {isSuccess ? <CheckCircle size={12} className="text-emerald-400 shrink-0" /> :
                       isBlock ? <XCircle size={12} className="text-rose-400 shrink-0" /> :
                       <AlertTriangle size={12} className="text-amber-400 shrink-0" />}
                      <span className={`w-24 shrink-0 ${isSuccess ? "text-emerald-400" : isBlock ? "text-rose-400" : "text-amber-400"}`}>{log.event_type}</span>
                      <span className="text-slate-500 w-28 shrink-0">{log.source_ip}</span>
                      <span className="text-slate-400 truncate flex-1">{log.details}</span>
                      <span className="text-slate-600 shrink-0">{new Date(log.created_at).toLocaleTimeString()}</span>
                    </div>
                  );
                })}
              </div>
            )}
          </Card>
        </TabsContent>

        {/* ── LLM Config ── */}
        <TabsContent value="llm" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <h3 className="text-sm font-semibold text-slate-200 tracking-wide mb-4">LLM CONFIGURATION</h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">PROVIDER</label>
                <select value={llm.provider} onChange={e => setLlm({ ...llm, provider: e.target.value })}
                  className="w-full bg-[#070b14] border border-cyan-500/15 rounded-md text-slate-200 font-mono text-sm px-3 py-2">
                  <option value="anthropic-vertex">Anthropic (Vertex AI)</option>
                  <option value="google-vertex">Google Gemini (Vertex AI)</option>
                  <option value="ollama">Ollama (Local)</option>
                </select>
              </div>
              <div>
                <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">MODEL</label>
                <Input value={llm.model} onChange={e => setLlm({ ...llm, model: e.target.value })}
                  placeholder="claude-3-5-haiku@20241022" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
              </div>
              <div>
                <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">REGION</label>
                <Input value={llm.region} onChange={e => setLlm({ ...llm, region: e.target.value })}
                  placeholder="us-east5" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
              </div>
              <div>
                <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">MAX TOKENS</label>
                <Input type="number" value={llm.max_tokens} onChange={e => setLlm({ ...llm, max_tokens: parseInt(e.target.value) || 512 })}
                  className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
              </div>
            </div>
            <div className="flex items-center justify-end mt-4 pt-4 border-t border-cyan-500/10 gap-2">
              {llmSaved && <span className="text-xs font-mono text-emerald-400">Saved</span>}
              <Button onClick={saveLLM} className="bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider">SAVE</Button>
            </div>
          </Card>
          <Card className="bg-[#0f1629] border-cyan-500/10 p-5">
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">TEST LLM</label>
            <div className="flex gap-3">
              <Input value={testPrompt} onChange={e => setTestPrompt(e.target.value)} placeholder="Enter test prompt..."
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
              <Button onClick={testLLM} disabled={testing} className="bg-emerald-600 hover:bg-emerald-500 text-white font-mono text-xs tracking-wider px-6">
                {testing ? "TESTING..." : "TEST"}
              </Button>
            </div>
            {testResult && (
              <div className="mt-3 p-3 rounded bg-[#070b14] border border-emerald-500/15">
                <p className="text-sm text-slate-300 font-mono">{testResult}</p>
              </div>
            )}
          </Card>
        </TabsContent>

        {/* ── System Prompts ── */}
        <TabsContent value="prompts" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-3">INTERACTIVE AGENT PROMPT</label>
            <Textarea value={interactivePrompt} onChange={e => setInteractivePrompt(e.target.value)}
              rows={6} className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm resize-none" />
          </Card>
          <Card className="bg-[#0f1629] border-violet-500/15 p-5">
            <label className="text-[10px] font-mono text-violet-400 tracking-wider block mb-3">CO-PILOT COACH PROMPT</label>
            <Textarea value={copilotPrompt} onChange={e => setCopilotPrompt(e.target.value)}
              rows={6} className="bg-[#070b14] border-violet-500/15 text-slate-200 font-mono text-sm resize-none" />
          </Card>
          <div className="flex items-center justify-end gap-2">
            {promptSaved && <span className="text-xs font-mono text-emerald-400">Saved</span>}
            <Button onClick={savePrompts} className="bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider">SAVE PROMPTS</Button>
          </div>
        </TabsContent>

        {/* ── Service Status ── */}
        <TabsContent value="status" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-slate-200 tracking-wide">SERVICE STATUS</h3>
              <Button onClick={loadServices} variant="outline" className="font-mono text-xs text-slate-400 border-cyan-500/20">REFRESH</Button>
            </div>
            <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
              {services.map(svc => (
                <div key={svc.name} className="flex items-center gap-3 p-3 rounded-md bg-[#070b14]">
                  <span className={`w-2 h-2 rounded-full ${svc.status === "online" ? "bg-emerald-500 animate-pulse" : "bg-rose-500"}`} />
                  <div>
                    <div className="text-sm text-slate-300">{svc.name}</div>
                    <div className="text-[10px] font-mono text-slate-600">{svc.port}</div>
                  </div>
                </div>
              ))}
            </div>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
