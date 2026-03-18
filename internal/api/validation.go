package api

import (
	"fmt"
	"net/mail"
	"strings"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

func validateEmail(email string) error {
	if _, err := mail.ParseAddress(email); err != nil {
		return fmt.Errorf("invalid email address: %s", email)
	}
	return nil
}

func validateAuthMethod(m string) error {
	switch m {
	case "api_key", "oauth2", "service_principal", "certificate", "basic", "token", "webhook":
		return nil
	}
	return fmt.Errorf("invalid auth method: %s (allowed: api_key, oauth2, service_principal, certificate, basic, token, webhook)", m)
}

func validateUserRole(r string) error {
	switch models.UserRole(r) {
	case models.RoleAdmin, models.RoleOperator, models.RoleViewer, models.RoleApprover:
		return nil
	}
	return fmt.Errorf("invalid role: %s (allowed: admin, operator, viewer, approver)", r)
}

func validateAssetType(t string) error {
	switch models.AssetType(t) {
	case models.AssetEndpoint, models.AssetServer, models.AssetApplication,
		models.AssetIdentity, models.AssetCloudAccount, models.AssetK8sCluster:
		return nil
	}
	return fmt.Errorf("invalid asset type: %s", t)
}

func validateSeverity(s string) error {
	switch models.Severity(s) {
	case models.SeverityCritical, models.SeverityHigh, models.SeverityMedium,
		models.SeverityLow, models.SeverityInformational:
		return nil
	}
	return fmt.Errorf("invalid severity: %s", s)
}

func validateFindingStatus(s string) error {
	switch models.FindingStatus(s) {
	case models.FindingObserved, models.FindingNeedsReview, models.FindingConfirmed,
		models.FindingTicketed, models.FindingFixed, models.FindingRetested,
		models.FindingClosed, models.FindingAcceptedRisk:
		return nil
	}
	return fmt.Errorf("invalid finding status: %s", s)
}

func validateRunStatus(s string) error {
	switch models.RunStatus(s) {
	case models.RunQueued, models.RunRunning, models.RunPaused, models.RunCompleted,
		models.RunFailed, models.RunCancelled, models.RunKilled:
		return nil
	}
	return fmt.Errorf("invalid run status: %s", s)
}

func validateRequired(fields map[string]string) error {
	var missing []string
	for name, value := range fields {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required fields missing: %s", strings.Join(missing, ", "))
	}
	return nil
}
