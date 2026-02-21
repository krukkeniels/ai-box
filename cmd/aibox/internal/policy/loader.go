package policy

import (
	"fmt"
	"log/slog"
	"os"

	"go.yaml.in/yaml/v3"
)

// LoadPolicy reads a policy YAML file from disk and returns the parsed Policy.
func LoadPolicy(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading policy file %s: %w", path, err)
	}

	var p Policy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing policy file %s: %w", path, err)
	}

	slog.Debug("loaded policy", "path", path, "version", p.Version)
	return &p, nil
}

// LoadPolicyHierarchy loads the three-level policy hierarchy from disk.
// Any path may be empty to skip that level (returning nil for that policy).
// At minimum orgPath must be provided.
func LoadPolicyHierarchy(orgPath, teamPath, projectPath string) (*Policy, *Policy, *Policy, error) {
	if orgPath == "" {
		return nil, nil, nil, fmt.Errorf("org policy path is required")
	}

	org, err := LoadPolicy(orgPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading org policy: %w", err)
	}

	var team *Policy
	if teamPath != "" {
		team, err = LoadPolicy(teamPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("loading team policy: %w", err)
		}
	}

	var project *Policy
	if projectPath != "" {
		project, err = LoadPolicy(projectPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("loading project policy: %w", err)
		}
	}

	return org, team, project, nil
}
