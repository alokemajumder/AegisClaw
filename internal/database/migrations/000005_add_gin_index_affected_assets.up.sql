-- GIN index on findings.affected_assets for efficient ANY() queries.
CREATE INDEX IF NOT EXISTS idx_findings_affected_assets ON findings USING GIN (affected_assets);
