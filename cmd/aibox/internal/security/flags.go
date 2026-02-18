package security

import (
	"fmt"
	"strings"
)

// SecurityFlags holds the mandatory container security flags that every
// AI-Box sandbox must be launched with. The CLI must refuse to start a
// container if any of these flags are missing.
type SecurityFlags struct {
	CapDrop          string // --cap-drop value (must be "ALL")
	NoNewPrivileges  bool   // --security-opt=no-new-privileges:true
	ReadOnly         bool   // --read-only root filesystem
	User             string // --user UID:GID
	SeccompProfile   string // absolute path to seccomp.json
	AppArmorProfile  string // AppArmor profile name
}

// DefaultFlags returns the mandatory security flags per SPEC-FINAL.md Section 9.4.
func DefaultFlags(seccompPath string) SecurityFlags {
	return SecurityFlags{
		CapDrop:          "ALL",
		NoNewPrivileges:  true,
		ReadOnly:         true,
		User:             "1000:1000",
		SeccompProfile:   seccompPath,
		AppArmorProfile:  "aibox-sandbox",
	}
}

// BuildArgs returns the security flags as a string slice suitable for passing
// to podman/docker run.
func (f SecurityFlags) BuildArgs() []string {
	args := []string{
		"--cap-drop=" + f.CapDrop,
		"--security-opt=no-new-privileges:true",
		"--read-only",
		"--user=" + f.User,
		"--security-opt=seccomp=" + f.SeccompProfile,
	}

	if f.AppArmorProfile != "" {
		args = append(args, "--security-opt=apparmor="+f.AppArmorProfile)
	}

	return args
}

// Validate checks that all mandatory security flags are correctly set.
// It returns an error describing every violation found so the CLI can
// refuse to launch.
func (f SecurityFlags) Validate() error {
	var violations []string

	if f.CapDrop != "ALL" {
		violations = append(violations, fmt.Sprintf("cap-drop must be ALL, got %q", f.CapDrop))
	}

	if !f.NoNewPrivileges {
		violations = append(violations, "no-new-privileges must be enabled")
	}

	if !f.ReadOnly {
		violations = append(violations, "read-only root filesystem is required")
	}

	if f.User == "" || f.User == "0" || f.User == "0:0" || f.User == "root" || f.User == "root:root" {
		violations = append(violations, fmt.Sprintf("container must run as non-root user, got %q", f.User))
	}

	if f.SeccompProfile == "" {
		violations = append(violations, "seccomp profile path is required")
	}

	if len(violations) > 0 {
		return fmt.Errorf("mandatory security flags violated:\n  - %s", strings.Join(violations, "\n  - "))
	}

	return nil
}

// ValidateArgs inspects a list of container runtime arguments and verifies
// that all mandatory security flags are present. This is used as a final
// gate before exec to ensure nothing was accidentally dropped.
func ValidateArgs(args []string) error {
	var (
		hasCapDropAll      bool
		hasNoNewPrivileges bool
		hasReadOnly        bool
		hasUser            bool
		hasSeccomp         bool
	)

	for _, arg := range args {
		switch {
		case arg == "--cap-drop=ALL":
			hasCapDropAll = true
		case arg == "--security-opt=no-new-privileges:true":
			hasNoNewPrivileges = true
		case arg == "--read-only":
			hasReadOnly = true
		case strings.HasPrefix(arg, "--user="):
			val := strings.TrimPrefix(arg, "--user=")
			if val != "0" && val != "0:0" && val != "root" && val != "root:root" && val != "" {
				hasUser = true
			}
		case strings.HasPrefix(arg, "--security-opt=seccomp="):
			val := strings.TrimPrefix(arg, "--security-opt=seccomp=")
			if val != "" && val != "unconfined" {
				hasSeccomp = true
			}
		}
	}

	var missing []string
	if !hasCapDropAll {
		missing = append(missing, "--cap-drop=ALL")
	}
	if !hasNoNewPrivileges {
		missing = append(missing, "--security-opt=no-new-privileges:true")
	}
	if !hasReadOnly {
		missing = append(missing, "--read-only")
	}
	if !hasUser {
		missing = append(missing, "--user=<non-root>")
	}
	if !hasSeccomp {
		missing = append(missing, "--security-opt=seccomp=<profile>")
	}

	if len(missing) > 0 {
		return fmt.Errorf("refusing to launch: missing mandatory security flags:\n  - %s", strings.Join(missing, "\n  - "))
	}

	return nil
}
