package policy

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/open-policy-agent/opa/v1/rego"
)

// Engine evaluates policy decisions using embedded OPA and the effective merged policy.
type Engine struct {
	query     rego.PreparedEvalQuery
	policy    *Policy
	policyVer string
	mu        sync.RWMutex
}

// NewEngine creates a policy engine by loading Rego files from policyDir
// and merging the YAML policy hierarchy.
func NewEngine(policyDir string) (*Engine, error) {
	e := &Engine{}

	if err := e.loadFromDir(policyDir); err != nil {
		return nil, fmt.Errorf("initializing policy engine: %w", err)
	}

	slog.Info("policy engine initialized", "policy_dir", policyDir, "version", e.policyVer)
	return e, nil
}

// NewEngineFromPolicy creates an engine from an already-merged effective policy
// and Rego source files in policyDir.
func NewEngineFromPolicy(effectivePolicy *Policy, policyDir string) (*Engine, error) {
	e := &Engine{
		policy: effectivePolicy,
	}

	regoFiles, err := findRegoFiles(policyDir)
	if err != nil {
		return nil, fmt.Errorf("finding rego files: %w", err)
	}

	if err := e.prepareQuery(regoFiles); err != nil {
		return nil, fmt.Errorf("preparing OPA query: %w", err)
	}

	e.policyVer = hashPolicy(effectivePolicy)

	slog.Info("policy engine initialized from effective policy", "version", e.policyVer)
	return e, nil
}

