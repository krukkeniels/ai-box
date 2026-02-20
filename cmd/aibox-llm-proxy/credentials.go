package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// CredentialManager reads and caches the LLM API key from a file on disk.
// It supports hot-reloading via SIGHUP.
type CredentialManager struct {
	secretPath string
	apiKey     string
	mu         sync.RWMutex
}

// NewCredentialManager creates a CredentialManager and reads the initial key.
// Returns an error if the secret file does not exist or is empty.
func NewCredentialManager(secretPath string) (*CredentialManager, error) {
	cm := &CredentialManager{secretPath: secretPath}
	if err := cm.Reload(); err != nil {
		return nil, fmt.Errorf("loading initial credential: %w", err)
	}
	return cm, nil
}

// GetAPIKey returns the current API key (thread-safe).
func (cm *CredentialManager) GetAPIKey() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.apiKey
}

// Reload re-reads the API key from the secret file.
func (cm *CredentialManager) Reload() error {
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
	cm.mu.Unlock()
	return nil
}
