package container

import (
	"os"
	"strings"
	"testing"
)

func TestValidateWorkspace_ValidDir(t *testing.T) {
	dir := t.TempDir()

	absPath, err := ValidateWorkspace(dir)
	if err != nil {
		t.Fatalf("ValidateWorkspace(%q) returned error: %v", dir, err)
	}

	if absPath == "" {
		t.Error("ValidateWorkspace() returned empty path")
	}
}

func TestValidateWorkspace_NonExistent(t *testing.T) {
	_, err := ValidateWorkspace("/nonexistent/path/abc123")
	if err == nil {
		t.Fatal("ValidateWorkspace() should return error for non-existent path")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error should mention 'does not exist', got: %v", err)
	}
}

func TestValidateWorkspace_FileNotDir(t *testing.T) {
	tmp, err := os.CreateTemp("", "aibox-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	_, err = ValidateWorkspace(tmp.Name())
	if err == nil {
		t.Fatal("ValidateWorkspace() should return error for a regular file")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error should mention 'not a directory', got: %v", err)
	}
}

func TestIsNTFSPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/mnt/c/Users/alice/project", true},
		{"/mnt/d/code/repo", true},
		{"/mnt/z/deep/path", true},
		{"/home/user/project", false},
		{"/workspace", false},
		{"/tmp/test", false},
		{"/mnt/", false},       // no drive letter
		{"/mnt/C/foo", false},  // uppercase (not a-z)
	}

	for _, tt := range tests {
		got := isNTFSPath(tt.path)
		if got != tt.want {
			t.Errorf("isNTFSPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestValidateWorkspace_NTFSPath(t *testing.T) {
	// We can't actually create /mnt/c/ in a test, but we can verify the
	// path validation logic by checking that known NTFS paths that don't
	// exist return the "does not exist" error (not the NTFS error, since
	// the path check happens first).
	_, err := ValidateWorkspace("/mnt/c/Users/test/project")
	if err == nil {
		t.Fatal("ValidateWorkspace() should return error for /mnt/c/ path")
	}
	// On a non-WSL2 system, the path won't exist, so we get "does not exist".
	// On WSL2, if the path exists, we'd get the NTFS error.
	// Either error is acceptable for this test.
}
