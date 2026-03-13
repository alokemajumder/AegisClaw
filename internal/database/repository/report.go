package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

type ReportRepo struct {
	q Querier
}

func NewReportRepo(q Querier) *ReportRepo {
	return &ReportRepo{q: q}
}

func (r *ReportRepo) Create(ctx context.Context, rpt *models.Report) error {
	rpt.ID = uuid.New()
	return r.q.QueryRow(ctx,
		`INSERT INTO reports (id, org_id, title, report_type, status, format, storage_path, generated_by, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING created_at, updated_at`,
		rpt.ID, rpt.OrgID, rpt.Title, rpt.ReportType, rpt.Status, rpt.Format, rpt.StoragePath, rpt.GeneratedBy, rpt.Metadata,
	).Scan(&rpt.CreatedAt, &rpt.UpdatedAt)
}

func (r *ReportRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Report, error) {
	var rpt models.Report
	err := r.q.QueryRow(ctx,
		`SELECT id, org_id, title, report_type, status, format, storage_path, generated_by, metadata, created_at, updated_at
		 FROM reports WHERE id = $1`, id,
	).Scan(&rpt.ID, &rpt.OrgID, &rpt.Title, &rpt.ReportType, &rpt.Status, &rpt.Format, &rpt.StoragePath, &rpt.GeneratedBy, &rpt.Metadata, &rpt.CreatedAt, &rpt.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("report not found: %w", err)
		}
		return nil, fmt.Errorf("getting report: %w", err)
	}
	return &rpt, nil
}

func (r *ReportRepo) ListByOrgID(ctx context.Context, orgID uuid.UUID, p models.PaginationParams) ([]models.Report, int, error) {
	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM reports WHERE org_id = $1`, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting reports: %w", err)
	}

	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, title, report_type, status, format, storage_path, generated_by, metadata, created_at, updated_at
		 FROM reports WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		orgID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("listing reports: %w", err)
	}
	defer rows.Close()

	var reports []models.Report
	for rows.Next() {
		var rpt models.Report
		if err := rows.Scan(&rpt.ID, &rpt.OrgID, &rpt.Title, &rpt.ReportType, &rpt.Status, &rpt.Format, &rpt.StoragePath, &rpt.GeneratedBy, &rpt.Metadata, &rpt.CreatedAt, &rpt.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning report: %w", err)
		}
		reports = append(reports, rpt)
	}
	return reports, total, rows.Err()
}

func (r *ReportRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string, storagePath string) error {
	_, err := r.q.Exec(ctx,
		`UPDATE reports SET status = $2, storage_path = $3, updated_at = now() WHERE id = $1`,
		id, status, storagePath)
	if err != nil {
		return fmt.Errorf("updating report status: %w", err)
	}
	return nil
}
