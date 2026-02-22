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

func tempLocalBackend(t *testing.T) *LocalBackend {
	t.Helper()
	dir := t.TempDir()
	b, err := NewLocalBackend(LocalConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewLocalBackend: %v", err)
	}
	return b
}

func makeBatch(t *testing.T, events []audit.AuditEvent, chain *audit.HashChain) Batch {
	t.Helper()
	var entries [][]byte
	for i := range events {
		if err := chain.Chain(&events[i]); err != nil {
			t.Fatalf("Chain event %d: %v", i, err)
		}
		data, err := json.Marshal(events[i])
		if err != nil {
			t.Fatalf("Marshal event %d: %v", i, err)
		}
		entries = append(entries, data)
	}
	return Batch{
		Entries:   entries,
		CreatedAt: time.Now().UTC(),
		ChainHead: chain.LastHash(),
	}
}

func makeEvents(n int) []audit.AuditEvent {
	events := make([]audit.AuditEvent, n)
	for i := range events {
		events[i] = audit.AuditEvent{
			Timestamp: time.Date(2026, 2, 21, 10, 0, i, 0, time.UTC),
			EventType: audit.EventSandboxCreate,
			SandboxID: "aibox-test-1234",
			UserID:    "dev1",
			Source:    audit.SourceCLI,
			Severity:  audit.SeverityInfo,
		}
	}
	return events
}

func TestLocalBackendName(t *testing.T) {
	b := tempLocalBackend(t)
	if got := b.Name(); got != "local" {
		t.Errorf("Name() = %q, want %q", got, "local")
	}
}

func TestLocalBackendAppendAndRead(t *testing.T) {
	b := tempLocalBackend(t)
	ctx := context.Background()
	chain := audit.NewHashChain()

	events := makeEvents(3)
	batch := makeBatch(t, events, chain)

	key, err := b.Append(ctx, batch)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if key == "" {
		t.Fatal("Append returned empty key")
	}

	got, err := b.Read(ctx, key)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if len(got.Entries) != 3 {
		t.Errorf("got %d entries, want 3", len(got.Entries))
	}
	if got.ChainHead != chain.LastHash() {
		t.Errorf("ChainHead = %q, want %q", got.ChainHead, chain.LastHash())
	}
}

func TestLocalBackendReadNotFound(t *testing.T) {
	b := tempLocalBackend(t)
	ctx := context.Background()

	_, err := b.Read(ctx, "nonexistent-key")
	if err != ErrBatchNotFound {
		t.Errorf("Read nonexistent = %v, want %v", err, ErrBatchNotFound)
	}
}

func TestLocalBackendImmutableViolation(t *testing.T) {
	b := tempLocalBackend(t)
	ctx := context.Background()
	chain := audit.NewHashChain()

	events := makeEvents(1)
	batch := makeBatch(t, events, chain)
	batch.Key = "test-batch"

	_, err := b.Append(ctx, batch)
	if err != nil {
		t.Fatalf("first Append: %v", err)
	}

	// Attempt to overwrite the same key.
	_, err = b.Append(ctx, batch)
	if err != ErrImmutableViolation {
		t.Errorf("second Append = %v, want %v", err, ErrImmutableViolation)
	}
}

func TestLocalBackendFilePermissions(t *testing.T) {
	b := tempLocalBackend(t)
	ctx := context.Background()
	chain := audit.NewHashChain()

	events := makeEvents(1)
	batch := makeBatch(t, events, chain)
	batch.Key = "perm-test"

	_, err := b.Append(ctx, batch)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	path := filepath.Join(b.cfg.BaseDir, "perm-test.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0o444 {
		t.Errorf("file permissions = %o, want 0444 (read-only)", perm)
	}
}

