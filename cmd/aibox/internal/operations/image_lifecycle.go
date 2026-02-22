// Package operations provides day-2 operational functions for AI-Box fleet
// management, including image lifecycle enforcement, KPI collection, and
// fleet health monitoring.
//
// See docs/phase6/day2-operations-runbooks.md and SPEC-FINAL.md Section 22.
package operations

import (
	"fmt"
	"time"
)

// ImageInfo holds metadata about a container image.
type ImageInfo struct {
	Variant   string    // e.g. "base", "java", "node", "dotnet", "full"
	Tag       string    // e.g. "24.04-20260221"
	Digest    string    // SHA256 digest
	BuildDate time.Time // when the image was built
	Signed    bool      // whether Cosign signature is valid
}

// MandatoryUpdate represents a mandatory update requirement.
type MandatoryUpdate struct {
	MinImageDate time.Time // images older than this must update
	CVE          string    // CVE that triggered the mandatory update
	CVSS         float64   // CVSS score
	Enforced     bool      // whether enforcement is active
}

// ImageLifecycleConfig holds configuration for image lifecycle management.
type ImageLifecycleConfig struct {
	MaxImageAgeDays    int           // alert threshold for image age (default 30)
	MandatoryUpdate    *MandatoryUpdate
	GCRetainCount      int           // number of weekly tags to retain (default 4)
	GCRetainCVEDays    int           // days to retain CVE-tagged images (default 90)
	RegistryURL        string        // Harbor registry URL
	WeeklyRebuildDay   time.Weekday  // day of week for rebuild (default Sunday)
}

// DefaultImageLifecycleConfig returns configuration with sensible defaults.
// Retention count of 4 weekly tags is per PO decision.
func DefaultImageLifecycleConfig() ImageLifecycleConfig {
	return ImageLifecycleConfig{
		MaxImageAgeDays:  30,
		GCRetainCount:    4,
		GCRetainCVEDays:  90,
		RegistryURL:      "harbor.internal",
		WeeklyRebuildDay: time.Sunday,
	}
}

// ImageAgeStatus represents the health status of an image based on age.
type ImageAgeStatus int

const (
	// ImageCurrent means the image is within acceptable age.
	ImageCurrent ImageAgeStatus = iota
	// ImageStale means the image is approaching the age threshold.
	ImageStale
	// ImageExpired means the image exceeds the age threshold.
	ImageExpired
	// ImageMandatoryUpdateRequired means a mandatory security update is pending.
	ImageMandatoryUpdateRequired
)

// String returns a human-readable representation of ImageAgeStatus.
func (s ImageAgeStatus) String() string {
	switch s {
	case ImageCurrent:
		return "current"
	case ImageStale:
		return "stale"
	case ImageExpired:
		return "expired"
	case ImageMandatoryUpdateRequired:
		return "mandatory-update-required"
	default:
		return "unknown"
	}
}

// CheckImageAge determines the health status of an image based on its build
// date and any mandatory update requirements.
func CheckImageAge(img ImageInfo, cfg ImageLifecycleConfig, now time.Time) ImageAgeStatus {
	// Check mandatory update first (highest priority).
	if cfg.MandatoryUpdate != nil && cfg.MandatoryUpdate.Enforced {
		if img.BuildDate.Before(cfg.MandatoryUpdate.MinImageDate) {
			return ImageMandatoryUpdateRequired
		}
	}

	age := now.Sub(img.BuildDate)
	maxAge := time.Duration(cfg.MaxImageAgeDays) * 24 * time.Hour
	warnAge := maxAge * 3 / 4 // warn at 75% of max age

	if age > maxAge {
		return ImageExpired
	}
	if age > warnAge {
		return ImageStale
	}
	return ImageCurrent
}

// ImageAgeDays returns the age of an image in days.
func ImageAgeDays(img ImageInfo, now time.Time) int {
	return int(now.Sub(img.BuildDate).Hours() / 24)
}

