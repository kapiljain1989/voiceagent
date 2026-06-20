"use client";

import { useRef, useEffect } from "react";
import { FileText, CheckCircle2, Heart, ShieldCheck } from "lucide-react";
import type { TranscriptEntry, AIInsight } from "@/lib/console-types";

const INSIGHT_ICON = {
  summary: FileText,
  action: CheckCircle2,
  sentiment: Heart,
  compliance: ShieldCheck,
} as const;

const INSIGHT_COLOR: Record<string, string> = {
  summary: "text-cyan-400",
  action: "text-emerald-400",
  sentiment: "text-violet-400",
  compliance: "text-amber-400",
};

export function TranscriptPanel({
  entries,
  insights,
}: {
  entries: TranscriptEntry[];
  insights: AIInsight[];
}) {
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (scrollRef.current)
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
  }, [entries, insights]);

  return (
    <div ref={scrollRef} className="h-full overflow-y-auto px-1 space-y-3">
      {entries.map((e, i) => (
        <div key={`t-${i}`} className="animate-[slide-up_0.3s_ease-out]">
          <div className="flex items-center gap-2 mb-0.5">
            <span
              className={`text-[9px] font-mono font-semibold tracking-wider ${e.speaker === "customer" ? "text-cyan-400" : "text-emerald-400"}`}
            >
              {e.speaker === "customer" ? "CALLER" : "AGENT"}
            </span>
            <span className="text-[9px] font-mono text-slate-700">
              {e.time}
            </span>
          </div>
          <p className="text-[13px] text-slate-400 leading-relaxed">{e.text}</p>
        </div>
      ))}
      {insights.length > 0 && (
        <div className="pt-2 border-t border-violet-500/10 space-y-2">
          <span className="text-[9px] font-mono text-violet-400 tracking-wider">
            AI INSIGHTS
          </span>
          {insights.map((ins, i) => {
            const Icon = INSIGHT_ICON[ins.type] || FileText;
            return (
              <div
                key={`i-${i}`}
                className="flex gap-2 p-2 rounded-md bg-violet-500/[0.04] border border-violet-500/10 animate-[slide-up_0.3s_ease-out]"
              >
                <Icon
                  size={13}
                  strokeWidth={1.8}
                  className={`shrink-0 mt-0.5 ${INSIGHT_COLOR[ins.type] || "text-slate-400"}`}
                />
                <p className="text-[12px] text-slate-400 leading-relaxed">
                  {ins.text}
                </p>
              </div>
            );
          })}
        </div>
      )}
      {entries.length === 0 && (
        <div className="h-full flex items-center justify-center">
          <span className="text-xs text-slate-700 font-mono">
            Waiting for call...
          </span>
        </div>
      )}
    </div>
  );
}
