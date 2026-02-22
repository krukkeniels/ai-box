// Package falco manages Falco runtime security monitoring for AI-Box.
//
// Falco runs on the host (not inside containers) using the eBPF driver to
// detect runtime security threats such as container escape attempts,
// unexpected network connections, privilege escalation, and access to
// sensitive files. See SPEC-FINAL.md Section 19.4 and 20.1.
package falco

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aibox/aibox/internal/audit"
)

//go:embed rules.yaml
var embeddedRules []byte

//go:embed deploy.yaml
var embeddedConfig []byte

// Config holds the configuration for Falco deployment.
type Config struct {
	RulesPath   string // path to install AI-Box rules (default "/etc/aibox/falco_rules.yaml")
	ConfigPath  string // path to install Falco config (default "/etc/aibox/falco.yaml")
	AlertsPath  string // path for Falco alert output (default "/var/log/aibox/falco-alerts.jsonl")
	Driver      string // eBPF driver type: "modern_ebpf" or "ebpf" (default "modern_ebpf")
	Enabled     bool   // whether Falco monitoring is enabled
	BufPreset   int    // syscall buffer size preset, 1-8 (default 4)
	OutputRate  int    // max alert output rate per second (default 100)
	MaxBurst    int    // max alert burst size (default 200)
}

// DefaultConfig returns a Config with sensible defaults matching the spec.
func DefaultConfig() Config {
	return Config{
		RulesPath:  "/etc/aibox/falco_rules.yaml",
		ConfigPath: "/etc/aibox/falco.yaml",
		AlertsPath: "/var/log/aibox/falco-alerts.jsonl",
		Driver:     "modern_ebpf",
		Enabled:    true,
		BufPreset:  4,
		OutputRate: 100,
		MaxBurst:   200,
	}
}

// Manager manages the Falco lifecycle: install, configure, start, stop, health.
type Manager struct {
	cfg Config
}

// NewManager creates a Manager, applying defaults for any zero-value fields.
func NewManager(cfg Config) *Manager {
	if cfg.RulesPath == "" {
		cfg.RulesPath = "/etc/aibox/falco_rules.yaml"
	}
	if cfg.ConfigPath == "" {
		cfg.ConfigPath = "/etc/aibox/falco.yaml"
	}
	if cfg.AlertsPath == "" {
		cfg.AlertsPath = "/var/log/aibox/falco-alerts.jsonl"
	}
	if cfg.Driver == "" {
		cfg.Driver = "modern_ebpf"
	}
	if cfg.BufPreset == 0 {
		cfg.BufPreset = 4
	}
	if cfg.OutputRate == 0 {
		cfg.OutputRate = 100
	}
	if cfg.MaxBurst == 0 {
		cfg.MaxBurst = 200
	}
	return &Manager{cfg: cfg}
}

// Config returns the current configuration (read-only copy).
func (m *Manager) Config() Config {
	return m.cfg
}

// RulesYAML returns the embedded AI-Box Falco rules as a string.
func RulesYAML() string {
	return string(embeddedRules)
}

// DeployConfig returns the embedded Falco deployment configuration as a string.
func DeployConfig() string {
	return string(embeddedConfig)
}

// IsInstalled checks whether the Falco binary is available in PATH.
func (m *Manager) IsInstalled() bool {
	_, err := exec.LookPath("falco")
	return err == nil
}

