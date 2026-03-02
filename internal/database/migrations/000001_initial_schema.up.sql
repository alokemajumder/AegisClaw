-- AegisClaw Initial Schema
-- All tables for the Autonomous Security Validation Platform

-- Organizations (single-tenant but schema supports multi-org)
CREATE TABLE organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    settings JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Users + RBAC
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    password_hash TEXT,
    role TEXT NOT NULL CHECK (role IN ('admin','operator','viewer','approver')),
    sso_subject TEXT,
    settings JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_users_org_id ON users(org_id);
CREATE INDEX idx_users_email ON users(email);

-- Assets (endpoints, servers, apps, identities, cloud accounts, k8s clusters)
CREATE TABLE assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    asset_type TEXT NOT NULL CHECK (asset_type IN ('endpoint','server','application','identity','cloud_account','k8s_cluster')),
    metadata JSONB NOT NULL DEFAULT '{}',
    owner TEXT,
    criticality TEXT CHECK (criticality IN ('critical','high','medium','low')),
    environment TEXT CHECK (environment IN ('production','staging','lab','development')),
    business_service TEXT,
    tags TEXT[] DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_assets_org_id ON assets(org_id);
CREATE INDEX idx_assets_type ON assets(asset_type);
CREATE INDEX idx_assets_criticality ON assets(criticality);

