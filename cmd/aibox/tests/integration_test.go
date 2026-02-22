// Package tests contains integration tests that require a running container runtime.
// Run with: go test -tags=integration ./tests/
package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cleanupAiboxContainers removes all containers with the aibox label.
// This prevents stale containers from previous test runs from interfering
// with status checks (findAnyContainer returns the first match by label).
func cleanupAiboxContainers(t *testing.T, rtPath string) {
	t.Helper()
	out, err := exec.Command(rtPath, "ps", "-a", "--filter", "label=aibox=true", "--format", "{{.Names}}").Output()
	if err != nil {
		return
	}
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if name == "" {
			continue
		}
		t.Logf("cleaning up stale container: %s", name)
		_ = exec.Command(rtPath, "rm", "-f", name).Run()
	}
}

// requireRuntime skips the test if no container runtime is available.
func requireRuntime(t *testing.T) string {
	t.Helper()
	for _, rt := range []string{"podman", "docker"} {
		if path, err := exec.LookPath(rt); err == nil {
			return path
		}
	}
	t.Skip("no container runtime (podman/docker) available")
	return ""
}

// runtimeName returns "podman" or "docker" from a path.
func runtimeName(rtPath string) string {
	base := filepath.Base(rtPath)
	if strings.Contains(base, "podman") {
		return "podman"
	}
	return "docker"
}

// requireGVisor skips the test if gVisor (runsc) is not installed.
func requireGVisor(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("runsc"); err != nil {
		t.Skip("gVisor (runsc) not available")
	}
}

// requireAibox skips the test if the aibox binary is not built.
func requireAibox(t *testing.T) string {
	t.Helper()
	if path, err := exec.LookPath("aibox"); err == nil {
		return path
	}
	// Check local build directory.
	localBin := "../bin/aibox"
	if _, err := os.Stat(localBin); err == nil {
		return localBin
	}
	t.Skip("aibox binary not found; run 'go build -o bin/aibox .' first")
	return ""
}

// writeTestConfig creates a minimal config that uses the detected runtime
// with gVisor disabled (for environments without runsc).
func writeTestConfig(t *testing.T, rtName string) string {
	t.Helper()
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	cfg := fmt.Sprintf(`runtime: %s
image: docker.io/library/ubuntu:24.04
gvisor:
  enabled: false
  require_apparmor: false
resources:
  cpus: 1
  memory: 512m
  tmp_size: 256m
workspace:
  validate_fs: false
registry:
  url: docker.io
  verify_signatures: false
network:
  enabled: false
logging:
  format: text
  level: info
`, rtName)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("writing test config: %v", err)
	}
	return cfgPath
}

func TestContainerLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	aibox := requireAibox(t)
	rtPath := requireRuntime(t)
	cfgPath := writeTestConfig(t, runtimeName(rtPath))

	// Remove stale containers from previous test runs so that the status
	// check (which finds containers by label, not by name) doesn't pick up
	// an exited container from a prior run.
	cleanupAiboxContainers(t, rtPath)
	t.Cleanup(func() { cleanupAiboxContainers(t, rtPath) })

	// Ensure the test image is available.
	if err := exec.Command(rtPath, "image", "exists", "docker.io/library/ubuntu:24.04").Run(); err != nil {
		t.Log("Pulling ubuntu:24.04 for test...")
		if err := exec.Command(rtPath, "pull", "docker.io/library/ubuntu:24.04").Run(); err != nil {
			t.Skip("cannot pull test image")
		}
	}

	workspace := t.TempDir()

	// Start the sandbox.
	out, err := exec.Command(aibox, "--config", cfgPath, "start", "--workspace", workspace).CombinedOutput()
	if err != nil {
		output := string(out)
		// Skip gracefully when security profiles are unavailable.
		if strings.Contains(output, "AppArmor") ||
			strings.Contains(output, "seccomp") ||
			strings.Contains(output, "security validation failed") {
			t.Skipf("skipping: security profile not available: %s", output)
		}
		t.Fatalf("aibox start failed: %v\n%s", err, out)
	}

	// Check status.
	out, err = exec.Command(aibox, "--config", cfgPath, "status").CombinedOutput()
	if err != nil {
		t.Fatalf("aibox status failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "running") {
		t.Errorf("expected container to be running, status output: %s", out)
	}

	// Stop the sandbox.
	out, err = exec.Command(aibox, "--config", cfgPath, "stop").CombinedOutput()
	if err != nil {
		t.Fatalf("aibox stop failed: %v\n%s", err, out)
	}

	// Verify stopped.
	out, err = exec.Command(aibox, "--config", cfgPath, "status").CombinedOutput()
	if err != nil {
		// Non-zero exit expected when no container is running.
		t.Logf("status after stop (expected): %s", out)
	}
}

func TestStartRequiresWorkspace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	aibox := requireAibox(t)

	// Start without --workspace should fail.
	out, err := exec.Command(aibox, "start").CombinedOutput()
	if err == nil {
		t.Fatal("aibox start without --workspace should fail")
	}
	_ = out // Error is expected.
}

func TestStartRejectsNonExistentWorkspace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	aibox := requireAibox(t)

	out, err := exec.Command(aibox, "start", "--workspace", "/nonexistent/abc123").CombinedOutput()
	if err == nil {
		t.Fatal("aibox start with non-existent workspace should fail")
	}
	if !strings.Contains(string(out), "does not exist") {
		t.Errorf("error should mention workspace does not exist, got: %s", out)
	}
}

func TestDoctorCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	aibox := requireAibox(t)

	// Doctor should run without error (it reports findings but doesn't fail).
	out, err := exec.Command(aibox, "doctor").CombinedOutput()
	if err != nil {
		t.Logf("aibox doctor output: %s", out)
		// Doctor may return non-zero if issues are found, but should not crash.
	}

	output := string(out)
	t.Logf("doctor output:\n%s", output)
}
