package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

type ApprovalRepo struct {
	q Querier
}

func NewApprovalRepo(q Querier) *ApprovalRepo {
	return &ApprovalRepo{q: q}
}

func (r *ApprovalRepo) Create(ctx context.Context, a *models.Approval) error {
	a.ID = uuid.New()
	return r.q.QueryRow(ctx,
		`INSERT INTO approvals (id, org_id, request_type, requested_by, target_entity_id, target_entity_type,
		 description, tier, status, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING created_at`,
		a.ID, a.OrgID, a.RequestType, a.RequestedBy, a.TargetEntityID, a.TargetEntityType,
		a.Description, a.Tier, a.Status, a.ExpiresAt,
	).Scan(&a.CreatedAt)
}

func (r *ApprovalRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Approval, error) {
	var a models.Approval
	err := r.q.QueryRow(ctx,
		`SELECT id, org_id, request_type, requested_by, target_entity_id, target_entity_type,
		 description, tier, status, decided_by, decision_rationale, expires_at, decided_at, created_at
		 FROM approvals WHERE id = $1`, id,
	).Scan(&a.ID, &a.OrgID, &a.RequestType, &a.RequestedBy, &a.TargetEntityID, &a.TargetEntityType,
		&a.Description, &a.Tier, &a.Status, &a.DecidedBy, &a.DecisionRationale, &a.ExpiresAt, &a.DecidedAt, &a.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("approval not found: %w", err)
		}
		return nil, fmt.Errorf("getting approval: %w", err)
	}
	return &a, nil
}

func (r *ApprovalRepo) ListByOrgID(ctx context.Context, orgID uuid.UUID, p models.PaginationParams) ([]models.Approval, int, error) {
	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM approvals WHERE org_id = $1`, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting approvals: %w", err)
	}

	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, request_type, requested_by, target_entity_id, target_entity_type,
		 description, tier, status, decided_by, decision_rationale, expires_at, decided_at, created_at
		 FROM approvals WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		orgID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("listing approvals: %w", err)
	}
	defer rows.Close()
	return r.scanAll(rows, total)
}

func (r *ApprovalRepo) ListPendingByOrgID(ctx context.Context, orgID uuid.UUID) ([]models.Approval, error) {
	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, request_type, requested_by, target_entity_id, target_entity_type,
		 description, tier, status, decided_by, decision_rationale, expires_at, decided_at, created_at
		 FROM approvals WHERE org_id = $1 AND status = 'pending' ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing pending approvals: %w", err)
	}
	defer rows.Close()
	approvals, _, err := r.scanAll(rows, 0)
	return approvals, err
}

func (r *ApprovalRepo) UpdateDecision(ctx context.Context, id uuid.UUID, status models.ApprovalStatus, decidedBy uuid.UUID, rationale string) error {
	now := time.Now().UTC()
	tag, err := r.q.Exec(ctx,
		`UPDATE approvals SET status = $2, decided_by = $3, decision_rationale = $4, decided_at = $5
		 WHERE id = $1 AND status = 'pending'`,
		id, status, decidedBy, rationale, now)
	if err != nil {
		return fmt.Errorf("updating approval decision: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("approval not found or already decided")
	}
	return nil
}

func (r *ApprovalRepo) scanAll(rows pgx.Rows, total int) ([]models.Approval, int, error) {
	var approvals []models.Approval
	for rows.Next() {
		var a models.Approval
		if err := rows.Scan(&a.ID, &a.OrgID, &a.RequestType, &a.RequestedBy, &a.TargetEntityID, &a.TargetEntityType,
			&a.Description, &a.Tier, &a.Status, &a.DecidedBy, &a.DecisionRationale, &a.ExpiresAt, &a.DecidedAt, &a.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning approval: %w", err)
		}
		approvals = append(approvals, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return approvals, total, nil
}
