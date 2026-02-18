package setup

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/aibox/aibox/internal/config"
)

// WSL2Setup extends the Linux setup with WSL2-specific steps.
// It runs the full Linux setup first, then applies WSL2 optimizations.
func WSL2Setup(cfg *config.Config) error {
	// Run the base Linux setup first.
	if err := LinuxSetup(cfg); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("WSL2-specific checks")
	fmt.Println(strings.Repeat("-", 40))

	steps := []Step{
		{"Check WSL2 kernel version", checkWSL2Kernel},
		{"Check WSL2 memory allocation", checkWSL2Memory},
		{"Check Podman machine (if applicable)", func() error { return checkPodmanMachine(cfg.Runtime) }},
	}

	for i, step := range steps {
		fmt.Printf("[%d/%d] %s ...\n", i+1, len(steps), step.Name)
		if err := step.Run(); err != nil {
			// WSL2 steps are advisory, not fatal.
			fmt.Printf("  WARN: %v\n", err)
		}
	}

	fmt.Println(strings.Repeat("-", 40))
	fmt.Println("WSL2 setup complete.")
	return nil
}

func checkWSL2Kernel() error {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return fmt.Errorf("cannot read kernel version: %w", err)
	}

	ver := strings.TrimSpace(string(data))
	parts := strings.Fields(ver)
	kernelVer := "unknown"
	if len(parts) >= 3 {
		kernelVer = parts[2]
	}

	// Parse major.minor.
	verParts := strings.SplitN(kernelVer, ".", 3)
	if len(verParts) >= 2 {
		major, _ := strconv.Atoi(verParts[0])
		minorStr := strings.SplitN(verParts[1], "-", 2)[0]
		minor, _ := strconv.Atoi(minorStr)

		if major < 5 || (major == 5 && minor < 15) {
			fmt.Printf("  WARN: Kernel %s may be too old for gVisor systrap (need 5.15+)\n", kernelVer)
			fmt.Println("  Update WSL2: wsl --update (from PowerShell)")
			return nil
		}
	}

	fmt.Printf("  Kernel: %s\n", kernelVer)
	return nil
}

func checkWSL2Memory() error {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return fmt.Errorf("cannot read memory info: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, _ := strconv.ParseInt(fields[1], 10, 64)
				gb := kb / (1024 * 1024)
				fmt.Printf("  Total memory: ~%d GB\n", gb)

				if gb < 12 {
					fmt.Printf("  WARN: Only %d GB allocated to WSL2.\n", gb)
					userprofile := "%USERPROFILE%"
				fmt.Printf("  Recommended: 16 GB. Update %s\\.wslconfig:\n", userprofile)
					fmt.Println("    [wsl2]")
					fmt.Println("    memory=16GB")
					fmt.Println("    processors=8")
					fmt.Println("    swap=4GB")
				}
				return nil
			}
		}
	}

	fmt.Println("  Could not determine memory allocation")
	return nil
}

func checkPodmanMachine(runtime string) error {
	if runtime != "podman" {
		fmt.Println("  Skipping (runtime is not podman)")
		return nil
	}

	// Check if podman machine is needed. In WSL2, podman runs natively
	// without a machine, but some setups use podman machine.
	rtPath, err := exec.LookPath(runtime)
	if err != nil {
		return fmt.Errorf("podman not found")
	}

	// Check if podman can run without a machine (native WSL2 install).
	if err := exec.Command(rtPath, "info").Run(); err == nil {
		fmt.Println("  Podman running natively in WSL2 (no machine needed)")
		return nil
	}

	// Check for podman machine.
	out, err := exec.Command(rtPath, "machine", "list", "--format", "{{.Name}} {{.Running}}").Output()
	if err != nil {
		fmt.Println("  Podman machine not available (native podman should work in WSL2)")
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			name := parts[0]
			running := parts[1]
			if running == "true" || running == "Currently" {
				fmt.Printf("  Podman machine %q is running\n", name)
				return nil
			}
		}
	}

	fmt.Println("  No running podman machine found.")
	fmt.Println("  If podman is installed natively in WSL2, this is fine.")
	fmt.Println("  Otherwise: podman machine init && podman machine start")
	return nil
}
