package api

import (
	"encoding/json"
	"net/http"

	"github.com/alokemajumder/AegisClaw/internal/models"
	natspkg "github.com/alokemajumder/AegisClaw/internal/nats"
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
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if err := h.Approvals.UpdateDecision(r.Context(), id, models.ApprovalApproved, claims.UserID, req.Rationale); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to approve request")
		return
	}

	// Publish approval-granted event so the orchestrator can resume the blocked step.
	// Only process run-type approvals — other entity types don't need NATS dispatch.
	if h.Publisher != nil && approval.TargetEntityID != nil &&
		approval.TargetEntityType != nil && *approval.TargetEntityType == "run" {
		runID := *approval.TargetEntityID
		// Find the blocked step number from the run steps
		steps, err := h.RunSteps.ListByRunID(r.Context(), runID)
		if err == nil {
			for _, step := range steps {
				if step.Status == models.StepBlocked {
					run, runErr := h.Runs.GetByID(r.Context(), runID)
					if runErr == nil {
						grantedMsg := natspkg.ApprovalGrantedMsg{
							RunID:        runID,
							StepNumber:   step.StepNumber,
							ApprovalID:   id,
							EngagementID: run.EngagementID,
							OrgID:        approval.OrgID,
						}
						if pubErr := h.Publisher.Publish(r.Context(), natspkg.SubjectApprovalGranted, approval.OrgID, grantedMsg); pubErr != nil {
							h.Logger.Error("publishing approval granted event", "error", pubErr, "approval_id", id)
						}
					}
					// Each approval targets one blocked step. Resume only the first
					// blocked step found; additional blocked steps require separate approvals.
					break
				}
			}
		}
	}

	resID := id.String()
	details, _ := json.Marshal(map[string]string{"rationale": req.Rationale})
	h.audit(r.Context(), r, claims, "approval.approve", "approval", &resID, details)

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
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if err := h.Approvals.UpdateDecision(r.Context(), id, models.ApprovalDenied, claims.UserID, req.Rationale); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to deny request")
		return
	}

	resID := id.String()
	details, _ := json.Marshal(map[string]string{"rationale": req.Rationale})
	h.audit(r.Context(), r, claims, "approval.deny", "approval", &resID, details)

	writeData(w, map[string]string{"status": "denied"})
}
