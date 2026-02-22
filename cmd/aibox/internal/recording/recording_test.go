package recording

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aibox/aibox/internal/audit"
	"github.com/aibox/aibox/internal/storage"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Error("Enabled should be false by default (opt-in)")
	}
	if cfg.RecordingsDir != "/var/log/aibox/recordings" {
		t.Errorf("RecordingsDir = %q, want /var/log/aibox/recordings", cfg.RecordingsDir)
	}
	if cfg.EncryptedDir != "/var/log/aibox/recordings/encrypted" {
		t.Errorf("EncryptedDir = %q, want /var/log/aibox/recordings/encrypted", cfg.EncryptedDir)
	}
	if cfg.MaxSizeMB != 500 {
		t.Errorf("MaxSizeMB = %d, want 500", cfg.MaxSizeMB)
	}
	if cfg.NoticeText == "" {
		t.Error("NoticeText should have default")
	}
}

func TestNewManager_DefaultsFilled(t *testing.T) {
	mgr := NewManager(Config{})

	cfg := mgr.Config()
	if cfg.RecordingsDir == "" {
		t.Error("RecordingsDir should have default")
	}
	if cfg.EncryptedDir == "" {
		t.Error("EncryptedDir should have default")
	}
	if cfg.NoticeText == "" {
		t.Error("NoticeText should have default")
	}
	if cfg.MaxSizeMB == 0 {
		t.Error("MaxSizeMB should have default")
	}
}

func TestNewManager_CustomConfig(t *testing.T) {
	mgr := NewManager(Config{
		Enabled:       true,
		RecordingsDir: "/custom/recordings",
		EncryptedDir:  "/custom/encrypted",
		MaxSizeMB:     1000,
	})

	cfg := mgr.Config()
	if cfg.RecordingsDir != "/custom/recordings" {
		t.Errorf("RecordingsDir = %q, want /custom/recordings", cfg.RecordingsDir)
	}
	if cfg.EncryptedDir != "/custom/encrypted" {
		t.Errorf("EncryptedDir = %q, want /custom/encrypted", cfg.EncryptedDir)
	}
	if cfg.MaxSizeMB != 1000 {
		t.Errorf("MaxSizeMB = %d, want 1000", cfg.MaxSizeMB)
	}
}

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		want    bool
	}{
		{"enabled", true, true},
		{"disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(Config{Enabled: tt.enabled})
			if got := mgr.IsEnabled(); got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNoticeText_ContainsRequiredInfo(t *testing.T) {
	mgr := NewManager(Config{})
	notice := mgr.NoticeText()

	requiredPhrases := []string{
		"SESSION RECORDING",
		"recorded",
		"security",
		"compliance",
		"encrypted",
		"acknowledge",
	}

	for _, phrase := range requiredPhrases {
		if !strings.Contains(strings.ToLower(notice), strings.ToLower(phrase)) {
			t.Errorf("notice should contain %q", phrase)
		}
	}
}

func TestSessionID_Unique(t *testing.T) {
	id1 := SessionID("sandbox-1", "user-1")
	id2 := SessionID("sandbox-1", "user-1")

	if id1 == id2 {
		t.Error("SessionID should generate unique IDs for each call")
	}
}

func TestSessionID_ContainsSandboxID(t *testing.T) {
	id := SessionID("my-sandbox", "user-1")
	if !strings.HasPrefix(id, "my-sandbox-") {
		t.Errorf("SessionID = %q, should start with sandbox ID", id)
	}
}

func TestSessionID_ContainsTimestamp(t *testing.T) {
	id := SessionID("sandbox-1", "user-1")

	// Should contain a date-like pattern (YYYYMMDD).
	parts := strings.Split(id, "-")
	if len(parts) < 3 {
		t.Errorf("SessionID = %q, should have at least 3 dash-separated parts", id)
	}
}

func TestEntrypointWrapper_DefaultShell(t *testing.T) {
	mgr := NewManager(Config{RecordingsDir: "/var/log/aibox/recordings"})
	wrapper := mgr.EntrypointWrapper("session-123", "")

	if !strings.Contains(wrapper, "script") {
		t.Error("wrapper should use script command")
	}
	if !strings.Contains(wrapper, "bash") {
		t.Error("wrapper should default to bash shell")
	}
	if !strings.Contains(wrapper, "session-123") {
		t.Error("wrapper should include session ID in file paths")
	}
	if !strings.Contains(wrapper, "--timing") {
		t.Error("wrapper should include timing flag for faithful playback")
	}
}

func TestEntrypointWrapper_CustomShell(t *testing.T) {
	mgr := NewManager(Config{RecordingsDir: "/recordings"})
	wrapper := mgr.EntrypointWrapper("session-456", "zsh")

	if !strings.Contains(wrapper, "zsh") {
		t.Error("wrapper should use the specified shell")
	}
}

func TestEntrypointWrapper_Paths(t *testing.T) {
	mgr := NewManager(Config{RecordingsDir: "/custom/dir"})
	wrapper := mgr.EntrypointWrapper("test-session", "bash")

	if !strings.Contains(wrapper, "/custom/dir/test-session.typescript") {
		t.Error("wrapper should use recording dir for typescript file")
	}
	if !strings.Contains(wrapper, "/custom/dir/test-session.timing") {
		t.Error("wrapper should use recording dir for timing file")
	}
}

func TestEnsureDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManager(Config{
		RecordingsDir: filepath.Join(tmpDir, "recordings"),
		EncryptedDir:  filepath.Join(tmpDir, "encrypted"),
	})

	if err := mgr.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories failed: %v", err)
	}

	for _, dir := range []string{
		filepath.Join(tmpDir, "recordings"),
		filepath.Join(tmpDir, "encrypted"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("directory not created: %s: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
		// Should be 0700 (owner-only).
		if info.Mode().Perm() != 0o700 {
			t.Errorf("directory %s has permissions %o, want 0700", dir, info.Mode().Perm())
		}
	}
}

func generateTestKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating test key: %v", err)
	}
	return key
}

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManager(Config{
		EncryptedDir: filepath.Join(tmpDir, "encrypted"),
	})

	key := generateTestKey(t)
	plaintext := []byte("This is a test terminal session recording.\nuser@sandbox:~$ ls\nfile1.go  file2.go\n")

	// Write raw recording.
	rawPath := filepath.Join(tmpDir, "test-session.typescript")
	if err := os.WriteFile(rawPath, plaintext, 0o644); err != nil {
		t.Fatalf("writing raw recording: %v", err)
	}

	// Encrypt.
	encPath, err := mgr.Encrypt(rawPath, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Encrypted file should exist.
	if _, err := os.Stat(encPath); err != nil {
		t.Fatalf("encrypted file not found: %v", err)
	}

	// Encrypted data should be different from plaintext.
	encData, err := os.ReadFile(encPath)
	if err != nil {
		t.Fatalf("reading encrypted file: %v", err)
	}
	if bytes.Equal(encData, plaintext) {
		t.Error("encrypted data should differ from plaintext")
	}

	// Decrypt should recover original.
	decrypted, err := mgr.Decrypt(encPath, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Error("decrypted data does not match original plaintext")
	}
}

