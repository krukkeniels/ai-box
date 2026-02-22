package audit

import "context"

// NopLogger is an EventLogger that discards all events. Useful for testing
// and environments where audit logging is disabled.
type NopLogger struct{}

// NewNopLogger returns a no-op audit logger.
func NewNopLogger() *NopLogger { return &NopLogger{} }

func (n *NopLogger) Log(_ context.Context, _ AuditEvent) error { return nil }
func (n *NopLogger) Flush(_ context.Context) error             { return nil }
func (n *NopLogger) Close() error                              { return nil }
