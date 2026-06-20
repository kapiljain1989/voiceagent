"use client";

import { useEffect, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { Sidebar } from "@/components/layout/Sidebar";
import { isAuthenticated } from "@/lib/auth";

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

  if (isLoginPage) {
    return <>{children}</>;
  }

  if (!checked) {
    return <div className="min-h-screen bg-[#0a0e1a]" />;
  }

  return (
    <>
      <Sidebar />
      <main className="main-content flex-1 min-h-screen transition-[margin-left] duration-200 ml-56">
        <div className="p-6 lg:p-8">{children}</div>
      </main>
    </>
  );
}
