package api

import (
	"net/http"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

func (h *Handler) ListApprovals(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	p := parsePagination(r)
	status := r.URL.Query().Get("status")

	if status == "pending" {
		approvals, err := h.Approvals.ListPendingByOrgID(r.Context(), claims.OrgID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "db_error", "Failed to list approvals")
			return
		}
		if approvals == nil {
			approvals = []models.Approval{}
		}
		writeData(w, approvals)
		return
	}

	approvals, total, err := h.Approvals.ListByOrgID(r.Context(), claims.OrgID, p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list approvals")
		return
	}
	if approvals == nil {
		approvals = []models.Approval{}
	}
	writeDataWithMeta(w, approvals, total, p.Page, p.PerPage)
}

func (h *Handler) GetApproval(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "approvalID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid approval ID")
		return
	}

	approval, err := h.Approvals.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Approval not found")
		return
	}
	if approval.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Approval not found")
		return
	}
	writeData(w, approval)
}

func (h *Handler) ApproveRequest(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "approvalID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid approval ID")
		return
	}

	approval, err := h.Approvals.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Approval not found")
		return
	}
	if approval.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Approval not found")
		return
	}

	var req struct {
		Rationale string `json:"rationale"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if err := h.Approvals.UpdateDecision(r.Context(), id, models.ApprovalApproved, claims.UserID, req.Rationale); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to approve request")
		return
	}
	writeData(w, map[string]string{"status": "approved"})
}

func (h *Handler) DenyRequest(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "approvalID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid approval ID")
		return
	}

	approval, err := h.Approvals.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Approval not found")
		return
	}
	if approval.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Approval not found")
		return
	}

	var req struct {
		Rationale string `json:"rationale"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if err := h.Approvals.UpdateDecision(r.Context(), id, models.ApprovalDenied, claims.UserID, req.Rationale); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to deny request")
		return
	}
	writeData(w, map[string]string{"status": "denied"})
}
