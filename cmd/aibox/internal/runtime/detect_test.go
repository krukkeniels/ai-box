package runtime

import (
	"errors"
	"testing"
)

func TestNoRuntimeError(t *testing.T) {
	err := ErrNoRuntime
	if err == nil {
		t.Fatal("ErrNoRuntime should not be nil")
	}

	msg := err.Error()
	if msg == "" {
		t.Error("NoRuntimeError.Error() should not be empty")
	}

	// Should be identifiable as a NoRuntimeError.
	var noRT *NoRuntimeError
	if !errors.As(err, &noRT) {
		t.Error("ErrNoRuntime should be identifiable via errors.As as *NoRuntimeError")
	}
}

func TestRuntimeInfo_Fields(t *testing.T) {
	info := RuntimeInfo{
		Name:    "podman",
		Path:    "/usr/bin/podman",
		Version: "podman version 5.0.0",
	}

	if info.Name != "podman" {
		t.Errorf("Name = %q, want %q", info.Name, "podman")
	}
	if info.Path != "/usr/bin/podman" {
		t.Errorf("Path = %q, want %q", info.Path, "/usr/bin/podman")
	}
	if info.Version != "podman version 5.0.0" {
		t.Errorf("Version = %q, want %q", info.Version, "podman version 5.0.0")
	}
}

func TestDetect_ReturnsResult(t *testing.T) {
	// On the test host, Detect() may or may not find a runtime.
	// Either result is valid -- we just verify it doesn't panic.
	info, err := Detect()

	if err != nil {
		// No runtime found -- that's OK for a test environment.
		var noRT *NoRuntimeError
		if !errors.As(err, &noRT) {
			t.Errorf("Detect() returned unexpected error type: %v", err)
		}
		t.Logf("no container runtime found (expected in test env): %v", err)
		return
	}

	// If a runtime was found, validate its fields.
	if info.Name != "podman" && info.Name != "docker" {
		t.Errorf("Detect().Name = %q, want 'podman' or 'docker'", info.Name)
	}
	if info.Path == "" {
		t.Error("Detect().Path should not be empty when runtime is found")
	}
	if info.Version == "" {
		t.Error("Detect().Version should not be empty when runtime is found")
	}

	t.Logf("detected runtime: %s at %s (%s)", info.Name, info.Path, info.Version)
}
