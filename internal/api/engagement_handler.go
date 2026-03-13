package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/internal/models"
	natspkg "github.com/alokemajumder/AegisClaw/internal/nats"
)

type createEngagementRequest struct {
	Name              string      `json:"name"`
	Description       *string     `json:"description,omitempty"`
	TargetAllowlist   []uuid.UUID `json:"target_allowlist"`
	TargetExclusions  []uuid.UUID `json:"target_exclusions"`
	AllowedTiers      []int       `json:"allowed_tiers"`
	AllowedTechniques []string    `json:"allowed_techniques"`
	ScheduleCron      *string     `json:"schedule_cron,omitempty"`
	RunWindowStart    *string     `json:"run_window_start,omitempty"`
	RunWindowEnd      *string     `json:"run_window_end,omitempty"`
	RateLimit         int         `json:"rate_limit"`
	ConcurrencyCap    int         `json:"concurrency_cap"`
	ConnectorIDs      []uuid.UUID `json:"connector_ids"`
}

func (h *Handler) ListEngagements(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	p := parsePagination(r)

	engagements, total, err := h.Engagements.ListByOrgID(r.Context(), claims.OrgID, p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list engagements")
		return
	}
	if engagements == nil {
		engagements = []models.Engagement{}
	}
	writeDataWithMeta(w, engagements, total, p.Page, p.PerPage)
}

func (h *Handler) CreateEngagement(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)

	var req createEngagementRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}
	if err := validateRequired(map[string]string{"name": req.Name}); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	if req.AllowedTiers == nil {
		req.AllowedTiers = []int{0, 1}
	}
	if req.RateLimit == 0 {
		req.RateLimit = 10
	}
	if req.ConcurrencyCap == 0 {
		req.ConcurrencyCap = 5
	}

	eng := &models.Engagement{
		OrgID:             claims.OrgID,
		Name:              req.Name,
		Description:       req.Description,
		Status:            models.EngagementDraft,
		TargetAllowlist:   req.TargetAllowlist,
		TargetExclusions:  req.TargetExclusions,
		AllowedTiers:      req.AllowedTiers,
		AllowedTechniques: req.AllowedTechniques,
		ScheduleCron:      req.ScheduleCron,
		RunWindowStart:    req.RunWindowStart,
		RunWindowEnd:      req.RunWindowEnd,
		BlackoutPeriods:   json.RawMessage(`[]`),
		RateLimit:         req.RateLimit,
		ConcurrencyCap:    req.ConcurrencyCap,
		ConnectorIDs:      req.ConnectorIDs,
		CreatedBy:         &claims.UserID,
	}
	if eng.TargetAllowlist == nil {
		eng.TargetAllowlist = []uuid.UUID{}
	}
	if eng.TargetExclusions == nil {
		eng.TargetExclusions = []uuid.UUID{}
	}
	if eng.AllowedTechniques == nil {
		eng.AllowedTechniques = []string{}
	}
	if eng.ConnectorIDs == nil {
		eng.ConnectorIDs = []uuid.UUID{}
	}

	if err := h.Engagements.Create(r.Context(), eng); err != nil {
		h.Logger.Error("creating engagement", "error", err)
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to create engagement")
		return
	}

	resID := eng.ID.String()
	h.audit(r.Context(), r, claims, "engagement.create", "engagement", &resID, nil)

	writeJSON(w, http.StatusCreated, models.APIResponse{Data: eng})
}

func (h *Handler) GetEngagement(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "engagementID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid engagement ID")
		return
	}

	eng, err := h.Engagements.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Engagement not found")
		return
	}
	if eng.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Engagement not found")
		return
	}
	writeData(w, eng)
}

