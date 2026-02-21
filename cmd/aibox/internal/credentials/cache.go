package credentials

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// CachedProvider wraps another Provider with TTL-based in-memory caching.
type CachedProvider struct {
	inner  Provider
	cache  *MemoryProvider
	ttl    time.Duration
	logger *slog.Logger
	mu     sync.RWMutex
}

// NewCachedProvider wraps inner with a TTL-based memory cache.
func NewCachedProvider(inner Provider, ttl time.Duration) *CachedProvider {
	return &CachedProvider{
		inner:  inner,
		cache:  NewMemoryProvider(),
		ttl:    ttl,
		logger: slog.Default(),
	}
}

// Get checks cache first, falls back to inner provider.
func (c *CachedProvider) Get(ctx context.Context, credType CredentialType) (*Credential, error) {
	c.mu.RLock()
	cred, err := c.cache.Get(ctx, credType)
	c.mu.RUnlock()

	if err == nil && !cred.IsExpired() {
		return cred, nil
	}

	// Cache miss or expired — fetch from inner provider.
	cred, err = c.inner.Get(ctx, credType)
	if err != nil {
		// On inner failure, return stale cached value if available.
		c.mu.RLock()
		stale, cacheErr := c.cache.Get(ctx, credType)
		c.mu.RUnlock()
		if cacheErr == nil {
			c.logger.Warn("returning stale cached credential",
				"type", string(credType),
				"inner_error", err,
			)
			return stale, nil
		}
		return nil, err
	}

	// Cache the result with TTL.
	cached := *cred
	cached.ExpiresAt = time.Now().Add(c.ttl)

	c.mu.Lock()
	_ = c.cache.Store(ctx, &cached)
	c.mu.Unlock()

	return cred, nil
}

// Store delegates to the inner provider and invalidates cache.
func (c *CachedProvider) Store(ctx context.Context, cred *Credential) error {
	if err := c.inner.Store(ctx, cred); err != nil {
		return err
	}
	// Update cache.
	cached := *cred
	cached.ExpiresAt = time.Now().Add(c.ttl)
	c.mu.Lock()
	_ = c.cache.Store(ctx, &cached)
	c.mu.Unlock()
	return nil
}

// Delete delegates to the inner provider and removes from cache.
func (c *CachedProvider) Delete(ctx context.Context, credType CredentialType) error {
	if err := c.inner.Delete(ctx, credType); err != nil {
		return err
	}
	c.mu.Lock()
	_ = c.cache.Delete(ctx, credType)
	c.mu.Unlock()
	return nil
}

// List delegates to the inner provider.
func (c *CachedProvider) List(ctx context.Context) ([]CredentialType, error) {
	return c.inner.List(ctx)
}

// Name returns the inner provider name with a "cached:" prefix.
func (c *CachedProvider) Name() string {
	return "cached:" + c.inner.Name()
}
