package api

import (
	"net/http"
	"sync"
	"time"

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

// loginAttempt tracks failed login attempts for account lockout.
type loginAttempt struct {
	Count    int
	LastFail time.Time
}

const (
	maxLoginAttempts  = 5
	lockoutDuration   = 15 * time.Minute
)

var (
	loginAttempts   = make(map[string]*loginAttempt)
	loginAttemptsMu sync.Mutex
)

// isLockedOut checks if the email is temporarily locked out.
func isLockedOut(email string) bool {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()
	a, ok := loginAttempts[email]
	if !ok {
		return false
	}
	if a.Count >= maxLoginAttempts && time.Since(a.LastFail) < lockoutDuration {
		return true
	}
	if time.Since(a.LastFail) >= lockoutDuration {
		delete(loginAttempts, email)
		return false
	}
	return false
}

// recordFailedLogin increments the failed attempt counter for an email.
func recordFailedLogin(email string) {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()
	a, ok := loginAttempts[email]
	if !ok {
		a = &loginAttempt{}
		loginAttempts[email] = a
	}
	a.Count++
	a.LastFail = time.Now()
}

// clearLoginAttempts resets the counter on successful login.
func clearLoginAttempts(email string) {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()
	delete(loginAttempts, email)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if err := validateRequired(map[string]string{"email": req.Email, "password": req.Password}); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	// Account lockout check
	if isLockedOut(req.Email) {
		writeError(w, http.StatusTooManyRequests, "account_locked", "Account temporarily locked due to too many failed attempts. Try again later.")
		return
	}

	user, err := h.Users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		recordFailedLogin(req.Email)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}

	if user.PasswordHash == nil {
		recordFailedLogin(req.Email)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
		recordFailedLogin(req.Email)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}

	// Successful login — clear lockout counter
	clearLoginAttempts(req.Email)

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

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := readJSON(r, &req); err != nil {
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

	writeData(w, tokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
	})
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
