-- Add maximum length constraints on key TEXT columns to prevent unbounded input.
-- Using CHECK constraints rather than altering type to VARCHAR to avoid table rewrites.

ALTER TABLE organizations ADD CONSTRAINT chk_organizations_name_len CHECK (length(name) <= 255);

ALTER TABLE users ADD CONSTRAINT chk_users_name_len CHECK (length(name) <= 255);
ALTER TABLE users ADD CONSTRAINT chk_users_email_len CHECK (length(email) <= 320);

ALTER TABLE assets ADD CONSTRAINT chk_assets_name_len CHECK (length(name) <= 255);
ALTER TABLE assets ADD CONSTRAINT chk_assets_hostname_len CHECK (hostname IS NULL OR length(hostname) <= 255);

ALTER TABLE connector_instances ADD CONSTRAINT chk_connector_instances_name_len CHECK (length(name) <= 255);

ALTER TABLE engagements ADD CONSTRAINT chk_engagements_name_len CHECK (length(name) <= 255);

ALTER TABLE findings ADD CONSTRAINT chk_findings_title_len CHECK (length(title) <= 500);

ALTER TABLE reports ADD CONSTRAINT chk_reports_title_len CHECK (length(title) <= 255);

-- Add foreign key on audit_log.org_id for referential integrity.
-- Use NO ACTION on delete since audit records should be preserved.
ALTER TABLE audit_log ADD CONSTRAINT fk_audit_log_org
    FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE NO ACTION;
