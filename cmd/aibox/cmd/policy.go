package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

var policyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Manage and inspect AI-Box security policies",
	Long:  `Policy provides subcommands for validating and explaining AI-Box security policies.`,
}

var policyValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate a policy.yaml file",
	Long: `Validate checks the structure of an AI-Box policy file. It looks for
policy.yaml in /aibox/policy.yaml (inside the container) or ./aibox/policy.yaml
in the current workspace directory.`,
	RunE: runPolicyValidate,
}

var policyExplainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Explain a policy decision from the audit log",
	Long: `Explain takes a log entry ID and provides a human-readable explanation
of why a particular policy decision was made. Requires OPA integration (Phase 3).`,
	RunE: runPolicyExplain,
}

func init() {
	policyExplainCmd.Flags().String("log-entry", "", "audit log entry ID to explain")
	_ = policyExplainCmd.MarkFlagRequired("log-entry")

	policyCmd.AddCommand(policyValidateCmd)
	policyCmd.AddCommand(policyExplainCmd)
	rootCmd.AddCommand(policyCmd)
}

// policyFile represents the minimal expected structure of policy.yaml.
type policyFile struct {
	Version string        `yaml:"version"`
	Network networkPolicy `yaml:"network"`
}

type networkPolicy struct {
	Mode string `yaml:"mode"`
}

func runPolicyValidate(cmd *cobra.Command, args []string) error {
	// Search for policy file in known locations.
	candidates := []string{
		"/aibox/policy.yaml",
		"./aibox/policy.yaml",
	}

	var policyPath string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			policyPath = p
			break
		}
	}

	if policyPath == "" {
		return fmt.Errorf("policy.yaml not found; searched %v", candidates)
	}

	fmt.Printf("Validating %s ...\n", policyPath)

	data, err := os.ReadFile(policyPath)
	if err != nil {
		return fmt.Errorf("reading policy file: %w", err)
	}

	var policy policyFile
	if err := yaml.Unmarshal(data, &policy); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}

	var errors []string

	if policy.Version == "" {
		errors = append(errors, "missing required field: version")
	}
	if policy.Network.Mode == "" {
		errors = append(errors, "missing required field: network.mode")
	} else {
		validModes := map[string]bool{"none": true, "filtered": true, "open": true}
		if !validModes[policy.Network.Mode] {
			errors = append(errors, fmt.Sprintf("invalid network.mode %q: must be one of none, filtered, open", policy.Network.Mode))
		}
	}

	if len(errors) > 0 {
		fmt.Println("Validation FAILED:")
		for _, e := range errors {
			fmt.Printf("  - %s\n", e)
		}
		return fmt.Errorf("policy validation failed with %d error(s)", len(errors))
	}

	fmt.Println("Validation PASSED")
	fmt.Printf("  version:      %s\n", policy.Version)
	fmt.Printf("  network.mode: %s\n", policy.Network.Mode)

	return nil
}

func runPolicyExplain(cmd *cobra.Command, args []string) error {
	logEntry, _ := cmd.Flags().GetString("log-entry")
	fmt.Printf("Log entry: %s\n", logEntry)
	fmt.Println("Full policy explanation requires OPA integration (Phase 3).")
	fmt.Println("This feature will be available once Phase 3 is complete.")

	return nil
}
