package network

import (
	"strings"
	"testing"
)

func TestDefaultSquidConfig(t *testing.T) {
	cfg := DefaultSquidConfig()

	if cfg.ListenAddr != "127.0.0.1" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "127.0.0.1")
	}
	if cfg.ListenPort != 3128 {
		t.Errorf("ListenPort = %d, want %d", cfg.ListenPort, 3128)
	}
	if len(cfg.AllowedDomains) == 0 {
		t.Error("AllowedDomains should have defaults")
	}
}

func TestNewSquidManager_DefaultsFilled(t *testing.T) {
	mgr := NewSquidManager(SquidConfig{})

	if mgr.cfg.ListenAddr != "127.0.0.1" {
		t.Errorf("ListenAddr = %q, want default", mgr.cfg.ListenAddr)
	}
	if mgr.cfg.ListenPort != 3128 {
		t.Errorf("ListenPort = %d, want default", mgr.cfg.ListenPort)
	}
	if mgr.cfg.LogPath == "" {
		t.Error("LogPath should have default")
	}
}

func TestGenerateConfig_DefaultDeny(t *testing.T) {
	mgr := NewSquidManager(DefaultSquidConfig())
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "http_access deny all") {
		t.Error("config must contain default-deny rule 'http_access deny all'")
	}
}

func TestGenerateConfig_AllowedDomainsACL(t *testing.T) {
	mgr := NewSquidManager(SquidConfig{
		AllowedDomains: []string{"harbor.internal", "nexus.internal"},
	})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "acl aibox_allowed dstdomain .harbor.internal") {
		t.Error("config should include harbor.internal in allowlist ACL")
	}
	if !strings.Contains(config, "acl aibox_allowed dstdomain .nexus.internal") {
		t.Error("config should include nexus.internal in allowlist ACL")
	}
}

func TestGenerateConfig_SubdomainMatching(t *testing.T) {
	mgr := NewSquidManager(SquidConfig{
		AllowedDomains: []string{"example.com"},
	})
	config := mgr.GenerateConfig()

	// Domains should have a leading dot for subdomain matching.
	if !strings.Contains(config, ".example.com") {
		t.Error("domain ACL should use leading dot for subdomain matching")
	}
}

func TestGenerateConfig_SafePorts(t *testing.T) {
	mgr := NewSquidManager(DefaultSquidConfig())
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "acl Safe_ports port 80") {
		t.Error("config should allow port 80")
	}
	if !strings.Contains(config, "acl Safe_ports port 443") {
		t.Error("config should allow port 443")
	}
	if !strings.Contains(config, "http_access deny !Safe_ports") {
		t.Error("config must deny non-safe ports")
	}
}

func TestGenerateConfig_CONNECTRestrictions(t *testing.T) {
	mgr := NewSquidManager(DefaultSquidConfig())
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "http_access deny CONNECT !SSL_ports") {
		t.Error("config must restrict CONNECT to SSL ports only")
	}
	if !strings.Contains(config, "http_access allow CONNECT aibox_allowed") {
		t.Error("config must allow CONNECT only to allowlisted domains")
	}
}

func TestGenerateConfig_SNIPeekAndSplice(t *testing.T) {
	mgr := NewSquidManager(DefaultSquidConfig())
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "ssl_bump peek all") {
		t.Error("config must use ssl_bump peek for SNI inspection")
	}
	if !strings.Contains(config, "ssl_bump splice all") {
		t.Error("config must use ssl_bump splice (NO MITM)")
	}
}

func TestGenerateConfig_NoMITM(t *testing.T) {
	mgr := NewSquidManager(DefaultSquidConfig())
	config := mgr.GenerateConfig()

	// Config should NOT contain ssl_bump stare/bump (those indicate MITM).
	if strings.Contains(config, "ssl_bump stare") {
		t.Error("config must NOT use ssl_bump stare (that's MITM)")
	}
	if strings.Contains(config, "ssl_bump bump") {
		t.Error("config must NOT use ssl_bump bump (that's MITM)")
	}
}

func TestGenerateConfig_CachingDisabled(t *testing.T) {
	mgr := NewSquidManager(DefaultSquidConfig())
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "cache deny all") {
		t.Error("config must disable caching")
	}
}

func TestGenerateConfig_ListenAddress(t *testing.T) {
	mgr := NewSquidManager(SquidConfig{
		ListenAddr: "10.0.0.1",
		ListenPort: 8080,
	})
	config := mgr.GenerateConfig()

	if !strings.Contains(config, "http_port 10.0.0.1:8080") {
		t.Error("config should bind to custom listen address")
	}
}

func TestGenerateConfig_AccessRuleOrdering(t *testing.T) {
	mgr := NewSquidManager(DefaultSquidConfig())
	config := mgr.GenerateConfig()

	// The deny all rule MUST come AFTER the allow rules.
	allowIdx := strings.Index(config, "http_access allow aibox_allowed")
	denyAllIdx := strings.Index(config, "http_access deny all")

	if allowIdx < 0 || denyAllIdx < 0 {
		t.Fatal("config missing required access rules")
	}
	if denyAllIdx < allowIdx {
		t.Error("deny all rule must come AFTER allow rules (order matters in Squid)")
	}
}

func TestGenerateConfig_NoArbitraryExternalDomains(t *testing.T) {
	mgr := NewSquidManager(SquidConfig{
		AllowedDomains: []string{"harbor.internal"},
	})
	config := mgr.GenerateConfig()

	// Should NOT contain any public domain allowlist entries.
	publicDomains := []string{"google.com", "github.com", "amazonaws.com", "azure.com"}
	for _, domain := range publicDomains {
		if strings.Contains(config, domain) {
			t.Errorf("config should NOT contain public domain %q", domain)
		}
	}
}

func TestDefaultAllowedDomains_InternalOnly(t *testing.T) {
	for _, domain := range DefaultAllowedDomains {
		if !strings.HasSuffix(domain, ".internal") {
			t.Errorf("default domain %q should be internal-only (suffix .internal)", domain)
		}
	}
}
