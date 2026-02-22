package policy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DecisionLogConfig controls the decision logger behaviour.
type DecisionLogConfig struct {
	Path          string        // Log file path (default: /var/log/aibox/decisions.jsonl)
	MaxSizeMB     int           // Max file size before rotation (default: 100)
	MaxAgeDays    int           // Max days to keep old files (default: 7)
	FlushInterval time.Duration // How often to flush buffer (default: 5s)
	SampleSafe    int           // Sample 1-in-N for safe decisions (default: 10, 0=log all)
}

// DefaultDecisionLogConfig returns a DecisionLogConfig with sensible defaults.
func DefaultDecisionLogConfig() DecisionLogConfig {
	return DecisionLogConfig{
		Path:          "/var/log/aibox/decisions.jsonl",
		MaxSizeMB:     100,
		MaxAgeDays:    7,
		FlushInterval: 5 * time.Second,
		SampleSafe:    10,
	}
}

// DecisionEntry is the structured log entry for a single policy decision.
type DecisionEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	PolicyVer  string    `json:"policy_version"`
	InputHash  string    `json:"input_hash"`
	Action     string    `json:"action"`
	Command    []string  `json:"command,omitempty"`
	Target     string    `json:"target,omitempty"`
	User       string    `json:"user"`
	Workspace  string    `json:"workspace"`
	SandboxID  string    `json:"sandbox_id"`
	Decision   string    `json:"decision"`
	RiskClass  string    `json:"risk_class"`
	Rule       string    `json:"rule"`
	Reason     string    `json:"reason"`
	DurationMS float64   `json:"duration_ms"`
}

// DecisionFilter specifies criteria for searching decision log entries.
type DecisionFilter struct {
	Since    time.Time
	Until    time.Time
	User     string
	Action   string
	Decision string // "allow" or "deny"
	Limit    int
}

// DecisionLogger writes structured JSON decision logs for audit.
type DecisionLogger struct {
	writer  *bufio.Writer
	file    *os.File
	mu      sync.Mutex
	config  DecisionLogConfig
	sampler *sampler

	done chan struct{}
	wg   sync.WaitGroup
}

// sampler provides deterministic 1-in-N sampling using a simple counter.
type sampler struct {
	mu    sync.Mutex
	rate  int
	count int
}

func newSampler(rate int) *sampler {
	return &sampler{rate: rate}
}

// shouldLog returns true if this event should be logged.
// When rate <= 1 (or 0), every event is logged.
func (s *sampler) shouldLog() bool {
	if s.rate <= 1 {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	if s.count >= s.rate {
		s.count = 0
		return true
	}
	return false
}

// NewDecisionLogger creates a new decision logger that writes JSON Lines to the
// configured path. A background goroutine periodically flushes the buffer.
func NewDecisionLogger(cfg DecisionLogConfig) (*DecisionLogger, error) {
	if cfg.Path == "" {
		cfg.Path = DefaultDecisionLogConfig().Path
	}
	if cfg.MaxSizeMB <= 0 {
		cfg.MaxSizeMB = 100
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 5 * time.Second
	}

	dir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("creating log directory %s: %w", dir, err)
	}

	f, err := os.OpenFile(cfg.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening decision log %s: %w", cfg.Path, err)
	}

	dl := &DecisionLogger{
		writer:  bufio.NewWriterSize(f, 64*1024), // 64 KiB buffer
		file:    f,
		config:  cfg,
		sampler: newSampler(cfg.SampleSafe),
		done:    make(chan struct{}),
	}

	dl.wg.Add(1)
	go dl.flushLoop()

	slog.Debug("decision logger started", "path", cfg.Path, "flush_interval", cfg.FlushInterval)
	return dl, nil
}

// Log writes a single decision entry. Safe decisions may be sampled; all other
// risk classes are always logged. The write is buffered and flushed periodically.
func (l *DecisionLogger) Log(entry DecisionEntry) error {
	// Sample safe decisions to reduce volume.
	if entry.RiskClass == RiskSafe && !l.sampler.shouldLog() {
		return nil
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling decision entry: %w", err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.rotateIfNeeded(); err != nil {
		slog.Error("decision log rotation failed", "error", err)
		// Continue writing to the current file.
	}

	if _, err := l.writer.Write(data); err != nil {
		return fmt.Errorf("writing decision entry: %w", err)
	}
	if err := l.writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("writing newline: %w", err)
	}

	return nil
}

// Flush forces a buffer flush to disk.
func (l *DecisionLogger) Flush() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.flushLocked()
}

// Close stops the background flush goroutine, flushes remaining data, and closes the file.
func (l *DecisionLogger) Close() error {
	close(l.done)
	l.wg.Wait()

	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.flushLocked(); err != nil {
		return fmt.Errorf("flushing on close: %w", err)
	}
	return l.file.Close()
}

