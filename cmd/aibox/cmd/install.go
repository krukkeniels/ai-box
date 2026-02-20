package cmd

import (
	"fmt"
	"strings"

	"github.com/aibox/aibox/internal/container"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install <toolpack>",
	Short: "Install a tool pack into the running sandbox",
	Long: `Install adds a tool pack (e.g. bazel@7, node@20) into the running
AI-Box sandbox container. Tool packs provide additional build tools
and language runtimes beyond what is included in the base image.`,
	Args: cobra.ExactArgs(1),
	RunE: runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	pack := args[0]

	// Validate tool pack format: name@version.
	parts := strings.SplitN(pack, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid tool pack format %q: expected name@version (e.g. bazel@7)", pack)
	}

	// Validate a container is running.
	mgr, err := container.NewManager(Cfg)
	if err != nil {
		return err
	}

	info, err := mgr.Status()
	if err != nil {
		return err
	}

	if info.State != "running" {
		return fmt.Errorf("no running AI-Box container found; start one first with 'aibox start --workspace <path>'")
	}

	fmt.Printf("Tool pack: %s (name=%s, version=%s)\n", pack, parts[0], parts[1])
	fmt.Println("Runtime tool pack installation requires Phase 2 infrastructure (package mirrors, volume mounts).")
	fmt.Println("This feature will be available once Phase 2 is complete.")

	return nil
}
