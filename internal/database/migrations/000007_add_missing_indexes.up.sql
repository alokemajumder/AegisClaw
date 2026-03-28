-- Add missing indexes for foreign key columns used in queries.
CREATE INDEX IF NOT EXISTS idx_engagements_created_by ON engagements(created_by);
CREATE INDEX IF NOT EXISTS idx_findings_run_step_id ON findings(run_step_id);
CREATE INDEX IF NOT EXISTS idx_findings_ticket_connector_id ON findings(ticket_connector_id);
CREATE INDEX IF NOT EXISTS idx_findings_retest_run_id ON findings(retest_run_id);
CREATE INDEX IF NOT EXISTS idx_coverage_entries_asset_id ON coverage_entries(asset_id);
CREATE INDEX IF NOT EXISTS idx_reports_generated_by ON reports(generated_by);
