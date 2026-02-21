package credentials

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestCachedProviderCacheHit(t *testing.T) {
	inner := NewMemoryProvider()
	ctx := context.Background()

	_ = inner.Store(ctx, &Credential{
		Type:   CredGitToken,
		Value:  "inner-token",
		Source: "memory",
	})

	cached := NewCachedProvider(inner, 1*time.Hour)

	// First call fetches from inner.
	cred, err := cached.Get(ctx, CredGitToken)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if cred.Value != "inner-token" {
		t.Errorf("Get() value = %q, want %q", cred.Value, "inner-token")
	}

	// Modify inner to verify second call uses cache.
	_ = inner.Store(ctx, &Credential{
		Type:   CredGitToken,
		Value:  "updated-token",
		Source: "memory",
	})

	cred, err = cached.Get(ctx, CredGitToken)
	if err != nil {
		t.Fatalf("second Get() error = %v", err)
	}
	if cred.Value != "inner-token" {
		t.Errorf("cached Get() value = %q, want %q (cached)", cred.Value, "inner-token")
	}
}

func TestCachedProviderCacheMiss(t *testing.T) {
	inner := NewMemoryProvider()
	ctx := context.Background()

	cached := NewCachedProvider(inner, 1*time.Hour)

	// Cache miss, inner also empty.
	_, err := cached.Get(ctx, CredGitToken)
	if err != ErrNotFound {
		t.Errorf("Get() on empty = %v, want ErrNotFound", err)
	}
}

func TestCachedProviderTTLExpiry(t *testing.T) {
	inner := NewMemoryProvider()
	ctx := context.Background()

	_ = inner.Store(ctx, &Credential{
		Type:   CredGitToken,
		Value:  "v1",
		Source: "memory",
	})

	cached := NewCachedProvider(inner, 50*time.Millisecond)

	// Populate cache.
	_, err := cached.Get(ctx, CredGitToken)
	if err != nil {
		t.Fatalf("first Get() error = %v", err)
	}

	// Update inner.
	_ = inner.Store(ctx, &Credential{
		Type:   CredGitToken,
		Value:  "v2",
		Source: "memory",
	})

	// Wait for cache TTL to expire.
	time.Sleep(100 * time.Millisecond)

	// Should fetch fresh from inner.
	cred, err := cached.Get(ctx, CredGitToken)
	if err != nil {
		t.Fatalf("Get() after TTL error = %v", err)
	}
	if cred.Value != "v2" {
		t.Errorf("Get() after TTL = %q, want %q", cred.Value, "v2")
	}
}

func TestCachedProviderFallbackOnInnerFailure(t *testing.T) {
	inner := &failingProvider{
		Provider:  NewMemoryProvider(),
		failAfter: 1,
	}
	ctx := context.Background()

	_ = inner.Store(ctx, &Credential{
		Type:   CredGitToken,
		Value:  "cached-value",
		Source: "memory",
	})

	cached := NewCachedProvider(inner, 50*time.Millisecond)

	// First call succeeds and caches.
	_, err := cached.Get(ctx, CredGitToken)
	if err != nil {
		t.Fatalf("first Get() error = %v", err)
	}

	// Wait for TTL to expire, inner will now fail.
	time.Sleep(100 * time.Millisecond)

	// Should return stale cached value.
	cred, err := cached.Get(ctx, CredGitToken)
	if err != nil {
		t.Fatalf("Get() with failing inner error = %v", err)
	}
	if cred.Value != "cached-value" {
		t.Errorf("Get() fallback value = %q, want %q", cred.Value, "cached-value")
	}
}

func TestCachedProviderStore(t *testing.T) {
	inner := NewMemoryProvider()
	ctx := context.Background()

	cached := NewCachedProvider(inner, 1*time.Hour)

	err := cached.Store(ctx, &Credential{
		Type:   CredGitToken,
		Value:  "stored-token",
		Source: "test",
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	// Should be in cache.
	cred, err := cached.Get(ctx, CredGitToken)
	if err != nil {
		t.Fatalf("Get() after Store() error = %v", err)
	}
	if cred.Value != "stored-token" {
		t.Errorf("Get() = %q, want %q", cred.Value, "stored-token")
	}
}

func TestCachedProviderDelete(t *testing.T) {
	inner := NewMemoryProvider()
	ctx := context.Background()

	_ = inner.Store(ctx, &Credential{
		Type:  CredGitToken,
		Value: "to-delete",
	})

	cached := NewCachedProvider(inner, 1*time.Hour)

	// Populate cache.
	_, _ = cached.Get(ctx, CredGitToken)

	// Delete.
	if err := cached.Delete(ctx, CredGitToken); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Should be gone from both cache and inner.
	_, err := cached.Get(ctx, CredGitToken)
	if err != ErrNotFound {
		t.Errorf("Get() after Delete() = %v, want ErrNotFound", err)
	}
}

func TestCachedProviderName(t *testing.T) {
	inner := NewMemoryProvider()
	cached := NewCachedProvider(inner, 1*time.Hour)

	if cached.Name() != "cached:memory" {
		t.Errorf("Name() = %q, want %q", cached.Name(), "cached:memory")
	}
}

func TestCachedProviderList(t *testing.T) {
	inner := NewMemoryProvider()
	ctx := context.Background()

	_ = inner.Store(ctx, &Credential{Type: CredGitToken, Value: "t"})
	_ = inner.Store(ctx, &Credential{Type: CredLLMAPIKey, Value: "k"})

	cached := NewCachedProvider(inner, 1*time.Hour)

	types, err := cached.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(types) != 2 {
		t.Errorf("List() returned %d types, want 2", len(types))
	}
}

// failingProvider wraps a Provider and starts returning errors after N Get calls.
type failingProvider struct {
	Provider
	failAfter int
	getCalls  int
}

func (f *failingProvider) Get(ctx context.Context, credType CredentialType) (*Credential, error) {
	f.getCalls++
	if f.getCalls > f.failAfter {
		return nil, fmt.Errorf("simulated inner provider failure")
	}
	return f.Provider.Get(ctx, credType)
}
