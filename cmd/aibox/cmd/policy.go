package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/aibox/aibox/internal/policy"
	"github.com/spf13/cobra"
)

var policyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Manage and inspect AI-Box security policies",
	Long:  `Policy provides subcommands for validating and explaining AI-Box security policies.`,
}

// Flag variables for policy validate.
var (
	policyOrgPath     string
	policyTeamPath    string
	policyProjectPath string
)

// Flag variables for policy explain.
var (
	policyLogEntry string
	policyLogFile  string
)

var policyValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate policy hierarchy (org, team, project)",
	Long: `Validate checks the structure and tighten-only invariant of a policy hierarchy.

Loads org, team, and project policy files, validates each individually for
schema correctness, then checks that child policies only tighten (never loosen)
constraints from parent levels.`,
	RunE: runPolicyValidate,
}

var policyExplainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Explain a policy decision from the audit log",
	Long: `Explain reads a decision log entry by line number and displays a
human-readable explanation of the policy decision, including the action,
decision outcome, matched rule, and risk class.`,
	RunE: runPolicyExplain,
}

func init() {
	policyValidateCmd.Flags().StringVar(&policyOrgPath, "org", "/etc/aibox/org-policy.yaml", "path to org baseline policy")
	policyValidateCmd.Flags().StringVar(&policyTeamPath, "team", "", "path to team policy (optional)")
	policyValidateCmd.Flags().StringVar(&policyProjectPath, "project", "./aibox/policy.yaml", "path to project policy")

	policyExplainCmd.Flags().StringVar(&policyLogEntry, "log-entry", "", "line number of the decision log entry to explain")
	policyExplainCmd.Flags().StringVar(&policyLogFile, "log-file", "/var/log/aibox/decisions.jsonl", "path to decision log file")
	_ = policyExplainCmd.MarkFlagRequired("log-entry")

	policyCmd.AddCommand(policyValidateCmd)
	policyCmd.AddCommand(policyExplainCmd)
	rootCmd.AddCommand(policyCmd)
}

func runPolicyValidate(cmd *cobra.Command, args []string) error {
	fmt.Println("Validating policy hierarchy...")

	// Track which levels we will load.
	type level struct {
		label string
		path  string
	}
	levels := []level{
		{"Org baseline", policyOrgPath},
	}
	if policyTeamPath != "" {
		levels = append(levels, level{"Team policy", policyTeamPath})
	}
	levels = append(levels, level{"Project policy", policyProjectPath})

	// Load each policy file.
	policies := make(map[string]*policy.Policy)
	var fileErr bool
	for _, l := range levels {
		p, err := policy.LoadPolicy(l.path)
		if err != nil {
			fmt.Printf("  %-16s %s \u2717\n", l.label+":", l.path)
			fmt.Printf("    Error: %v\n", err)
			fileErr = true
			continue
		}
		policies[l.label] = p
		fmt.Printf("  %-16s %s \u2713\n", l.label+":", l.path)
	}

	if fileErr {
		os.Exit(2)
	}

	// Validate each individually.
	var schemaErrors int
	for _, l := range levels {
		p := policies[l.label]
		if p == nil {
			continue
		}
		errs := policy.ValidatePolicy(p)
		if len(errs) > 0 {
			fmt.Printf("\n  %s: %s \u2717\n", l.label, l.path)
			for _, e := range errs {
				fmt.Printf("    - %s: %s\n", e.Field, e.Message)
			}
			schemaErrors += len(errs)
		}
	}

	if schemaErrors > 0 {
		fmt.Printf("\n%d schema error(s) found. Policy validation failed.\n", schemaErrors)
		os.Exit(1)
	}

	// If multiple levels, validate the merge (tighten-only check).
	org := policies["Org baseline"]
	team := policies["Team policy"]
	project := policies["Project policy"]

	if team != nil || project != nil {
		_, err := policy.MergePolicies(org, team, project)
		if err != nil {
			if mergeErr, ok := err.(*policy.MergeError); ok {
				fmt.Println()
				for _, v := range mergeErr.Violations {
					fmt.Println("VIOLATION: Policy loosening detected")
					fmt.Printf("  Detail: %s\n", v)
					fmt.Println()
				}
				fmt.Printf("%d violation(s) found. Policy validation failed.\n", len(mergeErr.Violations))
				os.Exit(1)
			}
			return fmt.Errorf("merge check failed: %w", err)
		}

		fmt.Println("\nEffective policy merged successfully.")
	}

	fmt.Println("All policies valid.")
	return nil
}

func runPolicyExplain(cmd *cobra.Command, args []string) error {
	lineNum, err := strconv.Atoi(policyLogEntry)
	if err != nil {
		return fmt.Errorf("--log-entry must be a line number (integer), got %q", policyLogEntry)
	}

	entry, err := readDecisionEntry(policyLogFile, lineNum)
	if err != nil {
		return err
	}

	// Format the human-readable explanation.
	fmt.Printf("Decision #%d at %s\n\n", lineNum, entry.Timestamp.Format("2006-01-02T15:04:05Z"))

	action := entry.Action
	if action == "" {
		action = "(unknown)"
	}
	fmt.Printf("Action:    %s\n", action)

	if len(entry.Command) > 0 {
		fmt.Printf("Command:   %s\n", strings.Join(entry.Command, " "))
	}
	if entry.Target != "" {
		fmt.Printf("Target:    %s\n", entry.Target)
	}
	fmt.Printf("User:      %s\n", entry.User)
	fmt.Printf("Sandbox:   %s\n", entry.SandboxID)

	decision := strings.ToUpper(entry.Decision)
	fmt.Printf("Decision:  %s\n", decision)

	fmt.Println()
	if entry.Reason != "" {
		fmt.Printf("Reason:    %s\n", entry.Reason)
	}
	if entry.RiskClass != "" {
		fmt.Printf("           Risk class: %s\n", entry.RiskClass)
	}
	if entry.PolicyVer != "" {
		fmt.Printf("           Policy version: %s\n", entry.PolicyVer)
	}

	if decision == "DENY" {
		fmt.Println()
		fmt.Println("To request an exception:")
		fmt.Println("  1. Submit a policy amendment request to the security team")
		fmt.Println("  2. Or use an approved alternative (e.g., package manager via Nexus)")
	}

	return nil
}

// readDecisionEntry reads a single JSONL entry at the given 0-based line number.
func readDecisionEntry(path string, lineNum int) (*policy.DecisionEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening decision log %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	cur := 0
	for scanner.Scan() {
		if cur == lineNum {
			var entry policy.DecisionEntry
			if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
				return nil, fmt.Errorf("parsing entry at line %d: %w", lineNum, err)
			}
			return &entry, nil
		}
		cur++
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning decision log: %w", err)
	}

	return nil, fmt.Errorf("line %d not found (file has %d lines)", lineNum, cur)
}
