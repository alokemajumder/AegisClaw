package playbook

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLoader() *Loader {
	return NewLoader(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
}

// writeYAML writes content to a file in dir and returns the full path.
func writeYAML(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

const validPlaybookYAML = `id: pb-test-001
name: Test Playbook
description: A test playbook for unit testing
tier: 1
technique_id: T1059.001
asset_types:
  - windows-endpoint
  - linux-server
tags:
  - initial-access
steps:
  - name: Execute Command
    agent_type: red-emulator
    action: exec_command
    description: Run a test command
    inputs:
      command: whoami
    timeout_seconds: 30
  - name: Validate Detection
    agent_type: blue-validator
    action: check_telemetry
    description: Verify the command was detected
    expected_telemetry:
      source: sentinel
      event_type: process_creation
      max_latency_sec: 60
`

func TestLoadFile_ValidYAML(t *testing.T) {
	loader := newTestLoader()
	dir := t.TempDir()
	path := writeYAML(t, dir, "valid.yaml", validPlaybookYAML)

	pb, err := loader.LoadFile(path)
	require.NoError(t, err)
	require.NotNil(t, pb)

	assert.Equal(t, "pb-test-001", pb.ID)
	assert.Equal(t, "Test Playbook", pb.Name)
	assert.Equal(t, "A test playbook for unit testing", pb.Description)
	assert.Equal(t, 1, pb.Tier)
	assert.Equal(t, "T1059.001", pb.TechniqueID)
	assert.Equal(t, []string{"windows-endpoint", "linux-server"}, pb.AssetTypes)
	assert.Equal(t, []string{"initial-access"}, pb.Tags)

	require.Len(t, pb.Steps, 2)
	assert.Equal(t, "Execute Command", pb.Steps[0].Name)
	assert.Equal(t, "red-emulator", pb.Steps[0].AgentType)
	assert.Equal(t, "exec_command", pb.Steps[0].Action)
	assert.Equal(t, 30, pb.Steps[0].TimeoutSeconds)
	assert.Equal(t, "whoami", pb.Steps[0].Inputs["command"])

	assert.Equal(t, "Validate Detection", pb.Steps[1].Name)
	assert.Equal(t, "blue-validator", pb.Steps[1].AgentType)
	assert.Equal(t, "check_telemetry", pb.Steps[1].Action)
	require.NotNil(t, pb.Steps[1].ExpectedTelemetry)
	assert.Equal(t, "sentinel", pb.Steps[1].ExpectedTelemetry.Source)
	assert.Equal(t, "process_creation", pb.Steps[1].ExpectedTelemetry.EventType)
	assert.Equal(t, 60, pb.Steps[1].ExpectedTelemetry.MaxLatencySec)
}

func TestLoadFile_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "missing ID",
			yaml: `name: No ID Playbook
tier: 1
steps:
  - name: Step One
    agent_type: red-emulator
    action: do_something
`,
			wantErr: "playbook ID is required",
		},
		{
			name: "missing name",
			yaml: `id: pb-no-name
tier: 1
steps:
  - name: Step One
    agent_type: red-emulator
    action: do_something
`,
			wantErr: "playbook name is required",
		},
		{
			name: "missing steps",
			yaml: `id: pb-no-steps
name: No Steps Playbook
tier: 1
steps: []
`,
			wantErr: "playbook must have at least one step",
		},
		{
			name: "no steps key at all",
			yaml: `id: pb-no-steps-key
name: No Steps Key Playbook
tier: 1
`,
			wantErr: "playbook must have at least one step",
		},
		{
			name: "step missing agent_type",
			yaml: `id: pb-step-no-agent
name: Step Missing Agent
tier: 1
steps:
  - name: Step One
    action: do_something
`,
			wantErr: "agent_type is required",
		},
		{
			name: "step missing action",
			yaml: `id: pb-step-no-action
name: Step Missing Action
tier: 1
steps:
  - name: Step One
    agent_type: red-emulator
`,
			wantErr: "action is required",
		},
		{
			name: "step missing name",
			yaml: `id: pb-step-no-name
name: Step Missing Name
tier: 1
steps:
  - agent_type: red-emulator
    action: do_something
`,
			wantErr: "name is required",
		},
		{
			name: "invalid tier too high",
			yaml: `id: pb-bad-tier
name: Bad Tier
tier: 5
steps:
  - name: Step
    agent_type: red
    action: do
`,
			wantErr: "invalid tier 5",
		},
	}

	loader := newTestLoader()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeYAML(t, dir, "bad.yaml", tc.yaml)

			pb, err := loader.LoadFile(path)
			assert.Nil(t, pb)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestLoadFile_FileNotFound(t *testing.T) {
	loader := newTestLoader()
	pb, err := loader.LoadFile("/nonexistent/path/to/playbook.yaml")
	assert.Nil(t, pb)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading playbook file")
}

