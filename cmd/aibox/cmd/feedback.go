package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aibox/aibox/internal/config"
	"github.com/aibox/aibox/internal/feedback"
	"github.com/spf13/cobra"
)

var feedbackCmd = &cobra.Command{
	Use:   "feedback",
	Short: "Collect and view developer feedback",
	Long: `Feedback provides subcommands for submitting developer experience
ratings and viewing feedback history. Feedback is stored locally in
~/.aibox/feedback/ as daily JSON files.`,
}

var feedbackSubmitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submit a feedback rating",
	Long: `Submit records a developer experience rating (1-5) with an optional comment.

Rating scale:
  1 = Unusable (could not complete work)
  2 = Poor (significant friction, fell back to local)
  3 = Acceptable (some friction but workable)
  4 = Good (minor friction only)
  5 = Excellent (better than or equal to local dev)

Examples:
  aibox feedback submit              # Interactive prompt
  aibox feedback submit -r 4         # Non-interactive, rating only
  aibox feedback submit -r 4 -c "Build was slow today"`,
	RunE: runFeedbackSubmit,
}

var feedbackStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show recent feedback and statistics",
	Long:  `Status displays recent feedback submissions and aggregate statistics.`,
	RunE:  runFeedbackStatus,
}

func init() {
	feedbackSubmitCmd.Flags().IntP("rating", "r", 0, "rating (1-5)")
	feedbackSubmitCmd.Flags().StringP("comment", "c", "", "optional comment")

	feedbackCmd.AddCommand(feedbackSubmitCmd)
	feedbackCmd.AddCommand(feedbackStatusCmd)
	rootCmd.AddCommand(feedbackCmd)
}

func runFeedbackSubmit(cmd *cobra.Command, args []string) error {
	rating, _ := cmd.Flags().GetInt("rating")
	comment, _ := cmd.Flags().GetString("comment")

	// Interactive mode if no rating flag provided.
	if rating == 0 {
		var err error
		rating, comment, err = promptFeedback(cmd)
		if err != nil {
			return err
		}
	}

	store, err := newFeedbackStore()
	if err != nil {
		return err
	}

	entry := feedback.Entry{
		Timestamp: time.Now().UTC(),
		Rating:    rating,
		Comment:   comment,
	}

	if err := store.Submit(entry); err != nil {
		return fmt.Errorf("submitting feedback: %w", err)
	}

	labels := []string{"", "Unusable", "Poor", "Acceptable", "Good", "Excellent"}
	fmt.Fprintf(cmd.OutOrStdout(), "Feedback recorded: %d/5 (%s)\n", rating, labels[rating])
	if comment != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Comment: %s\n", comment)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Thank you for your feedback!")

	return nil
}

func runFeedbackStatus(cmd *cobra.Command, args []string) error {
	store, err := newFeedbackStore()
	if err != nil {
		return err
	}

	stats, err := store.Stats()
	if err != nil {
		return fmt.Errorf("loading stats: %w", err)
	}

	if stats.TotalEntries == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No feedback submitted yet. Run 'aibox feedback submit' to get started.")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Feedback Summary\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Total submissions: %d\n", stats.TotalEntries)
	fmt.Fprintf(cmd.OutOrStdout(), "  Days with feedback: %d\n", stats.DaysWithFeedback)
	fmt.Fprintf(cmd.OutOrStdout(), "  Average rating:    %.1f / 5\n", stats.AverageRating)
	fmt.Fprintf(cmd.OutOrStdout(), "  Acceptable (>= 3): %d / %d\n", stats.AcceptableCount, stats.TotalEntries)

	recent, err := store.Recent(5)
	if err != nil {
		return fmt.Errorf("loading recent: %w", err)
	}

	if len(recent) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nRecent feedback:\n")
		for _, e := range recent {
			ts := e.Timestamp.Local().Format("2006-01-02 15:04")
			line := fmt.Sprintf("  %s  %d/5", ts, e.Rating)
			if e.Comment != "" {
				line += fmt.Sprintf("  %q", e.Comment)
			}
			fmt.Fprintln(cmd.OutOrStdout(), line)
		}
	}

	return nil
}

func promptFeedback(cmd *cobra.Command) (int, string, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Fprintln(cmd.OutOrStdout(), "AI-Box Feedback")
	fmt.Fprintln(cmd.OutOrStdout(), "  1 = Unusable    2 = Poor    3 = Acceptable    4 = Good    5 = Excellent")
	fmt.Fprint(cmd.OutOrStdout(), "Rating (1-5): ")

	ratingStr, err := reader.ReadString('\n')
	if err != nil {
		return 0, "", fmt.Errorf("reading rating: %w", err)
	}

	rating, err := strconv.Atoi(strings.TrimSpace(ratingStr))
	if err != nil || rating < 1 || rating > 5 {
		return 0, "", fmt.Errorf("invalid rating %q: must be 1-5", strings.TrimSpace(ratingStr))
	}

	fmt.Fprint(cmd.OutOrStdout(), "Comment (optional, press Enter to skip): ")
	comment, err := reader.ReadString('\n')
	if err != nil {
		return 0, "", fmt.Errorf("reading comment: %w", err)
	}

	return rating, strings.TrimSpace(comment), nil
}

func newFeedbackStore() (*feedback.Store, error) {
	homeDir, err := config.ResolveHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolving home directory: %w", err)
	}

	dir := feedback.DefaultDir(homeDir)
	return feedback.NewStore(dir), nil
}
