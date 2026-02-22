package falco

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aibox/aibox/internal/audit"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.RulesPath != "/etc/aibox/falco_rules.yaml" {
		t.Errorf("RulesPath = %q, want /etc/aibox/falco_rules.yaml", cfg.RulesPath)
	}
	if cfg.ConfigPath != "/etc/aibox/falco.yaml" {
		t.Errorf("ConfigPath = %q, want /etc/aibox/falco.yaml", cfg.ConfigPath)
	}
	if cfg.AlertsPath != "/var/log/aibox/falco-alerts.jsonl" {
		t.Errorf("AlertsPath = %q, want /var/log/aibox/falco-alerts.jsonl", cfg.AlertsPath)
	}
	if cfg.Driver != "modern_ebpf" {
		t.Errorf("Driver = %q, want modern_ebpf", cfg.Driver)
	}
	if !cfg.Enabled {
		t.Error("Enabled should be true by default")
	}
	if cfg.BufPreset != 4 {
		t.Errorf("BufPreset = %d, want 4", cfg.BufPreset)
	}
	if cfg.OutputRate != 100 {
		t.Errorf("OutputRate = %d, want 100", cfg.OutputRate)
	}
	if cfg.MaxBurst != 200 {
		t.Errorf("MaxBurst = %d, want 200", cfg.MaxBurst)
	}
}

func TestNewManager_DefaultsFilled(t *testing.T) {
	mgr := NewManager(Config{})

	cfg := mgr.Config()
	if cfg.RulesPath == "" {
		t.Error("RulesPath should have default")
	}
	if cfg.ConfigPath == "" {
		t.Error("ConfigPath should have default")
	}
	if cfg.AlertsPath == "" {
		t.Error("AlertsPath should have default")
	}
	if cfg.Driver == "" {
		t.Error("Driver should have default")
	}
	if cfg.BufPreset == 0 {
		t.Error("BufPreset should have default")
	}
	if cfg.OutputRate == 0 {
		t.Error("OutputRate should have default")
	}
	if cfg.MaxBurst == 0 {
		t.Error("MaxBurst should have default")
	}
}

func TestNewManager_CustomConfig(t *testing.T) {
	mgr := NewManager(Config{
		RulesPath:  "/custom/rules.yaml",
		ConfigPath: "/custom/falco.yaml",
		AlertsPath: "/custom/alerts.jsonl",
		Driver:     "ebpf",
		BufPreset:  2,
		OutputRate: 50,
		MaxBurst:   100,
	})

	cfg := mgr.Config()
	if cfg.RulesPath != "/custom/rules.yaml" {
		t.Errorf("RulesPath = %q, want /custom/rules.yaml", cfg.RulesPath)
	}
	if cfg.ConfigPath != "/custom/falco.yaml" {
		t.Errorf("ConfigPath = %q, want /custom/falco.yaml", cfg.ConfigPath)
	}
	if cfg.AlertsPath != "/custom/alerts.jsonl" {
		t.Errorf("AlertsPath = %q, want /custom/alerts.jsonl", cfg.AlertsPath)
	}
	if cfg.Driver != "ebpf" {
		t.Errorf("Driver = %q, want ebpf", cfg.Driver)
	}
	if cfg.BufPreset != 2 {
		t.Errorf("BufPreset = %d, want 2", cfg.BufPreset)
	}
}

func TestRulesYAML_NotEmpty(t *testing.T) {
	rules := RulesYAML()
	if rules == "" {
		t.Fatal("RulesYAML() returned empty string")
	}
}

func TestRulesYAML_ContainsAllRules(t *testing.T) {
	rules := RulesYAML()

	expectedRules := RuleNames()
	for _, name := range expectedRules {
		if !strings.Contains(rules, name) {
			t.Errorf("rules YAML does not contain rule %q", name)
		}
	}
}

func TestRulesYAML_HasCorrectRuleCount(t *testing.T) {
	rules := RulesYAML()

	// Count "- rule:" directives.
	count := strings.Count(rules, "- rule:")
	if count != 10 {
		t.Errorf("expected 10 rules, found %d", count)
	}
}

