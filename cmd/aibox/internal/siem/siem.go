// Package siem provides SIEM integration for AI-Box including detection rules,
// alert routing, and Vector sink configuration generation. Detection rules
// correlate events from multiple sources (Squid, CoreDNS, Falco, OPA, Vault,
// LLM proxy) to detect AI-Box-specific threat patterns.
//
// See SPEC-FINAL.md Section 19.5 and Phase 5 plan Work Stream 4.
package siem

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the top-level SIEM integration configuration.
type Config struct {
	Enabled    bool       // whether SIEM integration is active
	Sink       SinkConfig // Vector sink configuration
	ConfigPath string     // path to write generated SIEM Vector config (default "/etc/aibox/siem-sink.toml")
	Rules      []DetectionRule // active detection rules (defaults to DefaultRules)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:    false,
		Sink:       DefaultSinkConfig(),
		ConfigPath: "/etc/aibox/siem-sink.toml",
		Rules:      DefaultRules(),
	}
}

// Manager manages SIEM integration lifecycle: configuration, rule management,
// and Vector sink config generation.
type Manager struct {
	cfg Config
}

// NewManager creates a Manager, applying defaults for any zero-value fields.
func NewManager(cfg Config) *Manager {
	if cfg.ConfigPath == "" {
		cfg.ConfigPath = "/etc/aibox/siem-sink.toml"
	}
	if cfg.Rules == nil {
		cfg.Rules = DefaultRules()
	}
	if cfg.Sink.Format == "" {
		cfg.Sink.Format = "json"
	}
	return &Manager{cfg: cfg}
}

// Config returns the current configuration (read-only copy).
func (m *Manager) Config() Config {
	return m.cfg
}

// EnabledRuleCount returns the number of enabled detection rules.
func (m *Manager) EnabledRuleCount() int {
	count := 0
	for _, r := range m.cfg.Rules {
		if r.Enabled {
			count++
		}
	}
	return count
}

// GenerateFullConfig produces a complete Vector TOML snippet that includes
// the SIEM sink plus transform rules for detection/alerting. This is appended
// to the main Vector config.
func (m *Manager) GenerateFullConfig() string {
	var b strings.Builder

	// Sink configuration.
	b.WriteString(GenerateVectorSink(m.cfg.Sink))
	b.WriteString("\n")

	// Detection rule metadata as a comment block for operator reference.
	b.WriteString("# =============================================================================\n")
	b.WriteString("# AI-Box SIEM Detection Rules (Reference)\n")
	b.WriteString("# These rules are evaluated by the SIEM platform, not by Vector.\n")
	b.WriteString("# Import the rule definitions into your SIEM via `aibox siem export-rules`.\n")
	b.WriteString("# =============================================================================\n")

	enabled := EnabledRules(m.cfg.Rules)
	for _, r := range enabled {
		fmt.Fprintf(&b, "# [%s] %s (%s) — %s\n", r.ID, r.Name, r.Severity, r.Condition)
	}

	return b.String()
}

// WriteConfig writes the generated SIEM sink configuration to the configured path.
func (m *Manager) WriteConfig(path string) error {
	if path == "" {
		path = m.cfg.ConfigPath
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config directory %s: %w", dir, err)
	}

	content := m.GenerateFullConfig()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing siem config to %s: %w", path, err)
	}

	slog.Info("wrote siem config", "path", path, "sink_type", m.cfg.Sink.Type, "rules", m.EnabledRuleCount())
	return nil
}

// RuleSummary returns a human-readable summary of detection rule status.
func (m *Manager) RuleSummary() string {
	var b strings.Builder

	bySev := make(map[Severity]int)
	for _, r := range m.cfg.Rules {
		if r.Enabled {
			bySev[r.Severity]++
		}
	}

	fmt.Fprintf(&b, "SIEM Detection Rules: %d/%d enabled\n", m.EnabledRuleCount(), len(m.cfg.Rules))
	fmt.Fprintf(&b, "  Critical: %d (-> PagerDuty, 15 min SLA)\n", bySev[SeverityCritical])
	fmt.Fprintf(&b, "  High:     %d (-> Slack, 4 hour SLA)\n", bySev[SeverityHigh])
	fmt.Fprintf(&b, "  Medium:   %d (-> Email, next business day)\n", bySev[SeverityMedium])
	fmt.Fprintf(&b, "  Info:     %d (-> Dashboard, weekly review)\n", bySev[SeverityInfo])

	return b.String()
}

// ValidateRules checks that all rules have required fields and valid severities.
func ValidateRules(rules []DetectionRule) []string {
	var errors []string
	validSeverities := map[Severity]bool{
		SeverityCritical: true,
		SeverityHigh:     true,
		SeverityMedium:   true,
		SeverityInfo:     true,
	}

	seen := make(map[string]bool)
	for _, r := range rules {
		if r.ID == "" {
			errors = append(errors, "rule has empty ID")
			continue
		}
		if seen[r.ID] {
			errors = append(errors, fmt.Sprintf("duplicate rule ID: %s", r.ID))
		}
		seen[r.ID] = true

		if r.Name == "" {
			errors = append(errors, fmt.Sprintf("rule %s has empty Name", r.ID))
		}
		if r.Description == "" {
			errors = append(errors, fmt.Sprintf("rule %s has empty Description", r.ID))
		}
		if !validSeverities[r.Severity] {
			errors = append(errors, fmt.Sprintf("rule %s has invalid severity: %q", r.ID, r.Severity))
		}
		if r.Channel == "" {
			errors = append(errors, fmt.Sprintf("rule %s has empty Channel", r.ID))
		}
		if len(r.Sources) == 0 {
			errors = append(errors, fmt.Sprintf("rule %s has no sources", r.ID))
		}
		if r.Condition == "" {
			errors = append(errors, fmt.Sprintf("rule %s has empty Condition", r.ID))
		}
	}

	return errors
}
