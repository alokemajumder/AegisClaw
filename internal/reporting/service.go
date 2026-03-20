package reporting

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/internal/evidence"
	"github.com/alokemajumder/AegisClaw/internal/models"
)

// Service generates reports and stores them.
type Service struct {
	findings *repository.FindingRepo
	runs     *repository.RunRepo
	coverage *repository.CoverageRepo
	assets   *repository.AssetRepo
	reports  *repository.ReportRepo
	store    *evidence.Store
	logger   *slog.Logger
}

// NewService creates a new ReportService.
func NewService(
	findings *repository.FindingRepo,
	runs *repository.RunRepo,
	coverage *repository.CoverageRepo,
	assets *repository.AssetRepo,
	reports *repository.ReportRepo,
	store *evidence.Store,
	logger *slog.Logger,
) *Service {
	return &Service{
		findings: findings,
		runs:     runs,
		coverage: coverage,
		assets:   assets,
		reports:  reports,
		store:    store,
		logger:   logger,
	}
}

// Generate creates a report and stores it.
func (s *Service) Generate(ctx context.Context, orgID uuid.UUID, cfg ReportConfig, userID *uuid.UUID) (*models.Report, error) {
	// Gather data
	data, err := GatherData(ctx, orgID, s.findings, s.runs, s.coverage, s.assets)
	if err != nil {
		return nil, fmt.Errorf("gathering report data: %w", err)
	}

	// Create report record
	report := &models.Report{
		OrgID:       orgID,
		Title:       cfg.Title,
		ReportType:  string(cfg.Type),
		Status:      "generating",
		Format:      cfg.Format,
		GeneratedBy: userID,
	}
	if err := s.reports.Create(ctx, report); err != nil {
		return nil, fmt.Errorf("creating report record: %w", err)
	}

	// Render
	var content []byte
	var contentType string
	switch cfg.Format {
	case "json":
		content, err = RenderJSON(cfg, data)
		contentType = "application/json"
	case "pdf":
		content, err = RenderPDF(cfg, data)
		contentType = "application/pdf"
	default:
		md := RenderMarkdown(cfg, data)
		content = []byte(md)
		contentType = "text/markdown"
	}
	if err != nil {
		_ = s.reports.UpdateStatus(ctx, report.ID, "failed", "")
		return nil, fmt.Errorf("rendering report: %w", err)
	}

	// Store in MinIO if available
	storagePath := ""
	if s.store != nil {
		artifact, err := s.store.Upload(ctx, "reports", report.ID.String()+"."+cfg.Format, contentType, content)
		if err != nil {
			s.logger.Error("storing report", "error", err)
		} else {
			storagePath = artifact.ID
		}
	}

	// Update status
	if err := s.reports.UpdateStatus(ctx, report.ID, "completed", storagePath); err != nil {
		return nil, fmt.Errorf("updating report status: %w", err)
	}

	s.logger.Info("report generated", "id", report.ID, "type", cfg.Type, "format", cfg.Format)
	return report, nil
}

// CreatePending creates a report record in "generating" status without doing any work.
// This is used for async report generation where the caller returns immediately.
func (s *Service) CreatePending(ctx context.Context, orgID uuid.UUID, cfg ReportConfig, userID *uuid.UUID) (*models.Report, error) {
	report := &models.Report{
		OrgID:       orgID,
		Title:       cfg.Title,
		ReportType:  string(cfg.Type),
		Status:      "generating",
		Format:      cfg.Format,
		GeneratedBy: userID,
	}
	if err := s.reports.Create(ctx, report); err != nil {
		return nil, fmt.Errorf("creating report record: %w", err)
	}
	return report, nil
}

// GenerateAsync performs report generation for an already-created report record.
// It gathers data, renders the report, stores it, and updates the status.
// Intended to be called in a goroutine after CreatePending.
func (s *Service) GenerateAsync(ctx context.Context, report *models.Report, orgID uuid.UUID, cfg ReportConfig) error {
	data, err := GatherData(ctx, orgID, s.findings, s.runs, s.coverage, s.assets)
	if err != nil {
		_ = s.reports.UpdateStatus(ctx, report.ID, "failed", "")
		return fmt.Errorf("gathering report data: %w", err)
	}

	var content []byte
	var contentType string
	switch cfg.Format {
	case "json":
		content, err = RenderJSON(cfg, data)
		contentType = "application/json"
	case "pdf":
		content, err = RenderPDF(cfg, data)
		contentType = "application/pdf"
	default:
		md := RenderMarkdown(cfg, data)
		content = []byte(md)
		contentType = "text/markdown"
	}
	if err != nil {
		_ = s.reports.UpdateStatus(ctx, report.ID, "failed", "")
		return fmt.Errorf("rendering report: %w", err)
	}

	storagePath := ""
	if s.store != nil {
		artifact, uploadErr := s.store.Upload(ctx, "reports", report.ID.String()+"."+cfg.Format, contentType, content)
		if uploadErr != nil {
			s.logger.Error("storing report", "error", uploadErr)
		} else {
			storagePath = artifact.ID
		}
	}

	if err := s.reports.UpdateStatus(ctx, report.ID, "completed", storagePath); err != nil {
		return fmt.Errorf("updating report status: %w", err)
	}

	s.logger.Info("async report generated", "id", report.ID, "type", cfg.Type, "format", cfg.Format)
	return nil
}
