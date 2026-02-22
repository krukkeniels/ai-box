package storage

import (
	"context"
	"errors"
	"time"
)

// Backend is the interface for immutable audit log storage. Implementations
// must provide append-only semantics -- once a batch is stored, it cannot be
// modified or deleted (per spec Section 19.2).
type Backend interface {
	// Append stores a batch of raw log entries. Each entry is a JSON line.
	// The batch is stored atomically. Returns the batch key.
	Append(ctx context.Context, batch Batch) (string, error)

	// Read retrieves a stored batch by its key.
	Read(ctx context.Context, key string) (*Batch, error)

	// List returns batch keys in chronological order, optionally filtered
	// by time range. Use zero time values for no bound.
	List(ctx context.Context, since, until time.Time) ([]string, error)

	// Name returns the backend name for display (e.g., "local", "minio", "s3").
	Name() string
}

// Batch is a group of log entries stored together.
type Batch struct {
	Key       string    `json:"key"`
	Entries   [][]byte  `json:"entries"`   // raw JSON lines
	CreatedAt time.Time `json:"created_at"`
	ChainHead string    `json:"chain_head"` // hash chain head after last entry
	Checksum  string    `json:"checksum"`   // SHA-256 of all entries concatenated
}

// Verification errors.
var (
	ErrBatchNotFound   = errors.New("storage: batch not found")
	ErrBatchCorrupted  = errors.New("storage: batch checksum mismatch (corrupted)")
	ErrImmutableViolation = errors.New("storage: cannot modify immutable batch")
)
