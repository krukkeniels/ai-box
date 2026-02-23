package support

import (
	"fmt"
	"strings"

	"github.com/aibox/aibox/internal/config"
	"github.com/aibox/aibox/internal/doctor"
	"github.com/aibox/aibox/internal/host"
)

// BundleOptions controls bundle generation.
type BundleOptions struct {
	Redact bool
}

// Bundle holds all diagnostic information for an issue report.
type Bundle struct {
	HostInfo       host.HostInfo `json:"host_info"`
	ConfigSummary  string        `json:"config_summary"`
	DoctorOutput   string        `json:"doctor_output"`
	SSHDiagnostics string        `json:"ssh_diagnostics"`
}

// RedactConfig returns a string summary of the config with sensitive values redacted.
func RedactConfig(cfg *config.Config) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("runtime: %s\n", cfg.Runtime))
	b.WriteString(fmt.Sprintf("image: %s\n", cfg.Image))
	b.WriteString(fmt.Sprintf("gvisor.enabled: %v\n", cfg.GVisor.Enabled))
	b.WriteString(fmt.Sprintf("network.enabled: %v\n", cfg.Network.Enabled))
	b.WriteString(fmt.Sprintf("credentials.mode: %s\n", cfg.Credentials.Mode))
	if cfg.Credentials.VaultAddr != "" {
		b.WriteString("credentials.vault_addr: ***\n")
	}
	b.WriteString(fmt.Sprintf("ide.ssh_port: %d\n", cfg.IDE.SSHPort))
	b.WriteString(fmt.Sprintf("audit.enabled: %v\n", cfg.Audit.Enabled))
	return b.String()
}

// GenerateBundle creates a diagnostic bundle.
func GenerateBundle(cfg *config.Config, opts BundleOptions) (*Bundle, error) {
	bundle := &Bundle{}
	bundle.HostInfo = host.Detect()

	if opts.Redact {
		bundle.ConfigSummary = RedactConfig(cfg)
	} else {
		bundle.ConfigSummary = fmt.Sprintf("%+v", cfg)
	}

	report := doctor.RunAll(cfg)
	jsonOut, _ := report.JSON()
	bundle.DoctorOutput = jsonOut

	ideReport := doctor.RunIDEChecks(cfg)
	ideJSON, _ := ideReport.JSON()
	bundle.SSHDiagnostics = ideJSON

	return bundle, nil
}
