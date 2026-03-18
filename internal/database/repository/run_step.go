package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

type RunStepRepo struct {
	q Querier
}

func NewRunStepRepo(q Querier) *RunStepRepo {
	return &RunStepRepo{q: q}
}

func (r *RunStepRepo) Create(ctx context.Context, s *models.RunStep) error {
	s.ID = uuid.New()
	return r.q.QueryRow(ctx,
		`INSERT INTO run_steps (id, run_id, step_number, agent_type, action, tier, status, inputs)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING created_at`,
		s.ID, s.RunID, s.StepNumber, s.AgentType, s.Action, s.Tier, s.Status, s.Inputs,
	).Scan(&s.CreatedAt)
}

func (r *RunStepRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.RunStep, error) {
	var s models.RunStep
	err := r.q.QueryRow(ctx,
		`SELECT id, run_id, step_number, agent_type, action, tier, status, inputs, outputs,
		 evidence_ids, error_message, started_at, completed_at, cleanup_verified, created_at
		 FROM run_steps WHERE id = $1`, id,
	).Scan(&s.ID, &s.RunID, &s.StepNumber, &s.AgentType, &s.Action, &s.Tier, &s.Status, &s.Inputs, &s.Outputs,
		&s.EvidenceIDs, &s.ErrorMessage, &s.StartedAt, &s.CompletedAt, &s.CleanupVerified, &s.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("run step not found: %w", err)
		}
		return nil, fmt.Errorf("getting run step: %w", err)
	}
	return &s, nil
}

func (r *RunStepRepo) ListByRunID(ctx context.Context, runID uuid.UUID) ([]models.RunStep, error) {
	rows, err := r.q.Query(ctx,
		`SELECT id, run_id, step_number, agent_type, action, tier, status, inputs, outputs,
		 evidence_ids, error_message, started_at, completed_at, cleanup_verified, created_at
		 FROM run_steps WHERE run_id = $1 ORDER BY step_number`, runID)
	if err != nil {
		return nil, fmt.Errorf("listing run steps: %w", err)
	}
	defer rows.Close()

	var steps []models.RunStep
	for rows.Next() {
		var s models.RunStep
		if err := rows.Scan(&s.ID, &s.RunID, &s.StepNumber, &s.AgentType, &s.Action, &s.Tier, &s.Status, &s.Inputs, &s.Outputs,
			&s.EvidenceIDs, &s.ErrorMessage, &s.StartedAt, &s.CompletedAt, &s.CleanupVerified, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning run step: %w", err)
		}
		steps = append(steps, s)
	}
	return steps, rows.Err()
}

func (r *RunStepRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status models.RunStepStatus, errMsg *string) error {
	now := time.Now().UTC()
	switch status {
	case models.StepRunning:
		_, err := r.q.Exec(ctx,
			`UPDATE run_steps SET status = $2, started_at = $3 WHERE id = $1`,
			id, status, now)
		if err != nil {
			return fmt.Errorf("updating step to running: %w", err)
		}
		return nil
	default:
		_, err := r.q.Exec(ctx,
			`UPDATE run_steps SET status = $2, completed_at = $3, error_message = $4 WHERE id = $1`,
			id, status, now, errMsg)
		if err != nil {
			return fmt.Errorf("updating step status to %s: %w", status, err)
		}
		return nil
	}
}

func (r *RunStepRepo) SetOutputs(ctx context.Context, id uuid.UUID, outputs json.RawMessage, evidenceIDs []string, cleanupVerified bool) error {
	_, err := r.q.Exec(ctx,
		`UPDATE run_steps SET outputs = $2, evidence_ids = $3, cleanup_verified = $4 WHERE id = $1`,
		id, outputs, evidenceIDs, cleanupVerified)
	if err != nil {
		return fmt.Errorf("setting step outputs: %w", err)
	}
	return nil
}
