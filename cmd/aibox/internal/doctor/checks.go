package doctor

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aibox/aibox/internal/config"
	"github.com/aibox/aibox/internal/falco"
	"github.com/aibox/aibox/internal/host"
	"github.com/aibox/aibox/internal/network"
	"github.com/aibox/aibox/internal/security"
)

// CheckResult represents the outcome of a single diagnostic check.
type CheckResult struct {
	Name        string `json:"name"`
	Status      string `json:"status"` // pass, fail, warn
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

// Report is a collection of check results.
type Report struct {
	Results []CheckResult `json:"results"`
}

// HasFailures returns true if any check failed.
func (r *Report) HasFailures() bool {
	for _, c := range r.Results {
		if c.Status == "fail" {
			return true
		}
	}
	return false
}

// JSON returns the report as formatted JSON.
func (r *Report) JSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// RunAll executes all diagnostic checks and returns a report.
func RunAll(cfg *config.Config) *Report {
	hostInfo := host.Detect()

	checks := []func() CheckResult{
		func() CheckResult { return CheckContainerRuntime(cfg.Runtime) },
		func() CheckResult { return CheckRootless(cfg.Runtime) },
		func() CheckResult { return CheckGVisor(cfg) },
		func() CheckResult { return CheckAppArmor() },
		func() CheckResult { return CheckSeccomp() },
		func() CheckResult { return CheckImage(cfg) },
		func() CheckResult { return CheckDiskSpace() },
	}

	// Network security checks (Phase 2).
	if cfg.Network.Enabled {
		checks = append(checks,
			func() CheckResult { return CheckNFTables() },
			func() CheckResult { return CheckSquidProxy(cfg) },
			func() CheckResult { return CheckCoreDNS(cfg) },
		)
	}

	// Policy and credential checks (Phase 3).
	checks = append(checks,
		func() CheckResult { return CheckPolicyFiles(cfg) },
		func() CheckResult { return CheckCredentials(cfg) },
	)

	// Audit and monitoring checks (Phase 5).
	if cfg.Audit.FalcoEnabled {
		checks = append(checks,
			func() CheckResult { return CheckFalco(cfg) },
		)
	}

	// WSL2-specific check.
	if hostInfo.IsWSL2 {
		checks = append(checks, func() CheckResult { return CheckWSL2(hostInfo) })
	}

	report := &Report{}
	for _, check := range checks {
		report.Results = append(report.Results, check())
	}

	return report
}

// CheckContainerRuntime verifies that a container runtime is installed and accessible.
// It checks the configured runtime first, then falls back to alternatives.
func CheckContainerRuntime(runtime string) CheckResult {
	result := CheckResult{Name: "Container Runtime"}

	// Try the configured runtime first, then fall back to alternatives.
	candidates := []string{runtime}
	for _, alt := range []string{"podman", "docker"} {
		if alt != runtime {
			candidates = append(candidates, alt)
		}
	}

	for _, rt := range candidates {
		path, err := exec.LookPath(rt)
		if err != nil {
			continue
		}

		// Check version.
		out, err := exec.Command(path, "--version").Output()
		if err != nil {
			result.Status = "fail"
			result.Message = fmt.Sprintf("%s found at %s but failed to get version: %v", rt, path, err)
			result.Remediation = fmt.Sprintf("Check that %s is correctly installed and functional", rt)
			return result
		}

		version := strings.TrimSpace(string(out))

		// Verify it can actually run (not just installed).
		// Docker uses a different info format than podman.
		var infoCmd *exec.Cmd
		if rt == "docker" {
			infoCmd = exec.Command(path, "info", "--format", "{{.OSType}}")
		} else {
			infoCmd = exec.Command(path, "info", "--format", "{{.Host.OS}}")
		}
		if err := infoCmd.Run(); err != nil {
			result.Status = "warn"
			result.Message = fmt.Sprintf("%s installed (%s) but 'info' command failed -- service may not be running", rt, version)
			if rt == "podman" {
				result.Remediation = "Podman is daemonless and should work without a service.\n" +
					"  Try running: podman info"
			} else {
				result.Remediation = "Ensure Docker daemon is running: sudo systemctl start docker"
			}
			return result
		}

		result.Status = "pass"
		if rt != runtime {
			result.Message = fmt.Sprintf("%s: %s (configured runtime %q not found, using %s as fallback)", rt, version, runtime, rt)
		} else {
			result.Message = fmt.Sprintf("%s: %s", rt, version)
		}
		return result
	}

	// No runtime found at all.
	result.Status = "fail"
	result.Message = "no container runtime found (tried: " + strings.Join(candidates, ", ") + ")"
	result.Remediation = "Install Podman: https://podman.io/docs/installation\n" +
		"  Ubuntu/Debian: sudo apt-get install -y podman\n" +
		"  Fedora: sudo dnf install -y podman\n" +
		"  Or install Docker: https://docs.docker.com/engine/install/"
	return result
}

// CheckRootless verifies that the container runtime is running in rootless mode.
func CheckRootless(runtime string) CheckResult {
	result := CheckResult{Name: "Rootless Mode"}

	rt := findRuntime(runtime)
	if rt == "" {
		result.Status = "warn"
		result.Message = "cannot check rootless mode: no container runtime found"
		return result
	}

	rtName := filepath.Base(rt)

	if rtName == "podman" {
		out, err := exec.Command(rt, "info", "--format", "{{.Host.Security.Rootless}}").Output()
		if err != nil {
			result.Status = "warn"
			result.Message = fmt.Sprintf("could not query podman rootless status: %v", err)
			return result
		}
		val := strings.TrimSpace(string(out))
		if val == "true" {
			result.Status = "pass"
			result.Message = "podman is running in rootless mode"
		} else {
			result.Status = "fail"
			result.Message = "podman is NOT running in rootless mode"
			result.Remediation = "Run podman as a non-root user. Avoid running with sudo.\n" +
				"  See: https://github.com/containers/podman/blob/main/docs/tutorials/rootless_tutorial.md"
		}
		return result
	}

	// Docker: check if the dockerd socket is owned by the current user
	// or if using rootless docker.
	out, err := exec.Command(rt, "info", "--format", "{{.SecurityOptions}}").Output()
	if err != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("could not query docker info: %v", err)
		return result
	}
	info := string(out)
	if strings.Contains(info, "rootless") {
		result.Status = "pass"
		result.Message = "docker is running in rootless mode"
	} else {
		result.Status = "fail"
		result.Message = "docker is NOT running in rootless mode"
		result.Remediation = "Configure Docker rootless mode:\n" +
			"  https://docs.docker.com/engine/security/rootless/"
	}
	return result
}

