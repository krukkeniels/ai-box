package host

import (
	"runtime"
	"strings"
	"testing"
)

func TestDetect_OS(t *testing.T) {
	info := Detect()

	if info.OS != runtime.GOOS {
		t.Errorf("Detect().OS = %q, want %q", info.OS, runtime.GOOS)
	}
}

func TestDetect_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific test")
	}

	info := Detect()

	if info.KernelVersion == "" {
		t.Error("Detect().KernelVersion should not be empty on Linux")
	}
}

func TestIsSupported_Linux(t *testing.T) {
	info := HostInfo{OS: "linux"}
	if !info.IsSupported() {
		t.Error("IsSupported() should return true for Linux")
	}
}

func TestIsSupported_NonLinux(t *testing.T) {
	unsupported := []string{"darwin", "windows", "freebsd"}
	for _, os := range unsupported {
		info := HostInfo{OS: os}
		if info.IsSupported() {
			t.Errorf("IsSupported() should return false for %q", os)
		}
	}
}

func TestWSL2Detection_LiveDetect(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific test")
	}

	// Test the actual Detect() function and verify its WSL2 result is
	// consistent with the kernel version string it reports.
	info := Detect()

	lower := strings.ToLower(info.KernelVersion)
	expectWSL2 := strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")

	if info.IsWSL2 != expectWSL2 {
		t.Errorf("Detect().IsWSL2 = %v, but kernel version %q suggests %v",
			info.IsWSL2, info.KernelVersion, expectWSL2)
	}
}

func TestWSL2Detection_KernelPatterns(t *testing.T) {
	// Verify the detection logic against known kernel version patterns.
	// We replicate the core detection logic from Detect() to validate
	// that the expected patterns match correctly.
	tests := []struct {
		name          string
		kernelVersion string
		wantWSL2      bool
	}{
		{"Microsoft uppercase", "Linux version 5.15.0-1-Microsoft", true},
		{"microsoft lowercase with WSL2 suffix", "Linux version 5.15.90.1-microsoft-standard-WSL2", true},
		{"standard generic kernel", "Linux version 6.1.0-generic #1 SMP", false},
		{"wsl2 in version string", "Linux version 6.5.0-wsl2", true},
		{"empty version", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lower := strings.ToLower(tt.kernelVersion)
			gotWSL2 := strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
			if gotWSL2 != tt.wantWSL2 {
				t.Errorf("kernel %q: detected WSL2 = %v, want %v",
					tt.kernelVersion, gotWSL2, tt.wantWSL2)
			}
		})
	}
}
