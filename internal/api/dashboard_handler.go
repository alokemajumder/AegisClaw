package api

import (
	"net/http"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

func (h *Handler) GetCoverage(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	p := parsePagination(r)
	entries, total, err := h.Coverage.ListByOrgIDPaginated(r.Context(), claims.OrgID, p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list coverage")
		return
	}
	if entries == nil {
		entries = []models.CoverageEntry{}
	}
	writeDataWithMeta(w, entries, total, p.Page, p.PerPage)
}

func (h *Handler) GetCoverageGaps(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	p := parsePagination(r)
	gaps, total, err := h.Coverage.GetGapsPaginated(r.Context(), claims.OrgID, p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list coverage gaps")
		return
	}
	if gaps == nil {
		gaps = []models.CoverageEntry{}
	}
	writeDataWithMeta(w, gaps, total, p.Page, p.PerPage)
}

func (h *Handler) DashboardSummary(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	ctx := r.Context()

	var dbErrors int

	assetCount, err := h.Assets.CountByOrgID(ctx, claims.OrgID)
	if err != nil {
		h.Logger.Error("dashboard: counting assets", "error", err)
		dbErrors++
	}
	totalEngagements, activeEngagements, err := h.Engagements.CountByOrgID(ctx, claims.OrgID)
	if err != nil {
		h.Logger.Error("dashboard: counting engagements", "error", err)
		dbErrors++
	}
	_, activeRuns, completedRuns, err := h.Runs.CountByOrgID(ctx, claims.OrgID)
	if err != nil {
		h.Logger.Error("dashboard: counting runs", "error", err)
		dbErrors++
	}
	totalFindings, criticalFindings, highFindings, mediumFindings, lowFindings, err := h.Findings.CountByOrgIDFull(ctx, claims.OrgID)
	if err != nil {
		h.Logger.Error("dashboard: counting findings", "error", err)
		dbErrors++
	}
	coverageEntries, coverageGaps, err := h.Coverage.CountByOrgID(ctx, claims.OrgID)
	if err != nil {
		h.Logger.Error("dashboard: counting coverage", "error", err)
		dbErrors++
	}

	connectors, err := h.ConnInst.ListByOrgID(ctx, claims.OrgID)
	if err != nil {
		h.Logger.Error("dashboard: listing connectors", "error", err)
		dbErrors++
	}
	healthyConnectors := 0
	for _, c := range connectors {
		if c.HealthStatus == "healthy" {
			healthyConnectors++
		}
	}

	result := map[string]any{
		"total_assets":        assetCount,
		"active_engagements":  activeEngagements,
		"total_engagements":   totalEngagements,
		"running_runs":        activeRuns,
		"completed_runs":      completedRuns,
		"total_findings":      totalFindings,
		"critical_findings":   criticalFindings,
		"high_findings":       highFindings,
		"medium_findings":     mediumFindings,
		"low_findings":        lowFindings,
		"coverage_entries":    coverageEntries,
		"coverage_gaps":       coverageGaps,
		"connectors":          len(connectors),
		"healthy_connectors":  healthyConnectors,
		"kill_switch_engaged": h.IsKillSwitchEngaged(),
	}
	if dbErrors > 0 {
		result["partial"] = true
	}
	writeData(w, result)
}

func (h *Handler) DashboardActivity(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	p := models.PaginationParams{Page: 1, PerPage: 20}

	runs, _, err := h.Runs.ListByOrgID(r.Context(), claims.OrgID, p, "")
	if err != nil {
		h.Logger.Error("dashboard activity: listing runs", "error", err)
		runs = nil
	}
	findings, _, err := h.Findings.ListByOrgID(r.Context(), claims.OrgID, p, "", "")
	if err != nil {
		h.Logger.Error("dashboard activity: listing findings", "error", err)
		findings = nil
	}

	writeData(w, map[string]any{
		"recent_runs":     runs,
		"recent_findings": findings,
	})
}

func (h *Handler) DashboardHealth(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)

	connectors, err := h.ConnInst.ListByOrgID(r.Context(), claims.OrgID)
	if err != nil {
		h.Logger.Error("dashboard health: listing connectors", "error", err)
	}

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

	dbStatus := "ok"
	if err := h.DB.Ping(r.Context()); err != nil {
		dbStatus = "error"
	}

	natsStatus := "unknown"
	if h.NATSClient != nil {
		if h.NATSClient.HealthCheck() == nil {
			natsStatus = "ok"
		} else {
			natsStatus = "error"
		}
	}

	writeData(w, map[string]any{
		"database":            dbStatus,
		"nats":                natsStatus,
		"kill_switch_engaged": h.IsKillSwitchEngaged(),
		"connectors":          health,
	})
}
