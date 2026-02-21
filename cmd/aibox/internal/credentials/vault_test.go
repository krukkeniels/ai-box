package credentials

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newMockVaultServer creates a test HTTP server that simulates the Vault API.
func newMockVaultServer(t *testing.T, secrets map[string]string, healthy bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sys/health":
			if healthy {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]any{"initialized": true, "sealed": false})
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
			}

		case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/jwt/login":
			json.NewEncoder(w).Encode(map[string]any{
				"auth": map[string]any{
					"client_token":   "test-vault-token",
					"lease_duration": 3600,
				},
			})

		case r.Method == http.MethodGet && len(r.URL.Path) > len("/v1/aibox/data/"):
			token := r.Header.Get("X-Vault-Token")
			if token == "" {
				w.WriteHeader(http.StatusForbidden)
				return
			}

			credType := r.URL.Path[len("/v1/aibox/data/"):]
			value, ok := secrets[credType]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]any{"errors": []string{"secret not found"}})
				return
			}

			json.NewEncoder(w).Encode(map[string]any{
				"lease_id": "lease-" + credType,
				"data": map[string]any{
					"data": map[string]string{"value": value},
				},
			})

		case r.Method == http.MethodPut && r.URL.Path == "/v1/sys/leases/revoke":
			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestVaultProviderGet(t *testing.T) {
	secrets := map[string]string{
		"git-token":   "ghp_test123",
		"llm-api-key": "sk-test456",
	}
	srv := newMockVaultServer(t, secrets, true)
	defer srv.Close()

	vp, err := NewVaultProvider(VaultConfig{
		Address:    srv.URL,
		AuthMethod: "token",
		Token:      "test-token",
		MountPath:  "aibox",
		CacheTTL:   5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewVaultProvider() error = %v", err)
	}

	ctx := context.Background()

	// Fetch git-token.
	cred, err := vp.Get(ctx, CredGitToken)
	if err != nil {
		t.Fatalf("Get(git-token) error = %v", err)
	}
	if cred.Value != "ghp_test123" {
		t.Errorf("Get(git-token) value = %q, want %q", cred.Value, "ghp_test123")
	}
	if cred.Source != "vault" {
		t.Errorf("Get(git-token) source = %q, want %q", cred.Source, "vault")
	}

	// Fetch llm-api-key.
	cred, err = vp.Get(ctx, CredLLMAPIKey)
	if err != nil {
		t.Fatalf("Get(llm-api-key) error = %v", err)
	}
	if cred.Value != "sk-test456" {
		t.Errorf("Get(llm-api-key) value = %q, want %q", cred.Value, "sk-test456")
	}
}

func TestVaultProviderCaching(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/aibox/data/git-token" {
			callCount++
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"data": map[string]string{"value": "cached-token"},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	vp, err := NewVaultProvider(VaultConfig{
		Address:    srv.URL,
		AuthMethod: "token",
		Token:      "test-token",
		MountPath:  "aibox",
		CacheTTL:   1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewVaultProvider() error = %v", err)
	}

	ctx := context.Background()

	// First call hits Vault.
	_, err = vp.Get(ctx, CredGitToken)
	if err != nil {
		t.Fatalf("first Get() error = %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 Vault call, got %d", callCount)
	}

	// Second call should use cache.
	_, err = vp.Get(ctx, CredGitToken)
	if err != nil {
		t.Fatalf("second Get() error = %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 Vault call (cached), got %d", callCount)
	}
}

func TestVaultProviderGracefulDegradation(t *testing.T) {
	// Start a server, pre-populate cache, then shut it down.
	secrets := map[string]string{"git-token": "cached-value"}
	srv := newMockVaultServer(t, secrets, true)

	vp, err := NewVaultProvider(VaultConfig{
		Address:      srv.URL,
		AuthMethod:   "token",
		Token:        "test-token",
		MountPath:    "aibox",
		CacheTTL:     1 * time.Hour,
		MaxRetries:   0,
		RetryBackoff: 1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewVaultProvider() error = %v", err)
	}

	ctx := context.Background()

	// Populate cache.
	_, err = vp.Get(ctx, CredGitToken)
	if err != nil {
		t.Fatalf("initial Get() error = %v", err)
	}

	// Shut down the server to simulate Vault being unavailable.
	srv.Close()

	// Expire the cache entry so it tries Vault first.
	vp.cache.mu.Lock()
	if c, ok := vp.cache.creds[CredGitToken]; ok {
		c.ExpiresAt = time.Now().Add(-time.Minute)
	}
	vp.cache.mu.Unlock()

	// Should return stale cached value.
	cred, err := vp.Get(ctx, CredGitToken)
	if err != nil {
		t.Fatalf("Get() after Vault down error = %v, want cached fallback", err)
	}
	if cred.Value != "cached-value" {
		t.Errorf("Get() value = %q, want %q (cached)", cred.Value, "cached-value")
	}
}

func TestVaultProviderLeaseTracking(t *testing.T) {
	secrets := map[string]string{
		"git-token":   "token1",
		"llm-api-key": "key1",
	}
	revokedLeases := make([]string, 0)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && len(r.URL.Path) > len("/v1/aibox/data/"):
			credType := r.URL.Path[len("/v1/aibox/data/"):]
			value, ok := secrets[credType]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"lease_id": "lease-" + credType,
				"data": map[string]any{
					"data": map[string]string{"value": value},
				},
			})

		case r.Method == http.MethodPut && r.URL.Path == "/v1/sys/leases/revoke":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			revokedLeases = append(revokedLeases, body["lease_id"])
			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	vp, err := NewVaultProvider(VaultConfig{
		Address:    srv.URL,
		AuthMethod: "token",
		Token:      "test-token",
		MountPath:  "aibox",
		CacheTTL:   1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewVaultProvider() error = %v", err)
	}

	ctx := context.Background()

	// Fetch both credentials to create leases.
	_, _ = vp.Get(ctx, CredGitToken)
	_, _ = vp.Get(ctx, CredLLMAPIKey)

	// Verify leases are tracked.
	vp.mu.RLock()
	leaseCount := len(vp.leases)
	vp.mu.RUnlock()
	if leaseCount != 2 {
		t.Fatalf("tracked leases = %d, want 2", leaseCount)
	}

	// Revoke all.
	if err := vp.RevokeAll(ctx); err != nil {
		t.Fatalf("RevokeAll() error = %v", err)
	}

	if len(revokedLeases) != 2 {
		t.Errorf("revoked leases = %d, want 2", len(revokedLeases))
	}

	// Leases map should be empty after revocation.
	vp.mu.RLock()
	remaining := len(vp.leases)
	vp.mu.RUnlock()
	if remaining != 0 {
		t.Errorf("remaining leases after RevokeAll = %d, want 0", remaining)
	}
}

func TestVaultProviderStoreIsReadOnly(t *testing.T) {
	vp, err := NewVaultProvider(VaultConfig{
		Address:    "http://localhost:8200",
		AuthMethod: "token",
		Token:      "test",
	})
	if err != nil {
		t.Fatalf("NewVaultProvider() error = %v", err)
	}

	err = vp.Store(context.Background(), &Credential{
		Type:  CredGitToken,
		Value: "test",
	})
	if err == nil {
		t.Error("Store() should return error for vault provider")
	}
}

func TestVaultProviderName(t *testing.T) {
	vp, err := NewVaultProvider(VaultConfig{
		Address:    "http://localhost:8200",
		AuthMethod: "token",
		Token:      "test",
	})
	if err != nil {
		t.Fatalf("NewVaultProvider() error = %v", err)
	}

	if vp.Name() != "vault" {
		t.Errorf("Name() = %q, want %q", vp.Name(), "vault")
	}
}

func TestVaultProviderList(t *testing.T) {
	vp, err := NewVaultProvider(VaultConfig{
		Address:    "http://localhost:8200",
		AuthMethod: "token",
		Token:      "test",
	})
	if err != nil {
		t.Fatalf("NewVaultProvider() error = %v", err)
	}

	types, err := vp.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(types) != len(AllCredentialTypes) {
		t.Errorf("List() returned %d types, want %d", len(types), len(AllCredentialTypes))
	}
}

func TestVaultProviderMissingAddress(t *testing.T) {
	_, err := NewVaultProvider(VaultConfig{})
	if err == nil {
		t.Error("NewVaultProvider() with empty address should return error")
	}
}

func TestVaultProviderHealthCheck(t *testing.T) {
	srv := newMockVaultServer(t, nil, true)
	defer srv.Close()

	vp, err := NewVaultProvider(VaultConfig{
		Address:    srv.URL,
		AuthMethod: "token",
		Token:      "test-token",
	})
	if err != nil {
		t.Fatalf("NewVaultProvider() error = %v", err)
	}

	if !vp.checkHealth(context.Background()) {
		t.Error("checkHealth() = false for healthy mock server, want true")
	}
}

func TestVaultProviderSPIFFEAuth(t *testing.T) {
	secrets := map[string]string{"git-token": "spiffe-fetched-token"}
	srv := newMockVaultServer(t, secrets, true)
	defer srv.Close()

	vp, err := NewVaultProvider(VaultConfig{
		Address:    srv.URL,
		AuthMethod: "spiffe",
		MountPath:  "aibox",
		CacheTTL:   5 * time.Minute,
		SPIFFEConfig: &SPIFFEConfig{
			TrustDomain: "aibox.org.internal",
			AgentSocket: "/nonexistent/agent.sock",
		},
	})
	if err != nil {
		t.Fatalf("NewVaultProvider() error = %v", err)
	}

	ctx := context.Background()
	cred, err := vp.Get(ctx, CredGitToken)
	if err != nil {
		t.Fatalf("Get() with SPIFFE auth error = %v", err)
	}

	if cred.Value != "spiffe-fetched-token" {
		t.Errorf("Get() value = %q, want %q", cred.Value, "spiffe-fetched-token")
	}
}

func TestVaultProviderDelete(t *testing.T) {
	revokeCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/aibox/data/git-token":
			json.NewEncoder(w).Encode(map[string]any{
				"lease_id": "lease-git-token",
				"data": map[string]any{
					"data": map[string]string{"value": "token"},
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/v1/sys/leases/revoke":
			revokeCount++
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	vp, err := NewVaultProvider(VaultConfig{
		Address:    srv.URL,
		AuthMethod: "token",
		Token:      "test",
		MountPath:  "aibox",
		CacheTTL:   1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewVaultProvider() error = %v", err)
	}

	ctx := context.Background()

	// Fetch to create lease.
	_, _ = vp.Get(ctx, CredGitToken)

	// Delete should revoke the lease.
	if err := vp.Delete(ctx, CredGitToken); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if revokeCount != 1 {
		t.Errorf("revoke calls = %d, want 1", revokeCount)
	}

	// Cache should be cleared.
	_, err = vp.cache.Get(ctx, CredGitToken)
	if err != ErrNotFound {
		t.Errorf("cache after Delete() should return ErrNotFound, got %v", err)
	}
}
