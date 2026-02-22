package cmd

import (
	"fmt"

	"github.com/aibox/aibox/internal/toolpacks"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available resources",
	Long:  `List provides subcommands for listing tool packs, volumes, and other resources.`,
}

var listPacksCmd = &cobra.Command{
	Use:   "packs",
	Short: "List available and installed tool packs",
	Long: `List shows all tool packs that are available in the registry,
along with their installation status.`,
	RunE: runListPacks,
}

func init() {
	listPacksCmd.Flags().Bool("installed", false, "show only installed packs")
	listCmd.AddCommand(listPacksCmd)
	rootCmd.AddCommand(listCmd)
}

func runListPacks(cmd *cobra.Command, args []string) error {
	installedOnly, _ := cmd.Flags().GetBool("installed")

	packsDir := toolpacks.DefaultPacksDir()
	registry := toolpacks.NewRegistry(packsDir, "/opt/toolpacks")

	var packs []toolpacks.PackInfo
	var err error

	if installedOnly {
		packs, err = registry.InstalledPacks()
	} else {
		packs, err = registry.List()
	}
	if err != nil {
		return fmt.Errorf("listing packs: %w", err)
	}

	if len(packs) == 0 {
		if installedOnly {
			fmt.Println("No tool packs installed.")
		} else {
			fmt.Println("No tool packs found.")
			fmt.Printf("Pack directory: %s\n", packsDir)
		}
		return nil
	}

	fmt.Println("Tool Packs:")
	fmt.Printf("  %-20s %-10s %-12s %s\n", "NAME", "VERSION", "STATUS", "DESCRIPTION")
	for _, p := range packs {
		fmt.Printf("  %-20s %-10s %-12s %s\n",
			p.Manifest.Name,
			p.Manifest.Version,
			p.Status,
			p.Manifest.Description,
		)
	}

	return nil
}
