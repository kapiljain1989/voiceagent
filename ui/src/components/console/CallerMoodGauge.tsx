"use client";

import type { VoiceSentimentData } from "@/lib/console-types";

function GaugeBar({
  label,
  value,
  highThreshold = 0.6,
  midThreshold = 0.3,
}: {
  label: string;
  value: number;
  highThreshold?: number;
  midThreshold?: number;
}) {
  const color =
    value > highThreshold
      ? "bg-rose-500"
      : value > midThreshold
        ? "bg-amber-500"
        : "bg-emerald-500";
  const textColor =
    value > highThreshold
      ? "text-rose-400"
      : value > midThreshold
        ? "text-amber-400"
        : "text-emerald-400";

  return (
    <div className="flex items-center gap-2">
      <span className="text-[9px] font-mono text-slate-600 w-8 shrink-0">
        {label}
      </span>
      <div className="flex-1 h-1.5 rounded-full bg-white/[0.04] overflow-hidden">
        <div
          className={`h-full rounded-full ${color} transition-all duration-700`}
          style={{ width: `${Math.round(value * 100)}%` }}
        />
      </div>
      <span
        className={`text-[10px] font-mono font-semibold w-8 text-right ${textColor}`}
      >
        {Math.round(value * 100)}%
      </span>
    </div>
  );
}

function GaugeBarInverted({
  label,
  value,
  highThreshold = 0.6,
  midThreshold = 0.3,
}: {
  label: string;
  value: number;
  highThreshold?: number;
  midThreshold?: number;
}) {
  const color =
    value > highThreshold
      ? "bg-emerald-500"
      : value > midThreshold
        ? "bg-amber-500"
        : "bg-rose-500";
  const textColor =
    value > highThreshold
      ? "text-emerald-400"
      : value > midThreshold
        ? "text-amber-400"
        : "text-rose-400";

  return (
    <div className="flex items-center gap-2">
      <span className="text-[9px] font-mono text-slate-600 w-8 shrink-0">
        {label}
      </span>
      <div className="flex-1 h-1.5 rounded-full bg-white/[0.04] overflow-hidden">
        <div
          className={`h-full rounded-full ${color} transition-all duration-700`}
          style={{ width: `${Math.round(value * 100)}%` }}
        />
      </div>
      <span
        className={`text-[10px] font-mono font-semibold w-8 text-right ${textColor}`}
      >
        {Math.round(value * 100)}%
      </span>
    </div>
  );
}

export function CallerMoodGauge({
  sentiment,
  isActive,
}: {
  sentiment: VoiceSentimentData | null;
  isActive: boolean;
}) {
  if (!isActive || !sentiment) {
    return (
      <div className="px-3 py-3 border-b border-white/[0.05]">
        <span className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">
          CALLER MOOD
        </span>
        <div className="flex items-center justify-center py-4">
          <span className="text-[10px] font-mono text-slate-700">
            No active call
          </span>
        </div>
      </div>
    );
  }

  const mood =
    sentiment.frustration > 0.6
      ? "FRUSTRATED"
      : sentiment.agitation > 0.5
        ? "AGITATED"
        : sentiment.engagement > 0.7
          ? "ENGAGED"
          : "CALM";
  const moodColor =
    sentiment.frustration > 0.6
      ? "rose"
      : sentiment.agitation > 0.5
        ? "amber"
        : sentiment.engagement > 0.7
          ? "emerald"
          : "cyan";

  const borderMap: Record<string, string> = {
    rose: "border-rose-500/20",
    amber: "border-amber-500/20",
    emerald: "border-emerald-500/20",
    cyan: "border-cyan-500/20",
  };
  const glowMap: Record<string, string> = {
    rose: "shadow-[0_0_20px_rgba(244,63,94,0.12)]",
    amber: "shadow-[0_0_20px_rgba(245,158,11,0.12)]",
    emerald: "shadow-[0_0_20px_rgba(16,185,129,0.12)]",
    cyan: "shadow-[0_0_20px_rgba(6,182,212,0.12)]",
  };
  const bgMap: Record<string, string> = {
    rose: "bg-rose-500",
    amber: "bg-amber-500",
    emerald: "bg-emerald-500",
    cyan: "bg-cyan-400",
  };
  const textMap: Record<string, string> = {
    rose: "text-rose-400",
    amber: "text-amber-400",
    emerald: "text-emerald-400",
    cyan: "text-cyan-400",
  };

  return (
    <div
      className={`px-3 py-3 border-b ${borderMap[moodColor]} transition-colors duration-500`}
    >
      <div className="flex items-center justify-between mb-3">
        <span className="text-[10px] font-mono text-slate-500 tracking-wider">
          CALLER MOOD
        </span>
        <div
          className={`flex items-center gap-1.5 px-2 py-0.5 rounded-full border ${borderMap[moodColor]} ${glowMap[moodColor]}`}
        >
          <span
            className={`w-2 h-2 rounded-full ${bgMap[moodColor]} animate-pulse`}
          />
          <span
            className={`text-[10px] font-mono font-semibold tracking-wider ${textMap[moodColor]}`}
          >
            {mood}
          </span>
        </div>
      </div>

      <div className="space-y-2 mb-3">
        <GaugeBar
          label="FRS"
          value={sentiment.frustration}
          midThreshold={0.3}
          highThreshold={0.6}
        />
        <GaugeBar
          label="AGT"
          value={sentiment.agitation}
          midThreshold={0.3}
          highThreshold={0.5}
        />
        <GaugeBarInverted
          label="ENG"
          value={sentiment.engagement}
          midThreshold={0.4}
          highThreshold={0.7}
        />
      </div>

      <div className="grid grid-cols-2 gap-x-3 gap-y-1.5 text-[9px] font-mono">
        <div className="flex justify-between">
          <span className="text-slate-600">Pitch</span>
          <span className="text-slate-400">
            {sentiment.avg_pitch_hz.toFixed(0)} Hz
          </span>
        </div>
        <div className="flex justify-between">
          <span className="text-slate-600">Speed</span>
          <span className="text-slate-400">
            {sentiment.speaking_rate_wpm.toFixed(0)} wpm
          </span>
        </div>
        <div className="flex justify-between">
          <span className="text-slate-600">Energy</span>
          <span
            className={
              sentiment.energy_trend === "rising"
                ? "text-amber-400"
                : sentiment.energy_trend === "falling"
                  ? "text-emerald-400"
                  : "text-slate-400"
            }
          >
            {sentiment.energy_trend === "rising"
              ? "↑"
              : sentiment.energy_trend === "falling"
                ? "↓"
                : "↔"}{" "}
            {sentiment.energy_trend}
          </span>
        </div>
        <div className="flex justify-between">
          <span className="text-slate-600">Silence</span>
          <span className="text-slate-400">
            {(sentiment.silence_ratio * 100).toFixed(0)}%
          </span>
        </div>
      </div>

      <div className="mt-2 pt-2 border-t border-white/[0.03] flex items-center justify-between">
        <span className="text-[9px] font-mono text-slate-700">
          AI confidence
        </span>
        <span className="text-[9px] font-mono text-slate-500">
          {(sentiment.confidence * 100).toFixed(0)}%
        </span>
      </div>
    </div>
  );
}
