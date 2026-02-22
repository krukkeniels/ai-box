package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateSSHKeyPair(t *testing.T) {
	// Redirect key generation to a temp directory.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create the config dir structure.
	sshDir := filepath.Join(tmpDir, ".config", "aibox", "ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("creating ssh dir: %v", err)
	}

	// Override SSHKeyDir to use temp.
	origFunc := sshKeyDirFunc
	sshKeyDirFunc = func() (string, error) { return sshDir, nil }
	defer func() { sshKeyDirFunc = origFunc }()

	// Generate key pair.
	if err := GenerateSSHKeyPair(); err != nil {
		t.Fatalf("GenerateSSHKeyPair(): %v", err)
	}

	// Verify files exist.
	privPath := filepath.Join(sshDir, "aibox_ed25519")
	pubPath := filepath.Join(sshDir, "aibox_ed25519.pub")

	if _, err := os.Stat(privPath); err != nil {
		t.Errorf("private key not created: %v", err)
	}
	if _, err := os.Stat(pubPath); err != nil {
		t.Errorf("public key not created: %v", err)
	}

	// Verify private key permissions.
	info, _ := os.Stat(privPath)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("private key permissions = %o, want 600", info.Mode().Perm())
	}

	// Verify public key format.
	pubData, _ := os.ReadFile(pubPath)
	if !strings.HasPrefix(string(pubData), "ssh-ed25519 ") {
		t.Errorf("public key should start with 'ssh-ed25519 ', got: %s", string(pubData[:30]))
	}

	// Second call should not overwrite.
	if err := GenerateSSHKeyPair(); err != nil {
		t.Fatalf("second GenerateSSHKeyPair(): %v", err)
	}
}

func TestRemoveSSHBlock(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no block",
			input: "Host myserver\n  HostName 10.0.0.1\n",
			want:  "Host myserver\n  HostName 10.0.0.1\n",
		},
		{
			name: "with block",
			input: "Host myserver\n  HostName 10.0.0.1\n" +
				"# --- AI-Box (managed by aibox, do not edit) ---\n" +
				"Host aibox\n  Port 2222\n" +
				"# --- End AI-Box ---\n",
			want: "Host myserver\n  HostName 10.0.0.1\n",
		},
		{
			name: "block in middle",
			input: "Host before\n  HostName 1.1.1.1\n" +
				"# --- AI-Box (managed by aibox, do not edit) ---\n" +
				"Host aibox\n" +
				"# --- End AI-Box ---\n" +
				"Host after\n  HostName 2.2.2.2\n",
			want: "Host before\n  HostName 1.1.1.1\nHost after\n  HostName 2.2.2.2\n",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeSSHBlock(tt.input)
			if got != tt.want {
				t.Errorf("removeSSHBlock() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestWriteSSHConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create SSH key dir and key files so the config references them.
	sshKeyDir := filepath.Join(tmpDir, ".config", "aibox", "ssh")
	if err := os.MkdirAll(sshKeyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	privPath := filepath.Join(sshKeyDir, "aibox_ed25519")
	os.WriteFile(privPath, []byte("fake-key"), 0o600)

	origFunc := sshKeyDirFunc
	sshKeyDirFunc = func() (string, error) { return sshKeyDir, nil }
	defer func() { sshKeyDirFunc = origFunc }()

	// Write config.
	if err := WriteSSHConfig(2222); err != nil {
		t.Fatalf("WriteSSHConfig(): %v", err)
	}

	// Verify file.
	configPath := filepath.Join(tmpDir, ".ssh", "config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading SSH config: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Host aibox") {
		t.Error("SSH config missing 'Host aibox' entry")
	}
	if !strings.Contains(content, "Port 2222") {
		t.Error("SSH config missing 'Port 2222'")
	}
	if !strings.Contains(content, "User dev") {
		t.Error("SSH config missing 'User dev'")
	}
	if !strings.Contains(content, "StrictHostKeyChecking no") {
		t.Error("SSH config missing 'StrictHostKeyChecking no'")
	}

	// Write again to verify idempotency (block should be replaced, not duplicated).
	if err := WriteSSHConfig(3333); err != nil {
		t.Fatalf("second WriteSSHConfig(): %v", err)
	}

	data, _ = os.ReadFile(configPath)
	content = string(data)
	if strings.Count(content, "Host aibox") != 1 {
		t.Error("SSH config has duplicate 'Host aibox' entries")
	}
	if !strings.Contains(content, "Port 3333") {
		t.Error("SSH config should have updated port to 3333")
	}
}
