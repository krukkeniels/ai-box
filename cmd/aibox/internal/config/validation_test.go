package config

import (
	"strings"
	"testing"
)

// fullValidConfig returns a Config that passes all validation rules.
func fullValidConfig() *Config {
	return &Config{
		ConfigVersion: 1,
		Runtime:       "podman",
		Image:         "ghcr.io/krukkeniels/aibox/base:24.04",
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
			URL: "ghcr.io/krukkeniels/aibox",
		},
		Network: NetworkConfig{
			Enabled:   false,
			ProxyPort: 3128,
			DNSPort:   53,
		},
		Credentials: CredentialsConfig{
			Mode: "fallback",
		},
		Logging: LoggingConfig{
			Format: "text",
			Level:  "info",
		},
		IDE: IDEConfig{
			SSHPort: 2222,
		},
		Audit: AuditConfig{
			StorageBackend:  "local",
			RecordingPolicy: "disabled",
			LLMLoggingMode:  "hash",
			RuntimeBackend:  "none",
		},
		Shell: "bash",
	}
}

func TestValidateValidConfigPasses(t *testing.T) {
	result := Validate(fullValidConfig())
	if result.HasErrors() {
		t.Errorf("valid config should have no errors, got:\n%s", result.String())
	}
	if result.HasWarnings() {
		t.Errorf("valid config should have no warnings, got:\n%s", result.String())
	}
}

func TestValidateErrorChecks(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(c *Config)
		field   string
		wantErr bool
	}{
		{
			name:    "resources.cpus zero",
			modify:  func(c *Config) { c.Resources.CPUs = 0 },
			field:   "resources.cpus",
			wantErr: true,
		},
		{
			name:    "resources.cpus negative",
			modify:  func(c *Config) { c.Resources.CPUs = -1 },
			field:   "resources.cpus",
			wantErr: true,
		},
		{
			name:    "resources.memory empty",
			modify:  func(c *Config) { c.Resources.Memory = "" },
			field:   "resources.memory",
			wantErr: true,
		},
		{
			name:    "resources.memory invalid",
			modify:  func(c *Config) { c.Resources.Memory = "not-a-size" },
			field:   "resources.memory",
			wantErr: true,
		},
		{
			name:    "resources.tmp_size invalid",
			modify:  func(c *Config) { c.Resources.TmpSize = "bad" },
			field:   "resources.tmp_size",
			wantErr: true,
		},
		{
			name:    "logging.format invalid",
			modify:  func(c *Config) { c.Logging.Format = "xml" },
			field:   "logging.format",
			wantErr: true,
		},
		{
			name:    "logging.level invalid",
			modify:  func(c *Config) { c.Logging.Level = "trace" },
			field:   "logging.level",
			wantErr: true,
		},
		{
			name:    "credentials.mode invalid",
			modify:  func(c *Config) { c.Credentials.Mode = "plaintext" },
			field:   "credentials.mode",
			wantErr: true,
		},
		{
			name:    "audit.storage_backend invalid",
			modify:  func(c *Config) { c.Audit.StorageBackend = "gcs" },
			field:   "audit.storage_backend",
			wantErr: true,
		},
		{
			name:    "audit.recording_policy invalid",
			modify:  func(c *Config) { c.Audit.RecordingPolicy = "always" },
			field:   "audit.recording_policy",
			wantErr: true,
		},
		{
			name:    "audit.llm_logging_mode invalid",
			modify:  func(c *Config) { c.Audit.LLMLoggingMode = "plaintext" },
			field:   "audit.llm_logging_mode",
			wantErr: true,
		},
		{
			name:    "audit.runtime_backend invalid",
			modify:  func(c *Config) { c.Audit.RuntimeBackend = "bpf" },
			field:   "audit.runtime_backend",
			wantErr: true,
		},
		{
			name:    "shell invalid",
			modify:  func(c *Config) { c.Shell = "fish" },
			field:   "shell",
			wantErr: true,
		},
		{
			name:    "runtime invalid",
			modify:  func(c *Config) { c.Runtime = "containerd" },
			field:   "runtime",
			wantErr: true,
		},
		{
			name:    "ide.ssh_port zero",
			modify:  func(c *Config) { c.IDE.SSHPort = 0 },
			field:   "ide.ssh_port",
			wantErr: true,
		},
		{
			name:    "ide.ssh_port too high",
			modify:  func(c *Config) { c.IDE.SSHPort = 70000 },
			field:   "ide.ssh_port",
			wantErr: true,
		},
		{
			name:    "network.proxy_port zero",
			modify:  func(c *Config) { c.Network.ProxyPort = 0 },
			field:   "network.proxy_port",
			wantErr: true,
		},
		{
			name:    "network.dns_port too high",
			modify:  func(c *Config) { c.Network.DNSPort = 100000 },
			field:   "network.dns_port",
			wantErr: true,
		},
		// Valid values should not produce errors.
		{
			name:    "valid resources.memory 16g",
			modify:  func(c *Config) { c.Resources.Memory = "16g" },
			field:   "resources.memory",
			wantErr: false,
		},
		{
			name:    "valid logging.format json",
			modify:  func(c *Config) { c.Logging.Format = "json" },
			field:   "logging.format",
			wantErr: false,
		},
		{
			name:    "valid credentials.mode vault with addr",
			modify: func(c *Config) {
				c.Credentials.Mode = "vault"
				c.Credentials.VaultAddr = "https://vault.internal:8200"
			},
			field:   "credentials.mode",
			wantErr: false,
		},
		{
			name:    "valid audit.storage_backend s3",
			modify:  func(c *Config) { c.Audit.StorageBackend = "s3" },
			field:   "audit.storage_backend",
			wantErr: false,
		},
		{
			name:    "valid shell zsh",
			modify:  func(c *Config) { c.Shell = "zsh" },
			field:   "shell",
			wantErr: false,
		},
		{
			name:    "valid runtime docker",
			modify:  func(c *Config) { c.Runtime = "docker" },
			field:   "runtime",
			wantErr: false,
		},
		{
			name:    "valid ide.ssh_port 65535",
			modify:  func(c *Config) { c.IDE.SSHPort = 65535 },
			field:   "ide.ssh_port",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := fullValidConfig()
			tt.modify(cfg)
			result := Validate(cfg)

			hasFieldError := false
			for _, e := range result.Errors {
				if e.Field == tt.field {
					hasFieldError = true
					break
				}
			}

			if tt.wantErr && !hasFieldError {
				t.Errorf("expected error for field %q, got none. Result:\n%s", tt.field, result.String())
			}
			if !tt.wantErr && hasFieldError {
				t.Errorf("did not expect error for field %q, got:\n%s", tt.field, result.String())
			}
		})
	}
}

