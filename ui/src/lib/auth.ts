const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";

export type UserRole = "admin" | "supervisor" | "agent" | "viewer";

// Role-based page access control
const PAGE_ACCESS: Record<string, UserRole[]> = {
  "/":               ["admin", "supervisor"],
  "/agents":         ["admin", "supervisor"],
  "/calls":          ["admin", "supervisor", "agent"],
  "/console":        ["admin", "supervisor", "agent"],
  "/documents":      ["admin"],
  "/security":       ["admin"],
  "/infrastructure": ["admin"],
  "/settings":       ["admin"],
};

// Which nav items each role can see
export function getNavForRole(role: string): string[] {
  return Object.entries(PAGE_ACCESS)
    .filter(([, roles]) => roles.includes(role as UserRole))
    .map(([path]) => path);
}

export function canAccessPage(path: string, role: string): boolean {
  // Login page is always accessible
  if (path === "/login") return true;
  // Check exact match first
  const allowed = PAGE_ACCESS[path];
  if (allowed) return allowed.includes(role as UserRole);
  // Check prefix match (e.g., /calls/live)
  for (const [pagePath, roles] of Object.entries(PAGE_ACCESS)) {
    if (path.startsWith(pagePath) && pagePath !== "/") {
      return roles.includes(role as UserRole);
    }
  }
  // Default: admin only
  return role === "admin";
}

export function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("voiceagent_token");
}

export function getUser(): { username: string; role: string } | null {
  if (typeof window === "undefined") return null;
  const data = localStorage.getItem("voiceagent_user");
  return data ? JSON.parse(data) : null;
}

export function getUserRole(): UserRole {
  const user = getUser();
  return (user?.role as UserRole) || "agent";
}

export function isAuthenticated(): boolean {
  return !!getToken();
}

export function logout() {
  localStorage.removeItem("voiceagent_token");
  localStorage.removeItem("voiceagent_user");
  window.location.href = "/login";
}

// Default landing page per role
export function getDefaultPage(role: string): string {
  switch (role) {
    case "admin": return "/";
    case "supervisor": return "/";
    case "agent": return "/console";
    default: return "/console";
  }
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
