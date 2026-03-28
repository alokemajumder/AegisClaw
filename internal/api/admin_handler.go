package api

import (
	"encoding/json"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"github.com/alokemajumder/AegisClaw/internal/models"
	natspkg "github.com/alokemajumder/AegisClaw/internal/nats"
)

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	p := parsePagination(r)
	users, total, err := h.Users.ListByOrgIDPaginated(r.Context(), claims.OrgID, p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list users")
		return
	}
	if users == nil {
		users = []models.User{}
	}
	writeDataWithMeta(w, users, total, p.Page, p.PerPage)
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)

	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}
	if err := validateRequired(map[string]string{
		"email":    req.Email,
		"name":     req.Name,
		"password": req.Password,
		"role":     req.Role,
	}); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if err := validateEmail(req.Email); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if err := validateUserRole(req.Role); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if err := validateMaxLength(map[string]string{"name": req.Name, "email": req.Email}, 255); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if len(req.Password) < 8 || len(req.Password) > 128 {
		writeError(w, http.StatusBadRequest, "validation_error", "password must be between 8 and 128 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash_error", "Failed to hash password")
		return
	}
	hashStr := string(hash)

	user := &models.User{
		OrgID:        claims.OrgID,
		Email:        req.Email,
		Name:         req.Name,
		PasswordHash: &hashStr,
		Role:         models.UserRole(req.Role),
		Settings:     json.RawMessage(`{}`),
	}

	if err := h.Users.Create(r.Context(), user); err != nil {
		h.Logger.Error("creating user", "error", err)
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to create user")
		return
	}

	writeJSON(w, http.StatusCreated, models.APIResponse{Data: user})
}

func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "userID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	claims, _ := claimsFromRequest(r)

	user, err := h.Users.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if user.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "User not found")
		return
	}

	var req struct {
		Name *string `json:"name,omitempty"`
		Role *string `json:"role,omitempty"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if req.Name != nil {
		user.Name = *req.Name
	}
	if req.Role != nil {
		if err := validateUserRole(*req.Role); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_role", err.Error())
			return
		}
		user.Role = models.UserRole(*req.Role)
	}

	if err := h.Users.Update(r.Context(), user); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to update user")
		return
	}
	writeData(w, user)
}

func (h *Handler) QueryAuditLog(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	p := parsePagination(r)
	action := r.URL.Query().Get("action")
	resourceType := r.URL.Query().Get("resource_type")

	logs, total, err := h.AuditLogs.ListByOrgID(r.Context(), claims.OrgID, p, action, resourceType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list audit logs")
		return
	}
	if logs == nil {
		logs = []models.AuditLog{}
	}
	writeDataWithMeta(w, logs, total, p.Page, p.PerPage)
}

func (h *Handler) SystemHealth(w http.ResponseWriter, r *http.Request) {
	dbHealthy := h.DB.Ping(r.Context()) == nil

	status := "healthy"
	if !dbHealthy {
		status = "degraded"
	}

	writeData(w, map[string]any{
		"status":   status,
		"service":  "api-gateway",
		"version":  "1.0.0",
		"database": dbHealthy,
		"kill_switch_engaged": h.IsKillSwitchEngaged(),
	})
}

func (h *Handler) KillSwitch(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)

	var req struct {
		Engaged bool   `json:"engaged"`
		Reason  string `json:"reason"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	h.SetKillSwitch(req.Engaged)

	// Publish kill switch message via NATS
	if h.Publisher != nil {
		msg := natspkg.KillSwitchMsg{
			Engaged: req.Engaged,
			Reason:  req.Reason,
			ActorID: claims.UserID.String(),
		}
		if err := h.Publisher.Publish(r.Context(), natspkg.SubjectKillSwitch, claims.OrgID, msg); err != nil {
			h.Logger.Error("publishing kill switch", "error", err)
		}
	}

	// Audit log
	actionStr := "kill_switch_engaged"
	if !req.Engaged {
		actionStr = "kill_switch_disengaged"
	}
	details, _ := json.Marshal(map[string]string{"reason": req.Reason})
	auditEntry := &models.AuditLog{
		OrgID:        claims.OrgID,
		ActorType:    "user",
		ActorID:      claims.UserID.String(),
		Action:       actionStr,
		ResourceType: "system",
		Details:      json.RawMessage(details),
	}
	if err := h.AuditLogs.Create(r.Context(), auditEntry); err != nil {
		h.Logger.Error("creating audit log for kill switch", "error", err)
	}

	// If engaging, kill all running runs
	if req.Engaged {
		runs, err := h.Runs.ListRunning(r.Context())
		if err == nil {
			for _, run := range runs {
				_ = h.Runs.UpdateStatus(r.Context(), run.ID, models.RunKilled)
			}
			h.Logger.Info("kill switch engaged, killed running runs", "count", len(runs))
		}
	}

	writeData(w, map[string]any{
		"engaged": req.Engaged,
		"reason":  req.Reason,
	})
}
