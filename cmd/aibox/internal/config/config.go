package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config is the top-level configuration for aibox.
type Config struct {
	Runtime   string          `yaml:"runtime" mapstructure:"runtime"`
	Image     string          `yaml:"image" mapstructure:"image"`
	GVisor    GVisorConfig    `yaml:"gvisor" mapstructure:"gvisor"`
	Resources ResourceConfig  `yaml:"resources" mapstructure:"resources"`
	Workspace WorkspaceConfig `yaml:"workspace" mapstructure:"workspace"`
	Registry  RegistryConfig  `yaml:"registry" mapstructure:"registry"`
	Network   NetworkConfig   `yaml:"network" mapstructure:"network"`
	Logging   LoggingConfig   `yaml:"logging" mapstructure:"logging"`
}

// GVisorConfig holds gVisor sandbox settings.
type GVisorConfig struct {
	Enabled  bool   `yaml:"enabled" mapstructure:"enabled"`
	Platform string `yaml:"platform" mapstructure:"platform"` // systrap or ptrace
}

// ResourceConfig holds container resource limits.
type ResourceConfig struct {
	CPUs    int    `yaml:"cpus" mapstructure:"cpus"`
	Memory  string `yaml:"memory" mapstructure:"memory"`
	TmpSize string `yaml:"tmp_size" mapstructure:"tmp_size"`
}

// WorkspaceConfig holds workspace mount settings.
type WorkspaceConfig struct {
	DefaultPath string `yaml:"default_path" mapstructure:"default_path"`
	ValidateFS  bool   `yaml:"validate_fs" mapstructure:"validate_fs"`
}

// RegistryConfig holds container registry settings.
type RegistryConfig struct {
	URL              string `yaml:"url" mapstructure:"url"`
	VerifySignatures bool   `yaml:"verify_signatures" mapstructure:"verify_signatures"`
}

// NetworkConfig holds network security settings (Phase 2).
type NetworkConfig struct {
	Enabled        bool     `yaml:"enabled" mapstructure:"enabled"`
	ProxyAddr      string   `yaml:"proxy_addr" mapstructure:"proxy_addr"`           // Squid proxy address (default "127.0.0.1")
	ProxyPort      int      `yaml:"proxy_port" mapstructure:"proxy_port"`           // Squid proxy port (default 3128)
	DNSAddr        string   `yaml:"dns_addr" mapstructure:"dns_addr"`               // CoreDNS address (default "127.0.0.1")
	DNSPort        int      `yaml:"dns_port" mapstructure:"dns_port"`               // CoreDNS port (default 53)
	AllowedDomains []string `yaml:"allowed_domains" mapstructure:"allowed_domains"` // domains allowed through proxy
	LLMGateway     string   `yaml:"llm_gateway" mapstructure:"llm_gateway"`         // LLM API gateway (default "foundry.internal")
}

// LoggingConfig holds logging preferences.
type LoggingConfig struct {
	Format string `yaml:"format" mapstructure:"format"` // text or json
	Level  string `yaml:"level" mapstructure:"level"`
}

// setDefaults registers sensible default values matching the spec.
func setDefaults(v *viper.Viper) {
	v.SetDefault("runtime", "podman")
	v.SetDefault("image", "harbor.internal/aibox/base:24.04")
	v.SetDefault("gvisor.enabled", true)
	v.SetDefault("gvisor.platform", "systrap")
	v.SetDefault("resources.cpus", 4)
	v.SetDefault("resources.memory", "8g")
	v.SetDefault("resources.tmp_size", "2g")
	v.SetDefault("workspace.default_path", ".")
	v.SetDefault("workspace.validate_fs", true)
	v.SetDefault("registry.url", "harbor.internal")
	v.SetDefault("registry.verify_signatures", true)
	v.SetDefault("network.enabled", true)
	v.SetDefault("network.proxy_addr", "127.0.0.1")
	v.SetDefault("network.proxy_port", 3128)
	v.SetDefault("network.dns_addr", "127.0.0.1")
	v.SetDefault("network.dns_port", 53)
	v.SetDefault("network.allowed_domains", []string{
		"harbor.internal",
		"nexus.internal",
		"foundry.internal",
		"git.internal",
	})
	v.SetDefault("network.llm_gateway", "foundry.internal")
	v.SetDefault("logging.format", "text")
	v.SetDefault("logging.level", "info")
}

