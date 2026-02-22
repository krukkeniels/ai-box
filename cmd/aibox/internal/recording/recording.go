// Package recording implements optional terminal session recording for AI-Box
// sandboxes in classified environments.
//
// Session recording captures all terminal I/O via the `script` command wrapper,
// encrypts recordings at rest using AES-256-GCM, and provides a playback tool
// for incident investigation. See SPEC-FINAL.md Section 19.3.
package recording

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aibox/aibox/internal/audit"
)

// Config holds session recording configuration.
type Config struct {
	Enabled       bool   // whether session recording is enabled (policy-driven)
	RecordingsDir string // directory for raw recordings (default "/var/log/aibox/recordings")
	EncryptedDir  string // directory for encrypted recordings (default "/var/log/aibox/recordings/encrypted")
	ScriptPath    string // path to the `script` binary (default: auto-detect)
	NoticeText    string // notice displayed at sandbox start
	MaxSizeMB     int    // max recording file size in MB (default 500)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:       false, // opt-in, not opt-out
		RecordingsDir: "/var/log/aibox/recordings",
		EncryptedDir:  "/var/log/aibox/recordings/encrypted",
		NoticeText:    defaultNotice,
		MaxSizeMB:     500,
	}
}

const defaultNotice = `
================================================================================
  SESSION RECORDING NOTICE

  This terminal session is being recorded for security and compliance purposes.
  All terminal input and output is captured and stored in encrypted, tamper-
  evident storage. Recordings are accessible only to authorized security and
  compliance personnel for incident investigation.

  By continuing to use this session, you acknowledge this recording.
================================================================================
`

// SessionInfo holds metadata about a recording session.
type SessionInfo struct {
	ID        string    // unique session identifier
	SandboxID string    // associated sandbox ID
	UserID    string    // developer user ID
	StartTime time.Time // when recording started
	EndTime   time.Time // when recording ended (zero if still active)
	RawPath   string    // path to raw typescript file
	TimePath  string    // path to timing file
	EncPath   string    // path to encrypted recording
	SizeBytes int64     // size of raw recording in bytes
}

// Manager manages session recording lifecycle.
type Manager struct {
	cfg Config
}

// NewManager creates a Manager, applying defaults for any zero-value fields.
func NewManager(cfg Config) *Manager {
	if cfg.RecordingsDir == "" {
		cfg.RecordingsDir = "/var/log/aibox/recordings"
	}
	if cfg.EncryptedDir == "" {
		cfg.EncryptedDir = "/var/log/aibox/recordings/encrypted"
	}
	if cfg.NoticeText == "" {
		cfg.NoticeText = defaultNotice
	}
	if cfg.MaxSizeMB == 0 {
		cfg.MaxSizeMB = 500
	}
	return &Manager{cfg: cfg}
}

// Config returns the current configuration (read-only copy).
func (m *Manager) Config() Config {
	return m.cfg
}

// IsEnabled returns whether session recording is enabled.
func (m *Manager) IsEnabled() bool {
	return m.cfg.Enabled
}

// NoticeText returns the recording notice text displayed at sandbox start.
func (m *Manager) NoticeText() string {
	return m.cfg.NoticeText
}

// ScriptAvailable checks whether the `script` command is available.
func (m *Manager) ScriptAvailable() bool {
	path := m.findScript()
	return path != ""
}

// findScript locates the `script` binary.
func (m *Manager) findScript() string {
	if m.cfg.ScriptPath != "" {
		if _, err := os.Stat(m.cfg.ScriptPath); err == nil {
			return m.cfg.ScriptPath
		}
	}
	if path, err := exec.LookPath("script"); err == nil {
		return path
	}
	return ""
}

// SessionID generates a unique session identifier from sandbox and user info.
func SessionID(sandboxID, userID string) string {
	ts := time.Now().UTC().Format("20060102-150405")
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s:%d", sandboxID, userID, ts, time.Now().UnixNano())))
	return fmt.Sprintf("%s-%s-%s", sandboxID, ts, hex.EncodeToString(h[:4]))
}

// EntrypointWrapper returns the shell command that wraps the user's shell
// with `script` for recording. This is injected into the container entrypoint.
func (m *Manager) EntrypointWrapper(sessionID, shell string) string {
	if shell == "" {
		shell = "bash"
	}

	rawPath := filepath.Join(m.cfg.RecordingsDir, sessionID+".typescript")
	timePath := filepath.Join(m.cfg.RecordingsDir, sessionID+".timing")

	// Use script with timing for faithful playback.
	return fmt.Sprintf(
		"script --flush --timing=%s %s -c %s",
		timePath, rawPath, shell,
	)
}

// EnsureDirectories creates the recording and encrypted directories.
func (m *Manager) EnsureDirectories() error {
	for _, dir := range []string{m.cfg.RecordingsDir, m.cfg.EncryptedDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("creating recording directory %s: %w", dir, err)
		}
	}
	return nil
}

// Encrypt encrypts a raw recording file using AES-256-GCM and writes the
// encrypted output to the encrypted directory. The key should be 32 bytes
// (256 bits), typically derived from Vault.
func (m *Manager) Encrypt(rawPath string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes (AES-256), got %d", len(key))
	}

	plaintext, err := os.ReadFile(rawPath)
	if err != nil {
		return "", fmt.Errorf("reading raw recording %s: %w", rawPath, err)
	}

	ciphertext, err := encryptAESGCM(plaintext, key)
	if err != nil {
		return "", fmt.Errorf("encrypting recording: %w", err)
	}

	baseName := filepath.Base(rawPath) + ".enc"
	encPath := filepath.Join(m.cfg.EncryptedDir, baseName)

	if err := os.MkdirAll(m.cfg.EncryptedDir, 0o700); err != nil {
		return "", fmt.Errorf("creating encrypted directory: %w", err)
	}

	if err := os.WriteFile(encPath, ciphertext, 0o600); err != nil {
		return "", fmt.Errorf("writing encrypted recording to %s: %w", encPath, err)
	}

	slog.Info("encrypted recording", "raw", rawPath, "encrypted", encPath,
		"raw_size", len(plaintext), "enc_size", len(ciphertext))
	return encPath, nil
}

