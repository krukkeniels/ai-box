package credentials

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

const (
	backendLibsecret = "libsecret"
	backendFile      = "file"
)

// KeychainProvider stores credentials in the OS keychain.
// On Linux it uses secret-tool (libsecret / GNOME Keyring).
// When secret-tool is unavailable it falls back to an encrypted file store.
type KeychainProvider struct {
	backend  string
	fallback *FileProvider // used when no native keychain is available
}

// NewKeychainProvider detects the best available keychain backend and returns
// a provider. It never returns an error—if no native keychain is found it
// falls back to the encrypted file store.
func NewKeychainProvider() (*KeychainProvider, error) {
	// Check for secret-tool (libsecret / GNOME Keyring).
	if _, err := exec.LookPath("secret-tool"); err == nil {
		slog.Debug("keychain backend detected", "backend", backendLibsecret)
		return &KeychainProvider{backend: backendLibsecret}, nil
	}

	// Fallback: encrypted file.
	slog.Debug("no native keychain found, falling back to encrypted file store")
	fp, err := NewFileProvider("")
	if err != nil {
		return nil, fmt.Errorf("initializing file fallback: %w", err)
	}
	return &KeychainProvider{backend: backendFile, fallback: fp}, nil
}

func (k *KeychainProvider) Get(ctx context.Context, credType CredentialType) (*Credential, error) {
	if k.backend == backendFile {
		return k.fallback.Get(ctx, credType)
	}

	out, err := k.secretToolLookup(ctx, credType)
	if err != nil {
		return nil, ErrNotFound
	}

	return &Credential{
		Type:   credType,
		Value:  out,
		Source: k.backend,
	}, nil
}

func (k *KeychainProvider) Store(ctx context.Context, cred *Credential) error {
	if k.backend == backendFile {
		return k.fallback.Store(ctx, cred)
	}

	return k.secretToolStore(ctx, cred.Type, cred.Value)
}

func (k *KeychainProvider) Delete(ctx context.Context, credType CredentialType) error {
	if k.backend == backendFile {
		return k.fallback.Delete(ctx, credType)
	}

	return k.secretToolClear(ctx, credType)
}

func (k *KeychainProvider) List(ctx context.Context) ([]CredentialType, error) {
	if k.backend == backendFile {
		return k.fallback.List(ctx)
	}

	// secret-tool does not have a native list command, so we probe each known type.
	var found []CredentialType
	for _, ct := range AllCredentialTypes {
		if _, err := k.secretToolLookup(ctx, ct); err == nil {
			found = append(found, ct)
		}
	}
	return found, nil
}

func (k *KeychainProvider) Name() string {
	return k.backend
}

// secretToolLookup runs: secret-tool lookup service aibox type <credType>
func (k *KeychainProvider) secretToolLookup(ctx context.Context, credType CredentialType) (string, error) {
	cmd := exec.CommandContext(ctx, "secret-tool", "lookup", "service", "aibox", "type", string(credType))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("secret-tool lookup: %w: %s", err, stderr.String())
	}

	value := strings.TrimRight(stdout.String(), "\n")
	if value == "" {
		return "", ErrNotFound
	}

	return value, nil
}

// secretToolStore runs: echo -n <value> | secret-tool store --label="AI-Box <type>" service aibox type <credType>
func (k *KeychainProvider) secretToolStore(ctx context.Context, credType CredentialType, value string) error {
	label := fmt.Sprintf("AI-Box %s", credType)
	cmd := exec.CommandContext(ctx, "secret-tool", "store", "--label="+label, "service", "aibox", "type", string(credType))
	cmd.Stdin = strings.NewReader(value)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("secret-tool store: %w: %s", err, stderr.String())
	}
	return nil
}

// secretToolClear runs: secret-tool clear service aibox type <credType>
func (k *KeychainProvider) secretToolClear(ctx context.Context, credType CredentialType) error {
	cmd := exec.CommandContext(ctx, "secret-tool", "clear", "service", "aibox", "type", string(credType))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("secret-tool clear: %w: %s", err, stderr.String())
	}
	return nil
}
