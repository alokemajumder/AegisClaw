-- Mark all implemented connectors as available.
UPDATE connector_registry
SET status = 'available'
WHERE connector_type IN ('splunk', 'elastic', 'crowdstrike', 'jira', 'okta')
  AND status = 'beta';
