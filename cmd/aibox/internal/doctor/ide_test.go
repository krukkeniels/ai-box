package doctor

import (
	"testing"

	"github.com/aibox/aibox/internal/config"
	"github.com/aibox/aibox/internal/host"
)

func TestCheckSSHKeyExists(t *testing.T) {
	result := CheckSSHKeyExists()
	if result.Name != "SSH Key" {
		t.Errorf("expected name 'SSH Key', got %q", result.Name)
	}
	validStatuses := map[string]bool{StatusPass: true, StatusWarn: true, StatusFail: true}
	if !validStatuses[result.Status] {
		t.Errorf("unexpected status: %q", result.Status)
	}
}

func TestCheckSSHConfig(t *testing.T) {
	result := CheckSSHConfig()
	if result.Name != "SSH Config" {
		t.Errorf("expected name 'SSH Config', got %q", result.Name)
	}
}

func TestCheckSSHPort_NotConfigured(t *testing.T) {
	result := CheckSSHPort(0)
	if result.Status != StatusInfo {
		t.Errorf("port 0 should be info, got %q", result.Status)
	}
}

func TestCheckSSHHandshake_NotConfigured(t *testing.T) {
	result := CheckSSHHandshake(0)
	if result.Status != StatusInfo {
		t.Errorf("port 0 should be info, got %q", result.Status)
	}
}

func TestCheckIDEEnvironment_WSL(t *testing.T) {
	result := CheckIDEEnvironment(host.HostInfo{IsWSL2: true})
	if result.Status != StatusInfo {
		t.Errorf("expected info, got %q", result.Status)
	}
	if result.Name != "IDE Environment" {
		t.Errorf("expected name 'IDE Environment', got %q", result.Name)
	}
}

func TestCheckIDEEnvironment_Native(t *testing.T) {
	result := CheckIDEEnvironment(host.HostInfo{IsWSL2: false})
	if result.Status != StatusInfo {
		t.Errorf("expected info, got %q", result.Status)
	}
}

func TestRunIDEChecks(t *testing.T) {
	cfg := &config.Config{}
	cfg.IDE.SSHPort = 0 // avoid actual probing
	report := RunIDEChecks(cfg)
	if len(report.Results) == 0 {
		t.Error("expected at least one check result")
	}
	if len(report.Results) != 5 {
		t.Errorf("expected 5 IDE checks, got %d", len(report.Results))
	}
}
