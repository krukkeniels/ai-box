package setup

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/aibox/aibox/internal/host"
)

func TestCheckPreflightOS(t *testing.T) {
	tests := []struct {
		name   string
		info   host.HostInfo
		status string
		substr string
	}{
		{"native linux", host.HostInfo{OS: "linux"}, "pass", "native"},
		{"wsl2", host.HostInfo{OS: "linux", IsWSL2: true}, "pass", "WSL2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := checkPreflightOS(tt.info)
			if r.Status != tt.status {
				t.Errorf("status = %q, want %q (%s)", r.Status, tt.status, r.Message)
			}
			if tt.substr != "" && !contains(r.Message, tt.substr) {
				t.Errorf("message %q should contain %q", r.Message, tt.substr)
			}
		})
	}
}

func TestCheckPreflightWSL2(t *testing.T) {
	tests := []struct {
		name   string
		info   host.HostInfo
		status string
	}{
		{"not wsl2", host.HostInfo{OS: "linux", IsWSL2: false}, "pass"},
		{
			"old kernel",
			host.HostInfo{OS: "linux", IsWSL2: true, KernelVersion: "Linux version 5.10.0-microsoft-standard-WSL2"},
			"warn",
		},
		{
			"new kernel 5.15",
			host.HostInfo{OS: "linux", IsWSL2: true, KernelVersion: "Linux version 5.15.90.1-microsoft-standard-WSL2"},
			"pass",
		},
		{
			"new kernel 6.x",
			host.HostInfo{OS: "linux", IsWSL2: true, KernelVersion: "Linux version 6.1.21-microsoft-standard-WSL2+"},
			"pass",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := checkPreflightWSL2(tt.info)
			if r.Status != tt.status {
				t.Errorf("status = %q, want %q (%s)", r.Status, tt.status, r.Message)
			}
		})
	}
}

func TestParsePreflightKernelVersion(t *testing.T) {
	tests := []struct {
		input string
		major int
		minor int
	}{
		{"Linux version 5.15.90.1-microsoft-standard-WSL2", 5, 15},
		{"Linux version 6.1.21-microsoft-standard-WSL2+", 6, 1},
		{"Linux version 5.4.0-150-generic", 5, 4},
		{"Linux version 6.17.0-14-generic", 6, 17},
		{"", 0, 0},
		{"no version here", 0, 0},
	}
	for _, tt := range tests {
		major, minor := parsePreflightKernelVersion(tt.input)
		if major != tt.major || minor != tt.minor {
			t.Errorf("parsePreflightKernelVersion(%q) = %d.%d, want %d.%d", tt.input, major, minor, tt.major, tt.minor)
		}
	}
}

