package mcppacks

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// MCPServerEntry is a single MCP server in the generated config.
type MCPServerEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// MCPConfig is the top-level MCP configuration file format.
// This follows the standard MCP config schema used by Claude Code and other agents.
type MCPConfig struct {
	MCPServers map[string]MCPServerEntry `json:"mcpServers"`
}

// StateFile tracks which MCP packs are currently enabled.
type StateFile struct {
	Enabled []string `json:"enabled"`
}

// configDir returns the AI-Box config directory.
func configDir(homeDir string) string {
	return filepath.Join(homeDir, ".config", "aibox")
}

// MCPConfigPath returns the canonical MCP config path.
func MCPConfigPath(homeDir string) string {
	return filepath.Join(configDir(homeDir), "mcp.json")
}

// StatePath returns the path to the MCP state file that tracks enabled packs.
func StatePath(homeDir string) string {
	return filepath.Join(configDir(homeDir), "mcp-state.json")
}

// LoadState reads the current enabled-packs state from disk.
// Returns an empty state if the file doesn't exist.
func LoadState(homeDir string) (*StateFile, error) {
	path := StatePath(homeDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &StateFile{}, nil
		}
		return nil, fmt.Errorf("reading MCP state: %w", err)
	}
	var s StateFile
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing MCP state: %w", err)
	}
	return &s, nil
}

// SaveState writes the enabled-packs state to disk.
func SaveState(homeDir string, state *StateFile) error {
	dir := configDir(homeDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling MCP state: %w", err)
	}
	return os.WriteFile(StatePath(homeDir), data, 0o644)
}

// GenerateConfig builds the MCP config from the currently enabled packs
// and writes it to the canonical location and agent-specific locations.
func GenerateConfig(homeDir string, enabledPacks []string) error {
	cfg := MCPConfig{
		MCPServers: make(map[string]MCPServerEntry),
	}

	for _, name := range enabledPacks {
		pack := FindPack(name)
		if pack == nil {
			slog.Warn("enabled pack not found in registry, skipping", "pack", name)
			continue
		}
		// Use the pack name without "-mcp" suffix as the server key.
		key := packServerKey(pack.Name)
		cfg.MCPServers[key] = MCPServerEntry{
			Command: pack.Command,
			Args:    pack.Args,
		}
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling MCP config: %w", err)
	}

	// Write canonical config.
	configPath := MCPConfigPath(homeDir)
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("writing MCP config: %w", err)
	}
	slog.Info("wrote MCP config", "path", configPath)

	// Write agent-specific configs.
	writeAgentConfigs(homeDir, data)

	return nil
}

// writeAgentConfigs copies the MCP config to agent-specific locations.
func writeAgentConfigs(homeDir string, data []byte) {
	// Claude Code: ~/.config/claude/claude_desktop_config.json
	claudeDir := filepath.Join(homeDir, ".config", "claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		slog.Warn("failed to create Claude config dir", "error", err)
	} else {
		claudePath := filepath.Join(claudeDir, "claude_desktop_config.json")
		if err := os.WriteFile(claudePath, data, 0o644); err != nil {
			slog.Warn("failed to write Claude MCP config", "error", err)
		} else {
			slog.Info("wrote Claude MCP config", "path", claudePath)
		}
	}
}

// packServerKey derives the MCP server key from a pack name.
// "filesystem-mcp" -> "filesystem", "git-mcp" -> "git"
func packServerKey(name string) string {
	const suffix = "-mcp"
	if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
		return name[:len(name)-len(suffix)]
	}
	return name
}

// IsEnabled checks if a pack name is in the enabled list.
func IsEnabled(enabled []string, name string) bool {
	for _, e := range enabled {
		if e == name {
			return true
		}
	}
	return false
}
