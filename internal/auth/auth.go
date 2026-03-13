package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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

// TokenService handles JWT token generation and validation.
type TokenService struct {
	secret       []byte
	tokenExpiry  time.Duration
	refreshExpiry time.Duration
}

// NewTokenService creates a new token service.
func NewTokenService(cfg config.AuthConfig) *TokenService {
	return &TokenService{
		secret:        []byte(cfg.JWTSecret),
		tokenExpiry:   cfg.TokenExpiry,
		refreshExpiry: cfg.RefreshExpiry,
	}
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
