package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// VaultConfig holds Vault connection settings.
type VaultConfig struct {
	Address      string        // e.g., "https://vault.internal:8200"
	AuthMethod   string        // "spiffe" or "token" (for testing)
	Token        string        // static token (for testing/fallback only)
	MountPath    string        // Vault mount path, e.g., "aibox"
	SPIFFEConfig *SPIFFEConfig // SPIFFE config for JWT auth
	CacheTTL     time.Duration // How long to cache credentials in memory
	RetryBackoff time.Duration // Backoff between retries when Vault unreachable
	MaxRetries   int
}

// VaultProvider implements Provider using HashiCorp Vault.
type VaultProvider struct {
	config  VaultConfig
	cache   *MemoryProvider
	spiffe  *SPIFFEClient
	logger  *slog.Logger
	mu      sync.RWMutex
	leases  map[CredentialType]string // track Vault lease IDs for revocation
	healthy bool
	lastErr error
	client  *http.Client
	token   string // current Vault auth token
}

// NewVaultProvider creates a VaultProvider with the given configuration.
func NewVaultProvider(cfg VaultConfig) (*VaultProvider, error) {
	if cfg.Address == "" {
		return nil, fmt.Errorf("vault address is required")
	}
	if cfg.MountPath == "" {
		cfg.MountPath = "aibox"
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 5 * time.Minute
	}
	if cfg.RetryBackoff <= 0 {
		cfg.RetryBackoff = 2 * time.Second
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}

	vp := &VaultProvider{
		config:  cfg,
		cache:   NewMemoryProvider(),
		logger:  slog.Default(),
		leases:  make(map[CredentialType]string),
		healthy: false,
		client:  &http.Client{Timeout: 10 * time.Second},
	}

	if cfg.AuthMethod == "spiffe" && cfg.SPIFFEConfig != nil {
		vp.spiffe = NewSPIFFEClient(*cfg.SPIFFEConfig)
	}

	if cfg.AuthMethod == "token" && cfg.Token != "" {
		vp.token = cfg.Token
		vp.healthy = true
	}

	return vp, nil
}

// Get retrieves a credential — first checks cache, then fetches from Vault.
func (v *VaultProvider) Get(ctx context.Context, credType CredentialType) (*Credential, error) {
	// Check cache first.
	cred, err := v.cache.Get(ctx, credType)
	if err == nil && !cred.IsExpired() {
		return cred, nil
	}

	// Fetch from Vault.
	cred, err = v.fetchFromVault(ctx, credType)
	if err != nil {
		v.logger.Warn("failed to fetch from vault, returning cached if available",
			"type", string(credType),
			"error", err,
		)
		// Graceful degradation: return stale cache on Vault failure.
		if cached, cacheErr := v.cache.Get(ctx, credType); cacheErr == nil {
			return cached, nil
		}
		return nil, fmt.Errorf("fetching credential %s from vault: %w", credType, err)
	}

	// Store in cache with TTL.
	cred.ExpiresAt = time.Now().Add(v.config.CacheTTL)
	cred.Source = "vault"
	if storeErr := v.cache.Store(ctx, cred); storeErr != nil {
		v.logger.Warn("failed to cache credential", "type", string(credType), "error", storeErr)
	}

	return cred, nil
}

// Store is a no-op for Vault (credentials are managed by Vault, not stored by client).
func (v *VaultProvider) Store(_ context.Context, _ *Credential) error {
	return fmt.Errorf("vault provider is read-only: credentials are managed in Vault")
}

// Delete revokes a specific credential lease in Vault.
func (v *VaultProvider) Delete(ctx context.Context, credType CredentialType) error {
	v.mu.Lock()
	leaseID, ok := v.leases[credType]
	if ok {
		delete(v.leases, credType)
	}
	v.mu.Unlock()

	// Remove from cache.
	_ = v.cache.Delete(ctx, credType)

	if !ok {
		return nil
	}

	return v.revokeLease(ctx, leaseID)
}

// List returns credential types available in Vault.
func (v *VaultProvider) List(_ context.Context) ([]CredentialType, error) {
	// All known credential types are available via Vault.
	return AllCredentialTypes, nil
}

// Name returns "vault".
func (v *VaultProvider) Name() string {
	return "vault"
}

// RevokeAll revokes all active leases. Called during aibox stop.
func (v *VaultProvider) RevokeAll(ctx context.Context) error {
	v.mu.Lock()
	leases := make(map[CredentialType]string, len(v.leases))
	for k, lid := range v.leases {
		leases[k] = lid
	}
	v.leases = make(map[CredentialType]string)
	v.mu.Unlock()

	var lastErr error
	for ct, leaseID := range leases {
		if err := v.revokeLease(ctx, leaseID); err != nil {
			v.logger.Warn("failed to revoke lease", "type", string(ct), "lease_id", leaseID, "error", err)
			lastErr = err
		}
	}
	return lastErr
}

