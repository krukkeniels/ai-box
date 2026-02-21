package policy

import "testing"

func TestMatchCommand_ExactMatch(t *testing.T) {
	if !MatchCommand([]string{"git", "push"}, []string{"git", "push"}) {
		t.Error("exact match should succeed")
	}
}

func TestMatchCommand_PrefixMatch(t *testing.T) {
	if !MatchCommand([]string{"git", "push", "origin", "main"}, []string{"git", "push"}) {
		t.Error("prefix match should succeed")
	}
}

func TestMatchCommand_Wildcard(t *testing.T) {
	if !MatchCommand([]string{"curl", "https://example.com"}, []string{"curl", "*"}) {
		t.Error("wildcard match should succeed")
	}
}

func TestMatchCommand_WildcardWithMoreArgs(t *testing.T) {
	if !MatchCommand([]string{"curl", "-s", "https://example.com"}, []string{"curl", "*"}) {
		t.Error("wildcard should match any second token and allow extra args")
	}
}

func TestMatchCommand_NoMatch(t *testing.T) {
	if MatchCommand([]string{"npm", "publish"}, []string{"git", "push"}) {
		t.Error("different commands should not match")
	}
}

func TestMatchCommand_CommandShorterThanPattern(t *testing.T) {
	if MatchCommand([]string{"git"}, []string{"git", "push"}) {
		t.Error("command shorter than pattern should not match")
	}
}

func TestMatchCommand_EmptyCommand(t *testing.T) {
	if MatchCommand([]string{}, []string{"git"}) {
		t.Error("empty command should not match non-empty pattern")
	}
}

func TestMatchCommand_EmptyPattern(t *testing.T) {
	if MatchCommand([]string{"git"}, []string{}) {
		t.Error("empty pattern should not match")
	}
}

func TestMatchCommand_BothEmpty(t *testing.T) {
	if MatchCommand([]string{}, []string{}) {
		t.Error("both empty should not match (empty pattern)")
	}
}

func TestMatchCommand_SingleElement(t *testing.T) {
	if !MatchCommand([]string{"ls"}, []string{"ls"}) {
		t.Error("single element exact match should succeed")
	}
}

func TestMatchCommand_SingleElementPrefix(t *testing.T) {
	if !MatchCommand([]string{"ls", "-la"}, []string{"ls"}) {
		t.Error("single element prefix match should succeed")
	}
}

func TestMatchCommand_LongPattern(t *testing.T) {
	if !MatchCommand([]string{"rm", "-rf", "/workspace"}, []string{"rm", "-rf", "/workspace"}) {
		t.Error("exact long match should succeed")
	}
}

func TestMatchCommand_LongPatternExtended(t *testing.T) {
	if !MatchCommand([]string{"rm", "-rf", "/workspace", "extra"}, []string{"rm", "-rf", "/workspace"}) {
		t.Error("long pattern with extension should succeed")
	}
}

func TestMatchCommand_PartialMismatch(t *testing.T) {
	if MatchCommand([]string{"git", "pull"}, []string{"git", "push"}) {
		t.Error("partial mismatch in second token should fail")
	}
}

func TestFindMatchingRule_FirstMatchWins(t *testing.T) {
	rules := []ToolRule{
		{Match: []string{"git", "push"}, Allow: true, Risk: RiskReviewRequired},
		{Match: []string{"git", "*"}, Allow: true, Risk: RiskSafe},
	}

	rule := FindMatchingRule([]string{"git", "push"}, rules)
	if rule == nil {
		t.Fatal("expected a matching rule")
	}
	if rule.Risk != RiskReviewRequired {
		t.Errorf("first match should win: got risk %q, want %q", rule.Risk, RiskReviewRequired)
	}
}

func TestFindMatchingRule_WildcardFallback(t *testing.T) {
	rules := []ToolRule{
		{Match: []string{"git", "push"}, Allow: true, Risk: RiskReviewRequired},
		{Match: []string{"git", "*"}, Allow: true, Risk: RiskSafe},
	}

	rule := FindMatchingRule([]string{"git", "pull"}, rules)
	if rule == nil {
		t.Fatal("expected a matching rule via wildcard")
	}
	if rule.Risk != RiskSafe {
		t.Errorf("wildcard fallback: got risk %q, want %q", rule.Risk, RiskSafe)
	}
}

func TestFindMatchingRule_NoMatch(t *testing.T) {
	rules := []ToolRule{
		{Match: []string{"git", "push"}, Allow: true, Risk: RiskReviewRequired},
	}

	rule := FindMatchingRule([]string{"npm", "publish"}, rules)
	if rule != nil {
		t.Error("expected nil for no matching rule")
	}
}

func TestFindMatchingRule_EmptyCommand(t *testing.T) {
	rules := []ToolRule{
		{Match: []string{"git"}, Allow: true, Risk: RiskSafe},
	}

	rule := FindMatchingRule([]string{}, rules)
	if rule != nil {
		t.Error("expected nil for empty command")
	}
}

func TestFindMatchingRule_EmptyRules(t *testing.T) {
	rule := FindMatchingRule([]string{"git", "push"}, nil)
	if rule != nil {
		t.Error("expected nil for empty rules")
	}
}

func TestRiskClassPriority_ValidClasses(t *testing.T) {
	tests := []struct {
		class string
		want  int
	}{
		{RiskSafe, 0},
		{RiskReviewRequired, 1},
		{RiskBlockedByDefault, 2},
	}

	for _, tt := range tests {
		got := RiskClassPriority(tt.class)
		if got != tt.want {
			t.Errorf("RiskClassPriority(%q): got %d, want %d", tt.class, got, tt.want)
		}
	}
}

func TestRiskClassPriority_Ordering(t *testing.T) {
	safe := RiskClassPriority(RiskSafe)
	review := RiskClassPriority(RiskReviewRequired)
	blocked := RiskClassPriority(RiskBlockedByDefault)

	if safe >= review {
		t.Error("safe should be less restrictive than review-required")
	}
	if review >= blocked {
		t.Error("review-required should be less restrictive than blocked-by-default")
	}
}

func TestRiskClassPriority_Unknown(t *testing.T) {
	got := RiskClassPriority("unknown")
	if got != -1 {
		t.Errorf("RiskClassPriority(unknown): got %d, want -1", got)
	}
}
