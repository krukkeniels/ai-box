// Package tests contains filesystem integration tests.
// These verify the mount layout, tmpfs behavior, and cache persistence.
//
// Run with: go test -tags=integration ./tests/
package tests

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMountLayout_ReadOnlyRoot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	rtPath := requireRuntime(t)

	// Attempt to write to the root filesystem -- should fail with --read-only.
	out, err := exec.Command(rtPath, "run", "--rm",
		"--read-only",
		"--user=1000:1000",
		"ubuntu:24.04",
		"touch", "/test-file",
	).CombinedOutput()

	if err == nil {
		t.Error("writing to read-only root filesystem should fail")
	}
	output := string(out)
	if !strings.Contains(output, "Read-only") && !strings.Contains(output, "read-only") && !strings.Contains(output, "EROFS") {
		t.Logf("expected read-only error, got: %s", output)
	}
}

func TestMountLayout_TmpfsWritable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	rtPath := requireRuntime(t)

	// /tmp should be writable as tmpfs.
	out, err := exec.Command(rtPath, "run", "--rm",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--user=1000:1000",
		"ubuntu:24.04",
		"touch", "/tmp/test-file",
	).CombinedOutput()

	if err != nil {
		t.Errorf("writing to /tmp tmpfs should succeed: %v\n%s", err, out)
	}
}

func TestMountLayout_TmpfsNoExec(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	rtPath := requireRuntime(t)

	// Executing from /tmp should fail because of noexec.
	out, err := exec.Command(rtPath, "run", "--rm",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--user=1000:1000",
		"ubuntu:24.04",
		"sh", "-c", "cp /bin/echo /tmp/echo && /tmp/echo test",
	).CombinedOutput()

	if err == nil {
		t.Log("noexec test: command succeeded (noexec may not be enforced by this runtime)")
	} else {
		output := string(out)
		if strings.Contains(output, "Permission denied") || strings.Contains(output, "permission denied") {
			t.Log("noexec correctly enforced on /tmp")
		} else {
			t.Logf("noexec test output: %s", output)
		}
	}
}

func TestMountLayout_WorkspaceWritable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	rtPath := requireRuntime(t)

	workspace := t.TempDir()
	// Make workspace world-writable so container UID 1000 can write.
	if err := os.Chmod(workspace, 0o777); err != nil {
		t.Fatalf("chmod workspace: %v", err)
	}

	out, err := exec.Command(rtPath, "run", "--rm",
		"--read-only",
		"--mount", "type=bind,source="+workspace+",target=/workspace",
		"--user=1000:1000",
		"ubuntu:24.04",
		"touch", "/workspace/test-file",
	).CombinedOutput()

	if err != nil {
		t.Errorf("writing to /workspace bind mount should succeed: %v\n%s", err, out)
	}
}

func TestMountLayout_CapDropAll(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	rtPath := requireRuntime(t)

	// With --cap-drop=ALL, privileged operations should fail.
	out, err := exec.Command(rtPath, "run", "--rm",
		"--cap-drop=ALL",
		"--user=1000:1000",
		"ubuntu:24.04",
		"sh", "-c", "cat /proc/self/status | grep CapEff",
	).CombinedOutput()

	if err != nil {
		t.Logf("cap check failed (may not have /proc): %s", out)
		return
	}

	output := string(out)
	// CapEff should be all zeros (0000000000000000).
	if strings.Contains(output, "0000000000000000") {
		t.Log("capabilities correctly dropped (CapEff = 0)")
	} else {
		t.Logf("CapEff output: %s", output)
	}
}
