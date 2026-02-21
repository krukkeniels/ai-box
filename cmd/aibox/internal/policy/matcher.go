package policy

// MatchCommand checks if a command matches a tool rule pattern.
// Pattern format: ["git", "push"] matches "git push origin main"
// Wildcards: ["curl", "*"] matches any curl invocation
// Matching is prefix-based on tokenized command.
func MatchCommand(command []string, pattern []string) bool {
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
		if command[i] != p {
			return false
		}
	}
	return true
}

// FindMatchingRule finds the first tool rule that matches the given command.
// Returns nil if no rule matches (default to safe).
func FindMatchingRule(command []string, rules []ToolRule) *ToolRule {
	if len(command) == 0 {
		return nil
	}
	for i := range rules {
		if MatchCommand(command, rules[i].Match) {
			return &rules[i]
		}
	}
	return nil
}

// RiskClassPriority returns a numeric priority for risk ordering.
// Higher number = more restrictive.
// safe=0, review-required=1, blocked-by-default=2
func RiskClassPriority(riskClass string) int {
	if p, ok := riskLevel[riskClass]; ok {
		return p
	}
	return -1
}
