package config

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed templates/*.yaml
var templateFS embed.FS

// ValidTemplates lists the available config template names.
var ValidTemplates = []string{"minimal", "dev", "enterprise"}

// GetTemplate returns the content of a named config template.
func GetTemplate(name string) ([]byte, error) {
	path := fmt.Sprintf("templates/%s.yaml", name)
	data, err := templateFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("unknown template %q: valid templates are minimal, dev, enterprise", name)
	}
	return data, nil
}

// WriteTemplate writes a config template to the given path.
// If force is false and the file already exists, it returns an error.
func WriteTemplate(name, path string, force bool) error {
	data, err := GetTemplate(name)
	if err != nil {
		return err
	}

	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return fmt.Errorf("determining config path: %w", err)
		}
	}

	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config file already exists at %s (use --force to overwrite)", path)
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}
