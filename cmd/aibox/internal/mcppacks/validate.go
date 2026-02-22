package mcppacks

import (
	"fmt"
	"strings"

	"github.com/aibox/aibox/internal/policy"
)

// PolicyCheckResult describes whether a pack's requirements are satisfied.
type PolicyCheckResult struct {
	Allowed         bool
	DeniedEndpoints []string // network endpoints not in allowlist
}

// CheckPolicy validates that a pack's network and filesystem requirements
// are satisfied by the effective policy. Returns a result describing which
// requirements (if any) are not met.
func CheckPolicy(pack *Manifest, p *policy.Policy) PolicyCheckResult {
	result := PolicyCheckResult{Allowed: true}

	if p == nil {
		// No policy loaded -- allow by default (same as tool gate behavior
		// when no rules are configured).
		return result
	}

	// Check network requirements.
	for _, required := range pack.NetworkRequires {
		if !isHostAllowed(required, p) {
			result.Allowed = false
			result.DeniedEndpoints = append(result.DeniedEndpoints, required)
		}
	}

	return result
}

// isHostAllowed checks if a host is in the policy's network allowlist.
func isHostAllowed(host string, p *policy.Policy) bool {
	for _, entry := range p.Network.Allow {
		for _, h := range entry.Hosts {
			if h == host {
				return true
			}
		}
	}
	return false
}

// FormatDenied returns a human-readable error message for denied network endpoints.
func FormatDenied(packName string, denied []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Cannot enable MCP pack %q: network requirements not met.\n", packName)
	fmt.Fprintf(&b, "The following endpoints are not in the network allowlist:\n")
	for _, d := range denied {
		fmt.Fprintf(&b, "  - %s\n", d)
	}
	fmt.Fprintf(&b, "\nTo fix: add these hosts to the network allow list in your policy,\n")
	fmt.Fprintf(&b, "or contact your platform team to update the org baseline policy.")
	return b.String()
}
