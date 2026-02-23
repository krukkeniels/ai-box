package setup

import (
	"fmt"
	"strings"

	"github.com/aibox/aibox/internal/host"
)

// WSLDevProfileSteps returns the setup steps for the wsl-dev profile.
// This profile is optimized for WSL2 development and skips steps that
// don't apply on WSL (like AppArmor).
func WSLDevProfileSteps(hostInfo host.HostInfo) []Step {
	steps := []Step{
		{"Detect environment", func() error {
			if hostInfo.IsWSL2 {
				fmt.Println("  WSL2 detected (kernel: " + hostInfo.KernelVersion + ")")
			} else {
				fmt.Println("  Native Linux detected")
			}
			return nil
		}},
		{"Check Podman rootless", func() error {
			return checkRuntime("podman")
		}},
		{"Create configuration (minimal template)", func() error {
			return createDefaultConfig()
		}},
		{"Generate SSH keys", func() error {
			return GenerateSSHKeyPair()
		}},
		{"Write SSH config entry", func() error {
			return WriteSSHConfig(2222)
		}},
	}

	if hostInfo.IsWSL2 {
		steps = append(steps, Step{
			"Print WSL-specific instructions", func() error {
				fmt.Println()
				fmt.Println("  VS Code on WSL2:")
				fmt.Println("    1. Open a WSL terminal and run: code .")
				fmt.Println("    2. VS Code opens with the WSL extension")
				fmt.Println("    3. Use Remote-SSH from this VS Code window (NOT from Windows)")
				fmt.Println("    4. Connect to host 'aibox' for sandbox access")
				fmt.Println()
				fmt.Println("  Note: AppArmor warnings in 'aibox doctor' are expected on WSL2.")
				return nil
			},
		})
	}

	return steps
}

// RunProfile runs a named setup profile.
func RunProfile(name string) error {
	hostInfo := host.Detect()

	var steps []Step
	switch name {
	case "wsl-dev":
		steps = WSLDevProfileSteps(hostInfo)
	default:
		return fmt.Errorf("unknown profile: %q (available: wsl-dev)", name)
	}

	fmt.Printf("AI-Box setup (profile: %s)\n", name)
	fmt.Println(strings.Repeat("=", 50))

	for i, step := range steps {
		fmt.Printf("\n[%d/%d] %s...\n", i+1, len(steps), step.Name)
		if err := step.Run(); err != nil {
			return fmt.Errorf("step %q failed: %w", step.Name, err)
		}
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println("Setup complete!")
	fmt.Println()
	fmt.Println("  Next steps:")
	fmt.Println("    aibox doctor       # health check")
	fmt.Println("    aibox start -w .   # launch a sandbox")
	return nil
}
