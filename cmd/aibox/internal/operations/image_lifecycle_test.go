package operations

import (
	"testing"
	"time"
)

func TestCheckImageAge_Current(t *testing.T) {
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	img := ImageInfo{
		Variant:   "base",
		Tag:       "24.04-20260220",
		BuildDate: now.Add(-24 * time.Hour), // 1 day old
	}
	cfg := DefaultImageLifecycleConfig()

	status := CheckImageAge(img, cfg, now)
	if status != ImageCurrent {
		t.Errorf("expected ImageCurrent, got %s", status)
	}
}

func TestCheckImageAge_Stale(t *testing.T) {
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	img := ImageInfo{
		Variant:   "base",
		Tag:       "24.04-20260128",
		BuildDate: now.Add(-24 * 24 * time.Hour), // 24 days old (75% of 30)
	}
	cfg := DefaultImageLifecycleConfig()

	status := CheckImageAge(img, cfg, now)
	if status != ImageStale {
		t.Errorf("expected ImageStale, got %s", status)
	}
}

func TestCheckImageAge_Expired(t *testing.T) {
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	img := ImageInfo{
		Variant:   "base",
		Tag:       "24.04-20260115",
		BuildDate: now.Add(-35 * 24 * time.Hour), // 35 days old
	}
	cfg := DefaultImageLifecycleConfig()

	status := CheckImageAge(img, cfg, now)
	if status != ImageExpired {
		t.Errorf("expected ImageExpired, got %s", status)
	}
}

func TestCheckImageAge_MandatoryUpdate(t *testing.T) {
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	img := ImageInfo{
		Variant:   "base",
		Tag:       "24.04-20260220",
		BuildDate: now.Add(-24 * time.Hour), // 1 day old, but before mandatory date
	}
	cfg := DefaultImageLifecycleConfig()
	cfg.MandatoryUpdate = &MandatoryUpdate{
		MinImageDate: now, // must be built today or later
		CVE:          "CVE-2026-99999",
		CVSS:         9.8,
		Enforced:     true,
	}

	status := CheckImageAge(img, cfg, now)
	if status != ImageMandatoryUpdateRequired {
		t.Errorf("expected ImageMandatoryUpdateRequired, got %s", status)
	}
}

func TestCheckImageAge_MandatoryUpdateCompliant(t *testing.T) {
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	img := ImageInfo{
		Variant:   "base",
		Tag:       "24.04-20260221",
		BuildDate: now, // built today
	}
	cfg := DefaultImageLifecycleConfig()
	cfg.MandatoryUpdate = &MandatoryUpdate{
		MinImageDate: now,
		CVE:          "CVE-2026-99999",
		CVSS:         9.8,
		Enforced:     true,
	}

	status := CheckImageAge(img, cfg, now)
	if status != ImageCurrent {
		t.Errorf("expected ImageCurrent, got %s", status)
	}
}

func TestCheckImageAge_MandatoryNotEnforced(t *testing.T) {
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	img := ImageInfo{
		Variant:   "base",
		Tag:       "24.04-20260210",
		BuildDate: now.Add(-11 * 24 * time.Hour),
	}
	cfg := DefaultImageLifecycleConfig()
	cfg.MandatoryUpdate = &MandatoryUpdate{
		MinImageDate: now,
		CVE:          "CVE-2026-99999",
		CVSS:         9.8,
		Enforced:     false, // not enforced
	}

	status := CheckImageAge(img, cfg, now)
	if status != ImageCurrent {
		t.Errorf("expected ImageCurrent (mandatory not enforced), got %s", status)
	}
}

func TestImageAgeDays(t *testing.T) {
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	img := ImageInfo{BuildDate: now.Add(-7 * 24 * time.Hour)}

	days := ImageAgeDays(img, now)
	if days != 7 {
		t.Errorf("expected 7 days, got %d", days)
	}
}

