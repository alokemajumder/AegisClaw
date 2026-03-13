package api

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

func (h *Handler) ListFindings(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	p := parsePagination(r)
	severity := r.URL.Query().Get("severity")
	status := r.URL.Query().Get("status")

	findings, total, err := h.Findings.ListByOrgID(r.Context(), claims.OrgID, p, severity, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list findings")
		return
	}
	if findings == nil {
		findings = []models.Finding{}
	}
	writeDataWithMeta(w, findings, total, p.Page, p.PerPage)
}

func (h *Handler) GetFinding(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "findingID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid finding ID")
		return
	}

	finding, err := h.Findings.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Finding not found")
		return
	}
	if finding.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Finding not found")
		return
	}
	writeData(w, finding)
}

func (h *Handler) UpdateFinding(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "findingID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid finding ID")
		return
	}

	finding, err := h.Findings.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Finding not found")
		return
	}
	if finding.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Finding not found")
		return
	}

	var req struct {
		Title       *string `json:"title,omitempty"`
		Description *string `json:"description,omitempty"`
		Severity    *string `json:"severity,omitempty"`
		Status      *string `json:"status,omitempty"`
		Remediation *string `json:"remediation,omitempty"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if req.Title != nil {
		finding.Title = *req.Title
	}
	if req.Description != nil {
		finding.Description = req.Description
	}
	if req.Severity != nil {
		if err := validateSeverity(*req.Severity); err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		finding.Severity = models.Severity(*req.Severity)
	}
	if req.Status != nil {
		if err := validateFindingStatus(*req.Status); err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		finding.Status = models.FindingStatus(*req.Status)
	}
	if req.Remediation != nil {
		finding.Remediation = req.Remediation
	}

	if err := h.Findings.Update(r.Context(), finding); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to update finding")
		return
	}
	writeData(w, finding)
}

func (h *Handler) CreateFindingTicket(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "findingID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid finding ID")
		return
	}

	var req struct {
		ConnectorID string `json:"connector_id"`
		Priority    string `json:"priority"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	finding, err := h.Findings.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Finding not found")
		return
	}
	if finding.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Finding not found")
		return
	}

	connectorID, err := uuid.Parse(req.ConnectorID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid connector ID")
		return
	}

	ticketID := "TKT-" + finding.ID.String()[:8]
	if err := h.Findings.SetTicket(r.Context(), id, ticketID, connectorID); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to create ticket")
		return
	}

	writeData(w, map[string]string{"ticket_id": ticketID, "status": "ticketed"})
}

func (h *Handler) RetestFinding(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "findingID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid finding ID")
		return
	}

	finding, err := h.Findings.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Finding not found")
		return
	}
	if finding.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Finding not found")
		return
	}

	if err := h.Findings.UpdateStatus(r.Context(), id, models.FindingRetested); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to update finding")
		return
	}
	writeData(w, map[string]string{"status": "retested"})
}
