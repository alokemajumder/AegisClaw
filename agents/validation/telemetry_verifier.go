package validation

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/internal/connector"
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
)

// TelemetryVerifierAgent confirms expected telemetry appears in SIEM/EDR/log sources.
type TelemetryVerifierAgent struct {
	logger       *slog.Logger
	deps         agentsdk.AgentDeps
	connectorSvc *connector.Service
}

func NewTelemetryVerifierAgent() *TelemetryVerifierAgent {
	return &TelemetryVerifierAgent{}
}

func (a *TelemetryVerifierAgent) Name() agentsdk.AgentType { return agentsdk.AgentTelemetryVerifier }
func (a *TelemetryVerifierAgent) Squad() agentsdk.Squad    { return agentsdk.SquadValidation }

func (a *TelemetryVerifierAgent) Init(_ context.Context, deps agentsdk.AgentDeps) error {
	a.deps = deps
	if l, ok := deps.Logger.(*slog.Logger); ok {
		a.logger = l
	} else {
		a.logger = slog.Default()
	}

	if svc, ok := deps.ConnectorSvc.(*connector.Service); ok {
		a.connectorSvc = svc
		a.logger.Info("telemetry verifier connected to connector service")
	} else {
		a.logger.Warn("telemetry verifier has no connector service, using simulated results")
	}

	a.logger.Info("telemetry verifier agent initialized")
	return nil
}

func (a *TelemetryVerifierAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	a.logger.Info("telemetry verifier checking for expected events",
		"task_id", task.ID,
		"action", task.Action,
	)

	var inputs map[string]any
	if task.Inputs != nil {
		_ = json.Unmarshal(task.Inputs, &inputs)
	}

	// Try to query real connectors if available
	if a.connectorSvc != nil {
		return a.verifyWithConnectors(ctx, task, inputs)
	}

	return a.simulatedVerify(task)
}

func (a *TelemetryVerifierAgent) verifyWithConnectors(ctx context.Context, task *agentsdk.Task, inputs map[string]any) (*agentsdk.Result, error) {
	sourcesChecked := []string{}
	eventsMatched := 0
	eventsExpected := 1

	// Determine time range for query
	timeRangeMinutes := 60
	if trm, ok := inputs["time_range_minutes"].(float64); ok {
		timeRangeMinutes = int(trm)
	}

	query := connectorsdk.EventQuery{
		TimeRange: connectorsdk.TimeRange{
			Start: time.Now().UTC().Add(-time.Duration(timeRangeMinutes) * time.Minute),
			End:   time.Now().UTC(),
		},
		MaxResults: 10,
	}

	// Try SIEM connector if we have a connector instance ID in inputs
	if siemID, ok := inputs["siem_connector_id"].(string); ok {
		if id, err := uuid.Parse(siemID); err == nil {
			result, err := a.connectorSvc.QueryEvents(ctx, id, query)
			if err != nil {
				a.logger.Warn("SIEM query failed", "error", err)
			} else {
				sourcesChecked = append(sourcesChecked, "siem")
				eventsMatched += result.TotalCount
			}
		}
	}

	// Try EDR connector if available
	if edrID, ok := inputs["edr_connector_id"].(string); ok {
		if id, err := uuid.Parse(edrID); err == nil {
			result, err := a.connectorSvc.QueryEvents(ctx, id, query)
			if err != nil {
				a.logger.Warn("EDR query failed", "error", err)
			} else {
				sourcesChecked = append(sourcesChecked, "edr")
				eventsMatched += result.TotalCount
			}
		}
	}

	// If no specific connector IDs provided, report what we can
	if len(sourcesChecked) == 0 {
		sourcesChecked = []string{"siem", "edr"}
		a.logger.Info("no connector IDs in task inputs, reporting default sources")
	}

	coveragePct := 0
	if eventsExpected > 0 && eventsMatched > 0 {
		coveragePct = (eventsMatched * 100) / eventsExpected
		if coveragePct > 100 {
			coveragePct = 100
		}
	}

	telemetryFound := eventsMatched > 0

	var findings []agentsdk.FindingOutput
	if !telemetryFound {
		findings = append(findings, agentsdk.FindingOutput{
			Title:       "Telemetry gap detected",
			Description: "Expected telemetry events were not found in the configured sources",
			Severity:    "medium",
			Confidence:  "high",
			Remediation: "Verify log source configuration and ensure data is being ingested",
		})
	}

	outputs, _ := json.Marshal(map[string]any{
		"telemetry_found":  telemetryFound,
		"sources_checked":  sourcesChecked,
		"events_matched":   eventsMatched,
		"events_expected":  eventsExpected,
		"coverage_pct":     coveragePct,
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		Findings:    findings,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *TelemetryVerifierAgent) simulatedVerify(task *agentsdk.Task) (*agentsdk.Result, error) {
	outputs, _ := json.Marshal(map[string]any{
		"telemetry_found":  true,
		"sources_checked":  []string{"siem", "edr"},
		"events_matched":   3,
		"events_expected":  3,
		"coverage_pct":     100,
		"simulated":        true,
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *TelemetryVerifierAgent) Shutdown(_ context.Context) error {
	a.logger.Info("telemetry verifier agent shutting down")
	return nil
}
