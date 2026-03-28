package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

type UserRepo struct {
	q Querier
}

func NewUserRepo(q Querier) *UserRepo {
	return &UserRepo{q: q}
}

func (r *UserRepo) Create(ctx context.Context, u *models.User) error {
	u.ID = uuid.New()
	return r.q.QueryRow(ctx,
		`INSERT INTO users (id, org_id, email, name, password_hash, role, sso_subject, settings)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING created_at, updated_at`,
		u.ID, u.OrgID, u.Email, u.Name, u.PasswordHash, u.Role, u.SSOSubject, u.Settings,
	).Scan(&u.CreatedAt, &u.UpdatedAt)
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var u models.User
	err := r.q.QueryRow(ctx,
		`SELECT id, org_id, email, name, password_hash, role, sso_subject, settings, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.SSOSubject, &u.Settings, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user not found: %w", err)
		}
		return nil, fmt.Errorf("getting user: %w", err)
	}
	return &u, nil
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	err := r.q.QueryRow(ctx,
		`SELECT id, org_id, email, name, password_hash, role, sso_subject, settings, created_at, updated_at
		 FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.SSOSubject, &u.Settings, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user not found: %w", err)
		}
		return nil, fmt.Errorf("getting user by email: %w", err)
	}
	return &u, nil
}

func (r *UserRepo) ListByOrgID(ctx context.Context, orgID uuid.UUID) ([]models.User, error) {
	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, email, name, password_hash, role, sso_subject, settings, created_at, updated_at
		 FROM users WHERE org_id = $1 ORDER BY name LIMIT 1000`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.SSOSubject, &u.Settings, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *UserRepo) ListByOrgIDPaginated(ctx context.Context, orgID uuid.UUID, p models.PaginationParams) ([]models.User, int, error) {
	var total int
	err := r.q.QueryRow(ctx, `SELECT count(*) FROM users WHERE org_id = $1`, orgID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting users: %w", err)
	}

	rows, err := r.q.Query(ctx,
		`SELECT id, org_id, email, name, password_hash, role, sso_subject, settings, created_at, updated_at
		 FROM users WHERE org_id = $1 ORDER BY name LIMIT $2 OFFSET $3`, orgID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.SSOSubject, &u.Settings, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning user: %w", err)
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

func (r *UserRepo) Update(ctx context.Context, u *models.User) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE users SET email = $2, name = $3, role = $4, sso_subject = $5, settings = $6, updated_at = now()
		 WHERE id = $1`,
		u.ID, u.Email, u.Name, u.Role, u.SSOSubject, u.Settings,
	)
	if err != nil {
		return fmt.Errorf("updating user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func (r *UserRepo) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`,
		id, passwordHash,
	)
	if err != nil {
		return fmt.Errorf("updating password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func (r *UserRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.q.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}
