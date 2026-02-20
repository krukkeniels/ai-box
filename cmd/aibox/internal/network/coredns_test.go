package network

import (
	"strings"
	"testing"
)

func TestDefaultCoreDNSConfig(t *testing.T) {
	cfg := DefaultCoreDNSConfig()

	if cfg.ListenAddr != "127.0.0.1" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "127.0.0.1")
	}
	if cfg.ListenPort != 53 {
		t.Errorf("ListenPort = %d, want %d", cfg.ListenPort, 53)
	}
	if len(cfg.Entries) == 0 {
		t.Error("Entries should have defaults")
	}
}

func TestDefaultDomainEntries_HasRequiredDomains(t *testing.T) {
	entries := DefaultDomainEntries()

	required := []string{"harbor.internal", "nexus.internal", "foundry.internal", "git.internal"}
	domains := make(map[string]bool)
	for _, e := range entries {
		domains[e.Domain] = true
	}

	for _, r := range required {
		if !domains[r] {
			t.Errorf("DefaultDomainEntries missing required domain %q", r)
		}
	}
}

func TestDefaultDomainEntries_ValidIPs(t *testing.T) {
	entries := DefaultDomainEntries()

	for _, e := range entries {
		if e.IP == "" {
			t.Errorf("domain %q has empty IP", e.Domain)
		}
		// Simple check: should contain dots (IPv4).
		if !strings.Contains(e.IP, ".") {
			t.Errorf("domain %q has invalid IP %q", e.Domain, e.IP)
		}
	}
}

func TestGenerateCorefile_ContainsHostEntries(t *testing.T) {
	mgr := NewCoreDNSManager(DefaultCoreDNSConfig())
	corefile := mgr.GenerateCorefile()

	for _, entry := range DefaultDomainEntries() {
		expected := entry.IP + " " + entry.Domain
		if !strings.Contains(corefile, expected) {
			t.Errorf("Corefile missing host entry %q", expected)
		}
	}
}

func TestGenerateCorefile_NXDOMAINCatchAll(t *testing.T) {
	mgr := NewCoreDNSManager(DefaultCoreDNSConfig())
	corefile := mgr.GenerateCorefile()

	// Must contain NXDOMAIN catch-all template for ANY record type.
	if !strings.Contains(corefile, "template IN ANY .") {
		t.Error("Corefile must contain catch-all ANY template for NXDOMAIN")
	}
	if !strings.Contains(corefile, "rcode NXDOMAIN") {
		t.Error("Corefile must return NXDOMAIN for non-allowlisted domains")
	}
}

func TestGenerateCorefile_BlocksDNSTunneling(t *testing.T) {
	mgr := NewCoreDNSManager(DefaultCoreDNSConfig())
	corefile := mgr.GenerateCorefile()

	// TXT records can carry exfiltrated data.
	if !strings.Contains(corefile, "template IN TXT .") {
		t.Error("Corefile must block TXT records for non-allowlisted domains (DNS tunneling)")
	}

	// NULL records can be used for tunneling.
	if !strings.Contains(corefile, "template IN NULL .") {
		t.Error("Corefile must block NULL records for non-allowlisted domains (DNS tunneling)")
	}
}

func TestGenerateCorefile_HasHealthEndpoint(t *testing.T) {
	mgr := NewCoreDNSManager(DefaultCoreDNSConfig())
	corefile := mgr.GenerateCorefile()

	if !strings.Contains(corefile, "health") {
		t.Error("Corefile must include health check plugin")
	}
}

func TestGenerateCorefile_HasLogging(t *testing.T) {
	mgr := NewCoreDNSManager(DefaultCoreDNSConfig())
	corefile := mgr.GenerateCorefile()

	if !strings.Contains(corefile, "log") {
		t.Error("Corefile must include logging")
	}
	if !strings.Contains(corefile, "errors") {
		t.Error("Corefile must include error logging")
	}
}

func TestGenerateCorefile_HasMetrics(t *testing.T) {
	mgr := NewCoreDNSManager(DefaultCoreDNSConfig())
	corefile := mgr.GenerateCorefile()

	if !strings.Contains(corefile, "prometheus") {
		t.Error("Corefile must include Prometheus metrics")
	}
}

func TestGenerateCorefile_BindAddress(t *testing.T) {
	cfg := DefaultCoreDNSConfig()
	cfg.ListenAddr = "10.0.0.1"
	cfg.ListenPort = 5353
	mgr := NewCoreDNSManager(cfg)
	corefile := mgr.GenerateCorefile()

	if !strings.Contains(corefile, "10.0.0.1:5353") {
		t.Error("Corefile should bind to custom address")
	}
}

func TestGenerateCorefile_HostsFallthrough(t *testing.T) {
	mgr := NewCoreDNSManager(DefaultCoreDNSConfig())
	corefile := mgr.GenerateCorefile()

	// The hosts block should have fallthrough so non-matching queries
	// proceed to the catch-all NXDOMAIN templates.
	if !strings.Contains(corefile, "fallthrough") {
		t.Error("Corefile hosts block must have fallthrough")
	}
}

func TestAllowlistedDomains(t *testing.T) {
	mgr := NewCoreDNSManager(DefaultCoreDNSConfig())
	domains := mgr.AllowlistedDomains()

	if len(domains) != len(DefaultDomainEntries()) {
		t.Errorf("AllowlistedDomains count = %d, want %d", len(domains), len(DefaultDomainEntries()))
	}

	for _, d := range domains {
		if d == "" {
			t.Error("AllowlistedDomains should not contain empty strings")
		}
	}
}

func TestGenerateCorefile_NoExternalUpstream(t *testing.T) {
	mgr := NewCoreDNSManager(DefaultCoreDNSConfig())
	corefile := mgr.GenerateCorefile()

	// CoreDNS should NOT forward to external resolvers.
	publicDNS := []string{"8.8.8.8", "1.1.1.1", "9.9.9.9"}
	for _, dns := range publicDNS {
		if strings.Contains(corefile, dns) {
			t.Errorf("Corefile should NOT reference public DNS %q", dns)
		}
	}
}
