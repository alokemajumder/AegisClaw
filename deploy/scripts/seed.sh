#!/bin/bash
set -euo pipefail

DB_HOST="${AEGISCLAW_DATABASE_HOST:-localhost}"
DB_PORT="${AEGISCLAW_DATABASE_PORT:-5432}"
DB_USER="${AEGISCLAW_DATABASE_USER:-aegisclaw}"
DB_NAME="${AEGISCLAW_DATABASE_NAME:-aegisclaw}"

export PGPASSWORD="${AEGISCLAW_DATABASE_PASSWORD:-aegisclaw}"

echo "Seeding AegisClaw database..."

psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" <<'EOF'
-- Seed default organization
INSERT INTO organizations (id, name, settings) VALUES
('00000000-0000-0000-0000-000000000001', 'Default Organization', '{}')
ON CONFLICT DO NOTHING;

-- Seed admin user (password: admin — development only!)
INSERT INTO users (id, org_id, email, name, role, password_hash) VALUES
('00000000-0000-0000-0000-000000000010', '00000000-0000-0000-0000-000000000001', 'admin@aegisclaw.local', 'Admin User', 'admin', '$2a$10$placeholder')
ON CONFLICT DO NOTHING;

-- Seed default policy pack
INSERT INTO policy_packs (id, org_id, name, description, is_default, rules) VALUES
('00000000-0000-0000-0000-000000000100', '00000000-0000-0000-0000-000000000001', 'Default', 'Safe defaults for autonomous validation', true, '{"tiers":{"0":{"autonomous":true},"1":{"autonomous":true,"cleanup_required":true},"2":{"approval_required":true},"3":{"blocked":true}}}')
ON CONFLICT DO NOTHING;

-- Seed sample assets
INSERT INTO assets (org_id, name, asset_type, criticality, environment, tags) VALUES
('00000000-0000-0000-0000-000000000001', 'prod-dc-01.corp.local', 'server', 'critical', 'production', '{"domain-controller","windows"}'),
('00000000-0000-0000-0000-000000000001', 'ws-dev-042.corp.local', 'endpoint', 'low', 'development', '{"workstation","windows"}'),
('00000000-0000-0000-0000-000000000001', 'api.corp.com', 'application', 'high', 'production', '{"web-app","public-facing"}'),
('00000000-0000-0000-0000-000000000001', 'admin@corp.com', 'identity', 'critical', 'production', '{"admin","privileged"}'),
('00000000-0000-0000-0000-000000000001', 'aws-prod-account', 'cloud_account', 'high', 'production', '{"aws","production"}')
ON CONFLICT DO NOTHING;

EOF

echo "Database seeded successfully."
