package policy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testPolicyDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Write a minimal Rego policy (OPA v1 syntax).
	regoContent := `package aibox

default allow = false

allow if {
    input.action == "command"
    input.command[0] == "echo"
}

deny contains msg if {
    input.action == "command"
    input.command[0] == "rm"
    msg := "rm command is not permitted"
}
`
	if err := os.WriteFile(filepath.Join(dir, "test.rego"), []byte(regoContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a policy.yaml.
	yamlContent := `version: 1
network:
  mode: deny-by-default
  allow:
    - id: registry
      hosts:
        - harbor.internal
      ports:
        - 443
    - id: git
      hosts:
        - git.internal
      ports:
        - 443
filesystem:
  deny:
    - /etc/shadow
    - /proc/kcore
tools:
  rules:
    - match: [git, push]
      allow: true
      risk: review-required
    - match: [git, pull]
      allow: true
      risk: safe
    - match: [curl, "*"]
      allow: false
      risk: blocked-by-default
resources:
  cpu: "4"
  memory: 8g
runtime:
  engine: gvisor
  rootless: true
credentials:
  git_token_ttl: 4h
  revoke_on_stop: true
  no_persist_to_workspace: true
`
	if err := os.WriteFile(filepath.Join(dir, "policy.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestNewEngine(t *testing.T) {
	dir := testPolicyDir(t)
	engine, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if engine == nil {
		t.Fatal("engine is nil")
	}
	if engine.policyVer == "" {
		t.Error("policyVer should not be empty")
	}
}

func TestEngine_EvaluateCommand_GitPush(t *testing.T) {
	dir := testPolicyDir(t)
	engine, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	input := PolicyInput{
		Action:    "command",
		Command:   []string{"git", "push"},
		User:      "dev",
		Workspace: "/workspace",
		Timestamp: time.Now(),
	}

	result, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if !result.Allowed {
		t.Error("git push should be allowed")
	}
	if result.RiskClass != RiskReviewRequired {
		t.Errorf("risk: got %q, want %q", result.RiskClass, RiskReviewRequired)
	}
	if result.PolicyVer == "" {
		t.Error("policy version should not be empty")
	}
	if result.InputHash == "" {
		t.Error("input hash should not be empty")
	}
}

func TestEngine_EvaluateCommand_CurlBlocked(t *testing.T) {
	dir := testPolicyDir(t)
	engine, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	input := PolicyInput{
		Action:    "command",
		Command:   []string{"curl", "http://evil.com"},
		User:      "dev",
		Workspace: "/workspace",
		Timestamp: time.Now(),
	}

	result, err := engine.Evaluate(context.Background(), input)
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

func TestEngine_EvaluateCommand_NoMatchingRule(t *testing.T) {
	dir := testPolicyDir(t)
	engine, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	input := PolicyInput{
		Action:    "command",
		Command:   []string{"rm", "-rf", "/"},
		User:      "dev",
		Workspace: "/workspace",
		Timestamp: time.Now(),
	}

	result, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.Allowed {
		t.Error("unmatched command should be denied by default")
	}
}

func TestEngine_EvaluateNetwork_Allowed(t *testing.T) {
	dir := testPolicyDir(t)
	engine, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	input := PolicyInput{
		Action:    "network",
		Target:    "harbor.internal",
		User:      "dev",
		Timestamp: time.Now(),
	}

	result, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if !result.Allowed {
		t.Error("harbor.internal should be allowed")
	}
}

func TestEngine_EvaluateNetwork_Denied(t *testing.T) {
	dir := testPolicyDir(t)
	engine, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	input := PolicyInput{
		Action:    "network",
		Target:    "evil.com",
		User:      "dev",
		Timestamp: time.Now(),
	}

	result, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.Allowed {
		t.Error("evil.com should be denied")
	}
}

func TestEngine_EvaluateFilesystem_Denied(t *testing.T) {
	dir := testPolicyDir(t)
	engine, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	input := PolicyInput{
		Action:    "filesystem",
		Target:    "/etc/shadow",
		User:      "dev",
		Timestamp: time.Now(),
	}

	result, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.Allowed {
		t.Error("/etc/shadow should be denied")
	}
}

func TestEngine_EvaluateFilesystem_Allowed(t *testing.T) {
	dir := testPolicyDir(t)
	engine, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	input := PolicyInput{
		Action:    "filesystem",
		Target:    "/workspace/main.go",
		User:      "dev",
		Timestamp: time.Now(),
	}

	result, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if !result.Allowed {
		t.Error("/workspace/main.go should be allowed")
	}
}

func TestEngine_Reload(t *testing.T) {
	dir := testPolicyDir(t)
	engine, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	oldVer := engine.policyVer

	// Update the policy.yaml
	newContent := `version: 2
network:
  mode: deny-by-default
runtime:
  engine: gvisor
  rootless: true
`
	if err := os.WriteFile(filepath.Join(dir, "policy.yaml"), []byte(newContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := engine.Reload(dir); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if engine.policyVer == oldVer {
		t.Error("policy version should change after reload")
	}

	if engine.EffectivePolicy().Version != 2 {
		t.Errorf("version: got %d, want 2", engine.EffectivePolicy().Version)
	}
}

func TestEngine_EffectivePolicy(t *testing.T) {
	dir := testPolicyDir(t)
	engine, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	p := engine.EffectivePolicy()
	if p == nil {
		t.Fatal("effective policy is nil")
	}
	if p.Version != 1 {
		t.Errorf("version: got %d, want 1", p.Version)
	}
}

func TestNewEngineFromPolicy(t *testing.T) {
	dir := testPolicyDir(t)
	p := &Policy{
		Version: 1,
		Tools: ToolsPolicy{
			Rules: []ToolRule{
				{Match: []string{"echo"}, Allow: true, Risk: RiskSafe},
			},
		},
	}

	engine, err := NewEngineFromPolicy(p, dir)
	if err != nil {
		t.Fatalf("NewEngineFromPolicy: %v", err)
	}

	input := PolicyInput{
		Action:    "command",
		Command:   []string{"echo"},
		Timestamp: time.Now(),
	}

	result, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !result.Allowed {
		t.Error("echo should be allowed")
	}
}

func TestEngine_EmptyPolicyDir(t *testing.T) {
	dir := t.TempDir()

	engine, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine with empty dir: %v", err)
	}

	// Should still work with default policy.
	if engine.EffectivePolicy() == nil {
		t.Error("effective policy should not be nil")
	}
}

func TestEngine_Duration(t *testing.T) {
	dir := testPolicyDir(t)
	engine, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	input := PolicyInput{
		Action:    "command",
		Command:   []string{"git", "pull"},
		Timestamp: time.Now(),
	}

	result, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.Duration <= 0 {
		t.Error("duration should be positive")
	}
}

func TestHashInput(t *testing.T) {
	input := PolicyInput{
		Action:    "command",
		Command:   []string{"git", "push"},
		Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	hash1 := hashInput(input)
	hash2 := hashInput(input)

	if hash1 != hash2 {
		t.Error("same input should produce same hash")
	}
	if hash1 == "" {
		t.Error("hash should not be empty")
	}

	// Different input should produce different hash.
	input.Action = "network"
	hash3 := hashInput(input)
	if hash3 == hash1 {
		t.Error("different input should produce different hash")
	}
}

func TestHashPolicy(t *testing.T) {
	p := &Policy{Version: 1}
	hash := hashPolicy(p)
	if hash == "" {
		t.Error("hash should not be empty")
	}
}
