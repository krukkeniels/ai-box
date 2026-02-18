package cmd

import (
	"github.com/aibox/aibox/internal/container"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a sandboxed AI development environment",
	Long: `Start launches a new gVisor-sandboxed container with the specified
workspace mounted. The container provides a secure environment for
AI-assisted code generation and execution.

The workspace directory is bind-mounted into the container at /workspace.
All mandatory security controls (seccomp, AppArmor, capability drop,
read-only rootfs) are applied automatically.`,
	RunE: runStart,
}

func init() {
	startCmd.Flags().StringP("workspace", "w", "", "path to workspace directory (required)")
	startCmd.Flags().String("image", "", "container image to use (overrides config)")
	startCmd.Flags().Int("cpus", 0, "CPU limit (overrides config)")
	startCmd.Flags().String("memory", "", "memory limit, e.g. 8g (overrides config)")

	_ = startCmd.MarkFlagRequired("workspace")

	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	workspace, _ := cmd.Flags().GetString("workspace")
	image, _ := cmd.Flags().GetString("image")
	cpus, _ := cmd.Flags().GetInt("cpus")
	memory, _ := cmd.Flags().GetString("memory")

	// Validate workspace early so the user gets a clear error before we
	// check for the container runtime.
	if _, err := container.ValidateWorkspace(workspace); err != nil {
		return err
	}

	mgr, err := container.NewManager(Cfg)
	if err != nil {
		return err
	}

	return mgr.Start(container.StartOpts{
		Workspace: workspace,
		Image:     image,
		CPUs:      cpus,
		Memory:    memory,
	})
}
