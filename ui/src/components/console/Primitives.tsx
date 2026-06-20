import { STATUS_COLORS } from "@/lib/console-types";
import type { LucideIcon } from "lucide-react";

// ── StatusDot ──

const DOT_BG: Record<string, string> = {
  emerald: "bg-emerald-500",
  rose: "bg-rose-500",
  amber: "bg-amber-500",
  violet: "bg-violet-500",
  cyan: "bg-cyan-400",
  slate: "bg-slate-500",
};

export function StatusDot({
  status,
  size = 8,
  pulse = false,
}: {
  status: string;
  size?: number;
  pulse?: boolean;
}) {
  const color = STATUS_COLORS[status] || "slate";
  return (
    <span
      className={`inline-block rounded-full ${DOT_BG[color] || DOT_BG.slate} ${pulse ? "animate-pulse" : ""}`}
      style={{ width: size, height: size }}
    />
  );
}

// ── Badge ──

const BADGE_STYLES: Record<string, string> = {
  emerald: "bg-emerald-500/12 text-emerald-400 border-emerald-500/20",
  rose: "bg-rose-500/12 text-rose-400 border-rose-500/20",
  amber: "bg-amber-500/12 text-amber-400 border-amber-500/20",
  violet: "bg-violet-500/12 text-violet-400 border-violet-500/20",
  cyan: "bg-cyan-500/12 text-cyan-400 border-cyan-500/20",
  slate: "bg-slate-500/12 text-slate-400 border-slate-500/20",
};

export function ConsoleBadge({
  children,
  color = "slate",
  className = "",
}: {
  children: React.ReactNode;
  color?: string;
  className?: string;
}) {
  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded border text-[10px] font-mono font-medium tracking-wide ${BADGE_STYLES[color] || BADGE_STYLES.slate} ${className}`}
    >
      {children}
    </span>
  );
}

// ── IconBtn ──

const SIZE_MAP = { sm: "w-9 h-9", md: "w-12 h-12", lg: "w-14 h-14" };
const ICON_SIZE = { sm: 16, md: 20, lg: 22 };

const BTN_COLORS: Record<string, { active: string; inactive: string }> = {
  emerald: {
    active:
      "bg-emerald-500/20 text-emerald-400 border-emerald-500/30 shadow-[0_0_20px_rgba(16,185,129,0.12)]",
    inactive:
      "bg-emerald-500/8 text-emerald-400/70 border-emerald-500/15 hover:bg-emerald-500/15",
  },
  rose: {
    active:
      "bg-rose-500/20 text-rose-400 border-rose-500/30 shadow-[0_0_20px_rgba(244,63,94,0.12)]",
    inactive:
      "bg-rose-500/8 text-rose-400/70 border-rose-500/15 hover:bg-rose-500/15",
  },
  amber: {
    active:
      "bg-amber-500/20 text-amber-400 border-amber-500/30 shadow-[0_0_20px_rgba(245,158,11,0.12)]",
    inactive:
      "bg-amber-500/8 text-amber-400/70 border-amber-500/15 hover:bg-amber-500/15",
  },
  cyan: {
    active:
      "bg-cyan-500/20 text-cyan-400 border-cyan-500/30 shadow-[0_0_20px_rgba(6,182,212,0.12)]",
    inactive:
      "bg-cyan-500/8 text-cyan-400/70 border-cyan-500/15 hover:bg-cyan-500/15",
  },
  slate: {
    active: "bg-white/[0.08] text-slate-300 border-white/[0.1]",
    inactive:
      "bg-white/[0.04] text-slate-400 border-white/[0.06] hover:bg-white/[0.08]",
  },
};

export function IconBtn({
  Icon,
  label,
  onClick,
  active = false,
  color = "slate",
  size = "md",
  disabled = false,
}: {
  Icon: LucideIcon;
  label?: string;
  onClick?: () => void;
  active?: boolean;
  color?: string;
  size?: "sm" | "md" | "lg";
  disabled?: boolean;
}) {
  const scheme = BTN_COLORS[color] || BTN_COLORS.slate;
  return (
    <button
      onClick={disabled ? undefined : onClick}
      disabled={disabled}
      className={`transition-all duration-150 cursor-pointer select-none active:scale-95 ${SIZE_MAP[size]} rounded-xl border flex flex-col items-center justify-center gap-1 ${active ? scheme.active : scheme.inactive} ${disabled ? "opacity-30 cursor-not-allowed" : ""}`}
      title={label}
    >
      <Icon size={ICON_SIZE[size]} strokeWidth={1.8} />
      {label && size !== "sm" && (
        <span className="text-[8px] font-mono tracking-wider opacity-70">
          {label}
        </span>
      )}
    </button>
  );
}
