package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// memoryPattern matches common memory size strings like "4g", "512m", "8Gi", "2048".
var memoryPattern = regexp.MustCompile(`(?i)^[1-9]\d*\s*([kmg]i?b?)?$`)

// imagePattern matches a container image reference.
// Simplified: registry/repo:tag or registry/repo@sha256:...
var imagePattern = regexp.MustCompile(`^[a-zA-Z0-9][\w.\-/]*[a-zA-Z0-9](:[a-zA-Z0-9][\w.\-]*)?(@sha256:[a-f0-9]{64})?$`)

// Validate checks the configuration for invalid values and returns a
// descriptive error if any field is incorrect.
func (c *Config) Validate() error {
	var errs []string

	// Runtime must be podman or docker.
	switch c.Runtime {
	case "podman", "docker":
		// ok
	default:
		errs = append(errs, fmt.Sprintf("invalid runtime %q: must be \"podman\" or \"docker\"", c.Runtime))
	}

	// Image must not be empty and should look like a valid reference.
	if c.Image == "" {
		errs = append(errs, "image must not be empty")
	} else if !imagePattern.MatchString(c.Image) {
		errs = append(errs, fmt.Sprintf("invalid image reference %q", c.Image))
	}

	// gVisor platform must be systrap or ptrace when enabled.
	if c.GVisor.Enabled {
		switch c.GVisor.Platform {
		case "systrap", "ptrace":
			// ok
		case "":
			errs = append(errs, "gvisor.platform must be set when gvisor is enabled")
		default:
			errs = append(errs, fmt.Sprintf("invalid gvisor.platform %q: must be \"systrap\" or \"ptrace\"", c.GVisor.Platform))
		}
	}

	// Resources: CPUs must be positive.
	if c.Resources.CPUs < 1 {
		errs = append(errs, fmt.Sprintf("resources.cpus must be >= 1, got %d", c.Resources.CPUs))
	}

	// Resources: memory must be parseable.
	if c.Resources.Memory == "" {
		errs = append(errs, "resources.memory must not be empty")
	} else if !isValidMemorySize(c.Resources.Memory) {
		errs = append(errs, fmt.Sprintf("invalid resources.memory %q: use format like \"4g\", \"8Gi\", or \"2048\"", c.Resources.Memory))
	}

	// Resources: tmp_size must be parseable if set.
	if c.Resources.TmpSize != "" && !isValidMemorySize(c.Resources.TmpSize) {
		errs = append(errs, fmt.Sprintf("invalid resources.tmp_size %q: use format like \"2g\" or \"1024m\"", c.Resources.TmpSize))
	}

	// Registry URL must not be empty.
	if c.Registry.URL == "" {
		errs = append(errs, "registry.url must not be empty")
	}

	// Logging format.
	switch c.Logging.Format {
	case "text", "json":
		// ok
	default:
		errs = append(errs, fmt.Sprintf("invalid logging.format %q: must be \"text\" or \"json\"", c.Logging.Format))
	}

	// Logging level.
	switch strings.ToLower(c.Logging.Level) {
	case "debug", "info", "warn", "error":
		// ok
	default:
		errs = append(errs, fmt.Sprintf("invalid logging.level %q: must be debug, info, warn, or error", c.Logging.Level))
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  %s", strings.Join(errs, "\n  "))
	}

	return nil
}

// isValidMemorySize checks whether s looks like a valid memory size.
func isValidMemorySize(s string) bool {
	s = strings.TrimSpace(s)
	// Try as a plain integer (bytes).
	if _, err := strconv.ParseUint(s, 10, 64); err == nil {
		return true
	}
	return memoryPattern.MatchString(s)
}
