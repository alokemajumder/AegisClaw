const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

async function apiFetch(path: string, options?: RequestInit) {
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
  });
  if (!res.ok) throw new Error(`API error: ${res.status}`);
  return res.json();
}

export const api = {
  get: (path: string) => apiFetch(path),
  post: (path: string, data: unknown) =>
    apiFetch(path, { method: "POST", body: JSON.stringify(data) }),
  put: (path: string, data: unknown) =>
    apiFetch(path, { method: "PUT", body: JSON.stringify(data) }),
  delete: (path: string) => apiFetch(path, { method: "DELETE" }),
};
