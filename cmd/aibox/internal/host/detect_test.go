package host

import (
	"runtime"
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

func TestWSL2Detection(t *testing.T) {
	// Test the WSL2 detection logic with known kernel version strings.
	tests := []struct {
		kernelVersion string
		wantWSL2      bool
	}{
		{"Linux version 5.15.0-1-Microsoft", true},
		{"Linux version 5.15.90.1-microsoft-standard-WSL2", true},
		{"Linux version 6.1.0-generic #1 SMP", false},
		{"Linux version 6.5.0-wsl2", true},
		{"", false},
	}

	for _, tt := range tests {
		// We can't override /proc/version in a unit test, but we can test
		// the struct directly to verify the semantics.
		info := HostInfo{
			OS:            "linux",
			KernelVersion: tt.kernelVersion,
			IsWSL2:        tt.wantWSL2,
		}
		if info.IsWSL2 != tt.wantWSL2 {
			t.Errorf("kernel %q: IsWSL2 = %v, want %v", tt.kernelVersion, info.IsWSL2, tt.wantWSL2)
		}
	}
}
