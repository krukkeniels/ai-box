package policy

import (
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func TestPolicyYAMLRoundTrip(t *testing.T) {
	original := &Policy{
		Version: 1,
		Network: NetworkPolicy{
			Mode: "deny-by-default",
			Allow: []NetworkAllowEntry{
				{
					ID:    "registry",
					Hosts: []string{"harbor.internal"},
					Ports: []int{443},
					RateLimit: &RateLimit{
						RequestsPerMin: 60,
						TokensPerMin:   100000,
					},
				},
			},
		},
		Filesystem: FilesystemPolicy{
			WorkspaceRoot: "/workspace",
			Deny:          []string{"/etc/shadow", "/proc/kcore"},
		},
		Tools: ToolsPolicy{
			Rules: []ToolRule{
				{Match: []string{"git", "push"}, Allow: true, Risk: RiskReviewRequired},
				{Match: []string{"curl", "*"}, Allow: false, Risk: RiskBlockedByDefault},
			},
		},
		Resources: ResourcePolicy{
			CPU:    "4",
			Memory: "8g",
			Disk:   "50g",
		},
		Runtime: RuntimePolicy{
			Engine:   "gvisor",
			Rootless: true,
		},
		Credentials: CredentialPolicy{
			GitTokenTTL:    "4h",
			LLMKeyTTL:      "8h",
			MirrorTokenTTL: "8h",
			RevokeOnStop:   true,
			NoPersist:      true,
		},
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Policy
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify key fields survived the round trip.
	if decoded.Version != original.Version {
		t.Errorf("version: got %d, want %d", decoded.Version, original.Version)
	}
	if decoded.Network.Mode != original.Network.Mode {
		t.Errorf("network.mode: got %q, want %q", decoded.Network.Mode, original.Network.Mode)
	}
	if len(decoded.Network.Allow) != 1 {
		t.Fatalf("network.allow: got %d entries, want 1", len(decoded.Network.Allow))
	}
	if decoded.Network.Allow[0].RateLimit == nil {
		t.Fatal("network.allow[0].rate_limit: is nil")
	}
	if decoded.Network.Allow[0].RateLimit.RequestsPerMin != 60 {
		t.Errorf("rate_limit.requests_per_min: got %d, want 60", decoded.Network.Allow[0].RateLimit.RequestsPerMin)
	}
	if len(decoded.Filesystem.Deny) != 2 {
		t.Errorf("filesystem.deny: got %d, want 2", len(decoded.Filesystem.Deny))
	}
	if len(decoded.Tools.Rules) != 2 {
		t.Errorf("tools.rules: got %d, want 2", len(decoded.Tools.Rules))
	}
	if decoded.Runtime.Engine != "gvisor" {
		t.Errorf("runtime.engine: got %q, want %q", decoded.Runtime.Engine, "gvisor")
	}
	if !decoded.Runtime.Rootless {
		t.Error("runtime.rootless: got false, want true")
	}
	if !decoded.Credentials.RevokeOnStop {
		t.Error("credentials.revoke_on_stop: got false, want true")
	}
}

func TestPolicyInputSerialization(t *testing.T) {
	input := PolicyInput{
		Action:    "command",
		Command:   []string{"git", "push"},
		Target:    "git.internal",
		User:      "dev",
		Workspace: "/workspace",
		Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Metadata:  map[string]any{"branch": "main"},
	}

	if input.Action != "command" {
		t.Errorf("action: got %q, want %q", input.Action, "command")
	}
	if len(input.Command) != 2 || input.Command[0] != "git" {
		t.Errorf("command: got %v, want [git push]", input.Command)
	}
}

func TestDecisionResultFields(t *testing.T) {
	result := DecisionResult{
		Allowed:   true,
		RiskClass: RiskSafe,
		Rule:      "git push",
		Reason:    "allowed by rule",
		PolicyVer: "abc123",
		InputHash: "def456",
		Timestamp: time.Now(),
		Duration:  500 * time.Microsecond,
	}

	if !result.Allowed {
		t.Error("allowed: got false, want true")
	}
	if result.RiskClass != RiskSafe {
		t.Errorf("risk_class: got %q, want %q", result.RiskClass, RiskSafe)
	}
}

func TestRiskConstants(t *testing.T) {
	if RiskSafe != "safe" {
		t.Errorf("RiskSafe: got %q", RiskSafe)
	}
	if RiskReviewRequired != "review-required" {
		t.Errorf("RiskReviewRequired: got %q", RiskReviewRequired)
	}
	if RiskBlockedByDefault != "blocked-by-default" {
		t.Errorf("RiskBlockedByDefault: got %q", RiskBlockedByDefault)
	}
}

func TestRiskLevelOrdering(t *testing.T) {
	if riskLevel[RiskSafe] >= riskLevel[RiskReviewRequired] {
		t.Error("safe should be less restrictive than review-required")
	}
	if riskLevel[RiskReviewRequired] >= riskLevel[RiskBlockedByDefault] {
		t.Error("review-required should be less restrictive than blocked-by-default")
	}
}
