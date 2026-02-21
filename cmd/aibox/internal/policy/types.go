package policy

import "time"

// Risk class constants for tool permission classification.
const (
	RiskSafe             = "safe"
	RiskReviewRequired   = "review-required"
	RiskBlockedByDefault = "blocked-by-default"
)

// riskLevel maps risk class strings to comparable severity levels.
// Higher values are more restrictive.
var riskLevel = map[string]int{
	RiskSafe:             0,
	RiskReviewRequired:   1,
	RiskBlockedByDefault: 2,
}

// Policy is the top-level policy document loaded from YAML.
type Policy struct {
	Version     int              `yaml:"version"`
	Network     NetworkPolicy    `yaml:"network"`
	Filesystem  FilesystemPolicy `yaml:"filesystem"`
	Tools       ToolsPolicy      `yaml:"tools"`
	Resources   ResourcePolicy   `yaml:"resources"`
	Runtime     RuntimePolicy    `yaml:"runtime"`
	Credentials CredentialPolicy `yaml:"credentials"`
}

// NetworkPolicy controls outbound network access.
type NetworkPolicy struct {
	Mode  string              `yaml:"mode"` // "deny-by-default"
	Allow []NetworkAllowEntry `yaml:"allow"`
}

// NetworkAllowEntry is a single entry in the network allowlist.
type NetworkAllowEntry struct {
	ID        string     `yaml:"id"`
	Hosts     []string   `yaml:"hosts"`
	Ports     []int      `yaml:"ports"`
	RateLimit *RateLimit `yaml:"rate_limit,omitempty"`
}

// RateLimit configures rate limiting for a network allow entry.
type RateLimit struct {
	RequestsPerMin int `yaml:"requests_per_min"`
	TokensPerMin   int `yaml:"tokens_per_min"`
}

// FilesystemPolicy controls filesystem access.
type FilesystemPolicy struct {
	WorkspaceRoot string   `yaml:"workspace_root"`
	Deny          []string `yaml:"deny"`
}

// ToolRule defines a tool permission rule with risk classification.
type ToolRule struct {
	Match []string `yaml:"match"` // e.g. ["git", "push"] or ["curl", "*"]
	Allow bool     `yaml:"allow"`
	Risk  string   `yaml:"risk"` // "safe", "review-required", "blocked-by-default"
}

// ToolsPolicy holds the set of tool permission rules.
type ToolsPolicy struct {
	Rules []ToolRule `yaml:"rules"`
}

// ResourcePolicy defines resource limits for the sandbox.
type ResourcePolicy struct {
	CPU    string `yaml:"cpu"`
	Memory string `yaml:"memory"`
	Disk   string `yaml:"disk"`
}

// RuntimePolicy controls the sandbox runtime engine.
type RuntimePolicy struct {
	Engine   string `yaml:"engine"`   // "gvisor" or "kata"
	Rootless bool   `yaml:"rootless"`
}

// CredentialPolicy controls credential lifecycle management.
type CredentialPolicy struct {
	GitTokenTTL    string `yaml:"git_token_ttl"`
	LLMKeyTTL      string `yaml:"llm_api_key_ttl"`
	MirrorTokenTTL string `yaml:"mirror_token_ttl"`
	RevokeOnStop   bool   `yaml:"revoke_on_stop"`
	NoPersist      bool   `yaml:"no_persist_to_workspace"`
}

// PolicyInput is the evaluation input passed to the OPA engine.
type PolicyInput struct {
	Action    string         `json:"action"`    // "command", "network", "filesystem"
	Command   []string       `json:"command"`   // e.g. ["git", "push"]
	Target    string         `json:"target"`    // hostname, filepath
	User      string         `json:"user"`
	Workspace string         `json:"workspace"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// DecisionResult is the output of a policy evaluation.
type DecisionResult struct {
	Allowed   bool          `json:"allowed"`
	RiskClass string        `json:"risk_class"` // "safe", "review-required", "blocked-by-default"
	Rule      string        `json:"rule"`
	Reason    string        `json:"reason"`
	PolicyVer string        `json:"policy_version"`
	InputHash string        `json:"input_hash"`
	Timestamp time.Time     `json:"timestamp"`
	Duration  time.Duration `json:"duration_ms"`
}
