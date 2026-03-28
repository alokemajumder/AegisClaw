package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

type CoverageRepo struct {
	q Querier
}

func NewCoverageRepo(q Querier) *CoverageRepo {
	return &CoverageRepo{q: q}
}

func (r *CoverageRepo) Upsert(ctx context.Context, e *models.CoverageEntry) error {
	return r.q.QueryRow(ctx,
		`INSERT INTO coverage_entries (id, org_id, technique_id, asset_id, telemetry_source, has_telemetry, has_detection, has_alert, last_validated_at, last_run_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (org_id, technique_id, asset_id) DO UPDATE SET
		 telemetry_source = EXCLUDED.telemetry_source,
		 has_telemetry = EXCLUDED.has_telemetry,
		 has_detection = EXCLUDED.has_detection,
		 has_alert = EXCLUDED.has_alert,
		 last_validated_at = EXCLUDED.last_validated_at,
		 last_run_id = EXCLUDED.last_run_id,
		 updated_at = now()
		 RETURNING id, created_at, updated_at`,
		uuid.New(), e.OrgID, e.TechniqueID, e.AssetID, e.TelemetrySource, e.HasTelemetry, e.HasDetection, e.HasAlert, e.LastValidatedAt, e.LastRunID,
	).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
}

func (r *CoverageRepo) ListByOrgID(ctx context.Context, orgID uuid.UUID) ([]models.CoverageEntry, error) {
	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, technique_id, asset_id, telemetry_source, has_telemetry, has_detection, has_alert,
		 last_validated_at, last_run_id, created_at, updated_at
		 FROM coverage_entries WHERE org_id = $1 ORDER BY technique_id LIMIT 10000`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing coverage: %w", err)
	}
	defer rows.Close()
	return r.scanAll(rows)
}

func (r *CoverageRepo) ListByOrgIDPaginated(ctx context.Context, orgID uuid.UUID, p models.PaginationParams) ([]models.CoverageEntry, int, error) {
	var total int
	err := r.q.QueryRow(ctx, `SELECT count(*) FROM coverage_entries WHERE org_id = $1`, orgID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting coverage: %w", err)
	}

	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, technique_id, asset_id, telemetry_source, has_telemetry, has_detection, has_alert,
		 last_validated_at, last_run_id, created_at, updated_at
		 FROM coverage_entries WHERE org_id = $1 ORDER BY technique_id LIMIT $2 OFFSET $3`, orgID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("listing coverage: %w", err)
	}
	defer rows.Close()
	items, err := r.scanAll(rows)
	return items, total, err
}

func (r *CoverageRepo) GetGaps(ctx context.Context, orgID uuid.UUID) ([]models.CoverageEntry, error) {
	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, technique_id, asset_id, telemetry_source, has_telemetry, has_detection, has_alert,
		 last_validated_at, last_run_id, created_at, updated_at
		 FROM coverage_entries WHERE org_id = $1 AND (has_telemetry = false OR has_detection = false OR has_alert = false)
		 ORDER BY technique_id LIMIT 10000`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing coverage gaps: %w", err)
	}
	defer rows.Close()
	return r.scanAll(rows)
}

func (r *CoverageRepo) GetGapsPaginated(ctx context.Context, orgID uuid.UUID, p models.PaginationParams) ([]models.CoverageEntry, int, error) {
	var total int
	err := r.q.QueryRow(ctx,
		`SELECT count(*) FROM coverage_entries WHERE org_id = $1 AND (has_telemetry = false OR has_detection = false OR has_alert = false)`,
		orgID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting coverage gaps: %w", err)
	}

	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, technique_id, asset_id, telemetry_source, has_telemetry, has_detection, has_alert,
		 last_validated_at, last_run_id, created_at, updated_at
		 FROM coverage_entries WHERE org_id = $1 AND (has_telemetry = false OR has_detection = false OR has_alert = false)
		 ORDER BY technique_id LIMIT $2 OFFSET $3`, orgID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("listing coverage gaps: %w", err)
	}
	defer rows.Close()
	items, err := r.scanAll(rows)
	return items, total, err
}

// CountByOrgID returns the total number of coverage entries and the number of gaps.
func (r *CoverageRepo) CountByOrgID(ctx context.Context, orgID uuid.UUID) (entries int, gaps int, err error) {
	err = r.q.QueryRow(ctx,
		`SELECT count(*),
		 count(*) FILTER (WHERE has_telemetry = false OR has_detection = false OR has_alert = false)
		 FROM coverage_entries WHERE org_id = $1`, orgID,
	).Scan(&entries, &gaps)
	if err != nil {
		return 0, 0, fmt.Errorf("counting coverage: %w", err)
	}
	return
}

func (r *CoverageRepo) scanAll(rows pgx.Rows) ([]models.CoverageEntry, error) {
	var entries []models.CoverageEntry
	for rows.Next() {
		var e models.CoverageEntry
		if err := rows.Scan(&e.ID, &e.OrgID, &e.TechniqueID, &e.AssetID, &e.TelemetrySource, &e.HasTelemetry, &e.HasDetection, &e.HasAlert,
			&e.LastValidatedAt, &e.LastRunID, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning coverage: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