func TestValidateWarnings(t *testing.T) {
	tests := []struct {
		name   string
		modify func(c *Config)
		field  string
	}{
		{
			name:   "gvisor.platform unknown",
			modify: func(c *Config) { c.GVisor.Platform = "kvm" },
			field:  "gvisor.platform",
		},
		{
			name: "credentials.vault_addr missing in vault mode",
			modify: func(c *Config) {
				c.Credentials.Mode = "vault"
				c.Credentials.VaultAddr = ""
			},
			field: "credentials.vault_addr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := fullValidConfig()
			tt.modify(cfg)
			result := Validate(cfg)

			hasWarning := false
			for _, w := range result.Warnings {
				if w.Field == tt.field {
					hasWarning = true
					break
				}
			}
			if !hasWarning {
				t.Errorf("expected warning for field %q, got none. Result:\n%s", tt.field, result.String())
			}
		})
	}
}

func TestValidateNoWarningsForValidPlatforms(t *testing.T) {
	for _, p := range []string{"systrap", "ptrace"} {
		cfg := fullValidConfig()
		cfg.GVisor.Platform = p
		result := Validate(cfg)
		if result.HasWarnings() {
			t.Errorf("gvisor.platform=%q should not produce warnings, got:\n%s", p, result.String())
		}
	}
}

func TestValidationResultString(t *testing.T) {
	r := &ValidationResult{}
	if r.String() != "config validation passed" {
		t.Errorf("empty result String() = %q, want %q", r.String(), "config validation passed")
	}

	r.addError("shell", "fish", "must be bash, zsh, or pwsh")
	r.addWarning("gvisor.platform", "kvm", "should be systrap or ptrace")

	s := r.String()
	if !strings.Contains(s, "ERROR") {
		t.Error("String() should contain ERROR prefix")
	}
	if !strings.Contains(s, "WARN") {
		t.Error("String() should contain WARN prefix")
	}
	if !strings.Contains(s, "shell") {
		t.Error("String() should mention shell field")
	}
}

func TestValidationIssueString(t *testing.T) {
	issue := ValidationIssue{Field: "shell", Value: "fish", Message: "invalid"}
	s := issue.String()
	if !strings.Contains(s, "shell") || !strings.Contains(s, "fish") {
		t.Errorf("ValidationIssue.String() = %q, should contain field and value", s)
	}

	issue2 := ValidationIssue{Field: "credentials.vault_addr", Message: "should be set"}
	s2 := issue2.String()
	if strings.Contains(s2, "got") {
		t.Errorf("ValidationIssue.String() with empty value should not contain 'got', got %q", s2)
	}
}

func TestValidateMultipleErrorsAndWarnings(t *testing.T) {
	cfg := &Config{
		Runtime: "invalid",
		Shell:   "fish",
		Resources: ResourceConfig{
			CPUs:   0,
			Memory: "",
		},
		Logging: LoggingConfig{
			Format: "xml",
			Level:  "trace",
		},
		Credentials: CredentialsConfig{
			Mode: "plaintext",
		},
		GVisor: GVisorConfig{
			Platform: "kvm",
		},
		Network: NetworkConfig{
			ProxyPort: 0,
			DNSPort:   0,
		},
		IDE: IDEConfig{
			SSHPort: 0,
		},
		Audit: AuditConfig{
			StorageBackend:  "gcs",
			RecordingPolicy: "always",
			LLMLoggingMode:  "plaintext",
			RuntimeBackend:  "bpf",
		},
	}

	result := Validate(cfg)
	if !result.HasErrors() {
		t.Fatal("expected errors for fully invalid config")
	}

	// Should have errors for each invalid field.
	expectedErrors := []string{
		"resources.cpus", "resources.memory", "logging.format", "logging.level",
		"credentials.mode", "audit.storage_backend", "audit.recording_policy",
		"audit.llm_logging_mode", "audit.runtime_backend", "shell", "runtime",
		"ide.ssh_port", "network.proxy_port", "network.dns_port",
	}
	for _, field := range expectedErrors {
		found := false
		for _, e := range result.Errors {
			if e.Field == field {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected error for field %q, not found in result", field)
		}
	}

	// Should have warnings for gvisor.platform and vault_addr.
	if !result.HasWarnings() {
		t.Error("expected warnings")
	}
}
