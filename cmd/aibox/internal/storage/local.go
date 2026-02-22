package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// LocalConfig holds configuration for the local append-only storage backend.
type LocalConfig struct {
	BaseDir string // directory for storing batch files (default: /var/lib/aibox/audit)
}

// DefaultLocalConfig returns a LocalConfig with sensible defaults.
func DefaultLocalConfig() LocalConfig {
	return LocalConfig{
		BaseDir: "/var/lib/aibox/audit",
	}
}

// LocalBackend implements Backend using the local filesystem with append-only
// semantics. Each batch is stored as a separate JSON file. Once created, batch
// files are set read-only (0444) to prevent accidental modification.
type LocalBackend struct {
	cfg LocalConfig
	mu  sync.RWMutex
}

// NewLocalBackend creates a new local filesystem storage backend.
func NewLocalBackend(cfg LocalConfig) (*LocalBackend, error) {
	if cfg.BaseDir == "" {
		cfg.BaseDir = DefaultLocalConfig().BaseDir
	}

	if err := os.MkdirAll(cfg.BaseDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating storage directory %s: %w", cfg.BaseDir, err)
	}

	slog.Debug("local audit storage initialized", "dir", cfg.BaseDir)
	return &LocalBackend{cfg: cfg}, nil
}

// Name returns the backend name.
func (b *LocalBackend) Name() string { return "local" }

// Append stores a batch of entries as a read-only file.
func (b *LocalBackend) Append(_ context.Context, batch Batch) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if batch.Key == "" {
		batch.Key = generateBatchKey(batch.CreatedAt)
	}
	if batch.CreatedAt.IsZero() {
		batch.CreatedAt = time.Now().UTC()
	}
	batch.Checksum = computeBatchChecksum(batch.Entries)

	data, err := json.Marshal(batch)
	if err != nil {
		return "", fmt.Errorf("marshaling batch: %w", err)
	}

	path := b.batchPath(batch.Key)

	// Check for immutable violation.
	if _, err := os.Stat(path); err == nil {
		return "", ErrImmutableViolation
	}

	if err := os.WriteFile(path, data, 0o444); err != nil {
		return "", fmt.Errorf("writing batch %s: %w", batch.Key, err)
	}

	slog.Debug("batch stored", "key", batch.Key, "entries", len(batch.Entries), "path", path)
	return batch.Key, nil
}

// Read retrieves a batch by key and verifies its checksum.
func (b *LocalBackend) Read(_ context.Context, key string) (*Batch, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	path := b.batchPath(key)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrBatchNotFound
		}
		return nil, fmt.Errorf("reading batch %s: %w", key, err)
	}

	var batch Batch
	if err := json.Unmarshal(data, &batch); err != nil {
		return nil, fmt.Errorf("parsing batch %s: %w", key, err)
	}

	// Verify checksum.
	expected := computeBatchChecksum(batch.Entries)
	if batch.Checksum != expected {
		return nil, fmt.Errorf("%w: key=%s expected=%s got=%s", ErrBatchCorrupted, key, expected, batch.Checksum)
	}

	return &batch, nil
}

// List returns batch keys in chronological order, filtered by time range.
func (b *LocalBackend) List(_ context.Context, since, until time.Time) ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	entries, err := os.ReadDir(b.cfg.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("listing storage directory: %w", err)
	}

	var keys []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		key := strings.TrimSuffix(entry.Name(), ".json")

		// Filter by time if bounds are set.
		if !since.IsZero() || !until.IsZero() {
			batch, err := b.readBatchUnsafe(key)
			if err != nil {
				continue
			}
			if !since.IsZero() && batch.CreatedAt.Before(since) {
				continue
			}
			if !until.IsZero() && batch.CreatedAt.After(until) {
				continue
			}
		}

		keys = append(keys, key)
	}

	sort.Strings(keys)
	return keys, nil
}

// batchPath returns the filesystem path for a batch key.
func (b *LocalBackend) batchPath(key string) string {
	return filepath.Join(b.cfg.BaseDir, key+".json")
}

// readBatchUnsafe reads a batch without holding the lock. Caller must hold lock.
func (b *LocalBackend) readBatchUnsafe(key string) (*Batch, error) {
	path := b.batchPath(key)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var batch Batch
	if err := json.Unmarshal(data, &batch); err != nil {
		return nil, err
	}
	return &batch, nil
}

// generateBatchKey creates a unique key based on timestamp.
func generateBatchKey(t time.Time) string {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return t.UTC().Format("20060102T150405Z")
}

// computeBatchChecksum computes SHA-256 of all entries concatenated.
func computeBatchChecksum(entries [][]byte) string {
	h := sha256.New()
	for _, entry := range entries {
		h.Write(entry)
	}
	return hex.EncodeToString(h.Sum(nil))
}
