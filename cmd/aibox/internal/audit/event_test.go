package audit

import (
	"encoding/json"
	"testing"
	"time"
)

func validEvent() AuditEvent {
	return AuditEvent{
		Timestamp: time.Date(2026, 2, 21, 10, 30, 0, 0, time.UTC),
		EventType: EventSandboxCreate,
		SandboxID: "aibox-dev1-abc123",
		UserID:    "dev1",
		Source:    SourceCLI,
		Severity:  SeverityInfo,
		Details: map[string]any{
			"image": "aibox-base:latest",
		},
		HashPrev: GenesisHash,
	}
}

func TestAuditEventValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*AuditEvent)
		wantErr error
	}{
		{
			name:    "valid event",
			modify:  func(_ *AuditEvent) {},
			wantErr: nil,
		},
		{
			name:    "missing timestamp",
			modify:  func(e *AuditEvent) { e.Timestamp = time.Time{} },
			wantErr: ErrMissingTimestamp,
		},
		{
			name:    "missing event type",
			modify:  func(e *AuditEvent) { e.EventType = "" },
			wantErr: ErrMissingEventType,
		},
		{
			name:    "missing sandbox ID",
			modify:  func(e *AuditEvent) { e.SandboxID = "" },
			wantErr: ErrMissingSandboxID,
		},
		{
			name:    "missing user ID",
			modify:  func(e *AuditEvent) { e.UserID = "" },
			wantErr: ErrMissingUserID,
		},
		{
			name:    "missing source",
			modify:  func(e *AuditEvent) { e.Source = "" },
			wantErr: ErrMissingSource,
		},
		{
			name:    "missing severity",
			modify:  func(e *AuditEvent) { e.Severity = "" },
			wantErr: ErrMissingSeverity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := validEvent()
			tt.modify(&e)
			err := e.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAuditEventJSONRoundTrip(t *testing.T) {
	original := validEvent()

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded AuditEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !decoded.Timestamp.Equal(original.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", decoded.Timestamp, original.Timestamp)
	}
	if decoded.EventType != original.EventType {
		t.Errorf("EventType = %v, want %v", decoded.EventType, original.EventType)
	}
	if decoded.SandboxID != original.SandboxID {
		t.Errorf("SandboxID = %v, want %v", decoded.SandboxID, original.SandboxID)
	}
	if decoded.UserID != original.UserID {
		t.Errorf("UserID = %v, want %v", decoded.UserID, original.UserID)
	}
	if decoded.Source != original.Source {
		t.Errorf("Source = %v, want %v", decoded.Source, original.Source)
	}
	if decoded.Severity != original.Severity {
		t.Errorf("Severity = %v, want %v", decoded.Severity, original.Severity)
	}
	if decoded.HashPrev != original.HashPrev {
		t.Errorf("HashPrev = %v, want %v", decoded.HashPrev, original.HashPrev)
	}
}

func TestAuditEventJSONFields(t *testing.T) {
	e := validEvent()
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}

	requiredFields := []string{
		"timestamp", "event_type", "sandbox_id", "user_id",
		"source", "severity", "hash_prev",
	}
	for _, f := range requiredFields {
		if _, ok := parsed[f]; !ok {
			t.Errorf("missing required JSON field %q", f)
		}
	}
}

func TestAuditEventJSONTimestampFormat(t *testing.T) {
	e := validEvent()
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	ts, ok := parsed["timestamp"].(string)
	if !ok {
		t.Fatal("timestamp is not a string")
	}

	// Verify it parses as RFC 3339.
	if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
		t.Errorf("timestamp %q is not valid RFC 3339: %v", ts, err)
	}
}

func TestAuditEventDetailsOmittedWhenNil(t *testing.T) {
	e := validEvent()
	e.Details = nil

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if _, ok := parsed["details"]; ok {
		t.Error("expected details to be omitted when nil")
	}
}

func TestRetentionYears(t *testing.T) {
	tests := []struct {
		eventType EventType
		want      int
	}{
		{EventSandboxCreate, 2},
		{EventSandboxStart, 2},
		{EventSandboxStop, 2},
		{EventSandboxDestroy, 2},
		{EventToolInvoke, 2},
		{EventCredentialIssue, 2},
		{EventPolicyAllow, 2},
		{EventNetworkAllow, 1},
		{EventDNSQuery, 1},
		{EventLLMRequest, 1},
		{EventFileRead, 1},
		{EventFalcoAlert, 1},
		{EventType("unknown.type"), 1}, // default
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			got := RetentionYears(tt.eventType)
			if got != tt.want {
				t.Errorf("RetentionYears(%q) = %d, want %d", tt.eventType, got, tt.want)
			}
		})
	}
}

func TestEventTypeConstants(t *testing.T) {
	// Verify all event types are unique.
	allTypes := []EventType{
		EventSandboxCreate, EventSandboxStart, EventSandboxStop,
		EventSandboxDestroy, EventSandboxConfig,
		EventNetworkAllow, EventNetworkDeny,
		EventDNSQuery, EventDNSResponse,
		EventToolInvoke, EventToolApprove, EventToolDeny,
		EventCredentialIssue, EventCredentialUse,
		EventCredentialRotate, EventCredentialRevoke,
		EventPolicyAllow, EventPolicyDeny,
		EventLLMRequest, EventLLMResponse,
		EventFileRead, EventFileWrite,
		EventFalcoAlert,
		EventSessionStart, EventSessionEnd,
	}

	seen := make(map[EventType]bool)
	for _, et := range allTypes {
		if seen[et] {
			t.Errorf("duplicate event type: %q", et)
		}
		seen[et] = true
	}
}

func TestSeverityConstants(t *testing.T) {
	severities := []Severity{
		SeverityInfo, SeverityWarning, SeverityHigh, SeverityCritical,
	}

	seen := make(map[Severity]bool)
	for _, s := range severities {
		if seen[s] {
			t.Errorf("duplicate severity: %q", s)
		}
		seen[s] = true
	}
}

func TestSourceConstants(t *testing.T) {
	sources := []Source{
		SourceCLI, SourceAgent, SourceProxy, SourceSquid,
		SourceCoreDNS, SourceOPA, SourceVault, SourceFalco,
		SourceAuditd, SourceRecorder,
	}

	seen := make(map[Source]bool)
	for _, s := range sources {
		if seen[s] {
			t.Errorf("duplicate source: %q", s)
		}
		seen[s] = true
	}
}
