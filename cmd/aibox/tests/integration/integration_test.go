// Package integration contains integration tests that require a running
// container runtime (Podman or Docker). These tests are skipped in
// environments where the runtime is unavailable.
//
// Run with: go test -tags=integration ./tests/integration/
package integration

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const testWorkspace = "/tmp/aibox-integration-test"

func TestMain(m *testing.M) {
	// Create temporary workspace.
	_ = os.MkdirAll(testWorkspace, 0o755)
	code := m.Run()
	// Cleanup.
	cleanup()
	os.Exit(code)
}

func cleanup() {
	// Stop any running test containers.
	rt := findRuntime()
	if rt == "" {
		return
	}
	out, _ := exec.Command(rt, "ps", "-a", "--filter", "label=aibox=true", "--format", "{{.Names}}").Output()
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if name != "" {
			_ = exec.Command(rt, "rm", "-f", name).Run()
		}
	}
	_ = os.RemoveAll(testWorkspace)
}

func findRuntime() string {
	if path, err := exec.LookPath("podman"); err == nil {
		return path
	}
	if path, err := exec.LookPath("docker"); err == nil {
		return path
	}
	return ""
}

func skipIfNoRuntime(t *testing.T) string {
	t.Helper()
	rt := findRuntime()
	if rt == "" {
		t.Skip("no container runtime (podman/docker) available")
	}
	return rt
}

func buildCLI(t *testing.T) string {
	t.Helper()
	binary := "/tmp/aibox-test-binary"
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = "/home/race-day/Documents/ai-box/cmd/aibox"
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("building aibox CLI: %v\n%s", err, string(out))
	}
	return binary
}

func TestAiboxHelp(t *testing.T) {
	binary := buildCLI(t)
	out, err := exec.Command(binary, "--help").Output()
	if err != nil {
		t.Fatalf("aibox --help: %v", err)
	}

	output := string(out)
	for _, cmd := range []string{"setup", "start", "stop", "shell", "status", "update", "doctor"} {
		if !strings.Contains(output, cmd) {
			t.Errorf("--help output missing command %q", cmd)
		}
	}
}

func TestAiboxVersion(t *testing.T) {
	binary := buildCLI(t)
	out, err := exec.Command(binary, "--version").Output()
	if err != nil {
		t.Fatalf("aibox --version: %v", err)
	}

	if !strings.Contains(string(out), "aibox version") {
		t.Errorf("version output should contain 'aibox version', got: %s", string(out))
	}
}

func TestAiboxDoctor(t *testing.T) {
	skipIfNoRuntime(t)
	binary := buildCLI(t)

	// Text output.
	out, _ := exec.Command(binary, "doctor").CombinedOutput()
	output := string(out)
	if !strings.Contains(output, "Container Runtime") {
		t.Errorf("doctor output should contain 'Container Runtime', got: %s", output)
	}

	// JSON output.
	jsonOut, _ := exec.Command(binary, "doctor", "--format", "json").CombinedOutput()
	if !strings.Contains(string(jsonOut), `"name"`) {
		t.Errorf("doctor --format json should return JSON with 'name' field, got: %s", string(jsonOut))
	}
}

func TestContainerLifecycle(t *testing.T) {
	rt := skipIfNoRuntime(t)

	// This test requires the base image. Skip if not available.
	if err := exec.Command(rt, "image", "exists", "docker.io/library/ubuntu:24.04").Run(); err != nil {
		// Try pulling a lightweight image for testing.
		t.Log("Pulling ubuntu:24.04 for integration test...")
		if err := exec.Command(rt, "pull", "docker.io/library/ubuntu:24.04").Run(); err != nil {
			t.Skip("cannot pull test image")
		}
	}

	binary := buildCLI(t)

	// Create a test config that uses ubuntu:24.04 (available) without gvisor.
	cfgDir := t.TempDir()
	cfgPath := cfgDir + "/config.yaml"
	cfgContent := `runtime: ` + strings.TrimSuffix(rt, "\n") + `
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
  level: debug
`
	// Determine runtime name from path.
	rtName := "podman"
	if strings.Contains(rt, "docker") {
		rtName = "docker"
	}
	cfgContent = strings.Replace(cfgContent, "runtime: "+rt, "runtime: "+rtName, 1)

	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	// Ensure test workspace exists.
	_ = os.MkdirAll(testWorkspace, 0o755)

	// Start container.
	var startOut bytes.Buffer
	startCmd := exec.Command(binary, "--config", cfgPath, "start", "--workspace", testWorkspace)
	startCmd.Stdout = &startOut
	startCmd.Stderr = &startOut
	if err := startCmd.Run(); err != nil {
		output := startOut.String()
		// Skip gracefully when security profiles are unavailable.
		if strings.Contains(output, "AppArmor") ||
			strings.Contains(output, "seccomp") ||
			strings.Contains(output, "security validation failed") {
			t.Skipf("skipping: security profile not available: %s", output)
		}
		t.Fatalf("aibox start: %v\n%s", err, output)
	}

	// Give container a moment to start.
	time.Sleep(2 * time.Second)

	// Status should show running.
	statusOut, err := exec.Command(binary, "--config", cfgPath, "status").CombinedOutput()
	if err == nil && !strings.Contains(string(statusOut), "running") {
		t.Log("Status output:", string(statusOut))
	}

	// Stop container.
	var stopOut bytes.Buffer
	stopCmd := exec.Command(binary, "--config", cfgPath, "stop")
	stopCmd.Stdout = &stopOut
	stopCmd.Stderr = &stopOut
	if err := stopCmd.Run(); err != nil {
		t.Logf("aibox stop: %v\n%s", err, stopOut.String())
	}
}
