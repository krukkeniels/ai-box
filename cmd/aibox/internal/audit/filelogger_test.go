package audit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func tempLoggerConfig(t *testing.T) FileLoggerConfig {
	t.Helper()
	dir := t.TempDir()
	return FileLoggerConfig{
		Path:          filepath.Join(dir, "audit.jsonl"),
		MaxSizeMB:     100,
		FlushInterval: 50 * time.Millisecond,
	}
}

func testEvent(eventType EventType, userID string) AuditEvent {
	return AuditEvent{
		Timestamp: time.Date(2026, 2, 21, 10, 30, 0, 0, time.UTC),
		EventType: eventType,
		SandboxID: "aibox-test-1234",
		UserID:    userID,
		Source:    SourceCLI,
		Severity:  SeverityInfo,
		Details: map[string]any{
			"image": "aibox-base:latest",
		},
	}
}

func TestFileLoggerBasicWriteAndRead(t *testing.T) {
	cfg := tempLoggerConfig(t)
	logger, err := NewFileLogger(cfg)
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}
	defer logger.Close()

	ctx := context.Background()
	event := testEvent(EventSandboxCreate, "dev1")

	if err := logger.Log(ctx, event); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := logger.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	events, err := ReadEvents(cfg.Path)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}

	got := events[0]
	if got.EventType != EventSandboxCreate {
		t.Errorf("EventType = %q, want %q", got.EventType, EventSandboxCreate)
	}
	if got.UserID != "dev1" {
		t.Errorf("UserID = %q, want %q", got.UserID, "dev1")
	}
	if got.HashPrev != GenesisHash {
		t.Errorf("first event HashPrev = %q, want genesis hash", got.HashPrev)
	}
}

func TestFileLoggerHashChainIntegrity(t *testing.T) {
	cfg := tempLoggerConfig(t)
	logger, err := NewFileLogger(cfg)
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}

	ctx := context.Background()
	eventTypes := []EventType{
		EventSandboxCreate, EventSandboxStart,
		EventToolInvoke, EventNetworkAllow, EventSandboxStop,
	}

	for _, et := range eventTypes {
		event := AuditEvent{
			Timestamp: time.Now(),
			EventType: et,
			SandboxID: "aibox-chain-test",
			UserID:    "dev1",
			Source:    SourceCLI,
			Severity:  SeverityInfo,
		}
		if err := logger.Log(ctx, event); err != nil {
			t.Fatalf("Log %s: %v", et, err)
		}
	}

	logger.Close()

	events, err := ReadEvents(cfg.Path)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}

	if len(events) != len(eventTypes) {
		t.Fatalf("got %d events, want %d", len(events), len(eventTypes))
	}

	result := VerifyChain(events, GenesisHash)
	if !result.IsIntact {
		t.Errorf("hash chain should be intact, broken at index %d", result.BrokenAt)
	}
	if result.Verified != len(eventTypes) {
		t.Errorf("Verified = %d, want %d", result.Verified, len(eventTypes))
	}
}

func TestFileLoggerJSONFormat(t *testing.T) {
	cfg := tempLoggerConfig(t)
	logger, err := NewFileLogger(cfg)
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}
	defer logger.Close()

	ctx := context.Background()
	event := testEvent(EventNetworkDeny, "dev1")
	event.Details = map[string]any{
		"destination": "evil.com",
		"bytes":       float64(0),
	}

	if err := logger.Log(ctx, event); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := logger.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	data, err := os.ReadFile(cfg.Path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Verify it's valid JSON.
	var parsed map[string]any
	if err := json.Unmarshal(data[:len(data)-1], &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\ndata: %s", err, data)
	}

	requiredFields := []string{
		"timestamp", "event_type", "sandbox_id", "user_id",
		"source", "severity", "hash_prev",
	}
	for _, f := range requiredFields {
		if _, ok := parsed[f]; !ok {
			t.Errorf("missing required field %q in JSON output", f)
		}
	}

	// Verify details are present.
	details, ok := parsed["details"].(map[string]any)
	if !ok {
		t.Fatal("details field missing or not an object")
	}
	if details["destination"] != "evil.com" {
		t.Errorf("details.destination = %v, want %q", details["destination"], "evil.com")
	}
}

