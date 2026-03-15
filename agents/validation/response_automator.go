package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/internal/connector"
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
)

// ResponseAutomatorAgent creates tickets, sends notifications, and queues retests.
type ResponseAutomatorAgent struct {
	logger       *slog.Logger
	deps         agentsdk.AgentDeps
	connectorSvc *connector.Service
}

func NewResponseAutomatorAgent() *ResponseAutomatorAgent {
	return &ResponseAutomatorAgent{}
}

func (a *ResponseAutomatorAgent) Name() agentsdk.AgentType { return agentsdk.AgentResponseAutomator }
func (a *ResponseAutomatorAgent) Squad() agentsdk.Squad    { return agentsdk.SquadValidation }

func (a *ResponseAutomatorAgent) Init(_ context.Context, deps agentsdk.AgentDeps) error {
	a.deps = deps
	if l, ok := deps.Logger.(*slog.Logger); ok {
		a.logger = l
	} else {
		a.logger = slog.Default()
	}

	if svc, ok := deps.ConnectorSvc.(*connector.Service); ok {
		a.connectorSvc = svc
		a.logger.Info("response automator connected to connector service")
	} else {
		a.logger.Warn("response automator has no connector service")
	}

	a.logger.Info("response automator agent initialized")
	return nil
}

func (a *ResponseAutomatorAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	a.logger.Info("response automator processing findings",
		"task_id", task.ID,
		"run_id", task.RunID,
	)

	var inputs map[string]any
	if task.Inputs != nil {
		_ = json.Unmarshal(task.Inputs, &inputs)
	}

	ticketsCreated := 0
	retestsQueued := 0
	notificationsSent := 0

	// Create tickets for findings via ITSM connector
	if a.connectorSvc != nil {
		if itsmID, ok := inputs["itsm_connector_id"].(string); ok {
			if id, err := uuid.Parse(itsmID); err == nil {
				// Parse findings from inputs
				if findingsRaw, ok := inputs["findings"].([]any); ok {
					for _, fRaw := range findingsRaw {
						if f, ok := fRaw.(map[string]any); ok {
							title, _ := f["title"].(string)
							desc, _ := f["description"].(string)
							severity, _ := f["severity"].(string)

							// Sanitize inputs to prevent injection into ITSM systems.
							// Truncate to prevent oversized payloads and strip control chars.
							title = sanitizeTicketField(title, 200)
							desc = sanitizeTicketField(desc, 4000)
							severity = sanitizeTicketField(severity, 20)

							ticket := connectorsdk.TicketRequest{
								Title:       fmt.Sprintf("[AegisClaw] %s", title),
								Description: desc,
								Priority:    severity,
								Labels:      []string{"aegisclaw", "security-validation"},
							}

							result, err := a.connectorSvc.CreateTicket(ctx, id, ticket)
							if err != nil {
								a.logger.Error("failed to create ticket", "error", err, "title", title)
							} else {
								ticketsCreated++
								a.logger.Info("ticket created", "ticket_id", result.TicketID, "url", result.TicketURL)
							}
						}
					}
				}
			}
		}

		// Send notifications via notification connectors
		if notifID, ok := inputs["notification_connector_id"].(string); ok {
			if id, err := uuid.Parse(notifID); err == nil {
				notif := connectorsdk.NotificationRequest{
					Title:   fmt.Sprintf("AegisClaw Run %s Complete", task.RunID.String()[:8]),
					Message: fmt.Sprintf("Run completed with %d tickets created", ticketsCreated),
					Severity: "info",
					Metadata: map[string]string{
						"run_id":          task.RunID.String(),
						"tickets_created": fmt.Sprintf("%d", ticketsCreated),
					},
				}

				if err := a.connectorSvc.SendNotification(ctx, id, notif); err != nil {
					a.logger.Error("failed to send notification", "error", err)
				} else {
					notificationsSent++
				}
			}
		}
	}

	outputs, _ := json.Marshal(map[string]any{
		"tickets_created":    ticketsCreated,
		"retests_queued":     retestsQueued,
		"notifications_sent": notificationsSent,
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *ResponseAutomatorAgent) Shutdown(_ context.Context) error {
	a.logger.Info("response automator agent shutting down")
	return nil
}

// sanitizeTicketField strips control characters and truncates to maxLen to
// prevent injection attacks against ITSM ticket systems.
func sanitizeTicketField(s string, maxLen int) string {
	// Remove control characters (except newline/tab for descriptions)
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if len(cleaned) > maxLen {
		cleaned = cleaned[:maxLen]
	}
	return cleaned
}