// Decrypt decrypts an encrypted recording file and returns the plaintext.
func (m *Manager) Decrypt(encPath string, key []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("decryption key must be 32 bytes (AES-256), got %d", len(key))
	}

	ciphertext, err := os.ReadFile(encPath)
	if err != nil {
		return nil, fmt.Errorf("reading encrypted recording %s: %w", encPath, err)
	}

	plaintext, err := decryptAESGCM(ciphertext, key)
	if err != nil {
		return nil, fmt.Errorf("decrypting recording: %w", err)
	}

	return plaintext, nil
}

// encryptAESGCM encrypts plaintext using AES-256-GCM with a random nonce.
// Output format: nonce (12 bytes) || ciphertext+tag
func encryptAESGCM(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	// nonce is prepended to the ciphertext for storage.
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// decryptAESGCM decrypts ciphertext encrypted with encryptAESGCM.
func decryptAESGCM(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short: %d bytes (need at least %d for nonce)", len(ciphertext), nonceSize)
	}

	nonce, ciphertextBody := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBody, nil)
	if err != nil {
		return nil, fmt.Errorf("GCM decryption failed (wrong key or tampered data): %w", err)
	}

	return plaintext, nil
}

// Playback writes a decrypted recording to the given writer for replay.
// If timingPath is provided, playback respects the original timing.
func (m *Manager) Playback(encPath, timingPath string, key []byte, w io.Writer) error {
	plaintext, err := m.Decrypt(encPath, key)
	if err != nil {
		return err
	}

	if timingPath != "" {
		// Use scriptreplay for faithful timing playback.
		return m.scriptReplay(plaintext, timingPath, w)
	}

	// Without timing, just dump the content.
	_, err = w.Write(plaintext)
	return err
}

// scriptReplay replays a typescript file with timing using scriptreplay.
func (m *Manager) scriptReplay(typescript []byte, timingPath string, w io.Writer) error {
	// Write typescript to a temp file for scriptreplay.
	tmpFile, err := os.CreateTemp("", "aibox-playback-*.typescript")
	if err != nil {
		return fmt.Errorf("creating temp file for playback: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(typescript); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing temp typescript: %w", err)
	}
	tmpFile.Close()

	replayPath, err := exec.LookPath("scriptreplay")
	if err != nil {
		// Fall back to direct output without timing.
		slog.Warn("scriptreplay not found, playing back without timing")
		_, err = w.Write(typescript)
		return err
	}

	cmd := exec.Command(replayPath, "--timing", timingPath, "--typescript", tmpFile.Name())
	cmd.Stdout = w
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// ListSessions returns session IDs for all recordings in the encrypted directory.
func (m *Manager) ListSessions() ([]string, error) {
	entries, err := os.ReadDir(m.cfg.EncryptedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading encrypted directory: %w", err)
	}

	var sessions []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".typescript.enc") {
			sessionID := strings.TrimSuffix(name, ".typescript.enc")
			sessions = append(sessions, sessionID)
		}
	}
	return sessions, nil
}

// HealthCheck verifies the recording system is operational.
func (m *Manager) HealthCheck() error {
	if !m.cfg.Enabled {
		return nil // recording is optional, disabled is not an error
	}

	if !m.ScriptAvailable() {
		return fmt.Errorf("script command not found in PATH (required for session recording)")
	}

	// Check directories exist and are writable.
	for _, dir := range []string{m.cfg.RecordingsDir, m.cfg.EncryptedDir} {
		if _, err := os.Stat(dir); err != nil {
			return fmt.Errorf("recording directory not found: %s: %w", dir, err)
		}
		testFile := filepath.Join(dir, ".write-test")
		if err := os.WriteFile(testFile, []byte("test"), 0o600); err != nil {
			return fmt.Errorf("recording directory not writable: %s: %w", dir, err)
		}
		os.Remove(testFile)
	}

	return nil
}

// SessionStartEvent creates an audit event for session recording start.
func SessionStartEvent(sessionID, sandboxID, userID string) audit.AuditEvent {
	return audit.AuditEvent{
		Timestamp: time.Now().UTC(),
		EventType: audit.EventSessionStart,
		SandboxID: sandboxID,
		UserID:    userID,
		Source:    audit.SourceRecorder,
		Severity:  audit.SeverityInfo,
		Details: map[string]any{
			"session_id": sessionID,
			"action":     "start",
		},
	}
}

// SessionEndEvent creates an audit event for session recording end.
func SessionEndEvent(sessionID, sandboxID, userID string, sizeBytes int64) audit.AuditEvent {
	return audit.AuditEvent{
		Timestamp: time.Now().UTC(),
		EventType: audit.EventSessionEnd,
		SandboxID: sandboxID,
		UserID:    userID,
		Source:    audit.SourceRecorder,
		Severity:  audit.SeverityInfo,
		Details: map[string]any{
			"session_id": sessionID,
			"action":     "end",
			"size_bytes": sizeBytes,
		},
	}
}