// ShouldEnforceMandatoryUpdate returns true if the given image must be updated
// before a sandbox can start. This enforces the CVSS 9+ mandatory update
// policy from SPEC-FINAL.md Section 22.2.
func ShouldEnforceMandatoryUpdate(img ImageInfo, cfg ImageLifecycleConfig) bool {
	if cfg.MandatoryUpdate == nil || !cfg.MandatoryUpdate.Enforced {
		return false
	}
	return img.BuildDate.Before(cfg.MandatoryUpdate.MinImageDate)
}

// MandatoryUpdateMessage returns the user-facing message when a mandatory
// update blocks sandbox start.
func MandatoryUpdateMessage(update MandatoryUpdate) string {
	return fmt.Sprintf(
		"SECURITY UPDATE REQUIRED: A critical vulnerability (%s, CVSS %.1f) has been "+
			"patched. Run 'aibox update' to pull the latest image before starting your sandbox.",
		update.CVE,
		update.CVSS,
	)
}

// GCEligible determines whether an image tag is eligible for garbage collection
// based on the retention policy. Tags referenced by running sandboxes are never
// eligible (that check is external to this function).
func GCEligible(tag string, buildDate time.Time, cfg ImageLifecycleConfig, allTags []ImageInfo, now time.Time) bool {
	// Latest tag is never GC'd.
	if tag == "latest" {
		return false
	}

	// CVE-tagged images retained for GCRetainCVEDays.
	if isCVETag(tag) {
		age := now.Sub(buildDate)
		return age > time.Duration(cfg.GCRetainCVEDays)*24*time.Hour
	}

	// Count how many tags are newer than this one.
	newerCount := 0
	for _, t := range allTags {
		if t.Tag != tag && t.BuildDate.After(buildDate) && !isCVETag(t.Tag) {
			newerCount++
		}
	}

	// Eligible if there are more than GCRetainCount newer tags.
	return newerCount >= cfg.GCRetainCount
}

// isCVETag returns true if the tag contains a CVE identifier.
func isCVETag(tag string) bool {
	// CVE tags follow the pattern: *-cve-*
	for i := 0; i < len(tag)-4; i++ {
		if tag[i:i+4] == "-cve" {
			return true
		}
	}
	return false
}

// FleetImageSummary represents the image distribution across the fleet.
type FleetImageSummary struct {
	TotalSandboxes     int
	CurrentImages      int
	StaleImages        int
	ExpiredImages      int
	MandatoryPending   int
	UniqueVersions     int
	OldestImageAgeDays int
}

// ComputeFleetImageSummary aggregates image health across all active sandboxes.
func ComputeFleetImageSummary(images []ImageInfo, cfg ImageLifecycleConfig, now time.Time) FleetImageSummary {
	summary := FleetImageSummary{
		TotalSandboxes: len(images),
	}

	versions := make(map[string]struct{})
	oldestAge := 0

	for _, img := range images {
		versions[img.Tag] = struct{}{}

		age := ImageAgeDays(img, now)
		if age > oldestAge {
			oldestAge = age
		}

		status := CheckImageAge(img, cfg, now)
		switch status {
		case ImageCurrent:
			summary.CurrentImages++
		case ImageStale:
			summary.StaleImages++
		case ImageExpired:
			summary.ExpiredImages++
		case ImageMandatoryUpdateRequired:
			summary.MandatoryPending++
		}
	}

	summary.UniqueVersions = len(versions)
	summary.OldestImageAgeDays = oldestAge
	return summary
}

// MandatoryUpdateCompliancePercent returns the percentage of the fleet that
// has applied a mandatory update. Returns 100.0 if no mandatory update is active.
func MandatoryUpdateCompliancePercent(images []ImageInfo, cfg ImageLifecycleConfig) float64 {
	if cfg.MandatoryUpdate == nil || !cfg.MandatoryUpdate.Enforced {
		return 100.0
	}
	if len(images) == 0 {
		return 100.0
	}

	compliant := 0
	for _, img := range images {
		if !img.BuildDate.Before(cfg.MandatoryUpdate.MinImageDate) {
			compliant++
		}
	}
	return float64(compliant) / float64(len(images)) * 100.0
}
