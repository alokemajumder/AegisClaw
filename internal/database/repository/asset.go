package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

type AssetRepo struct {
	q Querier
}

func NewAssetRepo(q Querier) *AssetRepo {
	return &AssetRepo{q: q}
}

func (r *AssetRepo) Create(ctx context.Context, a *models.Asset) error {
	a.ID = uuid.New()
	return r.q.QueryRow(ctx,
		`INSERT INTO assets (id, org_id, name, asset_type, metadata, owner, criticality, environment, business_service, tags)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING created_at, updated_at`,
		a.ID, a.OrgID, a.Name, a.AssetType, a.Metadata, a.Owner, a.Criticality, a.Environment, a.BusinessService, a.Tags,
	).Scan(&a.CreatedAt, &a.UpdatedAt)
}

func (r *AssetRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Asset, error) {
	var a models.Asset
	err := r.q.QueryRow(ctx,
		`SELECT id, org_id, name, asset_type, metadata, owner, criticality, environment, business_service, tags, created_at, updated_at
		 FROM assets WHERE id = $1`, id,
	).Scan(&a.ID, &a.OrgID, &a.Name, &a.AssetType, &a.Metadata, &a.Owner, &a.Criticality, &a.Environment, &a.BusinessService, &a.Tags, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("asset not found: %w", err)
		}
		return nil, fmt.Errorf("getting asset: %w", err)
	}
	return &a, nil
}

func (r *AssetRepo) ListByOrgID(ctx context.Context, orgID uuid.UUID, p models.PaginationParams, assetType string) ([]models.Asset, int, error) {
	var total int
	countSQL := `SELECT count(*) FROM assets WHERE org_id = $1`
	querySQL := `SELECT id, org_id, name, asset_type, metadata, owner, criticality, environment, business_service, tags, created_at, updated_at
		FROM assets WHERE org_id = $1`

	args := []any{orgID}
	argIdx := 2

	if assetType != "" {
		filter := fmt.Sprintf(` AND asset_type = $%d`, argIdx)
		countSQL += filter
		querySQL += filter
		args = append(args, assetType)
		argIdx++
	}

	if err := r.q.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting assets: %w", err)
	}

	querySQL += fmt.Sprintf(` ORDER BY name LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, p.Limit(), p.Offset())

	rows, err := r.q.Query(ctx, querySQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing assets: %w", err)
	}
	defer rows.Close()

	var assets []models.Asset
	for rows.Next() {
		var a models.Asset
		if err := rows.Scan(&a.ID, &a.OrgID, &a.Name, &a.AssetType, &a.Metadata, &a.Owner, &a.Criticality, &a.Environment, &a.BusinessService, &a.Tags, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning asset: %w", err)
		}
		assets = append(assets, a)
	}
	return assets, total, rows.Err()
}

func (r *AssetRepo) Update(ctx context.Context, a *models.Asset) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE assets SET name = $2, asset_type = $3, metadata = $4, owner = $5, criticality = $6,
		 environment = $7, business_service = $8, tags = $9, updated_at = now() WHERE id = $1`,
		a.ID, a.Name, a.AssetType, a.Metadata, a.Owner, a.Criticality, a.Environment, a.BusinessService, a.Tags,
	)
	if err != nil {
		return fmt.Errorf("updating asset: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("asset not found")
	}
	return nil
}

func (r *AssetRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.q.Exec(ctx, `DELETE FROM assets WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting asset: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("asset not found")
	}
	return nil
}

func (r *AssetRepo) CountByOrgID(ctx context.Context, orgID uuid.UUID) (int, error) {
	var count int
	err := r.q.QueryRow(ctx, `SELECT count(*) FROM assets WHERE org_id = $1`, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting assets: %w", err)
	}
	return count, nil
}
