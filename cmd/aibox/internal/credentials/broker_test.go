package credentials

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBroker_InjectEnvVars_AllPresent(t *testing.T) {
	ctx := context.Background()
	p := NewMemoryProvider()

	_ = p.Store(ctx, &Credential{Type: CredGitToken, Value: "ghp_abc"})
	_ = p.Store(ctx, &Credential{Type: CredLLMAPIKey, Value: "sk-xyz"})
	_ = p.Store(ctx, &Credential{Type: CredMirrorToken, Value: "mirror-123"})

	b := NewBroker(p)
	envVars, err := b.InjectEnvVars(ctx)
	if err != nil {
		t.Fatalf("InjectEnvVars: %v", err)
	}

	if len(envVars) != 3 {
		t.Fatalf("got %d env vars, want 3", len(envVars))
	}

	envMap := make(map[string]string)
	for _, kv := range envVars {
		parts := strings.SplitN(kv, "=", 2)
		envMap[parts[0]] = parts[1]
	}

	if envMap["AIBOX_GIT_TOKEN"] != "ghp_abc" {
		t.Errorf("AIBOX_GIT_TOKEN = %q, want %q", envMap["AIBOX_GIT_TOKEN"], "ghp_abc")
	}
	if envMap["AIBOX_LLM_API_KEY"] != "sk-xyz" {
		t.Errorf("AIBOX_LLM_API_KEY = %q, want %q", envMap["AIBOX_LLM_API_KEY"], "sk-xyz")
	}
	if envMap["AIBOX_MIRROR_TOKEN"] != "mirror-123" {
		t.Errorf("AIBOX_MIRROR_TOKEN = %q, want %q", envMap["AIBOX_MIRROR_TOKEN"], "mirror-123")
	}
}

func TestBroker_InjectEnvVars_MissingSkipped(t *testing.T) {
	ctx := context.Background()
	p := NewMemoryProvider()

	// Only store one credential.
	_ = p.Store(ctx, &Credential{Type: CredGitToken, Value: "ghp_abc"})

	b := NewBroker(p)
	envVars, err := b.InjectEnvVars(ctx)
	if err != nil {
		t.Fatalf("InjectEnvVars: %v", err)
	}

	if len(envVars) != 1 {
		t.Fatalf("got %d env vars, want 1 (missing should be skipped)", len(envVars))
	}

	if !strings.HasPrefix(envVars[0], "AIBOX_GIT_TOKEN=") {
		t.Errorf("expected AIBOX_GIT_TOKEN, got %q", envVars[0])
	}
}

func TestBroker_InjectEnvVars_ExpiredSkipped(t *testing.T) {
	ctx := context.Background()
	p := NewMemoryProvider()

	_ = p.Store(ctx, &Credential{
		Type:      CredGitToken,
		Value:     "ghp_expired",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	})

	b := NewBroker(p)
	envVars, err := b.InjectEnvVars(ctx)
	if err != nil {
		t.Fatalf("InjectEnvVars: %v", err)
	}

	if len(envVars) != 0 {
		t.Errorf("expired credential should be skipped, got %v", envVars)
	}
}

func TestBroker_InjectEnvVars_Empty(t *testing.T) {
	ctx := context.Background()
	p := NewMemoryProvider()

	b := NewBroker(p)
	envVars, err := b.InjectEnvVars(ctx)
	if err != nil {
		t.Fatalf("InjectEnvVars: %v", err)
	}

	if len(envVars) != 0 {
		t.Errorf("got %d env vars, want 0", len(envVars))
	}
}

func TestBroker_ValidateCredentials_AllPresent(t *testing.T) {
	ctx := context.Background()
	p := NewMemoryProvider()

	_ = p.Store(ctx, &Credential{Type: CredGitToken, Value: "ghp_abc", Source: "memory"})
	_ = p.Store(ctx, &Credential{Type: CredLLMAPIKey, Value: "sk-xyz", Source: "memory"})
	_ = p.Store(ctx, &Credential{Type: CredMirrorToken, Value: "tok", Source: "memory"})

	b := NewBroker(p)
	statuses := b.ValidateCredentials(ctx)

	if len(statuses) != len(AllCredentialTypes) {
		t.Fatalf("got %d statuses, want %d", len(statuses), len(AllCredentialTypes))
	}

	for _, s := range statuses {
		if !s.Present {
			t.Errorf("credential %s should be present", s.Type)
		}
		if s.Expired {
			t.Errorf("credential %s should not be expired", s.Type)
		}
	}
}

func TestBroker_ValidateCredentials_Mixed(t *testing.T) {
	ctx := context.Background()
	p := NewMemoryProvider()

	// One present, one expired, one missing.
	_ = p.Store(ctx, &Credential{Type: CredGitToken, Value: "ghp_abc", Source: "memory"})
	_ = p.Store(ctx, &Credential{
		Type:      CredLLMAPIKey,
		Value:     "sk-expired",
		Source:    "memory",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	})

	b := NewBroker(p)
	statuses := b.ValidateCredentials(ctx)

	statusMap := make(map[CredentialType]CredentialStatus)
	for _, s := range statuses {
		statusMap[s.Type] = s
	}

	git := statusMap[CredGitToken]
	if !git.Present || git.Expired {
		t.Errorf("git-token: Present=%v Expired=%v, want Present=true Expired=false", git.Present, git.Expired)
	}

	llm := statusMap[CredLLMAPIKey]
	if !llm.Present || !llm.Expired {
		t.Errorf("llm-api-key: Present=%v Expired=%v, want Present=true Expired=true", llm.Present, llm.Expired)
	}

	mirror := statusMap[CredMirrorToken]
	if mirror.Present {
		t.Errorf("mirror-token: Present=%v, want false", mirror.Present)
	}
}

func TestBroker_ValidateCredentials_WithExpiry(t *testing.T) {
	ctx := context.Background()
	p := NewMemoryProvider()

	future := time.Now().Add(2 * time.Hour)
	_ = p.Store(ctx, &Credential{
		Type:      CredGitToken,
		Value:     "ghp_abc",
		Source:    "memory",
		ExpiresAt: future,
	})

	b := NewBroker(p)
	statuses := b.ValidateCredentials(ctx)

	for _, s := range statuses {
		if s.Type == CredGitToken {
			if s.Expired {
				t.Error("git-token should not be expired")
			}
			if s.ExpiresIn <= 0 {
				t.Errorf("ExpiresIn = %v, want positive duration", s.ExpiresIn)
			}
		}
	}
}

func TestBroker_Provider(t *testing.T) {
	p := NewMemoryProvider()
	b := NewBroker(p)

	if b.Provider() != p {
		t.Error("Provider() should return the underlying provider")
	}
}
