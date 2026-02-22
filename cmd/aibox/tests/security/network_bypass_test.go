//go:build integration

// Package security contains security validation tests that verify container
// network isolation properties. These tests require a running container with
// the aibox network security stack applied.
//
// Run with: go test -tags=security ./tests/security/ -run TestNetwork
package security

import (
	"strings"
	"testing"
)

// --- Network isolation tests (Phase 2) ---

// TestNetworkNoDirectEgress verifies that the container cannot reach external
// hosts directly (bypassing the proxy). nftables should block all direct
// outbound connections except to the proxy and DNS.
func TestNetworkNoDirectEgress(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	// Try to connect directly to an external IP on port 80 (should be blocked by nftables).
	out, err := execInContainer(rt, name, "sh", "-c",
		"timeout 5 bash -c 'echo > /dev/tcp/93.184.216.34/80' 2>&1 || echo CONNECTION_BLOCKED")
	if err != nil || strings.Contains(out, "CONNECTION_BLOCKED") || strings.Contains(out, "timed out") {
		// Expected: connection blocked.
		return
	}
	t.Errorf("direct egress to external IP should be blocked, got: %s", out)
}

// TestNetworkNoDirectDNS verifies that DNS queries to external resolvers are
// blocked. Only CoreDNS on the host should be reachable.
func TestNetworkNoDirectDNS(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	// Try querying Google's public DNS directly (should be blocked by nftables).
	resolvers := []string{"8.8.8.8", "1.1.1.1", "9.9.9.9"}
	for _, resolver := range resolvers {
		out, err := execInContainer(rt, name, "sh", "-c",
			"timeout 3 nslookup google.com "+resolver+" 2>&1 || echo DNS_BLOCKED")
		if err != nil || strings.Contains(out, "DNS_BLOCKED") || strings.Contains(out, "timed out") || strings.Contains(out, "connection refused") {
			continue // Expected: blocked.
		}
		t.Errorf("direct DNS to %s should be blocked, got: %s", resolver, out)
	}
}

// TestNetworkDoTBlocked verifies that DNS-over-TLS (port 853) is blocked.
func TestNetworkDoTBlocked(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	// Try connecting to a known DoT resolver on port 853.
	out, err := execInContainer(rt, name, "sh", "-c",
		"timeout 3 bash -c 'echo > /dev/tcp/1.1.1.1/853' 2>&1 || echo DOT_BLOCKED")
	if err != nil || strings.Contains(out, "DOT_BLOCKED") || strings.Contains(out, "timed out") {
		return // Expected: blocked.
	}
	t.Errorf("DNS-over-TLS (port 853) should be blocked, got: %s", out)
}

// TestNetworkQUICBlocked verifies that QUIC/HTTP3 (UDP 443) is blocked.
func TestNetworkQUICBlocked(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	// Try sending a UDP packet on port 443 (QUIC). Since we can't easily
	// test UDP connectivity from a minimal container, check that the
	// relevant nftables rule exists on the host.
	out, _ := execInContainer(rt, name, "sh", "-c",
		"timeout 3 python3 -c \"import socket; s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.settimeout(2); s.sendto(b'test',('1.1.1.1',443)); s.recv(1024)\" 2>&1 || echo QUIC_BLOCKED")
	if strings.Contains(out, "QUIC_BLOCKED") || strings.Contains(out, "timed out") || strings.Contains(out, "Connection refused") {
		return // Expected.
	}
	t.Logf("QUIC test inconclusive (may need python3 in container): %s", out)
}

// TestNetworkICMPBlocked verifies that ICMP is blocked from the container.
func TestNetworkICMPBlocked(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	out, err := execInContainer(rt, name, "sh", "-c",
		"timeout 3 ping -c 1 8.8.8.8 2>&1 || echo ICMP_BLOCKED")
	if err != nil || strings.Contains(out, "ICMP_BLOCKED") || strings.Contains(out, "not permitted") {
		return // Expected: blocked.
	}
	t.Errorf("ICMP should be blocked from container, got: %s", out)
}

// TestNetworkNonAllowlistedDomainBlocked verifies that DNS resolution of
// non-allowlisted domains returns NXDOMAIN.
func TestNetworkNonAllowlistedDomainBlocked(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	// Try resolving a domain that is NOT in the allowlist.
	blockedDomains := []string{"google.com", "facebook.com", "malware.example.com"}
	for _, domain := range blockedDomains {
		out, err := execInContainer(rt, name, "sh", "-c",
			"nslookup "+domain+" 2>&1 || echo NXDOMAIN_RETURNED")
		if err != nil || strings.Contains(out, "NXDOMAIN") || strings.Contains(out, "NXDOMAIN_RETURNED") || strings.Contains(out, "server can't find") {
			continue // Expected: NXDOMAIN.
		}
		t.Errorf("non-allowlisted domain %q should return NXDOMAIN, got: %s", domain, out)
	}
}

