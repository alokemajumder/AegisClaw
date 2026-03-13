package playbook

// Playbook represents a security validation playbook.
type Playbook struct {
	ID          string         `yaml:"id" json:"id"`
	Name        string         `yaml:"name" json:"name"`
	Description string         `yaml:"description" json:"description"`
	Tier        int            `yaml:"tier" json:"tier"`
	TechniqueID string         `yaml:"technique_id" json:"technique_id"`
	AssetTypes  []string       `yaml:"asset_types" json:"asset_types"`
	Tags        []string       `yaml:"tags,omitempty" json:"tags,omitempty"`
	Steps       []PlaybookStep `yaml:"steps" json:"steps"`
}

// PlaybookStep is a single action within a playbook.
type PlaybookStep struct {
	Name              string              `yaml:"name" json:"name"`
	AgentType         string              `yaml:"agent_type" json:"agent_type"`
	Action            string              `yaml:"action" json:"action"`
	Description       string              `yaml:"description" json:"description"`
	Inputs            map[string]any      `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	ExpectedTelemetry *ExpectedTelemetry  `yaml:"expected_telemetry,omitempty" json:"expected_telemetry,omitempty"`
	Cleanup           *CleanupAction      `yaml:"cleanup,omitempty" json:"cleanup,omitempty"`
	TimeoutSeconds    int                 `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty"`
}

// ExpectedTelemetry defines what telemetry should appear after a step.
type ExpectedTelemetry struct {
	Source       string            `yaml:"source" json:"source"`
	EventType    string            `yaml:"event_type" json:"event_type"`
	SearchQuery  string            `yaml:"search_query,omitempty" json:"search_query,omitempty"`
	MaxLatencySec int              `yaml:"max_latency_sec" json:"max_latency_sec"`
	Indicators   map[string]string `yaml:"indicators,omitempty" json:"indicators,omitempty"`
}

// CleanupAction defines how to revert a step's effects.
type CleanupAction struct {
	Action      string         `yaml:"action" json:"action"`
	Description string         `yaml:"description" json:"description"`
	Inputs      map[string]any `yaml:"inputs,omitempty" json:"inputs,omitempty"`
}
