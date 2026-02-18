package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/aibox/aibox/internal/config"
	"github.com/aibox/aibox/internal/host"
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
		func() CheckResult { return CheckPodman(cfg.Runtime) },
		func() CheckResult { return CheckGVisor(cfg) },
		func() CheckResult { return CheckAppArmor() },
		func() CheckResult { return CheckSeccomp() },
		func() CheckResult { return CheckImage(cfg) },
		func() CheckResult { return CheckDiskSpace() },
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

// CheckPodman verifies the container runtime is installed and accessible.
func CheckPodman(runtime string) CheckResult {
	result := CheckResult{Name: "Container Runtime"}

	path, err := exec.LookPath(runtime)
	if err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("%s not found in PATH", runtime)
		if runtime == "podman" {
			result.Remediation = "Install Podman: https://podman.io/docs/installation\n" +
				"  Ubuntu/Debian: sudo apt-get install -y podman\n" +
				"  Fedora: sudo dnf install -y podman"
		} else {
			result.Remediation = "Install Docker (rootless): https://docs.docker.com/engine/install/"
		}
		return result
	}

	// Check version.
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("%s found at %s but failed to get version: %v", runtime, path, err)
		result.Remediation = fmt.Sprintf("Check that %s is correctly installed and functional", runtime)
		return result
	}

	version := strings.TrimSpace(string(out))

	// Verify it can actually run (not just installed).
	if err := exec.Command(path, "info", "--format", "{{.Host.OS}}").Run(); err != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("%s installed (%s) but 'info' command failed -- service may not be running", runtime, version)
		if runtime == "podman" {
			result.Remediation = "Podman is daemonless and should work without a service.\n" +
				"  Try running: podman info"
		} else {
			result.Remediation = "Ensure Docker daemon is running: sudo systemctl start docker"
		}
		return result
	}

	result.Status = "pass"
	result.Message = fmt.Sprintf("%s: %s", runtime, version)
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
		result.Status = "fail"
		result.Message = "runsc (gVisor) binary not found"
		result.Remediation = "Install gVisor:\n" +
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

	// Check if runtime is registered with podman.
	rtPath, _ := exec.LookPath(cfg.Runtime)
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
		result.Remediation = "Check permissions: sudo aa-status"
		return result
	}

	if !loaded {
		result.Status = "fail"
		result.Message = "aibox-sandbox AppArmor profile is not loaded"
		result.Remediation = "Load the profile:\n" +
			"  sudo apparmor_parser -r configs/apparmor/aibox-sandbox\n" +
			"  Or run: aibox setup"
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
		"  sudo mkdir -p /etc/aibox\n" +
		"  sudo cp configs/seccomp.json /etc/aibox/seccomp.json\n" +
		"  Or run: aibox setup"
	return result
}

// CheckImage verifies the base container image is available locally.
func CheckImage(cfg *config.Config) CheckResult {
	result := CheckResult{Name: "Container Image"}

	rtPath, err := exec.LookPath(cfg.Runtime)
	if err != nil {
		result.Status = "warn"
		result.Message = "cannot check image: container runtime not found"
		return result
	}

	if err := exec.Command(rtPath, "image", "exists", cfg.Image).Run(); err != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("image %s not cached locally", cfg.Image)
		result.Remediation = fmt.Sprintf("Pull the image:\n  %s pull %s\n  Or run: aibox update", cfg.Runtime, cfg.Image)
		return result
	}

	// Check image age.
	out, err := exec.Command(rtPath, "image", "inspect", "--format", "{{.Created}}", cfg.Image).Output()
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

// CheckDiskSpace verifies sufficient disk space for aibox operations.
func CheckDiskSpace() CheckResult {
	result := CheckResult{Name: "Disk Space"}

	home, err := os.UserHomeDir()
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

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
