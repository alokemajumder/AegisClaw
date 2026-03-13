package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/internal/config"
	"github.com/alokemajumder/AegisClaw/internal/models"
)

type contextKey string

const userContextKey contextKey = "user"

// Claims holds JWT token claims.
type Claims struct {
	jwt.RegisteredClaims
	UserID uuid.UUID      `json:"user_id"`
	OrgID  uuid.UUID      `json:"org_id"`
	Email  string         `json:"email"`
	Role   models.UserRole `json:"role"`
}

// TokenBlacklistStore abstracts persistent token blacklist storage.
type TokenBlacklistStore interface {
	Add(ctx context.Context, hash string, exp time.Time) error
	IsBlacklisted(ctx context.Context, hash string) (bool, error)
	Cleanup(ctx context.Context) error
}

// TokenService handles JWT token generation and validation.
type TokenService struct {
	secret        []byte
	tokenExpiry   time.Duration
	refreshExpiry time.Duration
	Blacklist     *TokenBlacklist
}

// NewTokenService creates a new token service.
func NewTokenService(ctx context.Context, cfg config.AuthConfig) *TokenService {
	return &TokenService{
		secret:        []byte(cfg.JWTSecret),
		tokenExpiry:   cfg.TokenExpiry,
		refreshExpiry: cfg.RefreshExpiry,
		Blacklist:     NewTokenBlacklist(ctx),
	}
}

// SetBlacklistStore configures a persistent backing store for the blacklist.
// When set, Revoke and IsRevoked delegate to the store instead of the in-memory map.
func (s *TokenService) SetBlacklistStore(store TokenBlacklistStore) {
	s.Blacklist.store = store
}

// TokenBlacklist tracks revoked tokens until they naturally expire.
// When a TokenBlacklistStore is set, it delegates to the persistent store;
// otherwise it falls back to an in-memory map (useful for tests).
type TokenBlacklist struct {
	mu     sync.RWMutex
	tokens map[string]time.Time // token hash -> expiry time (in-memory fallback)
	store  TokenBlacklistStore  // persistent store (nil = in-memory only)
}

// NewTokenBlacklist creates a token blacklist with periodic cleanup.
func NewTokenBlacklist(ctx context.Context) *TokenBlacklist {
	bl := &TokenBlacklist{
		tokens: make(map[string]time.Time),
	}
	go bl.cleanup(ctx)
	return bl
}

// Revoke adds a token to the blacklist. The token stays blacklisted until its expiry.
func (bl *TokenBlacklist) Revoke(tokenStr string, expiry time.Time) {
	h := hashToken(tokenStr)
	if bl.store != nil {
		if err := bl.store.Add(context.Background(), h, expiry); err != nil {
			slog.Error("failed to persist token revocation", "error", err)
		}
		return
	}
	bl.mu.Lock()
	defer bl.mu.Unlock()
	bl.tokens[h] = expiry
}

// IsRevoked checks if a token has been revoked.
func (bl *TokenBlacklist) IsRevoked(tokenStr string) bool {
	h := hashToken(tokenStr)
	if bl.store != nil {
		revoked, err := bl.store.IsBlacklisted(context.Background(), h)
		if err != nil {
			slog.Error("failed to check token blacklist", "error", err)
			return false
		}
		return revoked
	}
	bl.mu.RLock()
	defer bl.mu.RUnlock()
	_, ok := bl.tokens[h]
	return ok
}

func (bl *TokenBlacklist) cleanup(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if bl.store != nil {
				if err := bl.store.Cleanup(ctx); err != nil {
					slog.Error("failed to cleanup token blacklist", "error", err)
				}
				continue
			}
			bl.mu.Lock()
			now := time.Now()
			for k, exp := range bl.tokens {
				if now.After(exp) {
					delete(bl.tokens, k)
				}
			}
			bl.mu.Unlock()
		}
	}
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// GenerateToken creates a new JWT access token for a user.
func (s *TokenService) GenerateToken(user *models.User) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.tokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "aegisclaw",
			Subject:   user.ID.String(),
		},
		UserID: user.ID,
		OrgID:  user.OrgID,
		Email:  user.Email,
		Role:   user.Role,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// GenerateRefreshToken creates a new JWT refresh token.
func (s *TokenService) GenerateRefreshToken(user *models.User) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.refreshExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "aegisclaw",
			Subject:   user.ID.String(),
		},
		UserID: user.ID,
		OrgID:  user.OrgID,
		Email:  user.Email,
		Role:   user.Role,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// ValidateToken parses and validates a JWT token string.
func (s *TokenService) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parsing token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// Middleware returns an HTTP middleware that validates JWT tokens.
func (s *TokenService) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":{"code":"unauthorized","message":"missing authorization header"}}`, http.StatusUnauthorized)
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenStr == authHeader {
			http.Error(w, `{"error":{"code":"unauthorized","message":"invalid authorization format"}}`, http.StatusUnauthorized)
			return
		}

		claims, err := s.ValidateToken(tokenStr)
		if err != nil {
			http.Error(w, `{"error":{"code":"unauthorized","message":"invalid token"}}`, http.StatusUnauthorized)
			return
		}

		if s.Blacklist != nil && s.Blacklist.IsRevoked(tokenStr) {
			http.Error(w, `{"error":{"code":"unauthorized","message":"token has been revoked"}}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserFromContext extracts the authenticated user claims from context.
func UserFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(userContextKey).(*Claims)
	return claims, ok
}

// RequireRole returns middleware that enforces a minimum role level.
func RequireRole(roles ...models.UserRole) func(http.Handler) http.Handler {
	roleSet := make(map[models.UserRole]bool, len(roles))
	for _, r := range roles {
		roleSet[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := UserFromContext(r.Context())
			if !ok {
				http.Error(w, `{"error":{"code":"unauthorized","message":"not authenticated"}}`, http.StatusUnauthorized)
				return
			}

			if !roleSet[claims.Role] {
				http.Error(w, `{"error":{"code":"forbidden","message":"insufficient permissions"}}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
