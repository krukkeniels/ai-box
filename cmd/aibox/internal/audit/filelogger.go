package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileLoggerConfig controls the file-based audit logger behaviour.
type FileLoggerConfig struct {
	Path          string        // Log file path (default: /var/log/aibox/audit.jsonl)
	MaxSizeMB     int           // Max file size before rotation (default: 100)
	FlushInterval time.Duration // How often to flush buffer (default: 5s)
}

// DefaultFileLoggerConfig returns a FileLoggerConfig with sensible defaults.
func DefaultFileLoggerConfig() FileLoggerConfig {
	return FileLoggerConfig{
		Path:          "/var/log/aibox/audit.jsonl",
		MaxSizeMB:     100,
		FlushInterval: 5 * time.Second,
	}
}

// FileLogger is an EventLogger that writes events as JSON Lines to a local file.
// It maintains a hash chain across all events and supports log rotation.
type FileLogger struct {
	writer *bufio.Writer
	file   *os.File
	mu     sync.Mutex
	config FileLoggerConfig
	chain  *HashChain
	closed bool

	done chan struct{}
	wg   sync.WaitGroup
}

// NewFileLogger creates a new file-based audit logger. A background goroutine
// periodically flushes the buffer.
func NewFileLogger(cfg FileLoggerConfig) (*FileLogger, error) {
	if cfg.Path == "" {
		cfg.Path = DefaultFileLoggerConfig().Path
	}
	if cfg.MaxSizeMB <= 0 {
		cfg.MaxSizeMB = 100
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 5 * time.Second
	}

	dir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("creating audit log directory %s: %w", dir, err)
	}

	// Determine chain head from existing log file.
	chain := recoverChainHead(cfg.Path)

	f, err := os.OpenFile(cfg.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening audit log %s: %w", cfg.Path, err)
	}

	fl := &FileLogger{
		writer: bufio.NewWriterSize(f, 64*1024),
		file:   f,
		config: cfg,
		chain:  chain,
		done:   make(chan struct{}),
	}

	fl.wg.Add(1)
	go fl.flushLoop()

	slog.Debug("audit file logger started", "path", cfg.Path, "flush_interval", cfg.FlushInterval)
	return fl, nil
}

// Log records a single audit event, setting its HashPrev via the hash chain.
func (fl *FileLogger) Log(_ context.Context, event AuditEvent) error {
	if err := event.Validate(); err != nil {
		return err
	}

	fl.mu.Lock()
	defer fl.mu.Unlock()

	if fl.closed {
		return ErrLoggerClosed
	}

	if err := fl.chain.Chain(&event); err != nil {
		return fmt.Errorf("computing hash chain: %w", err)
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshaling audit event: %w", err)
	}

	if err := fl.rotateIfNeeded(); err != nil {
		slog.Error("audit log rotation failed", "error", err)
	}

	if _, err := fl.writer.Write(data); err != nil {
		return fmt.Errorf("writing audit event: %w", err)
	}
	if err := fl.writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("writing newline: %w", err)
	}

	return nil
}

// Flush forces a buffer flush to disk.
func (fl *FileLogger) Flush(_ context.Context) error {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	return fl.writer.Flush()
}

// Close stops the background flush goroutine, flushes remaining data, and closes the file.
func (fl *FileLogger) Close() error {
	fl.mu.Lock()
	if fl.closed {
		fl.mu.Unlock()
		return nil
	}
	fl.closed = true
	fl.mu.Unlock()

	close(fl.done)
	fl.wg.Wait()

	fl.mu.Lock()
	defer fl.mu.Unlock()

	if err := fl.writer.Flush(); err != nil {
		return fmt.Errorf("flushing on close: %w", err)
	}
	return fl.file.Close()
}

// flushLoop runs in a background goroutine, flushing the buffer periodically.
func (fl *FileLogger) flushLoop() {
	defer fl.wg.Done()
	ticker := time.NewTicker(fl.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fl.mu.Lock()
			if err := fl.writer.Flush(); err != nil {
				slog.Error("periodic audit flush failed", "error", err)
			}
			fl.mu.Unlock()
		case <-fl.done:
			return
		}
	}
}

// rotateIfNeeded checks the current file size and rotates if it exceeds MaxSizeMB.
// Caller must hold fl.mu.
func (fl *FileLogger) rotateIfNeeded() error {
	info, err := fl.file.Stat()
	if err != nil {
		return fmt.Errorf("stat audit log: %w", err)
	}

	maxBytes := int64(fl.config.MaxSizeMB) * 1024 * 1024
	if info.Size() < maxBytes {
		return nil
	}

	slog.Info("rotating audit log", "size_bytes", info.Size(), "max_bytes", maxBytes)

	if err := fl.writer.Flush(); err != nil {
		return fmt.Errorf("flushing before rotation: %w", err)
	}
	if err := fl.file.Close(); err != nil {
		return fmt.Errorf("closing old log: %w", err)
	}

	// Shift existing rotated files: .2 -> .3, .1 -> .2, etc.
	for i := 8; i >= 1; i-- {
		old := fmt.Sprintf("%s.%d", fl.config.Path, i)
		newPath := fmt.Sprintf("%s.%d", fl.config.Path, i+1)
		_ = os.Rename(old, newPath)
	}

	if err := os.Rename(fl.config.Path, fl.config.Path+".1"); err != nil {
		return fmt.Errorf("renaming current audit log: %w", err)
	}

	f, err := os.OpenFile(fl.config.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("opening new audit log file: %w", err)
	}

	fl.file = f
	fl.writer = bufio.NewWriterSize(f, 64*1024)
	return nil
}

// recoverChainHead reads the last line of an existing log file to recover
// the hash chain head. Returns a new chain from genesis if the file doesn't
// exist or is empty.
func recoverChainHead(path string) *HashChain {
	f, err := os.Open(path)
	if err != nil {
		return NewHashChain()
	}
	defer f.Close()

	var lastLine []byte
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	for scanner.Scan() {
		lastLine = make([]byte, len(scanner.Bytes()))
		copy(lastLine, scanner.Bytes())
	}

	if len(lastLine) == 0 {
		return NewHashChain()
	}

	var event AuditEvent
	if err := json.Unmarshal(lastLine, &event); err != nil {
		slog.Warn("could not parse last audit log entry for chain recovery", "error", err)
		return NewHashChain()
	}

	hash, err := HashEvent(&event)
	if err != nil {
		slog.Warn("could not hash last audit log entry for chain recovery", "error", err)
		return NewHashChain()
	}

	return NewHashChainFrom(hash)
}

// ReadEvents reads all events from the log file. Used for verification and queries.
func ReadEvents(path string) ([]AuditEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening audit log for read: %w", err)
	}
	defer f.Close()

	var events []AuditEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	for scanner.Scan() {
		var event AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue // skip malformed lines
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning audit log: %w", err)
	}

	return events, nil
}
