package finding

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/internal/models"
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// Valid state transitions for findings.
var validTransitions = map[models.FindingStatus][]models.FindingStatus{
	models.FindingObserved:    {models.FindingNeedsReview, models.FindingConfirmed, models.FindingAcceptedRisk},
	models.FindingNeedsReview: {models.FindingConfirmed, models.FindingAcceptedRisk},
	models.FindingConfirmed:   {models.FindingTicketed, models.FindingAcceptedRisk},
	models.FindingTicketed:    {models.FindingFixed},
	models.FindingFixed:       {models.FindingRetested},
	models.FindingRetested:    {models.FindingClosed, models.FindingConfirmed},
}

// Service manages finding lifecycle, deduplication, and ticketing.
type Service struct {
	findings *repository.FindingRepo
	logger   *slog.Logger
}

// NewService creates a new FindingService.
func NewService(findings *repository.FindingRepo, logger *slog.Logger) *Service {
	return &Service{findings: findings, logger: logger}
}

// CreateFromAgentResult creates a finding from an agent result output.
func (s *Service) CreateFromAgentResult(ctx context.Context, orgID uuid.UUID, runID, stepID *uuid.UUID, output agentsdk.FindingOutput) (*models.Finding, error) {
	desc := output.Description
	rem := output.Remediation

	f := &models.Finding{
		OrgID:        orgID,
		RunID:        runID,
		RunStepID:    stepID,
		Title:        output.Title,
		Description:  &desc,
		Severity:     models.Severity(output.Severity),
		Confidence:   models.Confidence(output.Confidence),
		Status:       models.FindingObserved,
		TechniqueIDs: output.TechniqueIDs,
		EvidenceIDs:  output.EvidenceIDs,
		Remediation:  &rem,
	}
	if f.TechniqueIDs == nil {
		f.TechniqueIDs = []string{}
	}
	if f.EvidenceIDs == nil {
		f.EvidenceIDs = []string{}
	}
	if f.AffectedAssets == nil {
		f.AffectedAssets = []uuid.UUID{}
	}

	// Deduplicate
	clusterID := s.computeClusterID(orgID, output.Title, output.TechniqueIDs, output.AffectedAssets)
	f.ClusterID = &clusterID

	if err := s.findings.Create(ctx, f); err != nil {
		return nil, fmt.Errorf("creating finding: %w", err)
	}

	s.logger.Info("finding created",
		"id", f.ID,
		"title", f.Title,
		"severity", f.Severity,
		"cluster_id", clusterID,
	)

	return f, nil
}

// computeClusterID generates a deterministic UUID from finding content for dedup.
func (s *Service) computeClusterID(orgID uuid.UUID, title string, techniques, assets []string) uuid.UUID {
	sort.Strings(techniques)
	sort.Strings(assets)

	data := fmt.Sprintf("%s|%s|%s|%s",
		orgID.String(),
		strings.ToLower(title),
		strings.Join(techniques, ","),
		strings.Join(assets, ","),
	)
	hash := sha256.Sum256([]byte(data))
	// Use first 16 bytes of SHA256 as UUID
	return uuid.UUID(hash[:16])
}

// TransitionStatus validates and applies a status transition.
func (s *Service) TransitionStatus(ctx context.Context, findingID uuid.UUID, newStatus models.FindingStatus) error {
	f, err := s.findings.GetByID(ctx, findingID)
	if err != nil {
		return err
	}

	valid := validTransitions[f.Status]
	allowed := false
	for _, v := range valid {
		if v == newStatus {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("invalid transition from %s to %s", f.Status, newStatus)
	}

	return s.findings.UpdateStatus(ctx, findingID, newStatus)
}

// CreateTicket creates an ITSM ticket for a finding.
func (s *Service) CreateTicket(ctx context.Context, findingID uuid.UUID, ticketID string, connectorID uuid.UUID) error {
	return s.findings.SetTicket(ctx, findingID, ticketID, connectorID)
}
