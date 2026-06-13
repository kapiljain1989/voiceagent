"use client";

import { useRef, useEffect } from "react";
import {
  MessageCircle,
  Heart,
  ShieldCheck,
  TrendingUp,
} from "lucide-react";
import type { CopilotSuggestion } from "@/lib/console-types";

const CAT_STYLE = {
  answer: {
    bg: "bg-cyan-500/[0.06]",
    border: "border-cyan-500/15",
    text: "text-cyan-400",
    label: "ANSWER",
    Icon: MessageCircle,
  },
  empathy: {
    bg: "bg-violet-500/[0.06]",
    border: "border-violet-500/15",
    text: "text-violet-400",
    label: "EMPATHY",
    Icon: Heart,
  },
  compliance: {
    bg: "bg-amber-500/[0.06]",
    border: "border-amber-500/15",
    text: "text-amber-400",
    label: "COMPLIANCE",
    Icon: ShieldCheck,
  },
  upsell: {
    bg: "bg-pink-500/[0.06]",
    border: "border-pink-500/15",
    text: "text-pink-400",
    label: "UPSELL",
    Icon: TrendingUp,
  },
} as const;

export function AICopilotPanel({
  suggestions,
  isActive,
}: {
  suggestions: CopilotSuggestion[];
  isActive: boolean;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (scrollRef.current)
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
  }, [suggestions]);

  if (!isActive) {
    return (
      <div className="h-full flex items-center justify-center">
        <span className="text-[10px] font-mono text-slate-700">
          Connect a call for AI assist
        </span>
      </div>
    );
  }

  return (
    <div ref={scrollRef} className="h-full overflow-y-auto space-y-2 px-1">
      {suggestions.length === 0 && (
        <div className="flex flex-col items-center justify-center py-8 gap-2">
          <div className="flex gap-1">
            <span
              className="w-1.5 h-1.5 rounded-full bg-violet-400 animate-bounce"
              style={{ animationDelay: "0ms" }}
            />
            <span
              className="w-1.5 h-1.5 rounded-full bg-violet-400 animate-bounce"
              style={{ animationDelay: "150ms" }}
            />
            <span
              className="w-1.5 h-1.5 rounded-full bg-violet-400 animate-bounce"
              style={{ animationDelay: "300ms" }}
            />
          </div>
          <span className="text-[10px] font-mono text-slate-600">
            Analyzing conversation...
          </span>
        </div>
      )}
      {suggestions.map((s, i) => {
        const style = CAT_STYLE[s.category] || CAT_STYLE.answer;
        const { Icon } = style;
        return (
          <div
            key={i}
            className={`p-2.5 rounded-lg ${style.bg} border ${style.border} animate-[slide-up_0.3s_ease-out]`}
          >
            <div className="flex items-center justify-between mb-1.5">
              <div className="flex items-center gap-1.5">
                <Icon size={11} strokeWidth={2} className={style.text} />
                <span
                  className={`text-[9px] font-mono font-semibold tracking-wider ${style.text}`}
                >
                  {style.label}
                </span>
              </div>
              <span className="text-[9px] font-mono text-slate-600">
                {Math.round(s.confidence * 100)}%
              </span>
            </div>
            <p className="text-[12px] text-slate-300 leading-relaxed">
              {s.text}
            </p>
            {s.time && (
              <span className="text-[9px] font-mono text-slate-700 mt-1 block">
                {s.time}
              </span>
            )}
          </div>
        );
      })}
    </div>
  );
}
