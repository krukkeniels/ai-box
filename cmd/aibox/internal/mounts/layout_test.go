package mounts

import (
	"strings"
	"testing"
)

func TestLayout_CoreMounts(t *testing.T) {
	mounts, err := Layout("/home/user/project", "", "")
	if err != nil {
		t.Fatalf("Layout() returned error: %v", err)
	}

	// Expected targets from spec 10.1.
	expectedTargets := map[string]string{
		"/workspace":    "bind",
		"/home/dev":     "volume",
		"/opt/toolpacks": "volume",
		"/tmp":          "tmpfs",
		"/var/tmp":      "tmpfs",
	}

	found := make(map[string]bool)
	for _, m := range mounts {
		if wantType, ok := expectedTargets[m.Target]; ok {
			if m.Type != wantType {
				t.Errorf("mount %s has type %q, want %q", m.Target, m.Type, wantType)
			}
			found[m.Target] = true
		}
	}

	for target := range expectedTargets {
		if !found[target] {
			t.Errorf("missing expected mount target %s", target)
		}
	}
}

func TestLayout_WorkspaceBind(t *testing.T) {
	mounts, err := Layout("/home/user/myrepo", "", "")
	if err != nil {
		t.Fatalf("Layout() returned error: %v", err)
	}

	var workspace *Mount
	for i := range mounts {
		if mounts[i].Target == "/workspace" {
			workspace = &mounts[i]
			break
		}
	}

	if workspace == nil {
		t.Fatal("no /workspace mount found")
	}

	if workspace.Source != "/home/user/myrepo" {
		t.Errorf("workspace source = %q, want %q", workspace.Source, "/home/user/myrepo")
	}
	if workspace.Type != "bind" {
		t.Errorf("workspace type = %q, want %q", workspace.Type, "bind")
	}
	if !strings.Contains(workspace.Options, "nosuid") {
		t.Errorf("workspace options missing nosuid: %q", workspace.Options)
	}
	if !strings.Contains(workspace.Options, "nodev") {
		t.Errorf("workspace options missing nodev: %q", workspace.Options)
	}
}

func TestLayout_TmpfsDefaults(t *testing.T) {
	mounts, err := Layout("/home/user/project", "", "")
	if err != nil {
		t.Fatalf("Layout() returned error: %v", err)
	}

	for _, m := range mounts {
		if m.Target == "/tmp" {
			if !strings.Contains(m.Options, "size=2g") {
				t.Errorf("/tmp options should contain size=2g, got: %q", m.Options)
			}
			if !strings.Contains(m.Options, "noexec") {
				t.Errorf("/tmp options should contain noexec, got: %q", m.Options)
			}
		}
		if m.Target == "/var/tmp" {
			if !strings.Contains(m.Options, "size=1g") {
				t.Errorf("/var/tmp options should contain size=1g, got: %q", m.Options)
			}
			if !strings.Contains(m.Options, "noexec") {
				t.Errorf("/var/tmp options should contain noexec, got: %q", m.Options)
			}
		}
	}
}

func TestLayout_CustomTmpSizes(t *testing.T) {
	mounts, err := Layout("/home/user/project", "4g", "2g")
	if err != nil {
		t.Fatalf("Layout() returned error: %v", err)
	}

	for _, m := range mounts {
		if m.Target == "/tmp" && !strings.Contains(m.Options, "size=4g") {
			t.Errorf("/tmp should use custom size=4g, got: %q", m.Options)
		}
		if m.Target == "/var/tmp" && !strings.Contains(m.Options, "size=2g") {
			t.Errorf("/var/tmp should use custom size=2g, got: %q", m.Options)
		}
	}
}

func TestLayout_IncludesCacheVolumes(t *testing.T) {
	mounts, err := Layout("/home/user/project", "", "")
	if err != nil {
		t.Fatalf("Layout() returned error: %v", err)
	}

	cacheTargets := []string{
		"/home/dev/.m2/repository",
		"/home/dev/.gradle/caches",
		"/home/dev/.npm",
		"/home/dev/.yarn/cache",
		"/home/dev/.cache/bazel",
	}

	targets := make(map[string]bool)
	for _, m := range mounts {
		targets[m.Target] = true
	}

	for _, ct := range cacheTargets {
		if !targets[ct] {
			t.Errorf("Layout() missing cache volume mount %s", ct)
		}
	}
}

func TestLayout_VolumeNaming(t *testing.T) {
	mounts, err := Layout("/home/user/project", "", "")
	if err != nil {
		t.Fatalf("Layout() returned error: %v", err)
	}

	for _, m := range mounts {
		if m.Type == "volume" && !strings.HasPrefix(m.Source, "aibox-") {
			t.Errorf("volume %s source %q should start with 'aibox-'", m.Target, m.Source)
		}
	}
}

func TestRuntimeArgs_BindMount(t *testing.T) {
	mounts := []Mount{
		{Type: "bind", Source: "/host/path", Target: "/container/path", Options: "rw,nosuid"},
	}
	args := RuntimeArgs(mounts)

	if len(args) != 2 {
		t.Fatalf("RuntimeArgs() returned %d args, want 2 (--mount and value)", len(args))
	}
	if args[0] != "--mount" {
		t.Errorf("first arg = %q, want %q", args[0], "--mount")
	}
	if !strings.Contains(args[1], "type=bind") {
		t.Errorf("bind mount arg missing type=bind: %q", args[1])
	}
	if !strings.Contains(args[1], "source=/host/path") {
		t.Errorf("bind mount arg missing source: %q", args[1])
	}
	if !strings.Contains(args[1], "target=/container/path") {
		t.Errorf("bind mount arg missing target: %q", args[1])
	}
}

func TestRuntimeArgs_Tmpfs(t *testing.T) {
	mounts := []Mount{
		{Type: "tmpfs", Target: "/tmp", Options: "rw,noexec,nosuid,size=2g"},
	}
	args := RuntimeArgs(mounts)

	if len(args) != 2 {
		t.Fatalf("RuntimeArgs() returned %d args, want 2", len(args))
	}
	if !strings.Contains(args[1], "type=tmpfs") {
		t.Errorf("tmpfs mount arg missing type=tmpfs: %q", args[1])
	}
	if !strings.Contains(args[1], "target=/tmp") {
		t.Errorf("tmpfs mount arg missing target: %q", args[1])
	}
	if strings.Contains(args[1], ",size=") {
		t.Errorf("tmpfs mount arg should use tmpfs-size, not size: %q", args[1])
	}
	if !strings.Contains(args[1], "tmpfs-size=2g") {
		t.Errorf("tmpfs mount arg missing tmpfs-size=2g: %q", args[1])
	}
}

func TestVolumePrefix(t *testing.T) {
	prefix, err := VolumePrefix()
	if err != nil {
		t.Fatalf("VolumePrefix() returned error: %v", err)
	}
	if !strings.HasPrefix(prefix, "aibox-") {
		t.Errorf("VolumePrefix() = %q, should start with 'aibox-'", prefix)
	}
	if prefix == "aibox-" {
		t.Error("VolumePrefix() should include username after 'aibox-'")
	}
}
