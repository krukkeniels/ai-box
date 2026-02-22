package config

import (
	"fmt"
	"strings"
)

// ValidationIssue describes a single validation problem.
type ValidationIssue struct {
	Field   string // dotted config path, e.g. "resources.cpus"
	Value   string // the invalid value as a string
	Message string // human-readable description
}

func (i ValidationIssue) String() string {
	if i.Value != "" {
		return fmt.Sprintf("%s: %s (got %q)", i.Field, i.Message, i.Value)
	}
	return fmt.Sprintf("%s: %s", i.Field, i.Message)
}

// ValidationResult collects errors and warnings from config validation.
type ValidationResult struct {
	Errors   []ValidationIssue
	Warnings []ValidationIssue
}

// HasErrors returns true if there are any validation errors.
func (r *ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// HasWarnings returns true if there are any validation warnings.
func (r *ValidationResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}

// String returns a formatted summary of all errors and warnings.
func (r *ValidationResult) String() string {
	if !r.HasErrors() && !r.HasWarnings() {
		return "config validation passed"
	}

	var b strings.Builder
	for _, e := range r.Errors {
		fmt.Fprintf(&b, "ERROR  %s\n", e.String())
	}
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "WARN   %s\n", w.String())
	}
	return strings.TrimRight(b.String(), "\n")
}

func (r *ValidationResult) addError(field, value, message string) {
	r.Errors = append(r.Errors, ValidationIssue{Field: field, Value: value, Message: message})
}

func (r *ValidationResult) addWarning(field, value, message string) {
	r.Warnings = append(r.Warnings, ValidationIssue{Field: field, Value: value, Message: message})
}

// Validate checks cfg against all known rules and returns a ValidationResult.
func Validate(cfg *Config) *ValidationResult {
	r := &ValidationResult{}

	// --- ERROR checks ---

	// resources.cpus must be > 0
	if cfg.Resources.CPUs <= 0 {
		r.addError("resources.cpus", fmt.Sprintf("%d", cfg.Resources.CPUs), "must be greater than 0")
	}

	// resources.memory must be parseable
	if cfg.Resources.Memory == "" {
		r.addError("resources.memory", "", "must not be empty")
	} else if !isValidMemorySize(cfg.Resources.Memory) {
		r.addError("resources.memory", cfg.Resources.Memory, "must be a valid size (e.g. \"8g\", \"16384m\")")
	}

	// resources.tmp_size must be parseable if set
	if cfg.Resources.TmpSize != "" && !isValidMemorySize(cfg.Resources.TmpSize) {
		r.addError("resources.tmp_size", cfg.Resources.TmpSize, "must be a valid size (e.g. \"2g\", \"1024m\")")
	}

	// logging.format
	switch cfg.Logging.Format {
	case "text", "json":
	default:
		r.addError("logging.format", cfg.Logging.Format, "must be \"text\" or \"json\"")
	}

	// logging.level
	switch strings.ToLower(cfg.Logging.Level) {
	case "debug", "info", "warn", "error":
	default:
		r.addError("logging.level", cfg.Logging.Level, "must be \"debug\", \"info\", \"warn\", or \"error\"")
	}

	// credentials.mode
	switch cfg.Credentials.Mode {
	case "fallback", "vault", "":
	default:
		r.addError("credentials.mode", cfg.Credentials.Mode, "must be \"fallback\" or \"vault\"")
	}

	// audit.storage_backend
	if cfg.Audit.StorageBackend != "" {
		switch cfg.Audit.StorageBackend {
		case "local", "minio", "s3":
		default:
			r.addError("audit.storage_backend", cfg.Audit.StorageBackend, "must be \"local\", \"minio\", or \"s3\"")
		}
	}

	// audit.recording_policy
	if cfg.Audit.RecordingPolicy != "" {
		switch cfg.Audit.RecordingPolicy {
		case "required", "optional", "disabled":
		default:
			r.addError("audit.recording_policy", cfg.Audit.RecordingPolicy, "must be \"required\", \"optional\", or \"disabled\"")
		}
	}

	// audit.llm_logging_mode
	if cfg.Audit.LLMLoggingMode != "" {
		switch cfg.Audit.LLMLoggingMode {
		case "full", "hash", "metadata_only":
		default:
			r.addError("audit.llm_logging_mode", cfg.Audit.LLMLoggingMode, "must be \"full\", \"hash\", or \"metadata_only\"")
		}
	}

	// audit.runtime_backend
	if cfg.Audit.RuntimeBackend != "" {
		switch cfg.Audit.RuntimeBackend {
		case "falco", "auditd", "none":
		default:
			r.addError("audit.runtime_backend", cfg.Audit.RuntimeBackend, "must be \"falco\", \"auditd\", or \"none\"")
		}
	}

	// shell
	switch cfg.Shell {
	case "bash", "zsh", "pwsh":
	default:
		r.addError("shell", cfg.Shell, "must be \"bash\", \"zsh\", or \"pwsh\"")
	}

	// runtime
	switch cfg.Runtime {
	case "podman", "docker":
	default:
		r.addError("runtime", cfg.Runtime, "must be \"podman\" or \"docker\"")
	}

	// ide.ssh_port
	if cfg.IDE.SSHPort < 1 || cfg.IDE.SSHPort > 65535 {
		r.addError("ide.ssh_port", fmt.Sprintf("%d", cfg.IDE.SSHPort), "must be between 1 and 65535")
	}

	// network.proxy_port
	if cfg.Network.ProxyPort < 1 || cfg.Network.ProxyPort > 65535 {
		r.addError("network.proxy_port", fmt.Sprintf("%d", cfg.Network.ProxyPort), "must be between 1 and 65535")
	}

	// network.dns_port
	if cfg.Network.DNSPort < 1 || cfg.Network.DNSPort > 65535 {
		r.addError("network.dns_port", fmt.Sprintf("%d", cfg.Network.DNSPort), "must be between 1 and 65535")
	}

	// --- WARNING checks ---

	// gvisor.platform should be systrap or ptrace
	if cfg.GVisor.Platform != "" {
		switch cfg.GVisor.Platform {
		case "systrap", "ptrace":
		default:
			r.addWarning("gvisor.platform", cfg.GVisor.Platform, "should be \"systrap\" or \"ptrace\"")
		}
	}

	// credentials.vault_addr should be set if mode is vault
	if cfg.Credentials.Mode == "vault" && cfg.Credentials.VaultAddr == "" {
		r.addWarning("credentials.vault_addr", "", "should be set when credentials.mode is \"vault\"")
	}

	return r
}
