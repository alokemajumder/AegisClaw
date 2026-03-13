package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alokemajumder/AegisClaw/internal/config"
	"github.com/alokemajumder/AegisClaw/internal/models"
)

func newTestTokenService(expiry time.Duration) *TokenService {
	return NewTokenService(context.Background(), config.AuthConfig{
		JWTSecret:     "test-secret-key-for-unit-tests-only",
		TokenExpiry:   expiry,
		RefreshExpiry: 24 * time.Hour,
	})
}

func newTestUser() *models.User {
	return &models.User{
		ID:    uuid.New(),
		OrgID: uuid.New(),
		Email: "operator@aegisclaw.io",
		Name:  "Test Operator",
		Role:  models.RoleOperator,
	}
}

func TestGenerateToken_ValidateToken_RoundTrip(t *testing.T) {
	svc := newTestTokenService(15 * time.Minute)
	user := newTestUser()

	tokenStr, err := svc.GenerateToken(user)
	require.NoError(t, err)
	assert.NotEmpty(t, tokenStr)

	claims, err := svc.ValidateToken(tokenStr)
	require.NoError(t, err)

	assert.Equal(t, user.ID, claims.UserID)
	assert.Equal(t, user.OrgID, claims.OrgID)
	assert.Equal(t, user.Email, claims.Email)
	assert.Equal(t, user.Role, claims.Role)
	assert.Equal(t, "aegisclaw", claims.Issuer)
	assert.Equal(t, user.ID.String(), claims.Subject)
}

func TestGenerateToken_AllRoles(t *testing.T) {
	roles := []models.UserRole{
		models.RoleAdmin,
		models.RoleOperator,
		models.RoleViewer,
		models.RoleApprover,
	}

	svc := newTestTokenService(15 * time.Minute)

	for _, role := range roles {
		t.Run(string(role), func(t *testing.T) {
			user := newTestUser()
			user.Role = role

			tokenStr, err := svc.GenerateToken(user)
			require.NoError(t, err)

			claims, err := svc.ValidateToken(tokenStr)
			require.NoError(t, err)
			assert.Equal(t, role, claims.Role)
		})
	}
}

func TestValidateToken_Expired(t *testing.T) {
	// Use a negative expiry so the token is already expired at creation
	svc := newTestTokenService(-1 * time.Second)
	user := newTestUser()

	tokenStr, err := svc.GenerateToken(user)
	require.NoError(t, err)

	_, err = svc.ValidateToken(tokenStr)
	assert.Error(t, err, "expired token should fail validation")
	assert.Contains(t, err.Error(), "token")
}

func TestValidateToken_InvalidString(t *testing.T) {
	svc := newTestTokenService(15 * time.Minute)

	tests := []struct {
		name     string
		tokenStr string
	}{
		{name: "empty string", tokenStr: ""},
		{name: "random garbage", tokenStr: "not.a.jwt.token"},
		{name: "partial jwt", tokenStr: "eyJhbGciOiJIUzI1NiJ9.invalid"},
		{name: "two segments", tokenStr: "header.payload"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.ValidateToken(tt.tokenStr)
			assert.Error(t, err, "invalid token string should fail validation")
		})
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	svc1 := NewTokenService(context.Background(), config.AuthConfig{
		JWTSecret:   "secret-one",
		TokenExpiry: 15 * time.Minute,
	})
	svc2 := NewTokenService(context.Background(), config.AuthConfig{
		JWTSecret:   "secret-two",
		TokenExpiry: 15 * time.Minute,
	})

	user := newTestUser()
	tokenStr, err := svc1.GenerateToken(user)
	require.NoError(t, err)

	_, err = svc2.ValidateToken(tokenStr)
	assert.Error(t, err, "token signed with different secret should fail")
}

func TestGenerateRefreshToken(t *testing.T) {
	svc := newTestTokenService(15 * time.Minute)
	user := newTestUser()

	refreshToken, err := svc.GenerateRefreshToken(user)
	require.NoError(t, err)
	assert.NotEmpty(t, refreshToken)

	// Refresh token uses RegisteredClaims only, not full Claims
	// ValidateToken expects Claims so it should still parse but the custom fields
	// will have zero values. We just verify the token string is non-empty and valid JWT.
	accessToken, err := svc.GenerateToken(user)
	require.NoError(t, err)
	assert.NotEqual(t, refreshToken, accessToken, "refresh and access tokens should differ")
}

func TestMiddleware_ValidToken(t *testing.T) {
	svc := newTestTokenService(15 * time.Minute)
	user := newTestUser()

	tokenStr, err := svc.GenerateToken(user)
	require.NoError(t, err)

	var capturedClaims *Claims
	handler := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := UserFromContext(r.Context())
		if ok {
			capturedClaims = claims
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, capturedClaims)
	assert.Equal(t, user.ID, capturedClaims.UserID)
	assert.Equal(t, user.Email, capturedClaims.Email)
}

