package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/internal/models"
	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
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
	if err := readJSON(w, r, &req); err != nil {
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

	resID := finding.ID.String()
	h.audit(r.Context(), r, claims, "finding.update", "finding", &resID, nil)

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
	if err := readJSON(w, r, &req); err != nil {
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

	// Verify the connector belongs to the user's org
	conn, err := h.ConnInst.GetByID(r.Context(), connectorID)
	if err != nil || conn.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Connector not found")
		return
	}

	if h.ConnectorSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Connector service not configured")
		return
	}

	desc := finding.Title
	if finding.Description != nil {
		desc = fmt.Sprintf("%s\n\n%s", finding.Title, *finding.Description)
	}
	priority := req.Priority
	if priority == "" {
		priority = string(finding.Severity)
	}

	ticketReq := connectorsdk.TicketRequest{
		Title:       fmt.Sprintf("[AegisClaw] %s", finding.Title),
		Description: desc,
		Priority:    priority,
	}

	result, err := h.ConnectorSvc.CreateTicket(r.Context(), connectorID, ticketReq)
	if err != nil {
		h.Logger.Error("creating ticket via connector", "connector_id", connectorID, "error", err)
		writeError(w, http.StatusInternalServerError, "connector_error", "Failed to create ticket via connector")
		return
	}

	if err := h.Findings.SetTicket(r.Context(), id, result.TicketID, connectorID); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to save ticket reference")
		return
	}

	// Store ticket_url in finding metadata so it persists for future reads
	if result.TicketURL != "" {
		meta := map[string]any{}
		if finding.Metadata != nil {
			_ = json.Unmarshal(finding.Metadata, &meta)
		}
		meta["ticket_url"] = result.TicketURL
		metaBytes, _ := json.Marshal(meta)
		_ = h.Findings.UpdateMetadata(r.Context(), id, metaBytes)
	}

	writeData(w, map[string]string{"ticket_id": result.TicketID, "ticket_url": result.TicketURL, "status": "ticketed"})
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
