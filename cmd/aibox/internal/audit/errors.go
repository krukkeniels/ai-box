package audit

import "errors"

// Validation errors for AuditEvent fields.
var (
	ErrMissingTimestamp = errors.New("audit: timestamp is required")
	ErrMissingEventType = errors.New("audit: event_type is required")
	ErrMissingSandboxID = errors.New("audit: sandbox_id is required")
	ErrMissingUserID    = errors.New("audit: user_id is required")
	ErrMissingSource    = errors.New("audit: source is required")
	ErrMissingSeverity  = errors.New("audit: severity is required")
)

// Hash chain errors.
var (
	ErrChainBroken   = errors.New("audit: hash chain is broken (tamper detected)")
	ErrEmptyEvent    = errors.New("audit: cannot hash empty event data")
	ErrLoggerClosed  = errors.New("audit: logger is closed")
)
