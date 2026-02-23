package setup

import (
	"testing"

	"github.com/aibox/aibox/internal/host"
)

func TestWSLDevProfile_OnWSL(t *testing.T) {
	hostInfo := host.HostInfo{OS: "linux", IsWSL2: true, KernelVersion: "5.15.90.1-microsoft-standard-WSL2"}
	steps := WSLDevProfileSteps(hostInfo)
	if len(steps) == 0 {
		t.Error("expected WSL dev profile to have steps")
	}

	// Should include SSH key generation.
	hasSSH := false
	hasWSLInstructions := false
	for _, s := range steps {
		if s.Name == "Generate SSH keys" {
			hasSSH = true
		}
		if s.Name == "Print WSL-specific instructions" {
			hasWSLInstructions = true
		}
	}
	if !hasSSH {
		t.Error("WSL dev profile should include SSH key generation")
	}
	if !hasWSLInstructions {
		t.Error("WSL dev profile should include WSL-specific instructions on WSL2")
	}
}

func TestWSLDevProfile_NotWSL(t *testing.T) {
	hostInfo := host.HostInfo{OS: "linux", IsWSL2: false}
	steps := WSLDevProfileSteps(hostInfo)
	if len(steps) == 0 {
		t.Error("expected steps even on native Linux")
	}

	// Should NOT include WSL-specific instructions.
	for _, s := range steps {
		if s.Name == "Print WSL-specific instructions" {
			t.Error("native Linux profile should not include WSL-specific instructions")
		}
	}
}

func TestRunProfile_UnknownProfile(t *testing.T) {
	err := RunProfile("nonexistent")
	if err == nil {
		t.Error("expected error for unknown profile")
	}
}
