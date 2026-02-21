package policy

import (
	"testing"
)

func validPolicy() *Policy {
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
			},
		},
		Filesystem: FilesystemPolicy{
			Deny: []string{"/etc/shadow"},
		},
		Tools: ToolsPolicy{
			Rules: []ToolRule{
				{Match: []string{"git", "push"}, Allow: true, Risk: RiskReviewRequired},
			},
		},
		Resources: ResourcePolicy{
			CPU:    "4",
			Memory: "8g",
		},
		Runtime: RuntimePolicy{
			Engine:   "gvisor",
			Rootless: true,
		},
		Credentials: CredentialPolicy{
			GitTokenTTL:  "4h",
			RevokeOnStop: true,
			NoPersist:    true,
		},
	}
}

func TestValidatePolicy_Valid(t *testing.T) {
	p := validPolicy()
	errs := ValidatePolicy(p)
	if len(errs) > 0 {
		t.Errorf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestValidatePolicy_VersionZero(t *testing.T) {
	p := validPolicy()
	p.Version = 0
	errs := ValidatePolicy(p)
	if !hasErrorField(errs, "version") {
		t.Error("expected version error")
	}
}

func TestValidatePolicy_InvalidNetworkMode(t *testing.T) {
	p := validPolicy()
	p.Network.Mode = "allow-all"
	errs := ValidatePolicy(p)
	if !hasErrorField(errs, "network.mode") {
		t.Error("expected network.mode error")
	}
}

func TestValidatePolicy_WildcardHost(t *testing.T) {
	p := validPolicy()
	p.Network.Allow = []NetworkAllowEntry{
		{ID: "wild", Hosts: []string{"*"}, Ports: []int{443}},
	}
	errs := ValidatePolicy(p)
	found := false
	for _, e := range errs {
		if e.Field == "network.allow[0].hosts" {
			found = true
		}
	}
	if !found {
		t.Error("expected wildcard host error")
	}
}

func TestValidatePolicy_MissingAllowID(t *testing.T) {
	p := validPolicy()
	p.Network.Allow = []NetworkAllowEntry{
		{Hosts: []string{"foo.com"}, Ports: []int{443}},
	}
	errs := ValidatePolicy(p)
	if !hasErrorField(errs, "network.allow[0].id") {
		t.Error("expected missing id error")
	}
}

func TestValidatePolicy_EmptyHosts(t *testing.T) {
	p := validPolicy()
	p.Network.Allow = []NetworkAllowEntry{
		{ID: "empty", Hosts: []string{}, Ports: []int{443}},
	}
	errs := ValidatePolicy(p)
	if !hasErrorField(errs, "network.allow[0].hosts") {
		t.Error("expected empty hosts error")
	}
}

func TestValidatePolicy_EmptyPorts(t *testing.T) {
	p := validPolicy()
	p.Network.Allow = []NetworkAllowEntry{
		{ID: "nop", Hosts: []string{"foo.com"}, Ports: []int{}},
	}
	errs := ValidatePolicy(p)
	if !hasErrorField(errs, "network.allow[0].ports") {
		t.Error("expected empty ports error")
	}
}

func TestValidatePolicy_InvalidPort(t *testing.T) {
	p := validPolicy()
	p.Network.Allow = []NetworkAllowEntry{
		{ID: "bad", Hosts: []string{"foo.com"}, Ports: []int{0}},
	}
	errs := ValidatePolicy(p)
	if !hasErrorField(errs, "network.allow[0].ports") {
		t.Error("expected invalid port error")
	}
}

func TestValidatePolicy_RelativeDenyPath(t *testing.T) {
	p := validPolicy()
	p.Filesystem.Deny = []string{"relative/path"}
	errs := ValidatePolicy(p)
	if !hasErrorField(errs, "filesystem.deny[0]") {
		t.Error("expected relative path error")
	}
}

func TestValidatePolicy_EmptyToolMatch(t *testing.T) {
	p := validPolicy()
	p.Tools.Rules = []ToolRule{
		{Match: []string{}, Allow: true, Risk: RiskSafe},
	}
	errs := ValidatePolicy(p)
	if !hasErrorField(errs, "tools.rules[0].match") {
		t.Error("expected empty match error")
	}
}

func TestValidatePolicy_InvalidRiskClass(t *testing.T) {
	p := validPolicy()
	p.Tools.Rules = []ToolRule{
		{Match: []string{"test"}, Allow: true, Risk: "unknown"},
	}
	errs := ValidatePolicy(p)
	if !hasErrorField(errs, "tools.rules[0].risk") {
		t.Error("expected invalid risk class error")
	}
}

func TestValidatePolicy_InvalidEngine(t *testing.T) {
	p := validPolicy()
	p.Runtime.Engine = "runc"
	errs := ValidatePolicy(p)
	if !hasErrorField(errs, "runtime.engine") {
		t.Error("expected invalid engine error")
	}
}

func TestValidatePolicy_InvalidTTL(t *testing.T) {
	p := validPolicy()
	p.Credentials.GitTokenTTL = "not-a-duration"
	errs := ValidatePolicy(p)
	found := false
	for _, e := range errs {
		if e.Field == "credentials.git_token_ttl" {
			found = true
		}
	}
	if !found {
		t.Error("expected invalid TTL error")
	}
}

func TestValidatePolicy_InvalidResource(t *testing.T) {
	p := validPolicy()
	p.Resources.CPU = "abc"
	errs := ValidatePolicy(p)
	if !hasErrorField(errs, "resources.cpu") {
		t.Error("expected invalid resource error")
	}
}

func TestValidateMerge_Valid(t *testing.T) {
	org := validPolicy()
	team := &Policy{
		Version: 1,
		Resources: ResourcePolicy{
			CPU: "2",
		},
		Runtime: RuntimePolicy{Rootless: true},
		Credentials: CredentialPolicy{
			GitTokenTTL:  "2h",
			RevokeOnStop: true,
			NoPersist:    true,
		},
	}

	errs := ValidateMerge(org, team, nil)
	if len(errs) > 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestValidateMerge_SchemaErrors(t *testing.T) {
	org := validPolicy()
	team := &Policy{
		Version: 0, // Invalid
	}

	errs := ValidateMerge(org, team, nil)
	found := false
	for _, e := range errs {
		if e.Field == "team.version" {
			found = true
		}
	}
	if !found {
		t.Error("expected team version validation error")
	}
}

func TestValidateMerge_MergeViolation(t *testing.T) {
	org := validPolicy()
	team := &Policy{
		Version: 1,
		Runtime: RuntimePolicy{
			Engine: "runc", // Violation
		},
	}

	errs := ValidateMerge(org, team, nil)
	found := false
	for _, e := range errs {
		if e.Field == "merge" {
			found = true
		}
	}
	if !found {
		t.Error("expected merge violation error")
	}
}

func TestValidationErrorString(t *testing.T) {
	e := ValidationError{Field: "test.field", Message: "is required"}
	s := e.Error()
	if s != "test.field: is required" {
		t.Errorf("got %q", s)
	}
}

func hasErrorField(errs []ValidationError, field string) bool {
	for _, e := range errs {
		if e.Field == field {
			return true
		}
	}
	return false
}
