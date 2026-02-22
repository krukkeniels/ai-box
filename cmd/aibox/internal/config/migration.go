package config

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// CurrentConfigVersion is the latest config schema version.
const CurrentConfigVersion = 1

// migrationFunc transforms raw YAML data from one version to the next.
type migrationFunc func(data map[string]interface{}) (map[string]interface{}, error)

// migrations is an ordered list of version-to-version migration functions.
// Index 0 = v0 -> v1, index 1 = v1 -> v2, etc.
var migrations = []migrationFunc{
	migrateV0ToV1,
}

// migrateV0ToV1 adds config_version: 1 to a v0 config (which has no version field).
func migrateV0ToV1(data map[string]interface{}) (map[string]interface{}, error) {
	data["config_version"] = 1
	return data, nil
}

// DetectVersion returns the config_version from raw YAML data.
// If the field is absent, returns 0 (pre-versioning).
func DetectVersion(data []byte) (int, error) {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return 0, fmt.Errorf("parsing config: %w", err)
	}
	return detectVersionFromMap(raw), nil
}

func detectVersionFromMap(raw map[string]interface{}) int {
	v, ok := raw["config_version"]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	}
	return 0
}

// MigrateConfig runs all pending migrations on raw YAML data from fromVersion
// up to CurrentConfigVersion. Returns the migrated YAML bytes.
func MigrateConfig(data []byte, fromVersion int) ([]byte, error) {
	if fromVersion < 0 {
		return nil, fmt.Errorf("invalid source version: %d", fromVersion)
	}
	if fromVersion >= CurrentConfigVersion {
		return data, nil // already at latest
	}
	if fromVersion >= len(migrations) {
		return nil, fmt.Errorf("no migration path from version %d", fromVersion)
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing config for migration: %w", err)
	}

	for v := fromVersion; v < CurrentConfigVersion && v < len(migrations); v++ {
		var err error
		raw, err = migrations[v](raw)
		if err != nil {
			return nil, fmt.Errorf("migration v%d -> v%d failed: %w", v, v+1, err)
		}
	}

	out, err := yaml.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("serializing migrated config: %w", err)
	}
	return out, nil
}

// BackupConfig creates a backup of the config file at path with a version suffix.
// Returns the backup path.
func BackupConfig(path string, version int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading config for backup: %w", err)
	}

	backupPath := fmt.Sprintf("%s.backup.v%d", path, version)
	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		return "", fmt.Errorf("writing backup: %w", err)
	}
	return backupPath, nil
}

// MigrateConfigFile reads a config file, detects its version, backs it up,
// migrates it, validates the result, and writes it back.
// If dryRun is true, returns the migrated bytes without writing.
func MigrateConfigFile(path string, dryRun bool) ([]byte, int, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("reading config: %w", err)
	}

	fromVersion, err := DetectVersion(data)
	if err != nil {
		return nil, 0, 0, err
	}

	if fromVersion >= CurrentConfigVersion {
		return data, fromVersion, fromVersion, nil // already current
	}

	migrated, err := MigrateConfig(data, fromVersion)
	if err != nil {
		return nil, fromVersion, 0, err
	}

	// Validate the migrated config by loading it.
	cfg, err := loadFromBytes(migrated)
	if err != nil {
		return nil, fromVersion, 0, fmt.Errorf("migrated config failed to load: %w", err)
	}

	result := Validate(cfg)
	if result.HasErrors() {
		return nil, fromVersion, 0, fmt.Errorf("migrated config failed validation:\n%s", result.String())
	}

	if dryRun {
		return migrated, fromVersion, CurrentConfigVersion, nil
	}

	// Backup before writing.
	if _, err := BackupConfig(path, fromVersion); err != nil {
		return nil, fromVersion, 0, err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fromVersion, 0, fmt.Errorf("creating config directory: %w", err)
	}

	if err := os.WriteFile(path, migrated, 0o644); err != nil {
		return nil, fromVersion, 0, fmt.Errorf("writing migrated config: %w", err)
	}

	return migrated, fromVersion, CurrentConfigVersion, nil
}

// loadFromBytes parses YAML bytes into a Config using Viper.
func loadFromBytes(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
