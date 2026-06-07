"use client";

import { useState, useEffect, useRef } from "react";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { authFetch } from "@/lib/auth";

const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";

interface TranscriptEntry {
  speaker: string;
  text: string;
  time: string;
}

interface SuggestionEntry {
  text: string;
  category: string;
  confidence: number;
  time: string;
}

interface VoiceSentimentData {
  avg_energy: number;
  energy_trend: string;
  avg_pitch_hz: number;
  pitch_variance: number;
  speaking_rate_wpm: number;
  silence_ratio: number;
  agitation: number;
  engagement: number;
  frustration: number;
  sentiment: string;
  confidence: number;
}

interface SummaryData {
  summary: string;
  action_items: string[];
  sentiment: string;
  duration: number;
  voice_sentiment?: VoiceSentimentData;
}

const categoryStyle: Record<string, { bg: string; label: string }> = {
  answer: { bg: "bg-cyan-500/15 text-cyan-400 border-cyan-500/25", label: "ANSWER" },
  empathy: { bg: "bg-violet-500/15 text-violet-400 border-violet-500/25", label: "EMPATHY" },
  compliance: { bg: "bg-amber-500/15 text-amber-400 border-amber-500/25", label: "COMPLIANCE" },
  upsell: { bg: "bg-pink-500/15 text-pink-400 border-pink-500/25", label: "UPSELL" },
};