func TestLoadFile_InvalidYAMLSyntax(t *testing.T) {
	loader := newTestLoader()
	dir := t.TempDir()
	path := writeYAML(t, dir, "broken.yaml", `id: foo
name: [unterminated
`)

	pb, err := loader.LoadFile(path)
	assert.Nil(t, pb)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing playbook YAML")
}

func TestValidate_PlaybookWithoutID(t *testing.T) {
	loader := newTestLoader()
	pb := &Playbook{
		Name: "No ID",
		Tier: 1,
		Steps: []PlaybookStep{
			{Name: "s1", AgentType: "red", Action: "do"},
		},
	}
	err := loader.validate(pb)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "playbook ID is required")
}

func TestValidate_PlaybookWithoutSteps(t *testing.T) {
	loader := newTestLoader()
	pb := &Playbook{
		ID:   "pb-1",
		Name: "No Steps",
		Tier: 1,
	}
	err := loader.validate(pb)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "playbook must have at least one step")
}

func TestValidate_StepWithoutAction(t *testing.T) {
	loader := newTestLoader()
	pb := &Playbook{
		ID:   "pb-1",
		Name: "Bad Step",
		Tier: 0,
		Steps: []PlaybookStep{
			{Name: "s1", AgentType: "red"},
		},
	}
	err := loader.validate(pb)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "action is required")
}

func TestValidate_ValidPlaybook(t *testing.T) {
	loader := newTestLoader()
	pb := &Playbook{
		ID:   "pb-ok",
		Name: "Good Playbook",
		Tier: 2,
		Steps: []PlaybookStep{
			{Name: "s1", AgentType: "red-emulator", Action: "exec_cmd"},
		},
	}
	err := loader.validate(pb)
	assert.NoError(t, err)
}

func TestFilterByTier(t *testing.T) {
	playbooks := []Playbook{
		{ID: "pb-t0", Tier: 0},
		{ID: "pb-t1a", Tier: 1},
		{ID: "pb-t1b", Tier: 1},
		{ID: "pb-t2", Tier: 2},
		{ID: "pb-t3", Tier: 3},
	}

	tests := []struct {
		name         string
		allowedTiers []int
		wantIDs      []string
	}{
		{
			name:         "filter tier 0 only",
			allowedTiers: []int{0},
			wantIDs:      []string{"pb-t0"},
		},
		{
			name:         "filter tier 1 only",
			allowedTiers: []int{1},
			wantIDs:      []string{"pb-t1a", "pb-t1b"},
		},
		{
			name:         "filter tiers 1 and 2",
			allowedTiers: []int{1, 2},
			wantIDs:      []string{"pb-t1a", "pb-t1b", "pb-t2"},
		},
		{
			name:         "filter all tiers",
			allowedTiers: []int{0, 1, 2, 3},
			wantIDs:      []string{"pb-t0", "pb-t1a", "pb-t1b", "pb-t2", "pb-t3"},
		},
		{
			name:         "filter no tiers returns nil",
			allowedTiers: []int{},
			wantIDs:      nil,
		},
		{
			name:         "filter nonexistent tier",
			allowedTiers: []int{99},
			wantIDs:      nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FilterByTier(playbooks, tc.allowedTiers)
			var gotIDs []string
			for _, pb := range result {
				gotIDs = append(gotIDs, pb.ID)
			}
			assert.Equal(t, tc.wantIDs, gotIDs)
		})
	}
}

func TestFilterByAssetType(t *testing.T) {
	playbooks := []Playbook{
		{ID: "pb-win", AssetTypes: []string{"windows-endpoint"}},
		{ID: "pb-linux", AssetTypes: []string{"linux-server"}},
		{ID: "pb-multi", AssetTypes: []string{"windows-endpoint", "linux-server"}},
		{ID: "pb-wildcard", AssetTypes: []string{"*"}},
		{ID: "pb-empty", AssetTypes: nil},
	}

	tests := []struct {
		name      string
		assetType string
		wantIDs   []string
	}{
		{
			name:      "windows-endpoint matches specific and wildcard and empty",
			assetType: "windows-endpoint",
			wantIDs:   []string{"pb-win", "pb-multi", "pb-wildcard", "pb-empty"},
		},
		{
			name:      "linux-server matches specific and wildcard and empty",
			assetType: "linux-server",
			wantIDs:   []string{"pb-linux", "pb-multi", "pb-wildcard", "pb-empty"},
		},
		{
			name:      "cloud-instance matches only wildcard and empty",
			assetType: "cloud-instance",
			wantIDs:   []string{"pb-wildcard", "pb-empty"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FilterByAssetType(playbooks, tc.assetType)
			var gotIDs []string
			for _, pb := range result {
				gotIDs = append(gotIDs, pb.ID)
			}
			assert.Equal(t, tc.wantIDs, gotIDs)
		})
	}
}

