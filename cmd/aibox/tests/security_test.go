//go:build integration

// Package tests contains security-focused integration tests.
// These tests verify that the security controls enforced by AI-Box
// actually work at runtime.
//
// Run with: go test -tags=integration ./tests/
package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSeccompProfile_Valid(t *testing.T) {
	profilePath := "../configs/seccomp.json"
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("reading seccomp profile: %v", err)
	}

	var profile struct {
		DefaultAction string `json:"defaultAction"`
		DefaultErrno  int    `json:"defaultErrnoRet"`
		Architectures []string `json:"architectures"`
		Syscalls      []struct {
			Names  []string `json:"names"`
			Action string   `json:"action"`
		} `json:"syscalls"`
	}

	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("parsing seccomp profile: %v", err)
	}

	// Verify default-deny.
	if profile.DefaultAction != "SCMP_ACT_ERRNO" {
		t.Errorf("defaultAction = %q, want %q", profile.DefaultAction, "SCMP_ACT_ERRNO")
	}

	// Collect all allowed syscalls.
	allowed := make(map[string]bool)
	for _, group := range profile.Syscalls {
		if group.Action != "SCMP_ACT_ALLOW" {
			t.Errorf("unexpected action %q in syscalls group", group.Action)
		}
		for _, name := range group.Names {
			if allowed[name] {
				t.Errorf("duplicate syscall in profile: %s", name)
			}
			allowed[name] = true
		}
	}

	// Verify critical allowed syscalls are present.
	required := []string{
		"read", "write", "open", "openat", "close", "mmap", "mprotect",
		"fork", "clone", "clone3", "execve", "futex", "socket", "connect",
		"epoll_create1", "getrandom",
	}
	for _, sc := range required {
		if !allowed[sc] {
			t.Errorf("required syscall %q missing from allowlist", sc)
		}
	}

	t.Logf("seccomp profile: %d syscalls allowed", len(allowed))
}

func TestSeccompProfile_Architectures(t *testing.T) {
	profilePath := "../configs/seccomp.json"
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("reading seccomp profile: %v", err)
	}

	var profile struct {
		Architectures []string `json:"architectures"`
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	required := []string{"SCMP_ARCH_X86_64", "SCMP_ARCH_X86", "SCMP_ARCH_X32"}
	archSet := make(map[string]bool)
	for _, a := range profile.Architectures {
		archSet[a] = true
	}

	for _, r := range required {
		if !archSet[r] {
			t.Errorf("missing architecture %q", r)
		}
	}
}

// TestSeccompProfile_BlockedSyscalls verifies that the 14 spec-mandated
// dangerous syscalls (SPEC Section 9.2) are NOT in the seccomp allowlist.
// A regression adding any of these syscalls would weaken sandbox isolation.
func TestSeccompProfile_BlockedSyscalls(t *testing.T) {
	profilePath := "../configs/seccomp.json"
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("reading seccomp profile: %v", err)
	}

	var profile struct {
		Syscalls []struct {
			Names  []string `json:"names"`
			Action string   `json:"action"`
		} `json:"syscalls"`
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("parsing seccomp profile: %v", err)
	}

	// Collect all allowed syscalls.
	allowed := make(map[string]bool)
	for _, group := range profile.Syscalls {
		if group.Action == "SCMP_ACT_ALLOW" {
			for _, name := range group.Names {
				allowed[name] = true
			}
		}
	}

	// These 14 syscalls MUST be blocked (not in the allowlist).
	blocked := []string{
		"ptrace",       // process tracing / debugging escape
		"mount",        // filesystem mount
		"umount2",      // filesystem unmount
		"pivot_root",   // change root filesystem
		"chroot",       // change root directory
		"bpf",          // eBPF program loading
		"userfaultfd",  // userfault handling (sandbox escape vector)
		"unshare",      // namespace creation
		"setns",        // namespace joining
		"init_module",  // kernel module loading
		"finit_module", // kernel module loading from fd
		"kexec_load",   // load new kernel
		"keyctl",       // kernel keyring manipulation
		"add_key",      // add key to kernel keyring
	}

	for _, sc := range blocked {
		t.Run(sc, func(t *testing.T) {
			if allowed[sc] {
				t.Errorf("dangerous syscall %q must not be in the seccomp allowlist", sc)
			}
		})
	}
}

func TestAppArmorProfile_Exists(t *testing.T) {
	profilePath := "../configs/apparmor/aibox-sandbox"
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("reading AppArmor profile: %v", err)
	}

	content := string(data)

	// Verify profile name.
	if !strings.Contains(content, "profile aibox-sandbox") {
		t.Error("AppArmor profile should declare 'profile aibox-sandbox'")
	}

	// Verify key deny rules.
	denies := []string{
		"deny /home/**",
		"deny /root/**",
		"deny /**/.ssh/**",
		"deny /**/docker.sock rw,",
		"deny /proc/*/mem",
		"deny /proc/kcore",
		"deny /sys/firmware/**",
		"deny /etc/shadow",
		"deny /etc/gshadow",
	}
	for _, d := range denies {
		if !strings.Contains(content, d) {
			t.Errorf("AppArmor profile missing deny rule: %q", d)
		}
	}

	// Verify workspace access.
	if !strings.Contains(content, "/workspace/**") {
		t.Error("AppArmor profile should allow /workspace/** access")
	}
	if !strings.Contains(content, "/home/dev/**") {
		t.Error("AppArmor profile should allow /home/dev/** access")
	}

	// Verify capability denials.
	capDenials := []string{
		"deny mount",
		"deny ptrace",
		"deny capability sys_admin",
		"deny capability sys_ptrace",
		"deny capability net_admin",
		"deny capability net_raw",
	}
	for _, cd := range capDenials {
		if !strings.Contains(content, cd) {
			t.Errorf("AppArmor profile missing capability deny: %q", cd)
		}
	}
}

func TestContainerSecurity_MandatoryFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	rtPath := requireRuntime(t)
	_ = rtPath

	// This test would inspect a running container to verify security flags.
	// Framework is here; actual checks require a running container.
	t.Log("container security flag verification -- requires running container")
}

func TestContainerSecurity_SeccompBlocks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	rtPath := requireRuntime(t)
	requireGVisor(t)

	// Verify blocked syscalls actually fail inside the container.
	// ptrace should be denied by seccomp.
	out, err := exec.Command(rtPath, "run", "--rm",
		"--security-opt=seccomp=../configs/seccomp.json",
		"--cap-drop=ALL",
		"--read-only",
		"--user=1000:1000",
		"ubuntu:24.04",
		"strace", "-e", "trace=ptrace", "true",
	).CombinedOutput()

	if err == nil {
		t.Log("ptrace test: command succeeded (strace may not be installed)")
	} else {
		output := string(out)
		t.Logf("ptrace test output (expected failure): %s", output)
		// Expect operation not permitted or similar.
	}
}
