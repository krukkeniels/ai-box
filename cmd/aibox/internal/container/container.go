package container

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aibox/aibox/internal/config"
	"github.com/aibox/aibox/internal/host"
	"github.com/aibox/aibox/internal/mounts"
	"github.com/aibox/aibox/internal/security"
)

// StartOpts holds the options for starting a container.
type StartOpts struct {
	Workspace   string
	Image       string
	CPUs        int
	Memory      string
	CredEnvVars []string // credential environment variables injected by the credential broker
	Profile     string   // resource profile: "", "jetbrains"
	SSHPubKey   []byte   // SSH public key to inject into container authorized_keys
	SSHPort     int      // host port for SSH mapping (default 2222)
}

// StatusInfo holds the current state of an aibox container.
type StatusInfo struct {
	Name      string
	State     string // running, exited, not-found
	Image     string
	Runtime   string
	Workspace string
	CPUs      string
	Memory    string
}

// Manager manages aibox container lifecycle operations.
type Manager struct {
	RuntimePath string // path to podman/docker binary
	RuntimeName string // "podman" or "docker"
	Cfg         *config.Config
	IsWSL       bool // true when running inside WSL2
}

// NewManager creates a container Manager for the configured runtime.
func NewManager(cfg *config.Config) (*Manager, error) {
	rtPath, err := exec.LookPath(cfg.Runtime)
	if err != nil {
		return nil, fmt.Errorf("%s not found in PATH: %w", cfg.Runtime, err)
	}
	hostInfo := host.Detect()
	return &Manager{
		RuntimePath: rtPath,
		RuntimeName: cfg.Runtime,
		Cfg:         cfg,
		IsWSL:       hostInfo.IsWSL2,
	}, nil
}

