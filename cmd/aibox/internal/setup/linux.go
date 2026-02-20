package setup

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aibox/aibox/internal/config"
	"github.com/aibox/aibox/internal/network"
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

	// Phase 2: Network security steps.
	if cfg.Network.Enabled {
		steps = append(steps,
			Step{"Install nftables rules", func() error { return installNFTables(cfg) }},
			Step{"Configure Squid proxy", func() error { return configureSquid(cfg) }},
			Step{"Configure CoreDNS", func() error { return configureCoreDNS(cfg) }},
		)
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

// installNFTables generates and applies the nftables ruleset for container
// network isolation. Rules restrict container egress to the proxy and DNS only.
func installNFTables(cfg *config.Config) error {
	nftCfg := network.NFTablesConfig{
		ProxyIP:   cfg.Network.ProxyAddr,
		ProxyPort: cfg.Network.ProxyPort,
		DNSIP:     cfg.Network.DNSAddr,
		DNSPort:   cfg.Network.DNSPort,
	}
	mgr := network.NewNFTablesManager(nftCfg)

	// Write config to /etc/aibox/nftables.conf.
	configPath := "/etc/aibox/nftables.conf"
	if err := mgr.WriteConfig(configPath); err != nil {
		// Try with sudo.
		slog.Debug("direct write failed, trying sudo", "error", err)
		ruleset := mgr.GenerateRuleset()
		tmpFile := "/tmp/aibox-nftables.conf"
		if err := os.WriteFile(tmpFile, []byte(ruleset), 0o644); err != nil {
			return fmt.Errorf("writing temp nftables config: %w", err)
		}
		if err := exec.Command("sudo", "cp", tmpFile, configPath).Run(); err != nil {
			return fmt.Errorf("installing nftables config: %w (try running with sudo)", err)
		}
		os.Remove(tmpFile)
	}
	fmt.Printf("  nftables config written to %s\n", configPath)

	// Apply the ruleset.
	if err := mgr.Apply(); err != nil {
		// Try with sudo.
		slog.Debug("direct apply failed, trying sudo", "error", err)
		if err := exec.Command("sudo", "nft", "-f", configPath).Run(); err != nil {
			fmt.Printf("  WARN: could not apply nftables rules: %v\n", err)
			fmt.Println("  Apply manually: sudo nft -f /etc/aibox/nftables.conf")
			return nil
		}
	}
	fmt.Println("  nftables rules applied")
	return nil
}

// configureSquid generates the Squid proxy configuration and starts the service.
func configureSquid(cfg *config.Config) error {
	squidCfg := network.SquidConfig{
		ListenAddr:     cfg.Network.ProxyAddr,
		ListenPort:     cfg.Network.ProxyPort,
		AllowedDomains: cfg.Network.AllowedDomains,
	}
	mgr := network.NewSquidManager(squidCfg)

	// Write config.
	configPath := "/etc/aibox/squid.conf"
	if err := mgr.WriteConfig(configPath); err != nil {
		slog.Debug("direct write failed, trying sudo", "error", err)
		config := mgr.GenerateConfig()
		tmpFile := "/tmp/aibox-squid.conf"
		if err := os.WriteFile(tmpFile, []byte(config), 0o644); err != nil {
			return fmt.Errorf("writing temp squid config: %w", err)
		}
		if err := exec.Command("sudo", "cp", tmpFile, configPath).Run(); err != nil {
			return fmt.Errorf("installing squid config: %w (try running with sudo)", err)
		}
		os.Remove(tmpFile)
	}
	fmt.Printf("  Squid config written to %s\n", configPath)

	// Check if Squid is installed.
	if _, err := exec.LookPath("squid"); err != nil {
		fmt.Println("  WARN: squid not found in PATH")
		fmt.Println("  Install it: sudo apt-get install -y squid")
		return nil
	}

	// Check if already running.
	if mgr.IsRunning() {
		fmt.Println("  Squid proxy already running, reloading config")
		if err := mgr.Reload(); err != nil {
			fmt.Printf("  WARN: reload failed: %v\n", err)
		}
		return nil
	}

	// Start Squid.
	if err := mgr.Start(); err != nil {
		fmt.Printf("  WARN: could not start Squid: %v\n", err)
		fmt.Println("  Start manually: sudo systemctl start squid")
	} else {
		fmt.Println("  Squid proxy started")
	}
	return nil
}

// configureCoreDNS generates the CoreDNS Corefile and starts the service.
func configureCoreDNS(cfg *config.Config) error {
	entries := network.DefaultDomainEntries()
	dnsCfg := network.CoreDNSConfig{
		ListenAddr: cfg.Network.DNSAddr,
		ListenPort: cfg.Network.DNSPort,
		Entries:    entries,
	}
	mgr := network.NewCoreDNSManager(dnsCfg)

	// Write Corefile.
	configPath := "/etc/aibox/Corefile"
	if err := mgr.WriteCorefile(configPath); err != nil {
		slog.Debug("direct write failed, trying sudo", "error", err)
		corefile := mgr.GenerateCorefile()
		tmpFile := "/tmp/aibox-Corefile"
		if err := os.WriteFile(tmpFile, []byte(corefile), 0o644); err != nil {
			return fmt.Errorf("writing temp Corefile: %w", err)
		}
		if err := exec.Command("sudo", "cp", tmpFile, configPath).Run(); err != nil {
			return fmt.Errorf("installing Corefile: %w (try running with sudo)", err)
		}
		os.Remove(tmpFile)
	}
	fmt.Printf("  CoreDNS Corefile written to %s\n", configPath)

	// Check if CoreDNS is installed.
	if _, err := exec.LookPath("coredns"); err != nil {
		fmt.Println("  WARN: coredns not found in PATH")
		fmt.Println("  Install it: see https://coredns.io/manual/toc/#installation")
		return nil
	}

	// Check if already running.
	if mgr.IsRunning() {
		fmt.Println("  CoreDNS already running")
		return nil
	}

	// Start CoreDNS.
	if err := mgr.Start(); err != nil {
		fmt.Printf("  WARN: could not start CoreDNS: %v\n", err)
		fmt.Println("  Start manually: sudo coredns -conf /etc/aibox/Corefile &")
	} else {
		fmt.Println("  CoreDNS started")
	}
	return nil
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