func TestRulesYAML_AllRulesHavePriority(t *testing.T) {
	rules := RulesYAML()

	// Every rule must have a priority field.
	ruleCount := strings.Count(rules, "- rule:")
	priorityCount := strings.Count(rules, "priority:")
	if ruleCount != priorityCount {
		t.Errorf("found %d rules but only %d priority fields", ruleCount, priorityCount)
	}
}

func TestRulesYAML_AllRulesHaveCondition(t *testing.T) {
	rules := RulesYAML()

	ruleCount := strings.Count(rules, "- rule:")
	conditionCount := strings.Count(rules, "condition:")
	if ruleCount != conditionCount {
		t.Errorf("found %d rules but only %d condition fields", ruleCount, conditionCount)
	}
}

func TestRulesYAML_AllRulesHaveOutput(t *testing.T) {
	rules := RulesYAML()

	ruleCount := strings.Count(rules, "- rule:")
	outputCount := strings.Count(rules, "  output:")
	if ruleCount != outputCount {
		t.Errorf("found %d rules but only %d output fields", ruleCount, outputCount)
	}
}

func TestRulesYAML_AllRulesHaveAiboxTag(t *testing.T) {
	rules := RulesYAML()

	// Every tags field should contain "aibox".
	tagLines := 0
	for _, line := range strings.Split(rules, "\n") {
		if strings.Contains(line, "tags:") {
			tagLines++
			if !strings.Contains(line, "aibox") {
				t.Errorf("tags line missing 'aibox' tag: %s", line)
			}
		}
	}
	if tagLines != 10 {
		t.Errorf("expected 10 tags lines, found %d", tagLines)
	}
}

func TestRulesYAML_ContainerScopedRules(t *testing.T) {
	rules := RulesYAML()

	// All rule conditions should include container scoping.
	conditionCount := strings.Count(rules, "container.id != host")
	if conditionCount != 10 {
		t.Errorf("expected 10 container-scoped conditions, found %d", conditionCount)
	}
}

func TestRulesYAML_CriticalRules(t *testing.T) {
	rules := RulesYAML()

	criticalCount := strings.Count(rules, "priority: CRITICAL")
	if criticalCount != 5 {
		t.Errorf("expected 5 CRITICAL rules, found %d", criticalCount)
	}
}

func TestRulesYAML_WarningRules(t *testing.T) {
	rules := RulesYAML()

	warningCount := strings.Count(rules, "priority: WARNING")
	if warningCount != 5 {
		t.Errorf("expected 5 WARNING rules, found %d", warningCount)
	}
}

func TestDeployConfig_NotEmpty(t *testing.T) {
	config := DeployConfig()
	if config == "" {
		t.Fatal("DeployConfig() returned empty string")
	}
}

func TestDeployConfig_UsesEBPFDriver(t *testing.T) {
	config := DeployConfig()

	if !strings.Contains(config, "modern_ebpf") {
		t.Error("deploy config should use modern_ebpf driver")
	}
}

func TestDeployConfig_PodmanEnabled(t *testing.T) {
	config := DeployConfig()

	if !strings.Contains(config, "podman:") {
		t.Error("deploy config should reference podman")
	}
}

func TestDeployConfig_FileOutput(t *testing.T) {
	config := DeployConfig()

	if !strings.Contains(config, "file_output:") {
		t.Error("deploy config should enable file output")
	}
	if !strings.Contains(config, "falco-alerts.jsonl") {
		t.Error("deploy config should output to falco-alerts.jsonl")
	}
}

func TestRuleNames_HasTenRules(t *testing.T) {
	names := RuleNames()
	if len(names) != 10 {
		t.Errorf("expected 10 rule names, got %d", len(names))
	}
}

func TestRuleNames_AllPrefixed(t *testing.T) {
	for _, name := range RuleNames() {
		if !strings.HasPrefix(name, "aibox_") {
			t.Errorf("rule name %q should have aibox_ prefix", name)
		}
	}
}