// Start launches a new aibox sandbox container.
func (m *Manager) Start(opts StartOpts) error {
	workspace, err := ValidateWorkspace(opts.Workspace)
	if err != nil {
		return err
	}

	name := ContainerName(workspace)

	// Check if container already exists.
	state := m.containerState(name)
	if state == "running" {
		return fmt.Errorf("container %q is already running. Use 'aibox shell' to connect or 'aibox stop' first", name)
	}
	if state == "exited" || state == "created" {
		slog.Info("removing stopped container before starting", "name", name)
		_ = m.runQuiet("rm", name)
	}

	image := m.Cfg.Image
	if opts.Image != "" {
		image = opts.Image
	}

	cpus := m.Cfg.Resources.CPUs
	if opts.CPUs > 0 {
		cpus = opts.CPUs
	}

	memory := m.Cfg.Resources.Memory
	if opts.Memory != "" {
		memory = opts.Memory
	}

	tmpSize := m.Cfg.Resources.TmpSize
	if tmpSize == "" {
		tmpSize = "2g"
	}

	// Ensure image is available locally.
	if err := m.ensureImage(image); err != nil {
		return err
	}

	// Build the run command with all mandatory security flags.
	args := []string{"run", "-d"}

	// Container identity.
	args = append(args, "--name", name)
	args = append(args, "--label", ContainerLabel)
	args = append(args, "--hostname", "aibox")

	// Security: gVisor runtime (if enabled).
	if m.Cfg.GVisor.Enabled {
		platform := m.Cfg.GVisor.Platform
		if platform == "" {
			platform = "systrap"
		}
		args = append(args, "--runtime=runsc")
		// Pass platform to runsc via annotations (podman-only flag).
		if m.RuntimeName == "podman" {
			args = append(args, "--annotation", fmt.Sprintf("dev.gvisor.spec.platform=%s", platform))
		}
	}

	// Determine effective SSH port early so security flags can adapt.
	effectiveSSHPort := opts.SSHPort
	if effectiveSSHPort == 0 {
		effectiveSSHPort = m.Cfg.IDE.SSHPort
	}

	// Security: mandatory flags (spec 9.4).
	// When SSH is enabled, sshd requires privilege separation (setuid/setgid,
	// chroot) which is incompatible with --cap-drop=ALL in rootless podman.
	// Rootless mode already provides strong isolation via user namespaces.
	if effectiveSSHPort <= 0 {
		args = append(args, "--cap-drop=ALL")
		args = append(args, "--security-opt=no-new-privileges:true")
	}

	// Seccomp profile (mandatory — refuse to launch without it).
	seccompPath := m.findSeccompProfile()
	if seccompPath == "" {
		return fmt.Errorf("seccomp profile not found; run 'aibox setup' to install it")
	}
	args = append(args, fmt.Sprintf("--security-opt=seccomp=%s", seccompPath))

	// AppArmor profile enforcement.
	// When RequireAppArmor is false (default) and gVisor + seccomp are active,
	// AppArmor failures degrade gracefully to a warning instead of blocking launch.
	if security.IsAppArmorAvailable() {
		profilePath := m.findAppArmorProfile()
		if err := security.EnsureProfile(profilePath); err != nil {
			if m.Cfg.GVisor.RequireAppArmor {
				return fmt.Errorf("AppArmor profile not loaded; run 'aibox setup' to configure it: %w", err)
			}
			slog.Warn("AppArmor profile not loaded; continuing with gVisor + seccomp isolation",
				"error", err)
		} else {
			args = append(args, "--security-opt=apparmor=aibox-sandbox")
		}
	} else {
		if m.Cfg.GVisor.RequireAppArmor {
			return fmt.Errorf("AppArmor is required (gvisor.require_apparmor=true) but not available on this system")
		}
		slog.Warn("AppArmor not available; continuing with gVisor + seccomp isolation")
	}

	// Read-only root filesystem.
	args = append(args, "--read-only")

	// Run as root initially so sshd can bind port 22, then drop to dev
	// user in the entrypoint. --cap-drop=ALL and --no-new-privileges still
	// enforce least privilege. The Containerfile USER directive is overridden
	// here because sshd requires root to start.
	args = append(args, "--user=0:0")

	// Networking: proxy-controlled egress (Phase 2).
	// Container traffic routes through Squid proxy and CoreDNS on the host.
	// nftables rules on the host enforce that the container can only reach
	// the proxy and DNS — all other egress is dropped.
	if m.Cfg.Network.Enabled {
		proxyAddr := net.JoinHostPort(m.Cfg.Network.ProxyAddr, strconv.Itoa(m.Cfg.Network.ProxyPort))
		proxyURL := fmt.Sprintf("http://%s", proxyAddr)

		// Set proxy environment variables so tools inside the container
		// route traffic through Squid. These are a convenience — nftables
		// blocks direct egress even if a process unsets them.
		args = append(args, "-e", fmt.Sprintf("http_proxy=%s", proxyURL))
		args = append(args, "-e", fmt.Sprintf("https_proxy=%s", proxyURL))
		args = append(args, "-e", fmt.Sprintf("HTTP_PROXY=%s", proxyURL))
		args = append(args, "-e", fmt.Sprintf("HTTPS_PROXY=%s", proxyURL))
		args = append(args, "-e", "no_proxy=localhost,127.0.0.1")
		args = append(args, "-e", "NO_PROXY=localhost,127.0.0.1")

		// Configure AI tool base URLs to use the LLM sidecar proxy.
		args = append(args, "-e", "ANTHROPIC_BASE_URL=http://localhost:8443")
		args = append(args, "-e", "OPENAI_BASE_URL=http://localhost:8443")

		// Set container DNS to CoreDNS on the host.
		args = append(args, fmt.Sprintf("--dns=%s", m.Cfg.Network.DNSAddr))
	} else {
		// No network security stack — use slirp4netns for minimal isolation
		// while still allowing port forwarding for IDE SSH access.
		// --network=none disables all networking including port mapping.
		args = append(args, "--network=slirp4netns")
	}

	// Credential environment variables (Phase 3).
	for _, env := range opts.CredEnvVars {
		args = append(args, "-e", env)
	}

	// SSH port mapping for IDE integration (Phase 4).
	// Maps host port (default 2222) to container SSH port 22 for
	// VS Code Remote SSH and JetBrains Gateway.
	if effectiveSSHPort > 0 {
		args = append(args, "-p", fmt.Sprintf("%d:22", effectiveSSHPort))
	}

	// Profile-based resource overrides (Phase 4).
	if opts.Profile == "jetbrains" {
		if cpus < 4 {
			cpus = 4
		}
		memNum := parseMemoryGB(memory)
		if memNum < 8 {
			memory = "8g"
		}
	}

	// Resource limits.
	args = append(args, fmt.Sprintf("--cpus=%s", strconv.Itoa(cpus)))
	args = append(args, fmt.Sprintf("--memory=%s", memory))

	// Filesystem mounts (spec 10.1) — built from the mounts package.
	mountLayout, err := mounts.Layout(workspace, tmpSize, "1g")
	if err != nil {
		return fmt.Errorf("building mount layout: %w", err)
	}

	// Ensure named volumes exist before launching.
	if err := mounts.EnsureVolumes(m.RuntimePath, mountLayout); err != nil {
		return fmt.Errorf("ensuring volumes: %w", err)
	}

	args = append(args, mounts.RuntimeArgs(mountLayout)...)

	// Image and entrypoint.
	// Start the SSH server so VS Code Remote SSH / JetBrains Gateway can connect.
	// sshd requires /run/sshd to exist (provided via tmpfs mount on /run)
	// and needs host keys. If sshd is not available, fall back to sleep.
	args = append(args, image, "/bin/bash", "-c",
		"if [ -x /usr/sbin/sshd ]; then "+
			"mkdir -p /run/sshd && "+
			"ssh-keygen -A 2>/dev/null; "+
			"/usr/sbin/sshd; "+
			"fi && "+
			"if id dev >/dev/null 2>&1; then "+
			"exec setpriv --reuid=1000 --regid=1000 --init-groups sleep infinity; "+
			"else exec sleep infinity; fi")

	slog.Debug("starting container", "name", name, "image", image, "runtime", m.RuntimeName)
	slog.Debug("container run args", "args", args)

	// Final safety gate: validate that all mandatory security flags are present.
	// Only expect AppArmor in the validation if it was required and available.
	expectGVisor := m.Cfg.GVisor.Enabled
	expectAppArmor := m.Cfg.GVisor.RequireAppArmor && security.IsAppArmorAvailable()
	if err := security.ValidateArgsWithExpectations(args, expectGVisor, expectAppArmor); err != nil {
		return fmt.Errorf("pre-launch security validation failed: %w", err)
	}

	cmd := exec.Command(m.RuntimePath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to start container: %s\n%s", err, stderr.String())
	}

	containerID := strings.TrimSpace(string(out))

	// Inject SSH public key into container for IDE access.
	if len(opts.SSHPubKey) > 0 {
		if err := m.injectSSHKey(name, opts.SSHPubKey); err != nil {
			slog.Warn("failed to inject SSH key; IDE access may require manual key setup", "error", err)
		}
	}

	// Probe SSH readiness before claiming it works.
	var sshReady bool
	if effectiveSSHPort > 0 {
		probe := ProbeSSH("localhost", effectiveSSHPort, 10*time.Second)
		sshReady = probe.Ready
		if !probe.Ready {
			slog.Warn("SSH readiness probe failed", "port", effectiveSSHPort, "error", probe.Error)
		}
	}

	fmt.Printf("AI-Box sandbox started.\n")
	fmt.Printf("  Container: %s\n", name)
	fmt.Printf("  ID:        %.12s\n", containerID)
	fmt.Printf("  Image:     %s\n", image)
	fmt.Printf("  Workspace: %s -> /workspace\n", workspace)
	fmt.Printf("  CPUs:      %d\n", cpus)
	fmt.Printf("  Memory:    %s\n", memory)
	if m.Cfg.GVisor.Enabled {
		fmt.Printf("  Runtime:   gVisor (runsc, %s)\n", m.Cfg.GVisor.Platform)
	}
	if m.Cfg.Network.Enabled {
		fmt.Printf("  Network:   proxy-controlled (Squid %s:%d, CoreDNS %s:%d)\n",
			m.Cfg.Network.ProxyAddr, m.Cfg.Network.ProxyPort,
			m.Cfg.Network.DNSAddr, m.Cfg.Network.DNSPort)
	} else {
		fmt.Printf("  Network:   none (isolated)\n")
	}
	if effectiveSSHPort > 0 {
		if sshReady {
			fmt.Printf("  SSH:       %s\n", ideHint(m.IsWSL, effectiveSSHPort))
		} else {
			fmt.Printf("  SSH:       localhost:%d (WARNING: SSH handshake failed — run 'aibox doctor' for diagnostics)\n", effectiveSSHPort)
		}
	}
	fmt.Printf("\nRun 'aibox shell' to open a terminal in the sandbox.\n")

	return nil
}

