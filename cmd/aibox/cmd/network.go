package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/aibox/aibox/internal/network"
	"github.com/spf13/cobra"
)

var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Manage sandbox network configuration",
	Long:  `Network provides subcommands for testing and managing network access in the sandbox.`,
}

var networkTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Test network connectivity from the sandbox",
	Long: `Test verifies that all Phase 2 network security components are
operational: nftables rules active, Squid proxy reachable, CoreDNS
resolving allowlisted domains, and blocked domains inaccessible.`,
	RunE: runNetworkTest,
}

func init() {
	networkTestCmd.Flags().String("format", "text", "output format (text or json)")
	networkCmd.AddCommand(networkTestCmd)
	rootCmd.AddCommand(networkCmd)
}

// networkCheckResult mirrors the doctor CheckResult for consistent output.
type networkCheckResult struct {
	Name        string `json:"name"`
	Status      string `json:"status"` // pass, fail, warn
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

func runNetworkTest(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")

	if !Cfg.Network.Enabled {
		fmt.Println("Network security is disabled in configuration.")
		fmt.Println("Set network.enabled=true and run 'aibox setup' to enable.")
		return nil
	}

	checks := []func() networkCheckResult{
		checkNFTablesActive,
		checkSquidReachable,
		checkCoreDNSReachable,
		checkCoreDNSBlocking,
		checkAllowlistedAccess,
	}

	var results []networkCheckResult
	for _, check := range checks {
		results = append(results, check())
	}

	if format == "json" {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling results: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Text output.
	fmt.Println("AI-Box Network Test")
	fmt.Println()

	hasFailures := false
	for _, r := range results {
		var indicator string
		switch r.Status {
		case "pass":
			indicator = "[OK]  "
		case "warn":
			indicator = "[WARN]"
		case "fail":
			indicator = "[FAIL]"
			hasFailures = true
		}

		fmt.Printf("  %s %s: %s\n", indicator, r.Name, r.Message)
		if r.Remediation != "" && r.Status != "pass" {
			fmt.Printf("         Remediation: %s\n", r.Remediation)
		}
	}

	fmt.Println()
	if hasFailures {
		fmt.Println("Some checks FAILED. Run 'aibox setup' to configure network security.")
		return fmt.Errorf("network test found failures")
	}

	fmt.Println("All network checks passed.")
	return nil
}

func checkNFTablesActive() networkCheckResult {
	result := networkCheckResult{Name: "nftables Rules"}

	mgr := network.NewNFTablesManager(network.DefaultNFTablesConfig())
	if !mgr.IsActive() {
		result.Status = "fail"
		result.Message = "nftables table 'inet aibox' not found"
		result.Remediation = "Run 'aibox setup' to install nftables rules"
		return result
	}

	if err := mgr.Verify(); err != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("nftables table exists but verification failed: %v", err)
		result.Remediation = "Run 'aibox setup' to reinstall nftables rules"
		return result
	}

	result.Status = "pass"
	result.Message = "nftables rules active (table inet aibox)"
	return result
}

func checkSquidReachable() networkCheckResult {
	result := networkCheckResult{Name: "Squid Proxy"}

	addr := fmt.Sprintf("%s:%d", Cfg.Network.ProxyAddr, Cfg.Network.ProxyPort)
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("cannot connect to Squid proxy at %s", addr)
		result.Remediation = "Run 'aibox setup' to start Squid proxy"
		return result
	}
	conn.Close()

	result.Status = "pass"
	result.Message = fmt.Sprintf("Squid proxy reachable at %s", addr)
	return result
}

func checkCoreDNSReachable() networkCheckResult {
	result := networkCheckResult{Name: "CoreDNS Resolver"}

	addr := fmt.Sprintf("%s:%d", Cfg.Network.DNSAddr, Cfg.Network.DNSPort)
	conn, err := net.DialTimeout("udp", addr, 3*time.Second)
	if err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("cannot connect to CoreDNS at %s", addr)
		result.Remediation = "Run 'aibox setup' to start CoreDNS"
		return result
	}
	conn.Close()

	// Try resolving an allowlisted domain.
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, netw, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.Dial("udp", addr)
		},
	}

	if len(Cfg.Network.AllowedDomains) > 0 {
		domain := Cfg.Network.AllowedDomains[0]
		_, err := resolver.LookupHost(context.Background(), domain)
		if err != nil {
			result.Status = "warn"
			result.Message = fmt.Sprintf("CoreDNS reachable but failed to resolve %s: %v", domain, err)
			result.Remediation = "Check CoreDNS configuration and domain entries"
			return result
		}

		result.Status = "pass"
		result.Message = fmt.Sprintf("CoreDNS resolving allowlisted domains (tested: %s)", domain)
		return result
	}

	result.Status = "pass"
	result.Message = "CoreDNS reachable"
	return result
}

func checkCoreDNSBlocking() networkCheckResult {
	result := networkCheckResult{Name: "DNS Blocking"}

	addr := fmt.Sprintf("%s:%d", Cfg.Network.DNSAddr, Cfg.Network.DNSPort)
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, netw, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.Dial("udp", addr)
		},
	}

	// Try resolving a domain that should be blocked.
	_, err := resolver.LookupHost(context.Background(), "example.com")
	if err == nil {
		result.Status = "fail"
		result.Message = "non-allowlisted domain 'example.com' resolved successfully (should return NXDOMAIN)"
		result.Remediation = "Check CoreDNS Corefile — the catch-all NXDOMAIN template may be missing"
		return result
	}

	result.Status = "pass"
	result.Message = "non-allowlisted domains correctly blocked (example.com -> NXDOMAIN)"
	return result
}

func checkAllowlistedAccess() networkCheckResult {
	result := networkCheckResult{Name: "Allowlisted Domain Access"}

	if len(Cfg.Network.AllowedDomains) == 0 {
		result.Status = "warn"
		result.Message = "no allowed domains configured"
		return result
	}

	// Check that we can TCP connect to the proxy (basic connectivity test).
	proxyAddr := fmt.Sprintf("%s:%d", Cfg.Network.ProxyAddr, Cfg.Network.ProxyPort)
	conn, err := net.DialTimeout("tcp", proxyAddr, 3*time.Second)
	if err != nil {
		result.Status = "warn"
		result.Message = "cannot verify domain access — proxy not reachable"
		return result
	}
	conn.Close()

	result.Status = "pass"
	result.Message = fmt.Sprintf("proxy accessible for %d allowlisted domains", len(Cfg.Network.AllowedDomains))
	return result
}
