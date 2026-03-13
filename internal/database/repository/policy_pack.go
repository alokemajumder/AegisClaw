package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

type PolicyPackRepo struct {
	q Querier
}

func NewPolicyPackRepo(q Querier) *PolicyPackRepo {
	return &PolicyPackRepo{q: q}
}

func (r *PolicyPackRepo) Create(ctx context.Context, p *models.PolicyPack) error {
	p.ID = uuid.New()
	return r.q.QueryRow(ctx,
		`INSERT INTO policy_packs (id, org_id, name, description, is_default, rules, version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING created_at, updated_at`,
		p.ID, p.OrgID, p.Name, p.Description, p.IsDefault, p.Rules, p.Version,
	).Scan(&p.CreatedAt, &p.UpdatedAt)
}

func (r *PolicyPackRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.PolicyPack, error) {
	var p models.PolicyPack
	err := r.q.QueryRow(ctx,
		`SELECT id, org_id, name, description, is_default, rules, version, created_at, updated_at
		 FROM policy_packs WHERE id = $1`, id,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Description, &p.IsDefault, &p.Rules, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("policy pack not found: %w", err)
		}
		return nil, fmt.Errorf("getting policy pack: %w", err)
	}
	return &p, nil
}

func (r *PolicyPackRepo) GetDefaultByOrgID(ctx context.Context, orgID uuid.UUID) (*models.PolicyPack, error) {
	var p models.PolicyPack
	err := r.q.QueryRow(ctx,
		`SELECT id, org_id, name, description, is_default, rules, version, created_at, updated_at
		 FROM policy_packs WHERE org_id = $1 AND is_default = true LIMIT 1`, orgID,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Description, &p.IsDefault, &p.Rules, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("default policy pack not found: %w", err)
		}
		return nil, fmt.Errorf("getting default policy pack: %w", err)
	}
	return &p, nil
}

func (r *PolicyPackRepo) ListByOrgID(ctx context.Context, orgID uuid.UUID) ([]models.PolicyPack, error) {
	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, name, description, is_default, rules, version, created_at, updated_at
		 FROM policy_packs WHERE org_id = $1 ORDER BY name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing policy packs: %w", err)
	}
	defer rows.Close()

	var packs []models.PolicyPack
	for rows.Next() {
		var p models.PolicyPack
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Description, &p.IsDefault, &p.Rules, &p.Version, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning policy pack: %w", err)
		}
		packs = append(packs, p)
	}
	return packs, rows.Err()
}

func (r *PolicyPackRepo) Update(ctx context.Context, p *models.PolicyPack) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE policy_packs SET name = $2, description = $3, is_default = $4, rules = $5, version = version + 1, updated_at = now()
		 WHERE id = $1`,
		p.ID, p.Name, p.Description, p.IsDefault, p.Rules,
	)
	if err != nil {
		return fmt.Errorf("updating policy pack: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("policy pack not found")
	}
	return nil
}
