package security

import (
	"os"
	"strings"
	"testing"
)

func TestIsAppArmorAvailable(t *testing.T) {
	// This test checks the actual system state. The result depends on
	// whether the test host has AppArmor enabled.
	result := IsAppArmorAvailable()

	_, err := os.Stat("/sys/module/apparmor")
	expected := err == nil

	if result != expected {
		t.Errorf("IsAppArmorAvailable() = %v, expected %v based on /sys/module/apparmor", result, expected)
	}
}

func TestIsProfileLoaded_NonExistentProfile(t *testing.T) {
	if !IsAppArmorAvailable() {
		t.Skip("AppArmor not available on this system")
	}

	loaded, err := IsProfileLoaded("nonexistent-profile-abc123")
	if err != nil {
		// Permission denied is expected when running without root.
		if os.IsPermission(err) || strings.Contains(err.Error(), "permission denied") {
			t.Skip("AppArmor profiles not readable without root")
		}
		t.Fatalf("IsProfileLoaded() returned error: %v", err)
	}
	if loaded {
		t.Error("IsProfileLoaded() returned true for a non-existent profile")
	}
}

func TestIsProfileLoaded_NoAppArmor(t *testing.T) {
	if IsAppArmorAvailable() {
		t.Skip("AppArmor is available; cannot test missing AppArmor path")
	}

	_, err := IsProfileLoaded("anything")
	if err == nil {
		t.Error("IsProfileLoaded() should return error when AppArmor profiles file is not readable")
	}
}

func TestEnsureProfile_NoAppArmor(t *testing.T) {
	if IsAppArmorAvailable() {
		t.Skip("AppArmor is available; cannot test degraded mode")
	}

	// When AppArmor is not available, EnsureProfile should return nil
	// (graceful degradation).
	err := EnsureProfile("/nonexistent/path")
	if err != nil {
		t.Errorf("EnsureProfile() should return nil when AppArmor is unavailable, got: %v", err)
	}
}
