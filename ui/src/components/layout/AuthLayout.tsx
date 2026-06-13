"use client";

import { useEffect, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { Sidebar } from "@/components/layout/Sidebar";
import { isAuthenticated, getUser, logout } from "@/lib/auth";

export function AuthLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [checked, setChecked] = useState(false);

  const isLoginPage = pathname === "/login";

  useEffect(() => {
    if (!isLoginPage && !isAuthenticated()) {
      router.push("/login");
    }
    setChecked(true);
  }, [pathname, isLoginPage, router]);

  // Login page — no sidebar, full screen
  if (isLoginPage) {
    return <>{children}</>;
  }

  // Not checked yet — blank screen to prevent flash
  if (!checked) {
    return <div className="min-h-screen bg-[#0a0e1a]" />;
  }

  // Authenticated — show sidebar + content + user badge
  const user = getUser();

  return (
    <>
      <Sidebar />
      <main className="main-content flex-1 min-h-screen transition-[margin-left] duration-200 ml-56">
        {/* Top bar with user info */}
        <div className="flex items-center justify-end px-6 lg:px-8 pt-4">
          {user && (
            <div className="flex items-center gap-3">
              <div className="text-right">
                <div className="text-xs font-mono text-slate-400">{user.username}</div>
                <div className="text-[10px] font-mono text-slate-600">{user.role}</div>
              </div>
              <button
                onClick={logout}
                className="px-3 py-1.5 rounded text-[10px] font-mono text-slate-500 hover:text-rose-400 hover:bg-rose-500/10 border border-slate-800 hover:border-rose-500/20 transition-colors"
              >
                LOGOUT
              </button>
            </div>
          )}
        </div>
        <div className="p-6 lg:p-8 pt-2">{children}</div>
      </main>
    </>
  );
}
