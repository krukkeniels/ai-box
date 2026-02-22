package feedback

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEntryValidate(t *testing.T) {
	tests := []struct {
		name    string
		entry   Entry
		wantErr error
	}{
		{
			name: "valid entry",
			entry: Entry{
				Timestamp: time.Now(),
				Rating:    4,
				Comment:   "works well",
			},
			wantErr: nil,
		},
		{
			name: "valid entry without comment",
			entry: Entry{
				Timestamp: time.Now(),
				Rating:    3,
			},
			wantErr: nil,
		},
		{
			name: "rating 1 is valid",
			entry: Entry{
				Timestamp: time.Now(),
				Rating:    1,
			},
			wantErr: nil,
		},
		{
			name: "rating 5 is valid",
			entry: Entry{
				Timestamp: time.Now(),
				Rating:    5,
			},
			wantErr: nil,
		},
		{
			name: "missing timestamp",
			entry: Entry{
				Rating: 4,
			},
			wantErr: ErrMissingTimestamp,
		},
		{
			name: "rating too low",
			entry: Entry{
				Timestamp: time.Now(),
				Rating:    0,
			},
			wantErr: ErrInvalidRating,
		},
		{
			name: "rating too high",
			entry: Entry{
				Timestamp: time.Now(),
				Rating:    6,
			},
			wantErr: ErrInvalidRating,
		},
		{
			name: "negative rating",
			entry: Entry{
				Timestamp: time.Now(),
				Rating:    -1,
			},
			wantErr: ErrInvalidRating,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.entry.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestStoreSubmit(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	entry := Entry{
		Timestamp: time.Date(2026, 2, 21, 14, 30, 0, 0, time.UTC),
		Rating:    4,
		Comment:   "good experience",
	}

	if err := store.Submit(entry); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	// Verify file was created.
	path := filepath.Join(dir, "2026-02-21.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var dayFile DayFile
	if err := json.Unmarshal(data, &dayFile); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if dayFile.Date != "2026-02-21" {
		t.Errorf("Date = %q, want %q", dayFile.Date, "2026-02-21")
	}
	if len(dayFile.Entries) != 1 {
		t.Fatalf("len(Entries) = %d, want 1", len(dayFile.Entries))
	}
	if dayFile.Entries[0].Rating != 4 {
		t.Errorf("Rating = %d, want 4", dayFile.Entries[0].Rating)
	}
	if dayFile.Entries[0].Comment != "good experience" {
		t.Errorf("Comment = %q, want %q", dayFile.Entries[0].Comment, "good experience")
	}
}

func TestStoreSubmitAppends(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	ts := time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC)

	if err := store.Submit(Entry{Timestamp: ts, Rating: 3, Comment: "morning"}); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	ts2 := time.Date(2026, 2, 21, 17, 0, 0, 0, time.UTC)
	if err := store.Submit(Entry{Timestamp: ts2, Rating: 5, Comment: "afternoon"}); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	path := filepath.Join(dir, "2026-02-21.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var dayFile DayFile
	if err := json.Unmarshal(data, &dayFile); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if len(dayFile.Entries) != 2 {
		t.Fatalf("len(Entries) = %d, want 2", len(dayFile.Entries))
	}
	if dayFile.Entries[0].Comment != "morning" {
		t.Errorf("Entries[0].Comment = %q, want %q", dayFile.Entries[0].Comment, "morning")
	}
	if dayFile.Entries[1].Comment != "afternoon" {
		t.Errorf("Entries[1].Comment = %q, want %q", dayFile.Entries[1].Comment, "afternoon")
	}
}

func TestStoreSubmitMultipleDays(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	day1 := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)

	if err := store.Submit(Entry{Timestamp: day1, Rating: 3}); err != nil {
		t.Fatalf("Submit() day1 error = %v", err)
	}
	if err := store.Submit(Entry{Timestamp: day2, Rating: 4}); err != nil {
		t.Fatalf("Submit() day2 error = %v", err)
	}

	// Verify two files exist.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("len(entries) = %d, want 2", len(entries))
	}
}

func TestStoreSubmitValidation(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	err := store.Submit(Entry{Rating: 0})
	if err == nil {
		t.Fatal("Submit() should fail with invalid entry")
	}
}

func TestStoreRecent(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	day1 := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)

	for _, e := range []Entry{
		{Timestamp: day1, Rating: 2, Comment: "day1"},
		{Timestamp: day2, Rating: 3, Comment: "day2"},
		{Timestamp: day3, Rating: 5, Comment: "day3"},
	} {
		if err := store.Submit(e); err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
	}

	recent, err := store.Recent(2)
	if err != nil {
		t.Fatalf("Recent() error = %v", err)
	}

	if len(recent) != 2 {
		t.Fatalf("len(recent) = %d, want 2", len(recent))
	}

	// Most recent first.
	if recent[0].Comment != "day3" {
		t.Errorf("recent[0].Comment = %q, want %q", recent[0].Comment, "day3")
	}
	if recent[1].Comment != "day2" {
		t.Errorf("recent[1].Comment = %q, want %q", recent[1].Comment, "day2")
	}
}

func TestStoreRecentEmptyDir(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	recent, err := store.Recent(5)
	if err != nil {
		t.Fatalf("Recent() error = %v", err)
	}
	if len(recent) != 0 {
		t.Errorf("len(recent) = %d, want 0", len(recent))
	}
}

func TestStoreRecentNonExistentDir(t *testing.T) {
	store := NewStore("/tmp/aibox-test-nonexistent-" + t.Name())

	recent, err := store.Recent(5)
	if err != nil {
		t.Fatalf("Recent() error = %v", err)
	}
	if len(recent) != 0 {
		t.Errorf("len(recent) = %d, want 0", len(recent))
	}
}

func TestStoreStats(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	entries := []Entry{
		{Timestamp: time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC), Rating: 2},
		{Timestamp: time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC), Rating: 3},
		{Timestamp: time.Date(2026, 2, 20, 17, 0, 0, 0, time.UTC), Rating: 4},
		{Timestamp: time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC), Rating: 5},
	}

	for _, e := range entries {
		if err := store.Submit(e); err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
	}

	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}

	if stats.TotalEntries != 4 {
		t.Errorf("TotalEntries = %d, want 4", stats.TotalEntries)
	}
	if stats.DaysWithFeedback != 3 {
		t.Errorf("DaysWithFeedback = %d, want 3", stats.DaysWithFeedback)
	}
	if stats.RatingSum != 14 {
		t.Errorf("RatingSum = %d, want 14", stats.RatingSum)
	}
	wantAvg := 3.5
	if stats.AverageRating != wantAvg {
		t.Errorf("AverageRating = %f, want %f", stats.AverageRating, wantAvg)
	}
	if stats.AcceptableCount != 3 {
		t.Errorf("AcceptableCount = %d, want 3 (ratings >= 3)", stats.AcceptableCount)
	}
}

func TestStoreStatsEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}

	if stats.TotalEntries != 0 {
		t.Errorf("TotalEntries = %d, want 0", stats.TotalEntries)
	}
	if stats.AverageRating != 0 {
		t.Errorf("AverageRating = %f, want 0", stats.AverageRating)
	}
}

func TestDefaultDir(t *testing.T) {
	got := DefaultDir("/home/testuser")
	want := "/home/testuser/.aibox/feedback"
	if got != want {
		t.Errorf("DefaultDir() = %q, want %q", got, want)
	}
}

func TestNewStore(t *testing.T) {
	store := NewStore("/tmp/test-feedback")
	if store.dir != "/tmp/test-feedback" {
		t.Errorf("store.dir = %q, want %q", store.dir, "/tmp/test-feedback")
	}
}