func TestParseTotalMemoryGB(t *testing.T) {
	tests := []struct {
		name string
		data string
		want int
	}{
		{
			"32 GB",
			"MemTotal:       33554432 kB\nMemFree:        16777216 kB\n",
			32,
		},
		{
			"8 GB",
			"MemTotal:        8388608 kB\nMemFree:         4194304 kB\n",
			8,
		},
		{
			"2 GB",
			"MemTotal:        2097152 kB\nMemFree:         1048576 kB\n",
			2,
		},
		{
			"empty", "", 0,
		},
		{
			"no MemTotal",
			"MemFree:         1048576 kB\n",
			0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTotalMemoryGB([]byte(tt.data))
			if got != tt.want {
				t.Errorf("parseTotalMemoryGB() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCheckPreflightRAM_Live(t *testing.T) {
	// On a real machine, RAM check should return a valid status.
	r := checkPreflightRAM()
	if r.Status == "" {
		t.Error("expected a status from checkPreflightRAM")
	}
}

func TestCheckPreflightDisk_Live(t *testing.T) {
	r := checkPreflightDisk()
	if r.Status == "" {
		t.Error("expected a status from checkPreflightDisk")
	}
}

func TestCheckPreflightDisk_LowSpace(t *testing.T) {
	orig := statfsFunc
	defer func() { statfsFunc = orig }()

	// Simulate 3 GB free (should fail).
	statfsFunc = func(path string) (uint64, uint64, error) {
		gb3 := uint64(3) * 1024 * 1024 * 1024
		return gb3, 1, nil // avail * bsize = 3 GB
	}

	r := checkPreflightDisk()
	if r.Status != "fail" {
		t.Errorf("expected fail for 3 GB free, got %q: %s", r.Status, r.Message)
	}
}

func TestCheckPreflightDisk_WarnSpace(t *testing.T) {
	orig := statfsFunc
	defer func() { statfsFunc = orig }()

	// Simulate 10 GB free (should warn).
	statfsFunc = func(path string) (uint64, uint64, error) {
		gb10 := uint64(10) * 1024 * 1024 * 1024
		return gb10, 1, nil
	}

	r := checkPreflightDisk()
	if r.Status != "warn" {
		t.Errorf("expected warn for 10 GB free, got %q: %s", r.Status, r.Message)
	}
}

func TestCheckPreflightDisk_Plenty(t *testing.T) {
	orig := statfsFunc
	defer func() { statfsFunc = orig }()

	// Simulate 100 GB free (should pass).
	statfsFunc = func(path string) (uint64, uint64, error) {
		gb100 := uint64(100) * 1024 * 1024 * 1024
		return gb100, 1, nil
	}

	r := checkPreflightDisk()
	if r.Status != "pass" {
		t.Errorf("expected pass for 100 GB free, got %q: %s", r.Status, r.Message)
	}
}

func TestCheckPreflightDisk_Error(t *testing.T) {
	orig := statfsFunc
	defer func() { statfsFunc = orig }()

	statfsFunc = func(path string) (uint64, uint64, error) {
		return 0, 0, errors.New("permission denied")
	}

	r := checkPreflightDisk()
	if r.Status != "warn" {
		t.Errorf("expected warn on statfs error, got %q: %s", r.Status, r.Message)
	}
}

func TestCheckPreflightPodman_NotFound(t *testing.T) {
	orig := lookPathFunc
	defer func() { lookPathFunc = orig }()

	lookPathFunc = func(name string) (string, error) {
		return "", errors.New("not found")
	}

	r := checkPreflightPodman()
	if r.Status != "fail" {
		t.Errorf("expected fail when podman not found, got %q: %s", r.Status, r.Message)
	}
}

func TestCheckPreflightGVisor_NotFound(t *testing.T) {
	orig := lookPathFunc
	defer func() { lookPathFunc = orig }()

	lookPathFunc = func(name string) (string, error) {
		return "", errors.New("not found")
	}

	r := checkPreflightGVisor()
	if r.Status != "warn" {
		t.Errorf("expected warn when runsc not found, got %q: %s", r.Status, r.Message)
	}
}

func TestCheckPreflightNetwork_Unreachable(t *testing.T) {
	orig := dialTimeoutFunc
	defer func() { dialTimeoutFunc = orig }()

	dialTimeoutFunc = func(network, address string, timeout time.Duration) (net.Conn, error) {
		return nil, errors.New("connection refused")
	}

	r := checkPreflightNetwork()
	if r.Status != "warn" {
		t.Errorf("expected warn when network unreachable, got %q: %s", r.Status, r.Message)
	}
}

func TestPreflightReport_HasFailures(t *testing.T) {
	tests := []struct {
		name     string
		statuses []string
		want     bool
	}{
		{"all pass", []string{"pass", "pass"}, false},
		{"with warn", []string{"pass", "warn"}, false},
		{"with fail", []string{"pass", "fail"}, true},
		{"all fail", []string{"fail", "fail"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &PreflightReport{}
			for i, s := range tt.statuses {
				report.Results = append(report.Results, PreflightResult{
					Name:   "test" + string(rune('0'+i)),
					Status: s,
				})
			}
			if got := report.HasFailures(); got != tt.want {
				t.Errorf("HasFailures() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunPreflight_Returns7Checks(t *testing.T) {
	report := RunPreflight()
	if len(report.Results) != 7 {
		t.Errorf("expected 7 preflight checks, got %d", len(report.Results))
	}
	for _, r := range report.Results {
		if r.Name == "" {
			t.Error("check has empty name")
		}
		if r.Status == "" {
			t.Errorf("check %q has empty status", r.Name)
		}
	}
}

func TestCheckPreflightRAM_WithMockedMeminfo(t *testing.T) {
	tests := []struct {
		name   string
		data   string
		status string
	}{
		{
			"2 GB - fail",
			"MemTotal:        2097152 kB\n",
			"fail",
		},
		{
			"8 GB - warn",
			"MemTotal:        8388608 kB\n",
			"warn",
		},
		{
			"32 GB - pass",
			"MemTotal:       33554432 kB\n",
			"pass",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := readFileFunc
			defer func() { readFileFunc = orig }()

			readFileFunc = func(name string) ([]byte, error) {
				if name == "/proc/meminfo" {
					return []byte(tt.data), nil
				}
				return nil, errors.New("not found")
			}

			r := checkPreflightRAM()
			if r.Status != tt.status {
				t.Errorf("status = %q, want %q (%s)", r.Status, tt.status, r.Message)
			}
		})
	}
}

// contains is a test helper to check substring presence.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
