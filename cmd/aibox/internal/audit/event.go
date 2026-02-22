package audit

import (
	"encoding/json"
	"time"
)

// EventType classifies audit events. Matches spec Section 19.1.
type EventType string

const (
	// Sandbox lifecycle events (spec: 2+ year retention).
	EventSandboxCreate  EventType = "sandbox.create"
	EventSandboxStart   EventType = "sandbox.start"
	EventSandboxStop    EventType = "sandbox.stop"
	EventSandboxDestroy EventType = "sandbox.destroy"
	EventSandboxConfig  EventType = "sandbox.config"

	// Network events (spec: 1+ year retention).
	EventNetworkAllow EventType = "network.allow"
	EventNetworkDeny  EventType = "network.deny"

	// DNS events (spec: 1+ year retention).
	EventDNSQuery    EventType = "dns.query"
	EventDNSResponse EventType = "dns.response"

	// Tool invocation events (spec: 2+ year retention).
	EventToolInvoke  EventType = "tool.invoke"
	EventToolApprove EventType = "tool.approve"
	EventToolDeny    EventType = "tool.deny"

	// Credential events (spec: 2+ year retention).
	EventCredentialIssue  EventType = "credential.issue"
	EventCredentialUse    EventType = "credential.use"
	EventCredentialRotate EventType = "credential.rotate"
	EventCredentialRevoke EventType = "credential.revoke"

	// Policy decision events (spec: 2+ year retention).
	EventPolicyAllow EventType = "policy.allow"
	EventPolicyDeny  EventType = "policy.deny"

	// LLM API traffic events (spec: 1+ year retention).
	EventLLMRequest  EventType = "llm.request"
	EventLLMResponse EventType = "llm.response"

	// File access events (spec: 1+ year retention).
	EventFileRead  EventType = "file.read"
	EventFileWrite EventType = "file.write"

	// Falco runtime events.
	EventFalcoAlert EventType = "falco.alert"

	// Session recording events.
	EventSessionStart EventType = "session.start"
	EventSessionEnd   EventType = "session.end"
)

// Severity levels for audit events, ordered by severity.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Source identifies the component that generated the event.
type Source string

const (
	SourceCLI      Source = "aibox-cli"
	SourceAgent    Source = "aibox-agent"
	SourceProxy    Source = "aibox-llm-proxy"
	SourceSquid    Source = "squid"
	SourceCoreDNS  Source = "coredns"
	SourceOPA      Source = "opa"
	SourceVault    Source = "vault"
	SourceFalco    Source = "falco"
	SourceAuditd   Source = "auditd"
	SourceRecorder Source = "session-recorder"
)

// AuditEvent is the common event schema for all audit log entries.
// Every component emits events in this format. The HashPrev field
// links events into a tamper-evident hash chain (see Section 19.2).
type AuditEvent struct {
	Timestamp time.Time      `json:"timestamp"`
	EventType EventType      `json:"event_type"`
	SandboxID string         `json:"sandbox_id"`
	UserID    string         `json:"user_id"`
	Source    Source         `json:"source"`
	Severity  Severity       `json:"severity"`
	Details   map[string]any `json:"details,omitempty"`
	HashPrev  string         `json:"hash_prev"`
}

// MarshalJSON implements json.Marshaler with RFC 3339 timestamps.
func (e AuditEvent) MarshalJSON() ([]byte, error) {
	type Alias AuditEvent
	return json.Marshal(&struct {
		Timestamp string `json:"timestamp"`
		*Alias
	}{
		Timestamp: e.Timestamp.UTC().Format(time.RFC3339Nano),
		Alias:     (*Alias)(&e),
	})
}

// UnmarshalJSON implements json.Unmarshaler.
func (e *AuditEvent) UnmarshalJSON(data []byte) error {
	type Alias AuditEvent
	aux := &struct {
		Timestamp string `json:"timestamp"`
		*Alias
	}{
		Alias: (*Alias)(e),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	t, err := time.Parse(time.RFC3339Nano, aux.Timestamp)
	if err != nil {
		return err
	}
	e.Timestamp = t
	return nil
}

// Validate checks that all required fields are populated.
func (e *AuditEvent) Validate() error {
	if e.Timestamp.IsZero() {
		return ErrMissingTimestamp
	}
	if e.EventType == "" {
		return ErrMissingEventType
	}
	if e.SandboxID == "" {
		return ErrMissingSandboxID
	}
	if e.UserID == "" {
		return ErrMissingUserID
	}
	if e.Source == "" {
		return ErrMissingSource
	}
	if e.Severity == "" {
		return ErrMissingSeverity
	}
	return nil
}

// retentionYears maps event categories to their minimum retention in years.
var retentionYears = map[EventType]int{
	// 2+ years: lifecycle, tool, credential, policy.
	EventSandboxCreate:    2,
	EventSandboxStart:     2,
	EventSandboxStop:      2,
	EventSandboxDestroy:   2,
	EventSandboxConfig:    2,
	EventToolInvoke:       2,
	EventToolApprove:      2,
	EventToolDeny:         2,
	EventCredentialIssue:  2,
	EventCredentialUse:    2,
	EventCredentialRotate: 2,
	EventCredentialRevoke: 2,
	EventPolicyAllow:      2,
	EventPolicyDeny:       2,

	// 1+ year: network, DNS, LLM, file access.
	EventNetworkAllow: 1,
	EventNetworkDeny:  1,
	EventDNSQuery:     1,
	EventDNSResponse:  1,
	EventLLMRequest:   1,
	EventLLMResponse:  1,
	EventFileRead:     1,
	EventFileWrite:    1,

	// Falco and session: default 1 year.
	EventFalcoAlert:   1,
	EventSessionStart: 2,
	EventSessionEnd:   2,
}

// RetentionYears returns the minimum retention period for an event type.
// Returns 1 as the default for unknown event types.
func RetentionYears(et EventType) int {
	if y, ok := retentionYears[et]; ok {
		return y
	}
	return 1
}
