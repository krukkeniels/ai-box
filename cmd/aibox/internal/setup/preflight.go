package setup

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aibox/aibox/internal/config"
	"github.com/aibox/aibox/internal/host"
)

// PreflightResult represents the outcome of a single preflight check.
type PreflightResult struct {
	Name    string // check name
	Status  string // "pass", "warn", "fail"
	Message string // human-readable detail
}

// PreflightReport collects all preflight results.
type PreflightReport struct {
	Results []PreflightResult
}

// HasFailures returns true if any check failed.
func (r *PreflightReport) HasFailures() bool {
	for _, c := range r.Results {
		if c.Status == "fail" {
			return true
		}
	}
	return false
}

// --- Overridable functions for testing ---

var (
	detectHostFunc  = host.Detect
	lookPathFunc    = exec.LookPath
	readFileFunc    = os.ReadFile
	statfsFunc      = defaultStatfs
	dialTimeoutFunc = net.DialTimeout
)

func defaultStatfs(path string) (avail uint64, bsize uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	return st.Bavail, uint64(st.Bsize), nil
}

// RunPreflight executes all pre-flight checks before setup.
func RunPreflight() *PreflightReport {
	hostInfo := detectHostFunc()

	report := &PreflightReport{}
	checks := []func() PreflightResult{
		func() PreflightResult { return checkPreflightOS(hostInfo) },
		func() PreflightResult { return checkPreflightWSL2(hostInfo) },
		func() PreflightResult { return checkPreflightRAM() },
		func() PreflightResult { return checkPreflightDisk() },
		func() PreflightResult { return checkPreflightPodman() },
		func() PreflightResult { return checkPreflightGVisor() },
		func() PreflightResult { return checkPreflightNetwork() },
	}

	for _, check := range checks {
		report.Results = append(report.Results, check())
	}
	return report
}

// PrintPreflight runs pre-flight checks with step-by-step output.
// Returns an error if any check is fatal.
func PrintPreflight() error {
	checks := []struct {
		Name string
		Fn   func() PreflightResult
	}{
		{"Checking OS", func() PreflightResult { return checkPreflightOS(detectHostFunc()) }},
		{"Detecting WSL2", func() PreflightResult { return checkPreflightWSL2(detectHostFunc()) }},
		{"Checking available RAM", func() PreflightResult { return checkPreflightRAM() }},
		{"Checking disk space", func() PreflightResult { return checkPreflightDisk() }},
		{"Checking container runtime", func() PreflightResult { return checkPreflightPodman() }},
		{"Checking gVisor sandbox", func() PreflightResult { return checkPreflightGVisor() }},
		{"Checking network connectivity", func() PreflightResult { return checkPreflightNetwork() }},
	}

	var failures []PreflightResult
	for _, c := range checks {
		result := c.Fn()
		switch result.Status {
		case "pass":
			fmt.Printf("  [OK]   %s: %s\n", c.Name, result.Message)
		case "warn":
			fmt.Printf("  [WARN] %s: %s\n", c.Name, result.Message)
		case "fail":
			fmt.Printf("  [FAIL] %s: %s\n", c.Name, result.Message)
			failures = append(failures, result)
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("pre-flight checks failed: %d fatal issue(s)", len(failures))
	}
	return nil
}

// --- Individual checks ---

func checkPreflightOS(info host.HostInfo) PreflightResult {
	r := PreflightResult{Name: "OS version"}
	if runtime.GOOS != "linux" {
		r.Status = "fail"
		r.Message = fmt.Sprintf("unsupported OS: %s (Linux required)", runtime.GOOS)
		return r
	}
	r.Status = "pass"
	if info.IsWSL2 {
		r.Message = "Linux (WSL2)"
	} else {
		r.Message = "Linux (native)"
	}
	return r
}

func checkPreflightWSL2(info host.HostInfo) PreflightResult {
	r := PreflightResult{Name: "WSL2 detection"}
	if !info.IsWSL2 {
		r.Status = "pass"
		r.Message = "native Linux (not WSL2)"
		return r
	}
	major, minor := parsePreflightKernelVersion(info.KernelVersion)
	if major < 5 || (major == 5 && minor < 15) {
		r.Status = "warn"
		r.Message = fmt.Sprintf("WSL2 kernel too old for gVisor systrap (need 5.15+, have %d.%d)", major, minor)
		return r
	}
	r.Status = "pass"
	r.Message = fmt.Sprintf("WSL2 kernel %d.%d", major, minor)
	return r
}

func checkPreflightRAM() PreflightResult {
	r := PreflightResult{Name: "RAM"}

	memGB := getPreflightTotalMemoryGB()
	if memGB == 0 {
		r.Status = "warn"
		r.Message = "could not detect RAM"
		return r
	}

	if memGB < 4 {
		r.Status = "fail"
		r.Message = fmt.Sprintf("%d GB (minimum 4 GB required)", memGB)
		return r
	}
	if memGB < 16 {
		r.Status = "warn"
		r.Message = fmt.Sprintf("%d GB (16+ GB recommended; consider .wslconfig on WSL2)", memGB)
		return r
	}
	r.Status = "pass"
	r.Message = fmt.Sprintf("%d GB", memGB)
	return r
}

func checkPreflightDisk() PreflightResult {
	r := PreflightResult{Name: "Disk space"}

	home, err := config.ResolveHomeDir()
	if err != nil {
		r.Status = "warn"
		r.Message = "could not determine home directory"
		return r
	}

	avail, bsize, err := statfsFunc(home)
	if err != nil {
		r.Status = "warn"
		r.Message = fmt.Sprintf("could not check disk: %v", err)
		return r
	}

	freeGB := (avail * bsize) / (1024 * 1024 * 1024)

	if freeGB < 5 {
		r.Status = "fail"
		r.Message = fmt.Sprintf("%d GB free (minimum 5 GB required)", freeGB)
		return r
	}
	if freeGB < 20 {
		r.Status = "warn"
		r.Message = fmt.Sprintf("%d GB free (20+ GB recommended)", freeGB)
		return r
	}
	r.Status = "pass"
	r.Message = fmt.Sprintf("%d GB free", freeGB)
	return r
}

func checkPreflightPodman() PreflightResult {
	r := PreflightResult{Name: "Podman"}

	path, err := lookPathFunc("podman")
	if err != nil {
		// Try docker as fallback.
		if dPath, dErr := lookPathFunc("docker"); dErr == nil {
			out, _ := exec.Command(dPath, "--version").Output()
			r.Status = "pass"
			r.Message = fmt.Sprintf("docker found (%s)", strings.TrimSpace(string(out)))
			return r
		}
		r.Status = "fail"
		r.Message = "podman not found in PATH"
		return r
	}

	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		r.Status = "fail"
		r.Message = "podman found but version check failed"
		return r
	}

	r.Status = "pass"
	r.Message = strings.TrimSpace(string(out))
	return r
}

