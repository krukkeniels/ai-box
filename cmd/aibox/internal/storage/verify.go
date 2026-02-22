package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aibox/aibox/internal/audit"
)

var zeroTime time.Time

// VerifyResult holds the result of a full audit log verification.
type VerifyResult struct {
	TotalBatches   int    // total batches checked
	TotalEvents    int    // total events across all batches
	IntactBatches  int    // batches with valid checksums
	CorruptBatches int    // batches with checksum mismatches
	ChainIntact    bool   // true if the hash chain is intact across all batches
	ChainBrokenAt  int    // event index where chain broke (-1 if intact)
	FirstError     string // description of first error found (empty if none)
}

// Verify checks the integrity of all stored audit logs:
// 1. Each batch's checksum is verified.
// 2. Events within and across batches are verified for hash chain continuity.
func Verify(ctx context.Context, backend Backend) (*VerifyResult, error) {
	result := &VerifyResult{
		ChainBrokenAt: -1,
		ChainIntact:   true,
	}

	keys, err := backend.List(ctx, zeroTime, zeroTime)
	if err != nil {
		return nil, fmt.Errorf("listing batches: %w", err)
	}

	prevHash := audit.GenesisHash
	globalIndex := 0

	for _, key := range keys {
		result.TotalBatches++

		batch, err := backend.Read(ctx, key)
		if err != nil {
			result.CorruptBatches++
			if result.FirstError == "" {
				result.FirstError = fmt.Sprintf("batch %s: %v", key, err)
			}
			// Cannot continue chain verification if a batch is corrupt.
			result.ChainIntact = false
			if result.ChainBrokenAt == -1 {
				result.ChainBrokenAt = globalIndex
			}
			continue
		}

		result.IntactBatches++

		// Verify hash chain within this batch.
		for _, raw := range batch.Entries {
			var event audit.AuditEvent
			if err := json.Unmarshal(raw, &event); err != nil {
				result.ChainIntact = false
				if result.ChainBrokenAt == -1 {
					result.ChainBrokenAt = globalIndex
				}
				if result.FirstError == "" {
					result.FirstError = fmt.Sprintf("event %d: unmarshal error: %v", globalIndex, err)
				}
				globalIndex++
				continue
			}

			if event.HashPrev != prevHash {
				result.ChainIntact = false
				if result.ChainBrokenAt == -1 {
					result.ChainBrokenAt = globalIndex
				}
				if result.FirstError == "" {
					result.FirstError = fmt.Sprintf("event %d: hash chain broken (expected %s, got %s)",
						globalIndex, prevHash, event.HashPrev)
				}
			}

			hash, err := audit.HashEvent(&event)
			if err != nil {
				result.ChainIntact = false
				if result.ChainBrokenAt == -1 {
					result.ChainBrokenAt = globalIndex
				}
				if result.FirstError == "" {
					result.FirstError = fmt.Sprintf("event %d: hash computation error: %v", globalIndex, err)
				}
			} else {
				prevHash = hash
			}

			result.TotalEvents++
			globalIndex++
		}
	}

	return result, nil
}