func TestEncrypt_BadKeyLength(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManager(Config{
		EncryptedDir: filepath.Join(tmpDir, "encrypted"),
	})

	rawPath := filepath.Join(tmpDir, "test.typescript")
	if err := os.WriteFile(rawPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("writing raw recording: %v", err)
	}

	tests := []struct {
		name    string
		keySize int
	}{
		{"too short (16 bytes)", 16},
		{"too long (64 bytes)", 64},
		{"empty", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := make([]byte, tt.keySize)
			_, err := mgr.Encrypt(rawPath, key)
			if err == nil {
				t.Error("Encrypt should fail with wrong key size")
			}
		})
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManager(Config{
		EncryptedDir: filepath.Join(tmpDir, "encrypted"),
	})

	key1 := generateTestKey(t)
	key2 := generateTestKey(t)

	rawPath := filepath.Join(tmpDir, "test.typescript")
	if err := os.WriteFile(rawPath, []byte("secret session data"), 0o644); err != nil {
		t.Fatalf("writing raw recording: %v", err)
	}

	encPath, err := mgr.Encrypt(rawPath, key1)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Decrypting with wrong key should fail.
	_, err = mgr.Decrypt(encPath, key2)
	if err == nil {
		t.Error("Decrypt with wrong key should fail")
	}
}