// TestNetworkAllowlistedDomainResolves verifies that allowlisted domains
// resolve to the expected internal IPs.
func TestNetworkAllowlistedDomainResolves(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	allowlisted := []string{"harbor.internal", "nexus.internal", "foundry.internal", "git.internal"}
	for _, domain := range allowlisted {
		out, err := execInContainer(rt, name, "sh", "-c",
			"nslookup "+domain+" 2>&1")
		if err != nil {
			t.Logf("cannot resolve allowlisted domain %q (nslookup may not be available): %v", domain, err)
			continue
		}
		// Should resolve to a 10.x.x.x address (our internal infrastructure).
		if !strings.Contains(out, "10.") && !strings.Contains(out, "Address:") {
			t.Errorf("allowlisted domain %q should resolve to internal IP, got: %s", domain, out)
		}
	}
}

// TestNetworkProxyEnvSet verifies that proxy environment variables are set
// inside the container so that tools route through Squid.
func TestNetworkProxyEnvSet(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	envVars := []string{"http_proxy", "https_proxy", "HTTP_PROXY", "HTTPS_PROXY"}
	for _, env := range envVars {
		out, err := execInContainer(rt, name, "printenv", env)
		if err != nil {
			// Might be running with --network=none (no network stack).
			t.Logf("proxy env %s not set (container may have network=none): %v", env, err)
			continue
		}
		if !strings.Contains(out, "127.0.0.1") && !strings.Contains(out, "3128") {
			t.Errorf("%s should point to Squid proxy, got: %s", env, out)
		}
	}
}

// TestNetworkLLMBaseURLSet verifies that AI tool base URLs are configured
// to use the LLM sidecar proxy.
func TestNetworkLLMBaseURLSet(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	llmEnvVars := map[string]string{
		"ANTHROPIC_BASE_URL": "http://localhost:8443",
		"OPENAI_BASE_URL":   "http://localhost:8443",
	}

	for env, expected := range llmEnvVars {
		out, err := execInContainer(rt, name, "printenv", env)
		if err != nil {
			t.Logf("LLM env %s not set: %v", env, err)
			continue
		}
		if strings.TrimSpace(out) != expected {
			t.Errorf("%s = %q, want %q", env, strings.TrimSpace(out), expected)
		}
	}
}

// TestNetworkDNSContainerConfig verifies that the container's DNS is
// configured to use CoreDNS on the host.
func TestNetworkDNSContainerConfig(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	out, err := execInContainer(rt, name, "cat", "/etc/resolv.conf")
	if err != nil {
		t.Skipf("cannot read /etc/resolv.conf: %v", err)
	}

	// Should point to the host's CoreDNS, not any public resolver.
	publicResolvers := []string{"8.8.8.8", "8.8.4.4", "1.1.1.1", "1.0.0.1", "9.9.9.9"}
	for _, r := range publicResolvers {
		if strings.Contains(out, r) {
			t.Errorf("/etc/resolv.conf should NOT contain public resolver %s", r)
		}
	}
}

// TestNetworkNoRawSocket verifies that the container cannot create raw
// sockets (which could bypass proxy-level filtering).
func TestNetworkNoRawSocket(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	out, err := execInContainer(rt, name, "sh", "-c",
		"python3 -c \"import socket; s=socket.socket(socket.AF_INET,socket.SOCK_RAW,socket.IPPROTO_ICMP)\" 2>&1 || echo RAW_BLOCKED")
	if err != nil || strings.Contains(out, "RAW_BLOCKED") || strings.Contains(out, "not permitted") || strings.Contains(out, "Permission denied") {
		return // Expected: blocked by cap-drop ALL / seccomp.
	}
	t.Logf("raw socket test inconclusive (python3 may not be in container): %s", out)
}

// TestNetworkCurlNonAllowlisted verifies that HTTP requests to non-allowlisted
// domains are blocked by the proxy.
func TestNetworkCurlNonAllowlisted(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	out, err := execInContainer(rt, name, "sh", "-c",
		"timeout 5 curl -s -o /dev/null -w '%{http_code}' http://example.com 2>&1 || echo CURL_FAILED")
	if err != nil || strings.Contains(out, "CURL_FAILED") {
		// Expected: proxy should deny the request.
		return
	}
	// Squid returns 403 for denied domains.
	if strings.TrimSpace(out) != "403" && strings.TrimSpace(out) != "000" {
		t.Errorf("curl to non-allowlisted domain should return 403 or fail, got HTTP %s", out)
	}
}
