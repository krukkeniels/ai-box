package toolpacks

import (
	"fmt"
	"strings"
)

// ResolveDependencies returns a topologically sorted list of packs to install,
// including all transitive dependencies. The input pack is always last.
func ResolveDependencies(registry *Registry, name, version string) ([]*PackInfo, error) {
	// Track visited packs to detect cycles.
	visited := make(map[string]bool)
	resolved := make(map[string]bool)
	var order []*PackInfo

	if err := resolveDFS(registry, name, version, visited, resolved, &order); err != nil {
		return nil, err
	}

	return order, nil
}

func resolveDFS(registry *Registry, name, version string, visited, resolved map[string]bool, order *[]*PackInfo) error {
	key := name + "@" + version
	if resolved[key] {
		return nil
	}
	if visited[key] {
		return fmt.Errorf("circular dependency detected: %s", key)
	}

	visited[key] = true

	info, err := registry.Resolve(name, version)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", key, err)
	}

	// Resolve dependencies first.
	for _, dep := range info.Manifest.Dependencies {
		if err := resolveDFS(registry, dep.Name, dep.Version, visited, resolved, order); err != nil {
			return err
		}
	}

	resolved[key] = true
	*order = append(*order, info)
	return nil
}

// FormatDepTree returns a human-readable dependency tree string.
func FormatDepTree(packs []*PackInfo) string {
	if len(packs) == 0 {
		return "(no dependencies)"
	}

	var b strings.Builder
	for i, p := range packs {
		if i == len(packs)-1 {
			b.WriteString(fmt.Sprintf("  %s (requested)\n", p.Manifest.Ref()))
		} else {
			b.WriteString(fmt.Sprintf("  %s (dependency)\n", p.Manifest.Ref()))
		}
	}
	return b.String()
}
