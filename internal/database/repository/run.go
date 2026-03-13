package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

type RunRepo struct {
	q Querier
}

func NewRunRepo(q Querier) *RunRepo {
	return &RunRepo{q: q}
}

func (r *RunRepo) Create(ctx context.Context, run *models.Run) error {
	run.ID = uuid.New()
	return r.q.QueryRow(ctx,
		`INSERT INTO runs (id, engagement_id, org_id, status, tier, steps_total, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING created_at, updated_at`,
		run.ID, run.EngagementID, run.OrgID, run.Status, run.Tier, run.StepsTotal, run.Metadata,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
}

func (r *RunRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Run, error) {
	var run models.Run
	err := r.q.QueryRow(ctx,
		`SELECT id, engagement_id, org_id, status, tier, started_at, completed_at,
		 steps_total, steps_completed, steps_failed, receipt_id, metadata, created_at, updated_at
		 FROM runs WHERE id = $1`, id,
	).Scan(&run.ID, &run.EngagementID, &run.OrgID, &run.Status, &run.Tier, &run.StartedAt, &run.CompletedAt,
		&run.StepsTotal, &run.StepsCompleted, &run.StepsFailed, &run.ReceiptID, &run.Metadata, &run.CreatedAt, &run.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("run not found: %w", err)
		}
		return nil, fmt.Errorf("getting run: %w", err)
	}
	return &run, nil
}

func (r *RunRepo) ListByOrgID(ctx context.Context, orgID uuid.UUID, p models.PaginationParams, status string) ([]models.Run, int, error) {
	var total int
	countSQL := `SELECT count(*) FROM runs WHERE org_id = $1`
	querySQL := `SELECT id, engagement_id, org_id, status, tier, started_at, completed_at,
		steps_total, steps_completed, steps_failed, receipt_id, metadata, created_at, updated_at
		FROM runs WHERE org_id = $1`

	args := []any{orgID}
	argIdx := 2

	if status != "" {
		filter := fmt.Sprintf(` AND status = $%d`, argIdx)
		countSQL += filter
		querySQL += filter
		args = append(args, status)
		argIdx++
	}

	if err := r.q.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting runs: %w", err)
	}

	querySQL += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, p.Limit(), p.Offset())

	rows, err := r.q.Query(ctx, querySQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing runs: %w", err)
	}
	defer rows.Close()

	return r.scanAll(rows)
}

func (r *RunRepo) ListByEngagementID(ctx context.Context, engagementID uuid.UUID, p models.PaginationParams) ([]models.Run, int, error) {
	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM runs WHERE engagement_id = $1`, engagementID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting runs: %w", err)
	}

	rows, err := r.q.Query(ctx,
		`SELECT id, engagement_id, org_id, status, tier, started_at, completed_at,
		 steps_total, steps_completed, steps_failed, receipt_id, metadata, created_at, updated_at
		 FROM runs WHERE engagement_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		engagementID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("listing runs by engagement: %w", err)
	}
	defer rows.Close()
	return r.scanAll(rows)
}

func (r *RunRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status models.RunStatus) error {
	now := time.Now().UTC()
	var err error
	switch status {
	case models.RunRunning:
		_, err = r.q.Exec(ctx,
			`UPDATE runs SET status = $2, started_at = $3, updated_at = now() WHERE id = $1`,
			id, status, now)
	case models.RunCompleted, models.RunFailed, models.RunKilled, models.RunCancelled:
		_, err = r.q.Exec(ctx,
			`UPDATE runs SET status = $2, completed_at = $3, updated_at = now() WHERE id = $1`,
			id, status, now)
	default:
		_, err = r.q.Exec(ctx,
			`UPDATE runs SET status = $2, updated_at = now() WHERE id = $1`,
			id, status)
	}
	if err != nil {
		return fmt.Errorf("updating run status: %w", err)
	}
	return nil
}

func (r *RunRepo) IncrementSteps(ctx context.Context, id uuid.UUID, completed, failed int) error {
	_, err := r.q.Exec(ctx,
		`UPDATE runs SET steps_completed = steps_completed + $2, steps_failed = steps_failed + $3, updated_at = now()
		 WHERE id = $1`,
		id, completed, failed)
	if err != nil {
		return fmt.Errorf("incrementing steps: %w", err)
	}
	return nil
}

func (r *RunRepo) SetReceipt(ctx context.Context, id uuid.UUID, receiptID string) error {
	_, err := r.q.Exec(ctx,
		`UPDATE runs SET receipt_id = $2, updated_at = now() WHERE id = $1`,
		id, receiptID)
	if err != nil {
		return fmt.Errorf("setting receipt: %w", err)
	}
	return nil
}

func (r *RunRepo) SetStepsTotal(ctx context.Context, id uuid.UUID, total int) error {
	_, err := r.q.Exec(ctx,
		`UPDATE runs SET steps_total = $2, updated_at = now() WHERE id = $1`,
		id, total)
	if err != nil {
		return fmt.Errorf("setting steps total: %w", err)
	}
	return nil
}

func (r *RunRepo) ListRunning(ctx context.Context) ([]models.Run, error) {
	rows, err := r.q.Query(ctx,
		`SELECT id, engagement_id, org_id, status, tier, started_at, completed_at,
		 steps_total, steps_completed, steps_failed, receipt_id, metadata, created_at, updated_at
		 FROM runs WHERE status IN ('queued', 'running')`)
	if err != nil {
		return nil, fmt.Errorf("listing running runs: %w", err)
	}
	defer rows.Close()

	var runs []models.Run
	for rows.Next() {
		var run models.Run
		if err := rows.Scan(&run.ID, &run.EngagementID, &run.OrgID, &run.Status, &run.Tier, &run.StartedAt, &run.CompletedAt,
			&run.StepsTotal, &run.StepsCompleted, &run.StepsFailed, &run.ReceiptID, &run.Metadata, &run.CreatedAt, &run.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning run: %w", err)
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (r *RunRepo) scanAll(rows pgx.Rows) ([]models.Run, int, error) {
	var runs []models.Run
	for rows.Next() {
		var run models.Run
		if err := rows.Scan(&run.ID, &run.EngagementID, &run.OrgID, &run.Status, &run.Tier, &run.StartedAt, &run.CompletedAt,
			&run.StepsTotal, &run.StepsCompleted, &run.StepsFailed, &run.ReceiptID, &run.Metadata, &run.CreatedAt, &run.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	total := len(runs)
	return runs, total, nil
}