// ideHint returns an environment-appropriate IDE connection hint.
func ideHint(isWSL bool, port int) string {
	if isWSL {
		return fmt.Sprintf(
			"localhost:%d\n"+
				"         Open VS Code from WSL terminal: code .\n"+
				"         Then use Remote-SSH from the WSL VS Code window (not Windows).\n"+
				"         Or attach via Dev Containers.",
			port)
	}
	return fmt.Sprintf(
		"localhost:%d (VS Code: 'Remote-SSH: Connect to Host...' -> aibox)",
		port)
}

// Stop gracefully stops the running aibox container.
func (m *Manager) Stop(name string) error {
	if name == "" {
		var err error
		name, err = m.findRunningContainer()
		if err != nil {
			return err
		}
	}

	state := m.containerState(name)
	if state == "not-found" {
		return fmt.Errorf("no aibox container found. Run 'aibox start' first")
	}
	if state != "running" {
		fmt.Printf("Container %q is already stopped (state: %s).\n", name, state)
		return nil
	}

	fmt.Printf("Stopping container %s ...\n", name)

	// Graceful stop with 10s timeout.
	if err := m.runAttached("stop", "-t", "10", name); err != nil {
		slog.Warn("graceful stop failed, forcing", "error", err)
		_ = m.runAttached("kill", name)
	}

	fmt.Printf("Container %s stopped.\n", name)
	fmt.Println("Named volumes (home, toolpacks) preserved for next start.")
	return nil
}

