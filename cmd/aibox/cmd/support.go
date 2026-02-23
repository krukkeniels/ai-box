package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/aibox/aibox/internal/support"
	"github.com/spf13/cobra"
)

var supportCmd = &cobra.Command{
	Use:   "support",
	Short: "Support tools for diagnostics and issue reporting",
}

var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Generate a diagnostic bundle for issue reports",
	Long: `Generate a JSON diagnostic bundle containing redacted configuration,
doctor output, SSH diagnostics, and host info. Useful for attaching
to issue reports.`,
	RunE: runBundle,
}

func init() {
	bundleCmd.Flags().Bool("redact", true, "redact sensitive values (default true)")
	supportCmd.AddCommand(bundleCmd)
	rootCmd.AddCommand(supportCmd)
}

func runBundle(cmd *cobra.Command, args []string) error {
	redact, _ := cmd.Flags().GetBool("redact")
	bundle, err := support.GenerateBundle(Cfg, support.BundleOptions{Redact: redact})
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling bundle: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
