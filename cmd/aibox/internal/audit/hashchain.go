package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
)

// GenesisHash is the hash_prev value for the first event in a chain.
const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// HashChain maintains a tamper-evident chain of audit events.
// Each event's HashPrev is set to SHA-256(previous_event_json), creating
// a verifiable chain per spec Section 19.2.
type HashChain struct {
	mu       sync.Mutex
	lastHash string
}

// NewHashChain creates a new hash chain starting from the genesis hash.
func NewHashChain() *HashChain {
	return &HashChain{lastHash: GenesisHash}
}

// NewHashChainFrom creates a hash chain that continues from a known last hash.
// Use this when resuming from an existing log.
func NewHashChainFrom(lastHash string) *HashChain {
	if lastHash == "" {
		lastHash = GenesisHash
	}
	return &HashChain{lastHash: lastHash}
}

// Chain sets the event's HashPrev to the current chain head and advances
// the chain. Returns the event with HashPrev populated. This method is
// safe for concurrent use.
func (hc *HashChain) Chain(event *AuditEvent) error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	event.HashPrev = hc.lastHash

	hash, err := HashEvent(event)
	if err != nil {
		return err
	}

	hc.lastHash = hash
	return nil
}

// LastHash returns the current head of the chain.
func (hc *HashChain) LastHash() string {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	return hc.lastHash
}

// HashEvent computes the SHA-256 hash of a serialized audit event.
// The event is serialized as canonical JSON (sorted keys via json.Marshal).
func HashEvent(event *AuditEvent) (string, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", ErrEmptyEvent
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// VerifyChain checks a sequence of events for hash chain integrity.
// The prevHash parameter is the expected HashPrev of the first event
// in the sequence (use GenesisHash for the start of a log).
func VerifyChain(events []AuditEvent, prevHash string) *ChainVerification {
	result := &ChainVerification{
		BrokenAt: -1,
		IsIntact: true,
	}

	if len(events) == 0 {
		return result
	}

	for i := range events {
		if events[i].HashPrev != prevHash {
			result.BrokenAt = i
			result.IsIntact = false
			result.ExpectedHash = prevHash
			result.ActualHash = events[i].HashPrev
			return result
		}
		result.Verified++

		hash, err := HashEvent(&events[i])
		if err != nil {
			result.BrokenAt = i
			result.IsIntact = false
			return result
		}
		prevHash = hash
	}

	return result
}
