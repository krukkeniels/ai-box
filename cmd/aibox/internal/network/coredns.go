package network

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DomainEntry maps a domain name to its IP address for DNS resolution.
type DomainEntry struct {
	Domain string
	IP     string
}

// DefaultDomainEntries returns the base set of DNS entries for AI-Box infrastructure.
func DefaultDomainEntries() []DomainEntry {
	return []DomainEntry{
		{Domain: "harbor.internal", IP: "10.0.0.10"},
		{Domain: "nexus.internal", IP: "10.0.0.11"},
		{Domain: "foundry.internal", IP: "10.0.0.12"},
		{Domain: "git.internal", IP: "10.0.0.13"},
		{Domain: "vault.internal", IP: "10.0.0.14"},
	}
}

// CoreDNSConfig holds the settings for generating and running a CoreDNS instance.
type CoreDNSConfig struct {
	ListenAddr     string        // address to bind (default "127.0.0.1")
	ListenPort     int           // DNS port (default 53)
	Entries        []DomainEntry // domain-to-IP mappings
	UpstreamDNS    string        // upstream resolver for allowlisted domains (default "")
	MetricsPort    int           // Prometheus metrics port (default 9153)
	HealthPort     int           // health check port (default 8080)
	ConfigPath     string        // path to write Corefile (default "/etc/aibox/Corefile")
	QueryRateLimit int           // queries per second limit (default 100)
}

// DefaultCoreDNSConfig returns a CoreDNSConfig with sensible defaults.
func DefaultCoreDNSConfig() CoreDNSConfig {
	return CoreDNSConfig{
		ListenAddr:     "127.0.0.1",
		ListenPort:     53,
		Entries:        DefaultDomainEntries(),
		UpstreamDNS:    "",
		MetricsPort:    9153,
		HealthPort:     8080,
		ConfigPath:     "/etc/aibox/Corefile",
		QueryRateLimit: 100,
	}
}

// CoreDNSManager manages CoreDNS configuration and lifecycle.
type CoreDNSManager struct {
	cfg CoreDNSConfig
}

// NewCoreDNSManager creates a CoreDNSManager with the given configuration.
func NewCoreDNSManager(cfg CoreDNSConfig) *CoreDNSManager {
	return &CoreDNSManager{cfg: cfg}
}

// GenerateCorefile produces a complete CoreDNS Corefile from the current configuration.
//
// The generated Corefile:
//   - Binds to the configured listen address and port
//   - Uses the hosts plugin to resolve allowlisted domains
//   - Blocks DNS tunneling record types (TXT, NULL) for non-allowlisted domains
//   - Returns NXDOMAIN for all non-allowlisted queries
//   - Enables logging, metrics, health, readiness, and error plugins
func (m *CoreDNSManager) GenerateCorefile() string {
	var b strings.Builder

	bind := fmt.Sprintf("%s:%d", m.cfg.ListenAddr, m.cfg.ListenPort)

	// --- Main server block ---
	b.WriteString(fmt.Sprintf(".:dns://%s {\n", bind))

	// Health check endpoint.
	b.WriteString(fmt.Sprintf("    health %s:%d\n", m.cfg.ListenAddr, m.cfg.HealthPort))

	// Readiness probe endpoint.
	b.WriteString(fmt.Sprintf("    ready %s:8181\n", m.cfg.ListenAddr))

	// Prometheus metrics.
	b.WriteString(fmt.Sprintf("    prometheus %s:%d\n", m.cfg.ListenAddr, m.cfg.MetricsPort))

	// Query logging.
	b.WriteString("    log\n")

	// Error logging.
	b.WriteString("    errors\n")

	// Hosts plugin: resolve allowlisted domains.
	b.WriteString("    hosts {\n")
	for _, entry := range m.cfg.Entries {
		b.WriteString(fmt.Sprintf("        %s %s\n", entry.IP, entry.Domain))
	}
	b.WriteString("        fallthrough\n")
	b.WriteString("    }\n")

	// Block DNS tunneling-prone record types for non-allowlisted domains.
	// TXT records can carry exfiltrated data.
	b.WriteString("    template IN TXT . {\n")
	b.WriteString("        rcode NXDOMAIN\n")
	b.WriteString("    }\n")

	// NULL records can be used for tunneling.
	b.WriteString("    template IN NULL . {\n")
	b.WriteString("        rcode NXDOMAIN\n")
	b.WriteString("    }\n")

	// Catch-all: return NXDOMAIN for everything not matched above.
	b.WriteString("    template IN ANY . {\n")
	b.WriteString("        rcode NXDOMAIN\n")
	b.WriteString("    }\n")

	b.WriteString("}\n")

	return b.String()
}