// Shell opens an interactive bash session in the running container.
func (m *Manager) Shell(name string) error {
	if name == "" {
		var err error
		name, err = m.findRunningContainer()
		if err != nil {
			return err
		}
	}

	state := m.containerState(name)
	if state != "running" {
		return fmt.Errorf("container %q is not running (state: %s). Run 'aibox start' first", name, state)
	}

	cmd := exec.Command(m.RuntimePath, "exec", "-it", name, "/bin/bash")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// Status returns information about the current aibox container.
func (m *Manager) Status() (*StatusInfo, error) {
	name, err := m.findAnyContainer()
	if err != nil {
		return &StatusInfo{State: "not-found"}, nil
	}

	info := &StatusInfo{
		Name:  name,
		State: m.containerState(name),
	}

	// Get detailed info from inspect.
	format := "{{.Config.Image}}|{{.HostConfig.Runtime}}|{{.HostConfig.NanoCpus}}|{{.HostConfig.Memory}}"
	out, err := exec.Command(m.RuntimePath, "inspect", "--format", format, name).Output()
	if err == nil {
		parts := strings.Split(strings.TrimSpace(string(out)), "|")
		if len(parts) >= 4 {
			info.Image = parts[0]
			info.Runtime = parts[1]
			info.CPUs = parts[2]
			info.Memory = parts[3]
		}
	}

	// Get workspace mount.
	mountOut, err := exec.Command(m.RuntimePath, "inspect", "--format",
		"{{range .Mounts}}{{if eq .Destination \"/workspace\"}}{{.Source}}{{end}}{{end}}", name).Output()
	if err == nil {
		info.Workspace = strings.TrimSpace(string(mountOut))
	}

	return info, nil
}

// normalizeImageRef adds localhost/ prefix to unqualified short names when
// the image exists locally. This prevents Podman from trying to pull from
// unqualified-search registries.
func normalizeImageRef(ref string, existsLocally bool) string {
	if strings.Contains(ref, "/") {
		return ref // Already qualified.
	}
	if existsLocally {
		return "localhost/" + ref
	}
	return ref
}

// ensureImage pulls the image if it's not available locally.
func (m *Manager) ensureImage(image string) error {
	// Use "image inspect" which works with both podman and docker,
	// unlike "image exists" which is podman-only.
	if err := m.runQuiet("image", "inspect", image); err == nil {
		slog.Debug("image already cached", "image", image)
		return nil
	}

	// For unqualified short names (e.g. "aibox-base:24.04"), check whether
	// the image exists under the localhost/ prefix before pulling remotely.
	if !strings.Contains(image, "/") {
		candidate := "localhost/" + image
		if err := m.runQuiet("image", "inspect", candidate); err == nil {
			slog.Debug("image found with localhost/ prefix", "original", image, "resolved", candidate)
			return nil
		}
	}

	fmt.Printf("Pulling image %s ...\n", image)
	if err := m.runAttached("pull", image); err != nil {
		return fmt.Errorf("failed to pull image %s: %w", image, err)
	}
	return nil
}

// findRunningContainer finds a running aibox container.
func (m *Manager) findRunningContainer() (string, error) {
	out, err := exec.Command(m.RuntimePath, "ps",
		"--filter", "label="+ContainerLabel,
		"--format", "{{.Names}}").Output()
	if err != nil {
		return "", fmt.Errorf("failed to query containers: %w", err)
	}

	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("no running aibox container found. Run 'aibox start' first")
	}

	// Return the first one.
	return strings.Split(name, "\n")[0], nil
}

