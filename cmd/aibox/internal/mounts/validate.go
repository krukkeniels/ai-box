package mounts

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AllowedFilesystems is the set of filesystem types considered safe for
// workspace mounts. NTFS-backed filesystems (9p, drvfs) are explicitly
// excluded because they are 3-20x slower for development workflows.
var AllowedFilesystems = map[string]bool{
	"ext4":    true,
	"ext3":    true,
	"xfs":     true,
	"btrfs":   true,
	"overlay": true,
	"tmpfs":   true,
	"zfs":     true,
}

// BlockedFilesystems is the set of filesystem types that are blocked with
// an explicit error message.
var BlockedFilesystems = map[string]string{
	"9p":    "Windows NTFS mounted via WSL2 (Plan 9 / 9p) -- 3-10x slower than native ext4",
	"drvfs": "Windows NTFS mounted via WSL2 (drvfs) -- 3-10x slower than native ext4",
	"ntfs":  "NTFS filesystem -- not supported for development workspaces",
	"ntfs3": "NTFS filesystem (ntfs3 driver) -- not supported for development workspaces",
	"vfat":  "FAT filesystem -- not supported for development workspaces",
}

// ValidateWorkspace checks that the workspace path:
//  1. Exists on the host
//  2. Is on a supported filesystem (not NTFS-backed)
//
// Returns nil if the workspace is valid.
func ValidateWorkspace(path string, validateFS bool) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving workspace path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("workspace path does not exist: %s", absPath)
		}
		return fmt.Errorf("checking workspace path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace path is not a directory: %s", absPath)
	}

	if !validateFS {
		return nil
	}

	fsType, err := detectFilesystem(absPath)
	if err != nil {
		// If we cannot determine the filesystem type, log a warning but
		// do not block. This covers unusual /proc layouts.
		return nil
	}

	if reason, blocked := BlockedFilesystems[fsType]; blocked {
		return fmt.Errorf(
			"workspace %s is on a blocked filesystem (%s): %s\n\n"+
				"Remediation: clone your repository inside the WSL2 filesystem:\n"+
				"  cd ~\n"+
				"  git clone <repo-url>\n"+
				"  aibox start --workspace ~/your-repo",
			absPath, fsType, reason,
		)
	}

	return nil
}

// detectFilesystem reads /proc/mounts to find the filesystem type of the
// mount point containing the given path. It walks up the directory tree
// to find the longest matching mount point.
func detectFilesystem(path string) (string, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return "", err
	}
	defer f.Close()

	type mountEntry struct {
		mountPoint string
		fsType     string
	}

	var entries []mountEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		entries = append(entries, mountEntry{
			mountPoint: fields[1],
			fsType:     fields[2],
		})
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	// Find the longest mount point prefix that matches our path.
	// Use proper path boundary matching to avoid false prefix matches
	// (e.g. mount "/home/de" should not match path "/home/dev").
	var bestMatch mountEntry
	for _, e := range entries {
		if !pathIsUnder(path, e.mountPoint) {
			continue
		}
		if len(e.mountPoint) > len(bestMatch.mountPoint) {
			bestMatch = e
		}
	}

	if bestMatch.mountPoint == "" {
		return "", fmt.Errorf("no matching mount point found for %s", path)
	}

	return bestMatch.fsType, nil
}

// pathIsUnder returns true if path is equal to or a subdirectory of mountPoint,
// using proper path boundary matching.
func pathIsUnder(path, mountPoint string) bool {
	if mountPoint == "/" {
		return true
	}
	return path == mountPoint || strings.HasPrefix(path, mountPoint+"/")
}
