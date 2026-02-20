package network

import (
	"strings"
	"testing"
)

func TestDefaultNFTablesConfig(t *testing.T) {
	cfg := DefaultNFTablesConfig()

	if cfg.ProxyIP != "127.0.0.1" {
		t.Errorf("ProxyIP = %q, want %q", cfg.ProxyIP, "127.0.0.1")
	}
	if cfg.ProxyPort != 3128 {
		t.Errorf("ProxyPort = %d, want %d", cfg.ProxyPort, 3128)
	}
	if cfg.DNSIP != "127.0.0.1" {
		t.Errorf("DNSIP = %q, want %q", cfg.DNSIP, "127.0.0.1")
	}
	if cfg.DNSPort != 53 {
		t.Errorf("DNSPort = %d, want %d", cfg.DNSPort, 53)
	}
	if len(cfg.Interfaces) != 2 {
		t.Errorf("Interfaces count = %d, want 2", len(cfg.Interfaces))
	}
}

func TestNewNFTablesManager_DefaultsFilled(t *testing.T) {
	// Zero-value config should get defaults.
	mgr := NewNFTablesManager(NFTablesConfig{})

	if mgr.cfg.ProxyIP != "127.0.0.1" {
		t.Errorf("ProxyIP = %q, want default", mgr.cfg.ProxyIP)
	}
	if mgr.cfg.ProxyPort != 3128 {
		t.Errorf("ProxyPort = %d, want default", mgr.cfg.ProxyPort)
	}
	if mgr.cfg.LogPrefix != "aibox-drop" {
		t.Errorf("LogPrefix = %q, want default", mgr.cfg.LogPrefix)
	}
}

func TestNewNFTablesManager_CustomConfig(t *testing.T) {
	mgr := NewNFTablesManager(NFTablesConfig{
		ProxyIP:   "10.0.0.1",
		ProxyPort: 8080,
		DNSIP:     "10.0.0.2",
		DNSPort:   5353,
	})

	if mgr.cfg.ProxyIP != "10.0.0.1" {
		t.Errorf("ProxyIP = %q, want custom", mgr.cfg.ProxyIP)
	}
	if mgr.cfg.ProxyPort != 8080 {
		t.Errorf("ProxyPort = %d, want custom", mgr.cfg.ProxyPort)
	}
}

func TestGenerateRuleset_ContainsRequiredRules(t *testing.T) {
	mgr := NewNFTablesManager(DefaultNFTablesConfig())
	ruleset := mgr.GenerateRuleset()

	if ruleset == "" {
		t.Fatal("GenerateRuleset() returned empty string")
	}

	required := []struct {
		needle string
		desc   string
	}{
		{"table inet aibox", "nftables table declaration"},
		{"chain forward", "forward chain"},
		{"policy drop", "default-deny policy"},
		{"ct state established,related accept", "stateful return traffic"},
		{"tcp dport 3128 accept", "proxy accept rule"},
		{"udp dport 53 accept", "DNS accept rule"},
		{"udp dport 53 drop", "unauthorized DNS block"},
		{"tcp dport 53 drop", "unauthorized DNS-over-TCP block"},
		{"tcp dport 853 drop", "DNS-over-TLS block"},
		{"udp dport 443 drop", "QUIC/HTTP3 block"},
		{"ip protocol icmp drop", "ICMP block"},
		{"flush table inet aibox", "idempotent flush"},
	}

	for _, r := range required {
		if !strings.Contains(ruleset, r.needle) {
			t.Errorf("ruleset missing %s (%q)", r.desc, r.needle)
		}
	}
}

func TestGenerateRuleset_BlocksDoHResolvers(t *testing.T) {
	mgr := NewNFTablesManager(DefaultNFTablesConfig())
	ruleset := mgr.GenerateRuleset()

	for _, ip := range DoHResolverIPs {
		needle := "ip daddr " + ip + " tcp dport 443 drop"
		if !strings.Contains(ruleset, needle) {
			t.Errorf("ruleset missing DoH block for %s", ip)
		}
	}
}

