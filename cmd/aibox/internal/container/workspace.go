package container

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aibox/aibox/internal/mounts"
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
			"workspace %q is on an NTFS mount (Windows filesystem)\n\n"+
				"  WHY: Files under /mnt/c/ use a 9p/drvfs translation layer that is\n"+
				"  3-10x slower than the native ext4 filesystem inside WSL2.\n\n"+
				"  FIX:\n"+
				"    1. Clone inside WSL2:  cd ~ && git clone <repo-url>\n"+
				"    2. Start from there:   aibox start --workspace ~/your-repo\n"+
				"    3. Open IDE into WSL2: VS Code > 'Connect to WSL' or JetBrains Gateway\n\n"+
				"  ALREADY CLONED? Make sure you pass the WSL2 path, not the Windows path:\n"+
				"    Wrong:   aibox start --workspace /mnt/c/Users/you/repos/project\n"+
				"    Correct: aibox start --workspace ~/repos/project\n\n"+
				"  VERIFY: Run 'df -h .' inside your repo — the Type column should show ext4, not 9p/drvfs.",
			absPath,
		)
	}

	// Deep filesystem check via /proc/mounts — catches edge cases the
	// path-prefix check misses (custom mount points, bind mounts, non-standard
	// WSL2 configurations where Windows drives aren't at /mnt/).
	if err := mounts.ValidateWorkspace(absPath, true); err != nil {
		return "", err
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