func TestRuleSeverities_AllRulesCovered(t *testing.T) {
	names := RuleNames()
	severities := RuleSeverities()

	for _, name := range names {
		sev, ok := severities[name]
		if !ok {
			t.Errorf("rule %q missing from RuleSeverities map", name)
			continue
		}
		if sev != "critical" && sev != "warning" && sev != "info" {
			t.Errorf("rule %q has invalid severity %q", name, sev)
		}
	}
}

func TestRuleSeverities_CorrectCriticalRules(t *testing.T) {
	severities := RuleSeverities()

	criticalRules := []string{
		"aibox_write_outside_allowed_dirs",
		"aibox_raw_socket_open",
		"aibox_proc_environ_read",
		"aibox_ptrace_attempt",
		"aibox_write_sensitive_auth_files",
	}

	for _, name := range criticalRules {
		if severities[name] != "critical" {
			t.Errorf("rule %q should be critical, got %q", name, severities[name])
		}
	}
}

func TestRuleSeverities_CorrectWarningRules(t *testing.T) {
	severities := RuleSeverities()

	warningRules := []string{
		"aibox_connect_non_allowlisted",
		"aibox_unexpected_binary",
		"aibox_shell_unusual_parent",
		"aibox_high_entropy_dns",
		"aibox_llm_payload_oversize",
	}

	for _, name := range warningRules {
		if severities[name] != "warning" {
			t.Errorf("rule %q should be warning, got %q", name, severities[name])
		}
	}
}

func TestWriteRules_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "rules", "falco_rules.yaml")

	mgr := NewManager(Config{RulesPath: path})
	if err := mgr.WriteRules(""); err != nil {
		t.Fatalf("WriteRules failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written rules: %v", err)
	}
	if len(data) == 0 {
		t.Error("written rules file is empty")
	}
	if !strings.Contains(string(data), "aibox_write_outside_allowed_dirs") {
		t.Error("written rules should contain first rule")
	}
}

func TestWriteRules_CustomPath(t *testing.T) {
	tmpDir := t.TempDir()
	customPath := filepath.Join(tmpDir, "custom", "my_rules.yaml")

	mgr := NewManager(Config{})
	if err := mgr.WriteRules(customPath); err != nil {
		t.Fatalf("WriteRules with custom path failed: %v", err)
	}

	if _, err := os.Stat(customPath); err != nil {
		t.Errorf("custom path file not created: %v", err)
	}
}

func TestWriteConfig_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config", "falco.yaml")

	mgr := NewManager(Config{ConfigPath: path})
	if err := mgr.WriteConfig(""); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written config: %v", err)
	}
	if len(data) == 0 {
		t.Error("written config file is empty")
	}
	if !strings.Contains(string(data), "modern_ebpf") {
		t.Error("written config should contain eBPF driver reference")
	}
}

func TestAlertsPath(t *testing.T) {
	mgr := NewManager(Config{AlertsPath: "/custom/alerts.jsonl"})
	if mgr.AlertsPath() != "/custom/alerts.jsonl" {
		t.Errorf("AlertsPath = %q, want /custom/alerts.jsonl", mgr.AlertsPath())
	}
}

func TestCheckFalcoAlertOutput_WritableDir(t *testing.T) {
	tmpDir := t.TempDir()
	alertsPath := filepath.Join(tmpDir, "alerts", "falco-alerts.jsonl")

	mgr := NewManager(Config{AlertsPath: alertsPath})
	if err := mgr.CheckFalcoAlertOutput(); err != nil {
		t.Errorf("CheckFalcoAlertOutput failed for writable dir: %v", err)
	}

	// Verify the directory was created.
	alertDir := filepath.Dir(alertsPath)
	if _, err := os.Stat(alertDir); err != nil {
		t.Errorf("alerts directory not created: %v", err)
	}
}

func TestRulesYAML_RuleSpecificContent(t *testing.T) {
	rules := RulesYAML()

	tests := []struct {
		name     string
		contains string
	}{
		{"filesystem write detection", "/workspace/"},
		{"raw socket detection", "raw"},
		{"proc environ detection", "/proc/*/environ"},
		{"ptrace detection", "ptrace"},
		{"auth file detection", "/etc/shadow"},
		{"DNS tunneling reference", "dns_tunneling"},
		{"LLM payload reference", "1048576"},
		{"sandbox ID label", "sandbox_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(rules, tt.contains) {
				t.Errorf("rules should contain %q for %s", tt.contains, tt.name)
			}
		})
	}
}