func TestMiddleware_MissingAuthHeader(t *testing.T) {
	svc := newTestTokenService(15 * time.Minute)

	handler := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called when auth header is missing")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "missing authorization header")
}

func TestMiddleware_InvalidFormat(t *testing.T) {
	svc := newTestTokenService(15 * time.Minute)

	handler := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for invalid auth format")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid authorization format")
}

func TestMiddleware_InvalidToken(t *testing.T) {
	svc := newTestTokenService(15 * time.Minute)

	handler := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for invalid token")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid token")
}

func TestMiddleware_ExpiredToken(t *testing.T) {
	svc := newTestTokenService(-1 * time.Second)
	user := newTestUser()

	tokenStr, err := svc.GenerateToken(user)
	require.NoError(t, err)

	handler := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for expired token")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireRole_Allowed(t *testing.T) {
	tests := []struct {
		name         string
		userRole     models.UserRole
		allowedRoles []models.UserRole
	}{
		{
			name:         "admin in admin-only",
			userRole:     models.RoleAdmin,
			allowedRoles: []models.UserRole{models.RoleAdmin},
		},
		{
			name:         "operator in operator+admin",
			userRole:     models.RoleOperator,
			allowedRoles: []models.UserRole{models.RoleAdmin, models.RoleOperator},
		},
		{
			name:         "viewer in all roles",
			userRole:     models.RoleViewer,
			allowedRoles: []models.UserRole{models.RoleAdmin, models.RoleOperator, models.RoleViewer, models.RoleApprover},
		},
		{
			name:         "approver in approver-only",
			userRole:     models.RoleApprover,
			allowedRoles: []models.UserRole{models.RoleApprover},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})

			middleware := RequireRole(tt.allowedRoles...)
			handler := middleware(inner)

			claims := &Claims{
				UserID: uuid.New(),
				OrgID:  uuid.New(),
				Email:  "test@aegisclaw.io",
				Role:   tt.userRole,
			}
			ctx := context.WithValue(context.Background(), userContextKey, claims)
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil).WithContext(ctx)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.True(t, called, "handler should be called for allowed role")
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

func TestRequireRole_Denied(t *testing.T) {
	tests := []struct {
		name         string
		userRole     models.UserRole
		allowedRoles []models.UserRole
	}{
		{
			name:         "viewer denied admin-only",
			userRole:     models.RoleViewer,
			allowedRoles: []models.UserRole{models.RoleAdmin},
		},
		{
			name:         "operator denied approver-only",
			userRole:     models.RoleOperator,
			allowedRoles: []models.UserRole{models.RoleApprover},
		},
		{
			name:         "approver denied admin+operator",
			userRole:     models.RoleApprover,
			allowedRoles: []models.UserRole{models.RoleAdmin, models.RoleOperator},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("handler should not be called for denied role")
			})

			middleware := RequireRole(tt.allowedRoles...)
			handler := middleware(inner)

			claims := &Claims{
				UserID: uuid.New(),
				OrgID:  uuid.New(),
				Email:  "test@aegisclaw.io",
				Role:   tt.userRole,
			}
			ctx := context.WithValue(context.Background(), userContextKey, claims)
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil).WithContext(ctx)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusForbidden, rec.Code)
			assert.Contains(t, rec.Body.String(), "insufficient permissions")
		})
	}
}

func TestRequireRole_NoClaimsInContext(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called without claims in context")
	})

	middleware := RequireRole(models.RoleAdmin)
	handler := middleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "not authenticated")
}

func TestUserFromContext_WithClaims(t *testing.T) {
	userID := uuid.New()
	orgID := uuid.New()
	email := "admin@aegisclaw.io"
	role := models.RoleAdmin

	claims := &Claims{
		UserID: userID,
		OrgID:  orgID,
		Email:  email,
		Role:   role,
	}

	ctx := context.WithValue(context.Background(), userContextKey, claims)
	got, ok := UserFromContext(ctx)

	require.True(t, ok)
	assert.Equal(t, userID, got.UserID)
	assert.Equal(t, orgID, got.OrgID)
	assert.Equal(t, email, got.Email)
	assert.Equal(t, role, got.Role)
}

func TestUserFromContext_NoClaims(t *testing.T) {
	ctx := context.Background()
	got, ok := UserFromContext(ctx)

	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestUserFromContext_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), userContextKey, "not-claims")
	got, ok := UserFromContext(ctx)

	assert.False(t, ok)
	assert.Nil(t, got)
}