func TestGenerateRuleset_ContainerInterfaces(t *testing.T) {
	mgr := NewNFTablesManager(DefaultNFTablesConfig())
	ruleset := mgr.GenerateRuleset()

	// Should reference both podman and pasta interfaces.
	if !strings.Contains(ruleset, "podman") {
		t.Error("ruleset should reference podman interface pattern")
	}
	if !strings.Contains(ruleset, "pasta") {
		t.Error("ruleset should reference pasta interface pattern")
	}
}

func TestGenerateRuleset_NoDirectEgress(t *testing.T) {
	mgr := NewNFTablesManager(DefaultNFTablesConfig())
	ruleset := mgr.GenerateRuleset()

	// The ruleset should NOT contain any "accept" rules for arbitrary
	// ports (only proxy and DNS should be accepted).
	lines := strings.Split(ruleset, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "accept") {
			// Only three accept rules should exist:
			// 1. ct state established,related accept
			// 2. tcp dport <proxy> accept
			// 3. udp dport <dns> accept
			validAccepts := []string{
				"ct state established,related accept",
				"tcp dport 3128 accept",
				"udp dport 53 accept",
			}
			isValid := false
			for _, valid := range validAccepts {
				if strings.Contains(trimmed, valid) {
					isValid = true
					break
				}
			}
			if !isValid {
				t.Errorf("unexpected accept rule: %q", trimmed)
			}
		}
	}
}

func TestGenerateRuleset_CustomPorts(t *testing.T) {
	mgr := NewNFTablesManager(NFTablesConfig{
		ProxyPort: 9999,
		DNSPort:   5353,
	})
	ruleset := mgr.GenerateRuleset()

	if !strings.Contains(ruleset, "tcp dport 9999 accept") {
		t.Error("ruleset should use custom proxy port 9999")
	}
	if !strings.Contains(ruleset, "udp dport 5353 accept") {
		t.Error("ruleset should use custom DNS port 5353")
	}
}

func TestGenerateRuleset_LogRateLimit(t *testing.T) {
	mgr := NewNFTablesManager(DefaultNFTablesConfig())
	ruleset := mgr.GenerateRuleset()

	if !strings.Contains(ruleset, "limit rate 5/minute") {
		t.Error("ruleset should include rate-limited logging")
	}
	if !strings.Contains(ruleset, `log prefix "aibox-drop: "`) {
		t.Error("ruleset should include log prefix")
	}
}

func TestInterfaceMatch_Single(t *testing.T) {
	result := interfaceMatch([]string{"podman*"})
	if result != `iifname "podman*"` {
		t.Errorf("interfaceMatch single = %q, want single iifname", result)
	}
}

func TestInterfaceMatch_Multiple(t *testing.T) {
	result := interfaceMatch([]string{"podman*", "pasta*"})
	if !strings.Contains(result, "iifname {") {
		t.Errorf("interfaceMatch multiple should use set syntax, got %q", result)
	}
	if !strings.Contains(result, `"podman*"`) || !strings.Contains(result, `"pasta*"`) {
		t.Errorf("interfaceMatch should contain both interfaces, got %q", result)
	}
}

func TestDoHResolverIPs_NotEmpty(t *testing.T) {
	if len(DoHResolverIPs) < 6 {
		t.Errorf("DoHResolverIPs should contain at least 6 IPs (Google, Cloudflare, Quad9), got %d", len(DoHResolverIPs))
	}
}

func TestDoHResolverIPs_KnownProviders(t *testing.T) {
	required := map[string]string{
		"8.8.8.8":   "Google",
		"1.1.1.1":   "Cloudflare",
		"9.9.9.9":   "Quad9",
		"208.67.222.222": "OpenDNS",
	}

	ips := make(map[string]bool)
	for _, ip := range DoHResolverIPs {
		ips[ip] = true
	}

	for ip, provider := range required {
		if !ips[ip] {
			t.Errorf("DoHResolverIPs missing %s (%s)", provider, ip)
		}
	}
}