func TestRulesYAML_NoPublicDomains(t *testing.T) {
	rules := RulesYAML()

	publicDomains := []string{"google.com", "github.com", "amazonaws.com"}
	for _, domain := range publicDomains {
		if strings.Contains(rules, domain) {
			t.Errorf("rules should NOT contain public domain %q", domain)
		}
	}
}

func TestDeployConfig_StdoutDisabled(t *testing.T) {
	config := DeployConfig()

	// stdout_output should be disabled (reduces noise on dev workstations).
	if !strings.Contains(config, "stdout_output:") {
		t.Error("deploy config should have stdout_output section")
	}
}

func TestDeployConfig_WatchConfigEnabled(t *testing.T) {
	config := DeployConfig()

	if !strings.Contains(config, "watch_config_files: true") {
		t.Error("deploy config should enable watch_config_files for hot reload")
	}
}

// --- Audit event integration tests ---

func TestParseFalcoAlert_ValidJSON(t *testing.T) {
	alertJSON := `{
		"time": "2026-02-21T08:00:00.000000000Z",
		"rule": "aibox_write_outside_allowed_dirs",
		"priority": "CRITICAL",
		"output": "Container wrote outside allowed dirs",
		"output_fields": {
			"user.name": "dev",
			"proc.cmdline": "bash -c echo test > /etc/motd",
			"fd.name": "/etc/motd",
			"container.name": "aibox-sandbox-1",
			"container.image.repository": "harbor.internal/aibox/base",
			"container.label.aibox.sandbox_id": "sandbox-abc123"
		}
	}`

	alert, err := ParseFalcoAlert([]byte(alertJSON))
	if err != nil {
		t.Fatalf("ParseFalcoAlert failed: %v", err)
	}

	if alert.Rule != "aibox_write_outside_allowed_dirs" {
		t.Errorf("Rule = %q, want aibox_write_outside_allowed_dirs", alert.Rule)
	}
	if alert.Priority != "CRITICAL" {
		t.Errorf("Priority = %q, want CRITICAL", alert.Priority)
	}
}

func TestParseFalcoAlert_InvalidJSON(t *testing.T) {
	_, err := ParseFalcoAlert([]byte("not json"))
	if err == nil {
		t.Error("ParseFalcoAlert should fail for invalid JSON")
	}
}

func TestFalcoAlertToAuditEvent_CriticalAlert(t *testing.T) {
	alert := &FalcoAlert{
		Time:     "2026-02-21T08:00:00.000000000Z",
		Rule:     "aibox_ptrace_attempt",
		Priority: "CRITICAL",
		Output:   "Container attempted ptrace",
		OutputFields: map[string]string{
			"user.name":                        "dev",
			"proc.cmdline":                     "strace ls",
			"container.name":                   "aibox-sandbox-1",
			"container.label.aibox.sandbox_id": "sandbox-abc123",
			"container.image.repository":       "harbor.internal/aibox/base",
		},
	}

	event := alert.ToAuditEvent()

	if event.EventType != audit.EventFalcoAlert {
		t.Errorf("EventType = %q, want %q", event.EventType, audit.EventFalcoAlert)
	}
	if event.Source != audit.SourceFalco {
		t.Errorf("Source = %q, want %q", event.Source, audit.SourceFalco)
	}
	if event.Severity != audit.SeverityCritical {
		t.Errorf("Severity = %q, want %q", event.Severity, audit.SeverityCritical)
	}
	if event.SandboxID != "sandbox-abc123" {
		t.Errorf("SandboxID = %q, want sandbox-abc123", event.SandboxID)
	}
	if event.UserID != "dev" {
		t.Errorf("UserID = %q, want dev", event.UserID)
	}
	if event.Details["rule"] != "aibox_ptrace_attempt" {
		t.Errorf("Details[rule] = %q, want aibox_ptrace_attempt", event.Details["rule"])
	}
}