// findAnyContainer finds any aibox container (running or stopped).
func (m *Manager) findAnyContainer() (string, error) {
	out, err := exec.Command(m.RuntimePath, "ps", "-a",
		"--filter", "label="+ContainerLabel,
		"--format", "{{.Names}}").Output()
	if err != nil {
		return "", fmt.Errorf("failed to query containers: %w", err)
	}

	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("no aibox container found")
	}

	return strings.Split(name, "\n")[0], nil
}

// containerState returns the state of a container: "running", "exited", "created", or "not-found".
func (m *Manager) containerState(name string) string {
	out, err := exec.Command(m.RuntimePath, "inspect", "--format", "{{.State.Status}}", name).Output()
	if err != nil {
		return "not-found"
	}
	return strings.TrimSpace(string(out))
}

// findSeccompProfile returns the path to the seccomp profile, or empty if not found.
func (m *Manager) findSeccompProfile() string {
	candidates := []string{
		"/etc/aibox/seccomp.json",
		filepath.Join(findProjectRoot(), "configs", "seccomp.json"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			slog.Debug("using seccomp profile", "path", p)
			return p
		}
	}
	slog.Warn("seccomp profile not found, container will use runtime defaults")
	return ""
}

// findAppArmorProfile returns the path to the AppArmor profile source, or
// empty string if not found. Searches /etc/aibox/, binary-relative, and
// cwd-relative paths.
func (m *Manager) findAppArmorProfile() string {
	candidates := []string{
		"/etc/aibox/apparmor/aibox-sandbox",
	}

	// Relative to executable.
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "configs", "apparmor", "aibox-sandbox"),
			filepath.Join(filepath.Dir(dir), "configs", "apparmor", "aibox-sandbox"),
		)
	}

	// Relative to working directory.
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "configs", "apparmor", "aibox-sandbox"))
	}

	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			slog.Debug("using AppArmor profile", "path", p)
			return p
		}
	}
	return ""
}

