package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/alokemajumder/AegisClaw/internal/models"
	"github.com/alokemajumder/AegisClaw/internal/reporting"
)

func (h *Handler) ListReports(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	p := parsePagination(r)

	reports, total, err := h.Reports.ListByOrgID(r.Context(), claims.OrgID, p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list reports")
		return
	}
	if reports == nil {
		reports = []models.Report{}
	}
	writeDataWithMeta(w, reports, total, p.Page, p.PerPage)
}

func (h *Handler) GenerateReport(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)

	var req struct {
		Title      string `json:"title"`
		ReportType string `json:"report_type"`
		Format     string `json:"format"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if req.ReportType == "" {
		req.ReportType = "executive"
	}
	if req.Format == "" {
		req.Format = "markdown"
	}
	if req.Title == "" {
		req.Title = "Security Validation Report"
	}

	if h.ReportSvc != nil {
		cfg := reporting.ReportConfig{
			Title:  req.Title,
			Type:   reporting.ReportType(req.ReportType),
			Format: req.Format,
		}

		// Create the report record synchronously with "generating" status
		report, err := h.ReportSvc.CreatePending(r.Context(), claims.OrgID, cfg, &claims.UserID)
		if err != nil {
			h.Logger.Error("creating report record", "error", err)
			writeError(w, http.StatusInternalServerError, "generation_error", "Failed to create report")
			return
		}

		// Run the actual generation asynchronously so we don't block the HTTP request
		go func() {
			if err := h.ReportSvc.GenerateAsync(context.Background(), report, claims.OrgID, cfg); err != nil {
				h.Logger.Error("async report generation failed", "report_id", report.ID, "error", err)
			}
		}()

		// Return 202 Accepted with the report ID — client can poll status
		writeJSON(w, http.StatusAccepted, models.APIResponse{Data: report})
		return
	}

	// Report generation service not wired — return 503
	writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Report generation service not configured")
}

func (h *Handler) GetReport(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "reportID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid report ID")
		return
	}

	report, err := h.Reports.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Report not found")
		return
	}
	if report.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Report not found")
		return
	}
	writeData(w, report)
}

func (h *Handler) DownloadReport(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "reportID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid report ID")
		return
	}

	report, err := h.Reports.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Report not found")
		return
	}
	if report.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Report not found")
		return
	}

	if report.Status != "completed" {
		writeError(w, http.StatusConflict, "not_ready", "Report is still being generated")
		return
	}

	// Try downloading from evidence store
	if h.EvidenceStore != nil && report.StoragePath != nil && *report.StoragePath != "" {
		ext := "md"
		if report.Format == "json" {
			ext = "json"
		}
		fileName := report.ID.String() + "." + ext
		data, err := h.EvidenceStore.Download(r.Context(), "reports", *report.StoragePath, fileName)
		if err == nil {
			contentType := "text/markdown"
			if report.Format == "json" {
				contentType = "application/json"
			}
			w.Header().Set("Content-Type", contentType)
			w.Header().Set("Content-Disposition", "attachment; filename=report-"+report.ID.String()+"."+ext)
			w.Write(data)
			return
		}
		h.Logger.Warn("failed to download report from storage", "error", err)
	}

	// Fallback: return report metadata
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=report-"+report.ID.String()+".json")
	json.NewEncoder(w).Encode(report)
}
