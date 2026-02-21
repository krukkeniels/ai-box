package credentials

import (
	"context"
	"testing"
	"time"
)

func TestFormatSPIFFEID(t *testing.T) {
	tests := []struct {
		name        string
		trustDomain string
		user        string
		workspace   string
		want        string
	}{
		{
			name:        "standard format",
			trustDomain: "aibox.org.internal",
			user:        "user1",
			workspace:   "my-project",
			want:        "spiffe://aibox.org.internal/sandbox/user1/my-project",
		},
		{
			name:        "different domain",
			trustDomain: "example.com",
			user:        "alice",
			workspace:   "dev-env",
			want:        "spiffe://example.com/sandbox/alice/dev-env",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSPIFFEID(tt.trustDomain, tt.user, tt.workspace)
			if got != tt.want {
				t.Errorf("FormatSPIFFEID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSPIFFEClientDefaults(t *testing.T) {
	client := NewSPIFFEClient(SPIFFEConfig{
		TrustDomain: "aibox.org.internal",
	})

	if client.config.SVIDTTLSeconds != 3600 {
		t.Errorf("default SVIDTTLSeconds = %d, want 3600", client.config.SVIDTTLSeconds)
	}
}

func TestSPIFFEClientIsAvailable_NoSocket(t *testing.T) {
	client := NewSPIFFEClient(SPIFFEConfig{
		TrustDomain: "aibox.org.internal",
		AgentSocket: "/nonexistent/spire/agent.sock",
	})

	if client.IsAvailable() {
		t.Error("IsAvailable() = true for nonexistent socket, want false")
	}
}

func TestSPIFFEClientIsAvailable_EmptySocket(t *testing.T) {
	client := NewSPIFFEClient(SPIFFEConfig{
		TrustDomain: "aibox.org.internal",
		AgentSocket: "",
	})

	if client.IsAvailable() {
		t.Error("IsAvailable() = true for empty socket path, want false")
	}
}

func TestSPIFFEClientGetIdentity_Stub(t *testing.T) {
	client := NewSPIFFEClient(SPIFFEConfig{
		TrustDomain:    "aibox.org.internal",
		AgentSocket:    "/nonexistent/agent.sock",
		SVIDTTLSeconds: 1800,
	})

	ctx := context.Background()
	before := time.Now()
	identity, err := client.GetIdentity(ctx, "testuser", "workspace1")
	after := time.Now()

	if err != nil {
		t.Fatalf("GetIdentity() error = %v", err)
	}

	expectedID := "spiffe://aibox.org.internal/sandbox/testuser/workspace1"
	if identity.SPIFFEID != expectedID {
		t.Errorf("SPIFFEID = %q, want %q", identity.SPIFFEID, expectedID)
	}

	if identity.User != "testuser" {
		t.Errorf("User = %q, want %q", identity.User, "testuser")
	}

	if identity.Workspace != "workspace1" {
		t.Errorf("Workspace = %q, want %q", identity.Workspace, "workspace1")
	}

	if identity.IssuedAt.Before(before) || identity.IssuedAt.After(after) {
		t.Errorf("IssuedAt = %v, want between %v and %v", identity.IssuedAt, before, after)
	}

	expectedExpiry := identity.IssuedAt.Add(1800 * time.Second)
	if identity.ExpiresAt.Before(expectedExpiry.Add(-time.Second)) || identity.ExpiresAt.After(expectedExpiry.Add(time.Second)) {
		t.Errorf("ExpiresAt = %v, want approximately %v", identity.ExpiresAt, expectedExpiry)
	}
}

func TestSPIFFEClientGetIdentity_CancelledContext(t *testing.T) {
	client := NewSPIFFEClient(SPIFFEConfig{
		TrustDomain: "aibox.org.internal",
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.GetIdentity(ctx, "user", "ws")
	if err == nil {
		t.Error("GetIdentity() with cancelled context should return error")
	}
}
