package config

import (
	"strings"
	"testing"
)

func validConfig() *Config {
	return &Config{
		Runtime: "podman",
		Image:   "harbor.internal/aibox/base:24.04",
		GVisor: GVisorConfig{
			Enabled:  true,
			Platform: "systrap",
		},
		Resources: ResourceConfig{
			CPUs:    4,
			Memory:  "8g",
			TmpSize: "2g",
		},
		Workspace: WorkspaceConfig{
			DefaultPath: ".",
			ValidateFS:  true,
		},
		Registry: RegistryConfig{
			URL:              "harbor.internal",
			VerifySignatures: true,
		},
		Logging: LoggingConfig{
			Format: "text",
			Level:  "info",
		},
	}
}

func TestValidateValidConfig(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() on valid config: %v", err)
	}
}

func TestValidateRuntime(t *testing.T) {
	for _, rt := range []string{"podman", "docker"} {
		cfg := validConfig()
		cfg.Runtime = rt
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() with runtime=%q: %v", rt, err)
		}
	}

	cfg := validConfig()
	cfg.Runtime = "containerd"
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail for runtime=containerd")
	}
}

func TestValidateImageFormat(t *testing.T) {
	tests := []struct {
		image string
		valid bool
	}{
		{"harbor.internal/aibox/base:24.04", true},
		{"docker.io/library/ubuntu:22.04", true},
		{"myimage:latest", true},
		{"", false},
		{"invalid image ref!", false},
	}

	for _, tt := range tests {
		cfg := validConfig()
		cfg.Image = tt.image
		err := cfg.Validate()
		if tt.valid && err != nil {
			t.Errorf("Validate() with image=%q should pass, got: %v", tt.image, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("Validate() with image=%q should fail", tt.image)
		}
	}
}

func TestValidateGVisorPlatform(t *testing.T) {
	for _, platform := range []string{"systrap", "ptrace"} {
		cfg := validConfig()
		cfg.GVisor.Platform = platform
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() with gvisor.platform=%q: %v", platform, err)
		}
	}

	cfg := validConfig()
	cfg.GVisor.Enabled = true
	cfg.GVisor.Platform = "kvm"
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail for gvisor.platform=kvm")
	}

	// Disabled gVisor should not validate platform.
	cfg.GVisor.Enabled = false
	cfg.GVisor.Platform = "invalid"
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should pass when gvisor disabled regardless of platform: %v", err)
	}
}

func TestValidateCPUs(t *testing.T) {
	cfg := validConfig()
	cfg.Resources.CPUs = 0
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail for cpus=0")
	}

	cfg.Resources.CPUs = -1
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail for cpus=-1")
	}

	cfg.Resources.CPUs = 1
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should pass for cpus=1: %v", err)
	}
}

func TestValidateMemory(t *testing.T) {
	tests := []struct {
		memory string
		valid  bool
	}{
		{"4g", true},
		{"8Gi", true},
		{"512m", true},
		{"2048", true},
		{"16GB", true},
		{"", false},
		{"not-a-size", false},
		{"0g", false},
	}

	for _, tt := range tests {
		cfg := validConfig()
		cfg.Resources.Memory = tt.memory
		err := cfg.Validate()
		if tt.valid && err != nil {
			t.Errorf("Validate() with memory=%q should pass, got: %v", tt.memory, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("Validate() with memory=%q should fail", tt.memory)
		}
	}
}

func TestValidateTmpSize(t *testing.T) {
	cfg := validConfig()
	cfg.Resources.TmpSize = "invalid"
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail for tmp_size=invalid")
	}

	cfg.Resources.TmpSize = ""
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should pass for empty tmp_size: %v", err)
	}
}

func TestValidateLoggingFormat(t *testing.T) {
	for _, fmt := range []string{"text", "json"} {
		cfg := validConfig()
		cfg.Logging.Format = fmt
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() with logging.format=%q: %v", fmt, err)
		}
	}

	cfg := validConfig()
	cfg.Logging.Format = "xml"
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail for logging.format=xml")
	}
}

func TestValidateLoggingLevel(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error"} {
		cfg := validConfig()
		cfg.Logging.Level = level
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() with logging.level=%q: %v", level, err)
		}
	}

	cfg := validConfig()
	cfg.Logging.Level = "trace"
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail for logging.level=trace")
	}
}

func TestValidateMultipleErrors(t *testing.T) {
	cfg := &Config{
		Runtime: "invalid",
		Image:   "",
		GVisor:  GVisorConfig{Enabled: true, Platform: "bad"},
		Resources: ResourceConfig{
			CPUs:   0,
			Memory: "",
		},
		Registry: RegistryConfig{URL: ""},
		Logging:  LoggingConfig{Format: "bad", Level: "bad"},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() should fail with multiple errors")
	}

	errMsg := err.Error()
	// Should contain multiple error messages.
	expectedSubstrings := []string{
		"runtime",
		"image",
		"gvisor.platform",
		"resources.cpus",
		"resources.memory",
		"registry.url",
		"logging.format",
		"logging.level",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(strings.ToLower(errMsg), sub) {
			t.Errorf("error message should mention %q, got: %s", sub, errMsg)
		}
	}
}

func TestIsValidMemorySize(t *testing.T) {
	valid := []string{"4g", "512m", "8Gi", "2048", "16GB", "1k", "1K"}
	for _, s := range valid {
		if !isValidMemorySize(s) {
			t.Errorf("isValidMemorySize(%q) = false, want true", s)
		}
	}

	invalid := []string{"", "abc", "0g", "-1m", "not-a-size"}
	for _, s := range invalid {
		if isValidMemorySize(s) {
			t.Errorf("isValidMemorySize(%q) = true, want false", s)
		}
	}
}