// Version returns the installed Falco version string, or an error if Falco
// is not installed.
func (m *Manager) Version() (string, error) {
	falcoPath, err := exec.LookPath("falco")
	if err != nil {
		return "", fmt.Errorf("falco not found in PATH: %w", err)
	}

	out, err := exec.Command(falcoPath, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("falco version check failed: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

// Install installs the Falco package using the system package manager.
func (m *Manager) Install() error {
	aptPath, err := exec.LookPath("apt-get")
	if err != nil {
		return fmt.Errorf("apt-get not found: only Debian/Ubuntu systems are supported: %w", err)
	}

	slog.Info("installing falco via apt-get")
	cmd := exec.Command(aptPath, "install", "-y", "falco")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install falco: %w", err)
	}

	slog.Info("falco installed successfully")
	return nil
}

// WriteRules writes the AI-Box Falco rules to the configured path.
// Parent directories are created if they do not exist.
func (m *Manager) WriteRules(path string) error {
	if path == "" {
		path = m.cfg.RulesPath
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating rules directory %s: %w", dir, err)
	}

	if err := os.WriteFile(path, embeddedRules, 0o644); err != nil {
		return fmt.Errorf("writing falco rules to %s: %w", path, err)
	}

	slog.Info("wrote falco rules", "path", path)
	return nil
}

// WriteConfig writes the Falco deployment configuration to the configured path.
func (m *Manager) WriteConfig(path string) error {
	if path == "" {
		path = m.cfg.ConfigPath
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config directory %s: %w", dir, err)
	}

	if err := os.WriteFile(path, embeddedConfig, 0o644); err != nil {
		return fmt.Errorf("writing falco config to %s: %w", path, err)
	}

	slog.Info("wrote falco config", "path", path)
	return nil
}

// IsRunning checks whether Falco is currently running by looking for the process.
func (m *Manager) IsRunning() bool {
	// Check via systemctl first.
	if path, err := exec.LookPath("systemctl"); err == nil {
		cmd := exec.Command(path, "is-active", "--quiet", "falco")
		if err := cmd.Run(); err == nil {
			return true
		}
	}

	// Fall back to checking for the falco process.
	cmd := exec.Command("pgrep", "-x", "falco")
	return cmd.Run() == nil
}

// HealthCheck verifies that Falco is running and the alert output file exists.
func (m *Manager) HealthCheck() error {
	if !m.IsInstalled() {
		return fmt.Errorf("falco is not installed")
	}

	if !m.IsRunning() {
		return fmt.Errorf("falco is not running")
	}

	// Verify rules file exists.
	if _, err := os.Stat(m.cfg.RulesPath); err != nil {
		return fmt.Errorf("falco rules not found at %s: %w", m.cfg.RulesPath, err)
	}

	// Verify alerts directory exists (alert file may not exist yet).
	alertDir := filepath.Dir(m.cfg.AlertsPath)
	if _, err := os.Stat(alertDir); err != nil {
		return fmt.Errorf("falco alerts directory not found at %s: %w", alertDir, err)
	}

	slog.Debug("falco health check passed")
	return nil
}

// Start launches the Falco service. It first tries systemctl, then falls
// back to running falco directly.
func (m *Manager) Start() error {
	if m.IsRunning() {
		slog.Info("falco is already running")
		return nil
	}

	// Ensure alerts directory exists.
	alertDir := filepath.Dir(m.cfg.AlertsPath)
	if err := os.MkdirAll(alertDir, 0o755); err != nil {
		slog.Warn("could not create alerts directory", "path", alertDir, "error", err)
	}

	// Try systemctl first.
	if path, err := exec.LookPath("systemctl"); err == nil {
		slog.Debug("starting falco via systemctl", "path", path)
		cmd := exec.Command(path, "start", "falco")
		if out, err := cmd.CombinedOutput(); err != nil {
			slog.Warn("systemctl start falco failed, trying direct invocation",
				"error", err, "output", string(out))
		} else {
			slog.Info("falco started via systemctl")
			return nil
		}
	}

	// Fall back to direct invocation.
	falcoPath, err := exec.LookPath("falco")
	if err != nil {
		return fmt.Errorf("falco binary not found in PATH: %w", err)
	}

	slog.Debug("starting falco directly", "binary", falcoPath,
		"config", m.cfg.ConfigPath, "rules", m.cfg.RulesPath)
	cmd := exec.Command(falcoPath,
		"--config", m.cfg.ConfigPath,
		"--rules", m.cfg.RulesPath,
		"--daemon",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start falco: %s\n%s", err, string(out))
	}

	slog.Info("falco started", "config", m.cfg.ConfigPath)
	return nil
}

// Stop shuts down the Falco service.
func (m *Manager) Stop() error {
	// Try systemctl first.
	if path, err := exec.LookPath("systemctl"); err == nil {
		slog.Debug("stopping falco via systemctl")
		cmd := exec.Command(path, "stop", "falco")
		if out, err := cmd.CombinedOutput(); err != nil {
			slog.Warn("systemctl stop falco failed, trying pkill",
				"error", err, "output", string(out))
		} else {
			slog.Info("falco stopped via systemctl")
			return nil
		}
	}

	// Fall back to pkill.
	cmd := exec.Command("pkill", "-x", "falco")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stop falco: %s\n%s", err, string(out))
	}

	slog.Info("falco stopped")
	return nil
}

// Reload tells a running Falco to re-read its rules without restarting.
func (m *Manager) Reload() error {
	// Send SIGHUP via systemctl.
	if path, err := exec.LookPath("systemctl"); err == nil {
		cmd := exec.Command(path, "reload", "falco")
		if out, err := cmd.CombinedOutput(); err != nil {
			slog.Warn("systemctl reload falco failed, trying kill -HUP",
				"error", err, "output", string(out))
		} else {
			slog.Info("falco rules reloaded via systemctl")
			return nil
		}
	}

	// Fall back to kill -HUP.
	cmd := exec.Command("pkill", "-HUP", "-x", "falco")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to reload falco rules: %s\n%s", err, string(out))
	}

	slog.Info("falco rules reloaded")
	return nil
}

// ValidateRules checks that the rules file is valid Falco YAML by running
// falco --validate on it.
func (m *Manager) ValidateRules(rulesPath string) error {
	if rulesPath == "" {
		rulesPath = m.cfg.RulesPath
	}

	falcoPath, err := exec.LookPath("falco")
	if err != nil {
		return fmt.Errorf("falco not found in PATH: %w", err)
	}

	cmd := exec.Command(falcoPath, "--validate", rulesPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("falco rule validation failed: %s\n%s", err, string(out))
	}

	slog.Debug("falco rules validated", "path", rulesPath)
	return nil
}

