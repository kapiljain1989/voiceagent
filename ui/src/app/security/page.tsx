"use client";

import { useState, useEffect } from "react";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { authFetch } from "@/lib/auth";

interface PIIRule { name: string; regex: string; mask: string; level: string; is_default: boolean; }
interface RobocallKeyword { phrase: string; weight: number; category: string; is_default: boolean; }
interface BioConfig { key: string; value: string; }

export default function SecurityPage() {
  // Dynamic rules from API
  const [piiRules, setPiiRules] = useState<PIIRule[]>([]);
  const [robocallKeywords, setRobocallKeywords] = useState<RobocallKeyword[]>([]);
  const [bioConfig, setBioConfig] = useState<BioConfig[]>([]);

  // Add PII pattern form
  const [showAddPII, setShowAddPII] = useState(false);
  const [newPII, setNewPII] = useState({ name: "", regex: "", mask: "[REDACTED]", level: "high" });

  // Add robocall keyword form
  const [showAddKeyword, setShowAddKeyword] = useState(false);
  const [newKeyword, setNewKeyword] = useState({ phrase: "", weight: 1.0, category: "spam" });

  // Robocall
  const [blockNumber, setBlockNumber] = useState("");
  const [blockReason, setBlockReason] = useState("");
  const [blocklist, setBlocklist] = useState<Array<{ number: string; reason: string; source?: string; created_at?: string }>>([]);
  const [robocallText, setRobocallText] = useState("");
  const [robocallResult, setRobocallResult] = useState<any>(null);
  const [robocallStats, setRobocallStats] = useState<any>(null);

  // PII
  const [piiText, setPiiText] = useState("");
  const [piiResult, setPiiResult] = useState<any>(null);
  const [piiEnabled, setPiiEnabled] = useState(true);

  // Voice biometrics
  const [voiceprints, setVoiceprints] = useState<any[]>([]);
  const [vpLabel, setVpLabel] = useState("");
  const [vpType, setVpType] = useState("fraud");
  const [vpCallerID, setVpCallerID] = useState("");

  // Load on mount
  useEffect(() => {
    loadBlocklist();
    loadRobocallStats();
    loadVoiceprints();
    loadPIIRules();
    loadRobocallKeywords();
    loadBioConfig();
  }, []);

  async function loadPIIRules() {
    try { const res = await authFetch("/api/security/rules/pii"); if (res.ok) setPiiRules(await res.json()); } catch {}
  }
  async function addPIIRule() {
    if (!newPII.name || !newPII.regex) return;
    await authFetch("/api/security/rules/pii", { method: "POST", body: JSON.stringify(newPII) });
    setNewPII({ name: "", regex: "", mask: "[REDACTED]", level: "high" }); setShowAddPII(false); loadPIIRules();
  }
  async function removePIIRule(name: string) {
    await authFetch("/api/security/rules/pii", { method: "DELETE", body: JSON.stringify({ name }) }); loadPIIRules();
  }

  async function loadRobocallKeywords() {
    try { const res = await authFetch("/api/security/rules/robocall"); if (res.ok) setRobocallKeywords(await res.json()); } catch {}
  }
  async function addRobocallKeyword() {
    if (!newKeyword.phrase) return;
    await authFetch("/api/security/rules/robocall", { method: "POST", body: JSON.stringify(newKeyword) });
    setNewKeyword({ phrase: "", weight: 1.0, category: "spam" }); setShowAddKeyword(false); loadRobocallKeywords();
  }
  async function removeRobocallKeyword(phrase: string) {
    await authFetch("/api/security/rules/robocall", { method: "DELETE", body: JSON.stringify({ phrase }) }); loadRobocallKeywords();
  }

  async function loadBioConfig() {
    try { const res = await authFetch("/api/security/rules/biometric"); if (res.ok) setBioConfig(await res.json()); } catch {}
  }
  async function saveBioConfig(key: string, value: string) {
    await authFetch("/api/security/rules/biometric", { method: "POST", body: JSON.stringify({ key, value }) }); loadBioConfig();
  }

  async function loadBlocklist() {
    try { const res = await authFetch("/api/blocklist"); if (res.ok) setBlocklist(await res.json()); } catch {}
  }

  async function addToBlocklist() {
    if (!blockNumber) return;
    await authFetch("/api/blocklist", { method: "POST", body: JSON.stringify({ number: blockNumber, reason: blockReason || "manual" }) });
    setBlockNumber(""); setBlockReason("");
    loadBlocklist(); loadRobocallStats();
  }

  async function removeFromBlocklist(number: string) {
    await authFetch("/api/blocklist", { method: "DELETE", body: JSON.stringify({ number }) });
    loadBlocklist(); loadRobocallStats();
  }

  async function testRobocall() {
    if (!robocallText) return;
    const res = await authFetch("/api/robocall/test", { method: "POST", body: JSON.stringify({ text: robocallText }) });
    if (res.ok) setRobocallResult(await res.json());
  }

  async function loadRobocallStats() {
    try { const res = await authFetch("/api/robocall/stats"); if (res.ok) setRobocallStats(await res.json()); } catch {}
  }

  async function testPII() {
    if (!piiText) return;
    const res = await authFetch("/api/security/pii/test", { method: "POST", body: JSON.stringify({ text: piiText }) });
    if (res.ok) setPiiResult(await res.json());
  }

  async function togglePII() {
    const newState = !piiEnabled;
    await authFetch("/api/security/pii/config", { method: "POST", body: JSON.stringify({ enabled: newState }) });
    setPiiEnabled(newState);
  }

  async function loadVoiceprints() {
    try { const res = await authFetch("/api/security/voiceprints"); if (res.ok) setVoiceprints(await res.json()); } catch {}
  }

  async function enrollVoiceprint() {
    if (!vpLabel) return;
    await authFetch("/api/security/voiceprints", {
      method: "POST",
      body: JSON.stringify({ label: vpLabel, type: vpType, caller_id: vpCallerID }),
    });
    setVpLabel(""); setVpCallerID("");
    loadVoiceprints();
  }

  async function deleteVoiceprint(id: string) {
    await authFetch("/api/security/voiceprints", { method: "DELETE", body: JSON.stringify({ id }) });
    loadVoiceprints();
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-slate-100">Security & Compliance</h1>
        <p className="text-sm text-slate-500 font-mono mt-1">Robocall detection, PII masking, voice biometrics — configure and test</p>
      </div>

      {/* Overview stats */}
      <div className="grid grid-cols-2 md:grid-cols-5 gap-3">
        <Card className="bg-[#0f1629] border-rose-500/10 p-4 text-center">
          <div className="text-[10px] font-mono text-slate-500 mb-1">BLOCKLIST</div>
          <div className="font-mono text-2xl text-rose-400">{robocallStats?.blocklist_size || blocklist.length || 0}</div>
        </Card>
        <Card className="bg-[#0f1629] border-rose-500/10 p-4 text-center">
          <div className="text-[10px] font-mono text-slate-500 mb-1">ROBOCALLS</div>
          <div className="font-mono text-2xl text-rose-400">{robocallStats?.robocalls_detected || 0}</div>
        </Card>
        <Card className="bg-[#0f1629] border-amber-500/10 p-4 text-center">
          <div className="text-[10px] font-mono text-slate-500 mb-1">PII PATTERNS</div>
          <div className="font-mono text-2xl text-amber-400">{piiRules.length || 9}</div>
        </Card>
        <Card className="bg-[#0f1629] border-amber-500/10 p-4 text-center">
          <div className="text-[10px] font-mono text-slate-500 mb-1">PII ENGINE</div>
          <div className={`font-mono text-lg ${piiEnabled ? "text-emerald-400" : "text-rose-400"}`}>{piiEnabled ? "ON" : "OFF"}</div>
        </Card>
        <Card className="bg-[#0f1629] border-violet-500/10 p-4 text-center">
          <div className="text-[10px] font-mono text-slate-500 mb-1">VOICEPRINTS</div>
          <div className="font-mono text-2xl text-violet-400">{voiceprints.length}</div>
        </Card>
      </div>

      <Tabs defaultValue="robocall" className="space-y-4">
        <TabsList className="bg-[#0f1629] border border-cyan-500/10">
          <TabsTrigger value="robocall" className="font-mono text-xs data-[state=active]:bg-rose-500/15 data-[state=active]:text-rose-400">ROBOCALL</TabsTrigger>
          <TabsTrigger value="pii" className="font-mono text-xs data-[state=active]:bg-amber-500/15 data-[state=active]:text-amber-400">PII MASKING</TabsTrigger>
          <TabsTrigger value="biometrics" className="font-mono text-xs data-[state=active]:bg-violet-500/15 data-[state=active]:text-violet-400">VOICE BIOMETRICS</TabsTrigger>
        </TabsList>

        {/* Robocall */}
        <TabsContent value="robocall" className="space-y-4">
          {/* Test */}
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">TEST ROBOCALL DETECTION (3-LAYER)</label>
            <p className="text-xs text-slate-600 mb-3">Layer 1: Blocklist (&lt;1ms) | Layer 2: Audio pattern | Layer 3: 28 keyword phrases</p>
            <div className="flex gap-3">
              <Input placeholder="Press 1 for your auto warranty..." value={robocallText} onChange={(e) => setRobocallText(e.target.value)}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm placeholder:text-slate-700" />
              <Button onClick={testRobocall} className="bg-rose-600 hover:bg-rose-500 text-white font-mono text-xs tracking-wider px-6">ANALYZE</Button>
            </div>
            {robocallResult && (
              <div className="mt-4 p-3 rounded-md bg-[#070b14] border border-cyan-500/10">
                <div className="flex items-center gap-3 mb-2">
                  <Badge variant="outline" className={`font-mono text-[10px] ${
                    robocallResult?.keyword?.category === "robocall" ? "bg-rose-500/20 text-rose-400 border-rose-500/30" :
                    robocallResult?.keyword?.category === "uncertain" ? "bg-amber-500/20 text-amber-400 border-amber-500/30" :
                    "bg-emerald-500/20 text-emerald-400 border-emerald-500/30"
                  }`}>{robocallResult?.keyword?.category?.toUpperCase()}</Badge>
                  <span className="font-mono text-sm text-cyan-400">Score: {((robocallResult?.keyword?.score || 0) * 100).toFixed(0)}%</span>
                </div>
                {robocallResult?.keyword?.keywords?.length > 0 && (
                  <div className="flex flex-wrap gap-1.5 mt-2">
                    {robocallResult.keyword.keywords.map((kw: string, i: number) => (
                      <Badge key={i} variant="outline" className="text-[10px] font-mono bg-rose-500/10 text-rose-400 border-rose-500/20">{kw}</Badge>
                    ))}
                  </div>
                )}
              </div>
            )}
          </Card>

          {/* Blocklist */}
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-slate-200 tracking-wide">BLOCKLIST MANAGEMENT</h3>
              <Button onClick={() => { loadBlocklist(); loadRobocallStats(); }} variant="outline" className="font-mono text-xs text-slate-400 border-cyan-500/20">REFRESH</Button>
            </div>
            <div className="flex gap-3 mb-4">
              <Input placeholder="+15551234567" value={blockNumber} onChange={(e) => setBlockNumber(e.target.value)}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm w-48" />
              <Input placeholder="Reason (spam, fraud, etc.)" value={blockReason} onChange={(e) => setBlockReason(e.target.value)}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm flex-1" />
              <Button onClick={addToBlocklist} className="bg-rose-600 hover:bg-rose-500 text-white font-mono text-xs px-4">BLOCK</Button>
            </div>
            {blocklist.length > 0 ? (
              <div className="space-y-1">
                {blocklist.map((entry, i) => (
                  <div key={i} className="flex items-center justify-between p-2.5 rounded bg-[#070b14]">
                    <div className="flex items-center gap-3">
                      <span className="font-mono text-sm text-slate-300">{entry.number}</span>
                      <span className="text-xs text-slate-500">{entry.reason}</span>
                      {entry.source && <Badge variant="outline" className="text-[9px] font-mono bg-slate-500/10 text-slate-500">{entry.source}</Badge>}
                    </div>
                    <Button onClick={() => removeFromBlocklist(entry.number)} variant="ghost" className="text-xs text-rose-400 hover:text-rose-300 font-mono">REMOVE</Button>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-slate-600 text-center py-4">Blocklist empty. Add numbers above to block known spam callers.</p>
            )}
          </Card>
          {/* Robocall Keywords */}
          <Card className="bg-[#0f1629] border-rose-500/10 p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-slate-200 tracking-wide">DETECTION KEYWORDS ({robocallKeywords.length})</h3>
              <div className="flex gap-2">
                <Button onClick={() => setShowAddKeyword(!showAddKeyword)} variant="outline" className="font-mono text-xs text-rose-400 border-rose-500/20">
                  {showAddKeyword ? "CANCEL" : "+ ADD KEYWORD"}
                </Button>
                <Button onClick={loadRobocallKeywords} variant="outline" className="font-mono text-xs text-slate-400 border-cyan-500/20">REFRESH</Button>
              </div>
            </div>
            {showAddKeyword && (
              <div className="mb-4 p-3 rounded bg-rose-500/[0.03] border border-rose-500/10">
                <div className="flex gap-2">
                  <Input placeholder="Keyword phrase" value={newKeyword.phrase} onChange={(e) => setNewKeyword({...newKeyword, phrase: e.target.value})}
                    className="bg-[#070b14] border-rose-500/15 text-slate-200 font-mono text-sm flex-1" />
                  <Input type="number" step="0.1" placeholder="Weight" value={newKeyword.weight} onChange={(e) => setNewKeyword({...newKeyword, weight: parseFloat(e.target.value) || 1.0})}
                    className="bg-[#070b14] border-rose-500/15 text-slate-200 font-mono text-sm w-20" />
                  <Button onClick={addRobocallKeyword} className="bg-rose-600 hover:bg-rose-500 text-white font-mono text-xs px-4">ADD</Button>
                </div>
              </div>
            )}
            <div className="flex flex-wrap gap-1.5">
              {robocallKeywords.map((kw, i) => (
                <div key={i} className="group flex items-center gap-1 px-2 py-1 rounded bg-[#070b14] border border-rose-500/10">
                  <span className="font-mono text-xs text-slate-400">{kw.phrase}</span>
                  <span className="font-mono text-[9px] text-slate-600">({kw.weight})</span>
                  {!kw.is_default && (
                    <button onClick={() => removeRobocallKeyword(kw.phrase)} className="text-rose-500/50 hover:text-rose-400 ml-0.5 hidden group-hover:inline">x</button>
                  )}
                </div>
              ))}
            </div>
          </Card>
        </TabsContent>

        {/* PII Masking */}
        <TabsContent value="pii" className="space-y-4">
          <Card className="bg-[#0f1629] border-amber-500/15 p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-slate-200 tracking-wide">PII MASKING ENGINE — {piiRules.length} PATTERNS</h3>
              <div className="flex gap-2">
                <Button onClick={() => setShowAddPII(!showAddPII)} variant="outline" className="font-mono text-xs text-amber-400 border-amber-500/20">
                  {showAddPII ? "CANCEL" : "+ ADD PATTERN"}
                </Button>
                <Button onClick={togglePII} variant="outline" className={`font-mono text-xs ${piiEnabled ? "text-emerald-400 border-emerald-500/30" : "text-rose-400 border-rose-500/30"}`}>
                  {piiEnabled ? "ENABLED" : "DISABLED"}
                </Button>
              </div>
            </div>
            {showAddPII && (
              <div className="mb-4 p-3 rounded bg-amber-500/[0.03] border border-amber-500/10">
                <div className="grid grid-cols-1 md:grid-cols-4 gap-2">
                  <Input placeholder="Pattern name" value={newPII.name} onChange={(e) => setNewPII({...newPII, name: e.target.value})}
                    className="bg-[#070b14] border-amber-500/15 text-slate-200 font-mono text-sm" />
                  <Input placeholder="Regex (e.g. \b\d{10}\b)" value={newPII.regex} onChange={(e) => setNewPII({...newPII, regex: e.target.value})}
                    className="bg-[#070b14] border-amber-500/15 text-slate-200 font-mono text-sm" />
                  <Input placeholder="Mask text" value={newPII.mask} onChange={(e) => setNewPII({...newPII, mask: e.target.value})}
                    className="bg-[#070b14] border-amber-500/15 text-slate-200 font-mono text-sm" />
                  <div className="flex gap-2">
                    <select value={newPII.level} onChange={(e) => setNewPII({...newPII, level: e.target.value})}
                      className="bg-[#070b14] border border-amber-500/15 rounded-md text-slate-200 font-mono text-sm px-2 flex-1">
                      <option value="critical">Critical</option><option value="high">High</option><option value="medium">Medium</option>
                    </select>
                    <Button onClick={addPIIRule} className="bg-amber-600 hover:bg-amber-500 text-white font-mono text-xs px-4">ADD</Button>
                  </div>
                </div>
              </div>
            )}
            <div className="space-y-1">
              {piiRules.map((p) => (
                <div key={p.name} className="flex items-center justify-between p-2 rounded bg-[#070b14]">
                  <div className="flex items-center gap-2">
                    <Badge variant="outline" className={`text-[9px] font-mono ${p.level === "critical" ? "bg-rose-500/10 text-rose-400 border-rose-500/20" : "bg-amber-500/10 text-amber-400 border-amber-500/20"}`}>
                      {p.level}
                    </Badge>
                    <span className="font-mono text-sm text-slate-300">{p.name}</span>
                    {p.is_default && <Badge variant="outline" className="text-[8px] font-mono bg-slate-500/10 text-slate-500">BUILT-IN</Badge>}
                    <span className="font-mono text-[10px] text-slate-600 hidden md:inline">{p.mask}</span>
                  </div>
                  {!p.is_default && (
                    <Button onClick={() => removePIIRule(p.name)} variant="ghost" className="text-xs text-rose-400 hover:text-rose-300 font-mono">REMOVE</Button>
                  )}
                </div>
              ))}
              {piiRules.length === 0 && <p className="text-sm text-slate-600 text-center py-2">Loading patterns...</p>}
            </div>
          </Card>

          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">TEST PII DETECTION</label>
            <div className="flex gap-3">
              <Input placeholder="My credit card is 4111 1111 1111 1111 and SSN is 123-45-6789 dob 16121968"
                value={piiText} onChange={(e) => setPiiText(e.target.value)}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm placeholder:text-slate-700" />
              <Button onClick={testPII} className="bg-amber-600 hover:bg-amber-500 text-white font-mono text-xs tracking-wider px-6">SCAN</Button>
            </div>
            {piiResult && (
              <div className="mt-4 space-y-3">
                <Badge variant="outline" className={`font-mono text-[10px] ${piiResult.pii_found ? "bg-rose-500/20 text-rose-400 border-rose-500/30" : "bg-emerald-500/20 text-emerald-400 border-emerald-500/30"}`}>
                  {piiResult.pii_found ? `${piiResult.detections?.length} PII FOUND` : "CLEAN — NO PII DETECTED"}
                </Badge>
                {piiResult.pii_found && (
                  <>
                    <div className="p-3 rounded bg-rose-500/[0.06] border border-rose-500/15">
                      <div className="text-[10px] font-mono text-rose-400 mb-1">ORIGINAL</div>
                      <p className="text-sm text-slate-300 font-mono">{piiResult.original}</p>
                    </div>
                    <div className="p-3 rounded bg-emerald-500/[0.06] border border-emerald-500/15">
                      <div className="text-[10px] font-mono text-emerald-400 mb-1">MASKED (sent to LLM)</div>
                      <p className="text-sm text-slate-300 font-mono">{piiResult.masked}</p>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      {piiResult.detections?.map((d: any, i: number) => (
                        <Badge key={i} variant="outline" className={`text-[10px] font-mono ${d.level === "critical" ? "bg-rose-500/15 text-rose-400 border-rose-500/25" : "bg-amber-500/15 text-amber-400 border-amber-500/25"}`}>
                          {d.type} ({d.level})
                        </Badge>
                      ))}
                    </div>
                  </>
                )}
              </div>
            )}
          </Card>
        </TabsContent>

        {/* Voice Biometrics */}
        <TabsContent value="biometrics" className="space-y-4">
          <Card className="bg-[#0f1629] border-violet-500/15 p-5">
            <h3 className="text-sm font-semibold text-violet-300 tracking-wide mb-2">VOICE BIOMETRICS</h3>
            <p className="text-xs text-slate-600 mb-4">32-dimensional spectral fingerprint from raw PCM audio. Fraud profiles are matched during live calls. Verified profiles authenticate callers.</p>

            {/* Biometric Config */}
            <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
              {bioConfig.map((c) => (
                <div key={c.key} className="p-3 rounded bg-[#070b14]">
                  <div className="text-[10px] font-mono text-slate-500 mb-1">{c.key.replace(/_/g, " ").toUpperCase()}</div>
                  <Input value={c.value}
                    onChange={(e) => setBioConfig(prev => prev.map(p => p.key === c.key ? {...p, value: e.target.value} : p))}
                    onBlur={(e) => saveBioConfig(c.key, e.target.value)}
                    className="bg-[#070b14] border-violet-500/15 text-violet-300 font-mono text-sm h-8" />
                </div>
              ))}
              {bioConfig.length === 0 && <p className="text-xs text-slate-600 col-span-4">No biometric config. Database required.</p>}
            </div>
          </Card>

          <Card className="bg-[#0f1629] border-violet-500/15 p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-violet-300 tracking-wide">ENROLL VOICE PRINT</h3>
              <Button onClick={loadVoiceprints} variant="outline" className="font-mono text-xs text-slate-400 border-violet-500/20">REFRESH</Button>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-4 gap-3 mb-4">
              <Input placeholder="Profile label (e.g. fraud_caller_001)" value={vpLabel} onChange={(e) => setVpLabel(e.target.value)}
                className="bg-[#070b14] border-violet-500/15 text-slate-200 font-mono text-sm" />
              <Input placeholder="Caller ID (optional)" value={vpCallerID} onChange={(e) => setVpCallerID(e.target.value)}
                className="bg-[#070b14] border-violet-500/15 text-slate-200 font-mono text-sm" />
              <select value={vpType} onChange={(e) => setVpType(e.target.value)}
                className="bg-[#070b14] border border-violet-500/15 rounded-md text-slate-200 font-mono text-sm px-3">
                <option value="fraud">Fraud Profile</option>
                <option value="verified">Verified Identity</option>
              </select>
              <Button onClick={enrollVoiceprint} className="bg-violet-600 hover:bg-violet-500 text-white font-mono text-xs px-6">ENROLL</Button>
            </div>

            {voiceprints.length > 0 ? (
              <div className="space-y-2">
                {voiceprints.map((vp: any, i: number) => (
                  <div key={i} className="flex items-center justify-between p-3 rounded bg-[#070b14]">
                    <div className="flex items-center gap-3">
                      <Badge variant="outline" className={`text-[10px] font-mono ${vp.type === "fraud" ? "bg-rose-500/15 text-rose-400 border-rose-500/25" : "bg-emerald-500/15 text-emerald-400 border-emerald-500/25"}`}>
                        {vp.type?.toUpperCase()}
                      </Badge>
                      <span className="font-mono text-sm text-slate-300">{vp.label}</span>
                      {vp.caller_id && <span className="font-mono text-xs text-slate-500">{vp.caller_id}</span>}
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="text-[10px] font-mono text-slate-600">{vp.features_dim || 32}d</span>
                      <Button onClick={() => deleteVoiceprint(vp.id)} variant="ghost" className="text-xs text-rose-400 hover:text-rose-300 font-mono">DELETE</Button>
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-slate-600 text-center py-4">No voice prints enrolled. Voice prints are created automatically during calls or manually above.</p>
            )}
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
