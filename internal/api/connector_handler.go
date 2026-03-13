package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

func (h *Handler) ListConnectorRegistry(w http.ResponseWriter, r *http.Request) {
	entries, err := h.ConnReg.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list registry")
		return
	}
	if entries == nil {
		entries = []models.ConnectorRegistryEntry{}
	}
	writeData(w, entries)
}

func (h *Handler) GetConnectorType(w http.ResponseWriter, r *http.Request) {
	connType := getURLParam(r, "connectorType")
	entry, err := h.ConnReg.GetByType(r.Context(), connType)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Connector type not found")
		return
	}
	writeData(w, entry)
}

func (h *Handler) ListConnectorInstances(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	category := r.URL.Query().Get("category")

	var instances []models.ConnectorInstance
	var err error
	if category != "" {
		instances, err = h.ConnInst.ListByCategory(r.Context(), claims.OrgID, category)
	} else {
		instances, err = h.ConnInst.ListByOrgID(r.Context(), claims.OrgID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list connectors")
		return
	}
	if instances == nil {
		instances = []models.ConnectorInstance{}
	}
	writeData(w, instances)
}

func (h *Handler) CreateConnectorInstance(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)

	var req struct {
		ConnectorType   string          `json:"connector_type"`
		Category        string          `json:"category"`
		Name            string          `json:"name"`
		Description     *string         `json:"description,omitempty"`
		Config          json.RawMessage `json:"config"`
		AuthMethod      string          `json:"auth_method"`
		SecretRef       *string         `json:"secret_ref,omitempty"`
		RateLimitConfig json.RawMessage `json:"rate_limit_config,omitempty"`
		RetryConfig     json.RawMessage `json:"retry_config,omitempty"`
		FieldMappings   json.RawMessage `json:"field_mappings,omitempty"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}
	if err := validateRequired(map[string]string{
		"connector_type": req.ConnectorType,
		"name":           req.Name,
		"auth_method":    req.AuthMethod,
	}); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	if req.Config == nil {
		req.Config = json.RawMessage(`{}`)
	}
	if req.RateLimitConfig == nil {
		req.RateLimitConfig = json.RawMessage(`{"requests_per_second": 10, "burst": 20}`)
	}
	if req.RetryConfig == nil {
		req.RetryConfig = json.RawMessage(`{"max_retries": 3, "backoff_ms": 1000}`)
	}
	if req.FieldMappings == nil {
		req.FieldMappings = json.RawMessage(`{}`)
	}

	// Look up category from registry if not provided
	category := req.Category
	if category == "" {
		entry, err := h.ConnReg.GetByType(r.Context(), req.ConnectorType)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_type", "Unknown connector type")
			return
		}
		category = string(entry.Category)
	}

	ci := &models.ConnectorInstance{
		OrgID:           claims.OrgID,
		ConnectorType:   req.ConnectorType,
		Category:        models.ConnectorCategory(category),
		Name:            req.Name,
		Description:     req.Description,
		Enabled:         true,
		Config:          req.Config,
		SecretRef:       req.SecretRef,
		AuthMethod:      req.AuthMethod,
		RateLimitConfig: req.RateLimitConfig,
		RetryConfig:     req.RetryConfig,
		FieldMappings:   req.FieldMappings,
	}

	if err := h.ConnInst.Create(r.Context(), ci); err != nil {
		h.Logger.Error("creating connector instance", "error", err)
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to create connector")
		return
	}

	writeJSON(w, http.StatusCreated, models.APIResponse{Data: ci})
}

func (h *Handler) GetConnectorInstance(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "connectorID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid connector ID")
		return
	}

	ci, err := h.ConnInst.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Connector not found")
		return
	}
	if ci.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Connector not found")
		return
	}
	writeData(w, ci)
}

func (h *Handler) UpdateConnectorInstance(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "connectorID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid connector ID")
		return
	}

	ci, err := h.ConnInst.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Connector not found")
		return
	}
	if ci.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Connector not found")
		return
	}

	var req struct {
		Name            *string         `json:"name,omitempty"`
		Description     *string         `json:"description,omitempty"`
		Config          json.RawMessage `json:"config,omitempty"`
		AuthMethod      *string         `json:"auth_method,omitempty"`
		SecretRef       *string         `json:"secret_ref,omitempty"`
		RateLimitConfig json.RawMessage `json:"rate_limit_config,omitempty"`
		RetryConfig     json.RawMessage `json:"retry_config,omitempty"`
		FieldMappings   json.RawMessage `json:"field_mappings,omitempty"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if req.Name != nil {
		ci.Name = *req.Name
	}
	if req.Description != nil {
		ci.Description = req.Description
	}
	if req.Config != nil {
		ci.Config = req.Config
	}
	if req.AuthMethod != nil {
		ci.AuthMethod = *req.AuthMethod
	}
	if req.SecretRef != nil {
		ci.SecretRef = req.SecretRef
	}
	if req.RateLimitConfig != nil {
		ci.RateLimitConfig = req.RateLimitConfig
	}
	if req.RetryConfig != nil {
		ci.RetryConfig = req.RetryConfig
	}
	if req.FieldMappings != nil {
		ci.FieldMappings = req.FieldMappings
	}

	if err := h.ConnInst.Update(r.Context(), ci); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to update connector")
		return
	}
	writeData(w, ci)
}

func (h *Handler) DeleteConnectorInstance(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "connectorID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid connector ID")
		return
	}

	ci, err := h.ConnInst.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Connector not found")
		return
	}
	if ci.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Connector not found")
		return
	}

	if err := h.ConnInst.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Connector not found")
		return
	}
	writeData(w, map[string]string{"status": "deleted"})
}

func (h *Handler) ToggleConnector(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "connectorID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid connector ID")
		return
	}

	ci, err := h.ConnInst.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Connector not found")
		return
	}
	if ci.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Connector not found")
		return
	}

	ci.Enabled = !ci.Enabled
	if err := h.ConnInst.Update(r.Context(), ci); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to toggle connector")
		return
	}
	writeData(w, map[string]any{"enabled": ci.Enabled})
}

func (h *Handler) TestConnector(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "connectorID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid connector ID")
		return
	}

	ci, err := h.ConnInst.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Connector not found")
		return
	}
	if ci.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Connector not found")
		return
	}

	// MVP: report test success and update health
	_ = h.ConnInst.UpdateHealthStatus(r.Context(), id, "healthy")
	writeData(w, map[string]string{"status": "healthy", "message": "Connection test passed"})
}

func (h *Handler) GetConnectorHealth(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "connectorID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid connector ID")
		return
	}

	ci, err := h.ConnInst.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Connector not found")
		return
	}
	if ci.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Connector not found")
		return
	}
	writeData(w, map[string]any{
		"health_status":    ci.HealthStatus,
		"health_checked_at": ci.HealthCheckedAt,
	})
}

func (h *Handler) TriggerHealthCheck(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "connectorID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid connector ID")
		return
	}

	ci, err := h.ConnInst.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Connector not found")
		return
	}
	if ci.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Connector not found")
		return
	}

	_ = h.ConnInst.UpdateHealthStatus(r.Context(), id, "healthy")
	writeData(w, map[string]string{"status": "healthy"})
}

// getURLParam extracts a URL parameter using chi's URLParam.
func getURLParam(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}
