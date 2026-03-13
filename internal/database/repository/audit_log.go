package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

type AuditLogRepo struct {
	q Querier
}

func NewAuditLogRepo(q Querier) *AuditLogRepo {
	return &AuditLogRepo{q: q}
}

func (r *AuditLogRepo) Create(ctx context.Context, entry *models.AuditLog) error {
	return r.q.QueryRow(ctx,
		`INSERT INTO audit_log (org_id, actor_type, actor_id, action, resource_type, resource_id, details, ip_address)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, created_at`,
		entry.OrgID, entry.ActorType, entry.ActorID, entry.Action, entry.ResourceType, entry.ResourceID, entry.Details, entry.IPAddress,
	).Scan(&entry.ID, &entry.CreatedAt)
}

func (r *AuditLogRepo) ListByOrgID(ctx context.Context, orgID uuid.UUID, p models.PaginationParams, action, resourceType string) ([]models.AuditLog, int, error) {
	var total int
	countSQL := `SELECT count(*) FROM audit_log WHERE org_id = $1`
	querySQL := `SELECT id, org_id, actor_type, actor_id, action, resource_type, resource_id, details, ip_address, created_at
		FROM audit_log WHERE org_id = $1`

	args := []any{orgID}
	argIdx := 2

	if action != "" {
		filter := fmt.Sprintf(` AND action = $%d`, argIdx)
		countSQL += filter
		querySQL += filter
		args = append(args, action)
		argIdx++
	}
	if resourceType != "" {
		filter := fmt.Sprintf(` AND resource_type = $%d`, argIdx)
		countSQL += filter
		querySQL += filter
		args = append(args, resourceType)
		argIdx++
	}

	if err := r.q.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting audit logs: %w", err)
	}

	querySQL += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, p.Limit(), p.Offset())

	rows, err := r.q.Query(ctx, querySQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing audit logs: %w", err)
	}
	defer rows.Close()

	var logs []models.AuditLog
	for rows.Next() {
		var l models.AuditLog
		if err := rows.Scan(&l.ID, &l.OrgID, &l.ActorType, &l.ActorID, &l.Action, &l.ResourceType, &l.ResourceID, &l.Details, &l.IPAddress, &l.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning audit log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

// GetLastKillSwitchState returns true if the most recent kill switch event was an engagement.
// Returns false if no kill switch event exists (default: disengaged).
func (r *AuditLogRepo) GetLastKillSwitchState(ctx context.Context) (bool, error) {
	var action string
	err := r.q.QueryRow(ctx,
		`SELECT action FROM audit_log
		 WHERE action IN ('kill_switch_engaged', 'kill_switch_disengaged')
		 ORDER BY created_at DESC LIMIT 1`,
	).Scan(&action)
	if err != nil {
		// No kill switch events yet — default to disengaged
		return false, nil
	}
	return action == "kill_switch_engaged", nil
}
