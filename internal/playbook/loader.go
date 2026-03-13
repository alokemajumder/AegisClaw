package playbook

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Loader reads and validates playbooks from YAML files.
type Loader struct {
	logger *slog.Logger
}

// NewLoader creates a new playbook loader.
func NewLoader(logger *slog.Logger) *Loader {
	return &Loader{logger: logger}
}

// LoadAll reads all playbooks from the given base directory.
func (l *Loader) LoadAll(baseDir string) ([]Playbook, error) {
	var playbooks []Playbook

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		// Skip schema files
		if strings.Contains(path, "schemas") {
			return nil
		}

		pb, err := l.LoadFile(path)
		if err != nil {
			l.logger.Warn("skipping invalid playbook", "path", path, "error", err)
			return nil
		}
		playbooks = append(playbooks, *pb)
		l.logger.Debug("playbook loaded", "id", pb.ID, "name", pb.Name, "tier", pb.Tier)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking playbook directory %s: %w", baseDir, err)
	}

	l.logger.Info("playbooks loaded", "count", len(playbooks), "base_dir", baseDir)
	return playbooks, nil
}

// LoadFile reads a single playbook from a YAML file.
func (l *Loader) LoadFile(path string) (*Playbook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading playbook file: %w", err)
	}

	var pb Playbook
	if err := yaml.Unmarshal(data, &pb); err != nil {
		return nil, fmt.Errorf("parsing playbook YAML: %w", err)
	}

	if err := l.validate(&pb); err != nil {
		return nil, fmt.Errorf("validating playbook: %w", err)
	}

	return &pb, nil
}

func (l *Loader) validate(pb *Playbook) error {
	if pb.ID == "" {
		return fmt.Errorf("playbook ID is required")
	}
	if pb.Name == "" {
		return fmt.Errorf("playbook name is required")
	}
	if pb.Tier < 0 || pb.Tier > 3 {
		return fmt.Errorf("invalid tier %d (must be 0-3)", pb.Tier)
	}
	if len(pb.Steps) == 0 {
		return fmt.Errorf("playbook must have at least one step")
	}
	for i, step := range pb.Steps {
		if step.Name == "" {
			return fmt.Errorf("step %d: name is required", i)
		}
		if step.AgentType == "" {
			return fmt.Errorf("step %d: agent_type is required", i)
		}
		if step.Action == "" {
			return fmt.Errorf("step %d: action is required", i)
		}
	}
	return nil
}

// FilterByTier returns playbooks that match the given tier levels.
func FilterByTier(playbooks []Playbook, allowedTiers []int) []Playbook {
	tierSet := make(map[int]bool, len(allowedTiers))
	for _, t := range allowedTiers {
		tierSet[t] = true
	}

	var filtered []Playbook
	for _, pb := range playbooks {
		if tierSet[pb.Tier] {
			filtered = append(filtered, pb)
		}
	}
	return filtered
}

// FilterByAssetType returns playbooks that can target the given asset type.
func FilterByAssetType(playbooks []Playbook, assetType string) []Playbook {
	var filtered []Playbook
	for _, pb := range playbooks {
		if len(pb.AssetTypes) == 0 {
			filtered = append(filtered, pb)
			continue
		}
		for _, at := range pb.AssetTypes {
			if at == assetType || at == "*" {
				filtered = append(filtered, pb)
				break
			}
		}
	}
	return filtered
}
