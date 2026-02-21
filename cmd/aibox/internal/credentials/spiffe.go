package credentials

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"
)

// SPIFFEConfig holds SPIFFE/SPIRE configuration.
type SPIFFEConfig struct {
	TrustDomain    string // e.g., "aibox.org.internal"
	AgentSocket    string // SPIRE Agent socket path, e.g., "/run/spire/sockets/agent.sock"
	SVIDTTLSeconds int    // SVID TTL (default: 3600 = 1 hour)
}

// WorkloadIdentity represents a SPIFFE workload identity.
type WorkloadIdentity struct {
	SPIFFEID  string    // e.g., "spiffe://aibox.org.internal/sandbox/user1/my-project"
	User      string
	Workspace string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// FormatSPIFFEID generates a SPIFFE ID for a sandbox workload.
func FormatSPIFFEID(trustDomain, user, workspace string) string {
	return fmt.Sprintf("spiffe://%s/sandbox/%s/%s", trustDomain, user, workspace)
}

// SPIFFEClient interfaces with the SPIRE Agent to obtain SVIDs.
// This is a stub implementation — actual SPIRE workload API integration
// requires the SPIRE SDK, which will be added when SPIRE is deployed.
type SPIFFEClient struct {
	config SPIFFEConfig
	logger *slog.Logger
}

// NewSPIFFEClient creates a new SPIFFE client with the given configuration.
func NewSPIFFEClient(cfg SPIFFEConfig) *SPIFFEClient {
	if cfg.SVIDTTLSeconds <= 0 {
		cfg.SVIDTTLSeconds = 3600
	}
	return &SPIFFEClient{
		config: cfg,
		logger: slog.Default(),
	}
}

// GetIdentity obtains workload identity from SPIRE Agent.
// Returns a stub identity if SPIRE Agent is not available (graceful degradation).
func (c *SPIFFEClient) GetIdentity(ctx context.Context, user, workspace string) (*WorkloadIdentity, error) {
	if ctx.Err() != nil {
		return nil, fmt.Errorf("getting SPIFFE identity: %w", ctx.Err())
	}

	spiffeID := FormatSPIFFEID(c.config.TrustDomain, user, workspace)
	now := time.Now()
	ttl := time.Duration(c.config.SVIDTTLSeconds) * time.Second

	if !c.IsAvailable() {
		c.logger.Warn("SPIRE agent not available, returning stub identity",
			"spiffe_id", spiffeID,
			"socket", c.config.AgentSocket,
		)
	}

	// Stub: return a synthetic identity. When SPIRE is deployed, this will
	// call the SPIRE Workload API over the agent socket to fetch a real SVID.
	return &WorkloadIdentity{
		SPIFFEID:  spiffeID,
		User:      user,
		Workspace: workspace,
		IssuedAt:  now,
		ExpiresAt: now.Add(ttl),
	}, nil
}

// IsAvailable checks if the SPIRE Agent socket is reachable.
func (c *SPIFFEClient) IsAvailable() bool {
	if c.config.AgentSocket == "" {
		return false
	}

	conn, err := net.DialTimeout("unix", c.config.AgentSocket, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