func TestFileLoggerMultipleEvents(t *testing.T) {
	cfg := tempLoggerConfig(t)
	logger, err := NewFileLogger(cfg)
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}
	defer logger.Close()

	ctx := context.Background()
	for i := 0; i < 10; i++ {
		event := AuditEvent{
			Timestamp: time.Date(2026, 2, 21, 10, 0, i, 0, time.UTC),
			EventType: EventDNSQuery,
			SandboxID: "aibox-multi-test",
			UserID:    "dev1",
			Source:    SourceCoreDNS,
			Severity:  SeverityInfo,
			Details: map[string]any{
				"query": "github.com",
				"index": float64(i),
			},
		}
		if err := logger.Log(ctx, event); err != nil {
			t.Fatalf("Log event %d: %v", i, err)
		}
	}

	if err := logger.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	events, err := ReadEvents(cfg.Path)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}

	if len(events) != 10 {
		t.Errorf("got %d events, want 10", len(events))
	}
}

func TestFileLoggerValidationRejectsInvalid(t *testing.T) {
	cfg := tempLoggerConfig(t)
	logger, err := NewFileLogger(cfg)
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Missing UserID.
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: EventSandboxCreate,
		SandboxID: "aibox-test",
		Source:    SourceCLI,
		Severity:  SeverityInfo,
	}

	err = logger.Log(ctx, event)
	if err != ErrMissingUserID {
		t.Errorf("Log with missing UserID = %v, want %v", err, ErrMissingUserID)
	}
}

func TestFileLoggerConcurrentWrites(t *testing.T) {
	cfg := tempLoggerConfig(t)
	logger, err := NewFileLogger(cfg)
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}
	defer logger.Close()

	ctx := context.Background()
	const goroutines = 10
	const eventsPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < eventsPerGoroutine; i++ {
				event := AuditEvent{
					Timestamp: time.Now(),
					EventType: EventToolInvoke,
					SandboxID: "aibox-concurrent-test",
					UserID:    "dev1",
					Source:    SourceAgent,
					Severity:  SeverityInfo,
					Details: map[string]any{
						"goroutine": float64(id),
						"index":     float64(i),
					},
				}
				if err := logger.Log(ctx, event); err != nil {
					t.Errorf("goroutine %d, event %d: %v", id, i, err)
				}
			}
		}(g)
	}

	wg.Wait()

	if err := logger.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	events, err := ReadEvents(cfg.Path)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}

	expected := goroutines * eventsPerGoroutine
	if len(events) != expected {
		t.Errorf("got %d events from concurrent writes, want %d", len(events), expected)
	}

	// Verify hash chain integrity even with concurrent writes.
	result := VerifyChain(events, GenesisHash)
	if !result.IsIntact {
		t.Errorf("hash chain broken at index %d after concurrent writes", result.BrokenAt)
	}
}

func TestFileLoggerRotation(t *testing.T) {
	cfg := tempLoggerConfig(t)
	logger, err := NewFileLogger(cfg)
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}
	// Override max size to trigger rotation.
	logger.config.MaxSizeMB = 0

	ctx := context.Background()
	event := testEvent(EventSandboxCreate, "dev1")

	if err := logger.Log(ctx, event); err != nil {
		t.Fatalf("Log first: %v", err)
	}
	if err := logger.Flush(ctx); err != nil {
		t.Fatalf("Flush first: %v", err)
	}

	// Second write should trigger rotation.
	if err := logger.Log(ctx, testEvent(EventSandboxStart, "dev1")); err != nil {
		t.Fatalf("Log second: %v", err)
	}
	if err := logger.Flush(ctx); err != nil {
		t.Fatalf("Flush second: %v", err)
	}

	logger.Close()

	rotated := cfg.Path + ".1"
	if _, err := os.Stat(rotated); os.IsNotExist(err) {
		t.Error("expected rotated file .1 to exist")
	}
	if _, err := os.Stat(cfg.Path); os.IsNotExist(err) {
		t.Error("expected new main log file to exist")
	}
}

