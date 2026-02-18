package security

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// IsAppArmorAvailable checks whether AppArmor is enabled on the host kernel
// by inspecting /sys/module/apparmor.
func IsAppArmorAvailable() bool {
	info, err := os.Stat("/sys/module/apparmor")
	if err != nil {
		slog.Debug("AppArmor module not found", "error", err)
		return false
	}
	return info.IsDir()
}

// IsProfileLoaded checks whether a named AppArmor profile is loaded in the
// kernel by scanning /sys/kernel/security/apparmor/profiles.
func IsProfileLoaded(name string) (bool, error) {
	f, err := os.Open("/sys/kernel/security/apparmor/profiles")
	if err != nil {
		return false, fmt.Errorf("cannot read AppArmor profiles: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Format: "profile_name (mode)"
		if strings.HasPrefix(line, name+" ") {
			slog.Debug("AppArmor profile found", "name", name, "entry", line)
			return true, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("reading AppArmor profiles: %w", err)
	}

	return false, nil
}

// LoadProfile loads (or replaces) an AppArmor profile from the given file
// path using apparmor_parser.
func LoadProfile(profilePath string) error {
	parser, err := exec.LookPath("apparmor_parser")
	if err != nil {
		return fmt.Errorf("apparmor_parser not found: %w", err)
	}

	// -r replaces the profile if it already exists, -W writes cache.
	cmd := exec.Command(parser, "-r", "-W", profilePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	slog.Debug("loading AppArmor profile", "path", profilePath, "parser", parser)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("apparmor_parser failed for %s: %w", profilePath, err)
	}

	slog.Info("AppArmor profile loaded", "path", profilePath)
	return nil
}

// EnsureProfile checks that the aibox-sandbox AppArmor profile is loaded.
// If not, it attempts to load it from profilePath. If AppArmor is not
// available on the system, it logs a warning and returns nil (degraded mode).
func EnsureProfile(profilePath string) error {
	if !IsAppArmorAvailable() {
		slog.Warn("AppArmor is not available on this system; running with reduced isolation")
		return nil
	}

	loaded, err := IsProfileLoaded("aibox-sandbox")
	if err != nil {
		slog.Warn("could not check AppArmor profile status", "error", err)
		// Try loading anyway.
	}

	if loaded {
		slog.Debug("AppArmor profile already loaded", "name", "aibox-sandbox")
		return nil
	}

	return LoadProfile(profilePath)
}
