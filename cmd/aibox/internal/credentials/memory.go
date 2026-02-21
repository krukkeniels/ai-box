package credentials

import (
	"context"
	"sync"
)

// MemoryProvider stores credentials in memory. It is safe for concurrent use.
// Intended for testing and as a cache layer for other providers.
type MemoryProvider struct {
	creds map[CredentialType]*Credential
	mu    sync.RWMutex
}

// NewMemoryProvider returns an empty in-memory credential provider.
func NewMemoryProvider() *MemoryProvider {
	return &MemoryProvider{
		creds: make(map[CredentialType]*Credential),
	}
}

func (m *MemoryProvider) Get(_ context.Context, credType CredentialType) (*Credential, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cred, ok := m.creds[credType]
	if !ok {
		return nil, ErrNotFound
	}
	// Return a copy to prevent mutation.
	cp := *cred
	return &cp, nil
}

func (m *MemoryProvider) Store(_ context.Context, cred *Credential) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cp := *cred
	m.creds[cred.Type] = &cp
	return nil
}

func (m *MemoryProvider) Delete(_ context.Context, credType CredentialType) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.creds[credType]; !ok {
		return ErrNotFound
	}
	delete(m.creds, credType)
	return nil
}

func (m *MemoryProvider) List(_ context.Context) ([]CredentialType, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	types := make([]CredentialType, 0, len(m.creds))
	for ct := range m.creds {
		types = append(types, ct)
	}
	return types, nil
}

func (m *MemoryProvider) Name() string {
	return "memory"
}