func TestFileLoggerPeriodicFlush(t *testing.T) {
	cfg := tempLoggerConfig(t)
	cfg.FlushInterval = 50 * time.Millisecond
	logger, err := NewFileLogger(cfg)
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}
	defer logger.Close()

	ctx := context.Background()
	if err := logger.Log(ctx, testEvent(EventNetworkAllow, "dev1")); err != nil {
		t.Fatalf("Log: %v", err)
	}

	// Wait for background flush.
	time.Sleep(150 * time.Millisecond)

	info, err := os.Stat(cfg.Path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("expected non-zero file size after periodic flush")
	}
}

func TestFileLoggerCloseIdempotent(t *testing.T) {
	cfg := tempLoggerConfig(t)
	logger, err := NewFileLogger(cfg)
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}

	if err := logger.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	// Second close should not panic or return error.
	if err := logger.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestFileLoggerRejectsAfterClose(t *testing.T) {
	cfg := tempLoggerConfig(t)
	logger, err := NewFileLogger(cfg)
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}

	logger.Close()

	ctx := context.Background()
	err = logger.Log(ctx, testEvent(EventSandboxCreate, "dev1"))
	if err != ErrLoggerClosed {
		t.Errorf("Log after close = %v, want %v", err, ErrLoggerClosed)
	}
}

func TestFileLoggerChainRecovery(t *testing.T) {
	cfg := tempLoggerConfig(t)

	// Write some events with the first logger.
	logger1, err := NewFileLogger(cfg)
	if err != nil {
		t.Fatalf("NewFileLogger 1: %v", err)
	}

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		event := AuditEvent{
			Timestamp: time.Date(2026, 2, 21, 10, 0, i, 0, time.UTC),
			EventType: EventSandboxCreate,
			SandboxID: "aibox-recovery-test",
			UserID:    "dev1",
			Source:    SourceCLI,
			Severity:  SeverityInfo,
		}
		if err := logger1.Log(ctx, event); err != nil {
			t.Fatalf("Log 1.%d: %v", i, err)
		}
	}
	logger1.Close()

	// Open a second logger on the same file -- it should recover the chain.
	logger2, err := NewFileLogger(cfg)
	if err != nil {
		t.Fatalf("NewFileLogger 2: %v", err)
	}

	for i := 3; i < 6; i++ {
		event := AuditEvent{
			Timestamp: time.Date(2026, 2, 21, 10, 0, i, 0, time.UTC),
			EventType: EventSandboxStop,
			SandboxID: "aibox-recovery-test",
			UserID:    "dev1",
			Source:    SourceCLI,
			Severity:  SeverityInfo,
		}
		if err := logger2.Log(ctx, event); err != nil {
			t.Fatalf("Log 2.%d: %v", i, err)
		}
	}
	logger2.Close()

	// Read all events and verify the entire chain is intact.
	events, err := ReadEvents(cfg.Path)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}

	if len(events) != 6 {
		t.Fatalf("got %d events, want 6", len(events))
	}

	result := VerifyChain(events, GenesisHash)
	if !result.IsIntact {
		t.Errorf("hash chain broken at index %d after recovery", result.BrokenAt)
	}
	if result.Verified != 6 {
		t.Errorf("Verified = %d, want 6", result.Verified)
	}
}

func TestNopLogger(t *testing.T) {
	logger := NewNopLogger()
	ctx := context.Background()

	if err := logger.Log(ctx, validEvent()); err != nil {
		t.Errorf("NopLogger.Log: %v", err)
	}
	if err := logger.Flush(ctx); err != nil {
		t.Errorf("NopLogger.Flush: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Errorf("NopLogger.Close: %v", err)
	}
}