func TestLoadAll_MultipleFiles(t *testing.T) {
	loader := newTestLoader()
	dir := t.TempDir()

	pb1 := `id: pb-alpha
name: Alpha Playbook
tier: 1
steps:
  - name: Step A
    agent_type: red-emulator
    action: scan
`
	pb2 := `id: pb-beta
name: Beta Playbook
tier: 2
steps:
  - name: Step B
    agent_type: blue-validator
    action: verify
`
	writeYAML(t, dir, "alpha.yaml", pb1)
	writeYAML(t, dir, "beta.yml", pb2)

	playbooks, err := loader.LoadAll(dir)
	require.NoError(t, err)
	require.Len(t, playbooks, 2)

	ids := map[string]bool{}
	for _, pb := range playbooks {
		ids[pb.ID] = true
	}
	assert.True(t, ids["pb-alpha"], "should contain pb-alpha")
	assert.True(t, ids["pb-beta"], "should contain pb-beta")
}

func TestLoadAll_SkipsInvalidFiles(t *testing.T) {
	loader := newTestLoader()
	dir := t.TempDir()

	valid := `id: pb-good
name: Good Playbook
tier: 0
steps:
  - name: Step
    agent_type: red
    action: go
`
	invalid := `id:
name: Missing ID
tier: 0
steps:
  - name: S
    agent_type: r
    action: a
`
	writeYAML(t, dir, "good.yaml", valid)
	writeYAML(t, dir, "bad.yaml", invalid)

	playbooks, err := loader.LoadAll(dir)
	require.NoError(t, err)
	require.Len(t, playbooks, 1)
	assert.Equal(t, "pb-good", playbooks[0].ID)
}

func TestLoadAll_SkipsNonYAMLFiles(t *testing.T) {
	loader := newTestLoader()
	dir := t.TempDir()

	valid := `id: pb-yaml
name: YAML Playbook
tier: 0
steps:
  - name: Step
    agent_type: red
    action: go
`
	writeYAML(t, dir, "playbook.yaml", valid)
	writeYAML(t, dir, "readme.txt", "This is not YAML")
	writeYAML(t, dir, "data.json", `{"not": "yaml"}`)

	playbooks, err := loader.LoadAll(dir)
	require.NoError(t, err)
	require.Len(t, playbooks, 1)
	assert.Equal(t, "pb-yaml", playbooks[0].ID)
}

func TestLoadAll_SkipsSchemasDirectory(t *testing.T) {
	loader := newTestLoader()
	dir := t.TempDir()

	valid := `id: pb-main
name: Main Playbook
tier: 0
steps:
  - name: Step
    agent_type: red
    action: go
`
	schemaYAML := `id: schema-thing
name: Schema
tier: 0
steps:
  - name: S
    agent_type: r
    action: a
`
	writeYAML(t, dir, "main.yaml", valid)

	schemasDir := filepath.Join(dir, "schemas")
	require.NoError(t, os.MkdirAll(schemasDir, 0755))
	writeYAML(t, schemasDir, "schema.yaml", schemaYAML)

	playbooks, err := loader.LoadAll(dir)
	require.NoError(t, err)
	require.Len(t, playbooks, 1)
	assert.Equal(t, "pb-main", playbooks[0].ID)
}

func TestLoadAll_EmptyDirectory(t *testing.T) {
	loader := newTestLoader()
	dir := t.TempDir()

	playbooks, err := loader.LoadAll(dir)
	require.NoError(t, err)
	assert.Empty(t, playbooks)
}

func TestLoadAll_NonexistentDirectory(t *testing.T) {
	loader := newTestLoader()
	_, err := loader.LoadAll("/nonexistent/directory/path")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "walking playbook directory")
}

func TestLoadAll_NestedDirectories(t *testing.T) {
	loader := newTestLoader()
	dir := t.TempDir()

	pb1 := `id: pb-root
name: Root Playbook
tier: 0
steps:
  - name: Step
    agent_type: red
    action: go
`
	pb2 := `id: pb-nested
name: Nested Playbook
tier: 1
steps:
  - name: Step
    agent_type: blue
    action: check
`
	writeYAML(t, dir, "root.yaml", pb1)
	subDir := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	writeYAML(t, subDir, "nested.yml", pb2)

	playbooks, err := loader.LoadAll(dir)
	require.NoError(t, err)
	require.Len(t, playbooks, 2)

	ids := map[string]bool{}
	for _, pb := range playbooks {
		ids[pb.ID] = true
	}
	assert.True(t, ids["pb-root"])
	assert.True(t, ids["pb-nested"])
}
