// Package assets embeds static configuration files (seccomp profile,
// AppArmor profile, org policy template) into the binary so that
// `aibox setup --system` works from any install location without
// needing the source tree.
package assets

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed seccomp.json
var seccompProfile []byte

//go:embed apparmor/aibox-sandbox
var appArmorProfile []byte

//go:embed default-org-policy.yaml
var orgPolicy []byte

// SeccompProfile returns the embedded seccomp profile as a byte slice.
func SeccompProfile() []byte {
	return seccompProfile
}

// AppArmorProfile returns the embedded AppArmor profile as a byte slice.
func AppArmorProfile() []byte {
	return appArmorProfile
}

// WriteSeccompProfile writes the embedded seccomp profile to the given path.
// Parent directories are created if they do not exist.
func WriteSeccompProfile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}
	if err := os.WriteFile(path, seccompProfile, 0o644); err != nil {
		return fmt.Errorf("writing seccomp profile to %s: %w", path, err)
	}
	return nil
}

// WriteAppArmorProfile writes the embedded AppArmor profile to the given path.
// Parent directories are created if they do not exist.
func WriteAppArmorProfile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}
	if err := os.WriteFile(path, appArmorProfile, 0o644); err != nil {
		return fmt.Errorf("writing AppArmor profile to %s: %w", path, err)
	}
	return nil
}

// DefaultOrgPolicy returns the embedded default org policy as a byte slice.
func DefaultOrgPolicy() []byte {
	return orgPolicy
}

// WriteDefaultOrgPolicy writes the embedded default org policy to the given path.
// Parent directories are created if they do not exist.
func WriteDefaultOrgPolicy(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}
	if err := os.WriteFile(path, orgPolicy, 0o644); err != nil {
		return fmt.Errorf("writing org policy to %s: %w", path, err)
	}
	return nil
}
