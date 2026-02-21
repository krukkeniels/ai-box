package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// CredentialMode selects how the LLM proxy obtains its API key.
type CredentialMode string

const (
	CredentialModeFile  CredentialMode = "file"
	CredentialModeEnv   CredentialMode = "env"
	CredentialModeVault CredentialMode = "vault"
)

// CredentialManager reads and caches the LLM API key.
// It supports multiple credential sources controlled by AIBOX_CREDENTIAL_MODE:
//   - "file" (default): read from AIBOX_LLM_SECRET_PATH file
//   - "env": read from AIBOX_LLM_API_KEY env var
//   - "vault": call Vault HTTP API
type CredentialManager struct {
	mode       CredentialMode
	secretPath string
	apiKey     string
	expiresAt  time.Time
	mu         sync.RWMutex
}

// NewCredentialManager creates a CredentialManager based on the AIBOX_CREDENTIAL_MODE env var.
func NewCredentialManager(secretPath string) (*CredentialManager, error) {
	mode := CredentialMode(os.Getenv("AIBOX_CREDENTIAL_MODE"))
	if mode == "" {
		mode = CredentialModeFile
	}

	cm := &CredentialManager{
		mode:       mode,
		secretPath: secretPath,
	}

	if err := cm.Reload(); err != nil {
		return nil, fmt.Errorf("loading initial credential: %w", err)
	}
	return cm, nil
}

// GetAPIKey returns the current API key (thread-safe).
// If the key has expired, it attempts a transparent refresh.
func (cm *CredentialManager) GetAPIKey() string {
	cm.mu.RLock()
	key := cm.apiKey
	expired := !cm.expiresAt.IsZero() && time.Now().After(cm.expiresAt)
	cm.mu.RUnlock()

	if expired {
		// Attempt transparent refresh; return stale key on failure.
		_ = cm.Reload()
		cm.mu.RLock()
		key = cm.apiKey
		cm.mu.RUnlock()
	}

	return key
}

// Reload re-reads the API key from the configured source.
func (cm *CredentialManager) Reload() error {
	switch cm.mode {
	case CredentialModeFile:
		return cm.loadFromFile()
	case CredentialModeEnv:
		return cm.loadFromEnv()
	case CredentialModeVault:
		return cm.loadFromVault()
	default:
		return fmt.Errorf("unknown credential mode: %q", cm.mode)
	}
}

func (cm *CredentialManager) loadFromFile() error {
	data, err := os.ReadFile(cm.secretPath)
	if err != nil {
		return fmt.Errorf("reading secret file %s: %w", cm.secretPath, err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		return fmt.Errorf("secret file %s is empty", cm.secretPath)
	}
	cm.mu.Lock()
	cm.apiKey = key
	cm.expiresAt = time.Time{} // file-based keys don't expire
	cm.mu.Unlock()
	return nil
}

func (cm *CredentialManager) loadFromEnv() error {
	key := os.Getenv("AIBOX_LLM_API_KEY")
	if key == "" {
		return fmt.Errorf("AIBOX_LLM_API_KEY environment variable is not set")
	}
	cm.mu.Lock()
	cm.apiKey = key
	cm.expiresAt = time.Time{} // env-based keys don't expire
	cm.mu.Unlock()
	return nil
}

func (cm *CredentialManager) loadFromVault() error {
	vaultAddr := os.Getenv("AIBOX_VAULT_ADDR")
	if vaultAddr == "" {
		return fmt.Errorf("AIBOX_VAULT_ADDR is not set for vault credential mode")
	}

	vaultToken := os.Getenv("AIBOX_VAULT_TOKEN")
	if vaultToken == "" {
		// Try reading token from file as fallback.
		tokenPath := os.Getenv("AIBOX_VAULT_TOKEN_PATH")
		if tokenPath != "" {
			data, err := os.ReadFile(tokenPath)
			if err != nil {
				return fmt.Errorf("reading vault token file %s: %w", tokenPath, err)
			}
			vaultToken = strings.TrimSpace(string(data))
		}
		if vaultToken == "" {
			return fmt.Errorf("neither AIBOX_VAULT_TOKEN nor AIBOX_VAULT_TOKEN_PATH is set")
		}
	}

	url := fmt.Sprintf("%s/v1/aibox/data/llm-api-key", vaultAddr)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating vault request: %w", err)
	}
	req.Header.Set("X-Vault-Token", vaultToken)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("vault request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vault returned status %d: %s", resp.StatusCode, body)
	}

	var secret vaultKVResponse
	if err := json.NewDecoder(resp.Body).Decode(&secret); err != nil {
		return fmt.Errorf("decoding vault response: %w", err)
	}

	key, ok := secret.Data.Data["value"]
	if !ok || key == "" {
		return fmt.Errorf("vault secret has no 'value' key")
	}

	cm.mu.Lock()
	cm.apiKey = key
	// Set expiry based on lease duration, defaulting to 30 minutes.
	ttl := 30 * time.Minute
	if secret.LeaseDuration > 0 {
		ttl = time.Duration(secret.LeaseDuration) * time.Second
	}
	cm.expiresAt = time.Now().Add(ttl)
	cm.mu.Unlock()

	return nil
}

// vaultKVResponse represents the Vault KV v2 read response.
type vaultKVResponse struct {
	LeaseDuration int `json:"lease_duration"`
	Data          struct {
		Data map[string]string `json:"data"`
	} `json:"data"`
}