// bindEnvVars binds environment variable overrides with AIBOX_ prefix.
// Viper's AutomaticEnv only works for top-level keys by default, so we
// explicitly bind nested keys to their AIBOX_ equivalents.
func bindEnvVars(v *viper.Viper) {
	bindings := map[string]string{
		"runtime":                  "AIBOX_RUNTIME",
		"image":                    "AIBOX_IMAGE",
		"gvisor.enabled":           "AIBOX_GVISOR_ENABLED",
		"gvisor.platform":          "AIBOX_GVISOR_PLATFORM",
		"resources.cpus":           "AIBOX_RESOURCES_CPUS",
		"resources.memory":         "AIBOX_RESOURCES_MEMORY",
		"resources.tmp_size":       "AIBOX_RESOURCES_TMP_SIZE",
		"workspace.default_path":   "AIBOX_WORKSPACE_DEFAULT_PATH",
		"workspace.validate_fs":    "AIBOX_WORKSPACE_VALIDATE_FS",
		"registry.url":             "AIBOX_REGISTRY_URL",
		"registry.verify_signatures": "AIBOX_REGISTRY_VERIFY_SIGNATURES",
		"network.enabled":          "AIBOX_NETWORK_ENABLED",
		"network.proxy_addr":       "AIBOX_NETWORK_PROXY_ADDR",
		"network.proxy_port":       "AIBOX_NETWORK_PROXY_PORT",
		"network.dns_addr":         "AIBOX_NETWORK_DNS_ADDR",
		"network.dns_port":         "AIBOX_NETWORK_DNS_PORT",
		"network.llm_gateway":      "AIBOX_NETWORK_LLM_GATEWAY",
		"logging.format":           "AIBOX_LOGGING_FORMAT",
		"logging.level":            "AIBOX_LOGGING_LEVEL",
	}
	for key, env := range bindings {
		_ = v.BindEnv(key, env)
	}
}

// DefaultConfigDir returns the default configuration directory path.
func DefaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "aibox"), nil
}

// DefaultConfigPath returns the default configuration file path.
func DefaultConfigPath() (string, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Load reads the aibox configuration from disk, env vars, and defaults.
// If configPath is empty, it looks in ~/.config/aibox/config.yaml.
func Load(configPath string) (*Config, error) {
	v := viper.New()
	setDefaults(v)
	bindEnvVars(v)

	// Also support AIBOX_ prefix through AutomaticEnv for top-level keys.
	v.SetEnvPrefix("AIBOX")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			slog.Warn("could not determine home directory", "error", err)
		} else {
			cfgDir := filepath.Join(home, ".config", "aibox")
			v.AddConfigPath(cfgDir)
			v.SetConfigName("config")
			v.SetConfigType("yaml")
		}
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// If a config file was explicitly requested, treat missing file as an error.
			if configPath != "" {
				return nil, err
			}
			slog.Debug("no config file found, using defaults", "error", err)
		}
	} else {
		slog.Debug("loaded config file", "path", v.ConfigFileUsed())
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// WriteDefault creates a default config file at the given path (or the
// default location if path is empty). It does not overwrite an existing file.
func WriteDefault(path string) (string, error) {
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return "", err
		}
	}

	// Do not overwrite.
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	content := `# AI-Box configuration
# See: aibox --help

runtime: podman
image: harbor.internal/aibox/base:24.04

gvisor:
  enabled: true
  platform: systrap   # systrap (default, best perf) or ptrace (broader compat)

resources:
  cpus: 4
  memory: 8g
  tmp_size: 2g

workspace:
  default_path: "."
  validate_fs: true    # block NTFS-mounted workspaces on WSL2

registry:
  url: harbor.internal
  verify_signatures: true

network:
  enabled: true
  proxy_addr: "127.0.0.1"   # Squid proxy listen address
  proxy_port: 3128           # Squid proxy port
  dns_addr: "127.0.0.1"     # CoreDNS listen address
  dns_port: 53               # CoreDNS port
  allowed_domains:           # domains containers can reach
    - harbor.internal
    - nexus.internal
    - foundry.internal
    - git.internal
  llm_gateway: foundry.internal

logging:
  format: text         # text or json
  level: info
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}

	return path, nil
}