func checkPreflightGVisor() PreflightResult {
	r := PreflightResult{Name: "gVisor (runsc)"}

	runscPath, err := lookPathFunc("runsc")
	if err != nil {
		for _, p := range []string{"/usr/local/bin/runsc", "/usr/bin/runsc"} {
			if _, serr := os.Stat(p); serr == nil {
				runscPath = p
				break
			}
		}
	}

	if runscPath == "" {
		r.Status = "warn"
		r.Message = "not installed (optional but recommended)"
		return r
	}

	out, err := exec.Command(runscPath, "--version").Output()
	if err != nil {
		r.Status = "warn"
		r.Message = "found but version check failed"
		return r
	}

	ver := strings.TrimSpace(string(out))
	if idx := strings.IndexByte(ver, '\n'); idx >= 0 {
		ver = ver[:idx]
	}
	r.Status = "pass"
	r.Message = ver
	return r
}

func checkPreflightNetwork() PreflightResult {
	r := PreflightResult{Name: "Network connectivity"}

	conn, err := dialTimeoutFunc("tcp", "github.com:443", 5*time.Second)
	if err != nil {
		r.Status = "warn"
		r.Message = "cannot reach github.com (air-gapped or restricted network)"
		return r
	}
	conn.Close()

	r.Status = "pass"
	r.Message = "github.com reachable"
	return r
}

// --- Helpers ---

// parsePreflightKernelVersion extracts major.minor from a /proc/version string.
func parsePreflightKernelVersion(ver string) (int, int) {
	fields := strings.Fields(ver)
	verStr := ""
	for _, f := range fields {
		if len(f) > 0 && f[0] >= '0' && f[0] <= '9' {
			verStr = f
			break
		}
	}
	if verStr == "" {
		return 0, 0
	}
	parts := strings.SplitN(verStr, ".", 3)
	if len(parts) < 2 {
		return 0, 0
	}
	major, _ := strconv.Atoi(parts[0])
	minorStr := strings.SplitN(parts[1], "-", 2)[0]
	minor, _ := strconv.Atoi(minorStr)
	return major, minor
}

// getPreflightTotalMemoryGB reads total memory from /proc/meminfo.
func getPreflightTotalMemoryGB() int {
	data, err := readFileFunc("/proc/meminfo")
	if err != nil {
		return 0
	}
	return parseTotalMemoryGB(data)
}

// parseTotalMemoryGB extracts total memory in GB from /proc/meminfo content.
func parseTotalMemoryGB(data []byte) int {
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.ParseInt(fields[1], 10, 64)
				if err != nil {
					return 0
				}
				return int(kb / (1024 * 1024))
			}
		}
	}
	return 0
}
