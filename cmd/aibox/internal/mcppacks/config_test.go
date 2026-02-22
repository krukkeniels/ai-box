package mcppacks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPackServerKey(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"filesystem-mcp", "filesystem"},
		{"git-mcp", "git"},
		{"plain", "plain"},
		{"mcp", "mcp"}, // too short to strip suffix
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := packServerKey(tc.name)
			if got != tc.want {
				t.Errorf("packServerKey(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestGenerateConfig(t *testing.T) {
	home := t.TempDir()

	enabled := []string{"filesystem-mcp", "git-mcp"}
	if err := GenerateConfig(home, enabled); err != nil {
		t.Fatalf("GenerateConfig failed: %v", err)
	}

	// Verify canonical config.
	data, err := os.ReadFile(MCPConfigPath(home))
	if err != nil {
		t.Fatalf("reading MCP config: %v", err)
	}

	var cfg MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parsing MCP config: %v", err)
	}

	if _, ok := cfg.MCPServers["filesystem"]; !ok {
		t.Error("expected 'filesystem' server in config")
	}
	if _, ok := cfg.MCPServers["git"]; !ok {
		t.Error("expected 'git' server in config")
	}

	fs := cfg.MCPServers["filesystem"]
	if fs.Command != "aibox-mcp-filesystem" {
		t.Errorf("filesystem command = %q, want %q", fs.Command, "aibox-mcp-filesystem")
	}
	if len(fs.Args) != 2 || fs.Args[0] != "--root" || fs.Args[1] != "/workspace" {
		t.Errorf("filesystem args = %v, want [--root /workspace]", fs.Args)
	}

	// Verify Claude config is also written.
	claudePath := filepath.Join(home, ".config", "claude", "claude_desktop_config.json")
	if _, err := os.Stat(claudePath); err != nil {
		t.Errorf("Claude config not written: %v", err)
	}
}

func TestStateRoundTrip(t *testing.T) {
	home := t.TempDir()

	state := &StateFile{Enabled: []string{"filesystem-mcp", "git-mcp"}}
	if err := SaveState(home, state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	loaded, err := LoadState(home)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if len(loaded.Enabled) != 2 {
		t.Fatalf("expected 2 enabled packs, got %d", len(loaded.Enabled))
	}
	if loaded.Enabled[0] != "filesystem-mcp" || loaded.Enabled[1] != "git-mcp" {
		t.Errorf("enabled = %v, want [filesystem-mcp git-mcp]", loaded.Enabled)
	}
}

func TestLoadState_NoFile(t *testing.T) {
	home := t.TempDir()

	state, err := LoadState(home)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if len(state.Enabled) != 0 {
		t.Errorf("expected empty state, got %v", state.Enabled)
	}
}

func TestIsEnabled(t *testing.T) {
	enabled := []string{"filesystem-mcp", "git-mcp"}

	if !IsEnabled(enabled, "filesystem-mcp") {
		t.Error("expected filesystem-mcp to be enabled")
	}
	if IsEnabled(enabled, "jira-mcp") {
		t.Error("expected jira-mcp to NOT be enabled")
	}
}
