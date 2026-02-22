package setup

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"

	"github.com/aibox/aibox/internal/config"
)

// sshKeyDirFunc is the function used to determine the SSH key directory.
// It can be overridden in tests.
var sshKeyDirFunc = defaultSSHKeyDir

func defaultSSHKeyDir() (string, error) {
	cfgDir, err := config.DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, "ssh"), nil
}

// SSHKeyDir returns the directory where aibox stores SSH keys.
func SSHKeyDir() (string, error) {
	return sshKeyDirFunc()
}

// SSHKeyPaths returns the private and public key paths.
func SSHKeyPaths() (privPath, pubPath string, err error) {
	dir, err := SSHKeyDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(dir, "aibox_ed25519"), filepath.Join(dir, "aibox_ed25519.pub"), nil
}

// GenerateSSHKeyPair generates an Ed25519 SSH key pair for aibox container
// access. Keys are stored in ~/.config/aibox/ssh/. Existing keys are not
// overwritten.
func GenerateSSHKeyPair() error {
	privPath, pubPath, err := SSHKeyPaths()
	if err != nil {
		return fmt.Errorf("determining SSH key paths: %w", err)
	}

	// Don't overwrite existing keys.
	if _, err := os.Stat(privPath); err == nil {
		slog.Debug("SSH key pair already exists", "path", privPath)
		fmt.Printf("  SSH key pair already exists at %s\n", privPath)
		return nil
	}

	dir := filepath.Dir(privPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating SSH key directory %s: %w", dir, err)
	}

	// Generate Ed25519 key pair.
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generating Ed25519 key: %w", err)
	}

	// Marshal private key to PEM.
	privKeyBytes, err := ssh.MarshalPrivateKey(privKey, "aibox sandbox key")
	if err != nil {
		return fmt.Errorf("marshaling private key: %w", err)
	}
	if err := os.WriteFile(privPath, pem.EncodeToMemory(privKeyBytes), 0o600); err != nil {
		return fmt.Errorf("writing private key: %w", err)
	}

	// Marshal public key to authorized_keys format.
	sshPub, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return fmt.Errorf("converting public key: %w", err)
	}
	pubKeyLine := ssh.MarshalAuthorizedKey(sshPub)
	if err := os.WriteFile(pubPath, pubKeyLine, 0o644); err != nil {
		return fmt.Errorf("writing public key: %w", err)
	}

	fmt.Printf("  SSH key pair generated at %s\n", dir)
	return nil
}

// ReadPublicKey reads the aibox SSH public key.
func ReadPublicKey() ([]byte, error) {
	_, pubPath, err := SSHKeyPaths()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(pubPath)
	if err != nil {
		return nil, fmt.Errorf("reading SSH public key %s: %w", pubPath, err)
	}
	return data, nil
}

// WriteSSHConfig writes or updates the aibox entry in ~/.ssh/config.
func WriteSSHConfig(sshPort int) error {
	home, err := config.ResolveHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home dir: %w", err)
	}

	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return fmt.Errorf("creating ~/.ssh: %w", err)
	}

	privPath, _, err := SSHKeyPaths()
	if err != nil {
		return err
	}

	entry := fmt.Sprintf(`# --- AI-Box (managed by aibox, do not edit) ---
Host aibox
  HostName localhost
  Port %d
  User dev
  IdentityFile %s
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
# --- End AI-Box ---
`, sshPort, privPath)

	configPath := filepath.Join(sshDir, "config")

	// Read existing config if it exists.
	existing, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading SSH config: %w", err)
	}

	content := string(existing)

	// Remove any existing aibox block.
	content = removeSSHBlock(content)

	// Append the aibox block.
	if content != "" && content[len(content)-1] != '\n' {
		content += "\n"
	}
	content += entry

	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("writing SSH config: %w", err)
	}

	slog.Debug("updated SSH config", "path", configPath, "port", sshPort)
	return nil
}

// removeSSHBlock removes the AI-Box managed block from SSH config content.
func removeSSHBlock(content string) string {
	const startMarker = "# --- AI-Box (managed by aibox, do not edit) ---"
	const endMarker = "# --- End AI-Box ---"

	for {
		startIdx := indexOf(content, startMarker)
		if startIdx < 0 {
			break
		}
		endIdx := indexOf(content[startIdx:], endMarker)
		if endIdx < 0 {
			break
		}
		end := startIdx + endIdx + len(endMarker)
		// Also remove trailing newline.
		if end < len(content) && content[end] == '\n' {
			end++
		}
		content = content[:startIdx] + content[end:]
	}
	return content
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
