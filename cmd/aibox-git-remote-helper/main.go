// aibox-git-remote-helper implements Git's remote helper protocol to intercept
// git push when policy sets it to review-required. It redirects pushes to
// a staging ref (refs/aibox/staging/<user>/<timestamp>) and creates an
// approval request. The developer is NOT blocked -- they see a confirmation
// message and can continue working.
//
// For repos where git push is "safe" in policy, the push proceeds normally
// by delegating to the real remote helper (git-remote-https or git-remote-ssh).
//
// Installation: git remote helper binaries are discovered by name convention.
// When a remote URL uses the "aibox" transport (e.g., aibox://git.internal/repo),
// Git invokes this binary automatically.
//
// The helper can also be configured as a push wrapper via:
//   git config --global remote.origin.pushurl "aibox://git.internal/repo.git"
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

// ApprovalRequest represents a pending push approval.
type ApprovalRequest struct {
	ID           string    `json:"id"`
	User         string    `json:"user"`
	SourceBranch string    `json:"source_branch"`
	TargetBranch string    `json:"target_branch"`
	StagingRef   string    `json:"staging_ref"`
	CommitRange  string    `json:"commit_range"`
	RemoteURL    string    `json:"remote_url"`
	CreatedAt    time.Time `json:"created_at"`
	Status       string    `json:"status"` // "pending", "approved", "rejected"
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: git-remote-aibox <remote-name> <url>")
		os.Exit(1)
	}

	remoteName := os.Args[1]
	remoteURL := os.Args[2]

	// Parse the real URL from the aibox:// transport.
	realURL := parseAiboxURL(remoteURL)

	// Check if push requires review by examining policy.
	mode := getPushMode()

	if err := runHelper(os.Stdin, os.Stdout, os.Stderr, remoteName, realURL, mode); err != nil {
		fmt.Fprintf(os.Stderr, "aibox-git-remote-helper: %v\n", err)
		os.Exit(1)
	}
}

// pushMode determines how git push is handled.
type pushMode int

const (
	pushSafe           pushMode = iota // push proceeds normally
	pushReviewRequired                 // push redirected to staging ref
)

// getPushMode checks the AIBOX_GIT_PUSH_MODE env var.
// Default is "safe" unless the policy engine sets it to "review-required".
func getPushMode() pushMode {
	if os.Getenv("AIBOX_GIT_PUSH_MODE") == "review-required" {
		return pushReviewRequired
	}
	return pushSafe
}

// parseAiboxURL converts aibox://host/path to https://host/path.
func parseAiboxURL(url string) string {
	if strings.HasPrefix(url, "aibox://") {
		return "https://" + strings.TrimPrefix(url, "aibox://")
	}
	return url
}

// runHelper implements the git remote helper protocol.
// It reads commands from stdin and writes responses to stdout.
func runHelper(in io.Reader, out, errOut io.Writer, remoteName, remoteURL string, mode pushMode) error {
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		cmd, rest, _ := strings.Cut(line, " ")

		switch cmd {
		case "capabilities":
			// Report capabilities.
			fmt.Fprintln(out, "push")
			fmt.Fprintln(out, "")

		case "push":
			if err := handlePush(out, errOut, rest, remoteName, remoteURL, mode); err != nil {
				return err
			}

		default:
			// Unknown command -- fall through.
			return fmt.Errorf("unsupported command: %q", cmd)
		}
	}
	return scanner.Err()
}

// handlePush processes a push refspec.
// Format: "+<src>:<dst>" or "<src>:<dst>"
func handlePush(out, errOut io.Writer, refspec, remoteName, remoteURL string, mode pushMode) error {
	// Parse refspec.
	force := strings.HasPrefix(refspec, "+")
	if force {
		refspec = refspec[1:]
	}

	src, dst, ok := strings.Cut(refspec, ":")
	if !ok {
		return fmt.Errorf("invalid refspec: %q", refspec)
	}

	if mode == pushSafe {
		// Delegate directly to git push.
		return delegatePush(out, errOut, remoteURL, src, dst, force)
	}

	// Review-required mode: redirect to staging ref.
	return stagePush(out, errOut, remoteURL, src, dst, force)
}

// delegatePush runs the real git push via subprocess.
func delegatePush(out, errOut io.Writer, remoteURL, src, dst string, force bool) error {
	args := []string{"push", remoteURL}
	refspec := src + ":" + dst
	if force {
		refspec = "+" + refspec
	}
	args = append(args, refspec)

	cmd := exec.Command("git", args...)
	cmd.Stdout = errOut // git push output goes to stderr
	cmd.Stderr = errOut
	if err := cmd.Run(); err != nil {
		// Report failure in the protocol format.
		fmt.Fprintf(out, "error %s push failed\n", dst)
		fmt.Fprintln(out, "")
		return nil
	}

	fmt.Fprintf(out, "ok %s\n", dst)
	fmt.Fprintln(out, "")
	return nil
}

