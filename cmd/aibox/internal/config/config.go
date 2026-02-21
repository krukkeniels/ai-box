package config

import (
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// ResolveHomeDir returns the home directory of the real (non-root) user.
// When running under sudo, os.UserHomeDir() returns /root, which won't
// contain the user's config. This function checks SUDO_USER and resolves
// the invoking user's home directory instead.
func ResolveHomeDir() (string, error) {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		u, err := user.Lookup(sudoUser)
		if err != nil {
			slog.Debug("SUDO_USER lookup failed, falling back", "sudo_user", sudoUser, "error", err)
		} else {
			slog.Debug("resolved home via SUDO_USER", "user", sudoUser, "home", u.HomeDir)
			return u.HomeDir, nil
		}
	}
	return os.UserHomeDir()
}

// Config is the top-level configuration for aibox.
type Config struct {
	Runtime     string            `yaml:"runtime" mapstructure:"runtime"`
	Image       string            `yaml:"image" mapstructure:"image"`
	GVisor      GVisorConfig      `yaml:"gvisor" mapstructure:"gvisor"`
	Resources   ResourceConfig    `yaml:"resources" mapstructure:"resources"`
	Workspace   WorkspaceConfig   `yaml:"workspace" mapstructure:"workspace"`
	Registry    RegistryConfig    `yaml:"registry" mapstructure:"registry"`
	Network     NetworkConfig     `yaml:"network" mapstructure:"network"`
	Policy      PolicyConfig      `yaml:"policy" mapstructure:"policy"`
	Credentials CredentialsConfig `yaml:"credentials" mapstructure:"credentials"`
	Logging     LoggingConfig     `yaml:"logging" mapstructure:"logging"`
}

// GVisorConfig holds gVisor sandbox settings.
type GVisorConfig struct {
	Enabled         bool   `yaml:"enabled" mapstructure:"enabled"`
	Platform        string `yaml:"platform" mapstructure:"platform"`                 // systrap or ptrace
	RequireAppArmor bool   `yaml:"require_apparmor" mapstructure:"require_apparmor"` // if true, AppArmor failure is fatal (default false)
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

// PolicyConfig holds policy engine settings (Phase 3).
type PolicyConfig struct {
	OrgBaselinePath   string `yaml:"org_baseline_path" mapstructure:"org_baseline_path"`     // org baseline policy (default "/etc/aibox/org-policy.yaml")
	TeamPolicyPath    string `yaml:"team_policy_path" mapstructure:"team_policy_path"`       // team policy (optional)
	ProjectPolicyPath string `yaml:"project_policy_path" mapstructure:"project_policy_path"` // project policy (default "aibox/policy.yaml" relative to workspace)
	DecisionLogPath   string `yaml:"decision_log_path" mapstructure:"decision_log_path"`     // decision log (default "/var/log/aibox/decisions.jsonl")
	HotReloadSecs     int    `yaml:"hot_reload_secs" mapstructure:"hot_reload_secs"`         // policy reload interval in seconds (0=disabled)
}

// CredentialsConfig holds credential management settings (Phase 3).
type CredentialsConfig struct {
	Mode              string `yaml:"mode" mapstructure:"mode"`                               // "fallback" or "vault"
	VaultAddr         string `yaml:"vault_addr" mapstructure:"vault_addr"`                   // Vault server address
	SPIFFETrustDomain string `yaml:"spiffe_trust_domain" mapstructure:"spiffe_trust_domain"` // SPIFFE trust domain
	SPIFFESocketPath  string `yaml:"spiffe_socket_path" mapstructure:"spiffe_socket_path"`   // SPIRE agent socket
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
	v.SetDefault("gvisor.require_apparmor", false)
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
		"vault.internal",
	})
	v.SetDefault("network.llm_gateway", "foundry.internal")
	v.SetDefault("policy.org_baseline_path", "/etc/aibox/org-policy.yaml")
	v.SetDefault("policy.team_policy_path", "")
	v.SetDefault("policy.project_policy_path", "aibox/policy.yaml")
	v.SetDefault("policy.decision_log_path", "/var/log/aibox/decisions.jsonl")
	v.SetDefault("policy.hot_reload_secs", 0)
	v.SetDefault("credentials.mode", "fallback")
	v.SetDefault("credentials.vault_addr", "https://vault.internal:8200")
	v.SetDefault("credentials.spiffe_trust_domain", "aibox.org.internal")
	v.SetDefault("credentials.spiffe_socket_path", "/run/spire/sockets/agent.sock")
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
		"gvisor.require_apparmor":  "AIBOX_GVISOR_REQUIRE_APPARMOR",
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
		"policy.org_baseline_path":     "AIBOX_POLICY_ORG_BASELINE_PATH",
		"policy.team_policy_path":     "AIBOX_POLICY_TEAM_POLICY_PATH",
		"policy.project_policy_path":  "AIBOX_POLICY_PROJECT_POLICY_PATH",
		"policy.decision_log_path":    "AIBOX_POLICY_DECISION_LOG_PATH",
		"policy.hot_reload_secs":      "AIBOX_POLICY_HOT_RELOAD_SECS",
		"credentials.mode":            "AIBOX_CREDENTIALS_MODE",
		"credentials.vault_addr":      "AIBOX_CREDENTIALS_VAULT_ADDR",
		"credentials.spiffe_trust_domain": "AIBOX_CREDENTIALS_SPIFFE_TRUST_DOMAIN",
		"credentials.spiffe_socket_path":  "AIBOX_CREDENTIALS_SPIFFE_SOCKET_PATH",
		"logging.format":           "AIBOX_LOGGING_FORMAT",
		"logging.level":            "AIBOX_LOGGING_LEVEL",
	}
	for key, env := range bindings {
		_ = v.BindEnv(key, env)
	}
}

// DefaultConfigDir returns the default configuration directory path.
func DefaultConfigDir() (string, error) {
	home, err := ResolveHomeDir()
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
		home, err := ResolveHomeDir()
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
  platform: systrap        # systrap (default, best perf) or ptrace (broader compat)
  require_apparmor: false  # if true, container start fails without AppArmor

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
    - vault.internal
  llm_gateway: foundry.internal

policy:
  org_baseline_path: /etc/aibox/org-policy.yaml
  # team_policy_path: ""                       # team policy file (optional)
  project_policy_path: aibox/policy.yaml       # relative to workspace
  decision_log_path: /var/log/aibox/decisions.jsonl
  hot_reload_secs: 0                           # 0 = disabled; set to 300 for 5-min refresh

credentials:
  mode: fallback                               # "fallback" (env var) or "vault"
  vault_addr: "https://vault.internal:8200"
  spiffe_trust_domain: aibox.org.internal
  spiffe_socket_path: /run/spire/sockets/agent.sock

logging:
  format: text         # text or json
  level: info
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}

	return path, nil
}
