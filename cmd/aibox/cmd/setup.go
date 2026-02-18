package cmd

import (
	"fmt"

	"github.com/aibox/aibox/internal/host"
	"github.com/aibox/aibox/internal/setup"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Initialize AI-Box environment on this host",
	Long: `Setup verifies host prerequisites (runtime, gVisor, kernel features),
pulls required container images, and creates the initial configuration.

On native Linux, setup will:
  - Detect and verify the container runtime (Podman/Docker)
  - Check gVisor (runsc) installation
  - Install the seccomp profile to /etc/aibox/seccomp.json
  - Load the AppArmor profile (aibox-sandbox)
  - Create default config at ~/.config/aibox/config.yaml
  - Pull and verify the base container image

On WSL2, setup additionally:
  - Checks WSL2 kernel version (5.15+ required for gVisor systrap)
  - Verifies memory allocation (12+ GB recommended)
  - Checks Podman machine status if applicable`,
	RunE: runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	hostInfo := host.Detect()

	if !hostInfo.IsSupported() {
		return fmt.Errorf("unsupported host OS: %s. AI-Box requires Linux (native or WSL2)", hostInfo.OS)
	}

	if hostInfo.IsWSL2 {
		return setup.WSL2Setup(Cfg)
	}

	return setup.LinuxSetup(Cfg)
}