// IsHealthy returns whether Vault is currently reachable.
func (v *VaultProvider) IsHealthy() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.healthy
}

// authenticate obtains or refreshes a Vault token.
func (v *VaultProvider) authenticate(ctx context.Context) error {
	if v.config.AuthMethod == "token" {
		v.token = v.config.Token
		return nil
	}

	if v.spiffe == nil {
		return fmt.Errorf("SPIFFE client not configured for JWT auth")
	}

	// Get a workload identity JWT from SPIRE.
	identity, err := v.spiffe.GetIdentity(ctx, "aibox", "system")
	if err != nil {
		return fmt.Errorf("obtaining SPIFFE identity: %w", err)
	}

	// Login to Vault with the JWT.
	loginBody := map[string]string{
		"role": "aibox",
		"jwt":  identity.SPIFFEID, // In production, this would be an actual JWT SVID
	}
	bodyBytes, err := json.Marshal(loginBody)
	if err != nil {
		return fmt.Errorf("marshalling login body: %w", err)
	}

	url := fmt.Sprintf("%s/v1/auth/jwt/login", v.config.Address)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("creating auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("vault auth request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vault auth failed (status %d): %s", resp.StatusCode, body)
	}

	var authResp vaultAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return fmt.Errorf("decoding auth response: %w", err)
	}

	v.token = authResp.Auth.ClientToken
	return nil
}

// fetchFromVault reads a credential from Vault's KV v2 secrets engine.
func (v *VaultProvider) fetchFromVault(ctx context.Context, credType CredentialType) (*Credential, error) {
	if v.token == "" {
		if err := v.authenticate(ctx); err != nil {
			v.setHealthy(false, err)
			return nil, fmt.Errorf("authenticating to vault: %w", err)
		}
	}

	url := fmt.Sprintf("%s/v1/%s/data/%s", v.config.Address, v.config.MountPath, string(credType))

	var lastErr error
	for attempt := 0; attempt <= v.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(v.config.RetryBackoff):
			}
		}

		cred, err := v.doVaultRead(ctx, url, credType)
		if err == nil {
			v.setHealthy(true, nil)
			return cred, nil
		}
		lastErr = err
	}

	v.setHealthy(false, lastErr)
	return nil, fmt.Errorf("vault read failed after %d retries: %w", v.config.MaxRetries, lastErr)
}

func (v *VaultProvider) doVaultRead(ctx context.Context, url string, credType CredentialType) (*Credential, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("X-Vault-Token", v.token)

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault read failed (status %d): %s", resp.StatusCode, body)
	}

	var secretResp vaultSecretResponse
	if err := json.NewDecoder(resp.Body).Decode(&secretResp); err != nil {
		return nil, fmt.Errorf("decoding secret response: %w", err)
	}

	value, ok := secretResp.Data.Data["value"]
	if !ok {
		return nil, fmt.Errorf("secret %s has no 'value' key", credType)
	}

	// Track lease if present.
	if secretResp.LeaseID != "" {
		v.mu.Lock()
		v.leases[credType] = secretResp.LeaseID
		v.mu.Unlock()
	}

	return &Credential{
		Type:   credType,
		Value:  value,
		Source: "vault",
	}, nil
}

// revokeLease revokes a single Vault lease.
func (v *VaultProvider) revokeLease(ctx context.Context, leaseID string) error {
	body, err := json.Marshal(map[string]string{"lease_id": leaseID})
	if err != nil {
		return fmt.Errorf("marshalling revoke body: %w", err)
	}

	url := fmt.Sprintf("%s/v1/sys/leases/revoke", v.config.Address)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating revoke request: %w", err)
	}
	req.Header.Set("X-Vault-Token", v.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("vault revoke request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vault revoke failed (status %d): %s", resp.StatusCode, respBody)
	}

	return nil
}

// checkHealth performs a Vault health check.
func (v *VaultProvider) checkHealth(ctx context.Context) bool {
	url := fmt.Sprintf("%s/v1/sys/health", v.config.Address)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Vault returns 200 for initialized+unsealed+active.
	return resp.StatusCode == http.StatusOK
}

func (v *VaultProvider) setHealthy(healthy bool, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.healthy = healthy
	v.lastErr = err
}

// Vault API response types.

type vaultAuthResponse struct {
	Auth struct {
		ClientToken string `json:"client_token"`
		LeaseTTL    int    `json:"lease_duration"`
	} `json:"auth"`
}

type vaultSecretResponse struct {
	LeaseID string `json:"lease_id"`
	Data    struct {
		Data map[string]string `json:"data"`
	} `json:"data"`
}