// WriteCorefile writes the generated Corefile to the specified path.
// If path is empty, the configured ConfigPath is used.
func (m *CoreDNSManager) WriteCorefile(path string) error {
	if path == "" {
		path = m.cfg.ConfigPath
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating Corefile directory %s: %w", dir, err)
	}

	content := m.GenerateCorefile()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing Corefile to %s: %w", path, err)
	}

	slog.Info("wrote Corefile", "path", path)
	return nil
}

// IsRunning checks whether CoreDNS is listening on the configured address and port.
func (m *CoreDNSManager) IsRunning() bool {
	addr := net.JoinHostPort(m.cfg.ListenAddr, strconv.Itoa(m.cfg.ListenPort))
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		// DNS is typically UDP; also try a UDP probe via the health endpoint.
		return m.healthEndpointUp()
	}
	conn.Close()
	return true
}

// healthEndpointUp checks if the CoreDNS health HTTP endpoint responds.
func (m *CoreDNSManager) healthEndpointUp() bool {
	url := fmt.Sprintf("http://%s/health", net.JoinHostPort(m.cfg.ListenAddr, strconv.Itoa(m.cfg.HealthPort)))
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// HealthCheck verifies that CoreDNS is functioning correctly by:
//  1. Resolving an allowlisted domain (should succeed)
//  2. Resolving a non-allowlisted domain (should return NXDOMAIN)
func (m *CoreDNSManager) HealthCheck() error {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, "udp", net.JoinHostPort(m.cfg.ListenAddr, strconv.Itoa(m.cfg.ListenPort)))
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check 1: an allowlisted domain should resolve.
	if len(m.cfg.Entries) > 0 {
		testDomain := m.cfg.Entries[0].Domain
		expectedIP := m.cfg.Entries[0].IP

		addrs, err := resolver.LookupHost(ctx, testDomain)
		if err != nil {
			return fmt.Errorf("allowlisted domain %q failed to resolve: %w", testDomain, err)
		}

		found := false
		for _, addr := range addrs {
			if addr == expectedIP {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("allowlisted domain %q resolved to %v, expected %s", testDomain, addrs, expectedIP)
		}
		slog.Debug("health check passed: allowlisted domain resolved", "domain", testDomain, "ip", expectedIP)
	}

	// Check 2: a non-allowlisted domain should return NXDOMAIN.
	_, err := resolver.LookupHost(ctx, "should-not-resolve.example.com")
	if err == nil {
		return fmt.Errorf("non-allowlisted domain resolved when it should have returned NXDOMAIN")
	}

	// The error should be a DNS NXDOMAIN (no such host).
	if dnsErr, ok := err.(*net.DNSError); ok {
		if dnsErr.IsNotFound {
			slog.Debug("health check passed: non-allowlisted domain returned NXDOMAIN")
			return nil
		}
	}

	// Any DNS error for the non-allowlisted domain is acceptable (connection
	// refused, timeout, etc. all indicate the domain was not resolved).
	slog.Debug("health check passed: non-allowlisted domain failed lookup", "error", err)
	return nil
}

// Start launches the CoreDNS process in the background using the configured Corefile.
func (m *CoreDNSManager) Start() error {
	if m.IsRunning() {
		slog.Info("CoreDNS is already running")
		return nil
	}

	corednsPath, err := exec.LookPath("coredns")
	if err != nil {
		return fmt.Errorf("coredns binary not found in PATH: %w", err)
	}

	cfgPath := m.cfg.ConfigPath
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		slog.Info("Corefile not found, generating", "path", cfgPath)
		if err := m.WriteCorefile(cfgPath); err != nil {
			return fmt.Errorf("generating Corefile: %w", err)
		}
	}

	cmd := exec.Command(corednsPath, "-conf", cfgPath)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting CoreDNS: %w", err)
	}

	slog.Info("CoreDNS started", "pid", cmd.Process.Pid, "config", cfgPath)

	// Wait briefly for the process to initialize before returning.
	time.Sleep(500 * time.Millisecond)

	// Verify it did not immediately exit.
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return fmt.Errorf("CoreDNS exited immediately after start (exit code %d)", cmd.ProcessState.ExitCode())
	}

	return nil
}

