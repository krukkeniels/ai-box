package container

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateWorkspace checks that the workspace path exists, is a directory,
// and is on a native Linux filesystem (not NTFS mounted via WSL2).
func ValidateWorkspace(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving workspace path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("workspace path does not exist: %s", absPath)
		}
		return "", fmt.Errorf("checking workspace path: %w", err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("workspace path is not a directory: %s", absPath)
	}

	if isNTFSPath(absPath) {
		return "", fmt.Errorf(
			"workspace %q appears to be on an NTFS mount (Windows filesystem)\n"+
				"  Performance will be 3-10x slower. Clone your repo inside the WSL2 filesystem instead:\n"+
				"    cd ~ && git clone <repo-url>",
			absPath,
		)
	}

	return absPath, nil
}

// isNTFSPath detects common patterns for NTFS-mounted paths in WSL2.
func isNTFSPath(path string) bool {
	// WSL2 mounts Windows drives under /mnt/c, /mnt/d, etc.
	if strings.HasPrefix(path, "/mnt/") && len(path) > 5 {
		drive := path[5]
		if drive >= 'a' && drive <= 'z' {
			return true
		}
	}
	return false
}
