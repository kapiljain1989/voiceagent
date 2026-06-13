"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";

const nav = [
  { href: "/", label: "Command", icon: "grid", shortcut: "D" },
  { href: "/agents", label: "Agents", icon: "users", shortcut: "A" },
  { href: "/calls", label: "Calls", icon: "phone", shortcut: "C" },
  { href: "/calls/live", label: "Live Ops", icon: "radio", shortcut: "L" },
  { href: "/console", label: "Console", icon: "headset", shortcut: "O" },
  { href: "/documents", label: "Knowledge", icon: "file", shortcut: "K" },
  { href: "/security", label: "Security", icon: "shield", shortcut: "X" },
  { href: "/infrastructure", label: "Infra", icon: "server", shortcut: "I" },
  { href: "/settings", label: "Config", icon: "sliders", shortcut: "S" },
];

const icons: Record<string, React.ReactNode> = {
  grid: (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <rect x="3" y="3" width="7" height="7" rx="1" />
      <rect x="14" y="3" width="7" height="7" rx="1" />
      <rect x="3" y="14" width="7" height="7" rx="1" />
      <rect x="14" y="14" width="7" height="7" rx="1" />
    </svg>
  ),
  users: (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
      <circle cx="9" cy="7" r="4" />
      <path d="M22 21v-2a4 4 0 0 0-3-3.87" />
      <path d="M16 3.13a4 4 0 0 1 0 7.75" />
    </svg>
  ),
  phone: (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <path d="M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6 19.79 19.79 0 0 1-3.07-8.67A2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72c.127.96.361 1.903.7 2.81a2 2 0 0 1-.45 2.11L8.09 9.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45c.907.339 1.85.573 2.81.7A2 2 0 0 1 22 16.92z" />
    </svg>
  ),
  radio: (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <circle cx="12" cy="12" r="2" />
      <path d="M16.24 7.76a6 6 0 0 1 0 8.49m-8.48-.01a6 6 0 0 1 0-8.49m11.31-2.82a10 10 0 0 1 0 14.14m-14.14 0a10 10 0 0 1 0-14.14" />
    </svg>
  ),
  file: (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z" />
      <polyline points="14,2 14,8 20,8" />
      <line x1="16" y1="13" x2="8" y2="13" />
      <line x1="16" y1="17" x2="8" y2="17" />
    </svg>
  ),
  shield: (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
    </svg>
  ),
  server: (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
      <rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
      <line x1="6" y1="6" x2="6.01" y2="6" />
      <line x1="6" y1="18" x2="6.01" y2="18" />
    </svg>
  ),
  headset: (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <path d="M3 18v-6a9 9 0 0 1 18 0v6" />
      <path d="M21 19a2 2 0 0 1-2 2h-1a2 2 0 0 1-2-2v-3a2 2 0 0 1 2-2h3zM3 19a2 2 0 0 0 2 2h1a2 2 0 0 0 2-2v-3a2 2 0 0 0-2-2H3z" />
    </svg>
  ),
  sliders: (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <line x1="4" y1="21" x2="4" y2="14" />
      <line x1="4" y1="10" x2="4" y2="3" />
      <line x1="12" y1="21" x2="12" y2="12" />
      <line x1="12" y1="8" x2="12" y2="3" />
      <line x1="20" y1="21" x2="20" y2="16" />
      <line x1="20" y1="12" x2="20" y2="3" />
      <line x1="1" y1="14" x2="7" y2="14" />
      <line x1="9" y1="8" x2="15" y2="8" />
      <line x1="17" y1="16" x2="23" y2="16" />
    </svg>
  ),
};

export function Sidebar() {
  const pathname = usePathname();
  const [expanded, setExpanded] = useState(true);

  useEffect(() => {
    document.documentElement.dataset.sidebarCollapsed = expanded ? "false" : "true";
  }, [expanded]);

  return (
    <>
      {/* Mobile backdrop */}
      {expanded && (
        <div className="fixed inset-0 bg-black/40 z-40 lg:hidden" onClick={() => setExpanded(false)} />
      )}

      <aside
        className={`fixed left-0 top-0 h-screen bg-[#070b14] border-r border-cyan-500/10 flex flex-col z-50 transition-all duration-200 ${
          expanded ? "w-56" : "w-16"
        }`}
      >
        {/* Logo + toggle */}
        <div className="h-16 flex items-center justify-between px-4 border-b border-cyan-500/10">
          <div className="flex items-center min-w-0">
            <div className="w-8 h-8 shrink-0 rounded bg-cyan-500/20 flex items-center justify-center">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="#06b6d4" strokeWidth="2.5">
                <polygon points="12 2 22 8.5 22 15.5 12 22 2 15.5 2 8.5 12 2" />
              </svg>
            </div>
            {expanded && (
              <span className="ml-3 font-mono text-sm font-semibold text-cyan-400 tracking-wider whitespace-nowrap">
                VOICEAGENT
              </span>
            )}
          </div>
          <button
            onClick={() => setExpanded(!expanded)}
            className="w-6 h-6 shrink-0 flex items-center justify-center rounded text-slate-600 hover:text-cyan-400 transition-colors"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
              {expanded ? (
                <>
                  <polyline points="11 17 6 12 11 7" />
                  <polyline points="18 17 13 12 18 7" />
                </>
              ) : (
                <>
                  <polyline points="13 7 18 12 13 17" />
                  <polyline points="6 7 11 12 6 17" />
                </>
              )}
            </svg>
          </button>
        </div>

        {/* Nav */}
        <nav className="flex-1 py-4 space-y-1 px-2 overflow-y-auto">
          {nav.map((item) => {
            const active = item.href === "/" ? pathname === "/" : pathname.startsWith(item.href);
            return (
              <Link
                key={item.href}
                href={item.href}
                title={expanded ? undefined : item.label}
                onClick={() => { if (window.innerWidth < 1024) setExpanded(false); }}
                className={`
                  flex items-center gap-3 px-2.5 py-2.5 rounded-md text-sm transition-all duration-150
                  ${expanded ? "" : "justify-center"}
                  ${active
                    ? "bg-cyan-500/10 text-cyan-400 glow-border"
                    : "text-slate-500 hover:text-slate-300 hover:bg-white/[0.03]"
                  }
                `}
              >
                <span className={`shrink-0 ${active ? "text-cyan-400" : "text-slate-600"}`}>
                  {icons[item.icon]}
                </span>
                {expanded && <span className="font-medium whitespace-nowrap">{item.label}</span>}
                {expanded && active && (
                  <span className="ml-auto w-1.5 h-1.5 rounded-full bg-cyan-400 animate-pulse-glow" />
                )}
              </Link>
            );
          })}
        </nav>

        {/* Status */}
        <div className="px-3 py-3 border-t border-cyan-500/10">
          <div className={`flex items-center gap-2 text-xs font-mono text-slate-600 ${expanded ? "" : "justify-center"}`}>
            <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
            {expanded && <span>SYS ONLINE</span>}
          </div>
        </div>
      </aside>
    </>
  );
}
