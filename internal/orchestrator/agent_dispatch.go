package orchestrator

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/alokemajumder/AegisClaw/agents/emulation"
	"github.com/alokemajumder/AegisClaw/agents/governance"
	"github.com/alokemajumder/AegisClaw/agents/improvement"
	"github.com/alokemajumder/AegisClaw/agents/validation"
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// AgentRegistry maps agent types to instantiated agents.
type AgentRegistry struct {
	agents map[agentsdk.AgentType]agentsdk.Agent
	logger *slog.Logger
}

// NewAgentRegistry creates a registry with all agents and initializes them with dependencies.
func NewAgentRegistry(logger *slog.Logger, deps agentsdk.AgentDeps) *AgentRegistry {
	reg := &AgentRegistry{
		agents: map[agentsdk.AgentType]agentsdk.Agent{
			// Governance Squad (3 agents)
			agentsdk.AgentPolicyEnforcer: governance.NewPolicyEnforcerAgent(),
			agentsdk.AgentApprovalGate:   governance.NewApprovalGateAgent(),
			agentsdk.AgentReceipt:        governance.NewReceiptAgent(),

			// Emulation Squad (3 agents)
			agentsdk.AgentPlanner:  emulation.NewPlannerAgent(),
			agentsdk.AgentExecutor: emulation.NewExecutorAgent(),
			agentsdk.AgentEvidence: emulation.NewEvidenceAgent(),

			// Validation Squad (3 agents)
			agentsdk.AgentTelemetryVerifier:  validation.NewTelemetryVerifierAgent(),
			agentsdk.AgentDetectionEvaluator: validation.NewDetectionEvaluatorAgent(),
			agentsdk.AgentResponseAutomator:  validation.NewResponseAutomatorAgent(),

			// Improvement Squad (3 agents)
			agentsdk.AgentCoverageMapper: improvement.NewCoverageMapperAgent(),
			agentsdk.AgentDrift:          improvement.NewDriftAgent(),
			agentsdk.AgentRegression:     improvement.NewRegressionAgent(),
		},
		logger: logger,
	}

	// Initialize all agents with dependencies
	ctx := context.Background()
	for agentType, agent := range reg.agents {
		if err := agent.Init(ctx, deps); err != nil {
			logger.Error("failed to initialize agent", "type", agentType, "error", err)
		}
	}

	return reg
}

// Get returns the agent for the given type.
func (r *AgentRegistry) Get(agentType agentsdk.AgentType) (agentsdk.Agent, error) {
	agent, ok := r.agents[agentType]
	if !ok {
		return nil, fmt.Errorf("unknown agent type: %s", agentType)
	}
	return agent, nil
}

// GetByString returns agent by string name.
func (r *AgentRegistry) GetByString(agentType string) (agentsdk.Agent, error) {
	return r.Get(agentsdk.AgentType(agentType))
}
