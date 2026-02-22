package cmd

import (
	"fmt"
	"strings"

	"github.com/aibox/aibox/internal/container"
	"github.com/aibox/aibox/internal/toolpacks"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install <pack>@<version>",
	Short: "Install a tool pack into the running sandbox",
	Long: `Install adds a tool pack (e.g. java@21, node@20) into the running
AI-Box sandbox container. Tool packs provide additional build tools
and language runtimes beyond what is included in the base image.

Dependencies are resolved and installed automatically. For example,
installing angular@18 will also install node@20 if not present.`,
	Args: cobra.ExactArgs(1),
	RunE: runInstall,
}

func init() {
	installCmd.Flags().Bool("dry-run", false, "show what would be installed without installing")
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	pack := args[0]
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	// Parse pack reference.
	name, version := toolpacks.ParsePackRef(pack)
	if name == "" || version == "" {
		return fmt.Errorf("invalid tool pack format %q: expected name@version (e.g. java@21)", pack)
	}

	// Set up registry.
	packsDir := toolpacks.DefaultPacksDir()
	registry := toolpacks.NewRegistry(packsDir, "/opt/toolpacks")

	// Resolve the pack and dependencies.
	packs, err := toolpacks.ResolveDependencies(registry, name, version)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", pack, err)
	}

	// Validate manifests.
	for _, p := range packs {
		if errs := toolpacks.ValidateManifest(p.Manifest); len(errs) > 0 {
			msgs := make([]string, len(errs))
			for i, e := range errs {
				msgs[i] = e.Error()
			}
			return fmt.Errorf("invalid manifest for %s:\n  %s", p.Manifest.Ref(), strings.Join(msgs, "\n  "))
		}
	}

	if dryRun {
		fmt.Println("Would install:")
		fmt.Print(toolpacks.FormatDepTree(packs))
		return nil
	}

	// Validate a container is running.
	mgr, err := container.NewManager(Cfg)
	if err != nil {
		return err
	}

	info, err := mgr.Status()
	if err != nil {
		return err
	}

	if info.State != "running" {
		return fmt.Errorf("no running AI-Box container found; start one first with 'aibox start --workspace <path>'")
	}

	// Install via the installer.
	installer := toolpacks.NewInstaller(mgr.RuntimePath, info.Name, registry)
	return installer.Install(name, version)
}
