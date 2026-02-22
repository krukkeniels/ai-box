package mcppacks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifestValidate(t *testing.T) {
	tests := []struct {
		name    string
		m       Manifest
		wantErr bool
	}{
		{
			name: "valid manifest",
			m: Manifest{
				Name:    "test-mcp",
				Version: "1.0.0",
				Command: "test-server",
			},
		},
		{
			name:    "missing name",
			m:       Manifest{Version: "1.0.0", Command: "test"},
			wantErr: true,
		},
		{
			name:    "missing version",
			m:       Manifest{Name: "test", Command: "test"},
			wantErr: true,
		},
		{
			name:    "missing command",
			m:       Manifest{Name: "test", Version: "1.0.0"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.m.Validate()
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")

	content := `name: test-mcp
version: "1.0.0"
description: "A test MCP pack"
command: test-server
args:
  - "--port"
  - "8080"
network_requires:
  - api.internal
filesystem_requires:
  - /workspace
permissions:
  - read
  - write
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	if m.Name != "test-mcp" {
		t.Errorf("name = %q, want %q", m.Name, "test-mcp")
	}
	if m.Command != "test-server" {
		t.Errorf("command = %q, want %q", m.Command, "test-server")
	}
	if len(m.Args) != 2 {
		t.Errorf("args len = %d, want 2", len(m.Args))
	}
	if len(m.NetworkRequires) != 1 || m.NetworkRequires[0] != "api.internal" {
		t.Errorf("network_requires = %v, want [api.internal]", m.NetworkRequires)
	}
}

func TestLoadManifest_Invalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")

	// Missing required fields.
	if err := os.WriteFile(path, []byte("description: missing fields"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadManifest(path)
	if err == nil {
		t.Error("expected error for invalid manifest, got nil")
	}
}
