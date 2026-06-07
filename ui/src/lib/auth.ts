const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";

export function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("voiceagent_token");
}

export function getUser(): { username: string; role: string } | null {
  if (typeof window === "undefined") return null;
  const data = localStorage.getItem("voiceagent_user");
  return data ? JSON.parse(data) : null;
}

export function isAuthenticated(): boolean {
  return !!getToken();
}

export function logout() {
  localStorage.removeItem("voiceagent_token");
  localStorage.removeItem("voiceagent_user");
  window.location.href = "/login";
}

export async function authFetch(path: string, options?: RequestInit): Promise<Response> {
  const token = getToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(options?.headers as Record<string, string> || {}),
  };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const res = await fetch(`${GATEWAY}${path}`, { ...options, headers });

  if (res.status === 401) {
    logout();
  }

  return res;
}
