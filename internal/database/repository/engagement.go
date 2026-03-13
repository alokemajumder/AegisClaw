package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

type EngagementRepo struct {
	q Querier
}

func NewEngagementRepo(q Querier) *EngagementRepo {
	return &EngagementRepo{q: q}
}

func (r *EngagementRepo) Create(ctx context.Context, e *models.Engagement) error {
	e.ID = uuid.New()
	return r.q.QueryRow(ctx,
		`INSERT INTO engagements (id, org_id, name, description, status, target_allowlist, target_exclusions,
		 allowed_tiers, allowed_techniques, schedule_cron, run_window_start, run_window_end, blackout_periods,
		 rate_limit, concurrency_cap, connector_ids, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		 RETURNING created_at, updated_at`,
		e.ID, e.OrgID, e.Name, e.Description, e.Status, e.TargetAllowlist, e.TargetExclusions,
		e.AllowedTiers, e.AllowedTechniques, e.ScheduleCron, e.RunWindowStart, e.RunWindowEnd, e.BlackoutPeriods,
		e.RateLimit, e.ConcurrencyCap, e.ConnectorIDs, e.CreatedBy,
	).Scan(&e.CreatedAt, &e.UpdatedAt)
}

func (r *EngagementRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Engagement, error) {
	var e models.Engagement
	err := r.q.QueryRow(ctx,
		`SELECT id, org_id, name, description, status, target_allowlist, target_exclusions,
		 allowed_tiers, allowed_techniques, schedule_cron, run_window_start, run_window_end, blackout_periods,
		 rate_limit, concurrency_cap, connector_ids, created_by, created_at, updated_at
		 FROM engagements WHERE id = $1`, id,
	).Scan(&e.ID, &e.OrgID, &e.Name, &e.Description, &e.Status, &e.TargetAllowlist, &e.TargetExclusions,
		&e.AllowedTiers, &e.AllowedTechniques, &e.ScheduleCron, &e.RunWindowStart, &e.RunWindowEnd, &e.BlackoutPeriods,
		&e.RateLimit, &e.ConcurrencyCap, &e.ConnectorIDs, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("engagement not found: %w", err)
		}
		return nil, fmt.Errorf("getting engagement: %w", err)
	}
	return &e, nil
}

func (r *EngagementRepo) ListByOrgID(ctx context.Context, orgID uuid.UUID, p models.PaginationParams) ([]models.Engagement, int, error) {
	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM engagements WHERE org_id = $1`, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting engagements: %w", err)
	}

	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, name, description, status, target_allowlist, target_exclusions,
		 allowed_tiers, allowed_techniques, schedule_cron, run_window_start, run_window_end, blackout_periods,
		 rate_limit, concurrency_cap, connector_ids, created_by, created_at, updated_at
		 FROM engagements WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		orgID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("listing engagements: %w", err)
	}
	defer rows.Close()

	var engagements []models.Engagement
	for rows.Next() {
		var e models.Engagement
		if err := rows.Scan(&e.ID, &e.OrgID, &e.Name, &e.Description, &e.Status, &e.TargetAllowlist, &e.TargetExclusions,
			&e.AllowedTiers, &e.AllowedTechniques, &e.ScheduleCron, &e.RunWindowStart, &e.RunWindowEnd, &e.BlackoutPeriods,
			&e.RateLimit, &e.ConcurrencyCap, &e.ConnectorIDs, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning engagement: %w", err)
		}
		engagements = append(engagements, e)
	}
	return engagements, total, rows.Err()
}

func (r *EngagementRepo) ListActive(ctx context.Context) ([]models.Engagement, error) {
	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, name, description, status, target_allowlist, target_exclusions,
		 allowed_tiers, allowed_techniques, schedule_cron, run_window_start, run_window_end, blackout_periods,
		 rate_limit, concurrency_cap, connector_ids, created_by, created_at, updated_at
		 FROM engagements WHERE status = 'active' AND schedule_cron IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("listing active engagements: %w", err)
	}
	defer rows.Close()

	var engagements []models.Engagement
	for rows.Next() {
		var e models.Engagement
		if err := rows.Scan(&e.ID, &e.OrgID, &e.Name, &e.Description, &e.Status, &e.TargetAllowlist, &e.TargetExclusions,
			&e.AllowedTiers, &e.AllowedTechniques, &e.ScheduleCron, &e.RunWindowStart, &e.RunWindowEnd, &e.BlackoutPeriods,
			&e.RateLimit, &e.ConcurrencyCap, &e.ConnectorIDs, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning engagement: %w", err)
		}
		engagements = append(engagements, e)
	}
	return engagements, rows.Err()
}

func (r *EngagementRepo) Update(ctx context.Context, e *models.Engagement) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE engagements SET name = $2, description = $3, target_allowlist = $4, target_exclusions = $5,
		 allowed_tiers = $6, allowed_techniques = $7, schedule_cron = $8, run_window_start = $9, run_window_end = $10,
		 blackout_periods = $11, rate_limit = $12, concurrency_cap = $13, connector_ids = $14, updated_at = now()
		 WHERE id = $1`,
		e.ID, e.Name, e.Description, e.TargetAllowlist, e.TargetExclusions,
		e.AllowedTiers, e.AllowedTechniques, e.ScheduleCron, e.RunWindowStart, e.RunWindowEnd,
		e.BlackoutPeriods, e.RateLimit, e.ConcurrencyCap, e.ConnectorIDs,
	)
	if err != nil {
		return fmt.Errorf("updating engagement: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("engagement not found")
	}
	return nil
}

func (r *EngagementRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.q.Exec(ctx, `DELETE FROM engagements WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting engagement: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("engagement not found")
	}
	return nil
}

func (r *EngagementRepo) CountByOrgID(ctx context.Context, orgID uuid.UUID) (total int, active int, err error) {
	err = r.q.QueryRow(ctx,
		`SELECT count(*), count(*) FILTER (WHERE status = 'active') FROM engagements WHERE org_id = $1`, orgID,
	).Scan(&total, &active)
	if err != nil {
		return 0, 0, fmt.Errorf("counting engagements: %w", err)
	}
	return
}

func (r *EngagementRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status models.EngagementStatus) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE engagements SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("updating engagement status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("engagement not found")
	}
	return nil
}
