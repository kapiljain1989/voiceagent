"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { isAuthenticated } from "@/lib/auth";

export default function ConsoleLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const router = useRouter();
  const [checked, setChecked] = useState(false);

  useEffect(() => {
    if (!isAuthenticated()) {
      router.push("/login");
      return;
    }
    setChecked(true);
  }, [router]);

  if (!checked) {
    return <div className="h-screen bg-[#0a0e1a]" />;
  }

  return <>{children}</>;
}
