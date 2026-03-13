package repository

import (
	"context"
	"fmt"
	"time"
)

// TokenBlacklistRepo persists revoked token hashes in the database.
type TokenBlacklistRepo struct {
	q Querier
}

// NewTokenBlacklistRepo creates a new TokenBlacklistRepo.
func NewTokenBlacklistRepo(q Querier) *TokenBlacklistRepo {
	return &TokenBlacklistRepo{q: q}
}

// Add inserts a token hash into the blacklist with its expiry time.
func (r *TokenBlacklistRepo) Add(ctx context.Context, tokenHash string, expiresAt time.Time) error {
	_, err := r.q.Exec(ctx,
		`INSERT INTO token_blacklist (token_hash, expires_at) VALUES ($1, $2) ON CONFLICT (token_hash) DO NOTHING`,
		tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("adding token to blacklist: %w", err)
	}
	return nil
}

// IsBlacklisted checks whether a token hash exists in the blacklist.
func (r *TokenBlacklistRepo) IsBlacklisted(ctx context.Context, tokenHash string) (bool, error) {
	var exists bool
	err := r.q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM token_blacklist WHERE token_hash = $1)`,
		tokenHash).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking token blacklist: %w", err)
	}
	return exists, nil
}

// Cleanup removes expired entries from the blacklist.
func (r *TokenBlacklistRepo) Cleanup(ctx context.Context) error {
	_, err := r.q.Exec(ctx, `DELETE FROM token_blacklist WHERE expires_at < NOW()`)
	if err != nil {
		return fmt.Errorf("cleaning up token blacklist: %w", err)
	}
	return nil
}
