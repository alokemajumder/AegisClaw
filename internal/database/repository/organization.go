package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

type OrganizationRepo struct {
	q Querier
}

func NewOrganizationRepo(q Querier) *OrganizationRepo {
	return &OrganizationRepo{q: q}
}

func (r *OrganizationRepo) Create(ctx context.Context, org *models.Organization) error {
	org.ID = uuid.New()
	return r.q.QueryRow(ctx,
		`INSERT INTO organizations (id, name, settings) VALUES ($1, $2, $3)
		 RETURNING created_at, updated_at`,
		org.ID, org.Name, org.Settings,
	).Scan(&org.CreatedAt, &org.UpdatedAt)
}

func (r *OrganizationRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Organization, error) {
	var o models.Organization
	err := r.q.QueryRow(ctx,
		`SELECT id, name, settings, created_at, updated_at FROM organizations WHERE id = $1`, id,
	).Scan(&o.ID, &o.Name, &o.Settings, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("organization not found: %w", err)
		}
		return nil, fmt.Errorf("getting organization: %w", err)
	}
	return &o, nil
}

func (r *OrganizationRepo) Update(ctx context.Context, org *models.Organization) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE organizations SET name = $2, settings = $3, updated_at = now() WHERE id = $1`,
		org.ID, org.Name, org.Settings,
	)
	if err != nil {
		return fmt.Errorf("updating organization: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("organization not found")
	}
	return nil
}

func (r *OrganizationRepo) List(ctx context.Context) ([]models.Organization, error) {
	rows, err := r.q.Query(ctx,
		`SELECT id, name, settings, created_at, updated_at FROM organizations ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listing organizations: %w", err)
	}
	defer rows.Close()

	var orgs []models.Organization
	for rows.Next() {
		var o models.Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.Settings, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning organization: %w", err)
		}
		orgs = append(orgs, o)
	}
	return orgs, rows.Err()
}
