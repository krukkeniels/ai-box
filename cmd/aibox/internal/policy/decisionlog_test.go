package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func tempLogConfig(t *testing.T) DecisionLogConfig {
	t.Helper()
	dir := t.TempDir()
	return DecisionLogConfig{
		Path:          filepath.Join(dir, "decisions.jsonl"),
		MaxSizeMB:     100,
		MaxAgeDays:    7,
		FlushInterval: 50 * time.Millisecond, // fast for tests
		SampleSafe:    0,                      // log everything
	}
}

func makeEntry(action, decision, risk, user string) DecisionEntry {
	return DecisionEntry{
		Timestamp:  time.Date(2026, 2, 20, 10, 30, 0, 0, time.UTC),
		PolicyVer:  "sha256:abc123",
		InputHash:  "sha256:def456",
		Action:     action,
		Command:    []string{"git", "push"},
		User:       user,
		Workspace:  "my-service",
		SandboxID:  "aibox-test-1234",
		Decision:   decision,
		RiskClass:  risk,
		Rule:       "tools.rules[1]",
		Reason:     "test reason",
		DurationMS: 2.5,
	}
}

func TestDecisionLoggerBasicWriteAndRead(t *testing.T) {
	cfg := tempLogConfig(t)
	logger, err := NewDecisionLogger(cfg)
	if err != nil {
		t.Fatalf("NewDecisionLogger: %v", err)
	}
	defer logger.Close()

	entry := makeEntry("command", "allow", RiskReviewRequired, "dev1")
	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log: %v", err)
	}

	got, err := logger.ReadEntry(0)
	if err != nil {
		t.Fatalf("ReadEntry: %v", err)
	}

	if got.Action != "command" {
		t.Errorf("Action = %q, want %q", got.Action, "command")
	}
	if got.Decision != "allow" {
		t.Errorf("Decision = %q, want %q", got.Decision, "allow")
	}
	if got.User != "dev1" {
		t.Errorf("User = %q, want %q", got.User, "dev1")
	}
	if got.DurationMS != 2.5 {
		t.Errorf("DurationMS = %f, want 2.5", got.DurationMS)
	}
}

func TestDecisionLoggerJSONFormat(t *testing.T) {
	cfg := tempLogConfig(t)
	logger, err := NewDecisionLogger(cfg)
	if err != nil {
		t.Fatalf("NewDecisionLogger: %v", err)
	}
	defer logger.Close()

	entry := makeEntry("command", "deny", RiskBlockedByDefault, "dev1")
	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := logger.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	data, err := os.ReadFile(cfg.Path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Verify it's valid JSON.
	var parsed map[string]any
	if err := json.Unmarshal(data[:len(data)-1], &parsed); err != nil { // trim trailing newline
		t.Fatalf("invalid JSON: %v\ndata: %s", err, data)
	}

	// Verify required fields are present.
	requiredFields := []string{"timestamp", "policy_version", "action", "decision", "risk_class", "rule", "reason", "duration_ms"}
	for _, f := range requiredFields {
		if _, ok := parsed[f]; !ok {
			t.Errorf("missing required field %q in JSON output", f)
		}
	}
}

func TestDecisionLoggerMultipleEntries(t *testing.T) {
	cfg := tempLogConfig(t)
	logger, err := NewDecisionLogger(cfg)
	if err != nil {
		t.Fatalf("NewDecisionLogger: %v", err)
	}
	defer logger.Close()

	for i := 0; i < 5; i++ {
		entry := makeEntry("command", "allow", RiskReviewRequired, "dev1")
		entry.DurationMS = float64(i)
		if err := logger.Log(entry); err != nil {
			t.Fatalf("Log entry %d: %v", i, err)
		}
	}

	// Read back the third entry (0-indexed).
	got, err := logger.ReadEntry(2)
	if err != nil {
		t.Fatalf("ReadEntry(2): %v", err)
	}
	if got.DurationMS != 2.0 {
		t.Errorf("DurationMS = %f, want 2.0", got.DurationMS)
	}
}

func TestDecisionLoggerReadEntryOutOfRange(t *testing.T) {
	cfg := tempLogConfig(t)
	logger, err := NewDecisionLogger(cfg)
	if err != nil {
		t.Fatalf("NewDecisionLogger: %v", err)
	}
	defer logger.Close()

	entry := makeEntry("command", "allow", RiskSafe, "dev1")
	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log: %v", err)
	}

	_, err = logger.ReadEntry(10)
	if err == nil {
		t.Fatal("expected error for out-of-range line, got nil")
	}
}