func TestShouldEnforceMandatoryUpdate(t *testing.T) {
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		img      ImageInfo
		update   *MandatoryUpdate
		expected bool
	}{
		{
			name:     "no mandatory update",
			img:      ImageInfo{BuildDate: now.Add(-24 * time.Hour)},
			update:   nil,
			expected: false,
		},
		{
			name: "mandatory not enforced",
			img:  ImageInfo{BuildDate: now.Add(-24 * time.Hour)},
			update: &MandatoryUpdate{
				MinImageDate: now,
				Enforced:     false,
			},
			expected: false,
		},
		{
			name: "mandatory enforced, image too old",
			img:  ImageInfo{BuildDate: now.Add(-24 * time.Hour)},
			update: &MandatoryUpdate{
				MinImageDate: now,
				Enforced:     true,
			},
			expected: true,
		},
		{
			name: "mandatory enforced, image compliant",
			img:  ImageInfo{BuildDate: now},
			update: &MandatoryUpdate{
				MinImageDate: now,
				Enforced:     true,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultImageLifecycleConfig()
			cfg.MandatoryUpdate = tt.update
			result := ShouldEnforceMandatoryUpdate(tt.img, cfg)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestMandatoryUpdateMessage(t *testing.T) {
	update := MandatoryUpdate{
		CVE:  "CVE-2026-12345",
		CVSS: 9.8,
	}
	msg := MandatoryUpdateMessage(update)
	if msg == "" {
		t.Error("expected non-empty message")
	}
	if !containsStr(msg, "CVE-2026-12345") {
		t.Error("message should contain CVE ID")
	}
	if !containsStr(msg, "9.8") {
		t.Error("message should contain CVSS score")
	}
	if !containsStr(msg, "aibox update") {
		t.Error("message should instruct user to run aibox update")
	}
}

func TestGCEligible(t *testing.T) {
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	cfg := DefaultImageLifecycleConfig()

	allTags := []ImageInfo{
		{Tag: "24.04-20260221", BuildDate: now},
		{Tag: "24.04-20260214", BuildDate: now.Add(-7 * 24 * time.Hour)},
		{Tag: "24.04-20260207", BuildDate: now.Add(-14 * 24 * time.Hour)},
		{Tag: "24.04-20260131", BuildDate: now.Add(-21 * 24 * time.Hour)},
		{Tag: "24.04-20260124", BuildDate: now.Add(-28 * 24 * time.Hour)},
	}

	tests := []struct {
		name     string
		tag      string
		build    time.Time
		expected bool
	}{
		{"latest tag never GC'd", "latest", now, false},
		{"newest tag", "24.04-20260221", now, false},
		{"second newest", "24.04-20260214", now.Add(-7 * 24 * time.Hour), false},
		{"third newest", "24.04-20260207", now.Add(-14 * 24 * time.Hour), false},
		{"fourth newest", "24.04-20260131", now.Add(-21 * 24 * time.Hour), false},
		{"fifth (oldest, >4 newer)", "24.04-20260124", now.Add(-28 * 24 * time.Hour), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GCEligible(tt.tag, tt.build, cfg, allTags, now)
			if result != tt.expected {
				t.Errorf("GCEligible(%s): expected %v, got %v", tt.tag, tt.expected, result)
			}
		})
	}
}

func TestGCEligible_CVETag(t *testing.T) {
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	cfg := DefaultImageLifecycleConfig()

	// CVE tag within retention period.
	eligible := GCEligible("24.04-20260201-cve-12345", now.Add(-20*24*time.Hour), cfg, nil, now)
	if eligible {
		t.Error("CVE tag within retention should not be GC eligible")
	}

	// CVE tag beyond retention period.
	eligible = GCEligible("24.04-20251101-cve-12345", now.Add(-112*24*time.Hour), cfg, nil, now)
	if !eligible {
		t.Error("CVE tag beyond retention should be GC eligible")
	}
}

func TestIsCVETag(t *testing.T) {
	tests := []struct {
		tag      string
		expected bool
	}{
		{"24.04-20260221", false},
		{"24.04-20260221-cve-12345", true},
		{"latest", false},
		{"emergency-cve-fix", true},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			result := isCVETag(tt.tag)
			if result != tt.expected {
				t.Errorf("isCVETag(%s): expected %v, got %v", tt.tag, tt.expected, result)
			}
		})
	}
}

