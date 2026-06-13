"use client";

import { ArrowRightLeft, Users } from "lucide-react";
import type { TeamMember } from "@/lib/console-types";
import { StatusDot } from "./Primitives";

function getInitials(name: string) {
  return name
    .split(" ")
    .map((n) => n[0])
    .join("");
}

export function AgentDirectory({
  agents,
  onTransfer,
  onConference,
  callActive,
}: {
  agents: TeamMember[];
  onTransfer: (agent: TeamMember) => void;
  onConference: (agent: TeamMember) => void;
  callActive: boolean;
}) {
  return (
    <div className="space-y-0.5">
      {agents.map((a) => (
        <div
          key={a.id}
          className="flex items-center gap-2 px-2 py-2 rounded-md transition-colors group hover:bg-cyan-500/[0.03]"
        >
          <div className="w-7 h-7 rounded-full bg-white/[0.06] flex items-center justify-center shrink-0">
            <span className="text-[10px] font-mono text-slate-400">
              {getInitials(a.name)}
            </span>
          </div>
          <div className="flex-1 min-w-0">
            <div className="text-xs text-slate-300 truncate">{a.name}</div>
            <div className="flex items-center gap-1.5">
              <StatusDot
                status={a.status}
                size={5}
                pulse={a.status === "Available"}
              />
              <span className="text-[9px] font-mono text-slate-600">
                {a.status}
              </span>
            </div>
          </div>
          {callActive && a.status === "Available" && (
            <div className="hidden group-hover:flex gap-1 animate-[fade-in_0.2s_ease-out]">
              <button
                onClick={() => onTransfer(a)}
                title="Transfer"
                className="w-6 h-6 rounded-md bg-cyan-500/10 text-cyan-400 flex items-center justify-center hover:bg-cyan-500/20"
              >
                <ArrowRightLeft size={12} strokeWidth={2} />
              </button>
              <button
                onClick={() => onConference(a)}
                title="Conference"
                className="w-6 h-6 rounded-md bg-violet-500/10 text-violet-400 flex items-center justify-center hover:bg-violet-500/20"
              >
                <Users size={12} strokeWidth={2} />
              </button>
            </div>
          )}
          <span className="text-[10px] font-mono text-slate-700 group-hover:hidden">
            x{a.ext}
          </span>
        </div>
      ))}
    </div>
  );
}
