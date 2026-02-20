package main

import (
	"encoding/json"
	"os"
	"sync"
)

// PayloadEntry is a single audit log record written as JSON Lines.
type PayloadEntry struct {
	Timestamp    string `json:"timestamp"`
	SandboxID    string `json:"sandbox_id"`
	User         string `json:"user"`
	Method       string `json:"method"`
	Path         string `json:"path"`
	Model        string `json:"model,omitempty"`
	RequestSize  int    `json:"request_size_bytes"`
	ResponseSize int    `json:"response_size_bytes"`
	EstTokens    int    `json:"estimated_tokens"`
	DurationMS   int64  `json:"duration_ms"`
	StatusCode   int    `json:"status_code"`
	RequestBody  string `json:"request_body,omitempty"`
	ResponseBody string `json:"response_body,omitempty"`
}

// PayloadLogger writes audit log entries as JSON Lines to a file.
type PayloadLogger struct {
	file    *os.File
	encoder *json.Encoder
	mu      sync.Mutex
	maxSize int64
}

// NewPayloadLogger opens (or creates) the log file in append-only mode.
func NewPayloadLogger(path string, maxSize int64) (*PayloadLogger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	return &PayloadLogger{
		file:    f,
		encoder: json.NewEncoder(f),
		maxSize: maxSize,
	}, nil
}

// Log writes a single payload entry as a JSON line. Bodies are truncated if
// they exceed maxSize, but only in the log -- the full payload is always
// forwarded to the client.
func (l *PayloadLogger) Log(entry PayloadEntry) {
	if l.maxSize > 0 {
		entry.RequestBody = truncate(entry.RequestBody, l.maxSize)
		entry.ResponseBody = truncate(entry.ResponseBody, l.maxSize)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	// Errors are intentionally swallowed: logging must not break proxying.
	_ = l.encoder.Encode(entry)
}

// Close flushes and closes the log file.
func (l *PayloadLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// EstimateTokens provides a rough token estimate: ~4 bytes per token.
func EstimateTokens(requestSize, responseSize int) int {
	return (requestSize + responseSize) / 4
}

// ExtractModel attempts to pull the "model" field from a JSON request body.
func ExtractModel(body []byte) string {
	var partial struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &partial) == nil {
		return partial.Model
	}
	return ""
}

func truncate(s string, max int64) string {
	if int64(len(s)) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}
