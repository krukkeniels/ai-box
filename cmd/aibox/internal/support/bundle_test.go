package support

import (
	"strings"
	"testing"

	"github.com/aibox/aibox/internal/config"
)

func TestRedactConfig(t *testing.T) {
	cfg := &config.Config{
		Runtime: "podman",
		Image:   "ghcr.io/test/img:latest",
		Credentials: config.CredentialsConfig{
			Mode:      "vault",
			VaultAddr: "https://vault.internal:8200",
		},
		IDE: config.IDEConfig{SSHPort: 2222},
	}
	summary := RedactConfig(cfg)
	if strings.Contains(summary, "vault.internal") {
		t.Error("vault address should be redacted")
	}
	if !strings.Contains(summary, "***") {
		t.Error("redacted values should contain ***")
	}
	if !strings.Contains(summary, "podman") {
		t.Error("runtime should be visible")
	}
	if !strings.Contains(summary, "2222") {
		t.Error("SSH port should be visible")
	}
}

func TestRedactConfig_NoVault(t *testing.T) {
	cfg := &config.Config{
		Credentials: config.CredentialsConfig{Mode: "fallback"},
	}
	summary := RedactConfig(cfg)
	if strings.Contains(summary, "***") {
		t.Error("should not contain *** when no vault configured")
	}
}

func TestGenerateBundle(t *testing.T) {
	cfg := &config.Config{}
	bundle, err := GenerateBundle(cfg, BundleOptions{Redact: true})
	if err != nil {
		t.Fatalf("GenerateBundle: %v", err)
	}
	if bundle.DoctorOutput == "" {
		t.Error("expected doctor output in bundle")
	}
	if bundle.HostInfo.OS == "" {
		t.Error("expected host info")
	}
	if bundle.SSHDiagnostics == "" {
		t.Error("expected SSH diagnostics")
	}
}