// AlertsPath returns the configured path for Falco alert output.
func (m *Manager) AlertsPath() string {
	return m.cfg.AlertsPath
}

// RuleNames returns the names of all AI-Box Falco rules.
func RuleNames() []string {
	return []string{
		"aibox_write_outside_allowed_dirs",
		"aibox_raw_socket_open",
		"aibox_proc_environ_read",
		"aibox_ptrace_attempt",
		"aibox_connect_non_allowlisted",
		"aibox_write_sensitive_auth_files",
		"aibox_unexpected_binary",
		"aibox_shell_unusual_parent",
		"aibox_high_entropy_dns",
		"aibox_llm_payload_oversize",
	}
}

// RuleSeverities maps each rule name to its severity level.
func RuleSeverities() map[string]string {
	return map[string]string{
		"aibox_write_outside_allowed_dirs": "critical",
		"aibox_raw_socket_open":            "critical",
		"aibox_proc_environ_read":          "critical",
		"aibox_ptrace_attempt":             "critical",
		"aibox_connect_non_allowlisted":    "warning",
		"aibox_write_sensitive_auth_files":  "critical",
		"aibox_unexpected_binary":          "warning",
		"aibox_shell_unusual_parent":       "warning",
		"aibox_high_entropy_dns":           "warning",
		"aibox_llm_payload_oversize":       "warning",
	}
}

// CheckFalcoAlertOutput verifies that the alert output directory is writable.
func (m *Manager) CheckFalcoAlertOutput() error {
	dir := filepath.Dir(m.cfg.AlertsPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create alert output directory %s: %w", dir, err)
	}

	// Test write access.
	testFile := filepath.Join(dir, ".falco-write-test")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		return fmt.Errorf("alert output directory %s is not writable: %w", dir, err)
	}
	os.Remove(testFile)

	return nil
}

// FalcoAlert represents a parsed Falco JSON alert from the alert output file.
type FalcoAlert struct {
	Time         string            `json:"time"`
	Rule         string            `json:"rule"`
	Priority     string            `json:"priority"`
	Output       string            `json:"output"`
	OutputFields map[string]string `json:"output_fields"`
}

// ParseFalcoAlert parses a single Falco JSON alert line.
func ParseFalcoAlert(data []byte) (*FalcoAlert, error) {
	var alert FalcoAlert
	if err := json.Unmarshal(data, &alert); err != nil {
		return nil, fmt.Errorf("parsing falco alert: %w", err)
	}
	return &alert, nil
}

// falcoPriorityToSeverity maps Falco priority strings to audit.Severity.
var falcoPriorityToSeverity = map[string]audit.Severity{
	"EMERGENCY":     audit.SeverityCritical,
	"ALERT":         audit.SeverityCritical,
	"CRITICAL":      audit.SeverityCritical,
	"ERROR":         audit.SeverityHigh,
	"WARNING":       audit.SeverityWarning,
	"NOTICE":        audit.SeverityInfo,
	"INFORMATIONAL": audit.SeverityInfo,
	"DEBUG":         audit.SeverityInfo,
}

// ToAuditEvent converts a FalcoAlert into an audit.AuditEvent for the log pipeline.
func (a *FalcoAlert) ToAuditEvent() audit.AuditEvent {
	severity, ok := falcoPriorityToSeverity[strings.ToUpper(a.Priority)]
	if !ok {
		severity = audit.SeverityWarning
	}

	ts, err := time.Parse(time.RFC3339Nano, a.Time)
	if err != nil {
		ts = time.Now().UTC()
	}

	sandboxID := a.OutputFields["container.label.aibox.sandbox_id"]
	if sandboxID == "" {
		sandboxID = a.OutputFields["container.name"]
	}

	return audit.AuditEvent{
		Timestamp: ts,
		EventType: audit.EventFalcoAlert,
		SandboxID: sandboxID,
		UserID:    a.OutputFields["user.name"],
		Source:    audit.SourceFalco,
		Severity:  severity,
		Details: map[string]any{
			"rule":     a.Rule,
			"priority": a.Priority,
			"output":   a.Output,
			"command":  a.OutputFields["proc.cmdline"],
			"file":     a.OutputFields["fd.name"],
			"image":    a.OutputFields["container.image.repository"],
		},
	}
}

// RuleSeverityToAudit maps a rule severity string to audit.Severity.
func RuleSeverityToAudit(severity string) audit.Severity {
	switch strings.ToLower(severity) {
	case "critical":
		return audit.SeverityCritical
	case "warning":
		return audit.SeverityWarning
	case "info":
		return audit.SeverityInfo
	default:
		return audit.SeverityWarning
	}
}

