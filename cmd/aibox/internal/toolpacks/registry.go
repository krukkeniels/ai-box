package toolpacks

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PackStatus describes whether a pack is available, installed, or both.
type PackStatus string

const (
	StatusAvailable PackStatus = "available"
	StatusInstalled PackStatus = "installed"
)

// PackInfo combines a manifest with its installation status.
type PackInfo struct {
	Manifest *Manifest
	Status   PackStatus
	PackDir  string // directory containing the pack files
}

// Registry manages the set of available and installed tool packs.
type Registry struct {
	packsDir    string // base directory for pack manifests (e.g. aibox-toolpacks/packs)
	installBase string // base install directory (e.g. /opt/toolpacks)
}

// NewRegistry creates a registry backed by the given directories.
func NewRegistry(packsDir, installBase string) *Registry {
	return &Registry{
		packsDir:    packsDir,
		installBase: installBase,
	}
}

// List returns all known packs with their status.
func (r *Registry) List() ([]PackInfo, error) {
	manifests, err := DiscoverManifests(r.packsDir)
	if err != nil {
		return nil, fmt.Errorf("discovering packs: %w", err)
	}

	var result []PackInfo
	for _, m := range manifests {
		status := StatusAvailable
		if r.IsInstalled(m.Name, m.Version) {
			status = StatusInstalled
		}
		result = append(result, PackInfo{
			Manifest: m,
			Status:   status,
			PackDir:  filepath.Join(r.packsDir, m.Name),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Manifest.Name < result[j].Manifest.Name
	})

	return result, nil
}

// Resolve finds the manifest for a pack reference (name@version).
// Supports version shorthand: "21" matches "21", "21.0", "21.0.1", etc.
func (r *Registry) Resolve(name, version string) (*PackInfo, error) {
	manifests, err := DiscoverManifests(r.packsDir)
	if err != nil {
		return nil, fmt.Errorf("discovering packs: %w", err)
	}

	var candidates []*Manifest
	for _, m := range manifests {
		if m.Name != name {
			continue
		}
		if version == "" || m.Version == version || versionMatch(m.Version, version) {
			candidates = append(candidates, m)
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("pack %s@%s not found in registry", name, version)
	}

	// Sort by version descending to pick the latest match.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Version > candidates[j].Version
	})

	best := candidates[0]
	status := StatusAvailable
	if r.IsInstalled(best.Name, best.Version) {
		status = StatusInstalled
	}

	return &PackInfo{
		Manifest: best,
		Status:   status,
		PackDir:  filepath.Join(r.packsDir, best.Name),
	}, nil
}

// IsInstalled checks if a pack is installed at the install base.
func (r *Registry) IsInstalled(name, version string) bool {
	marker := filepath.Join(r.installBase, name, ".installed")
	data, err := os.ReadFile(marker)
	if err != nil {
		return false
	}
	installedVersion := strings.TrimSpace(string(data))
	return installedVersion == version
}

// MarkInstalled records that a pack has been installed.
func (r *Registry) MarkInstalled(name, version string) error {
	dir := filepath.Join(r.installBase, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating install dir %s: %w", dir, err)
	}
	marker := filepath.Join(dir, ".installed")
	if err := os.WriteFile(marker, []byte(version+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing install marker: %w", err)
	}
	slog.Debug("marked pack as installed", "name", name, "version", version)
	return nil
}

// InstalledPacks returns packs that are currently installed.
func (r *Registry) InstalledPacks() ([]PackInfo, error) {
	all, err := r.List()
	if err != nil {
		return nil, err
	}

	var installed []PackInfo
	for _, p := range all {
		if p.Status == StatusInstalled {
			installed = append(installed, p)
		}
	}
	return installed, nil
}

// versionMatch checks if a manifest version matches a shorthand version.
// For example, "21" matches "21", "21.0", "21.0.4".
func versionMatch(manifestVersion, shorthand string) bool {
	if manifestVersion == shorthand {
		return true
	}
	return strings.HasPrefix(manifestVersion, shorthand+".")
}

// DefaultPacksDir returns the default packs directory relative to the
// project root or a well-known system path.
func DefaultPacksDir() string {
	// Check well-known paths.
	candidates := []string{
		"/etc/aibox/toolpacks/packs",
	}

	// Try relative to the executable.
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "aibox-toolpacks", "packs"),
			filepath.Join(filepath.Dir(dir), "aibox-toolpacks", "packs"),
		)
	}

	// Try relative to CWD.
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "aibox-toolpacks", "packs"))
	}

	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}

	return "aibox-toolpacks/packs"
}
