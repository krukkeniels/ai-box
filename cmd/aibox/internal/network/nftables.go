package network

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

// DoHResolverIPs contains known DNS-over-HTTPS resolver IPs to block.
// This is defense-in-depth against DNS bypass via HTTPS.
var DoHResolverIPs = []string{
	"8.8.8.8", "8.8.4.4", // Google
	"1.1.1.1", "1.0.0.1", // Cloudflare
	"9.9.9.9", "149.112.112.112", // Quad9
	"208.67.222.222", "208.67.220.220", // OpenDNS
	"185.228.168.168", "185.228.169.168", // CleanBrowsing
}

// NFTablesConfig holds the configuration for nftables rule generation.
type NFTablesConfig struct {
	ProxyIP    string   // IP of Squid proxy (default "127.0.0.1")
	ProxyPort  int      // Port of Squid proxy (default 3128)
	DNSIP      string   // IP of CoreDNS (default "127.0.0.1")
	DNSPort    int      // Port of CoreDNS (default 53)
	Interfaces []string // Container interface patterns (default ["podman*", "pasta*"])
	LogPrefix  string   // nftables log prefix (default "aibox-drop")
	LogRate    string   // Log rate limit (default "5/minute")
}

// NFTablesManager generates and manages nftables rules for AI-Box network isolation.
type NFTablesManager struct {
	cfg NFTablesConfig
}

// DefaultNFTablesConfig returns an NFTablesConfig with sensible defaults.
// Includes both podman* (rootful/bridged) and pasta* (rootless) interface patterns.
func DefaultNFTablesConfig() NFTablesConfig {
	return NFTablesConfig{
		ProxyIP:    "127.0.0.1",
		ProxyPort:  3128,
		DNSIP:      "127.0.0.1",
		DNSPort:    53,
		Interfaces: []string{"podman*", "pasta*"},
		LogPrefix:  "aibox-drop",
		LogRate:    "5/minute",
	}
}

// NewNFTablesManager creates an NFTablesManager with the given config.
// Zero-value fields in cfg are filled with defaults.
func NewNFTablesManager(cfg NFTablesConfig) *NFTablesManager {
	defaults := DefaultNFTablesConfig()
	if cfg.ProxyIP == "" {
		cfg.ProxyIP = defaults.ProxyIP
	}
	if cfg.ProxyPort == 0 {
		cfg.ProxyPort = defaults.ProxyPort
	}
	if cfg.DNSIP == "" {
		cfg.DNSIP = defaults.DNSIP
	}
	if cfg.DNSPort == 0 {
		cfg.DNSPort = defaults.DNSPort
	}
	if len(cfg.Interfaces) == 0 {
		cfg.Interfaces = defaults.Interfaces
	}
	if cfg.LogPrefix == "" {
		cfg.LogPrefix = defaults.LogPrefix
	}
	if cfg.LogRate == "" {
		cfg.LogRate = defaults.LogRate
	}
	return &NFTablesManager{cfg: cfg}
}

// interfaceMatch returns the nftables iifname match expression.
// For a single interface it returns: iifname "podman*"
// For multiple interfaces it returns: iifname { "podman*", "pasta*" }
func interfaceMatch(interfaces []string) string {
	quoted := make([]string, len(interfaces))
	for i, iface := range interfaces {
		quoted[i] = fmt.Sprintf("%q", iface)
	}
	if len(quoted) == 1 {
		return "iifname " + quoted[0]
	}
	return "iifname { " + strings.Join(quoted, ", ") + " }"
}

// nftablesRulesetTmpl is the template for the complete nftables.conf content.
var nftablesRulesetTmpl = template.Must(template.New("nftables").Funcs(template.FuncMap{
	"ifmatch": interfaceMatch,
}).Parse(`#!/usr/sbin/nft -f
# ============================================================================
# AI-Box nftables ruleset — container egress firewall
# ============================================================================
# Forces all container traffic through the sanctioned proxy and DNS resolver.
# Everything else is dropped by default.
#
# Supports both rootful (podman bridge) and rootless (pasta) networking.
#
# Apply:  nft -f /etc/aibox/nftables.conf
# Remove: nft delete table inet aibox
# Verify: nft list table inet aibox
# ============================================================================

# Flush for idempotent re-application.
table inet aibox {
}
flush table inet aibox

table inet aibox {

    chain forward {
        type filter hook forward priority 0; policy drop;

        # ----------------------------------------------------------------
        # Stateful: allow return traffic for established connections.
        # ----------------------------------------------------------------
        ct state established,related accept

        # ----------------------------------------------------------------
        # Allow: container -> Squid proxy (HTTP/HTTPS egress via proxy).
        # ----------------------------------------------------------------
        {{ ifmatch .Interfaces }} ip daddr {{ .ProxyIP }} tcp dport {{ .ProxyPort }} accept

        # ----------------------------------------------------------------
        # Allow: container -> CoreDNS (sanctioned DNS resolver only).
        # ----------------------------------------------------------------
        {{ ifmatch .Interfaces }} ip daddr {{ .DNSIP }} udp dport {{ .DNSPort }} accept

        # ----------------------------------------------------------------
        # Block: DNS to any other destination (prevent DNS bypass).
        # ----------------------------------------------------------------
        {{ ifmatch .Interfaces }} udp dport 53 drop
        {{ ifmatch .Interfaces }} tcp dport 53 drop

        # ----------------------------------------------------------------
        # Block: DNS-over-TLS (TCP 853).
        # ----------------------------------------------------------------
        {{ ifmatch .Interfaces }} tcp dport 853 drop

        # ----------------------------------------------------------------
        # Block: known DoH resolver IPs on TCP 443 (defense-in-depth).
        # ----------------------------------------------------------------
{{- range .DoHResolverIPs }}
        {{ ifmatch $.Interfaces }} ip daddr {{ . }} tcp dport 443 drop
{{- end }}

        # ----------------------------------------------------------------
        # Block: QUIC / HTTP/3 (UDP 443) — prevents protocol downgrade.
        # ----------------------------------------------------------------
        {{ ifmatch .Interfaces }} udp dport 443 drop

        # ----------------------------------------------------------------
        # Block: ICMP from container interfaces.
        # ----------------------------------------------------------------
        {{ ifmatch .Interfaces }} ip protocol icmp drop
        {{ ifmatch .Interfaces }} ip6 nexthdr icmpv6 drop

        # ----------------------------------------------------------------
        # Log dropped packets (rate-limited to avoid log flooding).
        # ----------------------------------------------------------------
        {{ ifmatch .Interfaces }} limit rate {{ .LogRate }} log prefix "{{ .LogPrefix }}: " level warn

        # ----------------------------------------------------------------
        # Default: drop everything else from container interfaces.
        # (The chain policy is drop, but this explicit rule aids readability.)
        # ----------------------------------------------------------------
        {{ ifmatch .Interfaces }} counter drop
    }
}
`))

