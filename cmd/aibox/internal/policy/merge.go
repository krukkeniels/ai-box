package policy

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// MergeError reports one or more tighten-only violations during policy merge.
type MergeError struct {
	Violations []string
}

func (e *MergeError) Error() string {
	return fmt.Sprintf("policy merge violations:\n  - %s", strings.Join(e.Violations, "\n  - "))
}

// MergePolicies combines org, team, and project policies using tighten-only semantics.
// Team and project may be nil. The result is the effective merged policy.
// Returns a MergeError if any child policy attempts to loosen a parent constraint.
func MergePolicies(org, team, project *Policy) (*Policy, error) {
	if org == nil {
		return nil, fmt.Errorf("org policy is required")
	}

	result := deepCopyPolicy(org)
	var violations []string

	if team != nil {
		v := mergeInto(result, team)
		violations = append(violations, v...)
	}

	if project != nil {
		v := mergeInto(result, project)
		violations = append(violations, v...)
	}

	if len(violations) > 0 {
		return nil, &MergeError{Violations: violations}
	}

	slog.Debug("policy merge completed", "version", result.Version)
	return result, nil
}

// mergeInto merges a child policy into the current effective policy (parent).
// Returns a list of tighten-only violations.
func mergeInto(parent, child *Policy) []string {
	var violations []string

	violations = append(violations, mergeNetwork(&parent.Network, &child.Network)...)
	violations = append(violations, mergeFilesystem(&parent.Filesystem, &child.Filesystem)...)
	violations = append(violations, mergeTools(&parent.Tools, &child.Tools)...)
	violations = append(violations, mergeResources(&parent.Resources, &child.Resources)...)
	violations = append(violations, mergeRuntime(&parent.Runtime, &child.Runtime)...)
	violations = append(violations, mergeCredentials(&parent.Credentials, &child.Credentials)...)

	return violations
}

// mergeNetwork enforces intersection semantics on network allowlists.
// Child hosts must be a subset of parent hosts.
func mergeNetwork(parent, child *NetworkPolicy) []string {
	var violations []string

	if child.Mode != "" {
		parent.Mode = child.Mode
	}

	if len(child.Allow) == 0 {
		return nil
	}

	// Build a set of all parent-allowed hosts.
	parentHosts := make(map[string]bool)
	for _, entry := range parent.Allow {
		for _, h := range entry.Hosts {
			parentHosts[h] = true
		}
	}

	// Check that every child host exists in the parent set.
	for _, entry := range child.Allow {
		for _, h := range entry.Hosts {
			if !parentHosts[h] {
				violations = append(violations, fmt.Sprintf(
					"network: child adds host %q not in parent allowlist", h))
			}
		}
	}

	if len(violations) > 0 {
		return violations
	}

	// Intersect: replace parent allowlist with child (which is a subset).
	parent.Allow = child.Allow
	return nil
}

// mergeFilesystem enforces union semantics on deny lists.
// Child can add paths, never remove.
func mergeFilesystem(parent, child *FilesystemPolicy) []string {
	if child.WorkspaceRoot != "" {
		parent.WorkspaceRoot = child.WorkspaceRoot
	}

	if len(child.Deny) == 0 {
		return nil
	}

	// Build set of parent deny paths.
	existing := make(map[string]bool)
	for _, p := range parent.Deny {
		existing[p] = true
	}

	// Add child deny paths (union).
	for _, p := range child.Deny {
		if !existing[p] {
			parent.Deny = append(parent.Deny, p)
			existing[p] = true
		}
	}

	return nil
}

