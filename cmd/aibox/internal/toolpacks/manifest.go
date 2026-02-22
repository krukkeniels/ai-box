package toolpacks

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Manifest represents a tool pack manifest (manifest.yaml).
type Manifest struct {
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version"`
	Description string   `yaml:"description"`
	Maintainer  string   `yaml:"maintainer"`
	Tags        []string `yaml:"tags,omitempty"`

	Install    InstallSpec    `yaml:"install"`
	Network    NetworkSpec    `yaml:"network,omitempty"`
	Filesystem FilesystemSpec `yaml:"filesystem,omitempty"`
	Resources  ResourceSpec   `yaml:"resources,omitempty"`
	Security   SecuritySpec   `yaml:"security,omitempty"`

	Dependencies []Dependency `yaml:"dependencies,omitempty"`

	// Spec-level fields from the original schema.
	Packages    PackageSpec    `yaml:"packages,omitempty"`
	Binaries    []BinarySpec   `yaml:"binaries,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Verify      []VerifySpec   `yaml:"verify,omitempty"`
}

// InstallSpec describes how a tool pack is installed.
type InstallSpec struct {
	Method    string `yaml:"method"`     // "script" or "docker-layer"
	Script    string `yaml:"script"`     // path to install script (relative to pack dir)
	BaseImage string `yaml:"base_image"` // for docker-layer method
}

// NetworkSpec declares network requirements for a tool pack.
type NetworkSpec struct {
	Requires []NetworkRequirement `yaml:"requires,omitempty"`
}

// NetworkRequirement is a single network endpoint requirement.
type NetworkRequirement struct {
	ID    string   `yaml:"id"`
	Hosts []string `yaml:"hosts"`
	Ports []int    `yaml:"ports"`
}

// FilesystemSpec declares filesystem usage.
type FilesystemSpec struct {
	Creates []string `yaml:"creates,omitempty"` // directories created
	Caches  []string `yaml:"caches,omitempty"`  // cache directories to persist
}

// ResourceSpec declares minimum resource requirements.
type ResourceSpec struct {
	MinMemory         string `yaml:"min_memory,omitempty"`
	RecommendedMemory string `yaml:"recommended_memory,omitempty"`
}

// SecuritySpec holds signature and checksum info.
type SecuritySpec struct {
	Checksum string `yaml:"checksum,omitempty"` // SHA-256 of the pack archive
	SignedBy string `yaml:"signed_by,omitempty"` // Cosign signer identity
}

// Dependency declares a dependency on another tool pack.
type Dependency struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// PackageSpec declares system packages to install.
type PackageSpec struct {
	Apt  []string `yaml:"apt,omitempty"`
	Snap []string `yaml:"snap,omitempty"`
}

// BinarySpec describes a binary download.
type BinarySpec struct {
	Name        string `yaml:"name"`
	URL         string `yaml:"url"`
	SHA256      string `yaml:"sha256"`
	InstallPath string `yaml:"install_path"`
}

// VerifySpec describes a post-install verification command.
type VerifySpec struct {
	Command        string `yaml:"command"`
	ExpectExitCode int    `yaml:"expect_exit_code"`
}

// Ref returns the canonical "name@version" reference string.
func (m *Manifest) Ref() string {
	return m.Name + "@" + m.Version
}

// LoadManifest reads and parses a manifest YAML file.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest %s: %w", path, err)
	}
	return ParseManifest(data)
}

// ParseManifest parses manifest YAML bytes.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	return &m, nil
}

// ValidationError describes a manifest validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// namePattern matches valid pack names (lowercase, digits, hyphens).
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ValidateManifest checks a manifest for required fields and valid values.
func ValidateManifest(m *Manifest) []ValidationError {
	var errs []ValidationError

	if m.Name == "" {
		errs = append(errs, ValidationError{"name", "required"})
	} else if !namePattern.MatchString(m.Name) {
		errs = append(errs, ValidationError{"name", "must be lowercase alphanumeric with hyphens"})
	}

	if m.Version == "" {
		errs = append(errs, ValidationError{"version", "required"})
	}

	if m.Description == "" {
		errs = append(errs, ValidationError{"description", "required"})
	}

	if m.Maintainer == "" {
		errs = append(errs, ValidationError{"maintainer", "required"})
	}

	method := m.Install.Method
	if method == "" {
		errs = append(errs, ValidationError{"install.method", "required (script or docker-layer)"})
	} else if method != "script" && method != "docker-layer" {
		errs = append(errs, ValidationError{"install.method", fmt.Sprintf("invalid method %q: must be script or docker-layer", method)})
	}

	if method == "script" && m.Install.Script == "" {
		errs = append(errs, ValidationError{"install.script", "required when method is script"})
	}
	if method == "docker-layer" && m.Install.BaseImage == "" {
		errs = append(errs, ValidationError{"install.base_image", "required when method is docker-layer"})
	}

	for i, dep := range m.Dependencies {
		if dep.Name == "" {
			errs = append(errs, ValidationError{fmt.Sprintf("dependencies[%d].name", i), "required"})
		}
		if dep.Version == "" {
			errs = append(errs, ValidationError{fmt.Sprintf("dependencies[%d].version", i), "required"})
		}
	}

	for i, nr := range m.Network.Requires {
		if nr.ID == "" {
			errs = append(errs, ValidationError{fmt.Sprintf("network.requires[%d].id", i), "required"})
		}
		if len(nr.Hosts) == 0 {
			errs = append(errs, ValidationError{fmt.Sprintf("network.requires[%d].hosts", i), "at least one host required"})
		}
	}

	return errs
}

// DiscoverManifests walks a directory tree looking for manifest.yaml files
// and returns all successfully parsed manifests.
func DiscoverManifests(baseDir string) ([]*Manifest, error) {
	var manifests []*Manifest

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, fmt.Errorf("reading packs directory %s: %w", baseDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(baseDir, entry.Name(), "manifest.yaml")
		if _, err := os.Stat(manifestPath); err != nil {
			continue
		}
		m, err := LoadManifest(manifestPath)
		if err != nil {
			continue
		}
		manifests = append(manifests, m)
	}

	return manifests, nil
}

// ParsePackRef splits a "name@version" reference into name and version.
// If no version is specified, version is empty.
func ParsePackRef(ref string) (name, version string) {
	parts := strings.SplitN(ref, "@", 2)
	name = parts[0]
	if len(parts) == 2 {
		version = parts[1]
	}
	return
}