// templateData is the struct passed into the nftables template.
type templateData struct {
	NFTablesConfig
	DoHResolverIPs []string
}

// GenerateRuleset produces the complete nftables.conf content as a string.
func (m *NFTablesManager) GenerateRuleset() string {
	data := templateData{
		NFTablesConfig: m.cfg,
		DoHResolverIPs: DoHResolverIPs,
	}

	var buf bytes.Buffer
	if err := nftablesRulesetTmpl.Execute(&buf, data); err != nil {
		// Template is compiled at init; execution failure here is a programming error.
		slog.Error("failed to render nftables template", "error", err)
		return ""
	}
	return buf.String()
}

// WriteConfig writes the generated nftables ruleset to the specified path.
func (m *NFTablesManager) WriteConfig(path string) error {
	content := m.GenerateRuleset()
	if content == "" {
		return fmt.Errorf("generated empty ruleset")
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing nftables config to %s: %w", path, err)
	}

	slog.Info("nftables config written", "path", path)
	return nil
}

// Apply writes the ruleset to a temp file and applies it via `nft -f`.
func (m *NFTablesManager) Apply() error {
	content := m.GenerateRuleset()
	if content == "" {
		return fmt.Errorf("generated empty ruleset")
	}

	tmpFile, err := os.CreateTemp("", "aibox-nftables-*.conf")
	if err != nil {
		return fmt.Errorf("creating temp file for nftables: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing nftables temp file: %w", err)
	}
	tmpFile.Close()

	slog.Debug("applying nftables ruleset", "file", tmpFile.Name())

	cmd := exec.Command("nft", "-f", tmpFile.Name())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("applying nftables ruleset: %s: %w", strings.TrimSpace(stderr.String()), err)
	}

	slog.Info("nftables ruleset applied successfully")
	return nil
}

// Remove deletes the aibox nftables table, removing all managed rules.
func (m *NFTablesManager) Remove() error {
	slog.Debug("removing nftables table inet aibox")

	cmd := exec.Command("nft", "delete", "table", "inet", "aibox")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("removing nftables table: %s: %w", strings.TrimSpace(stderr.String()), err)
	}

	slog.Info("nftables table inet aibox removed")
	return nil
}

// Verify checks that the aibox nftables table is loaded and contains expected
// key rules (forward chain, proxy accept, DNS accept).
func (m *NFTablesManager) Verify() error {
	cmd := exec.Command("nft", "list", "table", "inet", "aibox")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("listing nftables table: %s: %w", strings.TrimSpace(stderr.String()), err)
	}

	output := stdout.String()

	// Check for critical rules that must be present.
	checks := []struct {
		needle string
		desc   string
	}{
		{"chain forward", "forward chain"},
		{"policy drop", "default drop policy"},
		{fmt.Sprintf("tcp dport %d accept", m.cfg.ProxyPort), "proxy accept rule"},
		{fmt.Sprintf("udp dport %d accept", m.cfg.DNSPort), "DNS accept rule"},
		{"tcp dport 853 drop", "DoT block rule"},
		{"udp dport 443 drop", "QUIC block rule"},
	}

	var missing []string
	for _, c := range checks {
		if !strings.Contains(output, c.needle) {
			missing = append(missing, c.desc)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("nftables verification failed, missing rules:\n  - %s", strings.Join(missing, "\n  - "))
	}

	slog.Info("nftables verification passed")
	return nil
}

// IsActive returns true if the aibox nftables table currently exists.
func (m *NFTablesManager) IsActive() bool {
	out, err := exec.Command("nft", "list", "tables").Output()
	if err != nil {
		slog.Debug("nft list tables failed", "error", err)
		return false
	}
	return strings.Contains(string(out), "inet aibox")
}
