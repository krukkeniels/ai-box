package mcppacks

import (
	"testing"

	"github.com/aibox/aibox/internal/policy"
)

func TestCheckPolicy_NoNetwork(t *testing.T) {
	pack := &Manifest{
		Name:    "filesystem-mcp",
		Version: "1.0.0",
		Command: "aibox-mcp-filesystem",
		// No network requirements.
	}

	p := &policy.Policy{Version: 1}
	result := CheckPolicy(pack, p)
	if !result.Allowed {
		t.Error("pack with no network requirements should be allowed")
	}
}

func TestCheckPolicy_NetworkAllowed(t *testing.T) {
	pack := &Manifest{
		Name:            "git-mcp",
		Version:         "1.0.0",
		Command:         "aibox-mcp-git",
		NetworkRequires: []string{"git.internal"},
	}

	p := &policy.Policy{
		Version: 1,
		Network: policy.NetworkPolicy{
			Mode: "deny-by-default",
			Allow: []policy.NetworkAllowEntry{
				{ID: "git", Hosts: []string{"git.internal"}},
			},
		},
	}

	result := CheckPolicy(pack, p)
	if !result.Allowed {
		t.Error("pack should be allowed when network requirements are met")
	}
}

func TestCheckPolicy_NetworkDenied(t *testing.T) {
	pack := &Manifest{
		Name:            "jira-mcp",
		Version:         "1.0.0",
		Command:         "aibox-mcp-jira",
		NetworkRequires: []string{"jira.example.com"},
	}

	p := &policy.Policy{
		Version: 1,
		Network: policy.NetworkPolicy{
			Mode: "deny-by-default",
			Allow: []policy.NetworkAllowEntry{
				{ID: "git", Hosts: []string{"git.internal"}},
			},
		},
	}

	result := CheckPolicy(pack, p)
	if result.Allowed {
		t.Error("pack should be denied when network requirements are not met")
	}
	if len(result.DeniedEndpoints) != 1 || result.DeniedEndpoints[0] != "jira.example.com" {
		t.Errorf("denied endpoints = %v, want [jira.example.com]", result.DeniedEndpoints)
	}
}

func TestCheckPolicy_NilPolicy(t *testing.T) {
	pack := &Manifest{
		Name:            "git-mcp",
		Version:         "1.0.0",
		Command:         "aibox-mcp-git",
		NetworkRequires: []string{"git.internal"},
	}

	result := CheckPolicy(pack, nil)
	if !result.Allowed {
		t.Error("nil policy should allow all packs")
	}
}

func TestFormatDenied(t *testing.T) {
	msg := FormatDenied("jira-mcp", []string{"jira.example.com"})
	if msg == "" {
		t.Error("expected non-empty message")
	}
}
