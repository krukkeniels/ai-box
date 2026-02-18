package setup

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aibox/aibox/internal/config"
	"github.com/aibox/aibox/internal/security"
)

// Step represents a named setup step with a run function.
type Step struct {
	Name string
	Run  func() error
}

// LinuxSetup performs the full setup flow on native Linux.
func LinuxSetup(cfg *config.Config) error {
	steps := []Step{
		{"Detect container runtime", func() error { return checkRuntime(cfg.Runtime) }},
		{"Check gVisor (runsc)", func() error { return checkGVisor(cfg) }},
		{"Install seccomp profile", func() error { return installSeccomp() }},
		{"Load AppArmor profile", func() error { return loadAppArmor() }},
		{"Create default configuration", func() error { return createDefaultConfig() }},
		{"Pull base image", func() error { return pullBaseImage(cfg) }},
	}

	fmt.Println("AI-Box setup (Linux)")
	fmt.Println(strings.Repeat("-", 40))

	for i, step := range steps {
		fmt.Printf("[%d/%d] %s ...\n", i+1, len(steps), step.Name)
		if err := step.Run(); err != nil {
			return fmt.Errorf("step %q failed: %w", step.Name, err)
		}
	}

	fmt.Println(strings.Repeat("-", 40))
	fmt.Println("Setup complete. Run 'aibox doctor' to verify.")
	return nil
}

func checkRuntime(runtime string) error {
	path, err := exec.LookPath(runtime)
	if err != nil {
		return fmt.Errorf("%s not found in PATH. Install it first:\n"+
			"  Ubuntu/Debian: sudo apt-get install -y podman\n"+
			"  Fedora: sudo dnf install -y podman\n"+
			"  See: https://podman.io/docs/installation", runtime)
	}

	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return fmt.Errorf("%s found at %s but version check failed: %w", runtime, path, err)
	}
	fmt.Printf("  Found %s\n", strings.TrimSpace(string(out)))
	return nil
}

func checkGVisor(cfg *config.Config) error {
	if !cfg.GVisor.Enabled {
		fmt.Println("  gVisor disabled in config, skipping")
		return nil
	}

	// Check for runsc binary.
	runscPath, err := exec.LookPath("runsc")
	if err != nil {
		// Check common paths.
		for _, p := range []string{"/usr/local/bin/runsc", "/usr/bin/runsc"} {
			if _, err := os.Stat(p); err == nil {
				runscPath = p
				break
			}
		}
	}

	if runscPath == "" {
		return fmt.Errorf("runsc (gVisor) not found. Install it:\n" +
			"  See: https://gvisor.dev/docs/user_guide/install/")
	}

	out, err := exec.Command(runscPath, "--version").Output()
	if err == nil {
		fmt.Printf("  Found runsc: %s\n", firstLine(strings.TrimSpace(string(out))))
	}

	// Try to register with podman.
	slog.Debug("checking if runsc is registered as OCI runtime")
	// runsc install registers itself; just check if it's accessible.
	fmt.Printf("  runsc path: %s\n", runscPath)
	return nil
}

func installSeccomp() error {
	targetPath := "/etc/aibox/seccomp.json"

	// If already installed, skip.
	if _, err := os.Stat(targetPath); err == nil {
		fmt.Printf("  Seccomp profile already at %s\n", targetPath)
		return nil
	}

	// Find source profile.
	sourcePath := findSeccompSource()
	if sourcePath == "" {
		fmt.Println("  WARN: seccomp profile source not found, skipping")
		fmt.Println("  You can install it manually later: sudo cp configs/seccomp.json /etc/aibox/seccomp.json")
		return nil
	}

	// Create target directory (needs root).
	fmt.Printf("  Installing seccomp profile to %s\n", targetPath)
	fmt.Println("  NOTE: This requires root privileges.")

	if err := os.MkdirAll("/etc/aibox", 0o755); err != nil {
		// Try with sudo.
		if err := exec.Command("sudo", "mkdir", "-p", "/etc/aibox").Run(); err != nil {
			return fmt.Errorf("cannot create /etc/aibox: %w (try running with sudo)", err)
		}
	}

	// Copy file.
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("reading seccomp source %s: %w", sourcePath, err)
	}

	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		// Try with sudo.
		cmd := exec.Command("sudo", "cp", sourcePath, targetPath)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("installing seccomp profile: %w (try running with sudo)", err)
		}
	}

	fmt.Printf("  Installed %s\n", targetPath)
	return nil
}

func loadAppArmor() error {
	if !security.IsAppArmorAvailable() {
		fmt.Println("  AppArmor not available on this system, skipping")
		fmt.Println("  Security will rely on gVisor + seccomp (still strong isolation)")
		return nil
	}

	// Check if already loaded.
	loaded, _ := security.IsProfileLoaded("aibox-sandbox")
	if loaded {
		fmt.Println("  aibox-sandbox profile already loaded")
		return nil
	}

	// Find the profile source.
	profilePath := findAppArmorSource()
	if profilePath == "" {
		fmt.Println("  WARN: AppArmor profile source not found, skipping")
		fmt.Println("  You can load it manually: sudo apparmor_parser -r configs/apparmor/aibox-sandbox")
		return nil
	}

	fmt.Printf("  Loading AppArmor profile from %s\n", profilePath)
	if err := security.LoadProfile(profilePath); err != nil {
		// May need sudo.
		cmd := exec.Command("sudo", "apparmor_parser", "-r", "-W", profilePath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("loading AppArmor profile: %w (try running with sudo)", err)
		}
	}

	fmt.Println("  aibox-sandbox profile loaded")
	return nil
}

func createDefaultConfig() error {
	path, err := config.WriteDefault("")
	if err != nil {
		return fmt.Errorf("creating default config: %w", err)
	}
	fmt.Printf("  Config at %s\n", path)
	return nil
}

func pullBaseImage(cfg *config.Config) error {
	rtPath, err := exec.LookPath(cfg.Runtime)
	if err != nil {
		return fmt.Errorf("%s not found: %w", cfg.Runtime, err)
	}

	// Check if image is already cached.
	if err := exec.Command(rtPath, "image", "exists", cfg.Image).Run(); err == nil {
		fmt.Printf("  Image %s already cached\n", cfg.Image)
		return nil
	}

	fmt.Printf("  Pulling %s ...\n", cfg.Image)
	cmd := exec.Command(rtPath, "pull", cfg.Image)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Image pull may fail if registry is unreachable (air-gapped, no network).
		// This is not fatal during setup -- the user can run aibox update later.
		fmt.Printf("  WARN: could not pull image: %v\n", err)
		fmt.Println("  You can pull it later with: aibox update")
		return nil
	}

	fmt.Printf("  Image %s cached\n", cfg.Image)
	return nil
}

func findSeccompSource() string {
	candidates := []string{
		"configs/seccomp.json",
	}

	// Relative to executable.
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "configs", "seccomp.json"),
			filepath.Join(filepath.Dir(dir), "configs", "seccomp.json"),
		)
	}

	// Relative to working directory.
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "configs", "seccomp.json"))
	}

	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

func findAppArmorSource() string {
	candidates := []string{
		"configs/apparmor/aibox-sandbox",
	}

	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "configs", "apparmor", "aibox-sandbox"),
			filepath.Join(filepath.Dir(dir), "configs", "apparmor", "aibox-sandbox"),
		)
	}

	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "configs", "apparmor", "aibox-sandbox"))
	}

	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
