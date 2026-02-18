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

	return nil
}
