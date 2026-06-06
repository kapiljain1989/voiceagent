const GATEWAY_URL = process.env.GATEWAY_URL || "http://localhost:8080";

export async function gatewayFetch(path: string, options?: RequestInit) {
  const res = await fetch(`${GATEWAY_URL}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
  });
  return res;
}

export async function originateCall(to: string, from?: string, mode?: string) {
  const res = await gatewayFetch("/call", {
    method: "POST",
    body: JSON.stringify({ to, from, mode }),
  });
  return res.json();
}

export async function getGatewayHealth() {
  const res = await gatewayFetch("/healthz");
  return res.json();
}
