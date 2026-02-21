package credentials

import (
	"context"
	"errors"
	"time"
)

// CredentialType identifies a credential category.
type CredentialType string

const (
	CredGitToken    CredentialType = "git-token"
	CredLLMAPIKey   CredentialType = "llm-api-key"
	CredMirrorToken CredentialType = "mirror-token"
)

// AllCredentialTypes lists every known credential type.
var AllCredentialTypes = []CredentialType{
	CredGitToken,
	CredLLMAPIKey,
	CredMirrorToken,
}

// envVarName maps credential types to container environment variable names.
var envVarName = map[CredentialType]string{
	CredGitToken:    "AIBOX_GIT_TOKEN",
	CredLLMAPIKey:   "AIBOX_LLM_API_KEY",
	CredMirrorToken: "AIBOX_MIRROR_TOKEN",
}

// EnvVarName returns the environment variable name for a credential type.
func EnvVarName(ct CredentialType) string {
	return envVarName[ct]
}

// Credential holds a single credential with metadata.
type Credential struct {
	Type      CredentialType
	Value     string
	ExpiresAt time.Time // zero value means no expiry
	Source    string    // "vault", "keychain", "env", "file"
}

// IsExpired reports whether the credential has passed its expiry time.
func (c *Credential) IsExpired() bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(c.ExpiresAt)
}

// Provider abstracts credential storage and retrieval.
type Provider interface {
	// Get retrieves a credential by type. Returns ErrNotFound if not stored.
	Get(ctx context.Context, credType CredentialType) (*Credential, error)

	// Store saves a credential.
	Store(ctx context.Context, cred *Credential) error

	// Delete removes a credential by type.
	Delete(ctx context.Context, credType CredentialType) error

	// List returns all stored credential types (not values).
	List(ctx context.Context) ([]CredentialType, error)

	// Name returns the provider name for display.
	Name() string
}

// ErrNotFound is returned when a credential is not stored.
var ErrNotFound = errors.New("credential not found")

// ValidCredentialType checks whether the given string is a known credential type.
func ValidCredentialType(s string) bool {
	for _, ct := range AllCredentialTypes {
		if string(ct) == s {
			return true
		}
	}
	return false
}
