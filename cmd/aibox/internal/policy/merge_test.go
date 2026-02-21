package policy

import (
	"errors"
	"testing"
)

func baseOrgPolicy() *Policy {
	return &Policy{
		Version: 1,
		Network: NetworkPolicy{
			Mode: "deny-by-default",
			Allow: []NetworkAllowEntry{
				{
					ID:    "registry",
					Hosts: []string{"harbor.internal"},
					Ports: []int{443},
				},
				{
					ID:    "git",
					Hosts: []string{"git.internal"},
					Ports: []int{443, 22},
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
				{Match: []string{"file_read"}, Allow: true, Risk: RiskSafe},
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
}

func TestMergePolicies_OrgOnly(t *testing.T) {
	org := baseOrgPolicy()
	result, err := MergePolicies(org, nil, nil)
	if err != nil {
		t.Fatalf("merge error: %v", err)
	}
	if result.Runtime.Engine != "gvisor" {
		t.Errorf("engine: got %q", result.Runtime.Engine)
	}
	if len(result.Network.Allow) != 2 {
		t.Errorf("allow entries: got %d, want 2", len(result.Network.Allow))
	}
}

func TestMergePolicies_NilOrg(t *testing.T) {
	_, err := MergePolicies(nil, nil, nil)
	if err == nil {
		t.Fatal("expected error with nil org")
	}
}

// --- Network merge tests ---

func TestMergeNetwork_SubsetAllowed(t *testing.T) {
	org := baseOrgPolicy()
	team := &Policy{
		Version:     1,
		Network: NetworkPolicy{
			Allow: []NetworkAllowEntry{
				{ID: "registry", Hosts: []string{"harbor.internal"}, Ports: []int{443}},
			},
		},
		Runtime:     RuntimePolicy{Rootless: true},
		Credentials: CredentialPolicy{RevokeOnStop: true, NoPersist: true},
	}

	result, err := MergePolicies(org, team, nil)
	if err != nil {
		t.Fatalf("merge error: %v", err)
	}
	if len(result.Network.Allow) != 1 {
		t.Errorf("allow entries: got %d, want 1", len(result.Network.Allow))
	}
}

func TestMergeNetwork_NewHostViolation(t *testing.T) {
	org := baseOrgPolicy()
	team := &Policy{
		Version: 1,
		Network: NetworkPolicy{
			Allow: []NetworkAllowEntry{
				{ID: "evil", Hosts: []string{"evil.com"}, Ports: []int{443}},
			},
		},
	}

	_, err := MergePolicies(org, team, nil)
	if err == nil {
		t.Fatal("expected merge violation for new host")
	}
	var mergeErr *MergeError
	if !errors.As(err, &mergeErr) {
		t.Fatalf("expected MergeError, got %T", err)
	}
	if len(mergeErr.Violations) == 0 {
		t.Fatal("expected violations")
	}
}

// --- Filesystem merge tests ---

func TestMergeFilesystem_UnionDeny(t *testing.T) {
	org := baseOrgPolicy()
	team := &Policy{
		Version: 1,
		Filesystem: FilesystemPolicy{
			Deny: []string{"/sys/firmware", "/etc/shadow"}, // /etc/shadow already exists
		},
		Runtime:     RuntimePolicy{Rootless: true},
		Credentials: CredentialPolicy{RevokeOnStop: true, NoPersist: true},
	}

	result, err := MergePolicies(org, team, nil)
	if err != nil {
		t.Fatalf("merge error: %v", err)
	}

	deny := result.Filesystem.Deny
	denySet := make(map[string]bool)
	for _, d := range deny {
		denySet[d] = true
	}

	expected := []string{"/etc/shadow", "/proc/kcore", "/sys/firmware"}
	for _, e := range expected {
		if !denySet[e] {
			t.Errorf("missing deny path: %s", e)
		}
	}
}

// --- Tool rules merge tests ---

func TestMergeTools_TightenAllowed(t *testing.T) {
	org := baseOrgPolicy()
	team := &Policy{
		Version: 1,
		Tools: ToolsPolicy{
			Rules: []ToolRule{
				// Tighten git push from review-required to blocked
				{Match: []string{"git", "push"}, Allow: false, Risk: RiskBlockedByDefault},
			},
		},
		Runtime:     RuntimePolicy{Rootless: true},
		Credentials: CredentialPolicy{RevokeOnStop: true, NoPersist: true},
	}

	result, err := MergePolicies(org, team, nil)
	if err != nil {
		t.Fatalf("merge error: %v", err)
	}

	for _, rule := range result.Tools.Rules {
		if matchKey(rule.Match) == "git push" {
			if rule.Risk != RiskBlockedByDefault {
				t.Errorf("git push risk: got %q, want %q", rule.Risk, RiskBlockedByDefault)
			}
			if rule.Allow {
				t.Error("git push should be disallowed")
			}
			return
		}
	}
	t.Error("git push rule not found in merged result")
}

func TestMergeTools_LoosenViolation(t *testing.T) {
	org := baseOrgPolicy()
	team := &Policy{
		Version: 1,
		Tools: ToolsPolicy{
			Rules: []ToolRule{
				// Attempt to loosen git push from review-required to safe
				{Match: []string{"git", "push"}, Allow: true, Risk: RiskSafe},
			},
		},
	}

	_, err := MergePolicies(org, team, nil)
	if err == nil {
		t.Fatal("expected violation for loosening tool risk")
	}
}

func TestMergeTools_NewRuleAllowed(t *testing.T) {
	org := baseOrgPolicy()
	team := &Policy{
		Version: 1,
		Tools: ToolsPolicy{
			Rules: []ToolRule{
				{Match: []string{"docker", "build"}, Allow: true, Risk: RiskReviewRequired},
			},
		},
		Runtime:     RuntimePolicy{Rootless: true},
		Credentials: CredentialPolicy{RevokeOnStop: true, NoPersist: true},
	}

	result, err := MergePolicies(org, team, nil)
	if err != nil {
		t.Fatalf("merge error: %v", err)
	}

	found := false
	for _, rule := range result.Tools.Rules {
		if matchKey(rule.Match) == "docker build" {
			found = true
			break
		}
	}
	if !found {
		t.Error("new docker build rule not found in merged result")
	}
}

// --- Resource merge tests ---

func TestMergeResources_ReduceAllowed(t *testing.T) {
	org := baseOrgPolicy()
	team := &Policy{
		Version: 1,
		Resources: ResourcePolicy{
			CPU:    "2",
			Memory: "4g",
		},
		Runtime:     RuntimePolicy{Rootless: true},
		Credentials: CredentialPolicy{RevokeOnStop: true, NoPersist: true},
	}

	result, err := MergePolicies(org, team, nil)
	if err != nil {
		t.Fatalf("merge error: %v", err)
	}

	if result.Resources.CPU != "2" {
		t.Errorf("cpu: got %q, want %q", result.Resources.CPU, "2")
	}
	if result.Resources.Memory != "4g" {
		t.Errorf("memory: got %q, want %q", result.Resources.Memory, "4g")
	}
}

func TestMergeResources_IncreaseViolation(t *testing.T) {
	org := baseOrgPolicy()
	team := &Policy{
		Version: 1,
		Resources: ResourcePolicy{
			CPU: "8", // More than org's 4
		},
	}

	_, err := MergePolicies(org, team, nil)
	if err == nil {
		t.Fatal("expected violation for increasing CPU")
	}
}

// --- Runtime merge tests ---

func TestMergeRuntime_GvisorCannotChange(t *testing.T) {
	org := baseOrgPolicy()
	team := &Policy{
		Version: 1,
		Runtime: RuntimePolicy{
			Engine: "runc",
		},
	}

	_, err := MergePolicies(org, team, nil)
	if err == nil {
		t.Fatal("expected violation for changing engine from gvisor")
	}
}

func TestMergeRuntime_RootlessCannotDisable(t *testing.T) {
	org := baseOrgPolicy()
	team := &Policy{
		Version: 1,
		Runtime: RuntimePolicy{
			Rootless: false,
		},
	}

	_, err := MergePolicies(org, team, nil)
	if err == nil {
		t.Fatal("expected violation for disabling rootless")
	}
}

// --- Credential merge tests ---

func TestMergeCredentials_ShorterTTLAllowed(t *testing.T) {
	org := baseOrgPolicy()
	team := &Policy{
		Version: 1,
		Runtime: RuntimePolicy{Rootless: true},
		Credentials: CredentialPolicy{
			GitTokenTTL:  "2h",
			RevokeOnStop: true,
			NoPersist:    true,
		},
	}

	result, err := MergePolicies(org, team, nil)
	if err != nil {
		t.Fatalf("merge error: %v", err)
	}

	if result.Credentials.GitTokenTTL != "2h" {
		t.Errorf("git_token_ttl: got %q, want %q", result.Credentials.GitTokenTTL, "2h")
	}
}

func TestMergeCredentials_LongerTTLViolation(t *testing.T) {
	org := baseOrgPolicy()
	team := &Policy{
		Version: 1,
		Credentials: CredentialPolicy{
			GitTokenTTL:  "12h", // Longer than org's 4h
			RevokeOnStop: true,
			NoPersist:    true,
		},
	}

	_, err := MergePolicies(org, team, nil)
	if err == nil {
		t.Fatal("expected violation for longer TTL")
	}
}

func TestMergeCredentials_DisableRevokeViolation(t *testing.T) {
	org := baseOrgPolicy()
	team := &Policy{
		Version: 1,
		Credentials: CredentialPolicy{
			RevokeOnStop: false,
		},
	}

	_, err := MergePolicies(org, team, nil)
	if err == nil {
		t.Fatal("expected violation for disabling revoke_on_stop")
	}
}

func TestMergeCredentials_DisableNoPersistViolation(t *testing.T) {
	org := baseOrgPolicy()
	team := &Policy{
		Version: 1,
		Credentials: CredentialPolicy{
			NoPersist: false,
		},
	}

	_, err := MergePolicies(org, team, nil)
	if err == nil {
		t.Fatal("expected violation for disabling no_persist_to_workspace")
	}
}

// --- Three-level merge tests ---

func TestMergeThreeLevels(t *testing.T) {
	org := baseOrgPolicy()
	team := &Policy{
		Version: 1,
		Resources: ResourcePolicy{
			CPU:    "2",
			Memory: "4g",
		},
		Runtime: RuntimePolicy{Rootless: true},
		Credentials: CredentialPolicy{
			GitTokenTTL:  "2h",
			RevokeOnStop: true,
			NoPersist:    true,
		},
	}
	project := &Policy{
		Version: 1,
		Resources: ResourcePolicy{
			CPU: "1",
		},
		Runtime: RuntimePolicy{Rootless: true},
		Credentials: CredentialPolicy{
			GitTokenTTL:  "1h",
			RevokeOnStop: true,
			NoPersist:    true,
		},
	}

	result, err := MergePolicies(org, team, project)
	if err != nil {
		t.Fatalf("merge error: %v", err)
	}

	if result.Resources.CPU != "1" {
		t.Errorf("cpu: got %q, want %q", result.Resources.CPU, "1")
	}
	if result.Resources.Memory != "4g" {
		t.Errorf("memory: got %q, want %q", result.Resources.Memory, "4g")
	}
	if result.Credentials.GitTokenTTL != "1h" {
		t.Errorf("git_token_ttl: got %q, want %q", result.Credentials.GitTokenTTL, "1h")
	}
}

// --- Deep copy test ---

func TestDeepCopyPolicy(t *testing.T) {
	orig := baseOrgPolicy()
	cp := deepCopyPolicy(orig)

	// Modify the copy and ensure original is untouched.
	cp.Network.Allow[0].Hosts[0] = "modified.com"
	if orig.Network.Allow[0].Hosts[0] == "modified.com" {
		t.Error("deep copy shares hosts slice with original")
	}

	cp.Filesystem.Deny = append(cp.Filesystem.Deny, "/new/path")
	if len(orig.Filesystem.Deny) == len(cp.Filesystem.Deny) {
		t.Error("deep copy shares deny slice with original")
	}
}

// --- Helper tests ---

func TestParseResourceBytes(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"4", 4},
		{"512m", 512 * 1024 * 1024},
		{"8g", 8 * 1024 * 1024 * 1024},
		{"1t", 1024 * 1024 * 1024 * 1024},
		{"2k", 2 * 1024},
		{"", 0},
		{"invalid", 0},
	}

	for _, tc := range tests {
		got := parseResourceBytes(tc.input)
		if got != tc.want {
			t.Errorf("parseResourceBytes(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestMatchCommand(t *testing.T) {
	tests := []struct {
		pattern []string
		command []string
		want    bool
	}{
		{[]string{"git", "push"}, []string{"git", "push"}, true},
		{[]string{"git", "push"}, []string{"git", "pull"}, false},
		{[]string{"git", "*"}, []string{"git", "push"}, true},
		{[]string{"git", "*"}, []string{"git", "pull"}, true},
		{[]string{"git"}, []string{"git", "push"}, true},
		{[]string{"git", "push"}, []string{"git"}, false},
		{[]string{}, []string{"git"}, false},
	}

	for _, tc := range tests {
		got := matchCommand(tc.pattern, tc.command)
		if got != tc.want {
			t.Errorf("matchCommand(%v, %v) = %v, want %v", tc.pattern, tc.command, got, tc.want)
		}
	}
}

func TestMergeErrorMessage(t *testing.T) {
	err := &MergeError{
		Violations: []string{"violation 1", "violation 2"},
	}
	msg := err.Error()
	if msg == "" {
		t.Fatal("error message should not be empty")
	}
}
