package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aibox/aibox/internal/config"
	"github.com/aibox/aibox/internal/container"
	"github.com/aibox/aibox/internal/host"
)

// RunIDEChecks executes IDE-specific diagnostic checks.
func RunIDEChecks(cfg *config.Config) *Report {
	hostInfo := host.Detect()
	report := &Report{}

	checks := []func() CheckResult{
		CheckSSHKeyExists,
		CheckSSHConfig,
		func() CheckResult { return CheckSSHPort(cfg.IDE.SSHPort) },
		func() CheckResult { return CheckSSHHandshake(cfg.IDE.SSHPort) },
		func() CheckResult { return CheckIDEEnvironment(hostInfo) },
	}

	for _, check := range checks {
		report.Results = append(report.Results, check())
	}
	return report
}

// CheckSSHKeyExists verifies the aibox SSH key pair exists.
func CheckSSHKeyExists() CheckResult {
	result := CheckResult{Name: "SSH Key"}
	home, err := config.ResolveHomeDir()
	if err != nil {
		result.Status = StatusFail
		result.Message = "cannot determine home directory"
		return result
	}

	privPath := filepath.Join(home, ".config", "aibox", "ssh", "aibox_ed25519")
	pubPath := privPath + ".pub"

	if _, err := os.Stat(privPath); err != nil {
		result.Status = StatusFail
		result.Message = "private key not found: " + privPath
		result.Remediation = "Generate SSH keys: aibox setup"
		return result
	}
	if _, err := os.Stat(pubPath); err != nil {
		result.Status = StatusFail
		result.Message = "public key not found: " + pubPath
		result.Remediation = "Generate SSH keys: aibox setup"
		return result
	}

	result.Status = StatusPass
	result.Message = "SSH key pair found"
	return result
}

// CheckSSHConfig verifies the aibox SSH config entry exists in ~/.ssh/config.
func CheckSSHConfig() CheckResult {
	result := CheckResult{Name: "SSH Config"}
	home, err := config.ResolveHomeDir()
	if err != nil {
		result.Status = StatusWarn
		result.Message = "cannot determine home directory"
		return result
	}

	sshConfig := filepath.Join(home, ".ssh", "config")
	data, err := os.ReadFile(sshConfig)
	if err != nil {
		result.Status = StatusWarn
		result.Message = "~/.ssh/config not found"
		result.Remediation = "Run 'aibox setup' to generate SSH config entry"
		return result
	}

	if !strings.Contains(string(data), "Host aibox") {
		result.Status = StatusWarn
		result.Message = "no 'Host aibox' entry in ~/.ssh/config"
		result.Remediation = "Run 'aibox setup' to add the SSH config entry"
		return result
	}

	result.Status = StatusPass
	result.Message = "SSH config entry 'Host aibox' found"
	return result
}

// CheckSSHPort verifies the SSH port is reachable via SSH banner probe.
func CheckSSHPort(port int) CheckResult {
	result := CheckResult{Name: "SSH Port"}
	if port == 0 {
		result.Status = StatusInfo
		result.Message = "SSH port not configured"
		return result
	}

	probe := container.ProbeSSH("localhost", port, 3*time.Second)
	if probe.Ready {
		result.Status = StatusPass
		result.Message = fmt.Sprintf("port %d reachable, banner: %s", port, probe.Banner)
	} else {
		result.Status = StatusFail
		result.Message = fmt.Sprintf("port %d: %s", port, probe.Error)
		result.Remediation = "Ensure a sandbox is running: aibox start --workspace <path>"
	}
	return result
}

// CheckSSHHandshake performs an SSH handshake test (banner + KEX).
func CheckSSHHandshake(port int) CheckResult {
	result := CheckResult{Name: "SSH Handshake"}
	if port == 0 {
		result.Status = StatusInfo
		result.Message = "SSH port not configured"
		return result
	}

	probe := container.ProbeSSH("localhost", port, 5*time.Second)
	if !probe.Ready {
		result.Status = StatusFail
		result.Message = "SSH handshake failed: " + probe.Error
		result.Remediation = "Check container SSH logs: aibox shell -- journalctl -u sshd\n" +
			"  Or restart the sandbox: aibox stop && aibox start --workspace <path>"
		return result
	}

	result.Status = StatusPass
	result.Message = "SSH handshake successful"
	return result
}

// CheckIDEEnvironment prints environment-specific IDE instructions.
func CheckIDEEnvironment(hostInfo host.HostInfo) CheckResult {
	result := CheckResult{Name: "IDE Environment"}
	if hostInfo.IsWSL2 {
		result.Status = StatusInfo
		result.Message = "WSL2 detected -- use 'code .' from WSL terminal, then Remote-SSH from the WSL VS Code window"
	} else {
		result.Status = StatusInfo
		result.Message = "Native Linux -- use VS Code Remote-SSH: 'Connect to Host...' -> aibox"
	}
	return result
}
