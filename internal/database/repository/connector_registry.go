package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

type ConnectorRegistryRepo struct {
	q Querier
}

func NewConnectorRegistryRepo(q Querier) *ConnectorRegistryRepo {
	return &ConnectorRegistryRepo{q: q}
}

func (r *ConnectorRegistryRepo) List(ctx context.Context) ([]models.ConnectorRegistryEntry, error) {
	rows, err := r.q.Query(ctx,
		`SELECT connector_type, category, display_name, description, version, config_schema, capabilities, status, created_at
		 FROM connector_registry ORDER BY category, display_name`)
	if err != nil {
		return nil, fmt.Errorf("listing connector registry: %w", err)
	}
	defer rows.Close()

	var entries []models.ConnectorRegistryEntry
	for rows.Next() {
		var e models.ConnectorRegistryEntry
		if err := rows.Scan(&e.ConnectorType, &e.Category, &e.DisplayName, &e.Description, &e.Version, &e.ConfigSchema, &e.Capabilities, &e.Status, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning connector registry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *ConnectorRegistryRepo) GetByType(ctx context.Context, connectorType string) (*models.ConnectorRegistryEntry, error) {
	var e models.ConnectorRegistryEntry
	err := r.q.QueryRow(ctx,
		`SELECT connector_type, category, display_name, description, version, config_schema, capabilities, status, created_at
		 FROM connector_registry WHERE connector_type = $1`, connectorType,
	).Scan(&e.ConnectorType, &e.Category, &e.DisplayName, &e.Description, &e.Version, &e.ConfigSchema, &e.Capabilities, &e.Status, &e.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("connector type not found: %w", err)
		}
		return nil, fmt.Errorf("getting connector type: %w", err)
	}
	return &e, nil
}
