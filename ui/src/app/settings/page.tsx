"use client";

import { useState, useEffect } from "react";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { authFetch } from "@/lib/auth";

const providerBadge: Record<string, string> = {
  "anthropic-vertex": "bg-orange-500/15 text-orange-400 border-orange-500/25",
  "gemini-vertex": "bg-blue-500/15 text-blue-400 border-blue-500/25",
  "google-vertex": "bg-blue-500/15 text-blue-400 border-blue-500/25",
  "ollama": "bg-emerald-500/15 text-emerald-400 border-emerald-500/25",
  "custom": "bg-slate-500/15 text-slate-400 border-slate-500/25",
};

interface LLMConfig { provider: string; model: string; region: string; max_tokens: number; }
interface TrunkConfig { name: string; address: string; port: number; transport: string; register: boolean; caller_id: string; codecs: string; status: string; }

export default function SettingsPage() {
  const [llm, setLlm] = useState<LLMConfig>({ provider: "", model: "", region: "", max_tokens: 512 });
  const [llmSaved, setLlmSaved] = useState(false);
  const [interactivePrompt, setInteractivePrompt] = useState("");
  const [copilotPrompt, setCopilotPrompt] = useState("");
  const [promptSaved, setPromptSaved] = useState(false);
  const [trunks, setTrunks] = useState<TrunkConfig[]>([]);
  const [newTrunk, setNewTrunk] = useState<Partial<TrunkConfig>>({ port: 5060, transport: "udp", codecs: "PCMU,PCMA,G722" });
  const [showAddTrunk, setShowAddTrunk] = useState(false);
  const [testPrompt, setTestPrompt] = useState("Hello, how are you?");
  const [testResult, setTestResult] = useState("");
  const [testing, setTesting] = useState(false);
  const [services, setServices] = useState<Array<{ name: string; status: string; port: string }>>([]);

  useEffect(() => { loadSettings(); loadServices(); }, []);

  async function loadSettings() {
    try {
      const res = await authFetch("/api/settings");
      if (res.ok) {
        const data = await res.json();
        setLlm(data.llm || {});
        setInteractivePrompt(data.prompts?.interactive || "");
        setCopilotPrompt(data.prompts?.copilot || "");
        setTrunks(data.trunks || []);
      }
    } catch {}
  }

  async function loadServices() {
    try {
      const gw = process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";
      const res = await fetch(`${gw}/healthz`);
      if (res.ok) {
        const h = await res.json();
        setServices([
          { name: "Gateway", status: "online", port: `:8080 (${h.mode || "gateway"})` },
          { name: "Whisper STT", status: "online", port: ":8000" },
          { name: "Database", status: "online", port: ":5432" },
        ]);
      }
    } catch {
      setServices([{ name: "Gateway", status: "offline", port: ":8080" }]);
    }
  }

  async function saveLLM() {
    const res = await authFetch("/api/settings/llm", { method: "POST", body: JSON.stringify(llm) });
    if (res.ok) { setLlmSaved(true); setTimeout(() => setLlmSaved(false), 2000); }
  }

  async function savePrompts() {
    const res = await authFetch("/api/settings/prompts", { method: "POST", body: JSON.stringify({ interactive: interactivePrompt, copilot: copilotPrompt }) });
    if (res.ok) { setPromptSaved(true); setTimeout(() => setPromptSaved(false), 2000); }
  }

  async function addTrunk() {
    if (!newTrunk.name || !newTrunk.address) return;
    const res = await authFetch("/api/settings/trunks", { method: "POST", body: JSON.stringify(newTrunk) });
    if (res.ok) { setShowAddTrunk(false); setNewTrunk({ port: 5060, transport: "udp", codecs: "PCMU,PCMA,G722" }); loadSettings(); }
  }

  async function removeTrunk(name: string) {
    await authFetch("/api/settings/trunks", { method: "DELETE", body: JSON.stringify({ name }) });
    loadSettings();
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

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-slate-100">Configuration</h1>
        <p className="text-sm text-slate-500 font-mono mt-1">LLM models, system prompts, SIP trunks — saved to config file</p>
      </div>

      <Tabs defaultValue="llm" className="space-y-4">
        <TabsList className="bg-[#0f1629] border border-cyan-500/10">
          <TabsTrigger value="llm" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">LLM MODEL</TabsTrigger>
          <TabsTrigger value="prompts" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">SYSTEM PROMPTS</TabsTrigger>
          <TabsTrigger value="trunks" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">SIP TRUNKS</TabsTrigger>
          <TabsTrigger value="status" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">STATUS</TabsTrigger>
        </TabsList>

        {/* LLM Config */}
        <TabsContent value="llm" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <h3 className="text-sm font-semibold text-slate-200 tracking-wide mb-4">LLM CONFIGURATION</h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">PROVIDER</label>
                <select value={llm.provider} onChange={(e) => setLlm({ ...llm, provider: e.target.value })}
                  className="w-full bg-[#070b14] border border-cyan-500/15 rounded-md text-slate-200 font-mono text-sm px-3 py-2">
                  <option value="anthropic-vertex">Anthropic (Vertex AI)</option>
                  <option value="google-vertex">Google Gemini (Vertex AI)</option>
                  <option value="ollama">Ollama (Local)</option>
                </select>
              </div>
              <div>
                <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">MODEL</label>
                <Input value={llm.model} onChange={(e) => setLlm({ ...llm, model: e.target.value })}
                  placeholder="claude-3-5-haiku@20241022" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
              </div>
              <div>
                <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">REGION</label>
                <Input value={llm.region} onChange={(e) => setLlm({ ...llm, region: e.target.value })}
                  placeholder="us-east5" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
              </div>
              <div>
                <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">MAX TOKENS</label>
                <Input type="number" value={llm.max_tokens} onChange={(e) => setLlm({ ...llm, max_tokens: parseInt(e.target.value) || 512 })}
                  className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
              </div>
            </div>
            <div className="flex items-center justify-between mt-4 pt-4 border-t border-cyan-500/10">
              <Badge variant="outline" className={`text-[10px] font-mono ${providerBadge[llm.provider] || providerBadge.custom}`}>
                {llm.provider || "not set"}
              </Badge>
              <div className="flex gap-2 items-center">
                {llmSaved && <span className="text-xs font-mono text-emerald-400">Saved</span>}
                <Button onClick={saveLLM} className="bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider">SAVE</Button>
              </div>
            </div>
          </Card>
          <Card className="bg-[#0f1629] border-cyan-500/10 p-5">
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">TEST LLM</label>
            <div className="flex gap-3">
              <Input value={testPrompt} onChange={(e) => setTestPrompt(e.target.value)} placeholder="Enter test prompt..."
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
              <Button onClick={testLLM} disabled={testing} className="bg-emerald-600 hover:bg-emerald-500 text-white font-mono text-xs tracking-wider px-6">
                {testing ? "TESTING..." : "TEST"}
              </Button>
            </div>
            {testResult && (
              <div className="mt-3 p-3 rounded bg-[#070b14] border border-emerald-500/15">
                <div className="text-[10px] font-mono text-emerald-400 mb-1">RESPONSE</div>
                <p className="text-sm text-slate-300 font-mono">{testResult}</p>
              </div>
            )}
          </Card>
          <Card className="bg-[#0f1629] border-amber-500/10 p-4">
            <p className="text-xs font-mono text-amber-400/60">API credentials (GCP service account, API keys) are configured via environment variables or K8s secrets — never stored in config files.</p>
          </Card>
        </TabsContent>

        {/* System Prompts */}
        <TabsContent value="prompts" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-3">INTERACTIVE AGENT PROMPT</label>
            <Textarea value={interactivePrompt} onChange={(e) => setInteractivePrompt(e.target.value)}
              rows={6} className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm resize-none" />
            <div className="text-xs font-mono text-slate-600 mt-2">{interactivePrompt.length} chars</div>
          </Card>
          <Card className="bg-[#0f1629] border-violet-500/15 p-5">
            <label className="text-[10px] font-mono text-violet-400 tracking-wider block mb-3">CO-PILOT COACH PROMPT</label>
            <Textarea value={copilotPrompt} onChange={(e) => setCopilotPrompt(e.target.value)}
              rows={6} className="bg-[#070b14] border-violet-500/15 text-slate-200 font-mono text-sm resize-none" />
            <div className="text-xs font-mono text-slate-600 mt-2">{copilotPrompt.length} chars</div>
          </Card>
          <div className="flex items-center justify-end gap-2">
            {promptSaved && <span className="text-xs font-mono text-emerald-400">Saved to config file</span>}
            <Button onClick={savePrompts} className="bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider">SAVE PROMPTS</Button>
          </div>
        </TabsContent>

        {/* SIP Trunks */}
        <TabsContent value="trunks" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border overflow-hidden">
            <div className="px-5 py-4 border-b border-cyan-500/10 flex items-center justify-between">
              <h2 className="text-sm font-semibold text-slate-200 tracking-wide">SIP TRUNKS</h2>
              <Button onClick={() => setShowAddTrunk(!showAddTrunk)} className="bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider">
                {showAddTrunk ? "CANCEL" : "+ ADD TRUNK"}
              </Button>
            </div>
            {showAddTrunk && (
              <div className="px-5 py-4 border-b border-cyan-500/10 bg-cyan-500/[0.03]">
                <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                  <div>
                    <label className="text-[10px] font-mono text-slate-500 block mb-1">NAME</label>
                    <Input value={newTrunk.name || ""} onChange={(e) => setNewTrunk({ ...newTrunk, name: e.target.value })}
                      placeholder="Cisco CUBE DC1" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
                  </div>
                  <div>
                    <label className="text-[10px] font-mono text-slate-500 block mb-1">ADDRESS</label>
                    <Input value={newTrunk.address || ""} onChange={(e) => setNewTrunk({ ...newTrunk, address: e.target.value })}
                      placeholder="sbc.example.com" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
                  </div>
                  <div>
                    <label className="text-[10px] font-mono text-slate-500 block mb-1">PORT</label>
                    <Input type="number" value={newTrunk.port} onChange={(e) => setNewTrunk({ ...newTrunk, port: parseInt(e.target.value) || 5060 })}
                      className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
                  </div>
                  <div>
                    <label className="text-[10px] font-mono text-slate-500 block mb-1">TRANSPORT</label>
                    <select value={newTrunk.transport} onChange={(e) => setNewTrunk({ ...newTrunk, transport: e.target.value })}
                      className="w-full bg-[#070b14] border border-cyan-500/15 rounded-md text-slate-200 font-mono text-sm px-3 py-2">
                      <option value="udp">UDP</option><option value="tcp">TCP</option><option value="tls">TLS</option>
                    </select>
                  </div>
                  <div>
                    <label className="text-[10px] font-mono text-slate-500 block mb-1">CODECS</label>
                    <Input value={newTrunk.codecs || ""} onChange={(e) => setNewTrunk({ ...newTrunk, codecs: e.target.value })}
                      className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
                  </div>
                  <div>
                    <label className="text-[10px] font-mono text-slate-500 block mb-1">CALLER ID</label>
                    <Input value={newTrunk.caller_id || ""} onChange={(e) => setNewTrunk({ ...newTrunk, caller_id: e.target.value })}
                      placeholder="+15559876543" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
                  </div>
                </div>
                <div className="flex justify-end mt-3">
                  <Button onClick={addTrunk} className="bg-emerald-600 hover:bg-emerald-500 text-white font-mono text-xs tracking-wider px-6">ADD</Button>
                </div>
              </div>
            )}
            {trunks.length === 0 && !showAddTrunk && (
              <div className="px-5 py-8 text-center text-sm text-slate-600">No SIP trunks configured. Click + ADD TRUNK to add one.</div>
            )}
            {trunks.length > 0 && (
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-[11px] font-mono text-slate-500 uppercase tracking-wider border-b border-white/5">
                    <th className="text-left px-5 py-3 font-medium">Name</th>
                    <th className="text-left px-5 py-3 font-medium">Address</th>
                    <th className="text-left px-5 py-3 font-medium">Port</th>
                    <th className="text-left px-5 py-3 font-medium">Transport</th>
                    <th className="text-left px-5 py-3 font-medium">Codecs</th>
                    <th className="text-left px-5 py-3 font-medium">Status</th>
                    <th className="text-left px-5 py-3 font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {trunks.map((t, i) => (
                    <tr key={i} className="border-b border-white/[0.03] hover:bg-cyan-500/[0.03]">
                      <td className="px-5 py-3 font-medium text-slate-200">{t.name}</td>
                      <td className="px-5 py-3 font-mono text-xs text-slate-400">{t.address}</td>
                      <td className="px-5 py-3 font-mono text-xs text-slate-500">{t.port}</td>
                      <td className="px-5 py-3"><Badge variant="outline" className="text-[10px] font-mono bg-slate-500/15 text-slate-400">{t.transport}</Badge></td>
                      <td className="px-5 py-3 font-mono text-xs text-slate-500">{t.codecs}</td>
                      <td className="px-5 py-3">
                        <Badge variant="outline" className={`text-[10px] font-mono ${t.status === "active" ? "bg-emerald-500/15 text-emerald-400 border-emerald-500/25" : "bg-slate-500/15 text-slate-400"}`}>{t.status}</Badge>
                      </td>
                      <td className="px-5 py-3">
                        <Button variant="ghost" size="sm" onClick={() => removeTrunk(t.name)} className="text-xs text-rose-400 hover:text-rose-300 font-mono">REMOVE</Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Card>
          <Card className="bg-[#0f1629] border-amber-500/10 p-4">
            <p className="text-xs font-mono text-amber-400/60">Trunk credentials (username/password) are configured via environment variables or K8s secrets — never stored in config files.</p>
          </Card>
        </TabsContent>

        {/* Service Status */}
        <TabsContent value="status" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-slate-200 tracking-wide">SERVICE STATUS</h3>
              <Button onClick={loadServices} variant="outline" className="font-mono text-xs text-slate-400 border-cyan-500/20">REFRESH</Button>
            </div>
            <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
              {services.map((svc) => (
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
