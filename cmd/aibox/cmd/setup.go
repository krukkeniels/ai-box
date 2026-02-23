package cmd

import (
	"fmt"

	"github.com/aibox/aibox/internal/host"
	"github.com/aibox/aibox/internal/setup"
	"github.com/spf13/cobra"
)

var (
	systemSetup bool
	forceSetup  bool
	offlineMode bool
	skipPull    bool
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Initialize AI-Box environment on this host",
	Long: `Setup prepares a host for running AI-Box sandboxes.

There are two phases:

  aibox setup --system   (requires root, run once per machine)
    Installs system-wide security profiles and network services:
    - Seccomp profile to /etc/aibox/seccomp.json
    - AppArmor profile (aibox-sandbox)
    - nftables rules for container network isolation
    - Squid proxy configuration and service
    - CoreDNS configuration and service

  aibox setup            (no root needed, run by each developer)
    Sets up the user environment:
    - Runs pre-flight checks (OS, RAM, disk, runtime)
    - Detects and verifies the container runtime (Podman/Docker)
    - Checks gVisor (runsc) installation
    - Creates default config at ~/.config/aibox/config.yaml
    - Generates SSH keys for IDE access
    - Pulls and verifies the base container image
    - Runs health checks

Flags:
  --force      Re-run all steps regardless of existing state
  --offline    Skip steps that require network access
  --skip-pull  Skip image pull even if not cached locally

On WSL2, setup additionally:
  - Checks WSL2 kernel version (5.15+ required for gVisor systrap)
  - Verifies memory allocation (12+ GB recommended)
  - Checks Podman machine status if applicable`,
	RunE: runSetup,
}

func init() {
	setupCmd.Flags().BoolVar(&systemSetup, "system", false, "run privileged system-level setup (requires root)")
	setupCmd.Flags().BoolVar(&forceSetup, "force", false, "re-run all steps regardless of existing state")
	setupCmd.Flags().BoolVar(&offlineMode, "offline", false, "skip steps that require network access")
	setupCmd.Flags().BoolVar(&skipPull, "skip-pull", false, "skip image pull even if not cached locally")
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	hostInfo := host.Detect()

	if !hostInfo.IsSupported() {
		return fmt.Errorf("unsupported host OS: %s. AI-Box requires Linux (native or WSL2)", hostInfo.OS)
	}

	opts := setup.SetupOptions{
		Force:    forceSetup,
		Offline:  offlineMode,
		SkipPull: skipPull,
	}

	if hostInfo.IsWSL2 {
		return setup.WSL2Setup(Cfg)
	}

	if systemSetup {
		return setup.SystemSetup(Cfg)
	}

	return setup.UserSetupWithOptions(Cfg, opts)
}