export default function LiveOpsPage() {
  const [callID, setCallID] = useState("");
  const [connected, setConnected] = useState(false);
  const [transcript, setTranscript] = useState<TranscriptEntry[]>([]);
  const [suggestions, setSuggestions] = useState<SuggestionEntry[]>([]);
  const [summary, setSummary] = useState<SummaryData | null>(null);
  const [dialNumber, setDialNumber] = useState("");
  const [activeSessions, setActiveSessions] = useState<Array<{call_id: string; duration: number; caller?: string; agent?: string; voice_sentiment?: VoiceSentimentData}>>([]);
  const scrollRef = useRef<HTMLDivElement>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

  // Poll active copilot sessions every 3s
  useEffect(() => {
    async function pollActive() {
      try {
        const res = await authFetch("/api/copilot/active");
        if (res.ok) {
          const data = await res.json();
          setActiveSessions(data || []);
          // Auto-connect to first session if not already connected
          if (!connected && data?.length > 0 && !callID) {
            setCallID(data[0].call_id);
            connectToCall(data[0].call_id);
          }
        }
      } catch {}
    }
    pollActive();
    const interval = setInterval(pollActive, 3000);
    return () => clearInterval(interval);
  }, [connected, callID]);

  // Auto-scroll transcript
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [transcript]);

  function connectToCall(id: string) {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
    }

    const es = new EventSource(`${GATEWAY}/siprec/events?call_id=${id}`);
    eventSourceRef.current = es;

    es.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        const now = new Date().toLocaleTimeString("en-US", { hour12: false });

        if (data.type === "transcript") {
          setTranscript((prev) => [...prev, {
            speaker: data.speaker,
            text: data.text,
            time: now,
          }]);
        } else if (data.type === "suggestion") {
          setSuggestions((prev) => [...prev, {
            text: data.suggestion,
            category: data.category || "answer",
            confidence: data.confidence || 0.8,
            time: now,
          }]);
        } else if (data.type === "summary") {
          setSummary({
            summary: data.summary || "",
            action_items: data.action_items || [],
            sentiment: data.sentiment || "neutral",
            duration: data.duration || 0,
            voice_sentiment: data.voice_sentiment || undefined,
          });
        }
      } catch {}
    };

    es.onopen = () => setConnected(true);
    es.onerror = () => {
      // Retry connection after 2s
      setTimeout(() => {
        if (callID) connectToCall(callID);
      }, 2000);
    };

    setCallID(id);
    setConnected(true);
    setTranscript([]);
    setSuggestions([]);
    setSummary(null);
  }

  function disconnect() {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
      eventSourceRef.current = null;
    }
    setConnected(false);
  }

  async function handleDial() {
    if (!dialNumber) return;
    try {
      const res = await authFetch("/call", {
        method: "POST",
        body: JSON.stringify({ to: dialNumber, mode: "loopback" }),
      });
      const data = await res.json();
      if (data.call_id) {
        connectToCall(data.call_id);
      }
    } catch (err) {
      console.error("Dial failed:", err);
    }
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          {connected && <span className="w-3 h-3 rounded-full bg-rose-500 animate-pulse" />}
          <h1 className="text-2xl font-semibold text-slate-100">Live Operations</h1>
        </div>
        <div className="flex items-center gap-2">
          {connected && (
            <>
              {/* Live sentiment from active session poll */}
              {activeSessions.find(s => s.call_id === callID)?.voice_sentiment && (() => {
                const vs = activeSessions.find(s => s.call_id === callID)!.voice_sentiment!;
                const mood = vs.frustration > 0.6 ? "FRUSTRATED" : vs.agitation > 0.5 ? "AGITATED" : vs.engagement > 0.6 ? "ENGAGED" : "CALM";
                const moodColor = vs.frustration > 0.6 ? "bg-rose-500/15 text-rose-400 border-rose-500/25" :
                  vs.agitation > 0.5 ? "bg-amber-500/15 text-amber-400 border-amber-500/25" :
                  "bg-emerald-500/15 text-emerald-400 border-emerald-500/25";
                return (
                  <Badge variant="outline" className={`${moodColor} font-mono text-[10px]`}>
                    {mood}
                  </Badge>
                );
              })()}
              <Badge variant="outline" className="bg-cyan-500/10 text-cyan-400 border-cyan-500/20 font-mono text-xs">
                {(() => {
                  const active = activeSessions.find(s => s.call_id === callID);
                  if (active?.caller) return `${active.caller} → ${active.agent || "agent"}`;
                  return `CALL: ${callID.slice(0, 12)}...`;
                })()}
              </Badge>
              <Button variant="outline" onClick={disconnect} className="border-rose-500/30 text-rose-400 hover:bg-rose-500/10 font-mono text-xs">
                DISCONNECT
              </Button>
            </>
          )}
        </div>
      </div>

      {/* Connect to call */}
      {!connected && (
        <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
          <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">CONNECT TO LIVE CALL</label>

          {/* Active sessions — click to connect */}
          {activeSessions.length > 0 && (
            <div className="mb-4">
              <div className="text-[10px] font-mono text-emerald-400 tracking-wider mb-2">
                {activeSessions.length} ACTIVE SESSION{activeSessions.length > 1 ? "S" : ""}
              </div>
              <div className="space-y-2">
                {activeSessions.map((s) => {
                  const vs = s.voice_sentiment;
                  const moodColor = vs && vs.frustration > 0.6 ? "border-rose-500/30" :
                    vs && vs.agitation > 0.5 ? "border-amber-500/30" : "border-emerald-500/20";
                  const moodDot = vs && vs.frustration > 0.6 ? "bg-rose-500" :
                    vs && vs.agitation > 0.5 ? "bg-amber-500" : "bg-emerald-500";
                  return (
                    <div key={s.call_id} className={`flex items-center justify-between p-2 rounded bg-[#070b14] border ${moodColor}`}>
                      <div className="flex items-center gap-2">
                        <span className={`w-2 h-2 rounded-full ${moodDot} animate-pulse`} />
                        {s.caller ? (
                          <span className="font-mono text-xs text-cyan-400" title={s.call_id}>{s.caller}</span>
                        ) : (
                          <span className="font-mono text-xs text-slate-300">{s.call_id.slice(0, 12)}...</span>
                        )}
                        {s.agent && (
                          <span className="font-mono text-[10px] text-emerald-400">→ {s.agent}</span>
                        )}
                        <span className="font-mono text-[10px] text-slate-500">{s.duration}s</span>
                        {vs && (
                          <div className="flex gap-1.5 ml-1">
                            <span className="font-mono text-[9px] text-slate-500" title="Agitation">
                              AGT:<span className={vs.agitation > 0.5 ? "text-amber-400" : "text-slate-500"}>{(vs.agitation * 100).toFixed(0)}%</span>
                            </span>
                            <span className="font-mono text-[9px] text-slate-500" title="Frustration">
                              FRS:<span className={vs.frustration > 0.6 ? "text-rose-400" : "text-slate-500"}>{(vs.frustration * 100).toFixed(0)}%</span>
                            </span>
                          </div>
                        )}
                      </div>
                      <Button onClick={() => { setCallID(s.call_id); connectToCall(s.call_id); }}
                        className="bg-emerald-600 hover:bg-emerald-500 text-white font-mono text-[10px] px-3 py-1 h-6">
                        CONNECT
                      </Button>
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          <div className="flex gap-3">
            <Input
              placeholder="Or paste a call ID manually"
              value={callID}
              onChange={(e) => setCallID(e.target.value)}
              className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm placeholder:text-slate-700"
            />
            <Button onClick={() => connectToCall(callID)} className="bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider px-6">
              CONNECT
            </Button>
          </div>
          {activeSessions.length === 0 && (
            <p className="text-xs text-slate-600 font-mono mt-2">
              No active calls. Dial 2001 from your softphone to start a co-pilot session.
            </p>
          )}
        </Card>
      )}

      {/* Main grid: Transcript + Copilot */}
      <div className="grid grid-cols-1 lg:grid-cols-5 gap-4">
        {/* Transcript panel */}
        <Card className="lg:col-span-3 bg-[#0f1629] border-cyan-500/10 glow-border flex flex-col" style={{ height: "520px" }}>
          <div className="px-5 py-3.5 border-b border-cyan-500/10 flex items-center justify-between">
            <div className="flex items-center gap-2">
              <span className={`w-2 h-2 rounded-full ${connected ? "bg-cyan-400 animate-pulse-glow" : "bg-slate-600"}`} />
              <h2 className="text-xs font-mono font-semibold text-slate-300 tracking-wider">LIVE TRANSCRIPT</h2>
            </div>
            <span className="text-[10px] font-mono text-slate-600">{transcript.length} utterances</span>
          </div>
          <div className="flex-1 overflow-y-auto px-5 py-3" ref={scrollRef}>
            <div className="space-y-4">
              {transcript.length === 0 && connected && (
                <div className="flex items-center gap-2 py-8 justify-center">
                  <div className="flex gap-1">
                    <span className="w-1.5 h-1.5 rounded-full bg-cyan-400 animate-bounce" style={{ animationDelay: "0ms" }} />
                    <span className="w-1.5 h-1.5 rounded-full bg-cyan-400 animate-bounce" style={{ animationDelay: "150ms" }} />
                    <span className="w-1.5 h-1.5 rounded-full bg-cyan-400 animate-bounce" style={{ animationDelay: "300ms" }} />
                  </div>
                  <span className="text-[10px] font-mono text-slate-600">Waiting for speech...</span>
                </div>
              )}
              {transcript.length === 0 && !connected && (
                <div className="py-8 text-center text-sm text-slate-600">Connect to a call to see live transcript</div>
              )}
              {transcript.map((t, i) => (
                <div key={i} className="transcript-line" data-speaker={t.speaker}>
                  <div className="flex items-center gap-2 mb-1">
                    <span className={`text-[10px] font-mono font-semibold tracking-wider ${
                      t.speaker === "customer" ? "text-cyan-400" : "text-emerald-400"
                    }`}>
                      {t.speaker === "customer" ? "CUSTOMER" : "AGENT"}
                    </span>
                    <span className="text-[10px] font-mono text-slate-600">{t.time}</span>
                  </div>
                  <p className="text-sm text-slate-300 leading-relaxed">{t.text}</p>
                </div>
              ))}
              {summary && (
                <div className="mt-6 p-4 rounded-md bg-amber-500/[0.06] border border-amber-500/20">
                  <div className="text-[10px] font-mono font-semibold text-amber-400 tracking-wider mb-2">CALL SUMMARY</div>
                  <p className="text-sm text-slate-300 mb-3">{summary.summary}</p>
                  {summary.action_items.length > 0 && (
                    <div className="mb-3">
                      <div className="text-[10px] font-mono text-slate-500 mb-1">ACTION ITEMS</div>
                      {summary.action_items.map((item, i) => (
                        <div key={i} className="text-sm text-slate-400 pl-3 border-l border-amber-500/30 mb-1">{item}</div>
                      ))}
                    </div>
                  )}
                  <div className="flex gap-4 text-[10px] font-mono text-slate-500 mb-3">
                    <span>Sentiment: <span className={summary.sentiment === "positive" ? "text-emerald-400" : summary.sentiment === "negative" ? "text-rose-400" : "text-slate-400"}>{summary.sentiment.toUpperCase()}</span></span>
                    <span>Duration: {summary.duration}s</span>
                  </div>

                  {/* Voice Sentiment Analysis */}
                  {summary.voice_sentiment && (
                    <div className="p-3 rounded bg-[#070b14] border border-slate-700/50">
                      <div className="text-[10px] font-mono text-cyan-400 tracking-wider mb-2">VOICE SENTIMENT ANALYSIS</div>
                      <div className="grid grid-cols-3 gap-3 mb-3">
                        <div className="text-center">
                          <div className="text-[10px] font-mono text-slate-500">AGITATION</div>
                          <div className={`font-mono text-lg ${summary.voice_sentiment.agitation > 0.6 ? "text-rose-400" : summary.voice_sentiment.agitation > 0.3 ? "text-amber-400" : "text-emerald-400"}`}>
                            {(summary.voice_sentiment.agitation * 100).toFixed(0)}%
                          </div>
                        </div>
                        <div className="text-center">
                          <div className="text-[10px] font-mono text-slate-500">FRUSTRATION</div>
                          <div className={`font-mono text-lg ${summary.voice_sentiment.frustration > 0.6 ? "text-rose-400" : summary.voice_sentiment.frustration > 0.3 ? "text-amber-400" : "text-emerald-400"}`}>
                            {(summary.voice_sentiment.frustration * 100).toFixed(0)}%
                          </div>
                        </div>
                        <div className="text-center">
                          <div className="text-[10px] font-mono text-slate-500">ENGAGEMENT</div>
                          <div className={`font-mono text-lg ${summary.voice_sentiment.engagement > 0.6 ? "text-emerald-400" : summary.voice_sentiment.engagement > 0.3 ? "text-amber-400" : "text-rose-400"}`}>
                            {(summary.voice_sentiment.engagement * 100).toFixed(0)}%
                          </div>
                        </div>
                      </div>
                      <div className="grid grid-cols-4 gap-2 text-[10px] font-mono text-slate-500">
                        <div>
                          <span className="text-slate-600">PITCH</span>
                          <div className="text-slate-300">{summary.voice_sentiment.avg_pitch_hz.toFixed(0)} Hz</div>
                        </div>
                        <div>
                          <span className="text-slate-600">ENERGY</span>
                          <div className="text-slate-300">{summary.voice_sentiment.energy_trend}</div>
                        </div>
                        <div>
                          <span className="text-slate-600">SPEED</span>
                          <div className="text-slate-300">{summary.voice_sentiment.speaking_rate_wpm.toFixed(0)} wpm</div>
                        </div>
                        <div>
                          <span className="text-slate-600">SILENCE</span>
                          <div className="text-slate-300">{(summary.voice_sentiment.silence_ratio * 100).toFixed(0)}%</div>
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>
        </Card>

        {/* Co-pilot panel */}
        <Card className="lg:col-span-2 bg-[#0f1629] border-violet-500/15 flex flex-col" style={{ height: "520px", boxShadow: "0 0 20px rgba(139, 92, 246, 0.08)" }}>
          <div className="px-5 py-3.5 border-b border-violet-500/15 flex items-center gap-2">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#8b5cf6" strokeWidth="2">
              <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
            </svg>
            <h2 className="text-xs font-mono font-semibold text-violet-300 tracking-wider">CO-PILOT ASSIST</h2>
            <span className="ml-auto text-[10px] font-mono text-slate-600">{suggestions.length} suggestions</span>
          </div>
          <div className="flex-1 overflow-y-auto px-4 py-3">
            <div className="space-y-3">
              {suggestions.length === 0 && (
                <div className="py-8 text-center text-sm text-slate-600">
                  {connected ? "Waiting for customer question..." : "Connect to a call to see suggestions"}
                </div>
              )}
              {suggestions.map((s, i) => (
                <div key={i} className="p-3.5 rounded-md bg-violet-500/[0.06] border border-violet-500/15">
                  <div className="flex items-center justify-between mb-2">
                    <Badge variant="outline" className={`text-[9px] font-mono ${categoryStyle[s.category]?.bg || "bg-slate-500/15 text-slate-400"}`}>
                      {categoryStyle[s.category]?.label || s.category.toUpperCase()}
                    </Badge>
                    <span className="text-[10px] font-mono text-slate-600">
                      {Math.round(s.confidence * 100)}%
                    </span>
                  </div>
                  <p className="text-sm text-slate-300 leading-relaxed">{s.text}</p>
                  <span className="text-[10px] font-mono text-slate-600 mt-2 block">{s.time}</span>
                </div>
              ))}
            </div>
          </div>
        </Card>
      </div>

      {/* Dial pad */}
      <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
        <div className="flex items-center gap-4">
          <div className="flex-1">
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">ORIGINATE OUTBOUND</label>
            <div className="flex gap-3">
              <Input
                placeholder="+1 (555) 000-0000"
                value={dialNumber}
                onChange={(e) => setDialNumber(e.target.value)}
                className="max-w-xs bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-lg placeholder:text-slate-700"
              />
              <Button onClick={handleDial} className="bg-emerald-600 hover:bg-emerald-500 text-white font-mono text-xs tracking-wider px-6">
                DIAL
              </Button>
            </div>
          </div>
          <div className="flex gap-6 text-center">
            <div>
              <div className="text-[10px] font-mono text-slate-600 mb-1">LLM</div>
              <Badge variant="outline" className="bg-cyan-500/10 text-cyan-400 border-cyan-500/20 font-mono text-[10px]">
                CLAUDE HAIKU
              </Badge>
            </div>
            <div>
              <div className="text-[10px] font-mono text-slate-600 mb-1">STT</div>
              <Badge variant="outline" className="bg-emerald-500/10 text-emerald-400 border-emerald-500/20 font-mono text-[10px]">
                WHISPER MED
              </Badge>
            </div>
            <div>
              <div className="text-[10px] font-mono text-slate-600 mb-1">RAG</div>
              <Badge variant="outline" className="bg-violet-500/10 text-violet-400 border-violet-500/20 font-mono text-[10px]">
                CHROMA DB
              </Badge>
            </div>
          </div>
        </div>
      </Card>
    </div>
  );
}
