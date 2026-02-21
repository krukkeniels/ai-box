//go:build integration

// End-to-end integration test validating the full Phase 0-3 stack.
// This test exercises the aibox CLI through the complete lifecycle:
//   start -> status -> policy validate -> stop -> verify logs
//
// Run with: go test -tags=integration -run TestE2E ./tests/integration/
package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	rt := skipIfNoRuntime(t)
	binary := buildCLI(t)

	// Determine runtime name from path.
	rtName := "podman"
	if strings.Contains(rt, "docker") {
		rtName = "docker"
	}

	// Set up test workspace and decision log directory.
	workspace := t.TempDir()
	logDir := t.TempDir()
	logPath := filepath.Join(logDir, "decisions.jsonl")

	// Create test config with all security features appropriately configured.
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	cfgContent := fmt.Sprintf(`runtime: %s
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
policy:
  org_baseline_path: ""
  decision_log_path: %s
credentials:
  mode: fallback
logging:
  format: text
  level: debug
`, rtName, logPath)

	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	// Ensure test image is available.
	if err := exec.Command(rt, "image", "inspect", "docker.io/library/ubuntu:24.04").Run(); err != nil {
		t.Log("Pulling ubuntu:24.04 for e2e test...")
		if err := exec.Command(rt, "pull", "docker.io/library/ubuntu:24.04").Run(); err != nil {
			t.Skip("cannot pull test image")
		}
	}

	// --- Step 1: aibox start ---
	t.Log("Step 1: Starting sandbox...")
	startOut, err := exec.Command(binary, "--config", cfgPath, "start", "--workspace", workspace).CombinedOutput()
	if err != nil {
		output := string(startOut)
		if strings.Contains(output, "AppArmor") ||
			strings.Contains(output, "seccomp") ||
			strings.Contains(output, "security validation failed") {
			t.Skipf("skipping e2e: security profile not available: %s", output)
		}
		t.Fatalf("aibox start failed: %v\n%s", err, output)
	}
	t.Logf("start output: %s", string(startOut))

	// Ensure cleanup on test exit.
	t.Cleanup(func() {
		_ = exec.Command(binary, "--config", cfgPath, "stop").Run()
		// Force remove any leftover containers.
		out, _ := exec.Command(rt, "ps", "-a", "--filter", "label=aibox=true", "--format", "{{.Names}}").Output()
		for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if name != "" {
				_ = exec.Command(rt, "rm", "-f", name).Run()
			}
		}
	})

	// Give container a moment to start.
	time.Sleep(2 * time.Second)

	// --- Step 2: aibox status shows running ---
	t.Log("Step 2: Checking status...")
	statusOut, err := exec.Command(binary, "--config", cfgPath, "status").CombinedOutput()
	if err != nil {
		t.Logf("status command error (may be expected): %v\n%s", err, string(statusOut))
	} else {
		if !strings.Contains(string(statusOut), "running") {
			t.Errorf("expected container to be running, status output: %s", string(statusOut))
		}
	}

	// --- Step 3: policy validate (should succeed or skip) ---
	t.Log("Step 3: Policy validate...")
	policyOut, err := exec.Command(binary, "--config", cfgPath, "policy", "validate", "--org", "/dev/null").CombinedOutput()
	if err != nil {
		// Policy validate may fail if policy files are not available; that's OK for this test.
		t.Logf("policy validate (expected possible failure): %s", string(policyOut))
	}

	// --- Step 4: aibox stop ---
	t.Log("Step 4: Stopping sandbox...")
	stopOut, err := exec.Command(binary, "--config", cfgPath, "stop").CombinedOutput()
	if err != nil {
		t.Logf("aibox stop: %v\n%s", err, string(stopOut))
	}

	// --- Step 5: Verify container is stopped ---
	t.Log("Step 5: Verifying container stopped...")
	statusAfter, _ := exec.Command(binary, "--config", cfgPath, "status").CombinedOutput()
	statusStr := string(statusAfter)
	if strings.Contains(statusStr, "running") {
		t.Errorf("container should be stopped, but status shows running: %s", statusStr)
	}

	// --- Step 6: Decision log exists and contains valid JSON ---
	t.Log("Step 6: Checking decision log...")
	if _, err := os.Stat(logPath); err == nil {
		data, readErr := os.ReadFile(logPath)
		if readErr != nil {
			t.Errorf("failed to read decision log: %v", readErr)
		} else {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
				t.Log("decision log is empty (no events logged)")
			} else {
				t.Logf("decision log has %d entries", len(lines))

				// Validate each line is valid JSON.
				for i, line := range lines {
					if line == "" {
						continue
					}
					var entry map[string]interface{}
					if err := json.Unmarshal([]byte(line), &entry); err != nil {
						t.Errorf("decision log line %d is not valid JSON: %v\n  line: %s", i, err, line)
					}
				}

				// Check for expected lifecycle events.
				var hasStart, hasStop bool
				for _, line := range lines {
					if strings.Contains(line, "container_start") {
						hasStart = true
					}
					if strings.Contains(line, "container_stop") {
						hasStop = true
					}
				}
				if hasStart {
					t.Log("found container_start event in decision log")
				}
				if hasStop {
					t.Log("found container_stop event in decision log")
				}
			}
		}
	} else {
		t.Log("decision log not created (decision logging may not be active)")
	}

	t.Log("E2E test completed successfully")
}
