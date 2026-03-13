package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

type FindingRepo struct {
	q Querier
}

func NewFindingRepo(q Querier) *FindingRepo {
	return &FindingRepo{q: q}
}

func (r *FindingRepo) Create(ctx context.Context, f *models.Finding) error {
	f.ID = uuid.New()
	return r.q.QueryRow(ctx,
		`INSERT INTO findings (id, org_id, run_id, run_step_id, title, description, severity, confidence, status,
		 affected_assets, technique_ids, evidence_ids, remediation, cluster_id, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 RETURNING created_at, updated_at`,
		f.ID, f.OrgID, f.RunID, f.RunStepID, f.Title, f.Description, f.Severity, f.Confidence, f.Status,
		f.AffectedAssets, f.TechniqueIDs, f.EvidenceIDs, f.Remediation, f.ClusterID, f.Metadata,
	).Scan(&f.CreatedAt, &f.UpdatedAt)
}

func (r *FindingRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Finding, error) {
	var f models.Finding
	err := r.q.QueryRow(ctx,
		`SELECT id, org_id, run_id, run_step_id, title, description, severity, confidence, status,
		 affected_assets, technique_ids, evidence_ids, remediation, ticket_id, ticket_connector_id,
		 retest_run_id, cluster_id, metadata, created_at, updated_at
		 FROM findings WHERE id = $1`, id,
	).Scan(&f.ID, &f.OrgID, &f.RunID, &f.RunStepID, &f.Title, &f.Description, &f.Severity, &f.Confidence, &f.Status,
		&f.AffectedAssets, &f.TechniqueIDs, &f.EvidenceIDs, &f.Remediation, &f.TicketID, &f.TicketConnectorID,
		&f.RetestRunID, &f.ClusterID, &f.Metadata, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("finding not found: %w", err)
		}
		return nil, fmt.Errorf("getting finding: %w", err)
	}
	return &f, nil
}

func (r *FindingRepo) ListByOrgID(ctx context.Context, orgID uuid.UUID, p models.PaginationParams, severity, status string) ([]models.Finding, int, error) {
	var total int
	countSQL := `SELECT count(*) FROM findings WHERE org_id = $1`
	querySQL := `SELECT id, org_id, run_id, run_step_id, title, description, severity, confidence, status,
		affected_assets, technique_ids, evidence_ids, remediation, ticket_id, ticket_connector_id,
		retest_run_id, cluster_id, metadata, created_at, updated_at
		FROM findings WHERE org_id = $1`

	args := []any{orgID}
	argIdx := 2

	if severity != "" {
		filter := fmt.Sprintf(` AND severity = $%d`, argIdx)
		countSQL += filter
		querySQL += filter
		args = append(args, severity)
		argIdx++
	}
	if status != "" {
		filter := fmt.Sprintf(` AND status = $%d`, argIdx)
		countSQL += filter
		querySQL += filter
		args = append(args, status)
		argIdx++
	}

	if err := r.q.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting findings: %w", err)
	}

	querySQL += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, p.Limit(), p.Offset())

	rows, err := r.q.Query(ctx, querySQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing findings: %w", err)
	}
	defer rows.Close()
	return r.scanAll(rows, total)
}

func (r *FindingRepo) ListByRunID(ctx context.Context, runID uuid.UUID) ([]models.Finding, error) {
	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, run_id, run_step_id, title, description, severity, confidence, status,
		 affected_assets, technique_ids, evidence_ids, remediation, ticket_id, ticket_connector_id,
		 retest_run_id, cluster_id, metadata, created_at, updated_at
		 FROM findings WHERE run_id = $1 ORDER BY severity, created_at DESC`, runID)
	if err != nil {
		return nil, fmt.Errorf("listing findings by run: %w", err)
	}
	defer rows.Close()
	findings, _, err := r.scanAll(rows, 0)
	return findings, err
}

func (r *FindingRepo) ListByAssetID(ctx context.Context, assetID uuid.UUID, p models.PaginationParams) ([]models.Finding, int, error) {
	var total int
	if err := r.q.QueryRow(ctx,
		`SELECT count(*) FROM findings WHERE $1 = ANY(affected_assets)`, assetID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting asset findings: %w", err)
	}

	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, run_id, run_step_id, title, description, severity, confidence, status,
		 affected_assets, technique_ids, evidence_ids, remediation, ticket_id, ticket_connector_id,
		 retest_run_id, cluster_id, metadata, created_at, updated_at
		 FROM findings WHERE $1 = ANY(affected_assets) ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		assetID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("listing asset findings: %w", err)
	}
	defer rows.Close()
	return r.scanAll(rows, total)
}

func (r *FindingRepo) Update(ctx context.Context, f *models.Finding) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE findings SET title = $2, description = $3, severity = $4, confidence = $5, status = $6,
		 remediation = $7, metadata = $8, updated_at = now() WHERE id = $1`,
		f.ID, f.Title, f.Description, f.Severity, f.Confidence, f.Status, f.Remediation, f.Metadata,
	)
	if err != nil {
		return fmt.Errorf("updating finding: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("finding not found")
	}
	return nil
}

func (r *FindingRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status models.FindingStatus) error {
	_, err := r.q.Exec(ctx,
		`UPDATE findings SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("updating finding status: %w", err)
	}
	return nil
}

func (r *FindingRepo) SetTicket(ctx context.Context, id uuid.UUID, ticketID string, connectorID uuid.UUID) error {
	_, err := r.q.Exec(ctx,
		`UPDATE findings SET ticket_id = $2, ticket_connector_id = $3, status = 'ticketed', updated_at = now()
		 WHERE id = $1`,
		id, ticketID, connectorID)
	if err != nil {
		return fmt.Errorf("setting ticket: %w", err)
	}
	return nil
}

func (r *FindingRepo) FindByHash(ctx context.Context, orgID uuid.UUID, clusterID uuid.UUID) ([]models.Finding, error) {
	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, run_id, run_step_id, title, description, severity, confidence, status,
		 affected_assets, technique_ids, evidence_ids, remediation, ticket_id, ticket_connector_id,
		 retest_run_id, cluster_id, metadata, created_at, updated_at
		 FROM findings WHERE org_id = $1 AND cluster_id = $2 ORDER BY created_at DESC`,
		orgID, clusterID)
	if err != nil {
		return nil, fmt.Errorf("finding by hash: %w", err)
	}
	defer rows.Close()
	findings, _, err := r.scanAll(rows, 0)
	return findings, err
}

func (r *FindingRepo) CountByOrgID(ctx context.Context, orgID uuid.UUID) (total int, critical int, high int, err error) {
	err = r.q.QueryRow(ctx,
		`SELECT count(*),
		 count(*) FILTER (WHERE severity = 'critical'),
		 count(*) FILTER (WHERE severity = 'high')
		 FROM findings WHERE org_id = $1`, orgID,
	).Scan(&total, &critical, &high)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("counting findings: %w", err)
	}
	return
}

func (r *FindingRepo) scanAll(rows pgx.Rows, total int) ([]models.Finding, int, error) {
	var findings []models.Finding
	for rows.Next() {
		var f models.Finding
		if err := rows.Scan(&f.ID, &f.OrgID, &f.RunID, &f.RunStepID, &f.Title, &f.Description, &f.Severity, &f.Confidence, &f.Status,
			&f.AffectedAssets, &f.TechniqueIDs, &f.EvidenceIDs, &f.Remediation, &f.TicketID, &f.TicketConnectorID,
			&f.RetestRunID, &f.ClusterID, &f.Metadata, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning finding: %w", err)
		}
		findings = append(findings, f)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return findings, total, nil
}
