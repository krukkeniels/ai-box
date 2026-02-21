package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/aibox/aibox/internal/container"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current state of the AI-Box sandbox",
	Long: `Status displays information about the running sandbox container
including its state, resource usage, workspace mount, and runtime.`,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().String("format", "text", "output format (text or json)")

	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	mgr, err := container.NewManager(Cfg)
	if err != nil {
		return err
	}

	info, err := mgr.Status()
	if err != nil {
		return err
	}

	format, _ := cmd.Flags().GetString("format")
	if format == "json" {
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if info.State == "not-found" {
		fmt.Println("No AI-Box container found. Run 'aibox start --workspace <path>' to create one.")
		return nil
	}

	fmt.Printf("AI-Box Sandbox Status\n")
	fmt.Printf("  Container: %s\n", info.Name)
	fmt.Printf("  State:     %s\n", info.State)
	if info.Image != "" {
		fmt.Printf("  Image:     %s\n", info.Image)
	}
	if info.Runtime != "" {
		fmt.Printf("  Runtime:   %s\n", info.Runtime)
	}
	if info.Workspace != "" {
		fmt.Printf("  Workspace: %s\n", info.Workspace)
	}

	// Policy and credential status (Phase 3).
	if Cfg != nil {
		fmt.Printf("\nPolicy:\n")
		fmt.Printf("  Org baseline:  %s\n", Cfg.Policy.OrgBaselinePath)
		if Cfg.Policy.TeamPolicyPath != "" {
			fmt.Printf("  Team policy:   %s\n", Cfg.Policy.TeamPolicyPath)
		}
		fmt.Printf("  Project policy: %s\n", Cfg.Policy.ProjectPolicyPath)
		fmt.Printf("  Decision log:  %s\n", Cfg.Policy.DecisionLogPath)

		fmt.Printf("\nCredentials:\n")
		fmt.Printf("  Mode: %s\n", Cfg.Credentials.Mode)
		if Cfg.Credentials.Mode == "vault" {
			fmt.Printf("  Vault: %s\n", Cfg.Credentials.VaultAddr)
			fmt.Printf("  SPIFFE domain: %s\n", Cfg.Credentials.SPIFFETrustDomain)
		}
	}

	return nil
}
