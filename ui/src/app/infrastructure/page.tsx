"use client";

import { useState, useEffect, useRef } from "react";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { authFetch } from "@/lib/auth";

const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";

const circuitColor: Record<string, string> = {
  closed: "bg-emerald-500/20 text-emerald-400 border-emerald-500/30",
  open: "bg-rose-500/20 text-rose-400 border-rose-500/30",
  "half-open": "bg-amber-500/20 text-amber-400 border-amber-500/30",
};

interface ServiceInfo { name: string; status: string; port: string; detail?: string; }

export default function DiagnosticsPage() {
  // Services
  const [services, setServices] = useState<ServiceInfo[]>([]);
  const [servicesLoading, setServicesLoading] = useState(false);

  // Failover
  const [failover, setFailover] = useState<any>(null);
  const [scale, setScale] = useState<any>(null);

  // Metrics
  const [metrics, setMetrics] = useState("");

  // Gateway logs
  const [logs, setLogs] = useState<string[]>([]);
  const [logFilter, setLogFilter] = useState("");
  const logsRef = useRef<HTMLDivElement>(null);

  // Testing
  const [dtmfDigits, setDtmfDigits] = useState("");
  const [dtmfResult, setDtmfResult] = useState<any>(null);
  const [robocallText, setRobocallText] = useState("");
  const [robocallResult, setRobocallResult] = useState<any>(null);
  const [piiText, setPiiText] = useState("");
  const [piiResult, setPiiResult] = useState<any>(null);

  useEffect(() => {
    loadServices();
    loadFailover();
    loadScale();
    loadRecentLogs();
  }, []);

  // Auto-refresh services every 10s
  useEffect(() => {
    const iv = setInterval(loadServices, 10000);
    return () => clearInterval(iv);
  }, []);

  async function loadServices() {
    setServicesLoading(true);
    try {
      const res = await authFetch("/api/services/status");
      if (res.ok) setServices(await res.json());
    } catch {}
    setServicesLoading(false);
  }

  async function loadFailover() {
    try { const res = await authFetch("/api/failover/status"); if (res.ok) setFailover(await res.json()); } catch {}
  }

  async function loadScale() {
    try { const res = await authFetch("/api/scale/status"); if (res.ok) setScale(await res.json()); } catch {}
  }

  async function loadMetrics() {
    try { const res = await fetch(`${GATEWAY}/metrics`); setMetrics(await res.text()); } catch {}
  }

  async function loadRecentLogs() {
    try {
      const res = await authFetch("/api/config");
      if (res.ok) {
        const config = await res.json();
        setLogs(prev => [...prev, `[${new Date().toLocaleTimeString()}] Gateway mode: ${config.mode}, database: ${config.database}`]);
      }
    } catch {}
    // Fetch active sessions as log entries
    try {
      const res = await authFetch("/api/copilot/active");
      if (res.ok) {
        const sessions = await res.json();
        if (sessions.length > 0) {
          sessions.forEach((s: any) => {
            setLogs(prev => [...prev, `[${new Date().toLocaleTimeString()}] Active session: ${s.call_id?.slice(0,12)} (${s.duration}s) caller=${s.caller || "?"}`]);
          });
        }
      }
    } catch {}
  }

  async function testDTMF() {
    const res = await authFetch("/api/dtmf/test", { method: "POST", body: JSON.stringify({ text: dtmfDigits }) });
    if (res.ok) setDtmfResult(await res.json());
  }

  async function testRobocall() {
    const res = await authFetch("/api/robocall/test", { method: "POST", body: JSON.stringify({ text: robocallText }) });
    if (res.ok) setRobocallResult(await res.json());
  }

  async function testPII() {
    const res = await authFetch("/api/security/pii/test", { method: "POST", body: JSON.stringify({ text: piiText }) });
    if (res.ok) setPiiResult(await res.json());
  }

  const filteredLogs = logFilter ? logs.filter(l => l.toLowerCase().includes(logFilter.toLowerCase())) : logs;
  const onlineCount = services.filter(s => s.status === "online").length;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-slate-100">Diagnostics</h1>
        <p className="text-sm text-slate-500 font-mono mt-1">Service health, logs, metrics, testing tools</p>
      </div>

      <Tabs defaultValue="services" className="space-y-4">
        <TabsList className="bg-[#0f1629] border border-cyan-500/10">
          <TabsTrigger value="services" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">
            SERVICES ({onlineCount}/{services.length})
          </TabsTrigger>
          <TabsTrigger value="circuits" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">
            CIRCUITS
          </TabsTrigger>
          <TabsTrigger value="logs" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">
            LOGS
          </TabsTrigger>
          <TabsTrigger value="metrics" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">
            METRICS
          </TabsTrigger>
          <TabsTrigger value="testing" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">
            TESTING
          </TabsTrigger>
        </TabsList>

        {/* Services */}
        <TabsContent value="services" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-slate-200 tracking-wide">SERVICE HEALTH</h3>
              <div className="flex items-center gap-2">
                {servicesLoading && <span className="w-2 h-2 rounded-full bg-cyan-400 animate-pulse" />}
                <span className="text-[10px] font-mono text-slate-500">auto-refresh 10s</span>
                <Button onClick={loadServices} variant="outline" className="font-mono text-xs text-slate-400 border-cyan-500/20">REFRESH</Button>
              </div>
            </div>
            <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
              {services.map((svc) => (
                <div key={svc.name} className={`p-4 rounded-md bg-[#070b14] border ${svc.status === "online" ? "border-emerald-500/15" : "border-rose-500/15"}`}>
                  <div className="flex items-center gap-2 mb-2">
                    <span className={`w-2.5 h-2.5 rounded-full ${svc.status === "online" ? "bg-emerald-500 animate-pulse" : svc.status === "offline" ? "bg-rose-500" : "bg-slate-500"}`} />
                    <span className="text-sm font-medium text-slate-200">{svc.name}</span>
                  </div>
                  <div className="font-mono text-[10px] text-slate-500">{svc.port}</div>
                  {svc.detail && <div className="font-mono text-[10px] text-slate-600 mt-1">{svc.detail}</div>}
                  <Badge variant="outline" className={`mt-2 text-[9px] font-mono ${svc.status === "online" ? "bg-emerald-500/10 text-emerald-400 border-emerald-500/20" : "bg-rose-500/10 text-rose-400 border-rose-500/20"}`}>
                    {svc.status.toUpperCase()}
                  </Badge>
                </div>
              ))}
            </div>
          </Card>

          {/* Admission & Scale */}
          {scale && (
            <Card className="bg-[#0f1629] border-cyan-500/10 p-5">
              <h3 className="text-sm font-semibold text-slate-200 tracking-wide mb-3">CAPACITY</h3>
              <div className="grid grid-cols-3 gap-4 mb-4">
                <div className="p-3 rounded bg-[#070b14] text-center">
                  <div className="font-mono text-2xl text-cyan-400">{scale.admission?.current || 0}</div>
                  <div className="text-[10px] font-mono text-slate-500">ACTIVE</div>
                </div>
                <div className="p-3 rounded bg-[#070b14] text-center">
                  <div className="font-mono text-2xl text-emerald-400">{scale.admission?.available || 0}</div>
                  <div className="text-[10px] font-mono text-slate-500">AVAILABLE</div>
                </div>
                <div className="p-3 rounded bg-[#070b14] text-center">
                  <div className="font-mono text-2xl text-slate-400">{scale.admission?.max_sessions || 0}</div>
                  <div className="text-[10px] font-mono text-slate-500">MAX</div>
                </div>
              </div>
              {["stt_pool", "tts_pool"].map((poolKey) => {
                const pool = scale[poolKey];
                if (!pool) return null;
                return (
                  <div key={poolKey} className="p-3 rounded bg-[#070b14] border border-cyan-500/10 mb-2">
                    <div className="flex items-center justify-between mb-1">
                      <span className="text-[10px] font-mono text-slate-500">{pool.name?.toUpperCase()} POOL</span>
                      <Badge variant="outline" className="text-[10px] font-mono bg-emerald-500/10 text-emerald-400 border-emerald-500/20">{pool.healthy}/{pool.total}</Badge>
                    </div>
                    {pool.workers?.map((w: any, i: number) => (
                      <div key={i} className="flex items-center gap-2 text-xs font-mono">
                        <span className={`w-2 h-2 rounded-full ${w.healthy ? "bg-emerald-500" : "bg-rose-500"}`} />
                        <span className="text-slate-400">{w.url}</span>
                      </div>
                    ))}
                  </div>
                );
              })}
            </Card>
          )}
        </TabsContent>

        {/* Circuits */}
        <TabsContent value="circuits" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-slate-200 tracking-wide">CIRCUIT BREAKERS</h3>
              <Button onClick={loadFailover} variant="outline" className="font-mono text-xs text-slate-400 border-cyan-500/20">REFRESH</Button>
            </div>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              {failover && Object.entries(failover).map(([service, data]: [string, any]) => (
                <div key={service} className="p-4 rounded-md bg-[#070b14] border border-cyan-500/10">
                  <div className="text-[10px] font-mono text-slate-500 tracking-wider mb-2">{service.toUpperCase()}</div>
                  <div className="flex items-center gap-2 mb-2">
                    <span className={`w-2.5 h-2.5 rounded-full ${data.state === "closed" ? "bg-emerald-500 animate-pulse" : data.state === "open" ? "bg-rose-500" : "bg-amber-500 animate-pulse"}`} />
                    <Badge variant="outline" className={`text-[10px] font-mono ${circuitColor[data.state] || ""}`}>{data.state?.toUpperCase()}</Badge>
                  </div>
                  <div className="font-mono text-xs text-slate-500">Failures: <span className={data.failures > 0 ? "text-rose-400" : "text-emerald-400"}>{data.failures}</span></div>
                </div>
              ))}
            </div>
          </Card>
          <Card className="bg-[#0f1629] border-cyan-500/10 p-5">
            <h3 className="text-sm font-semibold text-slate-200 tracking-wide mb-3">FAILOVER CHAIN</h3>
            <div className="space-y-2 font-mono text-sm">
              {[
                { trigger: "LLM drops", action: "Play hold audio -> auto-reconnect -> resume" },
                { trigger: "STT fails", action: "Buffer audio -> retry on half-open probe" },
                { trigger: "TTS fails", action: "Static tone via ESL fallback" },
                { trigger: "All circuits open", action: "SIP REFER to human queue (ext 3000)" },
              ].map((f, i) => (
                <div key={i} className="flex items-start gap-3 p-2 rounded bg-[#070b14]">
                  <Badge variant="outline" className="text-[10px] font-mono shrink-0 bg-amber-500/10 text-amber-400 border-amber-500/20">{f.trigger}</Badge>
                  <span className="text-slate-400 text-xs">{f.action}</span>
                </div>
              ))}
            </div>
          </Card>
        </TabsContent>

        {/* Logs */}
        <TabsContent value="logs" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-slate-200 tracking-wide">GATEWAY LOGS</h3>
              <div className="flex gap-2">
                <Input placeholder="Filter..." value={logFilter} onChange={(e) => setLogFilter(e.target.value)}
                  className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-xs w-48" />
                <Button onClick={loadRecentLogs} variant="outline" className="font-mono text-xs text-slate-400 border-cyan-500/20">REFRESH</Button>
                <Button onClick={() => setLogs([])} variant="outline" className="font-mono text-xs text-slate-400 border-cyan-500/20">CLEAR</Button>
              </div>
            </div>
            <div ref={logsRef} className="bg-[#070b14] rounded border border-cyan-500/10 p-3 font-mono text-xs text-slate-400 overflow-auto max-h-96 space-y-0.5">
              {filteredLogs.length === 0 && <div className="text-slate-600 text-center py-4">No logs. Click REFRESH to load.</div>}
              {filteredLogs.map((line, i) => (
                <div key={i} className={`py-0.5 ${line.includes("error") || line.includes("ERROR") ? "text-rose-400" : line.includes("WARN") ? "text-amber-400" : ""}`}>
                  {line}
                </div>
              ))}
            </div>
            <p className="text-[10px] font-mono text-slate-600 mt-2">For full container logs: <code className="text-cyan-400/60">docker logs voiceagent-gateway-1 -f</code></p>
          </Card>
        </TabsContent>

        {/* Metrics */}
        <TabsContent value="metrics" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-slate-200 tracking-wide">OBSERVABILITY</h3>
              <div className="flex gap-2">
                <Button onClick={loadMetrics} variant="outline" className="font-mono text-xs text-slate-400 border-cyan-500/20">LOAD RAW</Button>
                <a href="http://localhost:9090" target="_blank" rel="noopener noreferrer">
                  <Button variant="outline" className="font-mono text-xs text-cyan-400 border-cyan-500/20">PROMETHEUS</Button>
                </a>
                <a href="http://localhost:3001" target="_blank" rel="noopener noreferrer">
                  <Button variant="outline" className="font-mono text-xs text-violet-400 border-violet-500/20">GRAFANA</Button>
                </a>
              </div>
            </div>
            {metrics ? (
              <pre className="bg-[#070b14] p-4 rounded border border-cyan-500/10 text-xs font-mono text-slate-400 overflow-auto max-h-96 whitespace-pre">{metrics}</pre>
            ) : (
              <p className="text-sm text-slate-600 text-center py-8">Click LOAD RAW to view Prometheus metrics, or open Grafana for dashboards.</p>
            )}
          </Card>
        </TabsContent>

        {/* Testing */}
        <TabsContent value="testing" className="space-y-4">
          {/* DTMF */}
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">DTMF PARSER (RFC 2833)</label>
            <div className="flex gap-3">
              <Input placeholder="Enter digits: 482910" value={dtmfDigits} onChange={(e) => setDtmfDigits(e.target.value)}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-lg max-w-xs" />
              <Button onClick={testDTMF} className="bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs px-6">PARSE</Button>
            </div>
            {dtmfResult && (
              <div className="mt-3 p-3 rounded bg-[#070b14] border border-cyan-500/10">
                <span className="text-[10px] font-mono text-slate-500">LLM receives: </span>
                <span className="font-mono text-cyan-400">{dtmfResult.parsed}</span>
              </div>
            )}
          </Card>

          {/* Robocall */}
          <Card className="bg-[#0f1629] border-rose-500/10 p-5">
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">ROBOCALL DETECTION</label>
            <div className="flex gap-3">
              <Input placeholder="Press 1 for your auto warranty" value={robocallText} onChange={(e) => setRobocallText(e.target.value)}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
              <Button onClick={testRobocall} className="bg-rose-600 hover:bg-rose-500 text-white font-mono text-xs px-6">ANALYZE</Button>
            </div>
            {robocallResult && (
              <div className="mt-3 p-3 rounded bg-[#070b14] border border-rose-500/10">
                <Badge variant="outline" className={`text-[10px] font-mono ${robocallResult?.keyword?.category === "robocall" ? "bg-rose-500/20 text-rose-400" : "bg-emerald-500/20 text-emerald-400"}`}>
                  {robocallResult?.keyword?.category?.toUpperCase()} — Score: {((robocallResult?.keyword?.score || 0) * 100).toFixed(0)}%
                </Badge>
                {robocallResult?.keyword?.keywords?.length > 0 && (
                  <div className="flex flex-wrap gap-1 mt-2">
                    {robocallResult.keyword.keywords.map((kw: string, i: number) => (
                      <Badge key={i} variant="outline" className="text-[9px] font-mono bg-rose-500/10 text-rose-400 border-rose-500/20">{kw}</Badge>
                    ))}
                  </div>
                )}
              </div>
            )}
          </Card>

          {/* PII */}
          <Card className="bg-[#0f1629] border-amber-500/10 p-5">
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">PII MASKING (9 PATTERNS)</label>
            <div className="flex gap-3">
              <Input placeholder="My SSN is 123-45-6789" value={piiText} onChange={(e) => setPiiText(e.target.value)}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
              <Button onClick={testPII} className="bg-amber-600 hover:bg-amber-500 text-white font-mono text-xs px-6">SCAN</Button>
            </div>
            {piiResult && (
              <div className="mt-3 space-y-2">
                <div className="p-3 rounded bg-rose-500/[0.06] border border-rose-500/15">
                  <div className="text-[10px] font-mono text-rose-400 mb-1">ORIGINAL</div>
                  <p className="text-sm text-slate-300 font-mono">{piiResult.original}</p>
                </div>
                <div className="p-3 rounded bg-emerald-500/[0.06] border border-emerald-500/15">
                  <div className="text-[10px] font-mono text-emerald-400 mb-1">MASKED</div>
                  <p className="text-sm text-slate-300 font-mono">{piiResult.masked}</p>
                </div>
                <div className="flex flex-wrap gap-1">
                  {piiResult.detections?.map((d: any, i: number) => (
                    <Badge key={i} variant="outline" className={`text-[9px] font-mono ${d.level === "critical" ? "bg-rose-500/15 text-rose-400" : "bg-amber-500/15 text-amber-400"}`}>
                      {d.type}
                    </Badge>
                  ))}
                </div>
              </div>
            )}
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