func TestDecisionLoggerFlush(t *testing.T) {
	cfg := tempLogConfig(t)
	cfg.FlushInterval = 1 * time.Hour // effectively disable auto-flush
	logger, err := NewDecisionLogger(cfg)
	if err != nil {
		t.Fatalf("NewDecisionLogger: %v", err)
	}
	defer logger.Close()

	entry := makeEntry("command", "allow", RiskReviewRequired, "dev1")
	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log: %v", err)
	}

	// Before flush, file may be empty (buffered).
	info, err := os.Stat(cfg.Path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	sizeBeforeFlush := info.Size()

	if err := logger.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	info, err = os.Stat(cfg.Path)
	if err != nil {
		t.Fatalf("Stat after flush: %v", err)
	}

	if info.Size() <= sizeBeforeFlush {
		t.Error("file size did not increase after Flush")
	}
}

func TestDecisionLoggerPeriodicFlush(t *testing.T) {
	cfg := tempLogConfig(t)
	cfg.FlushInterval = 50 * time.Millisecond
	logger, err := NewDecisionLogger(cfg)
	if err != nil {
		t.Fatalf("NewDecisionLogger: %v", err)
	}
	defer logger.Close()

	entry := makeEntry("network", "deny", RiskBlockedByDefault, "dev1")
	if err := logger.Log(entry); err != nil {
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

func TestDecisionLoggerSampling(t *testing.T) {
	cfg := tempLogConfig(t)
	cfg.SampleSafe = 5 // log every 5th safe decision
	logger, err := NewDecisionLogger(cfg)
	if err != nil {
		t.Fatalf("NewDecisionLogger: %v", err)
	}
	defer logger.Close()

	// Write 20 safe entries.
	for i := 0; i < 20; i++ {
		entry := makeEntry("command", "allow", RiskSafe, "dev1")
		if err := logger.Log(entry); err != nil {
			t.Fatalf("Log: %v", err)
		}
	}

	if err := logger.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Read back all entries.
	results, err := logger.Search(DecisionFilter{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// With sample rate 5, 20 entries should produce 4 logged entries.
	if len(results) != 4 {
		t.Errorf("expected 4 sampled entries, got %d", len(results))
	}
}

func TestDecisionLoggerSamplingNonSafeAlwaysLogged(t *testing.T) {
	cfg := tempLogConfig(t)
	cfg.SampleSafe = 100 // aggressively sample safe
	logger, err := NewDecisionLogger(cfg)
	if err != nil {
		t.Fatalf("NewDecisionLogger: %v", err)
	}
	defer logger.Close()

	// Write 10 blocked-by-default entries.
	for i := 0; i < 10; i++ {
		entry := makeEntry("command", "deny", RiskBlockedByDefault, "dev1")
		if err := logger.Log(entry); err != nil {
			t.Fatalf("Log: %v", err)
		}
	}

	if err := logger.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	results, err := logger.Search(DecisionFilter{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// All non-safe entries should be logged regardless of sample rate.
	if len(results) != 10 {
		t.Errorf("expected 10 entries (all non-safe logged), got %d", len(results))
	}
}

func TestDecisionLoggerRotation(t *testing.T) {
	cfg := tempLogConfig(t)
	cfg.MaxSizeMB = 0 // will be set to minimum; we need a very small rotation threshold

	// Use a tiny max size to trigger rotation.
	logger, err := NewDecisionLogger(cfg)
	if err != nil {
		t.Fatalf("NewDecisionLogger: %v", err)
	}
	// Override max size to something tiny for testing.
	logger.config.MaxSizeMB = 0

	// Write enough to overflow. Each entry is ~300 bytes. MaxSizeMB=0 means maxBytes=0, so
	// every write triggers rotation.
	entry := makeEntry("command", "allow", RiskReviewRequired, "dev1")
	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log first entry: %v", err)
	}
	if err := logger.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Second write should trigger rotation.
	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log second entry: %v", err)
	}
	if err := logger.Flush(); err != nil {
		t.Fatalf("Flush after rotation: %v", err)
	}

	logger.Close()

	// Check that the rotated file exists.
	rotated := cfg.Path + ".1"
	if _, err := os.Stat(rotated); os.IsNotExist(err) {
		t.Error("expected rotated file .1 to exist")
	}

	// Check that the new main file also exists.
	if _, err := os.Stat(cfg.Path); os.IsNotExist(err) {
		t.Error("expected new main log file to exist")
	}
}

func TestDecisionLoggerConcurrentWrites(t *testing.T) {
	cfg := tempLogConfig(t)
	logger, err := NewDecisionLogger(cfg)
	if err != nil {
		t.Fatalf("NewDecisionLogger: %v", err)
	}
	defer logger.Close()

	const goroutines = 10
	const entriesPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < entriesPerGoroutine; i++ {
				entry := makeEntry("command", "allow", RiskReviewRequired, "dev1")
				entry.DurationMS = float64(id*1000 + i)
				if err := logger.Log(entry); err != nil {
					t.Errorf("goroutine %d, entry %d: %v", id, i, err)
				}
			}
		}(g)
	}

	wg.Wait()

	results, err := logger.Search(DecisionFilter{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	expected := goroutines * entriesPerGoroutine
	if len(results) != expected {
		t.Errorf("expected %d entries from concurrent writes, got %d", expected, len(results))
	}
}

func TestDecisionLoggerSearchFilter(t *testing.T) {
	cfg := tempLogConfig(t)
	logger, err := NewDecisionLogger(cfg)
	if err != nil {
		t.Fatalf("NewDecisionLogger: %v", err)
	}
	defer logger.Close()

	base := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)

	entries := []DecisionEntry{
		{Timestamp: base, Action: "command", Decision: "allow", RiskClass: RiskReviewRequired, User: "alice"},
		{Timestamp: base.Add(1 * time.Hour), Action: "network", Decision: "deny", RiskClass: RiskBlockedByDefault, User: "bob"},
		{Timestamp: base.Add(2 * time.Hour), Action: "command", Decision: "deny", RiskClass: RiskBlockedByDefault, User: "alice"},
		{Timestamp: base.Add(3 * time.Hour), Action: "filesystem", Decision: "allow", RiskClass: RiskReviewRequired, User: "charlie"},
	}
	for _, e := range entries {
		if err := logger.Log(e); err != nil {
			t.Fatalf("Log: %v", err)
		}
	}

	tests := []struct {
		name   string
		filter DecisionFilter
		want   int
	}{
		{"all", DecisionFilter{}, 4},
		{"user alice", DecisionFilter{User: "alice"}, 2},
		{"deny only", DecisionFilter{Decision: "deny"}, 2},
		{"action command", DecisionFilter{Action: "command"}, 2},
		{"limit 1", DecisionFilter{Limit: 1}, 1},
		{"since after first", DecisionFilter{Since: base.Add(30 * time.Minute)}, 3},
		{"until before last", DecisionFilter{Until: base.Add(2*time.Hour + 30*time.Minute)}, 3},
		{"combined", DecisionFilter{User: "alice", Decision: "deny"}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := logger.Search(tt.filter)
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if len(results) != tt.want {
				t.Errorf("got %d results, want %d", len(results), tt.want)
			}
		})
	}
}

func TestEntryFromResult(t *testing.T) {
	input := PolicyInput{
		Action:    "command",
		Command:   []string{"git", "push"},
		Target:    "",
		User:      "dev1",
		Workspace: "my-service",
		Timestamp: time.Now(),
	}

	result := DecisionResult{
		Allowed:   true,
		RiskClass: RiskReviewRequired,
		Rule:      "tools.rules[1]",
		Reason:    "git push is review-required",
		PolicyVer: "sha256:abc",
		InputHash: "sha256:def",
		Timestamp: time.Now(),
		Duration:  2500 * time.Microsecond,
	}

	entry := EntryFromResult(input, result, "aibox-sandbox-1234")
	if entry.Decision != "allow" {
		t.Errorf("Decision = %q, want %q", entry.Decision, "allow")
	}
	if entry.SandboxID != "aibox-sandbox-1234" {
		t.Errorf("SandboxID = %q, want %q", entry.SandboxID, "aibox-sandbox-1234")
	}
	if entry.DurationMS != 2.5 {
		t.Errorf("DurationMS = %f, want 2.5", entry.DurationMS)
	}
}

func TestEntryFromResultDeny(t *testing.T) {
	result := DecisionResult{Allowed: false}
	entry := EntryFromResult(PolicyInput{}, result, "")
	if entry.Decision != "deny" {
		t.Errorf("Decision = %q, want %q", entry.Decision, "deny")
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "/var/log/aibox/decisions.jsonl"},
		{"decisions", "/var/log/aibox/decisions.jsonl"},
		{"/custom/path/audit.jsonl", "/custom/path/audit.jsonl"},
		{"/custom/path/audit", "/custom/path/audit.jsonl"},
	}
	for _, tt := range tests {
		got := SanitizePath(tt.input)
		if got != tt.want {
			t.Errorf("SanitizePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSamplerAlwaysLogsWhenRateZero(t *testing.T) {
	s := newSampler(0)
	for i := 0; i < 100; i++ {
		if !s.shouldLog() {
			t.Fatal("sampler with rate=0 should always log")
		}
	}
}

func TestSamplerAlwaysLogsWhenRateOne(t *testing.T) {
	s := newSampler(1)
	for i := 0; i < 100; i++ {
		if !s.shouldLog() {
			t.Fatal("sampler with rate=1 should always log")
		}
	}
}

func TestSamplerRate(t *testing.T) {
	s := newSampler(5)
	logged := 0
	for i := 0; i < 100; i++ {
		if s.shouldLog() {
			logged++
		}
	}
	if logged != 20 {
		t.Errorf("sampler(5) logged %d out of 100, expected 20", logged)
	}
}
