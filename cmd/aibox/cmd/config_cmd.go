package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/aibox/aibox/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View and modify AI-Box configuration",
	Long: `Config provides subcommands for viewing and modifying the aibox
configuration file at ~/.config/aibox/config.yaml.

Examples:
  aibox config set dotfiles.repo https://github.com/user/dotfiles.git
  aibox config set shell zsh
  aibox config get shell
  aibox config get dotfiles.repo
  aibox config validate
  aibox config init --template minimal
  aibox config migrate --dry-run`,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set updates a configuration key in ~/.config/aibox/config.yaml.

Supported keys:
  dotfiles.repo    Git URL for dotfiles repository
  shell            Default shell: bash, zsh, or pwsh
  ide.ssh_port     Host port for SSH (default 2222)
  resources.cpus   CPU limit
  resources.memory Memory limit (e.g. 8g)`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the current configuration",
	Long:  `Validate checks the current configuration for errors and warnings.`,
	RunE:  runConfigValidate,
}

var configMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate configuration to the latest version",
	Long: `Migrate updates the configuration file schema to the latest version.
A backup is created before any changes are made.`,
	RunE: runConfigMigrate,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize configuration from a template",
	Long: `Init creates a new configuration file from a template.

Available templates:
  minimal     Open-source / personal use (no gVisor, no network security)
  dev         Local development with security features enabled
  enterprise  Full enterprise security stack (Harbor, Vault, Falco)`,
	RunE: runConfigInit,
}

// Flags for config subcommands.
var (
	migrateDryRun    bool
	initTemplate     string
	initForce        bool
)

func init() {
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configValidateCmd)
	configCmd.AddCommand(configMigrateCmd)
	configCmd.AddCommand(configInitCmd)

	configMigrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "preview migration without writing changes")
	configInitCmd.Flags().StringVar(&initTemplate, "template", "minimal", "config template: minimal, dev, or enterprise")
	configInitCmd.Flags().BoolVar(&initForce, "force", false, "overwrite existing config file")

	rootCmd.AddCommand(configCmd)
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	// Validate known keys.
	validKeys := map[string]bool{
		"dotfiles.repo":    true,
		"shell":            true,
		"ide.ssh_port":     true,
		"resources.cpus":   true,
		"resources.memory": true,
		"runtime":          true,
		"image":            true,
	}

	if !validKeys[key] {
		return fmt.Errorf("unknown config key %q. Valid keys: %s", key, strings.Join(sortedKeys(validKeys), ", "))
	}

	// Validate shell value.
	if key == "shell" {
		switch value {
		case "bash", "zsh", "pwsh":
			// ok
		default:
			return fmt.Errorf("invalid shell %q: must be bash, zsh, or pwsh", value)
		}
	}

	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return fmt.Errorf("determining config path: %w", err)
	}

	// Ensure config file exists.
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if _, err := config.WriteDefault(cfgPath); err != nil {
			return fmt.Errorf("creating default config: %w", err)
		}
	}

	v := viper.New()
	v.SetConfigFile(cfgPath)
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	v.Set(key, value)

	if err := v.WriteConfig(); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Printf("Set %s = %s\n", key, value)
	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return fmt.Errorf("determining config path: %w", err)
	}

	v := viper.New()
	v.SetConfigFile(cfgPath)

	if err := v.ReadInConfig(); err != nil {
		// If no config file, show default.
		fmt.Println(getDefault(key))
		return nil
	}

	value := v.GetString(key)
	if value == "" {
		value = getDefault(key)
	}
	fmt.Println(value)
	return nil
}

func runConfigValidate(cmd *cobra.Command, args []string) error {
	cfg := Cfg
	if cfg == nil {
		// Load config if not already loaded (e.g., running standalone).
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
	}

	result := config.Validate(cfg)
	if !result.HasErrors() && !result.HasWarnings() {
		fmt.Println("Configuration is valid.")
		return nil
	}

	fmt.Println(result.String())

	if result.HasErrors() {
		return fmt.Errorf("configuration has %d error(s)", len(result.Errors))
	}
	return nil
}

func runConfigMigrate(cmd *cobra.Command, args []string) error {
	cfgPath := cfgFile
	if cfgPath == "" {
		var err error
		cfgPath, err = config.DefaultConfigPath()
		if err != nil {
			return fmt.Errorf("determining config path: %w", err)
		}
	}

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s", cfgPath)
	}

	migrated, from, to, err := config.MigrateConfigFile(cfgPath, migrateDryRun)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	if from == to {
		fmt.Printf("Config is already at version %d (current).\n", from)
		return nil
	}

	if migrateDryRun {
		fmt.Printf("Dry run: would migrate v%d -> v%d\n\n", from, to)
		fmt.Println(string(migrated))
		return nil
	}

	fmt.Printf("Migrated config from v%d to v%d.\n", from, to)
	fmt.Printf("Backup saved to %s.backup.v%d\n", cfgPath, from)
	return nil
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	// Validate template name.
	valid := false
	for _, t := range config.ValidTemplates {
		if initTemplate == t {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("unknown template %q: valid templates are %s", initTemplate, strings.Join(config.ValidTemplates, ", "))
	}

	cfgPath := cfgFile
	if cfgPath == "" {
		var err error
		cfgPath, err = config.DefaultConfigPath()
		if err != nil {
			return fmt.Errorf("determining config path: %w", err)
		}
	}

	if err := config.WriteTemplate(initTemplate, cfgPath, initForce); err != nil {
		return err
	}

	fmt.Printf("Created config from %q template at %s\n", initTemplate, cfgPath)
	return nil
}

func getDefault(key string) string {
	defaults := map[string]string{
		"dotfiles.repo":    "",
		"shell":            "bash",
		"ide.ssh_port":     "2222",
		"resources.cpus":   "4",
		"resources.memory": "8g",
		"runtime":          "podman",
		"image":            "ghcr.io/krukkeniels/aibox/base:24.04",
	}
	return defaults[key]
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple insertion sort for small maps.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
