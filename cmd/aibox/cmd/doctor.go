package cmd

import (
	"fmt"

	"github.com/aibox/aibox/internal/doctor"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system health and prerequisites",
	Long: `Doctor runs a series of diagnostic checks to verify that the host
system meets all requirements for running AI-Box sandboxes. Checks include
container runtime availability, gVisor installation, kernel features,
network connectivity, and registry access.`,
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().String("format", "text", "output format (text or json)")

	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")

	report := doctor.RunAll(Cfg)

	if format == "json" {
		out, err := report.JSON()
		if err != nil {
			return fmt.Errorf("marshalling report: %w", err)
		}
		fmt.Println(out)
		return nil
	}

	// Text output with status indicators.
	fmt.Println("AI-Box Doctor")
	fmt.Println()

	for _, r := range report.Results {
		var indicator string
		switch r.Status {
		case "pass":
			indicator = "[OK]  "
		case "warn":
			indicator = "[WARN]"
		case "fail":
			indicator = "[FAIL]"
		case "info":
			indicator = "[INFO]"
		default:
			indicator = "[????]"
		}

		fmt.Printf("  %s %s: %s\n", indicator, r.Name, r.Message)

		if r.Remediation != "" && r.Status != "pass" && r.Status != "info" {
			// Indent remediation lines.
			fmt.Printf("         Remediation: %s\n", r.Remediation)
		}
	}

	fmt.Println()
	if report.HasFailures() {
		fmt.Println("Some checks FAILED. Fix the issues above and run 'aibox doctor' again.")
		fmt.Println("Or run 'aibox setup' to auto-configure.")
		return fmt.Errorf("doctor found failures")
	}

	fmt.Println("All checks passed.")
	return nil
}
