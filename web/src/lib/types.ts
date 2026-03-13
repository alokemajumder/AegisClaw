// TypeScript interfaces matching Go models

export interface ApiResponse<T> {
  data: T;
  error?: string;
  meta?: PaginationMeta;
}

export interface PaginationMeta {
  total: number;
  page: number;
  per_page: number;
}

export interface Asset {
  id: string;
  org_id: string;
  name: string;
  asset_type: string;
  description?: string;
  hostname?: string;
  ip_address?: string;
  os?: string;
  environment?: string;
  criticality?: string;
  owner?: string;
  tags: string[];
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface Engagement {
  id: string;
  org_id: string;
  name: string;
  description?: string;
  status: "draft" | "active" | "paused" | "completed" | "archived";
  target_allowlist: string[];
  target_exclusions: string[];
  allowed_tiers: number[];
  allowed_techniques: string[];
  schedule_cron?: string;
  run_window_start?: string;
  run_window_end?: string;
  blackout_periods?: unknown;
  rate_limit: number;
  concurrency_cap: number;
  connector_ids: string[];
  created_by?: string;
  created_at: string;
  updated_at: string;
}

export interface Run {
  id: string;
  org_id: string;
  engagement_id: string;
  status: "queued" | "running" | "paused" | "completed" | "failed" | "killed";
  tier: number;
  steps_completed: number;
  steps_total: number;
  started_at?: string;
  completed_at?: string;
  triggered_by?: string;
  receipt_hash?: string;
  receipt_url?: string;
  created_at: string;
  updated_at: string;
}

export interface RunStep {
  id: string;
  run_id: string;
  step_number: number;
  agent_type: string;
  technique_id?: string;
  status: "pending" | "running" | "completed" | "failed" | "skipped";
  inputs?: Record<string, unknown>;
  outputs?: Record<string, unknown>;
  error_message?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
}

export interface Finding {
  id: string;
  org_id: string;
  run_id?: string;
  asset_id?: string;
  title: string;
  description?: string;
  severity: "critical" | "high" | "medium" | "low" | "informational";
  confidence: "high" | "medium" | "low";
  status: string;
  technique_ids: string[];
  evidence_refs: string[];
  remediation?: string;
  ticket_id?: string;
  ticket_url?: string;
  cluster_id?: string;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface ConnectorRegistry {
  id: string;
  name: string;
  connector_type: string;
  category: string;
  description?: string;
  config_schema?: Record<string, unknown>;
  version: string;
  created_at: string;
}

export interface ConnectorInstance {
  id: string;
  org_id: string;
  registry_id?: string;
  connector_type: string;
  category: string;
  name: string;
  description?: string;
  config: Record<string, unknown>;
  enabled: boolean;
  health_status: string;
  last_health_check?: string;
  created_at: string;
  updated_at: string;
}

export interface Approval {
  id: string;
  org_id: string;
  run_id?: string;
  step_id?: string;
  request_type: string;
  description: string;
  status: "pending" | "approved" | "denied" | "expired";
  requested_by?: string;
  decided_by?: string;
  decided_at?: string;
  metadata?: Record<string, unknown>;
  created_at: string;
}

export interface Report {
  id: string;
  org_id: string;
  title: string;
  report_type: string;
  status: "generating" | "completed" | "failed";
  format: string;
  storage_path?: string;
  generated_by?: string;
  created_at: string;
  updated_at: string;
}

export interface AuditLogEntry {
  id: string;
  org_id: string;
  user_id?: string;
  action: string;
  resource_type: string;
  resource_id?: string;
  details?: Record<string, unknown>;
  ip_address?: string;
  created_at: string;
}

export interface DashboardSummary {
  total_assets: number;
  active_engagements: number;
  running_runs: number;
  total_findings: number;
  critical_findings: number;
  high_findings: number;
  medium_findings: number;
  low_findings: number;
  coverage_entries: number;
  coverage_gaps: number;
  kill_switch_engaged: boolean;
}

export interface DashboardHealth {
  database: string;
  nats: string;
  kill_switch_engaged: boolean;
}

export interface User {
  id: string;
  org_id: string;
  email: string;
  name: string;
  role: "admin" | "operator" | "viewer" | "approver";
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface AuthTokens {
  access_token: string;
  refresh_token?: string;
  expires_at: string;
}