func TestComputeFleetImageSummary(t *testing.T) {
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	cfg := DefaultImageLifecycleConfig()

	images := []ImageInfo{
		{Tag: "24.04-20260220", BuildDate: now.Add(-24 * time.Hour)},        // current
		{Tag: "24.04-20260220", BuildDate: now.Add(-24 * time.Hour)},        // current (same tag)
		{Tag: "24.04-20260128", BuildDate: now.Add(-24 * 24 * time.Hour)},   // stale
		{Tag: "24.04-20260115", BuildDate: now.Add(-37 * 24 * time.Hour)},   // expired
	}

	summary := ComputeFleetImageSummary(images, cfg, now)

	if summary.TotalSandboxes != 4 {
		t.Errorf("TotalSandboxes: expected 4, got %d", summary.TotalSandboxes)
	}
	if summary.CurrentImages != 2 {
		t.Errorf("CurrentImages: expected 2, got %d", summary.CurrentImages)
	}
	if summary.StaleImages != 1 {
		t.Errorf("StaleImages: expected 1, got %d", summary.StaleImages)
	}
	if summary.ExpiredImages != 1 {
		t.Errorf("ExpiredImages: expected 1, got %d", summary.ExpiredImages)
	}
	if summary.UniqueVersions != 3 {
		t.Errorf("UniqueVersions: expected 3, got %d", summary.UniqueVersions)
	}
	if summary.OldestImageAgeDays != 37 {
		t.Errorf("OldestImageAgeDays: expected 37, got %d", summary.OldestImageAgeDays)
	}
}

func TestMandatoryUpdateCompliancePercent(t *testing.T) {
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)

	images := []ImageInfo{
		{BuildDate: now},                          // compliant
		{BuildDate: now},                          // compliant
		{BuildDate: now.Add(-24 * time.Hour)},     // non-compliant
		{BuildDate: now.Add(-48 * time.Hour)},     // non-compliant
	}

	cfg := DefaultImageLifecycleConfig()
	cfg.MandatoryUpdate = &MandatoryUpdate{
		MinImageDate: now,
		Enforced:     true,
	}

	pct := MandatoryUpdateCompliancePercent(images, cfg)
	if pct != 50.0 {
		t.Errorf("expected 50.0%%, got %.1f%%", pct)
	}
}

func TestMandatoryUpdateCompliancePercent_NoMandatory(t *testing.T) {
	images := []ImageInfo{{}, {}}
	cfg := DefaultImageLifecycleConfig()

	pct := MandatoryUpdateCompliancePercent(images, cfg)
	if pct != 100.0 {
		t.Errorf("expected 100.0%% when no mandatory update, got %.1f%%", pct)
	}
}

func TestMandatoryUpdateCompliancePercent_EmptyFleet(t *testing.T) {
	cfg := DefaultImageLifecycleConfig()
	cfg.MandatoryUpdate = &MandatoryUpdate{MinImageDate: time.Now(), Enforced: true}

	pct := MandatoryUpdateCompliancePercent(nil, cfg)
	if pct != 100.0 {
		t.Errorf("expected 100.0%% for empty fleet, got %.1f%%", pct)
	}
}

func TestImageAgeStatus_String(t *testing.T) {
	tests := []struct {
		status   ImageAgeStatus
		expected string
	}{
		{ImageCurrent, "current"},
		{ImageStale, "stale"},
		{ImageExpired, "expired"},
		{ImageMandatoryUpdateRequired, "mandatory-update-required"},
		{ImageAgeStatus(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.status.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.status.String())
			}
		})
	}
}

func TestDefaultImageLifecycleConfig(t *testing.T) {
	cfg := DefaultImageLifecycleConfig()
	if cfg.MaxImageAgeDays != 30 {
		t.Errorf("MaxImageAgeDays: expected 30, got %d", cfg.MaxImageAgeDays)
	}
	if cfg.GCRetainCount != 4 {
		t.Errorf("GCRetainCount: expected 4, got %d", cfg.GCRetainCount)
	}
	if cfg.GCRetainCVEDays != 90 {
		t.Errorf("GCRetainCVEDays: expected 90, got %d", cfg.GCRetainCVEDays)
	}
	if cfg.WeeklyRebuildDay != time.Sunday {
		t.Errorf("WeeklyRebuildDay: expected Sunday, got %s", cfg.WeeklyRebuildDay)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
