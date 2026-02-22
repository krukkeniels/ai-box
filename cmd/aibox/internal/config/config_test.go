package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultValues(t *testing.T) {
	// Isolate from host config: point HOME at an empty temp dir so
	// Load("") cannot pick up ~/.config/aibox/config.yaml.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() with no config file: %v", err)
	}

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"config_version", cfg.ConfigVersion, 1},
		{"runtime", cfg.Runtime, "podman"},
		{"image", cfg.Image, "ghcr.io/krukkeniels/aibox/base:24.04"},
		{"gvisor.enabled", cfg.GVisor.Enabled, false},
		{"gvisor.platform", cfg.GVisor.Platform, "systrap"},
		{"resources.cpus", cfg.Resources.CPUs, 4},
		{"resources.memory", cfg.Resources.Memory, "8g"},
		{"resources.tmp_size", cfg.Resources.TmpSize, "2g"},
		{"workspace.default_path", cfg.Workspace.DefaultPath, "."},
		{"workspace.validate_fs", cfg.Workspace.ValidateFS, true},
		{"registry.url", cfg.Registry.URL, "ghcr.io/krukkeniels/aibox"},
		{"registry.verify_signatures", cfg.Registry.VerifySignatures, false},
		{"network.enabled", cfg.Network.Enabled, false},
		{"audit.enabled", cfg.Audit.Enabled, false},
		{"logging.format", cfg.Logging.Format, "text"},
		{"logging.level", cfg.Logging.Level, "info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("default %s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `runtime: docker
image: myregistry.io/aibox/custom:latest
gvisor:
  enabled: false
  platform: ptrace
resources:
  cpus: 8
  memory: 16g
  tmp_size: 4g
workspace:
  default_path: /home/dev/projects
  validate_fs: false
registry:
  url: myregistry.io
  verify_signatures: false
logging:
  format: json
  level: debug
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load(%s): %v", cfgPath, err)
	}

	if cfg.Runtime != "docker" {
		t.Errorf("runtime = %q, want %q", cfg.Runtime, "docker")
	}
	if cfg.Image != "myregistry.io/aibox/custom:latest" {
		t.Errorf("image = %q, want %q", cfg.Image, "myregistry.io/aibox/custom:latest")
	}
	if cfg.GVisor.Enabled {
		t.Error("gvisor.enabled = true, want false")
	}
	if cfg.GVisor.Platform != "ptrace" {
		t.Errorf("gvisor.platform = %q, want %q", cfg.GVisor.Platform, "ptrace")
	}
	if cfg.Resources.CPUs != 8 {
		t.Errorf("resources.cpus = %d, want 8", cfg.Resources.CPUs)
	}
	if cfg.Resources.Memory != "16g" {
		t.Errorf("resources.memory = %q, want %q", cfg.Resources.Memory, "16g")
	}
	if cfg.Resources.TmpSize != "4g" {
		t.Errorf("resources.tmp_size = %q, want %q", cfg.Resources.TmpSize, "4g")
	}
	if cfg.Workspace.ValidateFS {
		t.Error("workspace.validate_fs = true, want false")
	}
	if cfg.Registry.VerifySignatures {
		t.Error("registry.verify_signatures = true, want false")
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("logging.format = %q, want %q", cfg.Logging.Format, "json")
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("logging.level = %q, want %q", cfg.Logging.Level, "debug")
	}
}

func TestEnvVarOverrides(t *testing.T) {
	// Isolate from host config.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Set env vars.
	t.Setenv("AIBOX_RUNTIME", "docker")
	t.Setenv("AIBOX_IMAGE", "test-registry.io/aibox/test:v1")
	t.Setenv("AIBOX_GVISOR_ENABLED", "false")
	t.Setenv("AIBOX_RESOURCES_CPUS", "16")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}

	if cfg.Runtime != "docker" {
		t.Errorf("runtime = %q, want %q (from AIBOX_RUNTIME)", cfg.Runtime, "docker")
	}
	if cfg.Image != "test-registry.io/aibox/test:v1" {
		t.Errorf("image = %q, want %q (from AIBOX_IMAGE)", cfg.Image, "test-registry.io/aibox/test:v1")
	}
}

func TestLoadMissingExplicitFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Load() with missing explicit path should return error")
	}
}

func TestWriteDefault(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	path, err := WriteDefault(cfgPath)
	if err != nil {
		t.Fatalf("WriteDefault(): %v", err)
	}

	if path != cfgPath {
		t.Errorf("WriteDefault returned %q, want %q", path, cfgPath)
	}

	// File should exist.
	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("config file not created: %v", err)
	}

	// Should not overwrite existing file.
	if err := os.WriteFile(cfgPath, []byte("custom content"), 0o644); err != nil {
		t.Fatalf("writing custom content: %v", err)
	}

	path2, err := WriteDefault(cfgPath)
	if err != nil {
		t.Fatalf("WriteDefault() on existing file: %v", err)
	}
	if path2 != cfgPath {
		t.Errorf("WriteDefault returned %q, want %q", path2, cfgPath)
	}

	data, _ := os.ReadFile(cfgPath)
	if string(data) != "custom content" {
		t.Error("WriteDefault should not overwrite existing file")
	}
}
