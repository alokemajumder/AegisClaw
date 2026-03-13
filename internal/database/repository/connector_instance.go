package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

type ConnectorInstanceRepo struct {
	q Querier
}

func NewConnectorInstanceRepo(q Querier) *ConnectorInstanceRepo {
	return &ConnectorInstanceRepo{q: q}
}

func (r *ConnectorInstanceRepo) Create(ctx context.Context, ci *models.ConnectorInstance) error {
	ci.ID = uuid.New()
	return r.q.QueryRow(ctx,
		`INSERT INTO connector_instances (id, org_id, connector_type, category, name, description, enabled, config, secret_ref, auth_method, rate_limit_config, retry_config, field_mappings)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 RETURNING health_status, created_at, updated_at`,
		ci.ID, ci.OrgID, ci.ConnectorType, ci.Category, ci.Name, ci.Description, ci.Enabled, ci.Config, ci.SecretRef, ci.AuthMethod, ci.RateLimitConfig, ci.RetryConfig, ci.FieldMappings,
	).Scan(&ci.HealthStatus, &ci.CreatedAt, &ci.UpdatedAt)
}

func (r *ConnectorInstanceRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.ConnectorInstance, error) {
	var ci models.ConnectorInstance
	err := r.q.QueryRow(ctx,
		`SELECT id, org_id, connector_type, category, name, description, enabled, config, secret_ref, auth_method,
		 health_status, health_checked_at, rate_limit_config, retry_config, field_mappings, created_at, updated_at
		 FROM connector_instances WHERE id = $1`, id,
	).Scan(&ci.ID, &ci.OrgID, &ci.ConnectorType, &ci.Category, &ci.Name, &ci.Description, &ci.Enabled, &ci.Config, &ci.SecretRef, &ci.AuthMethod,
		&ci.HealthStatus, &ci.HealthCheckedAt, &ci.RateLimitConfig, &ci.RetryConfig, &ci.FieldMappings, &ci.CreatedAt, &ci.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("connector instance not found: %w", err)
		}
		return nil, fmt.Errorf("getting connector instance: %w", err)
	}
	return &ci, nil
}

func (r *ConnectorInstanceRepo) ListByOrgID(ctx context.Context, orgID uuid.UUID) ([]models.ConnectorInstance, error) {
	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, connector_type, category, name, description, enabled, config, secret_ref, auth_method,
		 health_status, health_checked_at, rate_limit_config, retry_config, field_mappings, created_at, updated_at
		 FROM connector_instances WHERE org_id = $1 ORDER BY category, name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing connector instances: %w", err)
	}
	defer rows.Close()
	return r.scanAll(rows)
}

func (r *ConnectorInstanceRepo) ListByCategory(ctx context.Context, orgID uuid.UUID, category string) ([]models.ConnectorInstance, error) {
	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, connector_type, category, name, description, enabled, config, secret_ref, auth_method,
		 health_status, health_checked_at, rate_limit_config, retry_config, field_mappings, created_at, updated_at
		 FROM connector_instances WHERE org_id = $1 AND category = $2 ORDER BY name`, orgID, category)
	if err != nil {
		return nil, fmt.Errorf("listing connector instances by category: %w", err)
	}
	defer rows.Close()
	return r.scanAll(rows)
}

func (r *ConnectorInstanceRepo) Update(ctx context.Context, ci *models.ConnectorInstance) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE connector_instances SET name = $2, description = $3, enabled = $4, config = $5, secret_ref = $6,
		 auth_method = $7, rate_limit_config = $8, retry_config = $9, field_mappings = $10, updated_at = now()
		 WHERE id = $1`,
		ci.ID, ci.Name, ci.Description, ci.Enabled, ci.Config, ci.SecretRef, ci.AuthMethod, ci.RateLimitConfig, ci.RetryConfig, ci.FieldMappings,
	)
	if err != nil {
		return fmt.Errorf("updating connector instance: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("connector instance not found")
	}
	return nil
}

func (r *ConnectorInstanceRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.q.Exec(ctx, `DELETE FROM connector_instances WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting connector instance: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("connector instance not found")
	}
	return nil
}

func (r *ConnectorInstanceRepo) UpdateHealthStatus(ctx context.Context, id uuid.UUID, status string) error {
	now := time.Now().UTC()
	_, err := r.q.Exec(ctx,
		`UPDATE connector_instances SET health_status = $2, health_checked_at = $3, updated_at = now() WHERE id = $1`,
		id, status, now,
	)
	if err != nil {
		return fmt.Errorf("updating health status: %w", err)
	}
	return nil
}

func (r *ConnectorInstanceRepo) scanAll(rows pgx.Rows) ([]models.ConnectorInstance, error) {
	var items []models.ConnectorInstance
	for rows.Next() {
		var ci models.ConnectorInstance
		if err := rows.Scan(&ci.ID, &ci.OrgID, &ci.ConnectorType, &ci.Category, &ci.Name, &ci.Description, &ci.Enabled, &ci.Config, &ci.SecretRef, &ci.AuthMethod,
			&ci.HealthStatus, &ci.HealthCheckedAt, &ci.RateLimitConfig, &ci.RetryConfig, &ci.FieldMappings, &ci.CreatedAt, &ci.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning connector instance: %w", err)
		}
		items = append(items, ci)
	}
	return items, rows.Err()
}
