package credentials

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aibox/aibox/internal/config"
)

// fileEntry is the on-disk representation of a stored credential.
type fileEntry struct {
	Value     string    `json:"value"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// FileProvider stores credentials in an AES-256-GCM encrypted JSON file.
type FileProvider struct {
	path string
	key  [32]byte
	mu   sync.Mutex
}

// NewFileProvider creates a provider backed by an encrypted file.
// If path is empty, ~/.config/aibox/credentials.enc is used.
func NewFileProvider(path string) (*FileProvider, error) {
	if path == "" {
		home, err := config.ResolveHomeDir()
		if err != nil {
			return nil, fmt.Errorf("determining home directory: %w", err)
		}
		path = filepath.Join(home, ".config", "aibox", "credentials.enc")
	}

	key, err := deriveKey()
	if err != nil {
		return nil, fmt.Errorf("deriving encryption key: %w", err)
	}

	return &FileProvider{
		path: path,
		key:  key,
	}, nil
}

// NewFileProviderWithKey creates a FileProvider with a caller-supplied key.
// This is primarily for testing.
func NewFileProviderWithKey(path string, key [32]byte) *FileProvider {
	return &FileProvider{
		path: path,
		key:  key,
	}
}

func (f *FileProvider) Get(_ context.Context, credType CredentialType) (*Credential, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	store, err := f.load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	entry, ok := store[string(credType)]
	if !ok {
		return nil, ErrNotFound
	}

	return &Credential{
		Type:      credType,
		Value:     entry.Value,
		ExpiresAt: entry.ExpiresAt,
		Source:    "file",
	}, nil
}

func (f *FileProvider) Store(_ context.Context, cred *Credential) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	store, err := f.load()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if store == nil {
		store = make(map[string]fileEntry)
	}

	store[string(cred.Type)] = fileEntry{
		Value:     cred.Value,
		ExpiresAt: cred.ExpiresAt,
	}

	return f.save(store)
}

func (f *FileProvider) Delete(_ context.Context, credType CredentialType) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	store, err := f.load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}

	if _, ok := store[string(credType)]; !ok {
		return ErrNotFound
	}

	delete(store, string(credType))
	return f.save(store)
}

func (f *FileProvider) List(_ context.Context) ([]CredentialType, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	store, err := f.load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	types := make([]CredentialType, 0, len(store))
	for k := range store {
		types = append(types, CredentialType(k))
	}
	return types, nil
}

func (f *FileProvider) Name() string {
	return "file"
}

// load reads and decrypts the credential store from disk.
func (f *FileProvider) load() (map[string]fileEntry, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, err
	}

	plaintext, err := f.decrypt(data)
	if err != nil {
		return nil, fmt.Errorf("decrypting credential store: %w", err)
	}

	var store map[string]fileEntry
	if err := json.Unmarshal(plaintext, &store); err != nil {
		return nil, fmt.Errorf("parsing credential store: %w", err)
	}

	return store, nil
}

// save encrypts and writes the credential store to disk.
func (f *FileProvider) save(store map[string]fileEntry) error {
	plaintext, err := json.Marshal(store)
	if err != nil {
		return fmt.Errorf("marshalling credential store: %w", err)
	}

	ciphertext, err := f.encrypt(plaintext)
	if err != nil {
		return fmt.Errorf("encrypting credential store: %w", err)
	}

	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating credential directory: %w", err)
	}

	return os.WriteFile(f.path, ciphertext, 0o600)
}

func (f *FileProvider) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(f.key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (f *FileProvider) decrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(f.key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// deriveKey produces a 256-bit key from the machine ID and current user.
// This is not meant to be a strong secret—it ties the credential file to this
// specific machine and user so it cannot simply be copied elsewhere.
func deriveKey() ([32]byte, error) {
	machineID, err := readMachineID()
	if err != nil {
		return [32]byte{}, fmt.Errorf("reading machine id: %w", err)
	}

	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("LOGNAME")
	}
	if username == "" {
		username = "aibox-user"
	}

	material := machineID + ":" + username + ":aibox-credentials"
	return sha256.Sum256([]byte(material)), nil
}

// readMachineID reads /etc/machine-id (systemd) or falls back to hostname.
func readMachineID() (string, error) {
	data, err := os.ReadFile("/etc/machine-id")
	if err == nil {
		id := string(data)
		if len(id) > 0 {
			return id, nil
		}
	}

	// Fallback to hostname.
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}
	return hostname, nil
}
