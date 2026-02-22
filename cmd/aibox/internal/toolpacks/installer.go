package toolpacks

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Installer handles installing tool packs into a running container.
type Installer struct {
	RuntimePath string // path to podman/docker binary
	ContainerID string // running container name/ID
	Registry    *Registry
}

// NewInstaller creates an installer targeting a running container.
func NewInstaller(runtimePath, containerID string, registry *Registry) *Installer {
	return &Installer{
		RuntimePath: runtimePath,
		ContainerID: containerID,
		Registry:    registry,
	}
}

// Install installs a pack and its dependencies into the container.
func (inst *Installer) Install(name, version string) error {
	// Resolve the full dependency chain.
	packs, err := ResolveDependencies(inst.Registry, name, version)
	if err != nil {
		return fmt.Errorf("resolving dependencies: %w", err)
	}

	if len(packs) > 1 {
		fmt.Printf("Install order:\n%s", FormatDepTree(packs))
	}

	for _, p := range packs {
		if inst.Registry.IsInstalled(p.Manifest.Name, p.Manifest.Version) {
			fmt.Printf("  %s already installed, skipping\n", p.Manifest.Ref())
			continue
		}

		if err := inst.installPack(p); err != nil {
			return fmt.Errorf("installing %s: %w", p.Manifest.Ref(), err)
		}
	}

	return nil
}

// installPack installs a single pack into the container.
func (inst *Installer) installPack(info *PackInfo) error {
	m := info.Manifest
	fmt.Printf("Installing %s ...\n", m.Ref())

	// Validate manifest.
	if errs := ValidateManifest(m); len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return fmt.Errorf("invalid manifest:\n  %s", strings.Join(msgs, "\n  "))
	}

	switch m.Install.Method {
	case "script":
		if err := inst.installViaScript(info); err != nil {
			return err
		}
	case "docker-layer":
		// Docker-layer method is handled at image build time, not runtime.
		// At runtime, fall back to script install if available.
		scriptPath := filepath.Join(info.PackDir, "install.sh")
		if _, err := os.Stat(scriptPath); err == nil {
			info2 := *info
			info2.Manifest.Install.Script = "install.sh"
			if err := inst.installViaScript(&info2); err != nil {
				return err
			}
		} else {
			fmt.Printf("  %s uses docker-layer method (pre-built image); skipping runtime install\n", m.Ref())
		}
	default:
		return fmt.Errorf("unsupported install method: %s", m.Install.Method)
	}

	// Set environment variables via profile.d script.
	if len(m.Environment) > 0 {
		if err := inst.writeProfileScript(m); err != nil {
			slog.Warn("failed to write profile script", "pack", m.Ref(), "error", err)
		}
	}

	// Mark as installed.
	if err := inst.markInstalledInContainer(m.Name, m.Version); err != nil {
		slog.Warn("failed to mark pack as installed in container", "pack", m.Ref(), "error", err)
	}

	// Also mark locally so the registry reflects the change.
	if err := inst.Registry.MarkInstalled(m.Name, m.Version); err != nil {
		slog.Warn("failed to mark pack in local registry", "error", err)
	}

	// Run verification commands.
	if len(m.Verify) > 0 {
		fmt.Printf("  Verifying %s ...\n", m.Ref())
		for _, v := range m.Verify {
			if err := inst.execInContainer("sh", "-c", v.Command); err != nil {
				slog.Warn("verification failed", "command", v.Command, "error", err)
				fmt.Printf("  WARNING: verification command failed: %s\n", v.Command)
			}
		}
	}

	fmt.Printf("  %s installed successfully\n", m.Ref())
	return nil
}

// installViaScript copies the install script into the container and runs it.
func (inst *Installer) installViaScript(info *PackInfo) error {
	scriptName := info.Manifest.Install.Script
	if scriptName == "" {
		scriptName = "install.sh"
	}
	scriptPath := filepath.Join(info.PackDir, scriptName)

	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("install script not found: %s", scriptPath)
	}

	// Copy the script into the container.
	containerDest := fmt.Sprintf("/opt/toolpacks/%s/", info.Manifest.Name)

	// Create the directory in the container.
	if err := inst.execInContainer("mkdir", "-p", containerDest); err != nil {
		return fmt.Errorf("creating install dir in container: %w", err)
	}

	// Copy script via podman/docker cp.
	destPath := fmt.Sprintf("%s:%s%s", inst.ContainerID, containerDest, scriptName)
	cpCmd := exec.Command(inst.RuntimePath, "cp", scriptPath, destPath)
	if out, err := cpCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("copying install script: %w\n%s", err, string(out))
	}

	// Make it executable and run it.
	fullScript := containerDest + scriptName
	if err := inst.execInContainer("chmod", "+x", fullScript); err != nil {
		return fmt.Errorf("chmod install script: %w", err)
	}

	slog.Debug("running install script in container", "script", fullScript)
	if err := inst.execInContainerAttached(fullScript); err != nil {
		return fmt.Errorf("install script failed: %w", err)
	}

	return nil
}

// writeProfileScript writes a /etc/profile.d/ script to set environment
// variables and update PATH for the tool pack.
func (inst *Installer) writeProfileScript(m *Manifest) error {
	var lines []string
	lines = append(lines, fmt.Sprintf("# Tool pack: %s", m.Ref()))

	for k, v := range m.Environment {
		lines = append(lines, fmt.Sprintf("export %s=%q", k, v))
	}

	// Add pack bin directory to PATH if it exists.
	packBin := fmt.Sprintf("/opt/toolpacks/%s/bin", m.Name)
	lines = append(lines, fmt.Sprintf("[ -d %q ] && export PATH=%q:$PATH", packBin, packBin))

	content := strings.Join(lines, "\n") + "\n"
	scriptName := fmt.Sprintf("/etc/profile.d/toolpack-%s.sh", m.Name)

	// Write via exec echo since we don't have direct filesystem access.
	cmd := exec.Command(inst.RuntimePath, "exec", inst.ContainerID,
		"sh", "-c", fmt.Sprintf("cat > %s << 'AIBOX_EOF'\n%sAIBOX_EOF", scriptName, content))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("writing profile script: %w\n%s", err, string(out))
	}

	return nil
}

// markInstalledInContainer creates the .installed marker inside the container.
func (inst *Installer) markInstalledInContainer(name, version string) error {
	marker := fmt.Sprintf("/opt/toolpacks/%s/.installed", name)
	cmd := exec.Command(inst.RuntimePath, "exec", inst.ContainerID,
		"sh", "-c", fmt.Sprintf("echo %q > %s", version, marker))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("writing install marker: %w\n%s", err, string(out))
	}
	return nil
}

// execInContainer runs a command in the container silently.
func (inst *Installer) execInContainer(args ...string) error {
	cmdArgs := append([]string{"exec", inst.ContainerID}, args...)
	cmd := exec.Command(inst.RuntimePath, cmdArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", stderr.String(), err)
	}
	return nil
}

// execInContainerAttached runs a command in the container with output visible.
func (inst *Installer) execInContainerAttached(args ...string) error {
	cmdArgs := append([]string{"exec", inst.ContainerID}, args...)
	cmd := exec.Command(inst.RuntimePath, cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