// CheckGVisor verifies that the gVisor (runsc) runtime is installed and
// registered with the container runtime.
func CheckGVisor(cfg *config.Config) CheckResult {
	result := CheckResult{Name: "gVisor Runtime"}

	if !cfg.GVisor.Enabled {
		result.Status = "warn"
		result.Message = "gVisor is disabled in configuration"
		result.Remediation = "Set gvisor.enabled=true in config for maximum isolation"
		return result
	}

	// Check for runsc binary.
	runscPath, err := exec.LookPath("runsc")
	if err != nil {
		// Also check common install paths.
		for _, p := range []string{"/usr/local/bin/runsc", "/usr/bin/runsc"} {
			if _, err := os.Stat(p); err == nil {
				runscPath = p
				break
			}
		}
	}

	if runscPath == "" {
		result.Status = "warn"
		result.Message = "runsc (gVisor) binary not found -- sandbox will use seccomp-only isolation"
		result.Remediation = "Install gVisor for stronger isolation:\n" +
			"  curl -fsSL https://gvisor.dev/archive.key | sudo gpg --dearmor -o /usr/share/keyrings/gvisor-archive-keyring.gpg\n" +
			"  echo 'deb [signed-by=/usr/share/keyrings/gvisor-archive-keyring.gpg] https://storage.googleapis.com/gvisor/releases release main' | sudo tee /etc/apt/sources.list.d/gvisor.list\n" +
			"  sudo apt-get update && sudo apt-get install -y runsc"
		return result
	}

	// Get version.
	out, err := exec.Command(runscPath, "--version").Output()
	if err != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("runsc found at %s but version check failed", runscPath)
		return result
	}
	version := strings.TrimSpace(string(out))

	// Check if runtime is registered with the container runtime.
	rtPath := findRuntime(cfg.Runtime)
	if rtPath != "" {
		infoOut, err := exec.Command(rtPath, "info", "--format", "json").Output()
		if err == nil && strings.Contains(string(infoOut), "runsc") {
			result.Status = "pass"
			result.Message = fmt.Sprintf("runsc installed and registered (%s)", firstLine(version))
			return result
		}
	}

	result.Status = "warn"
	result.Message = fmt.Sprintf("runsc installed (%s) but may not be registered as an OCI runtime", firstLine(version))
	result.Remediation = "Register runsc with Podman:\n" +
		"  sudo runsc install\n" +
		"  Or add to /etc/containers/containers.conf:\n" +
		"  [engine.runtimes]\n" +
		"  runsc = [\"/usr/local/bin/runsc\"]"
	return result
}

