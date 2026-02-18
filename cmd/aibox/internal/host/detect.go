package host

import (
	"os"
	"runtime"
	"strings"
)

// HostInfo describes the host operating system environment.
type HostInfo struct {
	OS            string // "linux", "darwin", "windows", etc.
	IsWSL2        bool
	KernelVersion string
}

// Detect inspects the current host and returns a HostInfo.
func Detect() HostInfo {
	info := HostInfo{
		OS: runtime.GOOS,
	}

	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/version")
		if err == nil {
			ver := string(data)
			info.KernelVersion = strings.TrimSpace(ver)
			lower := strings.ToLower(ver)
			if strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl") {
				info.IsWSL2 = true
			}
		}
	}

	return info
}

// IsSupported returns true if the host OS is supported (native Linux or WSL2).
func (h HostInfo) IsSupported() bool {
	return h.OS == "linux"
}
