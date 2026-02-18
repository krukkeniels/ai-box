package container

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aibox/aibox/internal/config"
)

// StartOpts holds the options for starting a container.
type StartOpts struct {
	Workspace string
	Image     string
	CPUs      int
	Memory    string
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
}

// NewManager creates a container Manager for the configured runtime.
func NewManager(cfg *config.Config) (*Manager, error) {
	rtPath, err := exec.LookPath(cfg.Runtime)
	if err != nil {
		return nil, fmt.Errorf("%s not found in PATH: %w", cfg.Runtime, err)
	}
	return &Manager{
		RuntimePath: rtPath,
		RuntimeName: cfg.Runtime,
		Cfg:         cfg,
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
		// Pass platform to runsc via annotations.
		args = append(args, "--annotation", fmt.Sprintf("dev.gvisor.spec.platform=%s", platform))
	}

	// Security: mandatory flags (spec 9.4).
	args = append(args, "--cap-drop=ALL")
	args = append(args, "--security-opt=no-new-privileges:true")

	// Seccomp profile.
	seccompPath := m.findSeccompProfile()
	if seccompPath != "" {
		args = append(args, fmt.Sprintf("--security-opt=seccomp=%s", seccompPath))
	}

	// AppArmor profile (only if the profile is actually loaded on the host).
	if m.isAppArmorProfileLoaded() {
		args = append(args, "--security-opt=apparmor=aibox-sandbox")
	}

	// Read-only root filesystem.
	args = append(args, "--read-only")

	// Run as non-root user.
	args = append(args, "--user=1000:1000")

	// Networking: none for Phase 1.
	args = append(args, "--network=none")

	// Resource limits.
	args = append(args, fmt.Sprintf("--cpus=%s", strconv.Itoa(cpus)))
	args = append(args, fmt.Sprintf("--memory=%s", memory))

	// Filesystem mounts (spec 10.1).
	// Use --mount syntax for Docker compatibility (Docker's -v does not support nosuid,nodev).

	// /workspace: bind mount the host workspace directory.
	args = append(args, "--mount", fmt.Sprintf("type=bind,source=%s,target=/workspace", workspace))

	// /home/dev: named volume for user config persistence.
	args = append(args, "--mount", fmt.Sprintf("type=volume,source=aibox-home-%s,target=/home/dev", sanitize(currentUsername())))

	// /opt/toolpacks: named volume for runtime-installed tools.
	args = append(args, "--mount", "type=volume,source=aibox-toolpacks,target=/opt/toolpacks")

	// /tmp: tmpfs (ephemeral).
	args = append(args, "--tmpfs", fmt.Sprintf("/tmp:rw,noexec,nosuid,size=%s", tmpSize))

	// /var/tmp: tmpfs (ephemeral).
	args = append(args, "--tmpfs", "/var/tmp:rw,noexec,nosuid,size=1g")

	// Image and default command.
	// The container needs a long-running process to stay alive for shell access.
	// The real aibox image has an init/SSH server; for generic images, use sleep.
	args = append(args, image, "sleep", "infinity")

	slog.Debug("starting container", "name", name, "image", image, "runtime", m.RuntimeName)
	slog.Debug("container run args", "args", args)

	cmd := exec.Command(m.RuntimePath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to start container: %s\n%s", err, stderr.String())
	}

	containerID := strings.TrimSpace(string(out))
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
	fmt.Printf("  Network:   none (Phase 1)\n")
	fmt.Printf("\nRun 'aibox shell' to open a terminal in the sandbox.\n")

	return nil
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

// ensureImage pulls the image if it's not available locally.
func (m *Manager) ensureImage(image string) error {
	if err := m.runQuiet("image", "exists", image); err == nil {
		slog.Debug("image already cached", "image", image)
		return nil
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

// isAppArmorProfileLoaded checks if the aibox-sandbox AppArmor profile is
// loaded on the host. We only apply AppArmor if the profile is actually loaded,
// not just if AppArmor is available, to avoid container startup failures.
func (m *Manager) isAppArmorProfileLoaded() bool {
	data, err := os.ReadFile("/sys/kernel/security/apparmor/profiles")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "aibox-sandbox")
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

// runAttached runs a runtime command with stdout/stderr attached to the terminal.
func (m *Manager) runAttached(args ...string) error {
	cmd := exec.Command(m.RuntimePath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