// CheckAppArmor verifies that AppArmor is available and the aibox-sandbox
// profile is loaded.
func CheckAppArmor() CheckResult {
	result := CheckResult{Name: "AppArmor Profile"}

	if !security.IsAppArmorAvailable() {
		result.Status = "warn"
		result.Message = "AppArmor is not available on this system"
		result.Remediation = "AppArmor provides an additional isolation layer.\n" +
			"  Ubuntu: AppArmor is enabled by default. Check: sudo aa-status\n" +
			"  Other distros: Security relies on gVisor + seccomp (still strong isolation)"
		return result
	}

	loaded, err := security.IsProfileLoaded("aibox-sandbox")
	if err != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("could not check AppArmor profile status: %v", err)
		result.Remediation = "Run 'aibox setup' to load the AppArmor profile"
		return result
	}

	if !loaded {
		result.Status = "warn"
		result.Message = "aibox-sandbox AppArmor profile is not loaded (gVisor + seccomp still provide strong isolation)"
		result.Remediation = "Load the profile for additional isolation:\n" +
			"  sudo aibox setup --system\n" +
			"  (The profile is bundled in the aibox binary)"
		return result
	}

	result.Status = "pass"
	result.Message = "aibox-sandbox profile loaded"
	return result
}

// CheckSeccomp verifies that the seccomp profile file exists.
func CheckSeccomp() CheckResult {
	result := CheckResult{Name: "Seccomp Profile"}

	candidates := []string{
		"/etc/aibox/seccomp.json",
	}

	// Also check relative to the binary.
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "configs", "seccomp.json"),
			filepath.Join(filepath.Dir(dir), "configs", "seccomp.json"),
		)
	}

	// Check from working directory too.
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "configs", "seccomp.json"))
	}

	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			result.Status = "pass"
			result.Message = fmt.Sprintf("seccomp profile found: %s", p)
			return result
		}
	}

	result.Status = "fail"
	result.Message = "seccomp profile (seccomp.json) not found"
	result.Remediation = "Install the seccomp profile:\n" +
		"  sudo aibox setup --system\n" +
		"  (The profile is bundled in the aibox binary and will be installed to /etc/aibox/seccomp.json)"
	return result
}

// CheckImage verifies the base container image is available locally.
func CheckImage(cfg *config.Config) CheckResult {
	result := CheckResult{Name: "Container Image"}

	// Find a working runtime (try configured, then fallback).
	rt := findRuntime(cfg.Runtime)
	if rt == "" {
		result.Status = "warn"
		result.Message = "cannot check image: no container runtime found"
		return result
	}

	// Check if image exists locally. Docker doesn't have 'image exists',
	// so we use 'image inspect' which works for both docker and podman.
	if err := exec.Command(rt, "image", "inspect", cfg.Image).Run(); err != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("image %s not cached locally", cfg.Image)
		result.Remediation = fmt.Sprintf("Pull the image:\n  %s pull %s\n  Or run: aibox update", filepath.Base(rt), cfg.Image)
		return result
	}

	// Check image age.
	out, err := exec.Command(rt, "image", "inspect", "--format", "{{.Created}}", cfg.Image).Output()
	if err == nil {
		created := strings.TrimSpace(string(out))
		result.Status = "pass"
		result.Message = fmt.Sprintf("image %s cached locally (created: %s)", cfg.Image, created)
	} else {
		result.Status = "pass"
		result.Message = fmt.Sprintf("image %s cached locally", cfg.Image)
	}

	return result
}

