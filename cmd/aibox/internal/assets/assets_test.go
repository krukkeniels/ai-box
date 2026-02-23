package assets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeccompProfile(t *testing.T) {
	data := SeccompProfile()
	if len(data) == 0 {
		t.Fatal("SeccompProfile() returned empty data")
	}

	// Must be valid JSON.
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("SeccompProfile() is not valid JSON: %v", err)
	}

	// Must contain the defaultAction field.
	if _, ok := parsed["defaultAction"]; !ok {
		t.Error("SeccompProfile() JSON missing 'defaultAction' field")
	}
}

func TestAppArmorProfile(t *testing.T) {
	data := AppArmorProfile()
	if len(data) == 0 {
		t.Fatal("AppArmorProfile() returned empty data")
	}

	content := string(data)
	if !strings.Contains(content, "aibox-sandbox") {
		t.Error("AppArmorProfile() does not contain 'aibox-sandbox'")
	}
}

func TestWriteSeccompProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "seccomp.json")

	if err := WriteSeccompProfile(path); err != nil {
		t.Fatalf("WriteSeccompProfile() error: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}

	if string(written) != string(SeccompProfile()) {
		t.Error("written seccomp profile does not match embedded content")
	}

	// Verify file permissions.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat written file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("expected permissions 0644, got %o", perm)
	}
}

func TestWriteAppArmorProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "aibox-sandbox")

	if err := WriteAppArmorProfile(path); err != nil {
		t.Fatalf("WriteAppArmorProfile() error: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}

	if string(written) != string(AppArmorProfile()) {
		t.Error("written AppArmor profile does not match embedded content")
	}

	// Verify file permissions.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat written file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("expected permissions 0644, got %o", perm)
	}
}
