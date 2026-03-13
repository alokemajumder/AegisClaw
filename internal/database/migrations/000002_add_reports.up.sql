-- Reports table for generated security reports
CREATE TABLE reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    report_type TEXT NOT NULL CHECK (report_type IN ('executive', 'technical', 'coverage', 'compliance')),
    status TEXT NOT NULL DEFAULT 'generating' CHECK (status IN ('generating', 'completed', 'failed')),
    format TEXT NOT NULL DEFAULT 'markdown' CHECK (format IN ('markdown', 'json', 'pdf')),
    storage_path TEXT,
    generated_by UUID REFERENCES users(id),
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_reports_org_id ON reports(org_id);
CREATE INDEX idx_reports_type ON reports(report_type);