func TestDecrypt_TamperedData(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManager(Config{
		EncryptedDir: filepath.Join(tmpDir, "encrypted"),
	})

	key := generateTestKey(t)

	rawPath := filepath.Join(tmpDir, "test.typescript")
	if err := os.WriteFile(rawPath, []byte("original data"), 0o644); err != nil {
		t.Fatalf("writing raw recording: %v", err)
	}

	encPath, err := mgr.Encrypt(rawPath, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Tamper with the encrypted file.
	data, _ := os.ReadFile(encPath)
	if len(data) > 20 {
		data[20] ^= 0xFF // flip a byte
	}
	if err := os.WriteFile(encPath, data, 0o600); err != nil {
		t.Fatalf("tampering with file: %v", err)
	}

	// Decryption should fail (GCM detects tampering).
	_, err = mgr.Decrypt(encPath, key)
	if err == nil {
		t.Error("Decrypt should fail for tampered data")
	}
}

func TestEncrypt_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManager(Config{
		EncryptedDir: filepath.Join(tmpDir, "encrypted"),
	})

	key := generateTestKey(t)

	// Create a 1MB recording.
	data := make([]byte, 1024*1024)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("generating test data: %v", err)
	}

	rawPath := filepath.Join(tmpDir, "large.typescript")
	if err := os.WriteFile(rawPath, data, 0o644); err != nil {
		t.Fatalf("writing large recording: %v", err)
	}

	encPath, err := mgr.Encrypt(rawPath, key)
	if err != nil {
		t.Fatalf("Encrypt large file failed: %v", err)
	}

	decrypted, err := mgr.Decrypt(encPath, key)
	if err != nil {
		t.Fatalf("Decrypt large file failed: %v", err)
	}
	if !bytes.Equal(decrypted, data) {
		t.Error("decrypted large file does not match original")
	}
}

func TestEncrypt_OutputFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	encDir := filepath.Join(tmpDir, "encrypted")
	mgr := NewManager(Config{
		EncryptedDir: encDir,
	})

	key := generateTestKey(t)
	rawPath := filepath.Join(tmpDir, "my-session.typescript")
	if err := os.WriteFile(rawPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("writing raw recording: %v", err)
	}

	encPath, err := mgr.Encrypt(rawPath, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	expected := filepath.Join(encDir, "my-session.typescript.enc")
	if encPath != expected {
		t.Errorf("encrypted path = %q, want %q", encPath, expected)
	}
}

func TestPlayback_DirectOutput(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManager(Config{
		EncryptedDir: filepath.Join(tmpDir, "encrypted"),
	})

	key := generateTestKey(t)
	content := []byte("session output line 1\nsession output line 2\n")

	rawPath := filepath.Join(tmpDir, "playback.typescript")
	if err := os.WriteFile(rawPath, content, 0o644); err != nil {
		t.Fatalf("writing raw recording: %v", err)
	}

	encPath, err := mgr.Encrypt(rawPath, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	var buf bytes.Buffer
	err = mgr.Playback(encPath, "", key, &buf)
	if err != nil {
		t.Fatalf("Playback failed: %v", err)
	}

	if !bytes.Equal(buf.Bytes(), content) {
		t.Errorf("Playback output = %q, want %q", buf.String(), string(content))
	}
}

func TestListSessions_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	encDir := filepath.Join(tmpDir, "encrypted")
	if err := os.MkdirAll(encDir, 0o700); err != nil {
		t.Fatalf("creating dir: %v", err)
	}

	mgr := NewManager(Config{EncryptedDir: encDir})
	sessions, err := mgr.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListSessions_FindsRecordings(t *testing.T) {
	tmpDir := t.TempDir()
	encDir := filepath.Join(tmpDir, "encrypted")
	if err := os.MkdirAll(encDir, 0o700); err != nil {
		t.Fatalf("creating dir: %v", err)
	}

	// Create some fake encrypted recordings.
	files := []string{
		"sandbox1-20260221-120000-abcd.typescript.enc",
		"sandbox2-20260221-130000-efgh.typescript.enc",
		"not-a-recording.txt",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(encDir, f), []byte("data"), 0o600); err != nil {
			t.Fatalf("creating test file: %v", err)
		}
	}

	mgr := NewManager(Config{EncryptedDir: encDir})
	sessions, err := mgr.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestListSessions_NonexistentDir(t *testing.T) {
	mgr := NewManager(Config{EncryptedDir: "/nonexistent/path"})
	sessions, err := mgr.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions should not error for nonexistent dir: %v", err)
	}
	if sessions != nil {
		t.Error("expected nil sessions for nonexistent dir")
	}
}

func TestHealthCheck_Disabled(t *testing.T) {
	mgr := NewManager(Config{Enabled: false})
	if err := mgr.HealthCheck(); err != nil {
		t.Errorf("HealthCheck should pass when disabled: %v", err)
	}
}