// mergeTools enforces most-restrictive-wins for tool rules.
// safe -> review-required -> blocked-by-default is allowed; reverse is a violation.
func mergeTools(parent, child *ToolsPolicy) []string {
	var violations []string

	if len(child.Rules) == 0 {
		return nil
	}

	// Index parent rules by match key for lookup.
	parentRuleIdx := make(map[string]int)
	for i, r := range parent.Rules {
		key := matchKey(r.Match)
		parentRuleIdx[key] = i
	}

	for _, childRule := range child.Rules {
		key := matchKey(childRule.Match)
		idx, exists := parentRuleIdx[key]

		if !exists {
			// New rule from child - allowed.
			parent.Rules = append(parent.Rules, childRule)
			continue
		}

		parentRule := parent.Rules[idx]
		parentLevel := riskLevel[parentRule.Risk]
		childLevel := riskLevel[childRule.Risk]

		if childLevel < parentLevel {
			violations = append(violations, fmt.Sprintf(
				"tools: rule %q attempts to loosen risk from %q to %q",
				key, parentRule.Risk, childRule.Risk))
			continue
		}

		// Child is at least as restrictive - apply it.
		parent.Rules[idx] = childRule
	}

	return violations
}

// matchKey produces a comparable string key from a match slice.
func matchKey(match []string) string {
	return strings.Join(match, " ")
}

// mergeResources enforces min(parent, child) for resource limits.
func mergeResources(parent, child *ResourcePolicy) []string {
	var violations []string

	if child.CPU != "" {
		v := mergeResourceValue("cpu", parent.CPU, child.CPU)
		if v != "" {
			violations = append(violations, v)
		} else {
			parent.CPU = minResourceStr(parent.CPU, child.CPU)
		}
	}

	if child.Memory != "" {
		v := mergeResourceValue("memory", parent.Memory, child.Memory)
		if v != "" {
			violations = append(violations, v)
		} else {
			parent.Memory = minResourceStr(parent.Memory, child.Memory)
		}
	}

	if child.Disk != "" {
		v := mergeResourceValue("disk", parent.Disk, child.Disk)
		if v != "" {
			violations = append(violations, v)
		} else {
			parent.Disk = minResourceStr(parent.Disk, child.Disk)
		}
	}

	return violations
}

// mergeResourceValue returns a violation string if child exceeds parent, otherwise "".
func mergeResourceValue(name, parentVal, childVal string) string {
	if parentVal == "" {
		return ""
	}
	pBytes := parseResourceBytes(parentVal)
	cBytes := parseResourceBytes(childVal)
	if cBytes > pBytes {
		return fmt.Sprintf("resources: child %s (%s) exceeds parent (%s)", name, childVal, parentVal)
	}
	return ""
}

// minResourceStr returns the smaller of two resource strings by parsed value.
func minResourceStr(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	aBytes := parseResourceBytes(a)
	bBytes := parseResourceBytes(b)
	if bBytes < aBytes {
		return b
	}
	return a
}

// parseResourceBytes parses a resource string like "8g", "512m", "4" into bytes.
// Supports suffixes: k/K (KiB), m/M (MiB), g/G (GiB), t/T (TiB).
// Plain numbers are returned as-is (interpreted as the base unit).
func parseResourceBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	multiplier := int64(1)
	numStr := s

	if len(s) > 0 {
		suffix := s[len(s)-1]
		switch suffix {
		case 'k', 'K':
			multiplier = 1024
			numStr = s[:len(s)-1]
		case 'm', 'M':
			multiplier = 1024 * 1024
			numStr = s[:len(s)-1]
		case 'g', 'G':
			multiplier = 1024 * 1024 * 1024
			numStr = s[:len(s)-1]
		case 't', 'T':
			multiplier = 1024 * 1024 * 1024 * 1024
			numStr = s[:len(s)-1]
		}
	}

	n, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0
	}
	return n * multiplier
}

// mergeRuntime enforces that gvisor cannot change to runc and rootless cannot be disabled.
func mergeRuntime(parent, child *RuntimePolicy) []string {
	var violations []string

	if child.Engine != "" && child.Engine != parent.Engine {
		if parent.Engine == "gvisor" && child.Engine != "gvisor" {
			violations = append(violations, fmt.Sprintf(
				"runtime: cannot change engine from %q to %q", parent.Engine, child.Engine))
		} else {
			parent.Engine = child.Engine
		}
	}

	if parent.Rootless && !child.Rootless {
		violations = append(violations, "runtime: cannot disable rootless mode")
	}

	return violations
}

