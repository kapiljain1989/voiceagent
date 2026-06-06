"use client";

import { useState } from "react";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

const mockLLMs = [
  { id: "l1", name: "Claude Haiku", provider: "anthropic-vertex", model: "claude-3-5-haiku@20241022", region: "us-east5", isDefault: true, maxTokens: 512 },
  { id: "l2", name: "Gemini Flash", provider: "gemini-vertex", model: "gemini-2.0-flash", region: "us-central1", isDefault: false, maxTokens: 1024 },
];

const providerBadge: Record<string, string> = {
  "anthropic-vertex": "bg-orange-500/15 text-orange-400 border-orange-500/25",
  "gemini-vertex": "bg-blue-500/15 text-blue-400 border-blue-500/25",
};

const defaultPrompt = `You are a helpful voice assistant. Keep responses concise and conversational. Do not use markdown, bullet points, or any text formatting — your responses will be spoken aloud. Respond in 1-3 short sentences unless the user asks for detail.`;

export default function SettingsPage() {
  const [prompt, setPrompt] = useState(defaultPrompt);
  const [testResult, setTestResult] = useState("");

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-slate-100">Configuration</h1>
        <p className="text-sm text-slate-500 font-mono mt-1">LLM models, system prompts, and SBC settings</p>
      </div>

      <Tabs defaultValue="llm" className="space-y-4">
        <TabsList className="bg-[#0f1629] border border-cyan-500/10">
          <TabsTrigger value="llm" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">
            LLM MODELS
          </TabsTrigger>
          <TabsTrigger value="prompts" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">
            SYSTEM PROMPTS
          </TabsTrigger>
          <TabsTrigger value="sbc" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">
            SBC / TRUNK
          </TabsTrigger>
        </TabsList>

        {/* LLM Config */}
        <TabsContent value="llm" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border overflow-hidden">
            <div className="px-5 py-4 border-b border-cyan-500/10 flex items-center justify-between">
              <h2 className="text-sm font-semibold text-slate-200 tracking-wide">LLM CONFIGURATIONS</h2>
              <Button className="bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider">
                + ADD MODEL
              </Button>
            </div>
            <table className="w-full text-sm">
              <thead>
                <tr className="text-[11px] font-mono text-slate-500 uppercase tracking-wider border-b border-white/5">
                  <th className="text-left px-5 py-3 font-medium">Name</th>
                  <th className="text-left px-5 py-3 font-medium">Provider</th>
                  <th className="text-left px-5 py-3 font-medium">Model</th>
                  <th className="text-left px-5 py-3 font-medium">Region</th>
                  <th className="text-left px-5 py-3 font-medium">Max Tokens</th>
                  <th className="text-left px-5 py-3 font-medium">Default</th>
                  <th className="text-left px-5 py-3 font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {mockLLMs.map((l) => (
                  <tr key={l.id} className="border-b border-white/[0.03] hover:bg-cyan-500/[0.03]">
                    <td className="px-5 py-3.5 font-medium text-slate-200">{l.name}</td>
                    <td className="px-5 py-3.5">
                      <Badge variant="outline" className={`text-[10px] font-mono ${providerBadge[l.provider]}`}>
                        {l.provider}
                      </Badge>
                    </td>
                    <td className="px-5 py-3.5 font-mono text-xs text-slate-400">{l.model}</td>
                    <td className="px-5 py-3.5 font-mono text-xs text-slate-500">{l.region}</td>
                    <td className="px-5 py-3.5 font-mono text-sm text-cyan-400">{l.maxTokens}</td>
                    <td className="px-5 py-3.5">
                      {l.isDefault && (
                        <Badge variant="outline" className="bg-emerald-500/15 text-emerald-400 border-emerald-500/25 text-[10px] font-mono">
                          DEFAULT
                        </Badge>
                      )}
                    </td>
                    <td className="px-5 py-3.5">
                      <div className="flex gap-2">
                        <Button variant="ghost" size="sm" className="text-xs text-slate-400 hover:text-cyan-400 font-mono">
                          TEST
                        </Button>
                        <Button variant="ghost" size="sm" className="text-xs text-slate-400 hover:text-slate-200 font-mono">
                          EDIT
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Card>

          {testResult && (
            <Card className="bg-[#0f1629] border-emerald-500/15 p-4">
              <div className="text-[10px] font-mono text-emerald-400 tracking-wider mb-2">TEST RESULT</div>
              <p className="text-sm text-slate-300">{testResult}</p>
            </Card>
          )}
        </TabsContent>

        {/* System Prompts */}
        <TabsContent value="prompts" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-3">
              INTERACTIVE AGENT PROMPT
            </label>
            <Textarea
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              rows={6}
              className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm resize-none"
            />
            <div className="flex items-center justify-between mt-4">
              <span className="text-xs font-mono text-slate-600">{prompt.length} chars</span>
              <Button className="bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider">
                SAVE PROMPT
              </Button>
            </div>
          </Card>

          <Card className="bg-[#0f1629] border-violet-500/15 p-5">
            <label className="text-[10px] font-mono text-violet-400 tracking-wider block mb-3">
              CO-PILOT COACH PROMPT
            </label>
            <Textarea
              defaultValue="You are an expert real-time agent coach. You silently observe a live call between a customer and a support agent..."
              rows={6}
              className="bg-[#070b14] border-violet-500/15 text-slate-200 font-mono text-sm resize-none"
            />
            <div className="flex justify-end mt-4">
              <Button className="bg-violet-600 hover:bg-violet-500 text-white font-mono text-xs tracking-wider">
                SAVE PROMPT
              </Button>
            </div>
          </Card>
        </TabsContent>

        {/* SBC Settings */}
        <TabsContent value="sbc" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <h3 className="text-sm font-semibold text-slate-200 tracking-wide mb-4">SBC TRUNK CONFIGURATION</h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">SBC ADDRESS</label>
                <Input defaultValue="127.0.0.1" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
              </div>
              <div>
                <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">SBC USERNAME</label>
                <Input defaultValue="voiceagent" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
              </div>
              <div>
                <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">SBC PASSWORD</label>
                <Input type="password" defaultValue="changeme" className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm" />
              </div>
              <div>
                <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">REGISTRATION</label>
                <div className="flex items-center gap-3 h-9">
                  <Badge variant="outline" className="bg-slate-500/15 text-slate-400 border-slate-500/25 font-mono text-xs cursor-pointer hover:bg-cyan-500/15 hover:text-cyan-400">
                    DISABLED
                  </Badge>
                </div>
              </div>
            </div>
            <div className="mt-4 pt-4 border-t border-cyan-500/10">
              <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">CRM WEBHOOK URL</label>
              <Input placeholder="https://hooks.salesforce.com/..." className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm placeholder:text-slate-700" />
            </div>
            <div className="flex justify-end mt-4">
              <Button className="bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider">
                SAVE CONFIGURATION
              </Button>
            </div>
          </Card>

          {/* Connection status */}
          <Card className="bg-[#0f1629] border-cyan-500/10 p-5">
            <h3 className="text-sm font-semibold text-slate-200 tracking-wide mb-4">SERVICE STATUS</h3>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              {[
                { name: "Gateway", status: "online", port: ":8080" },
                { name: "FreeSWITCH", status: "online", port: ":5070" },
                { name: "Whisper STT", status: "online", port: ":8000" },
                { name: "Piper TTS", status: "online", port: ":5000" },
              ].map((svc) => (
                <div key={svc.name} className="flex items-center gap-3 p-3 rounded-md bg-[#070b14]">
                  <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
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
