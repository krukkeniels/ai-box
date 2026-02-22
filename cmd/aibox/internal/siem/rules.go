package siem

import "time"

// Severity levels for detection rule alerts.
type Severity string

const (
	SeverityCritical Severity = "critical" // PagerDuty, 15-min response SLA
	SeverityHigh     Severity = "high"     // Slack channel, 4-hour response SLA
	SeverityMedium   Severity = "medium"   // Email digest, next business day
	SeverityInfo     Severity = "info"     // Dashboard only, weekly review
)

// AlertChannel is the routing destination for an alert.
type AlertChannel string

const (
	ChannelPagerDuty AlertChannel = "pagerduty"
	ChannelSlack     AlertChannel = "slack"
	ChannelEmail     AlertChannel = "email"
	ChannelDashboard AlertChannel = "dashboard"
)

// DetectionRule defines a single SIEM detection/correlation rule.
type DetectionRule struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Severity    Severity     `json:"severity"`
	Channel     AlertChannel `json:"channel"`
	Sources     []string     `json:"sources"`    // log sources to correlate
	Condition   string       `json:"condition"`  // human-readable trigger condition
	Threshold   float64      `json:"threshold"`  // threshold value (unit depends on rule)
	WindowMins  int          `json:"window_mins"` // time window in minutes
	ResponseSLA string       `json:"response_sla"`
	Enabled     bool         `json:"enabled"`
	MITRERef    string       `json:"mitre_ref,omitempty"` // MITRE ATT&CK reference
}

// AlertRouting maps severity levels to alert channels and response SLAs.
var AlertRouting = map[Severity]struct {
	Channel     AlertChannel
	ResponseSLA time.Duration
}{
	SeverityCritical: {ChannelPagerDuty, 15 * time.Minute},
	SeverityHigh:     {ChannelSlack, 4 * time.Hour},
	SeverityMedium:   {ChannelEmail, 24 * time.Hour},
	SeverityInfo:     {ChannelDashboard, 7 * 24 * time.Hour},
}

