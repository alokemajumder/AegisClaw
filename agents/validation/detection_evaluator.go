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

// DetectionEvaluatorAgent validates alerts fired, assesses latency and quality.
type DetectionEvaluatorAgent struct {
	logger       *slog.Logger
	deps         agentsdk.AgentDeps
	connectorSvc *connector.Service
}

func NewDetectionEvaluatorAgent() *DetectionEvaluatorAgent {
	return &DetectionEvaluatorAgent{}
}

func (a *DetectionEvaluatorAgent) Name() agentsdk.AgentType { return agentsdk.AgentDetectionEvaluator }
func (a *DetectionEvaluatorAgent) Squad() agentsdk.Squad    { return agentsdk.SquadValidation }

func (a *DetectionEvaluatorAgent) Init(_ context.Context, deps agentsdk.AgentDeps) error {
	a.deps = deps
	if l, ok := deps.Logger.(*slog.Logger); ok {
		a.logger = l
	} else {
		a.logger = slog.Default()
	}

	if svc, ok := deps.ConnectorSvc.(*connector.Service); ok {
		a.connectorSvc = svc
		a.logger.Info("detection evaluator connected to connector service")
	} else {
		a.logger.Warn("detection evaluator has no connector service, using simulated results")
	}

	a.logger.Info("detection evaluator agent initialized")
	return nil
}

func (a *DetectionEvaluatorAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	a.logger.Info("detection evaluator checking alert generation",
		"task_id", task.ID,
		"action", task.Action,
	)

	var inputs map[string]any
	if task.Inputs != nil {
		_ = json.Unmarshal(task.Inputs, &inputs)
	}

	if a.connectorSvc != nil {
		return a.evaluateWithConnectors(ctx, task, inputs)
	}

	return a.simulatedEvaluate(task)
}

func (a *DetectionEvaluatorAgent) evaluateWithConnectors(ctx context.Context, task *agentsdk.Task, inputs map[string]any) (*agentsdk.Result, error) {
	alertsFound := 0
	alertsExpected := 1
	var avgLatencyMs int64

	// Determine time window for alert search
	maxLatencySec := 120
	if mls, ok := inputs["max_latency_sec"].(float64); ok {
		maxLatencySec = int(mls)
	}

	query := connectorsdk.EventQuery{
		TimeRange: connectorsdk.TimeRange{
			Start: time.Now().UTC().Add(-time.Duration(maxLatencySec) * time.Second),
			End:   time.Now().UTC(),
		},
		Filters:    map[string]string{"category": "Alert"},
		MaxResults: 50,
	}

	// Query EDR for alerts if connector ID provided
	if edrID, ok := inputs["edr_connector_id"].(string); ok {
		if id, err := uuid.Parse(edrID); err == nil {
			startTime := time.Now()
			result, err := a.connectorSvc.QueryEvents(ctx, id, query)
			queryLatency := time.Since(startTime)

			if err != nil {
				a.logger.Warn("EDR alert query failed", "error", err)
			} else {
				alertsFound += result.TotalCount
				avgLatencyMs = queryLatency.Milliseconds()
			}
		}
	}

	// Query SIEM for correlated alerts
	if siemID, ok := inputs["siem_connector_id"].(string); ok {
		if id, err := uuid.Parse(siemID); err == nil {
			result, err := a.connectorSvc.QueryEvents(ctx, id, query)
			if err != nil {
				a.logger.Warn("SIEM alert query failed", "error", err)
			} else {
				alertsFound += result.TotalCount
			}
		}
	}

	// Assess quality
	qualityScore := 0.0
	suppressionGaps := 0
	if alertsFound >= alertsExpected {
		qualityScore = 1.0
	} else if alertsFound > 0 {
		qualityScore = float64(alertsFound) / float64(alertsExpected)
	} else {
		suppressionGaps = alertsExpected
	}

	var findings []agentsdk.FindingOutput

	// Generate findings for detection gaps
	if suppressionGaps > 0 {
		severity := "high"
		if expectedSev, ok := inputs["expected_severity"].(string); ok && expectedSev == "low" {
			severity = "medium"
		}

		techniqueIDs := []string{}
		if tid, ok := inputs["expected_technique"].(string); ok {
			techniqueIDs = append(techniqueIDs, tid)
		}

		findings = append(findings, agentsdk.FindingOutput{
			Title:        "Detection gap: alert not generated",
			Description:  "Expected security alert was not generated within the configured time window",
			Severity:     severity,
			Confidence:   "high",
			TechniqueIDs: techniqueIDs,
			Remediation:  "Review detection rules and ensure alerts are configured for this technique",
		})
	}

	outputs, _ := json.Marshal(map[string]any{
		"alerts_found":     alertsFound,
		"alerts_expected":  alertsExpected,
		"avg_latency_ms":   avgLatencyMs,
		"suppression_gaps": suppressionGaps,
		"quality_score":    qualityScore,
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		Findings:    findings,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *DetectionEvaluatorAgent) simulatedEvaluate(task *agentsdk.Task) (*agentsdk.Result, error) {
	outputs, _ := json.Marshal(map[string]any{
		"alerts_found":            0,
		"alerts_expected":         0,
		"avg_latency_ms":          0,
		"suppression_gaps":        0,
		"quality_score":           0.0,
		"no_connector_configured": true,
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *DetectionEvaluatorAgent) Shutdown(_ context.Context) error {
	a.logger.Info("detection evaluator agent shutting down")
	return nil
}
