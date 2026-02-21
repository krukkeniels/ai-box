package policy

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ApprovalMode determines how review-required commands are handled.
type ApprovalMode int

const (
	ApprovalAsync ApprovalMode = iota // Log and proceed (default)
	ApprovalSync                      // Block until approved (future)
)

// BlockedError is returned when a command is denied.
type BlockedError struct {
	Command   []string
	Rule      ToolRule
	RiskClass string
	Reason    string
}

func (e *BlockedError) Error() string {
	return fmt.Sprintf(
		"Command denied: %s\n  Rule: match: %s\n  Risk class: %s\n  Reason: %s\n\n  To request an exception, submit a policy amendment request.",
		strings.Join(e.Command, " "),
		formatMatch(e.Rule.Match),
		e.RiskClass,
		e.Reason,
	)
}

// ReviewRequiredError is returned when a command requires approval in sync mode.
type ReviewRequiredError struct {
	Command   []string
	Rule      ToolRule
	RiskClass string
}

func (e *ReviewRequiredError) Error() string {
	return fmt.Sprintf(
		"Command requires approval: %s\n  Rule: match: %s\n  Risk class: %s",
		strings.Join(e.Command, " "),
		formatMatch(e.Rule.Match),
		e.RiskClass,
	)
}

// formatMatch formats a match pattern for display.
func formatMatch(match []string) string {
	quoted := make([]string, len(match))
	for i, m := range match {
		quoted[i] = fmt.Sprintf("%q", m)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// ToolGate evaluates commands against the effective policy and enforces risk classes.
type ToolGate struct {
	policy    *Policy
	logger    *DecisionLogger
	user      string
	workspace string
	sandboxID string
	mu        sync.RWMutex
}

// NewToolGate creates a tool gate with the effective policy and decision logger.
func NewToolGate(policy *Policy, logger *DecisionLogger, user, workspace, sandboxID string) *ToolGate {
	return &ToolGate{
		policy:    policy,
		logger:    logger,
		user:      user,
		workspace: workspace,
		sandboxID: sandboxID,
	}
}

// Evaluate checks a command against the policy and returns the decision.
func (g *ToolGate) Evaluate(ctx context.Context, command []string) (*DecisionResult, error) {
	start := time.Now()

	g.mu.RLock()
	p := g.policy
	g.mu.RUnlock()

	result := &DecisionResult{
		PolicyVer: hashPolicy(p),
		InputHash: hashInput(PolicyInput{
			Action:    "command",
			Command:   command,
			User:      g.user,
			Workspace: g.workspace,
			Timestamp: start,
		}),
		Timestamp: start,
	}

	if p == nil || len(p.Tools.Rules) == 0 {
		// No rules configured -- default to safe.
		result.Allowed = true
		result.RiskClass = RiskSafe
		result.Rule = "default-safe"
		result.Reason = "no tool rules configured; default safe"
		result.Duration = time.Since(start)
		return result, nil
	}

	rule := FindMatchingRule(command, p.Tools.Rules)
	if rule == nil {
		// No matching rule -- default to safe.
		result.Allowed = true
		result.RiskClass = RiskSafe
		result.Rule = "default-safe"
		result.Reason = "no matching tool rule; default safe"
		result.Duration = time.Since(start)
		return result, nil
	}

	result.Allowed = rule.Allow
	result.RiskClass = rule.Risk
	result.Rule = matchKey(rule.Match)
	if rule.Allow {
		result.Reason = fmt.Sprintf("allowed by rule %q", result.Rule)
	} else {
		result.Reason = fmt.Sprintf("denied by rule %q", result.Rule)
	}

	result.Duration = time.Since(start)

	// Log the decision.
	if g.logger != nil {
		entry := EntryFromResult(PolicyInput{
			Action:    "command",
			Command:   command,
			User:      g.user,
			Workspace: g.workspace,
			Timestamp: start,
		}, *result, g.sandboxID)
		_ = g.logger.Log(entry)
	}

	return result, nil
}

// EvaluateAndEnforce evaluates and enforces the decision:
// - safe: returns nil (allow)
// - review-required: logs audit entry, returns nil (async) or ReviewRequiredError (sync)
// - blocked-by-default: returns BlockedError
func (g *ToolGate) EvaluateAndEnforce(ctx context.Context, command []string, mode ApprovalMode) error {
	result, err := g.Evaluate(ctx, command)
	if err != nil {
		return err
	}

	switch result.RiskClass {
	case RiskSafe:
		return nil

	case RiskReviewRequired:
		if mode == ApprovalSync {
			g.mu.RLock()
			p := g.policy
			g.mu.RUnlock()

			rule := FindMatchingRule(command, p.Tools.Rules)
			if rule == nil {
				return nil
			}
			return &ReviewRequiredError{
				Command:   command,
				Rule:      *rule,
				RiskClass: result.RiskClass,
			}
		}
		// Async mode: already logged in Evaluate, allow through.
		return nil

	case RiskBlockedByDefault:
		g.mu.RLock()
		p := g.policy
		g.mu.RUnlock()

		rule := FindMatchingRule(command, p.Tools.Rules)
		blockedRule := ToolRule{}
		if rule != nil {
			blockedRule = *rule
		}
		return &BlockedError{
			Command:   command,
			Rule:      blockedRule,
			RiskClass: result.RiskClass,
			Reason:    result.Reason,
		}

	default:
		// Unknown risk class -- treat as safe.
		return nil
	}
}

// UpdatePolicy hot-reloads the effective policy.
func (g *ToolGate) UpdatePolicy(p *Policy) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.policy = p
}
