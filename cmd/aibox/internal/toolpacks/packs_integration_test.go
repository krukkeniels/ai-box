package toolpacks_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/aibox/aibox/internal/toolpacks"
)

func findPacksDir() string {
	// Walk up from the test file to find aibox-toolpacks/packs.
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, "aibox-toolpacks", "packs")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

func TestAllPackManifests_Valid(t *testing.T) {
	packsDir := findPacksDir()
	if packsDir == "" {
		t.Skip("could not locate aibox-toolpacks/packs directory")
	}

	manifests, err := toolpacks.DiscoverManifests(packsDir)
	if err != nil {
		t.Fatalf("DiscoverManifests() error = %v", err)
	}

	if len(manifests) < 10 {
		t.Errorf("expected at least 10 manifests, got %d", len(manifests))
	}

	expectedPacks := map[string]bool{
		"java":      false,
		"node":      false,
		"python":    false,
		"bazel":     false,
		"scala":     false,
		"angular":   false,
		"dotnet":    false,
		"powershell": false,
		"angularjs": false,
		"ai-tools":  false,
	}

	for _, m := range manifests {
		// Only validate tool packs (not MCP packs which use a different schema).
		if _, isToolPack := expectedPacks[m.Name]; !isToolPack {
			continue
		}

		t.Run(m.Ref(), func(t *testing.T) {
			errs := toolpacks.ValidateManifest(m)
			if len(errs) > 0 {
				for _, e := range errs {
					t.Errorf("validation error: %s", e)
				}
			}

			if m.Install.Method != "script" && m.Install.Method != "docker-layer" {
				t.Errorf("invalid install method: %s", m.Install.Method)
			}
		})

		expectedPacks[m.Name] = true
	}

	for name, found := range expectedPacks {
		if !found {
			t.Errorf("expected pack %q not found in manifests", name)
		}
	}
}

func TestAllPackManifests_DependencyResolution(t *testing.T) {
	packsDir := findPacksDir()
	if packsDir == "" {
		t.Skip("could not locate aibox-toolpacks/packs directory")
	}

	registry := toolpacks.NewRegistry(packsDir, t.TempDir())

	// angular@18 depends on node@20.
	packs, err := toolpacks.ResolveDependencies(registry, "angular", "18")
	if err != nil {
		t.Fatalf("ResolveDependencies(angular@18) error = %v", err)
	}
	if len(packs) != 2 {
		t.Fatalf("expected 2 packs for angular@18, got %d", len(packs))
	}
	if packs[0].Manifest.Name != "node" {
		t.Errorf("first dependency should be node, got %s", packs[0].Manifest.Name)
	}

	// scala@3 depends on java@21.
	packs, err = toolpacks.ResolveDependencies(registry, "scala", "3")
	if err != nil {
		t.Fatalf("ResolveDependencies(scala@3) error = %v", err)
	}
	if len(packs) != 2 {
		t.Fatalf("expected 2 packs for scala@3, got %d", len(packs))
	}
	if packs[0].Manifest.Name != "java" {
		t.Errorf("first dependency should be java, got %s", packs[0].Manifest.Name)
	}

	// java@21 has no dependencies.
	packs, err = toolpacks.ResolveDependencies(registry, "java", "21")
	if err != nil {
		t.Fatalf("ResolveDependencies(java@21) error = %v", err)
	}
	if len(packs) != 1 {
		t.Fatalf("expected 1 pack for java@21, got %d", len(packs))
	}
}
