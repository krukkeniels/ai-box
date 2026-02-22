package cmd

import (
	"fmt"
	"strconv"

	"github.com/aibox/aibox/internal/container"
	"github.com/spf13/cobra"
)

var portForwardCmd = &cobra.Command{
	Use:   "port-forward <container-port> [host-port]",
	Short: "Forward a container port to the host",
	Long: `Port-forward creates an SSH tunnel from a host port to a container port,
enabling access to services running inside the sandbox (e.g., dev servers
on port 3000 or 4200 for hot-reload previews).

If host-port is not specified, it defaults to the same as container-port.

Examples:
  aibox port-forward 3000         # forward localhost:3000 -> container:3000
  aibox port-forward 4200 8080    # forward localhost:8080 -> container:4200

Note: gVisor blocks ptrace, so strace/GDB will not work inside the
container. Use IDE-integrated debug adapters (Node --inspect, JDWP,
debugpy) which use debug protocols instead of ptrace.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runPortForward,
}

func init() {
	rootCmd.AddCommand(portForwardCmd)
}

func runPortForward(cmd *cobra.Command, args []string) error {
	containerPort, err := strconv.Atoi(args[0])
	if err != nil || containerPort < 1 || containerPort > 65535 {
		return fmt.Errorf("invalid container port %q: must be 1-65535", args[0])
	}

	hostPort := containerPort
	if len(args) > 1 {
		hostPort, err = strconv.Atoi(args[1])
		if err != nil || hostPort < 1 || hostPort > 65535 {
			return fmt.Errorf("invalid host port %q: must be 1-65535", args[1])
		}
	}

	mgr, err := container.NewManager(Cfg)
	if err != nil {
		return err
	}

	return mgr.PortForward("", containerPort, hostPort)
}
