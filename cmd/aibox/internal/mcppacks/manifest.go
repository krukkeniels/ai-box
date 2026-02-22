package mcppacks

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
)

// Manifest describes an MCP pack and its requirements.
type Manifest struct {
	Name               string   `yaml:"name" json:"name"`
	Version            string   `yaml:"version" json:"version"`
	Description        string   `yaml:"description" json:"description"`
	Command            string   `yaml:"command" json:"command"`
	Args               []string `yaml:"args,omitempty" json:"args,omitempty"`
	NetworkRequires    []string `yaml:"network_requires,omitempty" json:"network_requires,omitempty"`       // hosts that must be in the network allowlist
	FilesystemRequires []string `yaml:"filesystem_requires,omitempty" json:"filesystem_requires,omitempty"` // paths the MCP server needs access to
	Permissions        []string `yaml:"permissions,omitempty" json:"permissions,omitempty"`                 // e.g. "read", "write", "execute"
}

// Validate checks that the manifest has all required fields.
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("manifest: name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("manifest: version is required for pack %q", m.Name)
	}
	if m.Command == "" {
		return fmt.Errorf("manifest: command is required for pack %q", m.Name)
	}
	return nil
}

// LoadManifest reads a manifest YAML file from disk.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest %s: %w", path, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest %s: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}
