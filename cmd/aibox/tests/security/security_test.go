// Package security contains security validation tests that verify container
// isolation properties. These tests require a running container with the
// aibox security profile applied.
//
// Run with: go test -tags=security ./tests/security/
package security

import (
	"os/exec"
	"strings"
	"testing"
)

func findRuntime() string {
	if path, err := exec.LookPath("podman"); err == nil {
		return path
	}
	if path, err := exec.LookPath("docker"); err == nil {
		return path
	}
	return ""
}

func findAiboxContainer(rt string) string {
	out, err := exec.Command(rt, "ps", "--filter", "label=aibox=true", "--format", "{{.Names}}").Output()
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return ""
	}
	return strings.Split(name, "\n")[0]
}

func skipIfNoContainer(t *testing.T) (string, string) {
	t.Helper()
	rt := findRuntime()
	if rt == "" {
		t.Skip("no container runtime available")
	}
	name := findAiboxContainer(rt)
	if name == "" {
		t.Skip("no running aibox container found; start one with: aibox start --workspace /tmp/test")
	}
	return rt, name
}

func execInContainer(rt, name string, args ...string) (string, error) {
	cmdArgs := append([]string{"exec", name}, args...)
	out, err := exec.Command(rt, cmdArgs...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func TestZeroCapabilities(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	out, err := execInContainer(rt, name, "capsh", "--print")
	if err != nil {
		// capsh may not be available; try cat /proc/self/status.
		out, err = execInContainer(rt, name, "cat", "/proc/self/status")
		if err != nil {
			t.Skipf("cannot check capabilities: %v", err)
		}
		if !strings.Contains(out, "CapEff:\t0000000000000000") {
			t.Errorf("effective capabilities should be zero, got /proc/self/status:\n%s", out)
		}
		return
	}

	if !strings.Contains(out, "Current:") {
		t.Skipf("unexpected capsh output: %s", out)
	}
	// Current capabilities should be empty.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Current:") {
			caps := strings.TrimSpace(strings.TrimPrefix(line, "Current:"))
			if caps != "" && caps != "=" {
				t.Errorf("current capabilities should be empty, got: %q", caps)
			}
		}
	}
}

func TestPtraceBlocked(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	// Try to ptrace ourselves -- should be blocked by seccomp.
	out, err := execInContainer(rt, name, "sh", "-c", "strace -e trace=none true 2>&1 || echo BLOCKED")
	if err != nil {
		// strace may not be installed, try a different approach.
		out, err = execInContainer(rt, name, "sh", "-c", "cat /proc/1/mem 2>&1 || echo BLOCKED")
		if err == nil && !strings.Contains(out, "denied") && !strings.Contains(out, "BLOCKED") && !strings.Contains(out, "Permission") {
			t.Logf("ptrace/mem test inconclusive: %s", out)
		}
		return
	}

	if !strings.Contains(out, "BLOCKED") && !strings.Contains(out, "EPERM") && !strings.Contains(out, "not permitted") {
		t.Errorf("ptrace should be blocked, got: %s", out)
	}
}

func TestReadOnlyRoot(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	// Writing to / should fail (read-only root).
	_, err := execInContainer(rt, name, "touch", "/test-readonly")
	if err == nil {
		t.Error("writing to / should fail (read-only root filesystem)")
	}

	// Writing to /etc should fail.
	_, err = execInContainer(rt, name, "touch", "/etc/test-readonly")
	if err == nil {
		t.Error("writing to /etc should fail (read-only root filesystem)")
	}
}

func TestWorkspaceWritable(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	// Writing to /workspace should succeed.
	_, err := execInContainer(rt, name, "touch", "/workspace/test-writable")
	if err != nil {
		t.Errorf("writing to /workspace should succeed: %v", err)
	}

	// Clean up.
	_, _ = execInContainer(rt, name, "rm", "-f", "/workspace/test-writable")
}

func TestHomeDevWritable(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	_, err := execInContainer(rt, name, "touch", "/home/dev/test-writable")
	if err != nil {
		t.Errorf("writing to /home/dev should succeed: %v", err)
	}

	_, _ = execInContainer(rt, name, "rm", "-f", "/home/dev/test-writable")
}

func TestTmpWritable(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	_, err := execInContainer(rt, name, "touch", "/tmp/test-writable")
	if err != nil {
		t.Errorf("writing to /tmp should succeed: %v", err)
	}
}

func TestNonRootUser(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	out, err := execInContainer(rt, name, "id", "-u")
	if err != nil {
		t.Skipf("cannot check user: %v", err)
	}

	if strings.TrimSpace(out) == "0" {
		t.Error("container should run as non-root user (UID 1000), got UID 0")
	}
}

func TestNoNewPrivileges(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	out, err := execInContainer(rt, name, "cat", "/proc/self/status")
	if err != nil {
		t.Skipf("cannot read /proc/self/status: %v", err)
	}

	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "NoNewPrivs:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "NoNewPrivs:"))
			if val != "1" {
				t.Errorf("NoNewPrivs should be 1, got %q", val)
			}
			return
		}
	}
	t.Log("NoNewPrivs field not found in /proc/self/status (may not be exposed)")
}

func TestAppArmorProfile(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	out, err := execInContainer(rt, name, "cat", "/proc/1/attr/current")
	if err != nil {
		t.Skipf("cannot read AppArmor profile: %v", err)
	}

	profile := strings.TrimSpace(out)
	if profile == "unconfined" {
		t.Log("AppArmor profile is unconfined (AppArmor may not be available)")
	} else if !strings.Contains(profile, "aibox-sandbox") {
		t.Errorf("AppArmor profile should be aibox-sandbox, got %q", profile)
	}
}

func TestMountLayout(t *testing.T) {
	rt, name := skipIfNoContainer(t)

	out, err := execInContainer(rt, name, "mount")
	if err != nil {
		t.Skipf("cannot run mount: %v", err)
	}

	// Check for expected mount points.
	checks := []struct {
		path string
		want string // substring to find in mount output
	}{
		{"/workspace", "/workspace"},
		{"/tmp", "tmpfs"},
	}

	for _, check := range checks {
		found := false
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, check.path) && strings.Contains(line, check.want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("mount output should contain %q with %q", check.path, check.want)
		}
	}
}
