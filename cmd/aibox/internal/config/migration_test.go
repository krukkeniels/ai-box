package config

import (
	"os"
	"path/filepath"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestDetectVersionMissing(t *testing.T) {
	data := []byte("runtime: podman\nimage: test:latest\n")
	v, err := DetectVersion(data)
	if err != nil {
		t.Fatalf("DetectVersion: %v", err)
	}
	if v != 0 {
		t.Errorf("DetectVersion = %d, want 0 for config without config_version", v)
	}
}

func TestDetectVersionPresent(t *testing.T) {
	data := []byte("config_version: 1\nruntime: podman\n")
	v, err := DetectVersion(data)
	if err != nil {
		t.Fatalf("DetectVersion: %v", err)
	}
	if v != 1 {
		t.Errorf("DetectVersion = %d, want 1", v)
	}
}

func TestDetectVersionInvalidYAML(t *testing.T) {
	data := []byte("{\n  invalid: [yaml\n")
	_, err := DetectVersion(data)
	if err == nil {
		t.Error("DetectVersion should fail on invalid YAML")
	}
}

func TestMigrateV0ToV1(t *testing.T) {
	input := []byte(`runtime: podman
image: harbor.internal/aibox/base:24.04
shell: bash
resources:
  cpus: 4
  memory: 8g
  tmp_size: 2g
logging:
  format: text
  level: info
credentials:
  mode: fallback
network:
  proxy_port: 3128
  dns_port: 53
ide:
  ssh_port: 2222
`)

	migrated, err := MigrateConfig(input, 0)
	if err != nil {
		t.Fatalf("MigrateConfig v0->v1: %v", err)
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(migrated, &raw); err != nil {
		t.Fatalf("unmarshal migrated: %v", err)
	}

	v, ok := raw["config_version"]
	if !ok {
		t.Fatal("migrated config missing config_version")
	}
	if vi, ok := v.(int); !ok || vi != 1 {
		t.Errorf("config_version = %v, want 1", v)
	}

	// Original fields should be preserved.
	if raw["runtime"] != "podman" {
		t.Errorf("runtime = %v, want podman", raw["runtime"])
	}
	if raw["shell"] != "bash" {
		t.Errorf("shell = %v, want bash", raw["shell"])
	}
}

func TestMigrateAlreadyCurrent(t *testing.T) {
	input := []byte("config_version: 1\nruntime: podman\n")
	out, err := MigrateConfig(input, CurrentConfigVersion)
	if err != nil {
		t.Fatalf("MigrateConfig on current version: %v", err)
	}
	// Should return unchanged.
	if string(out) != string(input) {
		t.Error("MigrateConfig should return unchanged data for current version")
	}
}

func TestMigrateNegativeVersion(t *testing.T) {
	_, err := MigrateConfig([]byte("runtime: podman\n"), -1)
	if err == nil {
		t.Error("MigrateConfig should fail for negative version")
	}
}

func TestBackupConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := "runtime: podman\nshell: bash\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	backupPath, err := BackupConfig(cfgPath, 0)
	if err != nil {
		t.Fatalf("BackupConfig: %v", err)
	}

	expected := cfgPath + ".backup.v0"
	if backupPath != expected {
		t.Errorf("backup path = %q, want %q", backupPath, expected)
	}

	data, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(data) != content {
		t.Errorf("backup content = %q, want %q", string(data), content)
	}
}

func TestBackupConfigMissingFile(t *testing.T) {
	_, err := BackupConfig("/nonexistent/config.yaml", 0)
	if err == nil {
		t.Error("BackupConfig should fail for missing file")
	}
}

func TestMigrateConfigFileDryRun(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	// Write a v0 config with all required fields for validation.
	content := `runtime: podman
image: ghcr.io/krukkeniels/aibox/base:24.04
shell: bash
resources:
  cpus: 4
  memory: 8g
  tmp_size: 2g
logging:
  format: text
  level: info
credentials:
  mode: fallback
network:
  proxy_port: 3128
  dns_port: 53
ide:
  ssh_port: 2222
audit:
  storage_backend: local
  recording_policy: disabled
  llm_logging_mode: hash
  runtime_backend: none
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	migrated, from, to, err := MigrateConfigFile(cfgPath, true)
	if err != nil {
		t.Fatalf("MigrateConfigFile dry-run: %v", err)
	}

	if from != 0 {
		t.Errorf("from = %d, want 0", from)
	}
	if to != CurrentConfigVersion {
		t.Errorf("to = %d, want %d", to, CurrentConfigVersion)
	}
	if len(migrated) == 0 {
		t.Error("dry-run should return migrated data")
	}

	// Original file should be unchanged (no backup).
	backupPath := cfgPath + ".backup.v0"
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Error("dry-run should not create a backup file")
	}

	// Original file should be unchanged.
	original, _ := os.ReadFile(cfgPath)
	if string(original) != content {
		t.Error("dry-run should not modify the original file")
	}
}

func TestMigrateConfigFileActual(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `runtime: podman
image: ghcr.io/krukkeniels/aibox/base:24.04
shell: bash
resources:
  cpus: 4
  memory: 8g
  tmp_size: 2g
logging:
  format: text
  level: info
credentials:
  mode: fallback
network:
  proxy_port: 3128
  dns_port: 53
ide:
  ssh_port: 2222
audit:
  storage_backend: local
  recording_policy: disabled
  llm_logging_mode: hash
  runtime_backend: none
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	_, from, to, err := MigrateConfigFile(cfgPath, false)
	if err != nil {
		t.Fatalf("MigrateConfigFile: %v", err)
	}

	if from != 0 || to != CurrentConfigVersion {
		t.Errorf("migration from=%d to=%d, want 0->%d", from, to, CurrentConfigVersion)
	}

	// Backup should exist.
	backupPath := cfgPath + ".backup.v0"
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("backup file should exist: %v", err)
	}

	// Migrated file should have config_version.
	migrated, _ := os.ReadFile(cfgPath)
	v, err := DetectVersion(migrated)
	if err != nil {
		t.Fatalf("DetectVersion on migrated file: %v", err)
	}
	if v != CurrentConfigVersion {
		t.Errorf("migrated file version = %d, want %d", v, CurrentConfigVersion)
	}
}

func TestMigrateConfigFileAlreadyCurrent(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := "config_version: 1\nruntime: podman\nshell: bash\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	_, from, to, err := MigrateConfigFile(cfgPath, false)
	if err != nil {
		t.Fatalf("MigrateConfigFile: %v", err)
	}
	if from != to {
		t.Errorf("already-current config should have from == to, got %d != %d", from, to)
	}
}

func TestMigrateConfigFileMissing(t *testing.T) {
	_, _, _, err := MigrateConfigFile("/nonexistent/config.yaml", false)
	if err == nil {
		t.Error("MigrateConfigFile should fail for missing file")
	}
}
