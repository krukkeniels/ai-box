package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aibox/aibox/internal/audit"
)

func TestVerifyIntactChain(t *testing.T) {
	b := tempLocalBackend(t)
	ctx := context.Background()
	chain := audit.NewHashChain()

	// Store 3 batches with 5 events each.
	for i := 0; i < 3; i++ {
		events := makeEvents(5)
		batch := makeBatch(t, events, chain)
		batch.Key = generateBatchKey(time.Date(2026, 2, 21, 10, 0, i, 0, time.UTC))
		if _, err := b.Append(ctx, batch); err != nil {
			t.Fatalf("Append batch %d: %v", i, err)
		}
	}

	result, err := Verify(ctx, b)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if result.TotalBatches != 3 {
		t.Errorf("TotalBatches = %d, want 3", result.TotalBatches)
	}
	if result.TotalEvents != 15 {
		t.Errorf("TotalEvents = %d, want 15", result.TotalEvents)
	}
	if result.IntactBatches != 3 {
		t.Errorf("IntactBatches = %d, want 3", result.IntactBatches)
	}
	if result.CorruptBatches != 0 {
		t.Errorf("CorruptBatches = %d, want 0", result.CorruptBatches)
	}
	if !result.ChainIntact {
		t.Errorf("ChainIntact = false, want true (broken at %d)", result.ChainBrokenAt)
	}
	if result.FirstError != "" {
		t.Errorf("FirstError = %q, want empty", result.FirstError)
	}
}

func TestVerifyDetectsCorruptBatch(t *testing.T) {
	b := tempLocalBackend(t)
	ctx := context.Background()
	chain := audit.NewHashChain()

	events := makeEvents(3)
	batch := makeBatch(t, events, chain)
	batch.Key = "corrupt-batch"
	if _, err := b.Append(ctx, batch); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Tamper with the stored batch.
	path := filepath.Join(b.cfg.BaseDir, "corrupt-batch.json")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var stored Batch
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	stored.Entries[1] = []byte(`{"tampered": true}`)
	tampered, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, tampered, 0o444); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := Verify(ctx, b)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if result.CorruptBatches != 1 {
		t.Errorf("CorruptBatches = %d, want 1", result.CorruptBatches)
	}
	if result.ChainIntact {
		t.Error("ChainIntact should be false after corruption")
	}
	if result.FirstError == "" {
		t.Error("FirstError should be set after corruption")
	}
}

func TestVerifyDetectsTamperedEvent(t *testing.T) {
	b := tempLocalBackend(t)
	ctx := context.Background()
	chain := audit.NewHashChain()

	// Create events and chain them properly.
	events := makeEvents(5)
	batch := makeBatch(t, events, chain)

	// Tamper with event 2's content but keep checksum valid.
	// We need to recompute checksum for the tampered entries.
	var parsedEvent audit.AuditEvent
	if err := json.Unmarshal(batch.Entries[2], &parsedEvent); err != nil {
		t.Fatalf("Unmarshal event 2: %v", err)
	}
	parsedEvent.UserID = "attacker"
	tampered, err := json.Marshal(parsedEvent)
	if err != nil {
		t.Fatalf("Marshal tampered event: %v", err)
	}
	batch.Entries[2] = tampered
	batch.Checksum = computeBatchChecksum(batch.Entries) // Fix checksum so batch read succeeds
	batch.Key = "tampered-event"

	if _, err := b.Append(ctx, batch); err != nil {
		t.Fatalf("Append: %v", err)
	}

	result, err := Verify(ctx, b)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if result.ChainIntact {
		t.Error("ChainIntact should be false after event tampering")
	}
	if result.ChainBrokenAt < 0 {
		t.Error("ChainBrokenAt should indicate where chain broke")
	}
}

func TestVerifyEmptyStorage(t *testing.T) {
	b := tempLocalBackend(t)
	ctx := context.Background()

	result, err := Verify(ctx, b)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if result.TotalBatches != 0 {
		t.Errorf("TotalBatches = %d, want 0", result.TotalBatches)
	}
	if result.TotalEvents != 0 {
		t.Errorf("TotalEvents = %d, want 0", result.TotalEvents)
	}
	if !result.ChainIntact {
		t.Error("empty storage should report chain as intact")
	}
}

func TestVerifySingleEvent(t *testing.T) {
	b := tempLocalBackend(t)
	ctx := context.Background()
	chain := audit.NewHashChain()

	events := makeEvents(1)
	batch := makeBatch(t, events, chain)
	batch.Key = "single"
	if _, err := b.Append(ctx, batch); err != nil {
		t.Fatalf("Append: %v", err)
	}

	result, err := Verify(ctx, b)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if result.TotalEvents != 1 {
		t.Errorf("TotalEvents = %d, want 1", result.TotalEvents)
	}
	if !result.ChainIntact {
		t.Errorf("single-event chain should be intact")
	}
}

func TestVerifyMultipleBatchesContinuousChain(t *testing.T) {
	b := tempLocalBackend(t)
	ctx := context.Background()
	chain := audit.NewHashChain()

	// Store 5 batches with 2 events each, using the same chain.
	for i := 0; i < 5; i++ {
		events := makeEvents(2)
		batch := makeBatch(t, events, chain)
		batch.Key = generateBatchKey(time.Date(2026, 2, 21, 10, 0, i, 0, time.UTC))
		if _, err := b.Append(ctx, batch); err != nil {
			t.Fatalf("Append batch %d: %v", i, err)
		}
	}

	result, err := Verify(ctx, b)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if result.TotalBatches != 5 {
		t.Errorf("TotalBatches = %d, want 5", result.TotalBatches)
	}
	if result.TotalEvents != 10 {
		t.Errorf("TotalEvents = %d, want 10", result.TotalEvents)
	}
	if !result.ChainIntact {
		t.Errorf("chain across batches should be intact, broken at %d", result.ChainBrokenAt)
	}
}
