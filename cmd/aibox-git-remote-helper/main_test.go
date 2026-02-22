package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAiboxURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"aibox://git.internal/repo.git", "https://git.internal/repo.git"},
		{"https://git.internal/repo.git", "https://git.internal/repo.git"},
		{"git@git.internal:repo.git", "git@git.internal:repo.git"},
		{"aibox://", "https://"},
	}

	for _, tc := range tests {
		got := parseAiboxURL(tc.input)
		if got != tc.want {
			t.Errorf("parseAiboxURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestGetPushMode(t *testing.T) {
	t.Setenv("AIBOX_GIT_PUSH_MODE", "")
	if got := getPushMode(); got != pushSafe {
		t.Errorf("default mode = %d, want pushSafe", got)
	}

	t.Setenv("AIBOX_GIT_PUSH_MODE", "review-required")
	if got := getPushMode(); got != pushReviewRequired {
		t.Errorf("review-required mode = %d, want pushReviewRequired", got)
	}

	t.Setenv("AIBOX_GIT_PUSH_MODE", "safe")
	if got := getPushMode(); got != pushSafe {
		t.Errorf("safe mode = %d, want pushSafe", got)
	}
}

func TestRunHelperCapabilities(t *testing.T) {
	input := "capabilities\n\n"
	var out bytes.Buffer
	var errOut bytes.Buffer

	err := runHelper(strings.NewReader(input), &out, &errOut, "origin", "https://git.internal/repo.git", pushSafe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "push") {
		t.Errorf("capabilities should include 'push', got: %q", output)
	}
}

func TestRunHelperUnknownCommand(t *testing.T) {
	input := "fetch refs/heads/main\n"
	var out, errOut bytes.Buffer

	err := runHelper(strings.NewReader(input), &out, &errOut, "origin", "https://git.internal/repo.git", pushSafe)
	if err == nil {
		t.Error("expected error for unsupported command")
	}
}

func TestSaveAndLoadApprovalRequests(t *testing.T) {
	// Override the approval dir via HOME.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	req := ApprovalRequest{
		ID:           "testuser-20260221-120000",
		User:         "testuser",
		SourceBranch: "refs/heads/main",
		TargetBranch: "refs/heads/main",
		StagingRef:   "refs/aibox/staging/testuser/20260221-120000",
		CommitRange:  "abc1234",
		RemoteURL:    "https://git.internal/repo.git",
		Status:       "pending",
	}

	if err := saveApprovalRequest(req); err != nil {
		t.Fatalf("saveApprovalRequest failed: %v", err)
	}

	// Verify file exists.
	path := filepath.Join(approvalDir(), req.ID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("approval file not found: %v", err)
	}

	var loaded ApprovalRequest
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("parsing approval file: %v", err)
	}

	if loaded.ID != req.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, req.ID)
	}
	if loaded.StagingRef != req.StagingRef {
		t.Errorf("StagingRef = %q, want %q", loaded.StagingRef, req.StagingRef)
	}
	if loaded.Status != "pending" {
		t.Errorf("Status = %q, want %q", loaded.Status, "pending")
	}

	// Test LoadApprovalRequests.
	requests, err := LoadApprovalRequests()
	if err != nil {
		t.Fatalf("LoadApprovalRequests failed: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}
	if requests[0].ID != req.ID {
		t.Errorf("loaded request ID = %q, want %q", requests[0].ID, req.ID)
	}
}

func TestLoadApprovalRequests_EmptyDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	requests, err := LoadApprovalRequests()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(requests) != 0 {
		t.Errorf("expected 0 requests, got %d", len(requests))
	}
}

func TestNotifyWebhook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req := ApprovalRequest{
		ID:           "testuser-20260221-120000",
		User:         "testuser",
		TargetBranch: "refs/heads/main",
		CommitRange:  "abc1234",
		StagingRef:   "refs/aibox/staging/testuser/20260221-120000",
	}

	err := notifyWebhook(srv.URL, req)
	if err != nil {
		t.Fatalf("notifyWebhook failed: %v", err)
	}
}

func TestNotifyWebhook_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	req := ApprovalRequest{
		ID:   "test",
		User: "testuser",
	}

	err := notifyWebhook(srv.URL, req)
	if err == nil {
		t.Error("expected error for server error response")
	}
}

func TestCancelApprovalRequest_NotFound(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := CancelApprovalRequest("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent request")
	}
}
