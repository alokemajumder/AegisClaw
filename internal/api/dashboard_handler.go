package api

import (
	"net/http"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

func (h *Handler) GetCoverage(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	entries, err := h.Coverage.ListByOrgID(r.Context(), claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list coverage")
		return
	}
	if entries == nil {
		entries = []models.CoverageEntry{}
	}
	writeData(w, entries)
}

func (h *Handler) GetCoverageGaps(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	gaps, err := h.Coverage.GetGaps(r.Context(), claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list coverage gaps")
		return
	}
	if gaps == nil {
		gaps = []models.CoverageEntry{}
	}
	writeData(w, gaps)
}

func (h *Handler) DashboardSummary(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	ctx := r.Context()

	assetCount, _ := h.Assets.CountByOrgID(ctx, claims.OrgID)
	totalEngagements, activeEngagements, _ := h.Engagements.CountByOrgID(ctx, claims.OrgID)
	_, activeRuns, completedRuns, _ := h.Runs.CountByOrgID(ctx, claims.OrgID)
	totalFindings, criticalFindings, highFindings, _ := h.Findings.CountByOrgID(ctx, claims.OrgID)

	connectors, _ := h.ConnInst.ListByOrgID(ctx, claims.OrgID)
	healthyConnectors := 0
	for _, c := range connectors {
		if c.HealthStatus == "healthy" {
			healthyConnectors++
		}
	}

	writeData(w, map[string]any{
		"assets":              assetCount,
		"active_engagements":  activeEngagements,
		"total_engagements":   totalEngagements,
		"active_runs":         activeRuns,
		"completed_runs":      completedRuns,
		"total_findings":      totalFindings,
		"critical_findings":   criticalFindings,
		"high_findings":       highFindings,
		"connectors":          len(connectors),
		"healthy_connectors":  healthyConnectors,
		"kill_switch_engaged": h.IsKillSwitchEngaged(),
	})
}

func (h *Handler) DashboardActivity(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	p := models.PaginationParams{Page: 1, PerPage: 20}

	runs, _, _ := h.Runs.ListByOrgID(r.Context(), claims.OrgID, p, "")
	findings, _, _ := h.Findings.ListByOrgID(r.Context(), claims.OrgID, p, "", "")

	writeData(w, map[string]any{
		"recent_runs":     runs,
		"recent_findings": findings,
	})
}

func (h *Handler) DashboardHealth(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)

	connectors, _ := h.ConnInst.ListByOrgID(r.Context(), claims.OrgID)

	type connHealth struct {
		ID           string  `json:"id"`
		Name         string  `json:"name"`
		Category     string  `json:"category"`
		Status       string  `json:"status"`
		CheckedAt    *string `json:"checked_at,omitempty"`
	}

	var health []connHealth
	for _, c := range connectors {
		ch := connHealth{
			ID:       c.ID.String(),
			Name:     c.Name,
			Category: string(c.Category),
			Status:   c.HealthStatus,
		}
		if c.HealthCheckedAt != nil {
			t := c.HealthCheckedAt.String()
			ch.CheckedAt = &t
		}
		health = append(health, ch)
	}

	dbHealthy := h.DB.Ping(r.Context()) == nil

	writeData(w, map[string]any{
		"database":   dbHealthy,
		"connectors": health,
	})
}