func TestEncryptAESGCM_InternalRoundtrip(t *testing.T) {
	key := generateTestKey(t)
	plaintext := []byte("internal roundtrip test")

	ciphertext, err := encryptAESGCM(plaintext, key)
	if err != nil {
		t.Fatalf("encryptAESGCM failed: %v", err)
	}

	// Ciphertext should be larger than plaintext (nonce + tag).
	if len(ciphertext) <= len(plaintext) {
		t.Error("ciphertext should be larger than plaintext")
	}

	decrypted, err := decryptAESGCM(ciphertext, key)
	if err != nil {
		t.Fatalf("decryptAESGCM failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Error("roundtrip failed: decrypted does not match plaintext")
	}
}

func TestDecryptAESGCM_TooShort(t *testing.T) {
	key := generateTestKey(t)
	_, err := decryptAESGCM([]byte("short"), key)
	if err == nil {
		t.Error("decryptAESGCM should fail for too-short ciphertext")
	}
}

func TestEncryptAESGCM_DifferentCiphertexts(t *testing.T) {
	key := generateTestKey(t)
	plaintext := []byte("same plaintext")

	ct1, err := encryptAESGCM(plaintext, key)
	if err != nil {
		t.Fatalf("first encryption failed: %v", err)
	}

	ct2, err := encryptAESGCM(plaintext, key)
	if err != nil {
		t.Fatalf("second encryption failed: %v", err)
	}

	// Same plaintext + same key should produce different ciphertexts (random nonce).
	if bytes.Equal(ct1, ct2) {
		t.Error("two encryptions of same plaintext should produce different ciphertexts")
	}
}

// --- Audit integration tests ---

func TestSessionStartEvent(t *testing.T) {
	ev := SessionStartEvent("session-abc", "sandbox-1", "user-42")

	if ev.EventType != audit.EventSessionStart {
		t.Errorf("EventType = %q, want %q", ev.EventType, audit.EventSessionStart)
	}
	if ev.SandboxID != "sandbox-1" {
		t.Errorf("SandboxID = %q, want sandbox-1", ev.SandboxID)
	}
	if ev.UserID != "user-42" {
		t.Errorf("UserID = %q, want user-42", ev.UserID)
	}
	if ev.Source != audit.SourceRecorder {
		t.Errorf("Source = %q, want %q", ev.Source, audit.SourceRecorder)
	}
	if ev.Severity != audit.SeverityInfo {
		t.Errorf("Severity = %q, want %q", ev.Severity, audit.SeverityInfo)
	}
	if ev.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
	if ev.Details["session_id"] != "session-abc" {
		t.Errorf("Details session_id = %v, want session-abc", ev.Details["session_id"])
	}
	if ev.Details["action"] != "start" {
		t.Errorf("Details action = %v, want start", ev.Details["action"])
	}
}

func TestSessionEndEvent(t *testing.T) {
	ev := SessionEndEvent("session-abc", "sandbox-1", "user-42", 1048576)

	if ev.EventType != audit.EventSessionEnd {
		t.Errorf("EventType = %q, want %q", ev.EventType, audit.EventSessionEnd)
	}
	if ev.SandboxID != "sandbox-1" {
		t.Errorf("SandboxID = %q, want sandbox-1", ev.SandboxID)
	}
	if ev.UserID != "user-42" {
		t.Errorf("UserID = %q, want user-42", ev.UserID)
	}
	if ev.Source != audit.SourceRecorder {
		t.Errorf("Source = %q, want %q", ev.Source, audit.SourceRecorder)
	}
	if ev.Severity != audit.SeverityInfo {
		t.Errorf("Severity = %q, want %q", ev.Severity, audit.SeverityInfo)
	}
	if ev.Details["session_id"] != "session-abc" {
		t.Errorf("Details session_id = %v, want session-abc", ev.Details["session_id"])
	}
	if ev.Details["action"] != "end" {
		t.Errorf("Details action = %v, want end", ev.Details["action"])
	}
	if ev.Details["size_bytes"] != int64(1048576) {
		t.Errorf("Details size_bytes = %v, want 1048576", ev.Details["size_bytes"])
	}
}

func TestSessionStartEvent_Validates(t *testing.T) {
	ev := SessionStartEvent("session-1", "sandbox-1", "user-1")
	if err := ev.Validate(); err != nil {
		t.Errorf("SessionStartEvent should pass validation: %v", err)
	}
}

func TestSessionEndEvent_Validates(t *testing.T) {
	ev := SessionEndEvent("session-1", "sandbox-1", "user-1", 500)
	if err := ev.Validate(); err != nil {
		t.Errorf("SessionEndEvent should pass validation: %v", err)
	}
}

// TestRecordingEvents_FileLoggerIntegration verifies that recording events
// flow through the audit FileLogger with hash chain integrity, then can be
// read back and verified -- proving clean integration with the audit pipeline.
func TestRecordingEvents_FileLoggerIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.jsonl")

	logger, err := audit.NewFileLogger(audit.FileLoggerConfig{
		Path:      logPath,
		MaxSizeMB: 10,
	})
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}

	ctx := context.Background()

	// Emit a session start event.
	startEvent := SessionStartEvent("session-e2e", "sandbox-e2e", "user-e2e")
	if err := logger.Log(ctx, startEvent); err != nil {
		t.Fatalf("Log SessionStartEvent: %v", err)
	}

	// Emit a session end event.
	endEvent := SessionEndEvent("session-e2e", "sandbox-e2e", "user-e2e", 42000)
	if err := logger.Log(ctx, endEvent); err != nil {
		t.Fatalf("Log SessionEndEvent: %v", err)
	}

	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Read events back and verify hash chain.
	events, err := audit.ReadEvents(logPath)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Verify event types.
	if events[0].EventType != audit.EventSessionStart {
		t.Errorf("event[0].EventType = %q, want session.start", events[0].EventType)
	}
	if events[1].EventType != audit.EventSessionEnd {
		t.Errorf("event[1].EventType = %q, want session.end", events[1].EventType)
	}

	// Verify hash chain integrity.
	result := audit.VerifyChain(events, audit.GenesisHash)
	if !result.IsIntact {
		t.Errorf("hash chain broken at index %d (expected %s, got %s)",
			result.BrokenAt, result.ExpectedHash, result.ActualHash)
	}
	if result.Verified != 2 {
		t.Errorf("verified %d events, want 2", result.Verified)
	}
}