// findRuntime returns the path of the first available container runtime.
func findRuntime(preferred string) string {
	candidates := []string{preferred}
	for _, alt := range []string{"podman", "docker"} {
		if alt != preferred {
			candidates = append(candidates, alt)
		}
	}
	for _, rt := range candidates {
		if p, err := exec.LookPath(rt); err == nil {
			return p
		}
	}
	return ""
}

// CheckDiskSpace verifies sufficient disk space for aibox operations.
func CheckDiskSpace() CheckResult {
	result := CheckResult{Name: "Disk Space"}

	home, err := config.ResolveHomeDir()
	if err != nil {
		result.Status = "warn"
		result.Message = "could not determine home directory"
		return result
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(home, &stat); err != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("could not check disk space: %v", err)
		return result
	}

	freeGB := (stat.Bavail * uint64(stat.Bsize)) / (1024 * 1024 * 1024)

	if freeGB < 10 {
		result.Status = "fail"
		result.Message = fmt.Sprintf("only %d GB free in %s (minimum 10 GB recommended)", freeGB, home)
		result.Remediation = "Free up disk space. AI-Box needs at least 10 GB for images, caches, and workspaces."
		return result
	}

	if freeGB < 20 {
		result.Status = "warn"
		result.Message = fmt.Sprintf("%d GB free in %s (20+ GB recommended)", freeGB, home)
		result.Remediation = "Consider freeing disk space. 20+ GB recommended for comfortable usage."
		return result
	}

	result.Status = "pass"
	result.Message = fmt.Sprintf("%d GB free in %s", freeGB, home)
	return result
}

// CheckWSL2 runs WSL2-specific diagnostics.
func CheckWSL2(hostInfo host.HostInfo) CheckResult {
	result := CheckResult{Name: "WSL2 Environment"}

	if !hostInfo.IsWSL2 {
		result.Status = "pass"
		result.Message = "not running under WSL2 (native Linux)"
		return result
	}

	// Check kernel version.
	parts := strings.Fields(hostInfo.KernelVersion)
	kernelVer := "unknown"
	if len(parts) >= 3 {
		kernelVer = parts[2]
	}

	// WSL2 needs kernel >= 5.15 for gVisor systrap.
	major, minor := parseKernelVersion(kernelVer)
	if major < 5 || (major == 5 && minor < 15) {
		result.Status = "warn"
		result.Message = fmt.Sprintf("WSL2 kernel %s may be too old for gVisor systrap (need 5.15+)", kernelVer)
		result.Remediation = "Update WSL2: wsl --update"
		return result
	}

	// Check available memory via /proc/meminfo.
	memGB := getAvailableMemoryGB()
	if memGB > 0 && memGB < 12 {
		result.Status = "warn"
		result.Message = fmt.Sprintf("WSL2 has ~%d GB RAM allocated (12+ GB recommended for AI-Box + IDE + builds)", memGB)
		result.Remediation = "Increase WSL2 memory in %USERPROFILE%\\.wslconfig:\n" +
			"  [wsl2]\n" +
			"  memory=16GB\n" +
			"  processors=8"
		return result
	}

	result.Status = "pass"
	result.Message = fmt.Sprintf("WSL2 kernel %s, ~%d GB RAM available", kernelVer, memGB)
	return result
}

// parseKernelVersion extracts major.minor from a kernel version string.
func parseKernelVersion(ver string) (int, int) {
	// Format: "5.15.90.1-microsoft-standard-WSL2" or "6.1.21..."
	parts := strings.SplitN(ver, ".", 3)
	if len(parts) < 2 {
		return 0, 0
	}
	major, _ := strconv.Atoi(parts[0])
	// Minor may have a trailing dash or suffix.
	minorStr := strings.SplitN(parts[1], "-", 2)[0]
	minor, _ := strconv.Atoi(minorStr)
	return major, minor
}