func TestLocalBackendChecksumVerification(t *testing.T) {
	b := tempLocalBackend(t)
	ctx := context.Background()
	chain := audit.NewHashChain()

	events := makeEvents(2)
	batch := makeBatch(t, events, chain)
	batch.Key = "checksum-test"

	_, err := b.Append(ctx, batch)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Tamper with the stored file.
	path := filepath.Join(b.cfg.BaseDir, "checksum-test.json")
	// Need to make writable first to tamper.
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

	// Tamper with an entry.
	stored.Entries[0] = []byte(`{"tampered": true}`)

	tampered, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("Marshal tampered: %v", err)
	}

	if err := os.WriteFile(path, tampered, 0o444); err != nil {
		t.Fatalf("WriteFile tampered: %v", err)
	}

	// Read should detect corruption.
	_, err = b.Read(ctx, "checksum-test")
	if err == nil {
		t.Fatal("expected error reading tampered batch")
	}
	if !isCorrupted(err) {
		t.Errorf("expected ErrBatchCorrupted, got: %v", err)
	}
}

func isCorrupted(err error) bool {
	return err != nil && (err == ErrBatchCorrupted || (err.Error() != "" && len(err.Error()) > 0 && err.Error()[:len("storage: batch checksum")] == "storage: batch checksum"))
}

func TestLocalBackendList(t *testing.T) {
	b := tempLocalBackend(t)
	ctx := context.Background()
	chain := audit.NewHashChain()

	// Create 3 batches with distinct keys.
	for i := 0; i < 3; i++ {
		events := makeEvents(1)
		batch := makeBatch(t, events, chain)
		batch.Key = generateBatchKey(time.Date(2026, 2, 21, 10, 0, i, 0, time.UTC))
		if _, err := b.Append(ctx, batch); err != nil {
			t.Fatalf("Append batch %d: %v", i, err)
		}
	}

	keys, err := b.List(ctx, zeroTime, zeroTime)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(keys) != 3 {
		t.Errorf("got %d keys, want 3", len(keys))
	}

	// Verify sorted order.
	for i := 1; i < len(keys); i++ {
		if keys[i] < keys[i-1] {
			t.Errorf("keys not sorted: %q < %q", keys[i], keys[i-1])
		}
	}
}

func TestLocalBackendListWithTimeFilter(t *testing.T) {
	b := tempLocalBackend(t)
	ctx := context.Background()
	chain := audit.NewHashChain()

	times := []time.Time{
		time.Date(2026, 2, 19, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC),
	}

	for _, ts := range times {
		events := makeEvents(1)
		batch := makeBatch(t, events, chain)
		batch.Key = generateBatchKey(ts)
		batch.CreatedAt = ts
		if _, err := b.Append(ctx, batch); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	// Filter: since Feb 20
	since := time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)
	keys, err := b.List(ctx, since, zeroTime)
	if err != nil {
		t.Fatalf("List with since: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("got %d keys with since filter, want 2", len(keys))
	}

	// Filter: until Feb 20
	until := time.Date(2026, 2, 20, 11, 0, 0, 0, time.UTC)
	keys, err = b.List(ctx, zeroTime, until)
	if err != nil {
		t.Fatalf("List with until: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("got %d keys with until filter, want 2", len(keys))
	}
}

func TestLocalBackendListEmpty(t *testing.T) {
	b := tempLocalBackend(t)
	ctx := context.Background()

	keys, err := b.List(ctx, zeroTime, zeroTime)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("got %d keys, want 0", len(keys))
	}
}

func TestGenerateBatchKey(t *testing.T) {
	ts := time.Date(2026, 2, 21, 15, 30, 45, 0, time.UTC)
	key := generateBatchKey(ts)

	if key != "20260221T153045Z" {
		t.Errorf("generateBatchKey = %q, want %q", key, "20260221T153045Z")
	}
}

func TestComputeBatchChecksum_Deterministic(t *testing.T) {
	entries := [][]byte{
		[]byte(`{"a": 1}`),
		[]byte(`{"b": 2}`),
	}

	sum1 := computeBatchChecksum(entries)
	sum2 := computeBatchChecksum(entries)

	if sum1 != sum2 {
		t.Errorf("checksum not deterministic: %q vs %q", sum1, sum2)
	}
}

func TestComputeBatchChecksum_DifferentForDifferentEntries(t *testing.T) {
	entries1 := [][]byte{[]byte(`{"a": 1}`)}
	entries2 := [][]byte{[]byte(`{"a": 2}`)}

	sum1 := computeBatchChecksum(entries1)
	sum2 := computeBatchChecksum(entries2)

	if sum1 == sum2 {
		t.Error("different entries should produce different checksums")
	}
}
