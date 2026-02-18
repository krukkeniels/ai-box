package cmd

import (
	"github.com/aibox/aibox/internal/container"
	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Open an interactive shell in the running sandbox",
	Long: `Shell attaches to the running AI-Box sandbox container and
provides an interactive bash session. The container must already
be running (use 'aibox start' first).`,
	RunE: runShell,
}

func init() {
	rootCmd.AddCommand(shellCmd)
}

func runShell(cmd *cobra.Command, args []string) error {
	mgr, err := container.NewManager(Cfg)
	if err != nil {
		return err
	}

	return mgr.Shell("")
}
