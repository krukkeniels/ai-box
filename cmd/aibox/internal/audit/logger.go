package audit

import "context"

// EventLogger is the interface that components use to emit structured audit events.
// All AI-Box components (CLI, agent, LLM proxy, etc.) depend on this interface
// rather than a concrete implementation, enabling testability and pluggable backends.
type EventLogger interface {
	// Log records a single audit event. Implementations must be safe for
	// concurrent use. The event's HashPrev field is set by the implementation
	// to maintain the hash chain.
	Log(ctx context.Context, event AuditEvent) error

	// Flush forces any buffered events to be written to the underlying store.
	Flush(ctx context.Context) error

	// Close flushes remaining events and releases resources.
	Close() error
}

// EventStore is the interface for reading back audit events. Separated from
// EventLogger to enforce write/read path separation (spec Section 19.2).
type EventStore interface {
	// Query returns events matching the given filter, ordered by timestamp.
	Query(ctx context.Context, filter EventFilter) ([]AuditEvent, error)

	// Verify walks the hash chain from the given start index and reports
	// the first broken link, or nil if the chain is intact.
	Verify(ctx context.Context, startIndex, count int) (*ChainVerification, error)
}

// EventFilter specifies criteria for querying stored audit events.
type EventFilter struct {
	Since     *int64    // Unix timestamp (inclusive)
	Until     *int64    // Unix timestamp (exclusive)
	EventType EventType // empty matches all
	SandboxID string    // empty matches all
	UserID    string    // empty matches all
	Source    Source    // empty matches all
	Severity  Severity  // empty matches all
	Limit     int       // 0 means no limit
	Offset    int       // skip first N results
}

// ChainVerification is the result of a hash chain integrity check.
type ChainVerification struct {
	Verified    int  // number of events verified
	BrokenAt    int  // index of the first broken link (-1 if intact)
	IsIntact    bool // true if the entire range is valid
	ExpectedHash string // what the hash should have been at the broken link
	ActualHash   string // what the hash actually was at the broken link
}
