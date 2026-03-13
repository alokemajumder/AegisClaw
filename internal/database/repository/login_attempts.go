package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// LoginAttemptRepo persists login attempt tracking for account lockout.
type LoginAttemptRepo struct {
	q Querier
}

// NewLoginAttemptRepo creates a new LoginAttemptRepo.
func NewLoginAttemptRepo(q Querier) *LoginAttemptRepo {
	return &LoginAttemptRepo{q: q}
}

// RecordFailure increments the failed attempt counter for an email.
// If the count reaches 5, the account is locked for 15 minutes.
func (r *LoginAttemptRepo) RecordFailure(ctx context.Context, email string) error {
	lockoutDuration := 15 * time.Minute
	_, err := r.q.Exec(ctx,
		`INSERT INTO login_attempts (email, attempt_count, last_attempt_at, updated_at)
		 VALUES ($1, 1, NOW(), NOW())
		 ON CONFLICT (email) DO UPDATE SET
		   attempt_count = login_attempts.attempt_count + 1,
		   last_attempt_at = NOW(),
		   locked_until = CASE
		     WHEN login_attempts.attempt_count + 1 >= 5 THEN NOW() + $2::interval
		     ELSE login_attempts.locked_until
		   END,
		   updated_at = NOW()`,
		email, fmt.Sprintf("%d seconds", int(lockoutDuration.Seconds())))
	if err != nil {
		return fmt.Errorf("recording failed login: %w", err)
	}
	return nil
}

// IsLocked checks whether the given email is currently locked out.
func (r *LoginAttemptRepo) IsLocked(ctx context.Context, email string) (bool, error) {
	var lockedUntil *time.Time
	err := r.q.QueryRow(ctx,
		`SELECT locked_until FROM login_attempts WHERE email = $1`, email,
	).Scan(&lockedUntil)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("checking login lockout: %w", err)
	}
	if lockedUntil == nil {
		return false, nil
	}
	return lockedUntil.After(time.Now()), nil
}

// Reset removes the login attempt record for the given email (on successful login).
func (r *LoginAttemptRepo) Reset(ctx context.Context, email string) error {
	_, err := r.q.Exec(ctx, `DELETE FROM login_attempts WHERE email = $1`, email)
	if err != nil {
		return fmt.Errorf("resetting login attempts: %w", err)
	}
	return nil
}
