"use client";

import { useState } from "react";
import { X } from "lucide-react";
import { DEPARTMENTS } from "@/lib/console-types";
import type { TeamMember } from "@/lib/console-types";
import { StatusDot } from "./Primitives";

export function TransferPanel({
  visible,
  agents,
  onBlindTransfer,
  onAttendedTransfer,
  onClose,
}: {
  visible: boolean;
  agents: TeamMember[];
  onBlindTransfer: (target: string) => void;
  onAttendedTransfer: (target: string) => void;
  onClose: () => void;
}) {
  const [mode, setMode] = useState<"blind" | "attended">("blind");
  const [search, setSearch] = useState("");

  if (!visible) return null;

  const available = agents.filter(
    (a) =>
      a.status === "Available" &&
      a.name.toLowerCase().includes(search.toLowerCase()),
  );

  const handleTransfer = (target: string) => {
    if (mode === "blind") onBlindTransfer(target);
    else onAttendedTransfer(target);
  };

  return (
    <div className="animate-[slide-up_0.3s_ease-out] bg-[#0f1629] border border-cyan-500/[0.08] rounded-lg p-4">
      <div className="flex items-center justify-between mb-3">
        <span className="text-[10px] font-mono text-slate-500 tracking-wider">
          TRANSFER CALL
        </span>
        <button
          onClick={onClose}
          className="text-slate-500 hover:text-slate-300"
        >
          <X size={14} />
        </button>
      </div>

      <div className="flex gap-1 mb-3 p-0.5 rounded-lg bg-[#070b14]">
        {(["blind", "attended"] as const).map((m) => (
          <button
            key={m}
            onClick={() => setMode(m)}
            className={`flex-1 py-1.5 rounded-md text-[10px] font-mono tracking-wider transition-colors ${mode === m ? "bg-cyan-500/15 text-cyan-400" : "text-slate-500 hover:text-slate-400"}`}
          >
            {m === "blind" ? "BLIND" : "ATTENDED"}
          </button>
        ))}
      </div>

      <div className="flex gap-1.5 mb-3 flex-wrap">
        {DEPARTMENTS.map((d) => (
          <button
            key={d}
            onClick={() => handleTransfer(d)}
            className="px-2.5 py-1 rounded-md bg-[#070b14] border border-white/[0.04] text-[10px] font-mono text-slate-400 hover:text-cyan-400 hover:border-cyan-500/20 transition-colors"
          >
            {d}
          </button>
        ))}
      </div>

      <input
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        placeholder="Search agents..."
        className="w-full px-3 py-2 rounded-md bg-[#070b14] border border-white/[0.06] text-sm text-slate-300 font-mono placeholder:text-slate-700 mb-2 focus:outline-none focus:ring-1 focus:ring-cyan-500/30"
      />

      <div className="max-h-32 overflow-y-auto space-y-1">
        {available.map((a) => (
          <button
            key={a.id}
            onClick={() => handleTransfer(a.name)}
            className="w-full flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-cyan-500/[0.05] transition-colors text-left"
          >
            <StatusDot status={a.status} size={6} />
            <span className="text-xs text-slate-300">{a.name}</span>
            <span className="text-[10px] font-mono text-slate-600 ml-auto">
              x{a.ext}
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}
