-- Revert connector status back to beta.
UPDATE connector_registry
SET status = 'beta', updated_at = now()
WHERE connector_type IN ('splunk', 'elastic', 'crowdstrike', 'jira', 'okta');
