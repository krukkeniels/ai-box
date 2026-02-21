package policy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPolicy(t *testing.T) {
	dir := t.TempDir()
	policyFile := filepath.Join(dir, "policy.yaml")

	content := `version: 1
network:
  mode: deny-by-default
  allow:
    - id: registry
      hosts:
        - harbor.internal
      ports:
        - 443
filesystem:
  workspace_root: /workspace
  deny:
    - /etc/shadow
tools:
  rules:
    - match: [git, push]
      allow: true
      risk: review-required
resources:
  cpu: "4"
  memory: 8g
  disk: 50g
runtime:
  engine: gvisor
  rootless: true
credentials:
  git_token_ttl: 4h
  llm_api_key_ttl: 8h
  mirror_token_ttl: 8h
  revoke_on_stop: true
  no_persist_to_workspace: true
`
	if err := os.WriteFile(policyFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadPolicy(policyFile)
	if err != nil {
		t.Fatalf("LoadPolicy: %v", err)
	}

	if p.Version != 1 {
		t.Errorf("version: got %d, want 1", p.Version)
	}
	if p.Network.Mode != "deny-by-default" {
		t.Errorf("network.mode: got %q", p.Network.Mode)
	}
	if len(p.Network.Allow) != 1 {
		t.Fatalf("network.allow: got %d entries", len(p.Network.Allow))
	}
	if p.Network.Allow[0].Hosts[0] != "harbor.internal" {
		t.Errorf("host: got %q", p.Network.Allow[0].Hosts[0])
	}
	if p.Runtime.Engine != "gvisor" {
		t.Errorf("engine: got %q", p.Runtime.Engine)
	}
	if !p.Runtime.Rootless {
		t.Error("rootless should be true")
	}
	if p.Credentials.GitTokenTTL != "4h" {
		t.Errorf("git_token_ttl: got %q", p.Credentials.GitTokenTTL)
	}
}

func TestLoadPolicy_FileNotFound(t *testing.T) {
	_, err := LoadPolicy("/nonexistent/path/policy.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadPolicy_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	policyFile := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(policyFile, []byte("{{invalid yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadPolicy(policyFile)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadPolicyHierarchy(t *testing.T) {
	dir := t.TempDir()

	orgContent := `version: 1
runtime:
  engine: gvisor
  rootless: true
`
	teamContent := `version: 1
resources:
  cpu: "2"
`
	projectContent := `version: 1
resources:
  cpu: "1"
`

	orgPath := filepath.Join(dir, "org.yaml")
	teamPath := filepath.Join(dir, "team.yaml")
	projPath := filepath.Join(dir, "project.yaml")

	for _, tc := range []struct {
		path    string
		content string
	}{
		{orgPath, orgContent},
		{teamPath, teamContent},
		{projPath, projectContent},
	} {
		if err := os.WriteFile(tc.path, []byte(tc.content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	org, team, project, err := LoadPolicyHierarchy(orgPath, teamPath, projPath)
	if err != nil {
		t.Fatalf("LoadPolicyHierarchy: %v", err)
	}

	if org.Runtime.Engine != "gvisor" {
		t.Errorf("org engine: got %q", org.Runtime.Engine)
	}
	if team.Resources.CPU != "2" {
		t.Errorf("team cpu: got %q", team.Resources.CPU)
	}
	if project.Resources.CPU != "1" {
		t.Errorf("project cpu: got %q", project.Resources.CPU)
	}
}

func TestLoadPolicyHierarchy_OrgRequired(t *testing.T) {
	_, _, _, err := LoadPolicyHierarchy("", "", "")
	if err == nil {
		t.Fatal("expected error when org path is empty")
	}
}

func TestLoadPolicyHierarchy_OptionalLevels(t *testing.T) {
	dir := t.TempDir()
	orgPath := filepath.Join(dir, "org.yaml")
	if err := os.WriteFile(orgPath, []byte("version: 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	org, team, project, err := LoadPolicyHierarchy(orgPath, "", "")
	if err != nil {
		t.Fatalf("LoadPolicyHierarchy: %v", err)
	}

	if org == nil {
		t.Fatal("org should not be nil")
	}
	if team != nil {
		t.Error("team should be nil when path is empty")
	}
	if project != nil {
		t.Error("project should be nil when path is empty")
	}
}
