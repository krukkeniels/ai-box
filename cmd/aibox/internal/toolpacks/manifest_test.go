package toolpacks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifest_Valid(t *testing.T) {
	yaml := `
name: java
version: "21"
description: "OpenJDK 21 (Temurin), Gradle 8.5, Maven 3.9.6"
maintainer: platform-team
tags:
  - language
  - jvm
install:
  method: script
  script: install.sh
network:
  requires:
    - id: maven-central
      hosts: ["nexus.internal"]
      ports: [443]
filesystem:
  creates: ["/opt/toolpacks/java"]
  caches:
    - "$HOME/.m2/repository"
    - "$HOME/.gradle/caches"
resources:
  min_memory: "4GB"
  recommended_memory: "8GB"
environment:
  JAVA_HOME: "/usr/lib/jvm/java-21"
verify:
  - command: "java --version"
    expect_exit_code: 0
`
	m, err := ParseManifest([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseManifest() error = %v", err)
	}

	if m.Name != "java" {
		t.Errorf("Name = %q, want %q", m.Name, "java")
	}
	if m.Version != "21" {
		t.Errorf("Version = %q, want %q", m.Version, "21")
	}
	if m.Install.Method != "script" {
		t.Errorf("Install.Method = %q, want %q", m.Install.Method, "script")
	}
	if len(m.Network.Requires) != 1 {
		t.Errorf("Network.Requires length = %d, want 1", len(m.Network.Requires))
	}
	if len(m.Filesystem.Caches) != 2 {
		t.Errorf("Filesystem.Caches length = %d, want 2", len(m.Filesystem.Caches))
	}
	if m.Environment["JAVA_HOME"] != "/usr/lib/jvm/java-21" {
		t.Errorf("Environment[JAVA_HOME] = %q, want %q", m.Environment["JAVA_HOME"], "/usr/lib/jvm/java-21")
	}
	if m.Ref() != "java@21" {
		t.Errorf("Ref() = %q, want %q", m.Ref(), "java@21")
	}
}

func TestValidateManifest_RequiredFields(t *testing.T) {
	m := &Manifest{}
	errs := ValidateManifest(m)
	if len(errs) == 0 {
		t.Fatal("ValidateManifest() should return errors for empty manifest")
	}

	fields := make(map[string]bool)
	for _, e := range errs {
		fields[e.Field] = true
	}

	required := []string{"name", "version", "description", "maintainer", "install.method"}
	for _, f := range required {
		if !fields[f] {
			t.Errorf("expected validation error for field %q", f)
		}
	}
}

func TestValidateManifest_InvalidName(t *testing.T) {
	m := &Manifest{
		Name:        "Java_Pack",
		Version:     "21",
		Description: "test",
		Maintainer:  "team",
		Install:     InstallSpec{Method: "script", Script: "install.sh"},
	}
	errs := ValidateManifest(m)
	found := false
	for _, e := range errs {
		if e.Field == "name" {
			found = true
		}
	}
	if !found {
		t.Error("expected validation error for invalid name format")
	}
}

func TestValidateManifest_ValidComplete(t *testing.T) {
	m := &Manifest{
		Name:        "node",
		Version:     "20",
		Description: "Node.js 20 LTS",
		Maintainer:  "platform-team",
		Install:     InstallSpec{Method: "script", Script: "install.sh"},
	}
	errs := ValidateManifest(m)
	if len(errs) != 0 {
		t.Errorf("ValidateManifest() returned %d errors for valid manifest: %v", len(errs), errs)
	}
}

func TestValidateManifest_ScriptMethodNoScript(t *testing.T) {
	m := &Manifest{
		Name:        "test",
		Version:     "1",
		Description: "test",
		Maintainer:  "team",
		Install:     InstallSpec{Method: "script"},
	}
	errs := ValidateManifest(m)
	found := false
	for _, e := range errs {
		if e.Field == "install.script" {
			found = true
		}
	}
	if !found {
		t.Error("expected validation error for script method without script path")
	}
}

func TestValidateManifest_DockerLayerNoImage(t *testing.T) {
	m := &Manifest{
		Name:        "test",
		Version:     "1",
		Description: "test",
		Maintainer:  "team",
		Install:     InstallSpec{Method: "docker-layer"},
	}
	errs := ValidateManifest(m)
	found := false
	for _, e := range errs {
		if e.Field == "install.base_image" {
			found = true
		}
	}
	if !found {
		t.Error("expected validation error for docker-layer method without base_image")
	}
}

func TestValidateManifest_InvalidDependency(t *testing.T) {
	m := &Manifest{
		Name:        "angular",
		Version:     "18",
		Description: "Angular CLI",
		Maintainer:  "team",
		Install:     InstallSpec{Method: "script", Script: "install.sh"},
		Dependencies: []Dependency{
			{Name: "", Version: "20"},
		},
	}
	errs := ValidateManifest(m)
	found := false
	for _, e := range errs {
		if e.Field == "dependencies[0].name" {
			found = true
		}
	}
	if !found {
		t.Error("expected validation error for dependency with empty name")
	}
}

func TestValidateManifest_NetworkRequiresNoHosts(t *testing.T) {
	m := &Manifest{
		Name:        "test",
		Version:     "1",
		Description: "test",
		Maintainer:  "team",
		Install:     InstallSpec{Method: "script", Script: "install.sh"},
		Network: NetworkSpec{
			Requires: []NetworkRequirement{
				{ID: "test", Hosts: nil},
			},
		},
	}
	errs := ValidateManifest(m)
	found := false
	for _, e := range errs {
		if e.Field == "network.requires[0].hosts" {
			found = true
		}
	}
	if !found {
		t.Error("expected validation error for network requirement without hosts")
	}
}

func TestParsePackRef(t *testing.T) {
	tests := []struct {
		ref     string
		name    string
		version string
	}{
		{"java@21", "java", "21"},
		{"node@20", "node", "20"},
		{"python@3.12", "python", "3.12"},
		{"ai-tools", "ai-tools", ""},
		{"angular@18", "angular", "18"},
	}
	for _, tt := range tests {
		name, version := ParsePackRef(tt.ref)
		if name != tt.name {
			t.Errorf("ParsePackRef(%q) name = %q, want %q", tt.ref, name, tt.name)
		}
		if version != tt.version {
			t.Errorf("ParsePackRef(%q) version = %q, want %q", tt.ref, version, tt.version)
		}
	}
}

func TestDiscoverManifests(t *testing.T) {
	dir := t.TempDir()

	// Create a pack directory with manifest.
	packDir := filepath.Join(dir, "test-pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `
name: test-pack
version: "1.0"
description: "Test pack"
maintainer: "test"
install:
  method: script
  script: install.sh
`
	if err := os.WriteFile(filepath.Join(packDir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	manifests, err := DiscoverManifests(dir)
	if err != nil {
		t.Fatalf("DiscoverManifests() error = %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("DiscoverManifests() returned %d manifests, want 1", len(manifests))
	}
	if manifests[0].Name != "test-pack" {
		t.Errorf("manifest name = %q, want %q", manifests[0].Name, "test-pack")
	}
}