// TestRecordingEvents_ImmutableStorageIntegration verifies that recording
// events can be serialized, stored in the immutable storage backend, read
// back with checksum verification, and the hash chain remains intact.
func TestRecordingEvents_ImmutableStorageIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.jsonl")

	// Write events through FileLogger to get hash chain.
	logger, err := audit.NewFileLogger(audit.FileLoggerConfig{
		Path:      logPath,
		MaxSizeMB: 10,
	})
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}

	ctx := context.Background()

	startEvent := SessionStartEvent("session-store", "sandbox-store", "user-store")
	if err := logger.Log(ctx, startEvent); err != nil {
		t.Fatalf("Log start: %v", err)
	}

	endEvent := SessionEndEvent("session-store", "sandbox-store", "user-store", 99000)
	if err := logger.Log(ctx, endEvent); err != nil {
		t.Fatalf("Log end: %v", err)
	}

	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Read raw JSONL lines for storage.
	rawData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var entries [][]byte
	for _, line := range bytes.Split(rawData, []byte("\n")) {
		if len(line) > 0 {
			entries = append(entries, line)
		}
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 JSONL entries, got %d", len(entries))
	}

	// Store in immutable backend.
	storageDir := filepath.Join(tmpDir, "storage")
	backend, err := storage.NewLocalBackend(storage.LocalConfig{BaseDir: storageDir})
	if err != nil {
		t.Fatalf("NewLocalBackend: %v", err)
	}

	batch := storage.Batch{Entries: entries}
	key, err := backend.Append(ctx, batch)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Read back and verify checksum.
	readBack, err := backend.Read(ctx, key)
	if err != nil {
		t.Fatalf("Read: %v (checksum verification failed = tampered data)", err)
	}

	if len(readBack.Entries) != 2 {
		t.Fatalf("read back %d entries, want 2", len(readBack.Entries))
	}

	// Verify the stored events via storage.Verify.
	verifyResult, err := storage.Verify(ctx, backend)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if verifyResult.TotalBatches != 1 {
		t.Errorf("TotalBatches = %d, want 1", verifyResult.TotalBatches)
	}
	if verifyResult.TotalEvents != 2 {
		t.Errorf("TotalEvents = %d, want 2", verifyResult.TotalEvents)
	}
	if !verifyResult.ChainIntact {
		t.Errorf("chain broken at event %d: %s", verifyResult.ChainBrokenAt, verifyResult.FirstError)
	}
	if verifyResult.CorruptBatches != 0 {
		t.Errorf("CorruptBatches = %d, want 0", verifyResult.CorruptBatches)
	}
}
