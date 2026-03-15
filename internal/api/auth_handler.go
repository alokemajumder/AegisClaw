package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/alokemajumder/AegisClaw/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

// LoginLockoutStore abstracts persistent login attempt tracking.
type LoginLockoutStore interface {
	RecordFailure(ctx context.Context, email string) error
	IsLocked(ctx context.Context, email string) (bool, error)
	Reset(ctx context.Context, email string) error
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if err := validateRequired(map[string]string{"email": req.Email, "password": req.Password}); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	// Account lockout check
	if h.LockoutStore != nil {
		locked, err := h.LockoutStore.IsLocked(r.Context(), req.Email)
		if err != nil {
			h.Logger.Error("checking login lockout", "error", err)
		}
		if locked {
			writeError(w, http.StatusTooManyRequests, "account_locked", "Account temporarily locked due to too many failed attempts. Try again later.")
			return
		}
	}

	user, err := h.Users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		h.recordFailedLogin(r.Context(), req.Email)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}

	if user.PasswordHash == nil {
		h.recordFailedLogin(r.Context(), req.Email)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
		h.recordFailedLogin(r.Context(), req.Email)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}

	// Successful login — clear lockout counter
	if h.LockoutStore != nil {
		if err := h.LockoutStore.Reset(r.Context(), req.Email); err != nil {
			h.Logger.Error("resetting login attempts", "error", err)
		}
	}

	accessToken, err := h.TokenSvc.GenerateToken(user)
	if err != nil {
		h.Logger.Error("generating access token", "error", err)
		writeError(w, http.StatusInternalServerError, "token_error", "Failed to generate token")
		return
	}

	refreshToken, err := h.TokenSvc.GenerateRefreshToken(user)
	if err != nil {
		h.Logger.Error("generating refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "token_error", "Failed to generate token")
		return
	}

	writeData(w, tokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
	})
}

// recordFailedLogin delegates to the lockout store if available.
func (h *Handler) recordFailedLogin(ctx context.Context, email string) {
	if h.LockoutStore != nil {
		if err := h.LockoutStore.RecordFailure(ctx, email); err != nil {
			h.Logger.Error("recording failed login", "error", err)
		}
	}
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	claims, err := h.TokenSvc.ValidateToken(req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_token", "Invalid refresh token")
		return
	}

	user, err := h.Users.GetByID(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_token", "User not found")
		return
	}

	accessToken, err := h.TokenSvc.GenerateToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_error", "Failed to generate token")
		return
	}

	refreshToken, err := h.TokenSvc.GenerateRefreshToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_error", "Failed to generate token")
		return
	}

	// Revoke the old refresh token to prevent reuse
	h.TokenSvc.Blacklist.Revoke(req.RefreshToken, claims.ExpiresAt.Time)

	writeData(w, tokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
	})
}

// Logout revokes the current access token.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenStr == "" || tokenStr == authHeader {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing authorization header")
		return
	}

	claims, err := h.TokenSvc.ValidateToken(tokenStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_token", "invalid token")
		return
	}

	h.TokenSvc.Blacklist.Revoke(tokenStr, claims.ExpiresAt.Time)
	writeJSON(w, http.StatusOK, models.APIResponse{Data: map[string]string{"message": "logged out successfully"}})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromRequest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Not authenticated")
		return
	}

	user, err := h.Users.GetByID(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "User not found")
		return
	}

	writeData(w, user)
}
