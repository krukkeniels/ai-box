package siem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultManagerConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Error("SIEM should be disabled by default")
	}
	if cfg.ConfigPath != "/etc/aibox/siem-sink.toml" {
		t.Errorf("ConfigPath = %q, want /etc/aibox/siem-sink.toml", cfg.ConfigPath)
	}
	if len(cfg.Rules) != 10 {
		t.Errorf("default rules count = %d, want 10", len(cfg.Rules))
	}
}

func TestNewManager_DefaultsFilled(t *testing.T) {
	mgr := NewManager(Config{})

	cfg := mgr.Config()
	if cfg.ConfigPath == "" {
		t.Error("ConfigPath should have default")
	}
	if cfg.Rules == nil {
		t.Error("Rules should have defaults")
	}
	if cfg.Sink.Format == "" {
		t.Error("Sink.Format should have default")
	}
}

func TestNewManager_CustomConfig(t *testing.T) {
	mgr := NewManager(Config{
		ConfigPath: "/custom/siem.toml",
		Sink: SinkConfig{
			Type:     SinkKafka,
			Endpoint: "kafka.internal:9092",
			Format:   "json",
		},
	})

	cfg := mgr.Config()
	if cfg.ConfigPath != "/custom/siem.toml" {
		t.Errorf("ConfigPath = %q, want /custom/siem.toml", cfg.ConfigPath)
	}
	if cfg.Sink.Type != SinkKafka {
		t.Errorf("Sink.Type = %q, want kafka", cfg.Sink.Type)
	}
}

func TestEnabledRuleCount(t *testing.T) {
	mgr := NewManager(Config{})
	if mgr.EnabledRuleCount() != 10 {
		t.Errorf("EnabledRuleCount = %d, want 10", mgr.EnabledRuleCount())
	}
}

func TestEnabledRuleCount_WithDisabled(t *testing.T) {
	rules := DefaultRules()
	rules[0].Enabled = false
	rules[1].Enabled = false

	mgr := NewManager(Config{Rules: rules})
	if mgr.EnabledRuleCount() != 8 {
		t.Errorf("EnabledRuleCount = %d, want 8", mgr.EnabledRuleCount())
	}
}

func TestGenerateFullConfig_ContainsSinkAndRules(t *testing.T) {
	mgr := NewManager(Config{
		Sink: SinkConfig{
			Type:     SinkHTTP,
			Endpoint: "https://siem.internal/ingest",
			Format:   "json",
			TLS:      true,
		},
	})

	output := mgr.GenerateFullConfig()

	// Should contain sink config.
	if !strings.Contains(output, "[sinks.siem]") {
		t.Error("full config should contain sink section")
	}

	// Should contain rule reference comments.
	if !strings.Contains(output, "AI-Box SIEM Detection Rules") {
		t.Error("full config should contain detection rules reference")
	}

	// Should contain all 10 rule IDs.
	for _, id := range []string{"aibox-001", "aibox-006", "aibox-010"} {
		if !strings.Contains(output, id) {
			t.Errorf("full config should reference rule %s", id)
		}
	}
}

func TestWriteConfig_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "siem", "siem-sink.toml")

	mgr := NewManager(Config{
		ConfigPath: path,
		Sink: SinkConfig{
			Type:     SinkHTTP,
			Endpoint: "https://siem.internal/ingest",
			Format:   "json",
		},
	})

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
	if !strings.Contains(string(data), "[sinks.siem]") {
		t.Error("written config should contain sink section")
	}
}

func TestWriteConfig_CustomPath(t *testing.T) {
	tmpDir := t.TempDir()
	customPath := filepath.Join(tmpDir, "custom", "my-siem.toml")

	mgr := NewManager(Config{})
	if err := mgr.WriteConfig(customPath); err != nil {
		t.Fatalf("WriteConfig with custom path failed: %v", err)
	}

	if _, err := os.Stat(customPath); err != nil {
		t.Errorf("custom path file not created: %v", err)
	}
}

func TestRuleSummary_Format(t *testing.T) {
	mgr := NewManager(Config{})
	summary := mgr.RuleSummary()

	checks := []string{
		"SIEM Detection Rules: 10/10 enabled",
		"Critical: 2",
		"PagerDuty",
		"High:     4",
		"Slack",
		"Medium:   4",
		"Email",
		"Info:     0",
		"Dashboard",
	}

	for _, c := range checks {
		if !strings.Contains(summary, c) {
			t.Errorf("summary missing %q", c)
		}
	}
}

func TestValidateRules_DefaultsAreValid(t *testing.T) {
	errors := ValidateRules(DefaultRules())
	if len(errors) > 0 {
		t.Errorf("default rules should be valid, got errors: %v", errors)
	}
}

func TestValidateRules_EmptyID(t *testing.T) {
	rules := []DetectionRule{{Name: "test"}}
	errors := ValidateRules(rules)

	found := false
	for _, e := range errors {
		if strings.Contains(e, "empty ID") {
			found = true
		}
	}
	if !found {
		t.Error("should detect empty ID")
	}
}

func TestValidateRules_DuplicateID(t *testing.T) {
	rules := []DetectionRule{
		{ID: "dup-1", Name: "A", Description: "A", Severity: SeverityHigh, Channel: ChannelSlack, Sources: []string{"test"}, Condition: "x > 1"},
		{ID: "dup-1", Name: "B", Description: "B", Severity: SeverityHigh, Channel: ChannelSlack, Sources: []string{"test"}, Condition: "y > 1"},
	}
	errors := ValidateRules(rules)

	found := false
	for _, e := range errors {
		if strings.Contains(e, "duplicate rule ID") {
			found = true
		}
	}
	if !found {
		t.Error("should detect duplicate ID")
	}
}

func TestValidateRules_InvalidSeverity(t *testing.T) {
	rules := []DetectionRule{
		{ID: "test-1", Name: "test", Description: "test", Severity: "invalid", Channel: ChannelSlack, Sources: []string{"test"}, Condition: "x > 1"},
	}
	errors := ValidateRules(rules)

	found := false
	for _, e := range errors {
		if strings.Contains(e, "invalid severity") {
			found = true
		}
	}
	if !found {
		t.Error("should detect invalid severity")
	}
}

func TestValidateRules_MissingFields(t *testing.T) {
	rules := []DetectionRule{
		{ID: "test-1"}, // all fields empty except ID
	}
	errors := ValidateRules(rules)

	expectedErrors := []string{"empty Name", "empty Description", "invalid severity", "empty Channel", "no sources", "empty Condition"}
	for _, expected := range expectedErrors {
		found := false
		for _, e := range errors {
			if strings.Contains(e, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("should detect %q", expected)
		}
	}
}
