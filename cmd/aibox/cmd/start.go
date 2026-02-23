package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/aibox/aibox/internal/config"
	"github.com/aibox/aibox/internal/container"
	"github.com/aibox/aibox/internal/credentials"
	"github.com/aibox/aibox/internal/dotfiles"
	"github.com/aibox/aibox/internal/mcppacks"
	"github.com/aibox/aibox/internal/policy"
	"github.com/aibox/aibox/internal/setup"
	"github.com/aibox/aibox/internal/toolpacks"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a sandboxed AI development environment",
	Long: `Start launches a new gVisor-sandboxed container with the specified
workspace mounted. The container provides a secure environment for
AI-assisted code generation and execution.

The workspace directory is bind-mounted into the container at /workspace.
All mandatory security controls (seccomp, AppArmor, capability drop,
read-only rootfs) are applied automatically.`,
	RunE: runStart,
}

func init() {
	startCmd.Flags().StringP("workspace", "w", "", "path to workspace directory (required)")
	startCmd.Flags().String("image", "", "container image to use (overrides config)")
	startCmd.Flags().Int("cpus", 0, "CPU limit (overrides config)")
	startCmd.Flags().String("memory", "", "memory limit, e.g. 8g (overrides config)")
	startCmd.Flags().String("toolpacks", "", "comma-separated tool packs to activate (e.g. bazel@7,node@20)")
	startCmd.Flags().String("profile", "", "resource profile: 'jetbrains' sets 4+ CPUs, 8+ GB RAM")

	_ = startCmd.MarkFlagRequired("workspace")

	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	workspace, _ := cmd.Flags().GetString("workspace")
	image, _ := cmd.Flags().GetString("image")
	cpus, _ := cmd.Flags().GetInt("cpus")
	memory, _ := cmd.Flags().GetString("memory")
	toolpacksFlag, _ := cmd.Flags().GetString("toolpacks")
	profile, _ := cmd.Flags().GetString("profile")

	// Validate requested tool packs upfront.
	var requestedPacks []struct{ name, version string }
	if toolpacksFlag != "" {
		packs := strings.Split(toolpacksFlag, ",")
		for _, pack := range packs {
			pack = strings.TrimSpace(pack)
			if pack == "" {
				continue
			}
			name, version := toolpacks.ParsePackRef(pack)
			if name == "" || version == "" {
				return fmt.Errorf("invalid tool pack format %q: expected name@version (e.g. bazel@7)", pack)
			}
			requestedPacks = append(requestedPacks, struct{ name, version string }{name, version})
		}
	}

	// Validate workspace early so the user gets a clear error before we
	// check for the container runtime.
	if _, err := container.ValidateWorkspace(workspace); err != nil {
		return err
	}

	// Credential injection (Phase 3).
	// Initialize the credential provider based on configuration mode and
	// inject credentials as environment variables into the container.
	var credEnvVars []string
	credProvider, provErr := credentials.NewKeychainProvider()
	if provErr != nil {
		slog.Warn("credential provider unavailable, starting without credentials", "error", provErr)
	} else {
		broker := credentials.NewBroker(credProvider)
		envVars, err := broker.InjectEnvVars(context.Background())
		if err != nil {
			slog.Warn("credential injection failed, starting without credentials", "error", err)
		} else {
			credEnvVars = envVars
		}
	}

	if len(credEnvVars) == 0 && provErr == nil {
		slog.Info("no optional credentials configured (git, llm, mirror) — this is normal for personal use")
	}

	// Policy engine (Phase 3).
	// Load the org/team/project policy hierarchy, validate, merge, and
	// log the container_start event to the decision audit log.
	var policyHash string
	effectivePolicy, policyHash, policyErr := loadAndMergePolicy(Cfg, workspace)
	if policyErr != nil {
		return fmt.Errorf("policy validation failed: %w", policyErr)
	}
	if effectivePolicy != nil {
		slog.Info("policy loaded", "hash", policyHash)
	}

	// Instantiate decision logger with user-writable path.
	logPath := resolveDecisionLogPath(Cfg)
	decisionLogger, dlErr := policy.NewDecisionLogger(policy.DecisionLogConfig{
		Path:          logPath,
		FlushInterval: 5 * time.Second,
	})
	if dlErr != nil {
		slog.Warn("decision logger unavailable, audit logging disabled", "error", dlErr)
	}

	// Log container_start event.
	if decisionLogger != nil {
		startEntry := policy.DecisionEntry{
			Timestamp: time.Now(),
			PolicyVer: policyHash,
			Action:    "container_start",
			User:      currentUser(),
			Workspace: workspace,
			SandboxID: container.ContainerName(workspace),
			Decision:  "allow",
			RiskClass: policy.RiskSafe,
			Rule:      "lifecycle",
			Reason:    fmt.Sprintf("container start: image=%s", resolveImage(Cfg, image)),
		}
		if err := decisionLogger.Log(startEntry); err != nil {
			slog.Warn("failed to log container_start event", "error", err)
		}
		if err := decisionLogger.Flush(); err != nil {
			slog.Warn("failed to flush decision log", "error", err)
		}
		if err := decisionLogger.Close(); err != nil {
			slog.Warn("failed to close decision log", "error", err)
		}
	}

	// Regenerate MCP config from enabled packs (Phase 4).
	if mcpHomeDir, mcpErr := config.ResolveHomeDir(); mcpErr == nil {
		mcpState, stateErr := mcppacks.LoadState(mcpHomeDir)
		if stateErr == nil && len(mcpState.Enabled) > 0 {
			if genErr := mcppacks.GenerateConfig(mcpHomeDir, mcpState.Enabled); genErr != nil {
				slog.Warn("failed to regenerate MCP config", "error", genErr)
			}
		}
	}

	// Read SSH public key for container injection (Phase 4).
	var sshPubKey []byte
	pubKey, err := setup.ReadPublicKey()
	if err != nil {
		slog.Warn("SSH key not found; run 'aibox setup' to generate one for IDE access", "error", err)
	} else {
		sshPubKey = pubKey
	}

	mgr, err := container.NewManager(Cfg)
	if err != nil {
		return err
	}

	if err := mgr.Start(container.StartOpts{
		Workspace:   workspace,
		Image:       image,
		CPUs:        cpus,
		Memory:      memory,
		CredEnvVars: credEnvVars,
		Profile:     profile,
		SSHPubKey:   sshPubKey,
		SSHPort:     Cfg.IDE.SSHPort,
	}); err != nil {
		return err
	}

	// Write/update SSH config for IDE access.
	if err := setup.WriteSSHConfig(Cfg.IDE.SSHPort); err != nil {
		slog.Warn("failed to update SSH config; manually configure your IDE", "error", err)
	}

	// Dotfiles sync (Phase 4).
	// Clone/update the user's dotfiles repo into the container's persistent
	// home volume, configure default shell, and source aibox-env.sh.
	if Cfg.Dotfiles.Repo != "" {
		statusInfo, statusErr := mgr.Status()
		if statusErr != nil {
			slog.Warn("could not get container status for dotfiles sync", "error", statusErr)
		} else {
			if err := dotfiles.Sync(dotfiles.SyncOpts{
				RuntimePath:   mgr.RuntimePath,
				ContainerName: statusInfo.Name,
				RepoURL:       Cfg.Dotfiles.Repo,
				Shell:         Cfg.Shell,
			}); err != nil {
				slog.Warn("dotfiles sync failed", "error", err)
			}
		}
	}

	// Install requested tool packs into the running container (Phase 4).
	if len(requestedPacks) > 0 {
		packsDir := toolpacks.DefaultPacksDir()
		registry := toolpacks.NewRegistry(packsDir, "/opt/toolpacks")

		statusInfo, statusErr := mgr.Status()
		if statusErr != nil {
			slog.Warn("could not get container status for tool pack install", "error", statusErr)
		} else {
			installer := toolpacks.NewInstaller(mgr.RuntimePath, statusInfo.Name, registry)
			for _, rp := range requestedPacks {
				if installErr := installer.Install(rp.name, rp.version); installErr != nil {
					slog.Warn("tool pack install failed", "pack", rp.name+"@"+rp.version, "error", installErr)
					fmt.Printf("WARNING: failed to install %s@%s: %v\n", rp.name, rp.version, installErr)
				}
			}
		}
	}

	return nil
}