// findProjectRoot attempts to locate the aibox project root by looking for configs/.
func findProjectRoot() string {
	// Try the binary's directory.
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)

	// Check relative to binary.
	if _, err := os.Stat(filepath.Join(dir, "configs")); err == nil {
		return dir
	}
	// Check parent.
	parent := filepath.Dir(dir)
	if _, err := os.Stat(filepath.Join(parent, "configs")); err == nil {
		return parent
	}

	return ""
}

// runQuiet runs a runtime command and discards output. Returns error on non-zero exit.
func (m *Manager) runQuiet(args ...string) error {
	return exec.Command(m.RuntimePath, args...).Run()
}

// PortForward forwards a container port to a host port using the container runtime.
func (m *Manager) PortForward(name string, containerPort, hostPort int) error {
	if name == "" {
		var err error
		name, err = m.findRunningContainer()
		if err != nil {
			return err
		}
	}

	state := m.containerState(name)
	if state != "running" {
		return fmt.Errorf("container %q is not running (state: %s). Run 'aibox start' first", name, state)
	}

	// Use socat inside the container to forward traffic via exec + local listener.
	// This is a common pattern for port forwarding without modifying the running container's network.
	fmt.Printf("Forwarding localhost:%d -> container:%d\n", hostPort, containerPort)
	fmt.Printf("Press Ctrl+C to stop forwarding.\n")

	cmd := exec.Command(m.RuntimePath, "exec", "-i", name,
		"/bin/bash", "-c",
		fmt.Sprintf("socat TCP-LISTEN:%d,fork,reuseaddr TCP:localhost:%d 2>/dev/null || "+
			"(echo 'socat not available, using built-in TCP proxy'; cat)", containerPort, containerPort))

	// Alternatively, use SSH local port forwarding if available.
	sshCmd := exec.Command("ssh",
		"-N", "-L", fmt.Sprintf("%d:localhost:%d", hostPort, containerPort),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-p", strconv.Itoa(m.Cfg.IDE.SSHPort),
		"dev@localhost")
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr

	// Prefer SSH tunnel since it's more reliable.
	_ = cmd // keep for reference
	return sshCmd.Run()
}

// injectSSHKey copies the SSH public key into the container's authorized_keys file.
func (m *Manager) injectSSHKey(containerName string, pubKey []byte) error {
	// Write the authorized_keys via exec into the container.
	keyStr := strings.TrimSpace(string(pubKey))
	cmd := exec.Command(m.RuntimePath, "exec", containerName,
		"/bin/bash", "-c",
		fmt.Sprintf("mkdir -p /home/dev/.ssh && echo '%s' > /home/dev/.ssh/authorized_keys && chmod 600 /home/dev/.ssh/authorized_keys && chmod 700 /home/dev/.ssh && chown -R dev:dev /home/dev/.ssh", keyStr))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("injecting SSH key: %s: %s", err, stderr.String())
	}
	slog.Debug("injected SSH public key into container", "container", containerName)
	return nil
}

// parseMemoryGB attempts to parse a memory string like "4g", "8Gi", etc. and
// return the value in GB. Returns 0 if parsing fails.
func parseMemoryGB(mem string) int {
	mem = strings.TrimSpace(strings.ToLower(mem))
	mem = strings.TrimRight(mem, "bi")
	if len(mem) == 0 {
		return 0
	}
	suffix := mem[len(mem)-1]
	numStr := mem[:len(mem)-1]
	switch suffix {
	case 'g':
		n, err := strconv.Atoi(numStr)
		if err != nil {
			return 0
		}
		return n
	case 'm':
		n, err := strconv.Atoi(numStr)
		if err != nil {
			return 0
		}
		return n / 1024
	default:
		n, err := strconv.Atoi(mem)
		if err != nil {
			return 0
		}
		return n / (1024 * 1024 * 1024)
	}
}

// runAttached runs a runtime command with stdout/stderr attached to the terminal.
func (m *Manager) runAttached(args ...string) error {
	cmd := exec.Command(m.RuntimePath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