// ReadEntry reads and parses a single log entry at the given 0-based line number.
// This is used by the `policy explain` command to reference specific decisions.
func (l *DecisionLogger) ReadEntry(lineNum int) (*DecisionEntry, error) {
	l.mu.Lock()
	// Flush so that buffered entries are readable.
	_ = l.flushLocked()
	l.mu.Unlock()

	f, err := os.Open(l.config.Path)
	if err != nil {
		return nil, fmt.Errorf("opening decision log for read: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	cur := 0
	for scanner.Scan() {
		if cur == lineNum {
			var entry DecisionEntry
			if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
				return nil, fmt.Errorf("parsing entry at line %d: %w", lineNum, err)
			}
			return &entry, nil
		}
		cur++
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning decision log: %w", err)
	}

	return nil, fmt.Errorf("line %d not found (file has %d lines)", lineNum, cur)
}

// Search returns entries matching the given filter. Entries are scanned from
// the current log file. A Limit of 0 returns all matches.
func (l *DecisionLogger) Search(filter DecisionFilter) ([]DecisionEntry, error) {
	l.mu.Lock()
	_ = l.flushLocked()
	l.mu.Unlock()

	f, err := os.Open(l.config.Path)
	if err != nil {
		return nil, fmt.Errorf("opening decision log for search: %w", err)
	}
	defer f.Close()

	var results []DecisionEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	for scanner.Scan() {
		var entry DecisionEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // skip malformed lines
		}

		if !matchesFilter(entry, filter) {
			continue
		}

		results = append(results, entry)
		if filter.Limit > 0 && len(results) >= filter.Limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning decision log: %w", err)
	}

	return results, nil
}

// matchesFilter checks whether an entry satisfies every non-zero field in the filter.
func matchesFilter(e DecisionEntry, f DecisionFilter) bool {
	if !f.Since.IsZero() && e.Timestamp.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && e.Timestamp.After(f.Until) {
		return false
	}
	if f.User != "" && e.User != f.User {
		return false
	}
	if f.Action != "" && e.Action != f.Action {
		return false
	}
	if f.Decision != "" && e.Decision != f.Decision {
		return false
	}
	return true
}

// flushLoop runs in a background goroutine, flushing the buffer periodically.
func (l *DecisionLogger) flushLoop() {
	defer l.wg.Done()
	ticker := time.NewTicker(l.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			if err := l.flushLocked(); err != nil {
				slog.Error("periodic flush failed", "error", err)
			}
			l.mu.Unlock()
		case <-l.done:
			return
		}
	}
}

// flushLocked flushes the bufio.Writer. Caller must hold l.mu.
func (l *DecisionLogger) flushLocked() error {
	return l.writer.Flush()
}

// rotateIfNeeded checks the current file size and rotates if it exceeds MaxSizeMB.
// Caller must hold l.mu.
func (l *DecisionLogger) rotateIfNeeded() error {
	info, err := l.file.Stat()
	if err != nil {
		return fmt.Errorf("stat decision log: %w", err)
	}

	maxBytes := int64(l.config.MaxSizeMB) * 1024 * 1024
	if info.Size() < maxBytes {
		return nil
	}

	slog.Info("rotating decision log", "size_bytes", info.Size(), "max_bytes", maxBytes)

	// Flush before rotating.
	if err := l.writer.Flush(); err != nil {
		return fmt.Errorf("flushing before rotation: %w", err)
	}

	if err := l.file.Close(); err != nil {
		return fmt.Errorf("closing old log: %w", err)
	}

	// Shift existing rotated files: .2 -> .3, .1 -> .2, current -> .1
	l.shiftRotatedFiles()

	// Rename current file to .1
	if err := os.Rename(l.config.Path, l.config.Path+".1"); err != nil {
		return fmt.Errorf("renaming current log: %w", err)
	}

	// Open a new file.
	f, err := os.OpenFile(l.config.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("opening new log file: %w", err)
	}

	l.file = f
	l.writer = bufio.NewWriterSize(f, 64*1024)
	return nil
}

// shiftRotatedFiles shifts .N -> .N+1 for existing rotated log files.
func (l *DecisionLogger) shiftRotatedFiles() {
	// Keep up to 9 rotated files.
	for i := 8; i >= 1; i-- {
		old := fmt.Sprintf("%s.%d", l.config.Path, i)
		new := fmt.Sprintf("%s.%d", l.config.Path, i+1)
		// Errors are expected for non-existent files; ignore them.
		_ = os.Rename(old, new)
	}
}

// EntryFromResult converts a PolicyInput + DecisionResult into a DecisionEntry
// suitable for logging. The sandboxID is provided by the caller.
func EntryFromResult(input PolicyInput, result DecisionResult, sandboxID string) DecisionEntry {
	decision := "deny"
	if result.Allowed {
		decision = "allow"
	}
	return DecisionEntry{
		Timestamp:  result.Timestamp,
		PolicyVer:  result.PolicyVer,
		InputHash:  result.InputHash,
		Action:     input.Action,
		Command:    input.Command,
		Target:     input.Target,
		User:       input.User,
		Workspace:  input.Workspace,
		SandboxID:  sandboxID,
		Decision:   decision,
		RiskClass:  result.RiskClass,
		Rule:       result.Rule,
		Reason:     result.Reason,
		DurationMS: float64(result.Duration.Microseconds()) / 1000.0,
	}
}

// SanitizePath ensures the log path has a .jsonl extension and is absolute.
func SanitizePath(path string) string {
	if path == "" {
		return DefaultDecisionLogConfig().Path
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join("/var/log/aibox", path)
	}
	if !strings.HasSuffix(path, ".jsonl") {
		path += ".jsonl"
	}
	return path
}
