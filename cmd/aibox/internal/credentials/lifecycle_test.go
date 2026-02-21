package credentials

import (
	"context"
	"testing"
	"time"
)

func TestMintAll(t *testing.T) {
	mp := NewMemoryProvider()
	ctx := context.Background()

	// Seed the provider with credentials.
	_ = mp.Store(ctx, &Credential{
		Type:      CredGitToken,
		Value:     "git-token-123",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Source:    "vault",
	})
	_ = mp.Store(ctx, &Credential{
		Type:      CredLLMAPIKey,
		Value:     "llm-key-456",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Source:    "vault",
	})
	_ = mp.Store(ctx, &Credential{
		Type:      CredMirrorToken,
		Value:     "mirror-tok-789",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Source:    "vault",
	})

	lm := NewLifecycleManager(mp, "sandbox-1", "testuser")
	envVars, err := lm.MintAll(ctx)
	if err != nil {
		t.Fatalf("MintAll failed: %v", err)
	}

	if len(envVars) != 3 {
		t.Fatalf("expected 3 env vars, got %d: %v", len(envVars), envVars)
	}

	expected := map[string]string{
		"AIBOX_GIT_TOKEN":    "git-token-123",
		"AIBOX_LLM_API_KEY":  "llm-key-456",
		"AIBOX_MIRROR_TOKEN": "mirror-tok-789",
	}

	for _, env := range envVars {
		found := false
		for k, v := range expected {
			if env == k+"="+v {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unexpected env var: %q", env)
		}
	}
}

func TestMintAllSkipsExpired(t *testing.T) {
	mp := NewMemoryProvider()
	ctx := context.Background()

	// Store one valid and one expired credential.
	_ = mp.Store(ctx, &Credential{
		Type:      CredGitToken,
		Value:     "git-token-123",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Source:    "vault",
	})
	_ = mp.Store(ctx, &Credential{
		Type:      CredLLMAPIKey,
		Value:     "expired-key",
		ExpiresAt: time.Now().Add(-1 * time.Hour), // expired
		Source:    "vault",
	})

	lm := NewLifecycleManager(mp, "sandbox-1", "testuser")
	envVars, err := lm.MintAll(ctx)
	if err != nil {
		t.Fatalf("MintAll failed: %v", err)
	}

	if len(envVars) != 1 {
		t.Fatalf("expected 1 env var (expired should be skipped), got %d: %v", len(envVars), envVars)
	}

	if envVars[0] != "AIBOX_GIT_TOKEN=git-token-123" {
		t.Errorf("unexpected env var: %q", envVars[0])
	}
}

func TestMintAllSkipsMissing(t *testing.T) {
	mp := NewMemoryProvider()
	ctx := context.Background()

	// Empty provider - no credentials available.
	lm := NewLifecycleManager(mp, "sandbox-1", "testuser")
	envVars, err := lm.MintAll(ctx)
	if err != nil {
		t.Fatalf("MintAll failed: %v", err)
	}

	if len(envVars) != 0 {
		t.Fatalf("expected 0 env vars for empty provider, got %d: %v", len(envVars), envVars)
	}
}

func TestRevokeAll(t *testing.T) {
	mp := NewMemoryProvider()
	ctx := context.Background()

	// Seed and mint.
	_ = mp.Store(ctx, &Credential{
		Type:      CredGitToken,
		Value:     "git-token-123",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Source:    "vault",
	})
	_ = mp.Store(ctx, &Credential{
		Type:      CredLLMAPIKey,
		Value:     "llm-key-456",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Source:    "vault",
	})

	lm := NewLifecycleManager(mp, "sandbox-1", "testuser")
	_, err := lm.MintAll(ctx)
	if err != nil {
		t.Fatalf("MintAll failed: %v", err)
	}

	// Verify leases exist.
	status := lm.Status(ctx)
	if len(status) != 2 {
		t.Fatalf("expected 2 leases before revoke, got %d", len(status))
	}

	// Revoke all.
	if err := lm.RevokeAll(ctx); err != nil {
		t.Fatalf("RevokeAll failed: %v", err)
	}

	// Verify leases are cleared.
	status = lm.Status(ctx)
	if len(status) != 0 {
		t.Fatalf("expected 0 leases after revoke, got %d", len(status))
	}
}

func TestRevokeAllEmpty(t *testing.T) {
	mp := NewMemoryProvider()
	ctx := context.Background()

	lm := NewLifecycleManager(mp, "sandbox-1", "testuser")

	// Revoking with no leases should succeed.
	if err := lm.RevokeAll(ctx); err != nil {
		t.Fatalf("RevokeAll on empty should not fail: %v", err)
	}
}

func TestStatus(t *testing.T) {
	mp := NewMemoryProvider()
	ctx := context.Background()

	_ = mp.Store(ctx, &Credential{
		Type:      CredGitToken,
		Value:     "token-1",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Source:    "vault",
	})

	lm := NewLifecycleManager(mp, "sandbox-2", "admin")
	_, _ = lm.MintAll(ctx)

	status := lm.Status(ctx)
	if len(status) != 1 {
		t.Fatalf("expected 1 lease in status, got %d", len(status))
	}

	if status[0].CredType != CredGitToken {
		t.Errorf("expected CredGitToken, got %q", status[0].CredType)
	}
	if status[0].LeaseID != "sandbox-2/admin/git-token" {
		t.Errorf("unexpected lease ID: %q", status[0].LeaseID)
	}
	if status[0].MintedAt.IsZero() {
		t.Error("MintedAt should not be zero")
	}
}

func TestRefreshExpiring(t *testing.T) {
	mp := NewMemoryProvider()
	ctx := context.Background()

	expiresAt := time.Now().Add(2 * time.Minute) // expiring soon

	_ = mp.Store(ctx, &Credential{
		Type:      CredGitToken,
		Value:     "git-token-old",
		ExpiresAt: expiresAt,
		Source:    "vault",
	})

	lm := NewLifecycleManager(mp, "sandbox-1", "testuser")
	_, _ = lm.MintAll(ctx)

	// Update the credential in the provider (simulating Vault returning a fresh one).
	newExpiry := time.Now().Add(1 * time.Hour)
	_ = mp.Store(ctx, &Credential{
		Type:      CredGitToken,
		Value:     "git-token-new",
		ExpiresAt: newExpiry,
		Source:    "vault",
	})

	// Refresh with a 5-minute threshold (2min < 5min, so it should refresh).
	err := lm.RefreshExpiring(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("RefreshExpiring failed: %v", err)
	}

	status := lm.Status(ctx)
	if len(status) != 1 {
		t.Fatalf("expected 1 lease, got %d", len(status))
	}

	// The expiry should have been updated.
	if status[0].ExpiresAt.Equal(expiresAt) {
		t.Error("lease expiry was not updated after refresh")
	}
}

func TestRefreshExpiringSkipsNonExpiring(t *testing.T) {
	mp := NewMemoryProvider()
	ctx := context.Background()

	_ = mp.Store(ctx, &Credential{
		Type:      CredGitToken,
		Value:     "git-token-ok",
		ExpiresAt: time.Now().Add(1 * time.Hour), // plenty of time
		Source:    "vault",
	})

	lm := NewLifecycleManager(mp, "sandbox-1", "testuser")
	_, _ = lm.MintAll(ctx)

	beforeStatus := lm.Status(ctx)
	beforeMint := beforeStatus[0].MintedAt

	// Threshold of 5 minutes; credential expires in 1 hour, so no refresh needed.
	err := lm.RefreshExpiring(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("RefreshExpiring failed: %v", err)
	}

	afterStatus := lm.Status(ctx)
	if !afterStatus[0].MintedAt.Equal(beforeMint) {
		t.Error("credential was refreshed when it should not have been")
	}
}

func TestRefreshExpiringSkipsZeroExpiry(t *testing.T) {
	mp := NewMemoryProvider()
	ctx := context.Background()

	_ = mp.Store(ctx, &Credential{
		Type:   CredGitToken,
		Value:  "git-token-no-expiry",
		Source: "vault",
		// ExpiresAt is zero - no expiry
	})

	lm := NewLifecycleManager(mp, "sandbox-1", "testuser")
	_, _ = lm.MintAll(ctx)

	beforeStatus := lm.Status(ctx)
	beforeMint := beforeStatus[0].MintedAt

	err := lm.RefreshExpiring(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("RefreshExpiring failed: %v", err)
	}

	afterStatus := lm.Status(ctx)
	if !afterStatus[0].MintedAt.Equal(beforeMint) {
		t.Error("credential with zero expiry was refreshed when it should not have been")
	}
}
