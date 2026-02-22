package cmd

import (
	"fmt"

	"github.com/aibox/aibox/internal/config"
	"github.com/aibox/aibox/internal/mcppacks"
	"github.com/aibox/aibox/internal/policy"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage MCP (Model Context Protocol) packs",
	Long:  `MCP provides subcommands for enabling, disabling, and listing MCP server packs.`,
}

var mcpEnableCmd = &cobra.Command{
	Use:   "enable <pack> [pack...]",
	Short: "Enable one or more MCP packs in the sandbox",
	Long: `Enable activates the specified MCP server packs inside the running
AI-Box sandbox. MCP packs provide tool integrations for AI agents.

Each pack's network and filesystem requirements are validated against the
current policy before enabling. Packs requiring network endpoints not in
the allowlist will be rejected with a clear message.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runMCPEnable,
}

var mcpListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available MCP packs and their status",
	Long:  `List shows all MCP packs that can be enabled in an AI-Box sandbox, with their current status.`,
	RunE:  runMCPList,
}

var mcpDisableCmd = &cobra.Command{
	Use:   "disable <pack> [pack...]",
	Short: "Disable one or more MCP packs",
	Long:  `Disable deactivates the specified MCP server packs and regenerates the MCP configuration.`,
	Args:  cobra.MinimumNArgs(1),
	RunE:  runMCPDisable,
}

func init() {
	mcpCmd.AddCommand(mcpEnableCmd)
	mcpCmd.AddCommand(mcpListCmd)
	mcpCmd.AddCommand(mcpDisableCmd)
	rootCmd.AddCommand(mcpCmd)
}

func runMCPEnable(cmd *cobra.Command, args []string) error {
	homeDir, err := config.ResolveHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}

	// Load current state.
	state, err := mcppacks.LoadState(homeDir)
	if err != nil {
		return fmt.Errorf("loading MCP state: %w", err)
	}

	// Load policy for validation (best-effort).
	var effectivePolicy *policy.Policy
	if Cfg != nil && Cfg.Policy.OrgBaselinePath != "" {
		p, loadErr := policy.LoadPolicy(Cfg.Policy.OrgBaselinePath)
		if loadErr == nil {
			effectivePolicy = p
		}
	}

	var enabled int
	for _, name := range args {
		pack := mcppacks.FindPack(name)
		if pack == nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: unknown MCP pack %q, skipping\n", name)
			continue
		}

		// Validate against policy.
		check := mcppacks.CheckPolicy(pack, effectivePolicy)
		if !check.Allowed {
			fmt.Fprintln(cmd.ErrOrStderr(), mcppacks.FormatDenied(name, check.DeniedEndpoints))
			continue
		}

		if mcppacks.IsEnabled(state.Enabled, name) {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s: already enabled\n", name)
			continue
		}

		state.Enabled = append(state.Enabled, name)
		fmt.Fprintf(cmd.OutOrStdout(), "  %s: enabled\n", name)
		enabled++
	}

	if enabled == 0 && len(state.Enabled) == 0 {
		return nil
	}

	// Save state and regenerate config.
	if err := mcppacks.SaveState(homeDir, state); err != nil {
		return fmt.Errorf("saving MCP state: %w", err)
	}
	if err := mcppacks.GenerateConfig(homeDir, state.Enabled); err != nil {
		return fmt.Errorf("generating MCP config: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nMCP config written. AI agents will discover enabled servers automatically.\n")
	return nil
}

func runMCPList(cmd *cobra.Command, args []string) error {
	homeDir, err := config.ResolveHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}

	state, err := mcppacks.LoadState(homeDir)
	if err != nil {
		return fmt.Errorf("loading MCP state: %w", err)
	}

	packs := mcppacks.BuiltinPacks()
	fmt.Fprintf(cmd.OutOrStdout(), "Available MCP packs:\n\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  %-20s %-10s %s\n", "NAME", "STATUS", "DESCRIPTION")
	fmt.Fprintf(cmd.OutOrStdout(), "  %-20s %-10s %s\n", "----", "------", "-----------")

	for _, p := range packs {
		status := "disabled"
		if mcppacks.IsEnabled(state.Enabled, p.Name) {
			status = "enabled"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  %-20s %-10s %s\n", p.Name, status, p.Description)
	}

	return nil
}

func runMCPDisable(cmd *cobra.Command, args []string) error {
	homeDir, err := config.ResolveHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}

	state, err := mcppacks.LoadState(homeDir)
	if err != nil {
		return fmt.Errorf("loading MCP state: %w", err)
	}

	for _, name := range args {
		if !mcppacks.IsEnabled(state.Enabled, name) {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s: not enabled\n", name)
			continue
		}
		state.Enabled = removePack(state.Enabled, name)
		fmt.Fprintf(cmd.OutOrStdout(), "  %s: disabled\n", name)
	}

	if err := mcppacks.SaveState(homeDir, state); err != nil {
		return fmt.Errorf("saving MCP state: %w", err)
	}
	if err := mcppacks.GenerateConfig(homeDir, state.Enabled); err != nil {
		return fmt.Errorf("generating MCP config: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nMCP config updated.\n")
	return nil
}

// removePack removes a pack name from a slice.
func removePack(packs []string, name string) []string {
	result := make([]string, 0, len(packs))
	for _, p := range packs {
		if p != name {
			result = append(result, p)
		}
	}
	return result
}
