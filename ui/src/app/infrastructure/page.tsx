"use client";

import { useState, useEffect } from "react";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";

const circuitColor: Record<string, string> = {
  closed: "bg-emerald-500/20 text-emerald-400 border-emerald-500/30",
  open: "bg-rose-500/20 text-rose-400 border-rose-500/30",
  "half-open": "bg-amber-500/20 text-amber-400 border-amber-500/30",
};

export default function InfrastructurePage() {
  const [failover, setFailover] = useState<any>(null);
  const [scale, setScale] = useState<any>(null);
  const [dtmfDigits, setDtmfDigits] = useState("");
  const [dtmfResult, setDtmfResult] = useState<any>(null);
  const [metrics, setMetrics] = useState("");

  async function loadFailover() {
    const res = await fetch(`${GATEWAY}/api/failover/status`);
    setFailover(await res.json());
  }

  async function loadScale() {
    const res = await fetch(`${GATEWAY}/api/scale/status`);
    setScale(await res.json());
  }

  async function testDTMF() {
    const res = await fetch(`${GATEWAY}/api/dtmf/test`, {
      method: "POST", headers: {"Content-Type":"application/json"},
      body: JSON.stringify({text: dtmfDigits}),
    });
    setDtmfResult(await res.json());
  }

  async function loadMetrics() {
    const res = await fetch(`${GATEWAY}/metrics`);
    setMetrics(await res.text());
  }

  useEffect(() => {
    loadFailover();
    loadScale();
  }, []);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-slate-100">Infrastructure</h1>
        <p className="text-sm text-slate-500 font-mono mt-1">Failover, scaling, DTMF, observability</p>
      </div>

      <Tabs defaultValue="failover" className="space-y-4">
        <TabsList className="bg-[#0f1629] border border-cyan-500/10">
          <TabsTrigger value="failover" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">
            FAILOVER
          </TabsTrigger>
          <TabsTrigger value="scale" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">
            SCALE
          </TabsTrigger>
          <TabsTrigger value="dtmf" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">
            DTMF
          </TabsTrigger>
          <TabsTrigger value="metrics" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">
            METRICS
          </TabsTrigger>
        </TabsList>

        {/* Failover Tab */}
        <TabsContent value="failover" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-slate-200 tracking-wide">CIRCUIT BREAKER STATUS</h3>
              <Button onClick={loadFailover} variant="outline" className="font-mono text-xs text-slate-400 border-cyan-500/20">
                REFRESH
              </Button>
            </div>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              {failover && Object.entries(failover).map(([service, data]: [string, any]) => (
                <div key={service} className="p-4 rounded-md bg-[#070b14] border border-cyan-500/10">
                  <div className="text-[10px] font-mono text-slate-500 tracking-wider mb-2">{service.toUpperCase()}</div>
                  <div className="flex items-center gap-2 mb-2">
                    <span className={`w-2.5 h-2.5 rounded-full ${
                      data.state === "closed" ? "bg-emerald-500 animate-pulse" :
                      data.state === "open" ? "bg-rose-500" : "bg-amber-500 animate-pulse"
                    }`} />
                    <Badge variant="outline" className={`text-[10px] font-mono ${circuitColor[data.state] || ""}`}>
                      {data.state?.toUpperCase()}
                    </Badge>
                  </div>
                  <div className="font-mono text-xs text-slate-500">
                    Failures: <span className={data.failures > 0 ? "text-rose-400" : "text-emerald-400"}>{data.failures}</span>
                  </div>
                </div>
              ))}
            </div>
          </Card>

          <Card className="bg-[#0f1629] border-cyan-500/10 p-5">
            <h3 className="text-sm font-semibold text-slate-200 tracking-wide mb-3">FAILOVER CHAIN</h3>
            <div className="space-y-2 font-mono text-sm">
              {[
                {trigger: "LLM drops", action: "Play hold audio → auto-reconnect → resume", color: "cyan"},
                {trigger: "STT fails", action: "Buffer audio → retry on half-open probe", color: "amber"},
                {trigger: "TTS fails", action: "Static tone via ESL fallback", color: "amber"},
                {trigger: "All circuits open", action: "SIP REFER to human queue (ext 3000) + X-Failover headers", color: "rose"},
              ].map((f, i) => (
                <div key={i} className="flex items-start gap-3 p-2 rounded bg-[#070b14]">
                  <Badge variant="outline" className={`text-[10px] font-mono shrink-0 bg-${f.color}-500/10 text-${f.color}-400 border-${f.color}-500/20`}>
                    {f.trigger}
                  </Badge>
                  <span className="text-slate-400 text-xs">{f.action}</span>
                </div>
              ))}
            </div>
          </Card>
        </TabsContent>

        {/* Scale Tab */}
        <TabsContent value="scale" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-slate-200 tracking-wide">WORKER POOLS & ADMISSION</h3>
              <Button onClick={loadScale} variant="outline" className="font-mono text-xs text-slate-400 border-cyan-500/20">
                REFRESH
              </Button>
            </div>

            {scale && (
              <div className="space-y-4">
                {/* Admission */}
                <div className="p-4 rounded bg-[#070b14] border border-cyan-500/10">
                  <div className="text-[10px] font-mono text-slate-500 tracking-wider mb-2">ADMISSION CONTROLLER</div>
                  <div className="grid grid-cols-3 gap-4">
                    <div className="text-center">
                      <div className="font-mono text-2xl text-cyan-400">{scale.admission?.current || 0}</div>
                      <div className="text-[10px] font-mono text-slate-500">ACTIVE</div>
                    </div>
                    <div className="text-center">
                      <div className="font-mono text-2xl text-emerald-400">{scale.admission?.available || 0}</div>
                      <div className="text-[10px] font-mono text-slate-500">AVAILABLE</div>
                    </div>
                    <div className="text-center">
                      <div className="font-mono text-2xl text-slate-400">{scale.admission?.max_sessions || 0}</div>
                      <div className="text-[10px] font-mono text-slate-500">MAX</div>
                    </div>
                  </div>
                </div>

                {/* STT Pool */}
                {["stt_pool", "tts_pool"].map((poolKey) => {
                  const pool = scale[poolKey];
                  if (!pool) return null;
                  return (
                    <div key={poolKey} className="p-4 rounded bg-[#070b14] border border-cyan-500/10">
                      <div className="flex items-center justify-between mb-2">
                        <div className="text-[10px] font-mono text-slate-500 tracking-wider">{pool.name?.toUpperCase()} WORKER POOL</div>
                        <Badge variant="outline" className="text-[10px] font-mono bg-emerald-500/10 text-emerald-400 border-emerald-500/20">
                          {pool.healthy}/{pool.total} HEALTHY
                        </Badge>
                      </div>
                      <div className="space-y-1">
                        {pool.workers?.map((w: any, i: number) => (
                          <div key={i} className="flex items-center gap-2 text-xs font-mono">
                            <span className={`w-2 h-2 rounded-full ${w.healthy ? "bg-emerald-500" : "bg-rose-500"}`} />
                            <span className="text-slate-400">{w.url}</span>
                          </div>
                        ))}
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </Card>
        </TabsContent>

        {/* DTMF Tab */}
        <TabsContent value="dtmf" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">RFC 2833 DTMF DIGIT PARSER</label>
            <p className="text-xs text-slate-600 mb-4">
              Intercepts keypad input from RFC 2833 RTP event packets. 100% accurate — no audio processing.
            </p>
            <div className="flex gap-3">
              <Input placeholder="Enter digits: 482910" value={dtmfDigits}
                onChange={(e) => setDtmfDigits(e.target.value)}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-lg placeholder:text-slate-700 max-w-xs" />
              <Button onClick={testDTMF} className="bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider px-6">
                PARSE
              </Button>
            </div>
            {dtmfResult && (
              <div className="mt-4 p-3 rounded bg-[#070b14] border border-cyan-500/10">
                <div className="text-[10px] font-mono text-slate-500 mb-1">INPUT</div>
                <div className="font-mono text-sm text-slate-300 mb-3">{dtmfResult.input}</div>
                <div className="text-[10px] font-mono text-cyan-400 mb-1">LLM RECEIVES</div>
                <div className="font-mono text-lg text-cyan-400">{dtmfResult.parsed}</div>
              </div>
            )}
          </Card>
        </TabsContent>

        {/* Metrics Tab */}
        <TabsContent value="metrics" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-slate-200 tracking-wide">PROMETHEUS METRICS</h3>
              <div className="flex gap-2">
                <Button onClick={loadMetrics} variant="outline" className="font-mono text-xs text-slate-400 border-cyan-500/20">
                  LOAD RAW
                </Button>
                <a href="http://localhost:9090" target="_blank" rel="noopener noreferrer">
                  <Button variant="outline" className="font-mono text-xs text-cyan-400 border-cyan-500/20">
                    PROMETHEUS
                  </Button>
                </a>
                <a href="http://localhost:3001" target="_blank" rel="noopener noreferrer">
                  <Button variant="outline" className="font-mono text-xs text-violet-400 border-violet-500/20">
                    GRAFANA
                  </Button>
                </a>
              </div>
            </div>
            {metrics ? (
              <pre className="bg-[#070b14] p-4 rounded border border-cyan-500/10 text-xs font-mono text-slate-400 overflow-auto max-h-96 whitespace-pre">
                {metrics}
              </pre>
            ) : (
              <p className="text-sm text-slate-600 text-center py-8">Click LOAD RAW to view Prometheus metrics</p>
            )}
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
