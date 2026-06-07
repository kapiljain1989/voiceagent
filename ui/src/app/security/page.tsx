"use client";

import { useState } from "react";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { authFetch } from "@/lib/auth";

export default function SecurityPage() {
  // Robocall state
  const [blockNumber, setBlockNumber] = useState("");
  const [blockReason, setBlockReason] = useState("");
  const [blocklist, setBlocklist] = useState<Array<{number: string; reason: string}>>([]);
  const [robocallText, setRobocallText] = useState("");
  const [robocallResult, setRobocallResult] = useState<any>(null);
  const [robocallStats, setRobocallStats] = useState<any>(null);

  // PII state
  const [piiText, setPiiText] = useState("");
  const [piiResult, setPiiResult] = useState<any>(null);
  const [piiEnabled, setPiiEnabled] = useState(true);

  // Voice biometrics state
  const [voiceprints, setVoiceprints] = useState<any[]>([]);
  const [vpLabel, setVpLabel] = useState("");
  const [vpType, setVpType] = useState("fraud");

  async function loadBlocklist() {
    const res = await authFetch("/api/blocklist");
    setBlocklist(await res.json());
  }

  async function addToBlocklist() {
    await authFetch("/api/blocklist", {
      method: "POST",
      body: JSON.stringify({number: blockNumber, reason: blockReason}),
    });
    setBlockNumber(""); setBlockReason("");
    loadBlocklist();
  }

  async function removeFromBlocklist(number: string) {
    await authFetch("/api/blocklist", {
      method: "DELETE",
      body: JSON.stringify({number}),
    });
    loadBlocklist();
  }

  async function testRobocall() {
    const res = await authFetch("/api/robocall/test", {
      method: "POST",
      body: JSON.stringify({text: robocallText}),
    });
    setRobocallResult(await res.json());
  }

  async function loadRobocallStats() {
    const res = await authFetch("/api/robocall/stats");
    setRobocallStats(await res.json());
  }

  async function testPII() {
    const res = await authFetch("/api/security/pii/test", {
      method: "POST",
      body: JSON.stringify({text: piiText}),
    });
    setPiiResult(await res.json());
  }

  async function togglePII() {
    const newState = !piiEnabled;
    await authFetch("/api/security/pii/config", {
      method: "POST",
      body: JSON.stringify({enabled: newState}),
    });
    setPiiEnabled(newState);
  }

  async function loadVoiceprints() {
    const res = await authFetch("/api/security/voiceprints");
    setVoiceprints(await res.json());
  }

  async function enrollVoiceprint() {
    await authFetch("/api/security/voiceprints", {
      method: "POST",
      body: JSON.stringify({label: vpLabel, type: vpType}),
    });
    setVpLabel("");
    loadVoiceprints();
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-slate-100">Security & Compliance</h1>
        <p className="text-sm text-slate-500 font-mono mt-1">Robocall detection, PII masking, voice biometrics</p>
      </div>

      <Tabs defaultValue="robocall" className="space-y-4">
        <TabsList className="bg-[#0f1629] border border-cyan-500/10">
          <TabsTrigger value="robocall" className="font-mono text-xs data-[state=active]:bg-rose-500/15 data-[state=active]:text-rose-400">
            ROBOCALL
          </TabsTrigger>
          <TabsTrigger value="pii" className="font-mono text-xs data-[state=active]:bg-amber-500/15 data-[state=active]:text-amber-400">
            PII MASKING
          </TabsTrigger>
          <TabsTrigger value="biometrics" className="font-mono text-xs data-[state=active]:bg-violet-500/15 data-[state=active]:text-violet-400">
            VOICE BIOMETRICS
          </TabsTrigger>
        </TabsList>

        {/* Robocall Tab */}
        <TabsContent value="robocall" className="space-y-4">
          {/* Test panel */}
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">TEST ROBOCALL DETECTION</label>
            <div className="flex gap-3">
              <Input placeholder="Enter suspicious text to classify..." value={robocallText}
                onChange={(e) => setRobocallText(e.target.value)}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm placeholder:text-slate-700" />
              <Button onClick={testRobocall} className="bg-rose-600 hover:bg-rose-500 text-white font-mono text-xs tracking-wider px-6">
                ANALYZE
              </Button>
            </div>
            {robocallResult && (
              <div className="mt-4 p-3 rounded-md bg-[#070b14] border border-cyan-500/10">
                <div className="flex items-center gap-3 mb-2">
                  <Badge variant="outline" className={`font-mono text-[10px] ${
                    robocallResult?.keyword?.category === "robocall" ? "bg-rose-500/20 text-rose-400 border-rose-500/30" :
                    robocallResult?.keyword?.category === "uncertain" ? "bg-amber-500/20 text-amber-400 border-amber-500/30" :
                    "bg-emerald-500/20 text-emerald-400 border-emerald-500/30"
                  }`}>
                    {robocallResult?.keyword?.category?.toUpperCase() || "UNKNOWN"}
                  </Badge>
                  <span className="font-mono text-sm text-cyan-400">
                    Score: {((robocallResult?.keyword?.score || 0) * 100).toFixed(0)}%
                  </span>
                </div>
                {robocallResult?.keyword?.keywords?.length > 0 && (
                  <div className="flex flex-wrap gap-1.5 mt-2">
                    {robocallResult.keyword.keywords.map((kw: string, i: number) => (
                      <Badge key={i} variant="outline" className="text-[10px] font-mono bg-rose-500/10 text-rose-400 border-rose-500/20">
                        {kw}
                      </Badge>
                    ))}
                  </div>
                )}
              </div>
            )}
          </Card>

          {/* Blocklist */}
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-slate-200 tracking-wide">BLOCKLIST</h3>
              <div className="flex gap-2">
                <Button onClick={loadBlocklist} variant="outline" className="font-mono text-xs text-slate-400 border-cyan-500/20">
                  REFRESH
                </Button>
                <Button onClick={loadRobocallStats} variant="outline" className="font-mono text-xs text-slate-400 border-cyan-500/20">
                  STATS
                </Button>
              </div>
            </div>
            <div className="flex gap-3 mb-4">
              <Input placeholder="+15551234567" value={blockNumber} onChange={(e) => setBlockNumber(e.target.value)}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm w-48" />
              <Input placeholder="Reason" value={blockReason} onChange={(e) => setBlockReason(e.target.value)}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm flex-1" />
              <Button onClick={addToBlocklist} className="bg-rose-600 hover:bg-rose-500 text-white font-mono text-xs px-4">
                BLOCK
              </Button>
            </div>
            {blocklist.length > 0 && (
              <div className="space-y-2">
                {blocklist.map((entry, i) => (
                  <div key={i} className="flex items-center justify-between p-2 rounded bg-[#070b14]">
                    <div>
                      <span className="font-mono text-sm text-slate-300">{entry.number}</span>
                      <span className="text-xs text-slate-500 ml-3">{entry.reason}</span>
                    </div>
                    <Button onClick={() => removeFromBlocklist(entry.number)} variant="ghost"
                      className="text-xs text-rose-400 hover:text-rose-300 font-mono">REMOVE</Button>
                  </div>
                ))}
              </div>
            )}
            {robocallStats && (
              <div className="mt-4 grid grid-cols-4 gap-3">
                {[
                  {label: "BLOCKLIST", value: robocallStats.blocklist_size},
                  {label: "DETECTED", value: robocallStats.robocalls_detected || 0},
                  {label: "BLOCKED", value: robocallStats.blocked || 0},
                  {label: "THRESHOLD", value: robocallStats.threshold},
                ].map((s) => (
                  <div key={s.label} className="p-3 rounded bg-[#070b14] text-center">
                    <div className="text-[10px] font-mono text-slate-500 mb-1">{s.label}</div>
                    <div className="font-mono text-lg text-cyan-400">{s.value}</div>
                  </div>
                ))}
              </div>
            )}
          </Card>
        </TabsContent>

        {/* PII Masking Tab */}
        <TabsContent value="pii" className="space-y-4">
          <Card className="bg-[#0f1629] border-amber-500/15 p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-slate-200 tracking-wide">PII MASKING ENGINE</h3>
              <Button onClick={togglePII} variant="outline" className={`font-mono text-xs ${
                piiEnabled ? "text-emerald-400 border-emerald-500/30" : "text-rose-400 border-rose-500/30"
              }`}>
                {piiEnabled ? "ENABLED" : "DISABLED"}
              </Button>
            </div>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mb-4">
              {["Credit Card", "SSN", "CVV", "Date of Birth", "Account Number", "Card (spoken)", "SSN (spoken)"].map((p) => (
                <div key={p} className="p-2 rounded bg-[#070b14] text-center">
                  <Badge variant="outline" className="text-[10px] font-mono bg-amber-500/10 text-amber-400 border-amber-500/20">
                    {p}
                  </Badge>
                </div>
              ))}
            </div>
          </Card>

          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">TEST PII DETECTION</label>
            <div className="flex gap-3">
              <Input placeholder="My credit card is 4111 1111 1111 1111 and SSN is 123-45-6789"
                value={piiText} onChange={(e) => setPiiText(e.target.value)}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm placeholder:text-slate-700" />
              <Button onClick={testPII} className="bg-amber-600 hover:bg-amber-500 text-white font-mono text-xs tracking-wider px-6">
                SCAN
              </Button>
            </div>
            {piiResult && (
              <div className="mt-4 space-y-3">
                <div className="flex items-center gap-2">
                  <Badge variant="outline" className={`font-mono text-[10px] ${
                    piiResult.pii_found ? "bg-rose-500/20 text-rose-400 border-rose-500/30" : "bg-emerald-500/20 text-emerald-400 border-emerald-500/30"
                  }`}>
                    {piiResult.pii_found ? `${piiResult.detections?.length} PII FOUND` : "CLEAN"}
                  </Badge>
                </div>
                {piiResult.pii_found && (
                  <>
                    <div className="p-3 rounded bg-rose-500/[0.06] border border-rose-500/15">
                      <div className="text-[10px] font-mono text-rose-400 mb-1">ORIGINAL</div>
                      <p className="text-sm text-slate-300 font-mono">{piiResult.original}</p>
                    </div>
                    <div className="p-3 rounded bg-emerald-500/[0.06] border border-emerald-500/15">
                      <div className="text-[10px] font-mono text-emerald-400 mb-1">MASKED</div>
                      <p className="text-sm text-slate-300 font-mono">{piiResult.masked}</p>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      {piiResult.detections?.map((d: any, i: number) => (
                        <Badge key={i} variant="outline" className={`text-[10px] font-mono ${
                          d.level === "critical" ? "bg-rose-500/15 text-rose-400 border-rose-500/25" : "bg-amber-500/15 text-amber-400 border-amber-500/25"
                        }`}>
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

        {/* Voice Biometrics Tab */}
        <TabsContent value="biometrics" className="space-y-4">
          <Card className="bg-[#0f1629] border-violet-500/15 p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-violet-300 tracking-wide">VOICE PRINT ENROLLMENT</h3>
              <Button onClick={loadVoiceprints} variant="outline" className="font-mono text-xs text-slate-400 border-violet-500/20">
                REFRESH
              </Button>
            </div>
            <div className="flex gap-3 mb-4">
              <Input placeholder="Profile label (e.g. fraud_caller_001)" value={vpLabel}
                onChange={(e) => setVpLabel(e.target.value)}
                className="bg-[#070b14] border-violet-500/15 text-slate-200 font-mono text-sm flex-1" />
              <select value={vpType} onChange={(e) => setVpType(e.target.value)}
                className="bg-[#070b14] border border-violet-500/15 rounded-md text-slate-200 font-mono text-sm px-3">
                <option value="fraud">Fraud Profile</option>
                <option value="verified">Verified Identity</option>
              </select>
              <Button onClick={enrollVoiceprint} className="bg-violet-600 hover:bg-violet-500 text-white font-mono text-xs px-6">
                ENROLL
              </Button>
            </div>
            {voiceprints.length > 0 && (
              <div className="space-y-2">
                {voiceprints.map((vp: any, i: number) => (
                  <div key={i} className="flex items-center justify-between p-3 rounded bg-[#070b14]">
                    <div className="flex items-center gap-3">
                      <Badge variant="outline" className={`text-[10px] font-mono ${
                        vp.type === "fraud" ? "bg-rose-500/15 text-rose-400 border-rose-500/25" : "bg-emerald-500/15 text-emerald-400 border-emerald-500/25"
                      }`}>
                        {vp.type?.toUpperCase()}
                      </Badge>
                      <span className="font-mono text-sm text-slate-300">{vp.label}</span>
                    </div>
                    <span className="text-[10px] font-mono text-slate-600">{vp.features_dim}d vector</span>
                  </div>
                ))}
              </div>
            )}
            {voiceprints.length === 0 && (
              <p className="text-sm text-slate-600 text-center py-4">No voice prints enrolled. Click REFRESH to load.</p>
            )}
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
