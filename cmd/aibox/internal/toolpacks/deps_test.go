package toolpacks

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestRegistry(t *testing.T) (*Registry, string) {
	t.Helper()
	dir := t.TempDir()
	installDir := t.TempDir()

	// Create node@20 pack.
	nodeDir := filepath.Join(dir, "node")
	_ = os.MkdirAll(nodeDir, 0o755)
	_ = os.WriteFile(filepath.Join(nodeDir, "manifest.yaml"), []byte(`
name: node
version: "20"
description: "Node.js 20 LTS"
maintainer: platform-team
install:
  method: script
  script: install.sh
`), 0o644)

	// Create angular@18 with dependency on node@20.
	angularDir := filepath.Join(dir, "angular")
	_ = os.MkdirAll(angularDir, 0o755)
	_ = os.WriteFile(filepath.Join(angularDir, "manifest.yaml"), []byte(`
name: angular
version: "18"
description: "Angular CLI 18.x"
maintainer: platform-team
install:
  method: script
  script: install.sh
dependencies:
  - name: node
    version: "20"
`), 0o644)

	// Create java@21 (no dependencies).
	javaDir := filepath.Join(dir, "java")
	_ = os.MkdirAll(javaDir, 0o755)
	_ = os.WriteFile(filepath.Join(javaDir, "manifest.yaml"), []byte(`
name: java
version: "21"
description: "OpenJDK 21"
maintainer: platform-team
install:
  method: script
  script: install.sh
`), 0o644)

	reg := NewRegistry(dir, installDir)
	return reg, installDir
}

func TestResolveDependencies_NoDeps(t *testing.T) {
	reg, _ := setupTestRegistry(t)

	packs, err := ResolveDependencies(reg, "java", "21")
	if err != nil {
		t.Fatalf("ResolveDependencies() error = %v", err)
	}
	if len(packs) != 1 {
		t.Fatalf("expected 1 pack, got %d", len(packs))
	}
	if packs[0].Manifest.Name != "java" {
		t.Errorf("pack name = %q, want %q", packs[0].Manifest.Name, "java")
	}
}

func TestResolveDependencies_WithDeps(t *testing.T) {
	reg, _ := setupTestRegistry(t)

	packs, err := ResolveDependencies(reg, "angular", "18")
	if err != nil {
		t.Fatalf("ResolveDependencies() error = %v", err)
	}
	if len(packs) != 2 {
		t.Fatalf("expected 2 packs, got %d", len(packs))
	}
	// node should come before angular (dependency order).
	if packs[0].Manifest.Name != "node" {
		t.Errorf("first pack = %q, want %q (dependency should come first)", packs[0].Manifest.Name, "node")
	}
	if packs[1].Manifest.Name != "angular" {
		t.Errorf("second pack = %q, want %q", packs[1].Manifest.Name, "angular")
	}
}

func TestResolveDependencies_NotFound(t *testing.T) {
	reg, _ := setupTestRegistry(t)

	_, err := ResolveDependencies(reg, "rust", "1.75")
	if err == nil {
		t.Fatal("expected error for unknown pack")
	}
}

func TestResolveDependencies_CircularDetection(t *testing.T) {
	dir := t.TempDir()
	installDir := t.TempDir()

	// Create pack A depends on B, B depends on A.
	aDir := filepath.Join(dir, "a")
	_ = os.MkdirAll(aDir, 0o755)
	_ = os.WriteFile(filepath.Join(aDir, "manifest.yaml"), []byte(`
name: a
version: "1"
description: "Pack A"
maintainer: test
install:
  method: script
  script: install.sh
dependencies:
  - name: b
    version: "1"
`), 0o644)

	bDir := filepath.Join(dir, "b")
	_ = os.MkdirAll(bDir, 0o755)
	_ = os.WriteFile(filepath.Join(bDir, "manifest.yaml"), []byte(`
name: b
version: "1"
description: "Pack B"
maintainer: test
install:
  method: script
  script: install.sh
dependencies:
  - name: a
    version: "1"
`), 0o644)

	reg := NewRegistry(dir, installDir)
	_, err := ResolveDependencies(reg, "a", "1")
	if err == nil {
		t.Fatal("expected error for circular dependency")
	}
}

func TestRegistry_VersionMatch(t *testing.T) {
	tests := []struct {
		manifest  string
		shorthand string
		want      bool
	}{
		{"21", "21", true},
		{"21.0", "21", true},
		{"21.0.4", "21", true},
		{"21", "20", false},
		{"3.12", "3.12", true},
		{"3.12.1", "3.12", true},
		{"3.11", "3.12", false},
	}
	for _, tt := range tests {
		got := versionMatch(tt.manifest, tt.shorthand)
		if got != tt.want {
			t.Errorf("versionMatch(%q, %q) = %v, want %v", tt.manifest, tt.shorthand, got, tt.want)
		}
	}
}

func TestRegistry_MarkAndCheckInstalled(t *testing.T) {
	dir := t.TempDir()
	installDir := t.TempDir()
	reg := NewRegistry(dir, installDir)

	if reg.IsInstalled("java", "21") {
		t.Error("java@21 should not be installed initially")
	}

	if err := reg.MarkInstalled("java", "21"); err != nil {
		t.Fatalf("MarkInstalled() error = %v", err)
	}

	if !reg.IsInstalled("java", "21") {
		t.Error("java@21 should be installed after MarkInstalled")
	}

	if reg.IsInstalled("java", "17") {
		t.Error("java@17 should not be installed")
	}
}

func TestRegistry_List(t *testing.T) {
	reg, _ := setupTestRegistry(t)

	packs, err := reg.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(packs) != 3 {
		t.Fatalf("List() returned %d packs, want 3", len(packs))
	}

	// Should be sorted by name.
	names := make([]string, len(packs))
	for i, p := range packs {
		names[i] = p.Manifest.Name
	}
	if names[0] != "angular" || names[1] != "java" || names[2] != "node" {
		t.Errorf("packs not sorted: %v", names)
	}
}

func TestRegistry_Resolve(t *testing.T) {
	reg, _ := setupTestRegistry(t)

	info, err := reg.Resolve("node", "20")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if info.Manifest.Name != "node" {
		t.Errorf("resolved name = %q, want %q", info.Manifest.Name, "node")
	}
	if info.Manifest.Version != "20" {
		t.Errorf("resolved version = %q, want %q", info.Manifest.Version, "20")
	}
}

func TestFormatDepTree(t *testing.T) {
	packs := []*PackInfo{
		{Manifest: &Manifest{Name: "node", Version: "20"}},
		{Manifest: &Manifest{Name: "angular", Version: "18"}},
	}
	out := FormatDepTree(packs)
	if out == "" {
		t.Error("FormatDepTree() returned empty string")
	}
}
