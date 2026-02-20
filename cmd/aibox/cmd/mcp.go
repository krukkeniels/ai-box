package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage MCP (Model Context Protocol) packs",
	Long:  `MCP provides subcommands for enabling and listing MCP server packs.`,
}

var mcpEnableCmd = &cobra.Command{
	Use:   "enable <pack> [pack...]",
	Short: "Enable one or more MCP packs in the sandbox",
	Long: `Enable activates the specified MCP server packs inside the running
AI-Box sandbox. MCP packs provide tool integrations for AI agents.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runMCPEnable,
}

var mcpListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available MCP packs",
	Long:  `List shows all MCP packs that can be enabled in an AI-Box sandbox.`,
	RunE:  runMCPList,
}

func init() {
	mcpCmd.AddCommand(mcpEnableCmd)
	mcpCmd.AddCommand(mcpListCmd)
	rootCmd.AddCommand(mcpCmd)
}

func runMCPEnable(cmd *cobra.Command, args []string) error {
	fmt.Println("Requested MCP packs:")
	for _, pack := range args {
		fmt.Printf("  - %s\n", pack)
	}
	fmt.Println("\nMCP pack management will be available in Phase 2.")
	fmt.Println("This requires the MCP sidecar infrastructure to be set up first.")

	return nil
}

// availableMCPPacks is the hardcoded list of known MCP packs for Phase 1.
var availableMCPPacks = []struct {
	Name        string
	Description string
}{
	{"filesystem-mcp", "Filesystem access tools for AI agents"},
	{"git-mcp", "Git operations and repository management"},
	{"jira-mcp", "Jira issue tracking integration"},
	{"docs-mcp", "Documentation search and retrieval"},
}

func runMCPList(cmd *cobra.Command, args []string) error {
	fmt.Println("Available MCP packs:")
	for _, pack := range availableMCPPacks {
		fmt.Printf("  %-20s %s\n", pack.Name, pack.Description)
	}

	return nil
}
