package cmd

import (
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/aibox/aibox/internal/config"
	"github.com/aibox/aibox/internal/logging"
	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

// Global flag values.
var (
	cfgFile   string
	verbose   bool
	logFormat string
)

// Cfg holds the loaded configuration, available to all subcommands.
var Cfg *config.Config

// SetVersionInfo is called from main to inject build-time version info.
func SetVersionInfo(v, c, d string) {
	version = v
	commit = c
	buildDate = d
	rootCmd.Version = v
	rootCmd.SetVersionTemplate(fmt.Sprintf("aibox version {{.Version}} (commit: %s, built: %s)\n", c, d))
}

var rootCmd = &cobra.Command{
	Use:   "aibox",
	Short: "AI-Box: secure, sandboxed AI development environments",
	Long: `AI-Box provides secure, gVisor-sandboxed container environments
for AI-assisted software development. It manages the full lifecycle
of sandbox containers including setup, start, stop, and health checks.`,
	Version: version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Set up logging first.
		logging.Setup(logFormat, verbose)

		// Load configuration.
		var err error
		Cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		// Auto-detect runtime if the configured one is not available.
		if _, lookErr := exec.LookPath(Cfg.Runtime); lookErr != nil {
			fallback := "docker"
			if Cfg.Runtime == "docker" {
				fallback = "podman"
			}
			if _, fbErr := exec.LookPath(fallback); fbErr == nil {
				slog.Info("configured runtime not found, falling back", "configured", Cfg.Runtime, "using", fallback)
				Cfg.Runtime = fallback
			}
		}

		return nil
	},
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.config/aibox/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose (debug) output")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text", "log output format (text or json)")

	rootCmd.SetVersionTemplate(fmt.Sprintf("aibox version {{.Version}} (commit: %s, built: %s)\n", commit, buildDate))
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
