package credentials

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// LeaseInfo tracks a single minted credential and its lease metadata.
type LeaseInfo struct {
	CredType  CredentialType
	LeaseID   string
	ExpiresAt time.Time
	MintedAt  time.Time
}

// LifecycleManager handles credential minting at start and revocation at stop.
type LifecycleManager struct {
	provider  Provider
	logger    *slog.Logger
	sandboxID string
	user      string
	leases    []LeaseInfo
	mu        sync.Mutex
}

// NewLifecycleManager creates a LifecycleManager for the given sandbox session.
func NewLifecycleManager(provider Provider, sandboxID, user string) *LifecycleManager {
	return &LifecycleManager{
		provider:  provider,
		logger:    slog.Default(),
		sandboxID: sandboxID,
		user:      user,
	}
}

// MintAll requests all required credentials from the provider.
// Called during `aibox start`.
// Returns env vars to inject into the container (e.g. "AIBOX_GIT_TOKEN=xxx").
func (m *LifecycleManager) MintAll(ctx context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var envVars []string
	var minted []LeaseInfo

	for _, ct := range AllCredentialTypes {
		cred, err := m.provider.Get(ctx, ct)
		if err != nil {
			m.logger.Warn("credential not available, skipping",
				"type", string(ct),
				"sandbox_id", m.sandboxID,
				"error", err,
			)
			continue
		}

		if cred.IsExpired() {
			m.logger.Warn("credential expired, skipping",
				"type", string(ct),
				"sandbox_id", m.sandboxID,
			)
			continue
		}

		envName := EnvVarName(ct)
		if envName == "" {
			return nil, fmt.Errorf("no env var mapping for credential type %q", ct)
		}

		envVars = append(envVars, fmt.Sprintf("%s=%s", envName, cred.Value))
		minted = append(minted, LeaseInfo{
			CredType:  ct,
			LeaseID:   fmt.Sprintf("%s/%s/%s", m.sandboxID, m.user, ct),
			ExpiresAt: cred.ExpiresAt,
			MintedAt:  time.Now(),
		})

		m.logger.Info("credential minted",
			"type", string(ct),
			"sandbox_id", m.sandboxID,
			"expires_at", cred.ExpiresAt,
		)
	}

	m.leases = minted
	return envVars, nil
}

// RevokeAll revokes all active credential leases.
// Called during `aibox stop`. Must complete within 5 seconds.
func (m *LifecycleManager) RevokeAll(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	m.mu.Lock()
	leases := make([]LeaseInfo, len(m.leases))
	copy(leases, m.leases)
	m.leases = nil
	m.mu.Unlock()

	var lastErr error
	for _, lease := range leases {
		if err := m.provider.Delete(ctx, lease.CredType); err != nil {
			m.logger.Warn("failed to revoke credential",
				"type", string(lease.CredType),
				"lease_id", lease.LeaseID,
				"error", err,
			)
			lastErr = err
			continue
		}

		m.logger.Info("credential revoked",
			"type", string(lease.CredType),
			"sandbox_id", m.sandboxID,
		)
	}

	return lastErr
}

// Status returns the current state of all managed credentials.
func (m *LifecycleManager) Status(_ context.Context) []LeaseInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]LeaseInfo, len(m.leases))
	copy(result, m.leases)
	return result
}

// RefreshExpiring checks for credentials expiring soon and refreshes them.
// Called periodically while sandbox is running.
func (m *LifecycleManager) RefreshExpiring(ctx context.Context, threshold time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	var lastErr error

	for i, lease := range m.leases {
		if lease.ExpiresAt.IsZero() {
			continue
		}
		if time.Until(lease.ExpiresAt) > threshold {
			continue
		}

		m.logger.Info("refreshing expiring credential",
			"type", string(lease.CredType),
			"expires_in", time.Until(lease.ExpiresAt),
			"sandbox_id", m.sandboxID,
		)

		cred, err := m.provider.Get(ctx, lease.CredType)
		if err != nil {
			m.logger.Warn("failed to refresh credential",
				"type", string(lease.CredType),
				"error", err,
			)
			lastErr = err
			continue
		}

		m.leases[i] = LeaseInfo{
			CredType:  lease.CredType,
			LeaseID:   fmt.Sprintf("%s/%s/%s", m.sandboxID, m.user, lease.CredType),
			ExpiresAt: cred.ExpiresAt,
			MintedAt:  now,
		}

		m.logger.Info("credential refreshed",
			"type", string(lease.CredType),
			"new_expiry", cred.ExpiresAt,
		)
	}

	return lastErr
}
