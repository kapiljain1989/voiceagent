"use client";

import {
  CheckCircle,
  X,
  Sparkles,
  ListChecks,
  CircleDot,
  ClipboardCopy,
} from "lucide-react";
import type { PostCallSummaryData } from "@/lib/console-types";
import { ConsoleBadge } from "./Primitives";

function formatTime(seconds: number) {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}

export function PostCallSummary({
  summary,
  duration,
  visible,
  onDismiss,
}: {
  summary: PostCallSummaryData | null;
  duration: number;
  visible: boolean;
  onDismiss: () => void;
}) {
  if (!visible || !summary) return null;

  const sentColor: Record<string, string> = {
    positive: "emerald",
    negative: "rose",
    neutral: "slate",
  };
  const resColor: Record<string, string> = {
    resolved: "emerald",
    escalated: "amber",
    unresolved: "rose",
  };

  return (
    <div className="absolute inset-0 bg-[#0a0e1a]/90 backdrop-blur-sm z-40 flex items-center justify-center p-8 animate-[fade-in_0.2s_ease-out]">
      <div
        className="w-full max-w-2xl bg-[#0f1629] border border-cyan-500/[0.08] rounded-lg p-6 space-y-5 animate-[slide-up_0.3s_ease-out]"
        style={{ boxShadow: "0 0 40px rgba(6, 182, 212, 0.08)" }}
      >
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-emerald-500/15 flex items-center justify-center">
              <CheckCircle
                size={20}
                strokeWidth={1.8}
                className="text-emerald-400"
              />
            </div>
            <div>
              <h2 className="text-base font-semibold text-slate-200">
                Call Complete
              </h2>
              <span className="text-[10px] font-mono text-slate-500">
                Duration: {formatTime(duration)}
              </span>
            </div>
          </div>
          <button
            onClick={onDismiss}
            className="w-8 h-8 rounded-lg bg-white/[0.04] border border-white/[0.06] flex items-center justify-center text-slate-500 hover:text-slate-300 hover:bg-white/[0.08] transition-colors"
          >
            <X size={16} />
          </button>
        </div>

        <div className="grid grid-cols-4 gap-3">
          <div className="px-3 py-2.5 rounded-lg bg-[#070b14] border border-white/[0.04] text-center">
            <div className="text-[9px] font-mono text-slate-600 tracking-wider mb-1">
              SENTIMENT
            </div>
            <ConsoleBadge color={sentColor[summary.sentiment]}>
              {summary.sentiment.toUpperCase()}
            </ConsoleBadge>
          </div>
          <div className="px-3 py-2.5 rounded-lg bg-[#070b14] border border-white/[0.04] text-center">
            <div className="text-[9px] font-mono text-slate-600 tracking-wider mb-1">
              RESOLUTION
            </div>
            <ConsoleBadge color={resColor[summary.resolution || "unresolved"]}>
              {(summary.resolution || "unresolved").toUpperCase()}
            </ConsoleBadge>
          </div>
          <div className="px-3 py-2.5 rounded-lg bg-[#070b14] border border-white/[0.04] text-center">
            <div className="text-[9px] font-mono text-slate-600 tracking-wider mb-1">
              CSAT PRED
            </div>
            <span
              className={`text-lg font-mono font-semibold ${(summary.csat_prediction ?? 0) >= 4 ? "text-emerald-400" : (summary.csat_prediction ?? 0) >= 3 ? "text-amber-400" : "text-rose-400"}`}
            >
              {(summary.csat_prediction ?? 0).toFixed(1)}
            </span>
          </div>
          <div className="px-3 py-2.5 rounded-lg bg-[#070b14] border border-white/[0.04] text-center">
            <div className="text-[9px] font-mono text-slate-600 tracking-wider mb-1">
              DURATION
            </div>
            <span className="text-lg font-mono font-semibold text-cyan-400">
              {formatTime(duration)}
            </span>
          </div>
        </div>

        <div>
          <div className="flex items-center gap-1.5 mb-2">
            <Sparkles size={13} strokeWidth={2} className="text-violet-400" />
            <span className="text-[10px] font-mono text-violet-400 tracking-wider">
              AI GENERATED SUMMARY
            </span>
          </div>
          <p className="text-sm text-slate-300 leading-relaxed bg-[#070b14] rounded-lg p-3 border border-white/[0.04]">
            {summary.summary}
          </p>
        </div>

        <div>
          <div className="flex items-center gap-1.5 mb-2">
            <ListChecks
              size={13}
              strokeWidth={2}
              className="text-emerald-400"
            />
            <span className="text-[10px] font-mono text-emerald-400 tracking-wider">
              ACTION ITEMS
            </span>
          </div>
          <div className="space-y-1.5">
            {summary.action_items.map((item, i) => (
              <div
                key={i}
                className="flex items-start gap-2 px-3 py-2 rounded-md bg-[#070b14] border border-white/[0.04]"
              >
                <CircleDot
                  size={12}
                  strokeWidth={2}
                  className="text-emerald-400 mt-0.5 shrink-0"
                />
                <span className="text-[12px] text-slate-300 leading-relaxed">
                  {item}
                </span>
              </div>
            ))}
          </div>
        </div>

        {summary.topics && summary.topics.length > 0 && (
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-[9px] font-mono text-slate-600">
              Topics:
            </span>
            {summary.topics.map((t) => (
              <span
                key={t}
                className="px-2 py-0.5 rounded-full bg-white/[0.04] border border-white/[0.06] text-[10px] font-mono text-slate-500"
              >
                {t}
              </span>
            ))}
          </div>
        )}

        <div className="flex gap-3 pt-2 border-t border-white/[0.05]">
          <button
            onClick={onDismiss}
            className="flex-1 py-2.5 rounded-lg bg-cyan-500/10 text-cyan-400 border border-cyan-500/15 font-mono text-xs tracking-wider hover:bg-cyan-500/15 transition-colors flex items-center justify-center gap-2"
          >
            <ClipboardCopy size={14} strokeWidth={2} />
            COPY TO CRM
          </button>
          <button
            onClick={onDismiss}
            className="flex-1 py-2.5 rounded-lg bg-white/[0.04] text-slate-400 border border-white/[0.06] font-mono text-xs tracking-wider hover:bg-white/[0.06] transition-colors"
          >
            DISMISS
          </button>
        </div>
      </div>
    </div>
  );
}
