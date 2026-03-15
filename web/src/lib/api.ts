import type {
  ApiResponse,
  Asset,
  Engagement,
  Run,
  RunStep,
  Finding,
  ConnectorRegistry,
  ConnectorInstance,
  Approval,
  Report,
  DashboardSummary,
  DashboardHealth,
  AuthTokens,
  User,
} from "./types";

// In production, API calls go through Next.js rewrites (relative path).
// NEXT_PUBLIC_API_URL can override for direct backend access during development.
const API_BASE = process.env.NEXT_PUBLIC_API_URL || "";
const TOKEN_KEY = "aegisclaw_token";

// Token management
export function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token);
  // Also set as cookie for middleware auth protection
  const secure = window.location.hostname !== "localhost" && window.location.hostname !== "127.0.0.1" ? "; Secure" : "";
  document.cookie = `${TOKEN_KEY}=${token}; path=/; max-age=${7 * 24 * 60 * 60}; SameSite=Lax${secure}`;
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
  document.cookie = `${TOKEN_KEY}=; path=/; max-age=0`;
}

export function isAuthenticated(): boolean {
  return !!getToken();
}

// Retry config
const MAX_RETRIES = 3;
const BASE_DELAY_MS = 200;
const RETRYABLE_STATUSES = new Set([408, 429, 500, 502, 503, 504]);

function isRetryable(status: number, method?: string): boolean {
  // Only retry on safe methods (GET, HEAD) or transient server errors
  if (status === 429) return true; // rate limited — always retry
  if (!RETRYABLE_STATUSES.has(status)) return false;
  const m = (method ?? "GET").toUpperCase();
  return m === "GET" || m === "HEAD" || status >= 500;
}

function retryDelay(attempt: number): number {
  // Exponential backoff with jitter: 200ms, 400ms, 800ms + random 0–100ms
  return BASE_DELAY_MS * Math.pow(2, attempt) + Math.random() * 100;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// Base fetch with auth and retry
async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const fetchOpts: RequestInit = {
    ...options,
    headers: {
      ...headers,
      ...options?.headers,
    },
  };

  let lastError: Error | undefined;

  for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
    if (attempt > 0) {
      await sleep(retryDelay(attempt - 1));
    }

    let res: Response;
    try {
      res = await fetch(`${API_BASE}${path}`, fetchOpts);
    } catch (err) {
      // Network error — retry
      lastError = err instanceof Error ? err : new Error(String(err));
      if (attempt < MAX_RETRIES) continue;
      throw lastError;
    }

    if (res.status === 401) {
      clearToken();
      if (typeof window !== "undefined") {
        window.location.href = "/login";
      }
      throw new Error("Unauthorized");
    }

    if (!res.ok) {
      if (attempt < MAX_RETRIES && isRetryable(res.status, options?.method)) {
        continue;
      }
      const body = await res.json().catch(() => ({}));
      throw new Error(body.error?.message || body.error?.code || (typeof body.error === 'string' ? body.error : null) || `API error: ${res.status}`);
    }

    return res.json();
  }

  throw lastError ?? new Error("Request failed after retries");
}

// Auth
export async function login(email: string, password: string): Promise<AuthTokens> {
  const resp = await apiFetch<ApiResponse<AuthTokens>>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
  if (resp.data?.access_token) {
    setToken(resp.data.access_token);
  }
  return resp.data;
}

export async function getMe(): Promise<User> {
  const resp = await apiFetch<ApiResponse<User>>("/api/v1/auth/me");
  return resp.data;
}

export async function logout() {
  try {
    await apiFetch("/api/v1/auth/logout", { method: "POST" });
  } catch {
    // Best-effort — clear local state regardless
  }
  clearToken();
}

// Assets
export async function listAssets(page = 1, perPage = 50, assetType?: string) {
  let url = `/api/v1/assets?page=${page}&per_page=${perPage}`;
  if (assetType) url += `&asset_type=${assetType}`;
  return apiFetch<ApiResponse<Asset[]>>(url);
}

