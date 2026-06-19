"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";

export default function LoginPage() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const router = useRouter();

  async function handleLogin(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      const res = await fetch(`${GATEWAY}/api/auth/login`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password }),
      });

      const data = await res.json();

      if (!res.ok) {
        setError(data.error || "Login failed");
        setLoading(false);
        return;
      }

      localStorage.setItem("voiceagent_token", data.token);
      localStorage.setItem("voiceagent_user", JSON.stringify({
        username: data.username,
        role: data.role,
      }));

      // Redirect based on role
      const { getDefaultPage } = await import("@/lib/auth");
      router.push(getDefaultPage(data.role));
    } catch {
      setError("Cannot connect to gateway");
    }
    setLoading(false);
  }

  return (
    <div className="min-h-screen w-screen flex items-center justify-center bg-[#0a0e1a] fixed inset-0 z-50">
      <Card className="w-full max-w-sm bg-[#0f1629] border-cyan-500/15 glow-border p-8">
        {/* Logo */}
        <div className="flex items-center justify-center gap-3 mb-8">
          <div className="w-10 h-10 rounded-lg bg-cyan-500/20 flex items-center justify-center">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#06b6d4" strokeWidth="2.5">
              <polygon points="12 2 22 8.5 22 15.5 12 22 2 15.5 2 8.5 12 2" />
            </svg>
          </div>
          <div>
            <div className="font-mono text-lg font-semibold text-cyan-400 tracking-wider">VOICEAGENT</div>
            <div className="text-[10px] font-mono text-slate-600 tracking-widest">COMMAND CENTER</div>
          </div>
        </div>

        <form onSubmit={handleLogin} className="space-y-4">
          <div>
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">USERNAME</label>
            <Input
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="admin"
              className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm placeholder:text-slate-700"
              autoFocus
            />
          </div>

          <div>
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">PASSWORD</label>
            <Input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
              className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm placeholder:text-slate-700"
            />
          </div>

          {error && (
            <div className="p-2 rounded bg-rose-500/10 border border-rose-500/20">
              <p className="text-xs font-mono text-rose-400">{error}</p>
            </div>
          )}

          <Button
            type="submit"
            disabled={loading}
            className="w-full bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider py-5"
          >
            {loading ? "AUTHENTICATING..." : "LOGIN"}
          </Button>
        </form>

        <p className="text-[10px] font-mono text-slate-600 text-center mt-6">
          Default: admin / admin
        </p>
      </Card>
    </div>
  );
}