// loadAndMergePolicy loads the three-level policy hierarchy, validates each
// level, and merges them. Returns the effective policy and its hash.
// Returns (nil, "", nil) if no org policy is configured or found.
func loadAndMergePolicy(cfg *config.Config, workspace string) (*policy.Policy, string, error) {
	orgPath := cfg.Policy.OrgBaselinePath
	if orgPath == "" {
		slog.Debug("no org policy path configured, skipping policy load")
		return nil, "", nil
	}

	// Check if the org policy file exists; if not, warn and continue.
	if _, err := os.Stat(orgPath); err != nil {
		slog.Warn("org policy not found, skipping policy enforcement", "path", orgPath)
		return nil, "", nil
	}

	teamPath := cfg.Policy.TeamPolicyPath

	// Resolve project policy relative to workspace.
	var projectPath string
	if cfg.Policy.ProjectPolicyPath != "" {
		candidate := filepath.Join(workspace, cfg.Policy.ProjectPolicyPath)
		if _, err := os.Stat(candidate); err == nil {
			projectPath = candidate
		}
	}

	org, team, project, err := policy.LoadPolicyHierarchy(orgPath, teamPath, projectPath)
	if err != nil {
		return nil, "", fmt.Errorf("loading policy files: %w", err)
	}

	// Validate each level.
	var validationErrs []policy.ValidationError
	if org != nil {
		validationErrs = append(validationErrs, policy.ValidatePolicy(org)...)
	}
	if team != nil {
		for _, e := range policy.ValidatePolicy(team) {
			e.Field = "team." + e.Field
			validationErrs = append(validationErrs, e)
		}
	}
	if project != nil {
		for _, e := range policy.ValidatePolicy(project) {
			e.Field = "project." + e.Field
			validationErrs = append(validationErrs, e)
		}
	}
	if len(validationErrs) > 0 {
		msgs := make([]string, len(validationErrs))
		for i, e := range validationErrs {
			msgs[i] = e.Error()
		}
		return nil, "", fmt.Errorf("policy schema errors:\n  %s", strings.Join(msgs, "\n  "))
	}

	// Merge with tighten-only semantics.
	effective, err := policy.MergePolicies(org, team, project)
	if err != nil {
		return nil, "", err
	}

	hash := policyDigest(effective)
	return effective, hash, nil
}

