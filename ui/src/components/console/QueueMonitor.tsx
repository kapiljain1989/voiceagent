"use client";

import { useState } from "react";
import { ChevronRight, PhoneIncoming } from "lucide-react";
import type { QueueData, QueueCaller } from "@/lib/console-types";

function formatTime(seconds: number) {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}

export function QueueMonitor({
  queues,
  onPickCall,
  callActive,
}: {
  queues: QueueData[];
  onPickCall: (caller: QueueCaller, queueName: string) => void;
  callActive: boolean;
}) {
  const [expandedQueue, setExpandedQueue] = useState<string | null>(null);

  return (
    <div className="space-y-2">
      {queues.map((q) => {
        const waiting = q.callers.length;
        const maxWaitSec =
          waiting > 0 ? Math.max(...q.callers.map((c) => c.waitSec)) : 0;
        const maxWait = formatTime(maxWaitSec);
        const slaColor =
          q.sla >= 90 ? "emerald" : q.sla >= 75 ? "amber" : "rose";
        const slaText: Record<string, string> = {
          emerald: "text-emerald-400",
          amber: "text-amber-400",
          rose: "text-rose-400",
        };
        const slaBg: Record<string, string> = {
          emerald: "bg-emerald-500",
          amber: "bg-amber-500",
          rose: "bg-rose-500",
        };
        const isExpanded = expandedQueue === q.name;

        return (
          <div
            key={q.name}
            className="rounded-lg bg-[#070b14] border border-white/[0.04] overflow-hidden"
          >
            <button
              onClick={() => setExpandedQueue(isExpanded ? null : q.name)}
              className="w-full px-3 py-2.5 text-left hover:bg-white/[0.02] transition-colors"
            >
              <div className="flex items-center justify-between mb-1.5">
                <div className="flex items-center gap-1.5">
                  <ChevronRight
                    size={11}
                    strokeWidth={2}
                    className={`text-slate-600 transition-transform duration-200 ${isExpanded ? "rotate-90" : ""}`}
                  />
                  <span className="text-xs font-medium text-slate-300">
                    {q.name}
                  </span>
                </div>
                <span
                  className={`text-[10px] font-mono font-semibold ${waiting > 3 ? "text-rose-400" : waiting > 0 ? "text-amber-400" : "text-emerald-400"}`}
                >
                  {waiting} waiting
                </span>
              </div>
              <div className="flex items-center gap-3 text-[10px] font-mono text-slate-500 pl-4">
                <span>
                  Max:{" "}
                  <span
                    className={
                      maxWaitSec > 300
                        ? "text-rose-400"
                        : maxWaitSec > 180
                          ? "text-amber-400"
                          : "text-slate-400"
                    }
                  >
                    {maxWait}
                  </span>
                </span>
                <span>Avg: {q.avgHandle}</span>
                <span className="ml-auto">
                  SLA:{" "}
                  <span className={slaText[slaColor]}>{q.sla}%</span>
                </span>
              </div>
              <div className="mt-1.5 h-1 rounded-full bg-white/[0.04] overflow-hidden ml-4">
                <div
                  className={`h-full rounded-full ${slaBg[slaColor]} transition-all duration-500`}
                  style={{ width: `${q.sla}%` }}
                />
              </div>
            </button>

            {isExpanded && q.callers.length > 0 && (
              <div className="border-t border-white/[0.04] animate-[slide-up_0.3s_ease-out]">
                {[...q.callers]
                  .sort((a, b) => {
                    // Priority first (high before normal), then by ID (stable order)
                    const priOrder: Record<string, number> = { high: 0, normal: 1, low: 2 };
                    const pa = priOrder[a.priority] ?? 1;
                    const pb = priOrder[b.priority] ?? 1;
                    if (pa !== pb) return pa - pb;
                    return a.id.localeCompare(b.id);
                  })
                  .map((caller) => {
                    const waitColor =
                      caller.waitSec > 300
                        ? "text-rose-400"
                        : caller.waitSec > 180
                          ? "text-amber-400"
                          : "text-slate-400";
                    const prioStyle: Record<string, string> = {
                      high: "bg-rose-500/15 text-rose-400 border-rose-500/20",
                      normal:
                        "bg-slate-500/10 text-slate-500 border-slate-500/15",
                      low: "bg-slate-500/8 text-slate-600 border-slate-500/10",
                    };
                    return (
                      <div
                        key={caller.id}
                        className="px-3 py-2 flex items-center gap-2 border-b border-white/[0.02] last:border-0 group hover:bg-white/[0.02] transition-colors"
                      >
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 mb-0.5">
                            <span className="text-[11px] font-mono text-slate-300">
                              {caller.number}
                            </span>
                            {caller.priority !== "normal" && (
                              <span
                                className={`inline-flex items-center px-1.5 py-0 rounded border text-[8px] font-mono font-semibold tracking-wider ${prioStyle[caller.priority]}`}
                              >
                                {caller.priority === "high" ? "HIGH" : "LOW"}
                              </span>
                            )}
                          </div>
                          <div className="flex items-center gap-2">
                            <span className="text-[10px] text-slate-600 truncate">
                              {caller.reason}
                            </span>
                            <span
                              className={`text-[10px] font-mono font-semibold shrink-0 ${waitColor}`}
                            >
                              {formatTime(caller.waitSec)}
                            </span>
                          </div>
                        </div>
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            onPickCall(caller, q.name);
                          }}
                          disabled={callActive}
                          className={`shrink-0 flex items-center gap-1 px-2.5 py-1.5 rounded-lg border text-[10px] font-mono tracking-wider transition-all active:scale-95 ${
                            callActive
                              ? "bg-white/[0.02] text-slate-700 border-white/[0.04] cursor-not-allowed"
                              : "bg-emerald-500/10 text-emerald-400 border-emerald-500/20 hover:bg-emerald-500/20 shadow-[0_0_20px_rgba(16,185,129,0.12)]"
                          }`}
                          title={
                            callActive
                              ? "End current call first"
                              : `Pick up ${caller.number}`
                          }
                        >
                          <PhoneIncoming size={12} strokeWidth={2} />
                          PICK
                        </button>
                      </div>
                    );
                  })}
              </div>
            )}

            {isExpanded && q.callers.length === 0 && (
              <div className="border-t border-white/[0.04] px-3 py-3 text-center animate-[fade-in_0.2s_ease-out]">
                <span className="text-[10px] font-mono text-slate-700">
                  Queue empty
                </span>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
