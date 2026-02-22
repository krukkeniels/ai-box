// Package feedback provides developer feedback collection for AI-Box rollout.
// Feedback entries are stored as JSON files in ~/.aibox/feedback/.
package feedback

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Entry represents a single feedback submission.
type Entry struct {
	Timestamp time.Time `json:"timestamp"`
	Rating    int       `json:"rating"`
	Comment   string    `json:"comment,omitempty"`
}

// Validate checks that the entry has required fields and valid values.
func (e *Entry) Validate() error {
	if e.Timestamp.IsZero() {
		return ErrMissingTimestamp
	}
	if e.Rating < 1 || e.Rating > 5 {
		return ErrInvalidRating
	}
	return nil
}

// DayFile holds all feedback entries for a single day.
type DayFile struct {
	Date    string  `json:"date"`
	Entries []Entry `json:"entries"`
}

// Predefined errors.
var (
	ErrMissingTimestamp = errors.New("feedback: missing timestamp")
	ErrInvalidRating    = errors.New("feedback: rating must be between 1 and 5")
	ErrNoFeedback       = errors.New("feedback: no feedback found")
)

// Store manages feedback persistence to the filesystem.
type Store struct {
	dir string
}

// NewStore creates a Store that reads/writes to the given directory.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// DefaultDir returns the default feedback storage directory (~/.aibox/feedback).
func DefaultDir(homeDir string) string {
	return filepath.Join(homeDir, ".aibox", "feedback")
}

// Submit records a new feedback entry. The entry is appended to the day file
// matching the entry's timestamp date.
func (s *Store) Submit(entry Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("feedback: creating directory: %w", err)
	}

	dateStr := entry.Timestamp.Format("2006-01-02")
	path := filepath.Join(s.dir, dateStr+".json")

	dayFile, err := s.loadDayFile(path, dateStr)
	if err != nil {
		return err
	}

	dayFile.Entries = append(dayFile.Entries, entry)

	data, err := json.MarshalIndent(dayFile, "", "  ")
	if err != nil {
		return fmt.Errorf("feedback: marshaling: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("feedback: writing file: %w", err)
	}

	return nil
}

// Recent returns the most recent n feedback entries across all day files,
// ordered by timestamp descending.
func (s *Store) Recent(n int) ([]Entry, error) {
	files, err := s.listDayFiles()
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, nil
	}

	var all []Entry
	// Iterate in reverse (most recent first).
	for i := len(files) - 1; i >= 0 && len(all) < n; i-- {
		dayFile, loadErr := s.loadDayFile(files[i], "")
		if loadErr != nil {
			continue
		}
		all = append(all, dayFile.Entries...)
	}

	// Sort descending by timestamp.
	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp.After(all[j].Timestamp)
	})

	if len(all) > n {
		all = all[:n]
	}

	return all, nil
}

// Stats returns aggregate statistics across all feedback entries.
func (s *Store) Stats() (*Summary, error) {
	files, err := s.listDayFiles()
	if err != nil {
		return nil, err
	}

	summary := &Summary{}
	for _, f := range files {
		dayFile, loadErr := s.loadDayFile(f, "")
		if loadErr != nil {
			continue
		}
		for _, e := range dayFile.Entries {
			summary.TotalEntries++
			summary.RatingSum += e.Rating
			if e.Rating >= 3 {
				summary.AcceptableCount++
			}
		}
		summary.DaysWithFeedback++
	}

	if summary.TotalEntries > 0 {
		summary.AverageRating = float64(summary.RatingSum) / float64(summary.TotalEntries)
	}

	return summary, nil
}

// Summary holds aggregate feedback statistics.
type Summary struct {
	TotalEntries     int     `json:"total_entries"`
	DaysWithFeedback int     `json:"days_with_feedback"`
	RatingSum        int     `json:"rating_sum"`
	AverageRating    float64 `json:"average_rating"`
	AcceptableCount  int     `json:"acceptable_count"` // Entries with rating >= 3
}

func (s *Store) loadDayFile(path string, dateStr string) (*DayFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &DayFile{Date: dateStr}, nil
		}
		return nil, fmt.Errorf("feedback: reading file: %w", err)
	}

	var dayFile DayFile
	if err := json.Unmarshal(data, &dayFile); err != nil {
		return nil, fmt.Errorf("feedback: parsing file: %w", err)
	}

	return &dayFile, nil
}

func (s *Store) listDayFiles() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("feedback: listing directory: %w", err)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		files = append(files, filepath.Join(s.dir, e.Name()))
	}

	sort.Strings(files)
	return files, nil
}
