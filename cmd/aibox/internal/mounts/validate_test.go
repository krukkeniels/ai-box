package mounts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathIsUnder(t *testing.T) {
	tests := []struct {
		path       string
		mountPoint string
		want       bool
	}{
		// Root mount matches everything.
		{"/home/dev/project", "/", true},
		{"/", "/", true},

		// Exact match.
		{"/home/dev", "/home/dev", true},

		// Subdirectory match.
		{"/home/dev/project", "/home/dev", true},
		{"/home/dev/project/src", "/home/dev", true},

		// Must not match partial path components.
		{"/home/developer", "/home/dev", false},
		{"/home/dev2", "/home/dev", false},

		// Unrelated paths.
		{"/var/log", "/home/dev", false},
		{"/tmp", "/home", false},

		// Trailing slashes should not matter (paths are cleaned).
		{"/home/dev/", "/home/dev", true},
	}

	for _, tt := range tests {
		got := pathIsUnder(tt.path, tt.mountPoint)
		if got != tt.want {
			t.Errorf("pathIsUnder(%q, %q) = %v, want %v", tt.path, tt.mountPoint, got, tt.want)
		}
	}
}

func TestBlockedFilesystems(t *testing.T) {
	expected := []string{"9p", "drvfs", "ntfs", "ntfs3", "vfat"}
	for _, fs := range expected {
		if _, ok := BlockedFilesystems[fs]; !ok {
			t.Errorf("BlockedFilesystems missing %q", fs)
		}
	}
}

func TestAllowedFilesystems(t *testing.T) {
	expected := []string{"ext4", "ext3", "xfs", "btrfs", "overlay", "tmpfs", "zfs"}
	for _, fs := range expected {
		if !AllowedFilesystems[fs] {
			t.Errorf("AllowedFilesystems missing %q", fs)
		}
	}
}

func TestBlockedNotInAllowed(t *testing.T) {
	for fs := range BlockedFilesystems {
		if AllowedFilesystems[fs] {
			t.Errorf("filesystem %q is in both blocked and allowed lists", fs)
		}
	}
}

func TestValidateWorkspace_NonExistentPath(t *testing.T) {
	err := ValidateWorkspace("/nonexistent/path/abc123", true)
	if err == nil {
		t.Fatal("ValidateWorkspace() should return error for non-existent path")
	}
}

func TestValidateWorkspace_FileNotDir(t *testing.T) {
	// Create a temporary file (not a directory).
	tmp, err := os.CreateTemp("", "aibox-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	err = ValidateWorkspace(tmp.Name(), true)
	if err == nil {
		t.Fatal("ValidateWorkspace() should return error for a file (not directory)")
	}
}

func TestValidateWorkspace_ValidDir(t *testing.T) {
	// Create a temporary directory.
	dir, err := os.MkdirTemp("", "aibox-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// With FS validation enabled. On a normal Linux test host this should
	// be on ext4/xfs/btrfs/tmpfs, all of which are allowed.
	err = ValidateWorkspace(dir, true)
	if err != nil {
		t.Errorf("ValidateWorkspace() returned error for valid temp dir: %v", err)
	}
}

func TestValidateWorkspace_SkipFSValidation(t *testing.T) {
	dir, err := os.MkdirTemp("", "aibox-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// With FS validation disabled, any existing directory should pass.
	err = ValidateWorkspace(dir, false)
	if err != nil {
		t.Errorf("ValidateWorkspace() with validateFS=false returned error: %v", err)
	}
}

func TestValidateWorkspace_RelativePath(t *testing.T) {
	// ValidateWorkspace should resolve relative paths via filepath.Abs.
	// Use the current working directory which should be valid.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	err = ValidateWorkspace(".", false)
	if err != nil {
		t.Errorf("ValidateWorkspace(\".\") returned error (cwd=%s): %v", cwd, err)
	}
}

func TestDetectFilesystem(t *testing.T) {
	// Test on the temp directory, which should be on a known filesystem.
	dir, err := os.MkdirTemp("", "aibox-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	absDir, _ := filepath.Abs(dir)

	fsType, err := detectFilesystem(absDir)
	if err != nil {
		t.Fatalf("detectFilesystem(%q) returned error: %v", absDir, err)
	}

	if fsType == "" {
		t.Fatal("detectFilesystem() returned empty filesystem type")
	}

	t.Logf("detected filesystem type for %s: %s", absDir, fsType)

	// The temp dir should be on an allowed or at least non-blocked filesystem.
	if _, blocked := BlockedFilesystems[fsType]; blocked {
		t.Errorf("temp directory is on blocked filesystem %q -- unexpected for test host", fsType)
	}
}