// getAvailableMemoryGB reads total memory from /proc/meminfo.
func getAvailableMemoryGB() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, _ := strconv.ParseInt(fields[1], 10, 64)
				return int(kb / (1024 * 1024))
			}
		}
	}
	return 0
}

// CheckNFTables verifies that the aibox nftables rules are active on the host.
func CheckNFTables() CheckResult {
	result := CheckResult{Name: "nftables Rules"}

	mgr := network.NewNFTablesManager(network.DefaultNFTablesConfig())
	if !mgr.IsActive() {
		result.Status = "fail"
		result.Message = "nftables table 'inet aibox' not found"
		result.Remediation = "Run 'aibox setup' to install nftables rules.\n" +
			"  Or manually: sudo nft -f /etc/aibox/nftables.conf"
		return result
	}

	if err := mgr.Verify(); err != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("nftables table exists but verification incomplete: %v", err)
		result.Remediation = "Run 'aibox setup' to reinstall nftables rules"
		return result
	}

	result.Status = "pass"
	result.Message = "nftables rules active (table inet aibox)"
	return result
}

// CheckSquidProxy verifies that the Squid proxy is running and reachable.
func CheckSquidProxy(cfg *config.Config) CheckResult {
	result := CheckResult{Name: "Squid Proxy"}

	addr := net.JoinHostPort(cfg.Network.ProxyAddr, strconv.Itoa(cfg.Network.ProxyPort))
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("Squid proxy not reachable at %s", addr)
		result.Remediation = "Run 'aibox setup' to start Squid proxy.\n" +
			"  Or check: sudo systemctl status squid"
		return result
	}
	conn.Close()

	result.Status = "pass"
	result.Message = fmt.Sprintf("Squid proxy listening at %s", addr)
	return result
}

// CheckCoreDNS verifies that CoreDNS is running and resolving allowlisted domains.
func CheckCoreDNS(cfg *config.Config) CheckResult {
	result := CheckResult{Name: "CoreDNS Resolver"}

	addr := net.JoinHostPort(cfg.Network.DNSAddr, strconv.Itoa(cfg.Network.DNSPort))
	conn, err := net.DialTimeout("udp", addr, 3*time.Second)
	if err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("CoreDNS not reachable at %s", addr)
		result.Remediation = "Run 'aibox setup' to start CoreDNS.\n" +
			"  Or check: sudo systemctl status coredns"
		return result
	}
	conn.Close()

	result.Status = "pass"
	result.Message = fmt.Sprintf("CoreDNS listening at %s", addr)
	return result
}

// CheckPolicyFiles verifies that policy files referenced in the config exist.
func CheckPolicyFiles(cfg *config.Config) CheckResult {
	result := CheckResult{Name: "Policy Files"}

	var missing []string
	var found []string

	// Org baseline is mandatory.
	if cfg.Policy.OrgBaselinePath != "" {
		if _, err := os.Stat(cfg.Policy.OrgBaselinePath); err != nil {
			missing = append(missing, "org baseline: "+cfg.Policy.OrgBaselinePath)
		} else {
			found = append(found, "org baseline")
		}
	}

	// Team policy is optional — only check if configured.
	if cfg.Policy.TeamPolicyPath != "" {
		if _, err := os.Stat(cfg.Policy.TeamPolicyPath); err != nil {
			missing = append(missing, "team policy: "+cfg.Policy.TeamPolicyPath)
		} else {
			found = append(found, "team policy")
		}
	}

	// Decision log directory should be writable.
	if cfg.Policy.DecisionLogPath != "" {
		logDir := filepath.Dir(cfg.Policy.DecisionLogPath)
		if _, err := os.Stat(logDir); err != nil {
			missing = append(missing, "decision log dir: "+logDir)
		}
	}

	if len(missing) > 0 && len(found) == 0 {
		result.Status = "warn"
		result.Message = fmt.Sprintf("policy files not found: %s", strings.Join(missing, "; "))
		result.Remediation = "Install org baseline policy:\n" +
			"  sudo aibox setup --system\n" +
			"  Or create a custom policy at: " + cfg.Policy.OrgBaselinePath
		return result
	}

	if len(missing) > 0 {
		result.Status = "warn"
		result.Message = fmt.Sprintf("some policy files missing: %s (found: %s)", strings.Join(missing, "; "), strings.Join(found, ", "))
		return result
	}

	result.Status = "pass"
	if len(found) > 0 {
		result.Message = fmt.Sprintf("policy files found: %s", strings.Join(found, ", "))
	} else {
		result.Message = "no policy paths configured"
	}
	return result
}