-- Connector type registry (available connector types with config schemas)
CREATE TABLE connector_registry (
    connector_type TEXT PRIMARY KEY,
    category TEXT NOT NULL CHECK (category IN ('siem','edr','itsm','identity','notification','cloud')),
    display_name TEXT NOT NULL,
    description TEXT,
    version TEXT NOT NULL,
    config_schema JSONB NOT NULL DEFAULT '{}',
    capabilities TEXT[] NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'available' CHECK (status IN ('available','beta','deprecated')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Connector instances (configured integrations — settings-driven)
CREATE TABLE connector_instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    connector_type TEXT NOT NULL REFERENCES connector_registry(connector_type),
    category TEXT NOT NULL CHECK (category IN ('siem','edr','itsm','identity','notification','cloud')),
    name TEXT NOT NULL,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT true,
    config JSONB NOT NULL DEFAULT '{}',
    secret_ref TEXT,
    auth_method TEXT NOT NULL CHECK (auth_method IN ('api_key','oauth2','service_principal','certificate')),
    health_status TEXT NOT NULL DEFAULT 'unknown' CHECK (health_status IN ('healthy','degraded','unhealthy','unknown')),
    health_checked_at TIMESTAMPTZ,
    rate_limit_config JSONB NOT NULL DEFAULT '{"requests_per_second": 10, "burst": 20}',
    retry_config JSONB NOT NULL DEFAULT '{"max_retries": 3, "backoff_ms": 1000}',
    field_mappings JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(org_id, connector_type, name)
);
CREATE INDEX idx_connector_instances_org ON connector_instances(org_id);
CREATE INDEX idx_connector_instances_type ON connector_instances(connector_type);
CREATE INDEX idx_connector_instances_category ON connector_instances(category);

-- Engagements (validation programs / campaigns)
CREATE TABLE engagements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','paused','completed','archived')),
    target_allowlist UUID[] NOT NULL DEFAULT '{}',
    target_exclusions UUID[] NOT NULL DEFAULT '{}',
    allowed_tiers INT[] NOT NULL DEFAULT '{0,1}',
    allowed_techniques TEXT[] DEFAULT '{}',
    schedule_cron TEXT,
    run_window_start TIME,
    run_window_end TIME,
    blackout_periods JSONB DEFAULT '[]',
    rate_limit INT DEFAULT 10,
    concurrency_cap INT DEFAULT 5,
    connector_ids UUID[] DEFAULT '{}',
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_engagements_org_id ON engagements(org_id);
CREATE INDEX idx_engagements_status ON engagements(status);

-- Runs (individual execution instances)
CREATE TABLE runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    engagement_id UUID NOT NULL REFERENCES engagements(id) ON DELETE CASCADE,
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','paused','completed','failed','cancelled','killed')),
    tier INT NOT NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    steps_total INT NOT NULL DEFAULT 0,
    steps_completed INT NOT NULL DEFAULT 0,
    steps_failed INT NOT NULL DEFAULT 0,
    receipt_id TEXT,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_runs_engagement ON runs(engagement_id);
CREATE INDEX idx_runs_org_id ON runs(org_id);
CREATE INDEX idx_runs_status ON runs(status);

-- Run steps (individual actions within a run)
CREATE TABLE run_steps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    step_number INT NOT NULL,
    agent_type TEXT NOT NULL,
    action TEXT NOT NULL,
    tier INT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed','skipped','blocked')),
    inputs JSONB NOT NULL DEFAULT '{}',
    outputs JSONB NOT NULL DEFAULT '{}',
    evidence_ids TEXT[] DEFAULT '{}',
    error_message TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    cleanup_verified BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_run_steps_run ON run_steps(run_id);
CREATE INDEX idx_run_steps_status ON run_steps(status);

-- Findings
CREATE TABLE findings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    run_id UUID REFERENCES runs(id),
    run_step_id UUID REFERENCES run_steps(id),
    title TEXT NOT NULL,
    description TEXT,
    severity TEXT NOT NULL CHECK (severity IN ('critical','high','medium','low','informational')),
    confidence TEXT NOT NULL CHECK (confidence IN ('confirmed','high','medium','low')),
    status TEXT NOT NULL DEFAULT 'observed' CHECK (status IN ('observed','needs_review','confirmed','ticketed','fixed','retested','closed','accepted_risk')),
    affected_assets UUID[] DEFAULT '{}',
    technique_ids TEXT[] DEFAULT '{}',
    evidence_ids TEXT[] DEFAULT '{}',
    remediation TEXT,
    ticket_id TEXT,
    ticket_connector_id UUID REFERENCES connector_instances(id),
    retest_run_id UUID REFERENCES runs(id),
    cluster_id UUID,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_findings_org_id ON findings(org_id);
CREATE INDEX idx_findings_run ON findings(run_id);
CREATE INDEX idx_findings_severity ON findings(severity);
CREATE INDEX idx_findings_status ON findings(status);
CREATE INDEX idx_findings_cluster ON findings(cluster_id);

-- Approvals
CREATE TABLE approvals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    request_type TEXT NOT NULL CHECK (request_type IN ('tier2_action','policy_change','connector_change')),
    requested_by TEXT NOT NULL,
    target_entity_id UUID,
    target_entity_type TEXT,
    description TEXT NOT NULL,
    tier INT,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','denied','expired')),
    decided_by UUID REFERENCES users(id),
    decision_rationale TEXT,
    expires_at TIMESTAMPTZ,
    decided_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_approvals_org_id ON approvals(org_id);
CREATE INDEX idx_approvals_status ON approvals(status);

-- Audit log (immutable, append-only)
CREATE TABLE audit_log (
    id BIGSERIAL PRIMARY KEY,
    org_id UUID NOT NULL,
    actor_type TEXT NOT NULL CHECK (actor_type IN ('user','agent','system')),
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT,
    details JSONB NOT NULL DEFAULT '{}',
    ip_address INET,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_log_org_id ON audit_log(org_id);
CREATE INDEX idx_audit_log_actor ON audit_log(actor_type, actor_id);
CREATE INDEX idx_audit_log_resource ON audit_log(resource_type, resource_id);
CREATE INDEX idx_audit_log_created_at ON audit_log(created_at);

-- Revoke UPDATE and DELETE on audit_log for safety
-- (enforced at application level; DB-level REVOKE requires specific role setup)

-- Coverage matrix entries
CREATE TABLE coverage_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    technique_id TEXT NOT NULL,
    asset_id UUID REFERENCES assets(id),
    telemetry_source TEXT,
    has_telemetry BOOLEAN DEFAULT false,
    has_detection BOOLEAN DEFAULT false,
    has_alert BOOLEAN DEFAULT false,
    last_validated_at TIMESTAMPTZ,
    last_run_id UUID REFERENCES runs(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(org_id, technique_id, asset_id)
);
CREATE INDEX idx_coverage_org_technique ON coverage_entries(org_id, technique_id);

-- Policy packs
CREATE TABLE policy_packs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    is_default BOOLEAN DEFAULT false,
    rules JSONB NOT NULL DEFAULT '{}',
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_policy_packs_org ON policy_packs(org_id);

-- Seed the connector registry with available types
INSERT INTO connector_registry (connector_type, category, display_name, description, version, config_schema, capabilities, status) VALUES
('sentinel', 'siem', 'Microsoft Sentinel', 'Azure Sentinel SIEM integration for log analytics and security event queries', '1.0.0',
 '{"type":"object","properties":{"workspace_id":{"type":"string","description":"Log Analytics Workspace ID"},"tenant_id":{"type":"string","description":"Azure AD Tenant ID"},"subscription_id":{"type":"string","description":"Azure Subscription ID"},"resource_group":{"type":"string","description":"Resource Group name"}},"required":["workspace_id","tenant_id"]}',
 '{"query_events","deep_link"}', 'available'),

('defender', 'edr', 'Microsoft Defender for Endpoint', 'Defender for Endpoint EDR integration for security alerts and device management', '1.0.0',
 '{"type":"object","properties":{"tenant_id":{"type":"string","description":"Azure AD Tenant ID"},"api_url":{"type":"string","description":"API base URL","default":"https://api.securitycenter.microsoft.com"}},"required":["tenant_id"]}',
 '{"query_events","fetch_assets","deep_link"}', 'available'),

('entraid', 'identity', 'Microsoft Entra ID', 'Azure Active Directory / Entra ID integration for SSO and identity asset sync', '1.0.0',
 '{"type":"object","properties":{"tenant_id":{"type":"string","description":"Azure AD Tenant ID"},"authority":{"type":"string","description":"Authority URL","default":"https://login.microsoftonline.com"}},"required":["tenant_id"]}',
 '{"fetch_assets","query_events"}', 'available'),

('servicenow', 'itsm', 'ServiceNow', 'ServiceNow ITSM integration for incident and change management', '1.0.0',
 '{"type":"object","properties":{"instance_url":{"type":"string","description":"ServiceNow instance URL (e.g., https://company.service-now.com)"},"api_version":{"type":"string","description":"API version","default":"now"}},"required":["instance_url"]}',
 '{"create_ticket","update_ticket","deep_link"}', 'available'),

('teams', 'notification', 'Microsoft Teams', 'Microsoft Teams webhook integration for notifications and alerts', '1.0.0',
 '{"type":"object","properties":{"webhook_url":{"type":"string","description":"Teams incoming webhook URL"},"channel_name":{"type":"string","description":"Target channel name for display"}},"required":["webhook_url"]}',
 '{"send_notification"}', 'available'),

('slack', 'notification', 'Slack', 'Slack integration for notifications and alerts via webhooks or bot tokens', '1.0.0',
 '{"type":"object","properties":{"webhook_url":{"type":"string","description":"Slack incoming webhook URL"},"channel":{"type":"string","description":"Default channel (e.g., #security-alerts)"},"bot_token":{"type":"string","description":"Optional bot token for richer integration"}},"required":["webhook_url"]}',
 '{"send_notification"}', 'available'),

('splunk', 'siem', 'Splunk Enterprise', 'Splunk Enterprise/Cloud SIEM integration', '1.0.0',
 '{"type":"object","properties":{"base_url":{"type":"string","description":"Splunk base URL"},"port":{"type":"integer","description":"Management port","default":8089}},"required":["base_url"]}',
 '{"query_events","deep_link"}', 'beta'),

('elastic', 'siem', 'Elastic Security', 'Elastic Security SIEM integration', '1.0.0',
 '{"type":"object","properties":{"base_url":{"type":"string","description":"Elasticsearch URL"},"index_pattern":{"type":"string","description":"Security index pattern","default":".siem-signals-*"}},"required":["base_url"]}',
 '{"query_events","deep_link"}', 'beta'),

('crowdstrike', 'edr', 'CrowdStrike Falcon', 'CrowdStrike Falcon EDR integration', '1.0.0',
 '{"type":"object","properties":{"base_url":{"type":"string","description":"CrowdStrike API base URL","default":"https://api.crowdstrike.com"},"client_id":{"type":"string","description":"OAuth2 Client ID"}},"required":["base_url","client_id"]}',
 '{"query_events","fetch_assets","deep_link"}', 'beta'),

('jira', 'itsm', 'Jira Service Management', 'Atlassian Jira Service Management ITSM integration', '1.0.0',
 '{"type":"object","properties":{"base_url":{"type":"string","description":"Jira instance URL"},"project_key":{"type":"string","description":"Default project key"}},"required":["base_url","project_key"]}',
 '{"create_ticket","update_ticket","deep_link"}', 'beta'),

('okta', 'identity', 'Okta', 'Okta identity platform integration for SSO and identity management', '1.0.0',
 '{"type":"object","properties":{"org_url":{"type":"string","description":"Okta organization URL (e.g., https://company.okta.com)"}},"required":["org_url"]}',
 '{"fetch_assets","query_events"}', 'beta');
