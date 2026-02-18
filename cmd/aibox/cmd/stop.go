package cmd

import (
	"github.com/aibox/aibox/internal/container"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running AI-Box sandbox container",
	Long: `Stop gracefully shuts down the active AI-Box sandbox container
with a 10-second timeout, then force-kills if necessary.
Named volumes (home, toolpacks) are preserved for the next start.`,
	RunE: runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	mgr, err := container.NewManager(Cfg)
	if err != nil {
		return err
	}

	return mgr.Stop("")
}