// policyDigest returns a short hex digest of the effective policy.
func policyDigest(p *policy.Policy) string {
	data, _ := json.Marshal(p)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:8])
}

// resolveDecisionLogPath returns a user-writable decision log path.
// Falls back to ~/.local/share/aibox/log/decisions.jsonl if the configured
// path's directory is not writable.
func resolveDecisionLogPath(cfg *config.Config) string {
	cfgPath := cfg.Policy.DecisionLogPath
	if cfgPath != "" {
		dir := filepath.Dir(cfgPath)
		if isWritable(dir) {
			return cfgPath
		}
	}

	// Fall back to user-writable path.
	home, err := config.ResolveHomeDir()
	if err != nil {
		return cfgPath
	}
	return filepath.Join(home, ".local", "share", "aibox", "log", "decisions.jsonl")
}

// isWritable checks if a directory exists and is writable by the current user.
func isWritable(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return false
	}
	// Try creating a temp file to check writability.
	tmp := filepath.Join(dir, ".aibox-write-test")
	f, err := os.Create(tmp)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(tmp)
	return true
}

// currentUser returns the current username for audit logging.
func currentUser() string {
	u, err := user.Current()
	if err != nil {
		return "unknown"
	}
	return u.Username
}

// resolveImage returns the effective container image.
func resolveImage(cfg *config.Config, override string) string {
	if override != "" {
		return override
	}
	return cfg.Image
}
