ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS fk_audit_log_org;

ALTER TABLE reports DROP CONSTRAINT IF EXISTS chk_reports_title_len;
ALTER TABLE findings DROP CONSTRAINT IF EXISTS chk_findings_title_len;
ALTER TABLE engagements DROP CONSTRAINT IF EXISTS chk_engagements_name_len;
ALTER TABLE connector_instances DROP CONSTRAINT IF EXISTS chk_connector_instances_name_len;
ALTER TABLE assets DROP CONSTRAINT IF EXISTS chk_assets_hostname_len;
ALTER TABLE assets DROP CONSTRAINT IF EXISTS chk_assets_name_len;
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_email_len;
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_name_len;
ALTER TABLE organizations DROP CONSTRAINT IF EXISTS chk_organizations_name_len;
