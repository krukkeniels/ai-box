package siem

import (
	"testing"
)

func TestDefaultRulesCount(t *testing.T) {
	rules := DefaultRules()
	if len(rules) != 10 {
		t.Errorf("got %d rules, want 10", len(rules))
	}
}

func TestDefaultRulesUniqueIDs(t *testing.T) {
	rules := DefaultRules()
	seen := make(map[string]bool)
	for _, r := range rules {
		if seen[r.ID] {
			t.Errorf("duplicate rule ID: %q", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestDefaultRulesUniqueNames(t *testing.T) {
	rules := DefaultRules()
	seen := make(map[string]bool)
	for _, r := range rules {
		if seen[r.Name] {
			t.Errorf("duplicate rule name: %q", r.Name)
		}
		seen[r.Name] = true
	}
}

func TestDefaultRulesAllEnabled(t *testing.T) {
	rules := DefaultRules()
	for _, r := range rules {
		if !r.Enabled {
			t.Errorf("rule %q (%s) should be enabled by default", r.Name, r.ID)
		}
	}
}

func TestDefaultRulesHaveSources(t *testing.T) {
	rules := DefaultRules()
	for _, r := range rules {
		if len(r.Sources) == 0 {
			t.Errorf("rule %q (%s) has no sources", r.Name, r.ID)
		}
	}
}

func TestDefaultRulesHaveRequiredFields(t *testing.T) {
	rules := DefaultRules()
	for _, r := range rules {
		if r.ID == "" {
			t.Error("rule has empty ID")
		}
		if r.Name == "" {
			t.Errorf("rule %s has empty Name", r.ID)
		}
		if r.Description == "" {
			t.Errorf("rule %s has empty Description", r.ID)
		}
		if r.Severity == "" {
			t.Errorf("rule %s has empty Severity", r.ID)
		}
		if r.Channel == "" {
			t.Errorf("rule %s has empty Channel", r.ID)
		}
		if r.Condition == "" {
			t.Errorf("rule %s has empty Condition", r.ID)
		}
		if r.ResponseSLA == "" {
			t.Errorf("rule %s has empty ResponseSLA", r.ID)
		}
	}
}

func TestDefaultRulesSeverityDistribution(t *testing.T) {
	rules := DefaultRules()

	counts := make(map[Severity]int)
	for _, r := range rules {
		counts[r.Severity]++
	}

	// Spec defines: 2 critical, 4 high, 4 medium.
	tests := []struct {
		severity Severity
		want     int
	}{
		{SeverityCritical, 2},
		{SeverityHigh, 4},
		{SeverityMedium, 4},
	}

	for _, tt := range tests {
		if counts[tt.severity] != tt.want {
			t.Errorf("%s rules: got %d, want %d", tt.severity, counts[tt.severity], tt.want)
		}
	}
}

func TestDefaultRulesChannelRouting(t *testing.T) {
	rules := DefaultRules()

	for _, r := range rules {
		expected := AlertRouting[r.Severity]
		if r.Channel != expected.Channel {
			t.Errorf("rule %q (%s): channel = %q, want %q for %s severity",
				r.Name, r.ID, r.Channel, expected.Channel, r.Severity)
		}
	}
}

func TestRuleByID_Found(t *testing.T) {
	rules := DefaultRules()

	tests := []struct {
		id   string
		name string
	}{
		{"aibox-001", "Anomalous Outbound Data Volume"},
		{"aibox-006", "Container Escape Indicator"},
		{"aibox-010", "Git Push to Unexpected Remote"},
	}

	for _, tt := range tests {
		r := RuleByID(rules, tt.id)
		if r == nil {
			t.Errorf("RuleByID(%q) returned nil", tt.id)
			continue
		}
		if r.Name != tt.name {
			t.Errorf("RuleByID(%q).Name = %q, want %q", tt.id, r.Name, tt.name)
		}
	}
}

func TestRuleByID_NotFound(t *testing.T) {
	rules := DefaultRules()
	r := RuleByID(rules, "nonexistent")
	if r != nil {
		t.Errorf("RuleByID(nonexistent) should return nil, got %+v", r)
	}
}

func TestEnabledRules(t *testing.T) {
	rules := DefaultRules()
	// Disable one rule.
	rules[0].Enabled = false

	enabled := EnabledRules(rules)
	if len(enabled) != 9 {
		t.Errorf("got %d enabled rules, want 9", len(enabled))
	}
}

func TestEnabledRules_AllDisabled(t *testing.T) {
	rules := DefaultRules()
	for i := range rules {
		rules[i].Enabled = false
	}

	enabled := EnabledRules(rules)
	if len(enabled) != 0 {
		t.Errorf("got %d enabled rules, want 0", len(enabled))
	}
}

func TestRulesBySeverity(t *testing.T) {
	rules := DefaultRules()

	tests := []struct {
		severity Severity
		want     int
	}{
		{SeverityCritical, 2},
		{SeverityHigh, 4},
		{SeverityMedium, 4},
		{SeverityInfo, 0},
	}

	for _, tt := range tests {
		got := RulesBySeverity(rules, tt.severity)
		if len(got) != tt.want {
			t.Errorf("RulesBySeverity(%s): got %d, want %d", tt.severity, len(got), tt.want)
		}
	}
}

func TestAlertRoutingComplete(t *testing.T) {
	severities := []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityInfo}

	for _, s := range severities {
		routing, ok := AlertRouting[s]
		if !ok {
			t.Errorf("missing AlertRouting for %s", s)
			continue
		}
		if routing.Channel == "" {
			t.Errorf("AlertRouting[%s] has empty channel", s)
		}
		if routing.ResponseSLA <= 0 {
			t.Errorf("AlertRouting[%s] has zero/negative SLA", s)
		}
	}
}

func TestCriticalRules_ContainerEscapeAndCredentialMisuse(t *testing.T) {
	rules := DefaultRules()
	critical := RulesBySeverity(rules, SeverityCritical)

	ruleNames := make(map[string]bool)
	for _, r := range critical {
		ruleNames[r.Name] = true
	}

	if !ruleNames["Container Escape Indicator"] {
		t.Error("critical rules should include Container Escape Indicator")
	}
	if !ruleNames["Credential Access After Sandbox Stop"] {
		t.Error("critical rules should include Credential Access After Sandbox Stop")
	}
}

func TestRulesSpecIDs(t *testing.T) {
	rules := DefaultRules()

	expectedIDs := []string{
		"aibox-001", "aibox-002", "aibox-003", "aibox-004", "aibox-005",
		"aibox-006", "aibox-007", "aibox-008", "aibox-009", "aibox-010",
	}

	for _, id := range expectedIDs {
		r := RuleByID(rules, id)
		if r == nil {
			t.Errorf("missing rule with ID %q", id)
		}
	}
}