// mergeCredentials enforces that TTLs can only be shortened and
// revoke_on_stop/no_persist cannot be disabled.
func mergeCredentials(parent, child *CredentialPolicy) []string {
	var violations []string

	if child.GitTokenTTL != "" {
		v := mergeTTL("credentials.git_token_ttl", parent.GitTokenTTL, child.GitTokenTTL)
		if v != "" {
			violations = append(violations, v)
		} else {
			parent.GitTokenTTL = minTTLStr(parent.GitTokenTTL, child.GitTokenTTL)
		}
	}

	if child.LLMKeyTTL != "" {
		v := mergeTTL("credentials.llm_api_key_ttl", parent.LLMKeyTTL, child.LLMKeyTTL)
		if v != "" {
			violations = append(violations, v)
		} else {
			parent.LLMKeyTTL = minTTLStr(parent.LLMKeyTTL, child.LLMKeyTTL)
		}
	}

	if child.MirrorTokenTTL != "" {
		v := mergeTTL("credentials.mirror_token_ttl", parent.MirrorTokenTTL, child.MirrorTokenTTL)
		if v != "" {
			violations = append(violations, v)
		} else {
			parent.MirrorTokenTTL = minTTLStr(parent.MirrorTokenTTL, child.MirrorTokenTTL)
		}
	}

	if parent.RevokeOnStop && !child.RevokeOnStop {
		violations = append(violations, "credentials: cannot disable revoke_on_stop")
	}

	if parent.NoPersist && !child.NoPersist {
		violations = append(violations, "credentials: cannot disable no_persist_to_workspace")
	}

	return violations
}

// mergeTTL returns a violation if child TTL exceeds parent TTL.
func mergeTTL(name, parentTTL, childTTL string) string {
	if parentTTL == "" {
		return ""
	}
	pDur, err := time.ParseDuration(parentTTL)
	if err != nil {
		return ""
	}
	cDur, err := time.ParseDuration(childTTL)
	if err != nil {
		return fmt.Sprintf("%s: invalid TTL %q", name, childTTL)
	}
	if cDur > pDur {
		return fmt.Sprintf("%s: child TTL (%s) exceeds parent (%s)", name, childTTL, parentTTL)
	}
	return ""
}

// minTTLStr returns the shorter of two TTL strings.
func minTTLStr(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	aDur, _ := time.ParseDuration(a)
	bDur, _ := time.ParseDuration(b)
	if bDur < aDur {
		return b
	}
	return a
}

// deepCopyPolicy returns a deep copy of the given policy.
func deepCopyPolicy(p *Policy) *Policy {
	cp := *p

	// Deep copy slices.
	if len(p.Network.Allow) > 0 {
		cp.Network.Allow = make([]NetworkAllowEntry, len(p.Network.Allow))
		for i, entry := range p.Network.Allow {
			cp.Network.Allow[i] = entry
			if len(entry.Hosts) > 0 {
				cp.Network.Allow[i].Hosts = make([]string, len(entry.Hosts))
				copy(cp.Network.Allow[i].Hosts, entry.Hosts)
			}
			if len(entry.Ports) > 0 {
				cp.Network.Allow[i].Ports = make([]int, len(entry.Ports))
				copy(cp.Network.Allow[i].Ports, entry.Ports)
			}
			if entry.RateLimit != nil {
				rl := *entry.RateLimit
				cp.Network.Allow[i].RateLimit = &rl
			}
		}
	}

	if len(p.Filesystem.Deny) > 0 {
		cp.Filesystem.Deny = make([]string, len(p.Filesystem.Deny))
		copy(cp.Filesystem.Deny, p.Filesystem.Deny)
	}

	if len(p.Tools.Rules) > 0 {
		cp.Tools.Rules = make([]ToolRule, len(p.Tools.Rules))
		for i, r := range p.Tools.Rules {
			cp.Tools.Rules[i] = r
			if len(r.Match) > 0 {
				cp.Tools.Rules[i].Match = make([]string, len(r.Match))
				copy(cp.Tools.Rules[i].Match, r.Match)
			}
		}
	}

	return &cp
}