export async function createAsset(data: Partial<Asset>) {
  return apiFetch<ApiResponse<Asset>>("/api/v1/assets", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function getAsset(id: string) {
  return apiFetch<ApiResponse<Asset>>(`/api/v1/assets/${id}`);
}

export async function updateAsset(id: string, data: Partial<Asset>) {
  return apiFetch<ApiResponse<Asset>>(`/api/v1/assets/${id}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

export async function deleteAsset(id: string) {
  return apiFetch<ApiResponse<null>>(`/api/v1/assets/${id}`, { method: "DELETE" });
}

// Engagements
export async function listEngagements(page = 1, perPage = 50) {
  return apiFetch<ApiResponse<Engagement[]>>(`/api/v1/engagements?page=${page}&per_page=${perPage}`);
}

export async function createEngagement(data: Partial<Engagement>) {
  return apiFetch<ApiResponse<Engagement>>("/api/v1/engagements", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function getEngagement(id: string) {
  return apiFetch<ApiResponse<Engagement>>(`/api/v1/engagements/${id}`);
}

export async function updateEngagement(id: string, data: Partial<Engagement>) {
  return apiFetch<ApiResponse<Engagement>>(`/api/v1/engagements/${id}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

export async function deleteEngagement(id: string) {
  return apiFetch<ApiResponse<null>>(`/api/v1/engagements/${id}`, { method: "DELETE" });
}

export async function triggerRun(engagementId: string) {
  return apiFetch<ApiResponse<Run>>(`/api/v1/engagements/${engagementId}/runs`, {
    method: "POST",
    body: JSON.stringify({}),
  });
}

// Runs
export async function listRuns(page = 1, perPage = 50, status?: string) {
  let url = `/api/v1/runs?page=${page}&per_page=${perPage}`;
  if (status) url += `&status=${status}`;
  return apiFetch<ApiResponse<Run[]>>(url);
}

export async function getRun(id: string) {
  return apiFetch<ApiResponse<Run>>(`/api/v1/runs/${id}`);
}

export async function listRunSteps(runId: string) {
  return apiFetch<ApiResponse<RunStep[]>>(`/api/v1/runs/${runId}/steps`);
}

export async function killRun(id: string) {
  return apiFetch<ApiResponse<null>>(`/api/v1/runs/${id}/kill`, { method: "POST" });
}

// Findings
export async function listFindings(page = 1, perPage = 50, severity?: string, status?: string) {
  let url = `/api/v1/findings?page=${page}&per_page=${perPage}`;
  if (severity) url += `&severity=${severity}`;
  if (status) url += `&status=${status}`;
  return apiFetch<ApiResponse<Finding[]>>(url);
}

export async function getFinding(id: string) {
  return apiFetch<ApiResponse<Finding>>(`/api/v1/findings/${id}`);
}

export async function createFindingTicket(id: string, data: { connector_id: string }) {
  return apiFetch<ApiResponse<Finding>>(`/api/v1/findings/${id}/ticket`, {
    method: "POST",
    body: JSON.stringify(data),
  });
}

// Connectors
export async function listConnectorRegistry() {
  return apiFetch<ApiResponse<ConnectorRegistry[]>>("/api/v1/connectors/registry");
}

export async function listConnectorInstances(page = 1, perPage = 50) {
  return apiFetch<ApiResponse<ConnectorInstance[]>>(`/api/v1/connectors?page=${page}&per_page=${perPage}`);
}

export async function createConnectorInstance(data: Partial<ConnectorInstance>) {
  return apiFetch<ApiResponse<ConnectorInstance>>("/api/v1/connectors", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function testConnector(id: string) {
  return apiFetch<ApiResponse<{ status: string }>>(`/api/v1/connectors/${id}/test`, {
    method: "POST",
  });
}

export async function deleteConnectorInstance(id: string) {
  return apiFetch<ApiResponse<null>>(`/api/v1/connectors/${id}`, { method: "DELETE" });
}

// Approvals
export async function listApprovals(page = 1, perPage = 50) {
  return apiFetch<ApiResponse<Approval[]>>(`/api/v1/approvals?page=${page}&per_page=${perPage}`);
}

export async function approveRequest(id: string, notes?: string) {
  return apiFetch<ApiResponse<Approval>>(`/api/v1/approvals/${id}/approve`, {
    method: "POST",
    body: JSON.stringify({ notes }),
  });
}

export async function denyRequest(id: string, reason?: string) {
  return apiFetch<ApiResponse<Approval>>(`/api/v1/approvals/${id}/deny`, {
    method: "POST",
    body: JSON.stringify({ reason }),
  });
}

// Reports
export async function listReports(page = 1, perPage = 50) {
  return apiFetch<ApiResponse<Report[]>>(`/api/v1/reports?page=${page}&per_page=${perPage}`);
}

export async function generateReport(data: { type: string; format: string; title: string }) {
  return apiFetch<ApiResponse<Report>>("/api/v1/reports/generate", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export function getReportDownloadUrl(id: string): string {
  return `${API_BASE}/api/v1/reports/${id}/download`;
}

// Dashboard
export async function getDashboardSummary() {
  return apiFetch<ApiResponse<DashboardSummary>>("/api/v1/dashboard/summary");
}

export async function getDashboardActivity() {
  return apiFetch<ApiResponse<{ recent_runs: Run[]; recent_findings: Finding[] }>>("/api/v1/dashboard/activity");
}

export async function getDashboardHealth() {
  return apiFetch<ApiResponse<DashboardHealth>>("/api/v1/dashboard/health");
}

// Admin
export async function toggleKillSwitch(engaged: boolean) {
  return apiFetch<ApiResponse<{ engaged: boolean }>>("/api/v1/admin/system/kill-switch", {
    method: "POST",
    body: JSON.stringify({ engaged }),
  });
}

// Asset findings
export async function listAssetFindings(assetId: string, page = 1, perPage = 50) {
  return apiFetch<ApiResponse<Finding[]>>(`/api/v1/assets/${assetId}/findings?page=${page}&per_page=${perPage}`);
}

// Engagement runs
export async function listEngagementRuns(engagementId: string, page = 1, perPage = 50) {
  return apiFetch<ApiResponse<Run[]>>(`/api/v1/engagements/${engagementId}/runs?page=${page}&per_page=${perPage}`);
}

// Run receipt
export async function getRunReceipt(runId: string) {
  return apiFetch<ApiResponse<{ receipt_id: string; run_id: string }>>(`/api/v1/runs/${runId}/receipt`);
}

// Generic helper for compatibility
export const api = {
  get: <T = unknown>(path: string) => apiFetch<T>(path),
  post: <T = unknown>(path: string, data: unknown) =>
    apiFetch<T>(path, { method: "POST", body: JSON.stringify(data) }),
  put: <T = unknown>(path: string, data: unknown) =>
    apiFetch<T>(path, { method: "PUT", body: JSON.stringify(data) }),
  delete: <T = unknown>(path: string) => apiFetch<T>(path, { method: "DELETE" }),
};
