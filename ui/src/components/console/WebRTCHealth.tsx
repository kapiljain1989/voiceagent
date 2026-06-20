"use client";

import { useState, useEffect } from "react";
import type { CallState } from "@/lib/console-types";
import { StatusDot } from "./Primitives";

function AudioMeter({ active }: { active: boolean }) {
  const [levels, setLevels] = useState([0.2, 0.3, 0.5, 0.3, 0.2]);

  useEffect(() => {
    if (!active) {
      setLevels([0.1, 0.1, 0.1, 0.1, 0.1]);
      return;
    }
    const iv = setInterval(() => {
      setLevels(Array.from({ length: 5 }, () => 0.15 + Math.random() * 0.85));
    }, 120);
    return () => clearInterval(iv);
  }, [active]);

  return (
    <div className="flex items-end gap-px h-3">
      {levels.map((l, i) => (
        <div
          key={i}
          className={`w-[3px] rounded-sm transition-all duration-100 ${active ? "bg-emerald-400" : "bg-slate-700"}`}
          style={{ height: `${l * 100}%` }}
        />
      ))}
    </div>
  );
}

export function WebRTCHealth({ callState }: { callState: CallState }) {
  const isActive =
    callState === "connected" ||
    callState === "hold" ||
    callState === "muted";
  const [latency, setLatency] = useState(24);
  const [jitter, setJitter] = useState(2);

  useEffect(() => {
    if (!isActive) return;
    const iv = setInterval(() => {
      setLatency(Math.floor(18 + Math.random() * 20));
      setJitter(Math.floor(1 + Math.random() * 5));
    }, 2000);
    return () => clearInterval(iv);
  }, [isActive]);

  return (
    <div className="flex items-center gap-3 px-3 py-2 rounded-lg bg-[#070b14] border border-white/[0.04]">
      <div className="flex items-center gap-1.5">
        <StatusDot
          status={isActive ? "Available" : "Busy"}
          size={6}
          pulse={isActive}
        />
        <span className="text-[9px] font-mono text-slate-500 tracking-wider">
          SIP
        </span>
      </div>
      <div className="w-px h-4 bg-white/[0.06]" />

      <div className="flex items-center gap-1.5">
        <span
          className={`text-[10px] font-mono font-semibold ${latency < 30 ? "text-emerald-400" : latency < 60 ? "text-amber-400" : "text-rose-400"}`}
        >
          {latency}ms
        </span>
        <span className="text-[9px] font-mono text-slate-600">RTT</span>
      </div>
      <div className="w-px h-4 bg-white/[0.06]" />

      <div className="flex items-center gap-1.5">
        <span className="text-[10px] font-mono text-slate-400">{jitter}ms</span>
        <span className="text-[9px] font-mono text-slate-600">JTR</span>
      </div>
      <div className="w-px h-4 bg-white/[0.06]" />

      <div className="flex items-center gap-2">
        <div className="flex items-center gap-0.5">
          <span className="text-[9px] font-mono text-slate-600">IN</span>
          <AudioMeter active={isActive && callState !== "hold"} />
        </div>
        <div className="flex items-center gap-0.5">
          <span className="text-[9px] font-mono text-slate-600">OUT</span>
          <AudioMeter
            active={isActive && callState !== "muted" && callState !== "hold"}
          />
        </div>
      </div>
    </div>
  );
}