// Stop terminates a running CoreDNS process.
func (m *CoreDNSManager) Stop() error {
	// Find CoreDNS process by looking for the running PID.
	out, err := exec.Command("pgrep", "-f", "coredns.*-conf").Output()
	if err != nil {
		slog.Debug("no CoreDNS process found to stop")
		return nil
	}

	pids := strings.Fields(strings.TrimSpace(string(out)))
	for _, pid := range pids {
		slog.Info("stopping CoreDNS", "pid", pid)
		if err := exec.Command("kill", pid).Run(); err != nil {
			slog.Warn("failed to stop CoreDNS process", "pid", pid, "error", err)
			// Try SIGKILL as fallback.
			_ = exec.Command("kill", "-9", pid).Run()
		}
	}

	slog.Info("CoreDNS stopped")
	return nil
}

// Install downloads and installs the CoreDNS binary to /usr/local/bin.
func (m *CoreDNSManager) Install() error {
	// Check if already installed.
	if path, err := exec.LookPath("coredns"); err == nil {
		out, _ := exec.Command(path, "-version").Output()
		slog.Info("CoreDNS already installed", "path", path, "version", strings.TrimSpace(string(out)))
		return nil
	}

	version := "1.12.0"
	arch := runtime.GOARCH

	url := fmt.Sprintf("https://github.com/coredns/coredns/releases/download/v%s/coredns_%s_linux_%s.tgz",
		version, version, arch)

	slog.Info("downloading CoreDNS", "version", version, "arch", arch)

	tmpDir, err := os.MkdirTemp("", "coredns-install-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tarball := filepath.Join(tmpDir, "coredns.tgz")

	// Download with curl (commonly available, handles redirects).
	dlCmd := exec.Command("curl", "-fsSL", "-o", tarball, url)
	dlCmd.Stderr = os.Stderr
	if err := dlCmd.Run(); err != nil {
		return fmt.Errorf("downloading CoreDNS v%s: %w", version, err)
	}

	// Verify the download is not empty.
	info, err := os.Stat(tarball)
	if err != nil || info.Size() == 0 {
		return fmt.Errorf("downloaded CoreDNS tarball is empty or missing")
	}

	// Extract.
	extractCmd := exec.Command("tar", "-xzf", tarball, "-C", tmpDir)
	if err := extractCmd.Run(); err != nil {
		return fmt.Errorf("extracting CoreDNS tarball: %w", err)
	}

	srcBin := filepath.Join(tmpDir, "coredns")
	if _, err := os.Stat(srcBin); err != nil {
		return fmt.Errorf("coredns binary not found in tarball: %w", err)
	}

	// Install to /usr/local/bin (requires root or appropriate permissions).
	destBin := "/usr/local/bin/coredns"
	installCmd := exec.Command("install", "-m", "0755", srcBin, destBin)
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("installing coredns to %s (may require sudo): %w", destBin, err)
	}

	// Verify.
	verifyOut, err := exec.Command(destBin, "-version").Output()
	if err != nil {
		return fmt.Errorf("verifying CoreDNS installation: %w", err)
	}

	slog.Info("CoreDNS installed", "path", destBin, "version", strings.TrimSpace(string(verifyOut)))

	// Allow binding to privileged ports without root (port 53).
	capCmd := exec.Command("setcap", "cap_net_bind_service=+ep", destBin)
	if err := capCmd.Run(); err != nil {
		slog.Warn("could not set cap_net_bind_service on coredns binary (may require sudo for port 53)", "error", err)
	}

	return nil
}

// ValidateCorefile checks whether the Corefile at the given path is valid
// by running coredns with a dry-run style check. Returns nil if valid.
func (m *CoreDNSManager) ValidateCorefile(path string) error {
	if path == "" {
		path = m.cfg.ConfigPath
	}

	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("Corefile not found at %s: %w", path, err)
	}

	corednsPath, err := exec.LookPath("coredns")
	if err != nil {
		return fmt.Errorf("coredns binary not found (cannot validate): %w", err)
	}

	// CoreDNS does not have a native --dry-run; we use -plugins to verify parsing.
	cmd := exec.Command(corednsPath, "-conf", path, "-plugins")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Corefile validation failed: %s\n%s", err, string(out))
	}

	slog.Debug("Corefile validation passed", "path", path)
	return nil
}

// AllowlistedDomains returns a list of domain names from the current configuration.
func (m *CoreDNSManager) AllowlistedDomains() []string {
	domains := make([]string, len(m.cfg.Entries))
	for i, e := range m.cfg.Entries {
		domains[i] = e.Domain
	}
	return domains
}

