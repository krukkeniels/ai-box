package cmd

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/aibox/aibox/internal/container"
	"github.com/aibox/aibox/internal/policy"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running AI-Box sandbox container",
	Long: `Stop gracefully shuts down the active AI-Box sandbox container
with a 10-second timeout, then force-kills if necessary.
Named volumes (home, toolpacks) are preserved for the next start.`,
	RunE: runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	mgr, err := container.NewManager(Cfg)
	if err != nil {
		return err
	}

	// Log container_stop event to the decision audit log.
	logPath := resolveDecisionLogPath(Cfg)
	decisionLogger, dlErr := policy.NewDecisionLogger(policy.DecisionLogConfig{
		Path:          logPath,
		FlushInterval: 5 * time.Second,
	})
	if dlErr != nil {
		slog.Warn("decision logger unavailable, skipping container_stop audit", "error", dlErr)
	}

	if decisionLogger != nil {
		stopEntry := policy.DecisionEntry{
			Timestamp: time.Now(),
			Action:    "container_stop",
			User:      currentUser(),
			Decision:  "allow",
			RiskClass: policy.RiskSafe,
			Rule:      "lifecycle",
			Reason:    fmt.Sprintf("container stop requested by %s", currentUser()),
		}
		if err := decisionLogger.Log(stopEntry); err != nil {
			slog.Warn("failed to log container_stop event", "error", err)
		}
		if err := decisionLogger.Flush(); err != nil {
			slog.Warn("failed to flush decision log", "error", err)
		}
		if err := decisionLogger.Close(); err != nil {
			slog.Warn("failed to close decision log", "error", err)
		}
	}

	return mgr.Stop("")
}
