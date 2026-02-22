package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aibox/aibox/internal/config"
	"github.com/spf13/cobra"
)

// approvalRequest mirrors the struct from the git remote helper.
type approvalRequest struct {
	ID           string    `json:"id"`
	User         string    `json:"user"`
	SourceBranch string    `json:"source_branch"`
	TargetBranch string    `json:"target_branch"`
	StagingRef   string    `json:"staging_ref"`
	CommitRange  string    `json:"commit_range"`
	RemoteURL    string    `json:"remote_url"`
	CreatedAt    time.Time `json:"created_at"`
	Status       string    `json:"status"`
}

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Manage pending git push approvals",
	Long: `Push provides subcommands for viewing and managing pending
git push approval requests created by the non-blocking approval flow.`,
}

var pushStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show pending push approval requests",
	Long:  `Status displays all pending git push approvals that are awaiting review.`,
	RunE:  runPushStatus,
}

var pushCancelCmd = &cobra.Command{
	Use:   "cancel <request-id>",
	Short: "Cancel a pending push approval",
	Long:  `Cancel removes a pending push approval and deletes the staging ref from the remote.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runPushCancel,
}

func init() {
	pushCmd.AddCommand(pushStatusCmd)
	pushCmd.AddCommand(pushCancelCmd)
	rootCmd.AddCommand(pushCmd)
}

func runPushStatus(cmd *cobra.Command, args []string) error {
	requests, err := loadApprovalRequests()
	if err != nil {
		return err
	}

	if len(requests) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No pending push approvals.")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pending push approvals:\n\n")
	for _, req := range requests {
		age := time.Since(req.CreatedAt).Truncate(time.Minute)
		fmt.Fprintf(cmd.OutOrStdout(), "  ID:          %s\n", req.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "  Target:      %s\n", req.TargetBranch)
		fmt.Fprintf(cmd.OutOrStdout(), "  Commits:     %s\n", req.CommitRange)
		fmt.Fprintf(cmd.OutOrStdout(), "  Staging ref: %s\n", req.StagingRef)
		fmt.Fprintf(cmd.OutOrStdout(), "  Status:      %s\n", req.Status)
		fmt.Fprintf(cmd.OutOrStdout(), "  Age:         %s\n", age)
		fmt.Fprintln(cmd.OutOrStdout())
	}

	return nil
}

func runPushCancel(cmd *cobra.Command, args []string) error {
	id := args[0]

	homeDir, err := config.ResolveHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}

	dir := filepath.Join(homeDir, ".local", "share", "aibox", "push-approvals")
	path := filepath.Join(dir, id+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("approval request %q not found", id)
	}

	var req approvalRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("parsing approval request: %w", err)
	}

	if req.Status != "pending" {
		return fmt.Errorf("approval request %q is already %s", id, req.Status)
	}

	// Remove the request file.
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("removing approval request: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Cancelled push approval: %s\n", id)
	fmt.Fprintf(cmd.OutOrStdout(), "Note: the staging ref %s may still exist on the remote.\n", req.StagingRef)
	fmt.Fprintf(cmd.OutOrStdout(), "To clean up: git push %s --delete %s\n", req.RemoteURL, req.StagingRef)

	return nil
}

// loadApprovalRequests reads all approval request files.
func loadApprovalRequests() ([]approvalRequest, error) {
	homeDir, err := config.ResolveHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolving home directory: %w", err)
	}

	dir := filepath.Join(homeDir, ".local", "share", "aibox", "push-approvals")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading approval dir: %w", err)
	}

	var requests []approvalRequest
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, e.Name()))
		if readErr != nil {
			continue
		}
		var req approvalRequest
		if err := json.Unmarshal(data, &req); err != nil {
			continue
		}
		requests = append(requests, req)
	}
	return requests, nil
}
