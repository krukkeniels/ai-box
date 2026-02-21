package credentials

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// runProviderContractTests exercises the Provider interface contract against
// any implementation.
func runProviderContractTests(t *testing.T, p Provider) {
	t.Helper()
	ctx := context.Background()

	t.Run("GetMissing", func(t *testing.T) {
		_, err := p.Get(ctx, CredGitToken)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("Get missing: got err=%v, want ErrNotFound", err)
		}
	})

	t.Run("StoreAndGet", func(t *testing.T) {
		cred := &Credential{
			Type:   CredGitToken,
			Value:  "ghp_test123",
			Source: p.Name(),
		}
		if err := p.Store(ctx, cred); err != nil {
			t.Fatalf("Store: %v", err)
		}

		got, err := p.Get(ctx, CredGitToken)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Value != "ghp_test123" {
			t.Errorf("Value = %q, want %q", got.Value, "ghp_test123")
		}
		if got.Type != CredGitToken {
			t.Errorf("Type = %q, want %q", got.Type, CredGitToken)
		}
	})

	t.Run("StoreOverwrite", func(t *testing.T) {
		cred := &Credential{
			Type:   CredGitToken,
			Value:  "ghp_updated",
			Source: p.Name(),
		}
		if err := p.Store(ctx, cred); err != nil {
			t.Fatalf("Store: %v", err)
		}

		got, err := p.Get(ctx, CredGitToken)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Value != "ghp_updated" {
			t.Errorf("Value = %q, want %q", got.Value, "ghp_updated")
		}
	})

	t.Run("List", func(t *testing.T) {
		// Store a second credential.
		if err := p.Store(ctx, &Credential{
			Type:  CredLLMAPIKey,
			Value: "sk-test",
		}); err != nil {
			t.Fatalf("Store: %v", err)
		}

		types, err := p.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}

		found := make(map[CredentialType]bool)
		for _, ct := range types {
			found[ct] = true
		}
		if !found[CredGitToken] {
			t.Error("List should include git-token")
		}
		if !found[CredLLMAPIKey] {
			t.Error("List should include llm-api-key")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := p.Delete(ctx, CredGitToken); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		_, err := p.Get(ctx, CredGitToken)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("Get after Delete: got err=%v, want ErrNotFound", err)
		}
	})

	t.Run("DeleteMissing", func(t *testing.T) {
		err := p.Delete(ctx, CredMirrorToken)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("Delete missing: got err=%v, want ErrNotFound", err)
		}
	})

	t.Run("Name", func(t *testing.T) {
		name := p.Name()
		if name == "" {
			t.Error("Name() should not be empty")
		}
	})
}

func TestMemoryProvider_Contract(t *testing.T) {
	runProviderContractTests(t, NewMemoryProvider())
}

func TestFileProvider_Contract(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.enc")
	key := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	p := NewFileProviderWithKey(path, key)
	runProviderContractTests(t, p)
}

func TestFileProvider_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.enc")
	key := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	ctx := context.Background()

	// Store with one instance.
	p1 := NewFileProviderWithKey(path, key)
	if err := p1.Store(ctx, &Credential{
		Type:  CredGitToken,
		Value: "persist-test",
	}); err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Read with a new instance to verify persistence.
	p2 := NewFileProviderWithKey(path, key)
	got, err := p2.Get(ctx, CredGitToken)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Value != "persist-test" {
		t.Errorf("Value = %q, want %q", got.Value, "persist-test")
	}
}

func TestFileProvider_WrongKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.enc")
	key1 := [32]byte{1, 2, 3}
	key2 := [32]byte{4, 5, 6}

	ctx := context.Background()

	p1 := NewFileProviderWithKey(path, key1)
	if err := p1.Store(ctx, &Credential{
		Type:  CredGitToken,
		Value: "secret",
	}); err != nil {
		t.Fatalf("Store: %v", err)
	}

	p2 := NewFileProviderWithKey(path, key2)
	_, err := p2.Get(ctx, CredGitToken)
	if err == nil {
		t.Error("Get with wrong key should fail")
	}
}

func TestFileProvider_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.enc")
	key := [32]byte{1}

	ctx := context.Background()
	p := NewFileProviderWithKey(path, key)
	if err := p.Store(ctx, &Credential{
		Type:  CredGitToken,
		Value: "test",
	}); err != nil {
		t.Fatalf("Store: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}
}

func TestFileProvider_ExpiresAt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.enc")
	key := [32]byte{1}
	ctx := context.Background()

	p := NewFileProviderWithKey(path, key)
	expiry := time.Now().Add(1 * time.Hour).Truncate(time.Millisecond)

	if err := p.Store(ctx, &Credential{
		Type:      CredGitToken,
		Value:     "expiring-token",
		ExpiresAt: expiry,
	}); err != nil {
		t.Fatalf("Store: %v", err)
	}

	got, err := p.Get(ctx, CredGitToken)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// JSON time marshalling may lose sub-millisecond precision.
	if !got.ExpiresAt.Truncate(time.Millisecond).Equal(expiry) {
		t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, expiry)
	}
}

func TestCredential_IsExpired(t *testing.T) {
	tests := []struct {
		name    string
		cred    Credential
		expired bool
	}{
		{
			name:    "no expiry",
			cred:    Credential{Type: CredGitToken, Value: "x"},
			expired: false,
		},
		{
			name:    "future expiry",
			cred:    Credential{Type: CredGitToken, Value: "x", ExpiresAt: time.Now().Add(1 * time.Hour)},
			expired: false,
		},
		{
			name:    "past expiry",
			cred:    Credential{Type: CredGitToken, Value: "x", ExpiresAt: time.Now().Add(-1 * time.Hour)},
			expired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cred.IsExpired(); got != tt.expired {
				t.Errorf("IsExpired() = %v, want %v", got, tt.expired)
			}
		})
	}
}

func TestValidCredentialType(t *testing.T) {
	if !ValidCredentialType("git-token") {
		t.Error("git-token should be valid")
	}
	if !ValidCredentialType("llm-api-key") {
		t.Error("llm-api-key should be valid")
	}
	if ValidCredentialType("unknown") {
		t.Error("unknown should not be valid")
	}
}