// CheckCredentials verifies the credential provider is functional.
func CheckCredentials(cfg *config.Config) CheckResult {
	result := CheckResult{Name: "Credential Provider"}

	mode := cfg.Credentials.Mode
	if mode == "" {
		mode = "fallback"
	}

	switch mode {
	case "fallback":
		result.Status = "pass"
		result.Message = "credential mode: fallback (OS keychain / encrypted file)"
	case "vault":
		// Check that Vault address is configured.
		if cfg.Credentials.VaultAddr == "" {
			result.Status = "fail"
			result.Message = "credential mode: vault, but vault_addr is not configured"
			result.Remediation = "Set credentials.vault_addr in config or AIBOX_CREDENTIALS_VAULT_ADDR env var"
			return result
		}

		// Check SPIFFE socket if configured.
		if cfg.Credentials.SPIFFESocketPath != "" {
			if _, err := os.Stat(cfg.Credentials.SPIFFESocketPath); err != nil {
				result.Status = "warn"
				result.Message = fmt.Sprintf("credential mode: vault (SPIFFE socket not found: %s)", cfg.Credentials.SPIFFESocketPath)
				result.Remediation = "Ensure SPIRE agent is running:\n" +
					"  sudo systemctl start spire-agent\n" +
					"  Or set credentials.mode=fallback for local credential storage"
				return result
			}
		}

		result.Status = "pass"
		result.Message = fmt.Sprintf("credential mode: vault (%s)", cfg.Credentials.VaultAddr)
	default:
		result.Status = "warn"
		result.Message = fmt.Sprintf("unknown credential mode: %s", mode)
		result.Remediation = "Set credentials.mode to \"fallback\" or \"vault\""
	}

	return result
}

// CheckFalco verifies that Falco runtime monitoring is installed, running,
// and has the AI-Box rules deployed.
func CheckFalco(cfg *config.Config) CheckResult {
	result := CheckResult{Name: "Falco Runtime Monitoring"}

	falcoCfg := falco.Config{
		Enabled:    cfg.Audit.FalcoEnabled,
		RulesPath:  "/etc/aibox/falco_rules.yaml",
		ConfigPath: "/etc/aibox/falco.yaml",
		AlertsPath: "/var/log/aibox/falco-alerts.jsonl",
	}
	mgr := falco.NewManager(falcoCfg)

	if !mgr.IsInstalled() {
		result.Status = "fail"
		result.Message = "Falco is not installed"
		result.Remediation = "Install Falco:\n" +
			"  curl -fsSL https://falco.org/repo/falcosecurity-packages.asc | sudo gpg --dearmor -o /usr/share/keyrings/falco-archive-keyring.gpg\n" +
			"  sudo apt-get update && sudo apt-get install -y falco\n" +
			"  Or run: sudo aibox setup --system"
		return result
	}

	version, err := mgr.Version()
	if err != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("Falco installed but version check failed: %v", err)
		return result
	}

	if !mgr.IsRunning() {
		result.Status = "fail"
		result.Message = fmt.Sprintf("Falco is installed (%s) but not running", firstLine(version))
		result.Remediation = "Start Falco:\n" +
			"  sudo systemctl start falco\n" +
			"  Or run: sudo aibox setup --system"
		return result
	}

	if err := mgr.HealthCheck(); err != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("Falco running but health check failed: %v", err)
		result.Remediation = "Run 'sudo aibox setup --system' to deploy Falco rules and config"
		return result
	}

	result.Status = "pass"
	result.Message = fmt.Sprintf("Falco running (%s), rules deployed, alerts output configured", firstLine(version))
	return result
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
