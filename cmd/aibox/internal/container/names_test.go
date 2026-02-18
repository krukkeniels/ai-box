package container

import (
	"strings"
	"testing"
)

func TestContainerName_Format(t *testing.T) {
	name := ContainerName("/home/user/project")

	if !strings.HasPrefix(name, "aibox-") {
		t.Errorf("ContainerName() = %q, should start with 'aibox-'", name)
	}

	// Should contain a hash suffix.
	parts := strings.SplitN(name, "-", 3)
	if len(parts) < 3 {
		t.Errorf("ContainerName() = %q, expected format 'aibox-<user>-<hash>'", name)
	}
}

func TestContainerName_Deterministic(t *testing.T) {
	name1 := ContainerName("/home/user/project")
	name2 := ContainerName("/home/user/project")

	if name1 != name2 {
		t.Errorf("ContainerName() not deterministic: %q != %q", name1, name2)
	}
}

func TestContainerName_DifferentPaths(t *testing.T) {
	name1 := ContainerName("/home/user/project-a")
	name2 := ContainerName("/home/user/project-b")

	if name1 == name2 {
		t.Errorf("different workspace paths should produce different names: both = %q", name1)
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"alice", "alice"},
		{"Alice", "alice"},
		{"alice.bob", "alicebob"},
		{"alice_bob", "alicebob"},
		{"alice@company", "alicecompany"},
		{"user123", "user123"},
		{"DOMAIN\\user", "domainuser"},
		{"", "user"}, // empty returns "user" fallback
		{"!!!@@@", "user"}, // all invalid returns "user" fallback
	}

	for _, tt := range tests {
		got := sanitize(tt.input)
		if got != tt.want {
			t.Errorf("sanitize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestContainerLabel(t *testing.T) {
	if ContainerLabel == "" {
		t.Error("ContainerLabel should not be empty")
	}
	if !strings.Contains(ContainerLabel, "aibox") {
		t.Errorf("ContainerLabel = %q, should contain 'aibox'", ContainerLabel)
	}
}