// Evaluate runs a policy decision for the given input.
func (e *Engine) Evaluate(ctx context.Context, input PolicyInput) (*DecisionResult, error) {
	start := time.Now()

	e.mu.RLock()
	defer e.mu.RUnlock()

	result := &DecisionResult{
		PolicyVer: e.policyVer,
		InputHash: hashInput(input),
		Timestamp: start,
	}

	// For tool/command actions, use the effective policy's tool rules directly.
	if input.Action == "command" && len(input.Command) > 0 {
		e.evaluateToolRules(input, result)
		result.Duration = time.Since(start)
		return result, nil
	}

	// For network and filesystem actions, use the effective policy directly.
	switch input.Action {
	case "network":
		e.evaluateNetworkRules(input, result)
	case "filesystem":
		e.evaluateFilesystemRules(input, result)
	default:
		// Fall through to OPA for unknown action types.
		if err := e.evaluateOPA(ctx, input, result); err != nil {
			return nil, err
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

// Reload replaces the engine's policy and Rego rules from the given directory.
func (e *Engine) Reload(policyDir string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.loadFromDir(policyDir); err != nil {
		return fmt.Errorf("reloading policy engine: %w", err)
	}

	slog.Info("policy engine reloaded", "policy_dir", policyDir, "version", e.policyVer)
	return nil
}

// EffectivePolicy returns the current effective merged policy.
func (e *Engine) EffectivePolicy() *Policy {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.policy
}

// loadFromDir loads Rego files and policy YAML from a directory.
func (e *Engine) loadFromDir(dir string) error {
	regoFiles, err := findRegoFiles(dir)
	if err != nil {
		return fmt.Errorf("finding rego files in %s: %w", dir, err)
	}

	if err := e.prepareQuery(regoFiles); err != nil {
		return fmt.Errorf("preparing OPA query: %w", err)
	}

	// Try to load policy.yaml from the directory if it exists.
	policyPath := filepath.Join(dir, "policy.yaml")
	if _, statErr := os.Stat(policyPath); statErr == nil {
		p, loadErr := LoadPolicy(policyPath)
		if loadErr != nil {
			return fmt.Errorf("loading policy from %s: %w", policyPath, loadErr)
		}
		e.policy = p
	} else if e.policy == nil {
		// Initialize with an empty default policy if none loaded.
		e.policy = &Policy{Version: 1}
	}

	e.policyVer = hashPolicy(e.policy)
	return nil
}

// prepareQuery compiles Rego source files into a prepared evaluation query.
func (e *Engine) prepareQuery(regoFiles map[string]string) error {
	if len(regoFiles) == 0 {
		// No Rego files -- prepare a minimal default query.
		r := rego.New(
			rego.Query("data.aibox"),
			rego.Module("default.rego", "package aibox\n\ndefault allow = false\n"),
		)
		pq, err := r.PrepareForEval(context.Background())
		if err != nil {
			return fmt.Errorf("preparing default OPA query: %w", err)
		}
		e.query = pq
		return nil
	}

	opts := []func(*rego.Rego){
		rego.Query("data.aibox"),
	}

	for name, src := range regoFiles {
		opts = append(opts, rego.Module(name, src))
	}

	r := rego.New(opts...)
	pq, err := r.PrepareForEval(context.Background())
	if err != nil {
		return fmt.Errorf("preparing OPA query: %w", err)
	}

	e.query = pq
	return nil
}

// evaluateToolRules matches command inputs against the effective policy's tool rules.
func (e *Engine) evaluateToolRules(input PolicyInput, result *DecisionResult) {
	if e.policy == nil || len(e.policy.Tools.Rules) == 0 {
		result.Allowed = false
		result.RiskClass = RiskBlockedByDefault
		result.Rule = "no-rules"
		result.Reason = "no tool rules configured; default deny"
		return
	}

	for _, rule := range e.policy.Tools.Rules {
		if matchCommand(rule.Match, input.Command) {
			result.Allowed = rule.Allow
			result.RiskClass = rule.Risk
			result.Rule = matchKey(rule.Match)
			if rule.Allow {
				result.Reason = fmt.Sprintf("allowed by rule %q", result.Rule)
			} else {
				result.Reason = fmt.Sprintf("denied by rule %q", result.Rule)
			}
			return
		}
	}

	// No matching rule -- default deny.
	result.Allowed = false
	result.RiskClass = RiskBlockedByDefault
	result.Rule = "default-deny"
	result.Reason = "no matching tool rule; default deny"
}

// matchCommand checks if a command matches a rule's match pattern.
// Supports wildcards: ["git", "*"] matches any git subcommand.
func matchCommand(pattern, command []string) bool {
	if len(pattern) == 0 {
		return false
	}
	if len(command) < len(pattern) {
		return false
	}

	for i, p := range pattern {
		if p == "*" {
			continue
		}
		if i >= len(command) || command[i] != p {
			return false
		}
	}
	return true
}

// evaluateNetworkRules checks network access against the effective policy.
func (e *Engine) evaluateNetworkRules(input PolicyInput, result *DecisionResult) {
	if e.policy == nil {
		result.Allowed = false
		result.RiskClass = RiskBlockedByDefault
		result.Rule = "no-policy"
		result.Reason = "no policy loaded; default deny"
		return
	}

	target := input.Target
	for _, entry := range e.policy.Network.Allow {
		for _, host := range entry.Hosts {
			if host == target {
				result.Allowed = true
				result.RiskClass = RiskSafe
				result.Rule = entry.ID
				result.Reason = fmt.Sprintf("host %q allowed by network rule %q", target, entry.ID)
				return
			}
		}
	}

	result.Allowed = false
	result.RiskClass = RiskBlockedByDefault
	result.Rule = "network-deny-default"
	result.Reason = fmt.Sprintf("host %q not in network allowlist", target)
}

// evaluateFilesystemRules checks filesystem access against the effective policy.
func (e *Engine) evaluateFilesystemRules(input PolicyInput, result *DecisionResult) {
	if e.policy == nil {
		result.Allowed = false
		result.RiskClass = RiskBlockedByDefault
		result.Rule = "no-policy"
		result.Reason = "no policy loaded; default deny"
		return
	}

	target := input.Target
	for _, denied := range e.policy.Filesystem.Deny {
		if strings.HasPrefix(target, denied) {
			result.Allowed = false
			result.RiskClass = RiskBlockedByDefault
			result.Rule = "filesystem-deny"
			result.Reason = fmt.Sprintf("path %q is denied by filesystem policy", target)
			return
		}
	}

	result.Allowed = true
	result.RiskClass = RiskSafe
	result.Rule = "filesystem-allow"
	result.Reason = fmt.Sprintf("path %q is not in deny list", target)
}

// evaluateOPA runs the prepared OPA query against the given input.
func (e *Engine) evaluateOPA(ctx context.Context, input PolicyInput, result *DecisionResult) error {
	inputMap, err := structToMap(input)
	if err != nil {
		return fmt.Errorf("converting input to map: %w", err)
	}

	rs, err := e.query.Eval(ctx, rego.EvalInput(inputMap))
	if err != nil {
		return fmt.Errorf("evaluating OPA query: %w", err)
	}

	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		result.Allowed = false
		result.RiskClass = RiskBlockedByDefault
		result.Rule = "opa-no-result"
		result.Reason = "OPA returned no results; default deny"
		return nil
	}

	// Extract decision from OPA result.
	resultMap, ok := rs[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		result.Allowed = false
		result.RiskClass = RiskBlockedByDefault
		result.Rule = "opa-parse-error"
		result.Reason = "could not parse OPA result"
		return nil
	}

	if allow, ok := resultMap["allow"].(bool); ok {
		result.Allowed = allow
	}

	if deny, ok := resultMap["deny"]; ok {
		// deny is typically a set of strings. If non-empty, deny.
		switch d := deny.(type) {
		case []interface{}:
			if len(d) > 0 {
				result.Allowed = false
				reasons := make([]string, 0, len(d))
				for _, r := range d {
					reasons = append(reasons, fmt.Sprint(r))
				}
				result.Reason = strings.Join(reasons, "; ")
			}
		case map[string]interface{}:
			if len(d) > 0 {
				result.Allowed = false
			}
		}
	}

	if result.RiskClass == "" {
		if result.Allowed {
			result.RiskClass = RiskSafe
		} else {
			result.RiskClass = RiskBlockedByDefault
		}
	}

	if result.Rule == "" {
		result.Rule = "opa-eval"
	}

	return nil
}

// findRegoFiles discovers all .rego files under the given directory.
func findRegoFiles(dir string) (map[string]string, error) {
	files := make(map[string]string)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".rego") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("reading %s: %w", path, readErr)
		}
		relPath, _ := filepath.Rel(dir, path)
		files[relPath] = string(data)
		return nil
	})

	if err != nil {
		return nil, err
	}
	return files, nil
}

// structToMap converts a struct to a map[string]interface{} via JSON round-trip.
func structToMap(v interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// hashInput produces a SHA-256 hex digest of the input for audit logging.
func hashInput(input PolicyInput) string {
	data, _ := json.Marshal(input)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:8])
}

// hashPolicy produces a SHA-256 hex digest of the effective policy for versioning.
func hashPolicy(p *Policy) string {
	data, _ := json.Marshal(p)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:8])
}