// stagePush redirects the push to a staging ref and creates an approval request.
func stagePush(out, errOut io.Writer, remoteURL, src, dst string, force bool) error {
	username := currentUser()
	timestamp := time.Now().UTC().Format("20060102-150405")
	stagingRef := fmt.Sprintf("refs/aibox/staging/%s/%s", username, timestamp)

	// Push to the staging ref instead of the real destination.
	refspec := src + ":" + stagingRef
	if force {
		refspec = "+" + refspec
	}
	pushArgs := []string{"push", remoteURL, refspec}

	cmd := exec.Command("git", pushArgs...)
	cmd.Stdout = errOut
	cmd.Stderr = errOut
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(out, "error %s staging push failed\n", dst)
		fmt.Fprintln(out, "")
		return nil
	}

	// Determine commit range for the approval request.
	commitRange := resolveCommitRange(src)

	// Create and save the approval request.
	req := ApprovalRequest{
		ID:           fmt.Sprintf("%s-%s", username, timestamp),
		User:         username,
		SourceBranch: src,
		TargetBranch: dst,
		StagingRef:   stagingRef,
		CommitRange:  commitRange,
		RemoteURL:    remoteURL,
		CreatedAt:    time.Now().UTC(),
		Status:       "pending",
	}

	if err := saveApprovalRequest(req); err != nil {
		fmt.Fprintf(errOut, "warning: could not save approval request: %v\n", err)
	}

	// Send webhook notification.
	if webhookURL := os.Getenv("AIBOX_PUSH_WEBHOOK_URL"); webhookURL != "" {
		if err := notifyWebhook(webhookURL, req); err != nil {
			fmt.Fprintf(errOut, "warning: webhook notification failed: %v\n", err)
		}
	}

	// Print user-friendly message.
	fmt.Fprintf(errOut, "\n")
	fmt.Fprintf(errOut, "Push staged for approval.\n")
	fmt.Fprintf(errOut, "  Staging ref: %s\n", stagingRef)
	fmt.Fprintf(errOut, "  Target:      %s\n", dst)
	fmt.Fprintf(errOut, "  Commits:     %s\n", commitRange)
	fmt.Fprintf(errOut, "\n")
	fmt.Fprintf(errOut, "Continue working. Check status: aibox push status\n")
	fmt.Fprintf(errOut, "\n")

	// Report success to git (the staging push succeeded).
	fmt.Fprintf(out, "ok %s\n", dst)
	fmt.Fprintln(out, "")
	return nil
}

// resolveCommitRange returns a short commit range description.
func resolveCommitRange(src string) string {
	// Try to get the short SHA of HEAD.
	cmd := exec.Command("git", "rev-parse", "--short", src)
	out, err := cmd.Output()
	if err != nil {
		return src
	}
	return strings.TrimSpace(string(out))
}

// currentUser returns the current username.
func currentUser() string {
	u, err := user.Current()
	if err != nil {
		return "unknown"
	}
	return u.Username
}

// approvalDir returns the directory where approval requests are stored.
func approvalDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/aibox-approvals"
	}
	return filepath.Join(home, ".local", "share", "aibox", "push-approvals")
}

// saveApprovalRequest persists an approval request to disk.
func saveApprovalRequest(req ApprovalRequest) error {
	dir := approvalDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating approval dir: %w", err)
	}

	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling approval request: %w", err)
	}

	path := filepath.Join(dir, req.ID+".json")
	return os.WriteFile(path, data, 0o644)
}

// LoadApprovalRequests reads all pending approval requests from disk.
func LoadApprovalRequests() ([]ApprovalRequest, error) {
	dir := approvalDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading approval dir: %w", err)
	}

	var requests []ApprovalRequest
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var req ApprovalRequest
		if err := json.Unmarshal(data, &req); err != nil {
			continue
		}
		requests = append(requests, req)
	}
	return requests, nil
}

// CancelApprovalRequest removes a pending approval and deletes the staging ref.
func CancelApprovalRequest(id string) error {
	dir := approvalDir()
	path := filepath.Join(dir, id+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("approval request %q not found", id)
	}

	var req ApprovalRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("parsing approval request: %w", err)
	}

	if req.Status != "pending" {
		return fmt.Errorf("approval request %q is already %s", id, req.Status)
	}

	// Delete the staging ref from the remote.
	delCmd := exec.Command("git", "push", req.RemoteURL, "--delete", req.StagingRef)
	if out, err := delCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not delete staging ref: %s\n", string(out))
	}

	// Remove the request file.
	return os.Remove(path)
}

// notifyWebhook sends a webhook notification about the staged push.
func notifyWebhook(url string, req ApprovalRequest) error {
	payload, err := json.Marshal(map[string]interface{}{
		"text": fmt.Sprintf(
			"Push approval requested by %s\nTarget: %s\nCommits: %s\nStaging ref: %s",
			req.User, req.TargetBranch, req.CommitRange, req.StagingRef,
		),
		"approval_request": req,
	})
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("webhook POST failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}