func (h *Handler) UpdateEngagement(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "engagementID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid engagement ID")
		return
	}

	eng, err := h.Engagements.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Engagement not found")
		return
	}
	if eng.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Engagement not found")
		return
	}

	var req createEngagementRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if req.Name != "" {
		eng.Name = req.Name
	}
	if req.Description != nil {
		eng.Description = req.Description
	}
	if req.TargetAllowlist != nil {
		eng.TargetAllowlist = req.TargetAllowlist
	}
	if req.TargetExclusions != nil {
		eng.TargetExclusions = req.TargetExclusions
	}
	if req.AllowedTiers != nil {
		eng.AllowedTiers = req.AllowedTiers
	}
	if req.AllowedTechniques != nil {
		eng.AllowedTechniques = req.AllowedTechniques
	}
	if req.ScheduleCron != nil {
		eng.ScheduleCron = req.ScheduleCron
	}
	if req.RateLimit > 0 {
		eng.RateLimit = req.RateLimit
	}
	if req.ConcurrencyCap > 0 {
		eng.ConcurrencyCap = req.ConcurrencyCap
	}
	if req.ConnectorIDs != nil {
		eng.ConnectorIDs = req.ConnectorIDs
	}

	if err := h.Engagements.Update(r.Context(), eng); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to update engagement")
		return
	}
	writeData(w, eng)
}

func (h *Handler) DeleteEngagement(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "engagementID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid engagement ID")
		return
	}

	eng, err := h.Engagements.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Engagement not found")
		return
	}
	if eng.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Engagement not found")
		return
	}

	if err := h.Engagements.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Engagement not found")
		return
	}
	writeData(w, map[string]string{"status": "deleted"})
}

func (h *Handler) ActivateEngagement(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "engagementID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid engagement ID")
		return
	}

	eng, err := h.Engagements.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Engagement not found")
		return
	}
	if eng.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Engagement not found")
		return
	}

	if err := h.Engagements.UpdateStatus(r.Context(), id, models.EngagementActive); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to activate engagement")
		return
	}

	resID := id.String()
	h.audit(r.Context(), r, claims, "engagement.activate", "engagement", &resID, nil)

	writeData(w, map[string]string{"status": "active"})
}

func (h *Handler) PauseEngagement(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "engagementID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid engagement ID")
		return
	}

	eng, err := h.Engagements.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Engagement not found")
		return
	}
	if eng.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Engagement not found")
		return
	}

	if err := h.Engagements.UpdateStatus(r.Context(), id, models.EngagementPaused); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to pause engagement")
		return
	}

	resID := id.String()
	h.audit(r.Context(), r, claims, "engagement.pause", "engagement", &resID, nil)

	writeData(w, map[string]string{"status": "paused"})
}

func (h *Handler) ListEngagementRuns(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "engagementID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid engagement ID")
		return
	}

	// Verify the engagement belongs to the requesting org before listing its runs
	eng, err := h.Engagements.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Engagement not found")
		return
	}
	if eng.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Engagement not found")
		return
	}

	p := parsePagination(r)

	runs, total, err := h.Runs.ListByEngagementID(r.Context(), id, p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list runs")
		return
	}
	if runs == nil {
		runs = []models.Run{}
	}
	writeDataWithMeta(w, runs, total, p.Page, p.PerPage)
}

func (h *Handler) TriggerRun(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "engagementID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid engagement ID")
		return
	}

	if h.IsKillSwitchEngaged() {
		writeError(w, http.StatusServiceUnavailable, "kill_switch", "Kill switch is engaged — no new runs allowed")
		return
	}

	eng, err := h.Engagements.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Engagement not found")
		return
	}
	if eng.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Engagement not found")
		return
	}

	if eng.Status != models.EngagementActive && eng.Status != models.EngagementDraft {
		writeError(w, http.StatusConflict, "invalid_status", "Engagement must be active or draft to trigger a run")
		return
	}

	// Publish run trigger via NATS
	msg := natspkg.RunTriggerMsg{
		EngagementID: eng.ID,
		OrgID:        eng.OrgID,
		TriggeredBy:  claims.UserID.String(),
	}

	if h.Publisher != nil {
		if err := h.Publisher.Publish(r.Context(), natspkg.SubjectRunTrigger, eng.OrgID, msg); err != nil {
			h.Logger.Error("publishing run trigger", "error", err)
		}
	}

	// Create run record
	maxTier := 0
	for _, t := range eng.AllowedTiers {
		if t > maxTier {
			maxTier = t
		}
	}
	run := &models.Run{
		EngagementID: eng.ID,
		OrgID:        eng.OrgID,
		Status:       models.RunQueued,
		Tier:         maxTier,
		Metadata:     json.RawMessage(`{}`),
	}
	if err := h.Runs.Create(r.Context(), run); err != nil {
		h.Logger.Error("creating run", "error", err)
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to create run")
		return
	}

	writeJSON(w, http.StatusCreated, models.APIResponse{Data: run})
}
