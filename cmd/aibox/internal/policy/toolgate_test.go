package policy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testPolicy() *Policy {
	return &Policy{
		Version: 1,
		Tools: ToolsPolicy{
			Rules: []ToolRule{
				{Match: []string{"git", "pull"}, Allow: true, Risk: RiskSafe},
				{Match: []string{"git", "push"}, Allow: true, Risk: RiskReviewRequired},
				{Match: []string{"curl", "*"}, Allow: false, Risk: RiskBlockedByDefault},
				{Match: []string{"npm", "publish"}, Allow: false, Risk: RiskBlockedByDefault},
			},
		},
	}
}

func testDecisionLogger(t *testing.T) *DecisionLogger {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "decisions.jsonl")
	cfg := DecisionLogConfig{
		Path:       logPath,
		MaxSizeMB:  1,
		SampleSafe: 0, // log all
	}
	logger, err := NewDecisionLogger(cfg)
	if err != nil {
		t.Fatalf("NewDecisionLogger: %v", err)
	}
	t.Cleanup(func() { logger.Close() })
	return logger
}

func TestToolGate_SafeCommandAllowed(t *testing.T) {
	gate := NewToolGate(testPolicy(), nil, "dev", "/workspace", "sandbox-1")

	result, err := gate.Evaluate(context.Background(), []string{"git", "pull"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !result.Allowed {
		t.Error("git pull should be allowed")
	}
	if result.RiskClass != RiskSafe {
		t.Errorf("risk: got %q, want %q", result.RiskClass, RiskSafe)
	}
}

func TestToolGate_ReviewRequiredCommand(t *testing.T) {
	gate := NewToolGate(testPolicy(), nil, "dev", "/workspace", "sandbox-1")

	result, err := gate.Evaluate(context.Background(), []string{"git", "push"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !result.Allowed {
		t.Error("git push should be allowed (review-required but still allowed)")
	}
	if result.RiskClass != RiskReviewRequired {
		t.Errorf("risk: got %q, want %q", result.RiskClass, RiskReviewRequired)
	}
}

func TestToolGate_BlockedCommand(t *testing.T) {
	gate := NewToolGate(testPolicy(), nil, "dev", "/workspace", "sandbox-1")

	result, err := gate.Evaluate(context.Background(), []string{"curl", "https://evil.com"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Allowed {
		t.Error("curl should be blocked")
	}
	if result.RiskClass != RiskBlockedByDefault {
		t.Errorf("risk: got %q, want %q", result.RiskClass, RiskBlockedByDefault)
	}
}

func TestToolGate_UnmatchedCommandDefaultsSafe(t *testing.T) {
	gate := NewToolGate(testPolicy(), nil, "dev", "/workspace", "sandbox-1")

	result, err := gate.Evaluate(context.Background(), []string{"ls", "-la"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !result.Allowed {
		t.Error("unmatched command should default to safe")
	}
	if result.RiskClass != RiskSafe {
		t.Errorf("risk: got %q, want %q", result.RiskClass, RiskSafe)
	}
	if result.Rule != "default-safe" {
		t.Errorf("rule: got %q, want %q", result.Rule, "default-safe")
	}
}

func TestToolGate_NoPolicyDefaultsSafe(t *testing.T) {
	gate := NewToolGate(&Policy{Version: 1}, nil, "dev", "/workspace", "sandbox-1")

	result, err := gate.Evaluate(context.Background(), []string{"anything"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !result.Allowed {
		t.Error("no-rules policy should default to safe")
	}
	if result.RiskClass != RiskSafe {
		t.Errorf("risk: got %q, want %q", result.RiskClass, RiskSafe)
	}
}

func TestToolGate_NilPolicyDefaultsSafe(t *testing.T) {
	gate := NewToolGate(nil, nil, "dev", "/workspace", "sandbox-1")

	result, err := gate.Evaluate(context.Background(), []string{"anything"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !result.Allowed {
		t.Error("nil policy should default to safe")
	}
}

func TestToolGate_EvaluateAndEnforce_SafePasses(t *testing.T) {
	gate := NewToolGate(testPolicy(), nil, "dev", "/workspace", "sandbox-1")

	err := gate.EvaluateAndEnforce(context.Background(), []string{"git", "pull"}, ApprovalAsync)
	if err != nil {
		t.Errorf("safe command should pass: %v", err)
	}
}

func TestToolGate_EvaluateAndEnforce_ReviewAsyncPasses(t *testing.T) {
	logger := testDecisionLogger(t)
	gate := NewToolGate(testPolicy(), logger, "dev", "/workspace", "sandbox-1")

	err := gate.EvaluateAndEnforce(context.Background(), []string{"git", "push"}, ApprovalAsync)
	if err != nil {
		t.Errorf("review-required in async mode should pass: %v", err)
	}
}

func TestToolGate_EvaluateAndEnforce_ReviewSyncBlocks(t *testing.T) {
	gate := NewToolGate(testPolicy(), nil, "dev", "/workspace", "sandbox-1")

	err := gate.EvaluateAndEnforce(context.Background(), []string{"git", "push"}, ApprovalSync)
	if err == nil {
		t.Fatal("review-required in sync mode should return error")
	}

	var reviewErr *ReviewRequiredError
	if !errors.As(err, &reviewErr) {
		t.Fatalf("expected ReviewRequiredError, got %T: %v", err, err)
	}
	if reviewErr.RiskClass != RiskReviewRequired {
		t.Errorf("risk: got %q, want %q", reviewErr.RiskClass, RiskReviewRequired)
	}
}

func TestToolGate_EvaluateAndEnforce_BlockedReturnsError(t *testing.T) {
	gate := NewToolGate(testPolicy(), nil, "dev", "/workspace", "sandbox-1")

	err := gate.EvaluateAndEnforce(context.Background(), []string{"curl", "https://evil.com"}, ApprovalAsync)
	if err == nil {
		t.Fatal("blocked command should return error")
	}

	var blockedErr *BlockedError
	if !errors.As(err, &blockedErr) {
		t.Fatalf("expected BlockedError, got %T: %v", err, err)
	}
	if blockedErr.RiskClass != RiskBlockedByDefault {
		t.Errorf("risk: got %q, want %q", blockedErr.RiskClass, RiskBlockedByDefault)
	}
	if len(blockedErr.Command) == 0 {
		t.Error("BlockedError.Command should not be empty")
	}
}

func TestToolGate_BlockedError_Message(t *testing.T) {
	be := &BlockedError{
		Command:   []string{"curl", "https://external.com"},
		Rule:      ToolRule{Match: []string{"curl", "*"}, Allow: false, Risk: RiskBlockedByDefault},
		RiskClass: RiskBlockedByDefault,
		Reason:    `denied by rule "curl *"`,
	}

	msg := be.Error()
	if msg == "" {
		t.Fatal("error message should not be empty")
	}

	// Check key fragments exist.
	for _, fragment := range []string{"Command denied", "curl", "blocked-by-default", "policy amendment"} {
		if !containsStr(msg, fragment) {
			t.Errorf("error message should contain %q, got:\n%s", fragment, msg)
		}
	}
}

func TestToolGate_UpdatePolicy(t *testing.T) {
	gate := NewToolGate(testPolicy(), nil, "dev", "/workspace", "sandbox-1")

	// Initially, npm publish is blocked.
	result, _ := gate.Evaluate(context.Background(), []string{"npm", "publish"})
	if result.Allowed {
		t.Error("npm publish should initially be blocked")
	}

	// Update policy to allow npm publish.
	newPolicy := &Policy{
		Version: 2,
		Tools: ToolsPolicy{
			Rules: []ToolRule{
				{Match: []string{"npm", "publish"}, Allow: true, Risk: RiskSafe},
			},
		},
	}
	gate.UpdatePolicy(newPolicy)

	result, _ = gate.Evaluate(context.Background(), []string{"npm", "publish"})
	if !result.Allowed {
		t.Error("npm publish should be allowed after policy update")
	}
}

func TestToolGate_DecisionLogging(t *testing.T) {
	logger := testDecisionLogger(t)
	gate := NewToolGate(testPolicy(), logger, "dev", "/workspace", "sandbox-1")

	// Evaluate a command that triggers logging.
	_, err := gate.Evaluate(context.Background(), []string{"git", "push"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	// Flush and verify log entry exists.
	if err := logger.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	data, err := os.ReadFile(logger.config.Path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Error("decision log should contain entries after evaluation")
	}
}

func TestToolGate_ResultFields(t *testing.T) {
	gate := NewToolGate(testPolicy(), nil, "dev", "/workspace", "sandbox-1")

	result, err := gate.Evaluate(context.Background(), []string{"git", "push"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.PolicyVer == "" {
		t.Error("PolicyVer should not be empty")
	}
	if result.InputHash == "" {
		t.Error("InputHash should not be empty")
	}
	if result.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
	if result.Duration <= 0 {
		t.Error("Duration should be positive")
	}
	if result.Rule == "" {
		t.Error("Rule should not be empty")
	}
	if result.Reason == "" {
		t.Error("Reason should not be empty")
	}
}

// containsStr checks if s contains substr.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
