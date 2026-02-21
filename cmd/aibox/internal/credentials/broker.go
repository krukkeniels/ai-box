package credentials

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// CredentialStatus describes the state of a single credential.
type CredentialStatus struct {
	Type      CredentialType
	Present   bool
	Expired   bool
	ExpiresIn time.Duration
	Source    string
}

// Broker manages credential lifecycle for sandbox containers.
type Broker struct {
	provider Provider
	logger   *slog.Logger
}

// NewBroker creates a broker backed by the given provider.
func NewBroker(provider Provider) *Broker {
	return &Broker{
		provider: provider,
		logger:   slog.Default(),
	}
}

// InjectEnvVars returns environment variables suitable for passing to a
// container. Each variable is formatted as KEY=VALUE. Only credentials that
// are present and not expired are included; missing or expired credentials
// are logged as warnings and skipped.
func (b *Broker) InjectEnvVars(ctx context.Context) ([]string, error) {
	var envVars []string

	for _, ct := range AllCredentialTypes {
		cred, err := b.provider.Get(ctx, ct)
		if err != nil {
			b.logger.Warn("credential not configured, skipping",
				"type", string(ct),
				"provider", b.provider.Name(),
			)
			continue
		}

		if cred.IsExpired() {
			b.logger.Warn("credential expired, skipping",
				"type", string(ct),
				"expired_at", cred.ExpiresAt,
			)
			continue
		}

		envName := EnvVarName(ct)
		if envName == "" {
			return nil, fmt.Errorf("no env var mapping for credential type %q", ct)
		}
		envVars = append(envVars, fmt.Sprintf("%s=%s", envName, cred.Value))
	}

	return envVars, nil
}

// ValidateCredentials checks the status of all known credential types
// against the provider.
func (b *Broker) ValidateCredentials(ctx context.Context) []CredentialStatus {
	var statuses []CredentialStatus

	for _, ct := range AllCredentialTypes {
		status := CredentialStatus{Type: ct}

		cred, err := b.provider.Get(ctx, ct)
		if err != nil {
			statuses = append(statuses, status)
			continue
		}

		status.Present = true
		status.Source = cred.Source
		if !cred.ExpiresAt.IsZero() {
			remaining := time.Until(cred.ExpiresAt)
			if remaining <= 0 {
				status.Expired = true
			}
			status.ExpiresIn = remaining
		}

		statuses = append(statuses, status)
	}

	return statuses
}

// Provider returns the underlying credential provider.
func (b *Broker) Provider() Provider {
	return b.provider
}
