package api

import (
	"net/http"

	"github.com/alokemajumder/AegisClaw/internal/models"
	natspkg "github.com/alokemajumder/AegisClaw/internal/nats"
)

func (h *Handler) ListRuns(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	p := parsePagination(r)
	status := r.URL.Query().Get("status")

	runs, total, err := h.Runs.ListByOrgID(r.Context(), claims.OrgID, p, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list runs")
		return
	}
	if runs == nil {
		runs = []models.Run{}
	}
	writeDataWithMeta(w, runs, total, p.Page, p.PerPage)
}

func (h *Handler) GetRun(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "runID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid run ID")
		return
	}

	run, err := h.Runs.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Run not found")
		return
	}
	if run.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Run not found")
		return
	}
	writeData(w, run)
}

func (h *Handler) ListRunSteps(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "runID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid run ID")
		return
	}

	// Verify the run belongs to the requesting org before listing its steps
	run, err := h.Runs.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Run not found")
		return
	}
	if run.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Run not found")
		return
	}

	steps, err := h.RunSteps.ListByRunID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list steps")
		return
	}
	if steps == nil {
		steps = []models.RunStep{}
	}
	writeData(w, steps)
}

func (h *Handler) GetRunReceipt(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "runID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid run ID")
		return
	}

	run, err := h.Runs.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Run not found")
		return
	}
	if run.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Run not found")
		return
	}

	if run.ReceiptID == nil {
		writeError(w, http.StatusNotFound, "not_found", "No receipt available for this run")
		return
	}

	writeData(w, map[string]string{"receipt_id": *run.ReceiptID, "run_id": run.ID.String()})
}

func (h *Handler) KillRun(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "runID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid run ID")
		return
	}

	run, err := h.Runs.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Run not found")
		return
	}
	if run.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Run not found")
		return
	}

	if err := h.Runs.UpdateStatus(r.Context(), id, models.RunKilled); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to kill run")
		return
	}
	writeData(w, map[string]string{"status": "killed"})
}

func (h *Handler) PauseRun(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "runID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid run ID")
		return
	}

	run, err := h.Runs.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Run not found")
		return
	}
	if run.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Run not found")
		return
	}

	if err := h.Runs.UpdateStatus(r.Context(), id, models.RunPaused); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to pause run")
		return
	}
	writeData(w, map[string]string{"status": "paused"})
}

func (h *Handler) ResumeRun(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "runID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid run ID")
		return
	}

	run, err := h.Runs.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Run not found")
		return
	}
	if run.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Run not found")
		return
	}

	if err := h.Runs.UpdateStatus(r.Context(), id, models.RunRunning); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to resume run")
		return
	}

	// Publish a run trigger so the orchestrator re-dispatches the run
	if h.Publisher != nil {
		msg := natspkg.RunTriggerMsg{
			EngagementID: run.EngagementID,
			OrgID:        run.OrgID,
			TriggeredBy:  claims.UserID.String(),
		}
		if err := h.Publisher.Publish(r.Context(), natspkg.SubjectRunTrigger, run.OrgID, msg); err != nil {
			h.Logger.Error("publishing run trigger on resume", "error", err, "run_id", id)
		}
	}

	writeData(w, map[string]string{"status": "running"})
}