func TestFalcoAlertToAuditEvent_WarningAlert(t *testing.T) {
	alert := &FalcoAlert{
		Time:     "2026-02-21T09:00:00.000000000Z",
		Rule:     "aibox_unexpected_binary",
		Priority: "WARNING",
		Output:   "Unexpected binary executed",
		OutputFields: map[string]string{
			"user.name":      "dev",
			"container.name": "sandbox-test",
		},
	}

	event := alert.ToAuditEvent()

	if event.Severity != audit.SeverityWarning {
		t.Errorf("Severity = %q, want %q", event.Severity, audit.SeverityWarning)
	}
	// When sandbox_id label is missing, fall back to container.name.
	if event.SandboxID != "sandbox-test" {
		t.Errorf("SandboxID = %q, want sandbox-test (fallback to container.name)", event.SandboxID)
	}
}

func TestFalcoAlertToAuditEvent_ValidatesProperly(t *testing.T) {
	alert := &FalcoAlert{
		Time:     "2026-02-21T08:00:00.000000000Z",
		Rule:     "aibox_raw_socket_open",
		Priority: "CRITICAL",
		Output:   "Container opened raw socket",
		OutputFields: map[string]string{
			"user.name":                        "dev",
			"container.label.aibox.sandbox_id": "sandbox-123",
		},
	}

	event := alert.ToAuditEvent()

	if err := event.Validate(); err != nil {
		t.Errorf("AuditEvent validation failed: %v", err)
	}
}

func TestFalcoAlertToAuditEvent_Serializable(t *testing.T) {
	alert := &FalcoAlert{
		Time:     "2026-02-21T08:00:00.000000000Z",
		Rule:     "aibox_write_sensitive_auth_files",
		Priority: "CRITICAL",
		Output:   "Container wrote to /etc/shadow",
		OutputFields: map[string]string{
			"user.name":                        "dev",
			"fd.name":                          "/etc/shadow",
			"container.label.aibox.sandbox_id": "sandbox-456",
		},
	}

	event := alert.ToAuditEvent()

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal AuditEvent: %v", err)
	}

	var parsed audit.AuditEvent
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal AuditEvent: %v", err)
	}

	if parsed.EventType != audit.EventFalcoAlert {
		t.Errorf("roundtrip: EventType = %q, want %q", parsed.EventType, audit.EventFalcoAlert)
	}
}

func TestRuleSeverityToAudit(t *testing.T) {
	tests := []struct {
		input string
		want  audit.Severity
	}{
		{"critical", audit.SeverityCritical},
		{"CRITICAL", audit.SeverityCritical},
		{"warning", audit.SeverityWarning},
		{"WARNING", audit.SeverityWarning},
		{"info", audit.SeverityInfo},
		{"INFO", audit.SeverityInfo},
		{"unknown", audit.SeverityWarning},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := RuleSeverityToAudit(tt.input)
			if got != tt.want {
				t.Errorf("RuleSeverityToAudit(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFalcoPriorityMapping(t *testing.T) {
	tests := []struct {
		priority string
		want     audit.Severity
	}{
		{"EMERGENCY", audit.SeverityCritical},
		{"ALERT", audit.SeverityCritical},
		{"CRITICAL", audit.SeverityCritical},
		{"ERROR", audit.SeverityHigh},
		{"WARNING", audit.SeverityWarning},
		{"NOTICE", audit.SeverityInfo},
		{"INFORMATIONAL", audit.SeverityInfo},
		{"DEBUG", audit.SeverityInfo},
	}

	for _, tt := range tests {
		t.Run(tt.priority, func(t *testing.T) {
			alert := &FalcoAlert{
				Time:         "2026-02-21T08:00:00Z",
				Rule:         "test_rule",
				Priority:     tt.priority,
				OutputFields: map[string]string{"container.label.aibox.sandbox_id": "sb-1", "user.name": "dev"},
			}
			event := alert.ToAuditEvent()
			if event.Severity != tt.want {
				t.Errorf("priority %q mapped to %q, want %q", tt.priority, event.Severity, tt.want)
			}
		})
	}
}