// DefaultRules returns the 10 AI-Box-specific detection rules from the Phase 5 plan.
func DefaultRules() []DetectionRule {
	return []DetectionRule{
		{
			ID:          "aibox-001",
			Name:        "Anomalous Outbound Data Volume",
			Description: "Squid logs show excessive data transfer from a single sandbox, indicating potential exfiltration.",
			Severity:    SeverityHigh,
			Channel:     ChannelSlack,
			Sources:     []string{"squid"},
			Condition:   "bytes_transferred > threshold within window from single sandbox_id",
			Threshold:   100 * 1024 * 1024, // 100 MB
			WindowMins:  30,
			ResponseSLA: "4 hours",
			Enabled:     true,
			MITRERef:    "T1041",
		},
		{
			ID:          "aibox-002",
			Name:        "DNS Query Spike",
			Description: "CoreDNS logs show excessive DNS queries from a sandbox, indicating potential DNS tunneling.",
			Severity:    SeverityMedium,
			Channel:     ChannelEmail,
			Sources:     []string{"coredns"},
			Condition:   "dns_queries > threshold per minute from single sandbox_id",
			Threshold:   100,
			WindowMins:  5,
			ResponseSLA: "next business day",
			Enabled:     true,
			MITRERef:    "T1071.004",
		},
		{
			ID:          "aibox-003",
			Name:        "Off-Hours Credential Access",
			Description: "Vault audit shows credential issuance outside configured business hours.",
			Severity:    SeverityMedium,
			Channel:     ChannelEmail,
			Sources:     []string{"vault"},
			Condition:   "credential.issue event outside business hours (configurable)",
			Threshold:   0,
			WindowMins:  0,
			ResponseSLA: "next business day",
			Enabled:     true,
		},
		{
			ID:          "aibox-004",
			Name:        "Repeated Blocked Network Attempts",
			Description: "Multiple denied network requests from the same sandbox, indicating proxy bypass attempts.",
			Severity:    SeverityHigh,
			Channel:     ChannelSlack,
			Sources:     []string{"squid", "nftables"},
			Condition:   "network.deny events > threshold within window from single sandbox_id",
			Threshold:   20,
			WindowMins:  10,
			ResponseSLA: "4 hours",
			Enabled:     true,
			MITRERef:    "T1090",
		},
		{
			ID:          "aibox-005",
			Name:        "LLM Payload Size Anomaly",
			Description: "LLM proxy detects request payload significantly larger than normal, indicating potential bulk exfiltration via LLM channel.",
			Severity:    SeverityMedium,
			Channel:     ChannelEmail,
			Sources:     []string{"aibox-llm-proxy"},
			Condition:   "llm.request payload_size > 95th percentile * 3",
			Threshold:   1024 * 1024, // 1 MB
			WindowMins:  60,
			ResponseSLA: "next business day",
			Enabled:     true,
			MITRERef:    "T1567",
		},
		{
			ID:          "aibox-006",
			Name:        "Container Escape Indicator",
			Description: "Falco fires a critical-severity rule indicating a container escape attempt.",
			Severity:    SeverityCritical,
			Channel:     ChannelPagerDuty,
			Sources:     []string{"falco"},
			Condition:   "falco.alert with severity=critical",
			Threshold:   1,
			WindowMins:  0,
			ResponseSLA: "15 minutes",
			Enabled:     true,
			MITRERef:    "T1611",
		},
		{
			ID:          "aibox-007",
			Name:        "Policy Violation Burst",
			Description: "OPA decision log shows excessive denials from the same sandbox, indicating automated evasion attempts.",
			Severity:    SeverityHigh,
			Channel:     ChannelSlack,
			Sources:     []string{"opa"},
			Condition:   "policy.deny events > threshold within window from single sandbox_id",
			Threshold:   15,
			WindowMins:  5,
			ResponseSLA: "4 hours",
			Enabled:     true,
		},
		{
			ID:          "aibox-008",
			Name:        "Credential Access After Sandbox Stop",
			Description: "Vault audit shows credential use after the sandbox lifecycle shows a stop event, indicating stolen credentials.",
			Severity:    SeverityCritical,
			Channel:     ChannelPagerDuty,
			Sources:     []string{"vault", "aibox-cli"},
			Condition:   "credential.use event after sandbox.stop for same sandbox_id",
			Threshold:   1,
			WindowMins:  60,
			ResponseSLA: "15 minutes",
			Enabled:     true,
			MITRERef:    "T1528",
		},
		{
			ID:          "aibox-009",
			Name:        "Base64 Payload Anomaly",
			Description: "LLM proxy detects base64-encoded blocks in unusual request fields, indicating encoded data exfiltration.",
			Severity:    SeverityMedium,
			Channel:     ChannelEmail,
			Sources:     []string{"aibox-llm-proxy"},
			Condition:   "llm.request contains base64 blocks > threshold bytes in non-content fields",
			Threshold:   10240, // 10 KB base64 blocks
			WindowMins:  60,
			ResponseSLA: "next business day",
			Enabled:     true,
			MITRERef:    "T1132.001",
		},
		{
			ID:          "aibox-010",
			Name:        "Git Push to Unexpected Remote",
			Description: "Git operation logs show push to a remote not in the allowlist, indicating code exfiltration.",
			Severity:    SeverityHigh,
			Channel:     ChannelSlack,
			Sources:     []string{"aibox-agent"},
			Condition:   "tool.invoke with command=git push to remote not in allowlist",
			Threshold:   1,
			WindowMins:  0,
			ResponseSLA: "4 hours",
			Enabled:     true,
			MITRERef:    "T1567.001",
		},
	}
}

// RuleByID returns the rule with the given ID, or nil if not found.
func RuleByID(rules []DetectionRule, id string) *DetectionRule {
	for i := range rules {
		if rules[i].ID == id {
			return &rules[i]
		}
	}
	return nil
}

// EnabledRules returns only the enabled rules from the given set.
func EnabledRules(rules []DetectionRule) []DetectionRule {
	var enabled []DetectionRule
	for _, r := range rules {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	return enabled
}

// RulesBySeverity returns rules filtered by severity level.
func RulesBySeverity(rules []DetectionRule, severity Severity) []DetectionRule {
	var filtered []DetectionRule
	for _, r := range rules {
		if r.Severity == severity {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
