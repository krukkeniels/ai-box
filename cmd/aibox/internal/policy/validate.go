package policy

import (
	"fmt"
	"strings"
	"time"
)

// ValidationError describes a single validation failure.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidatePolicy checks a single policy for schema correctness and valid values.
func ValidatePolicy(p *Policy) []ValidationError {
	var errs []ValidationError

	if p.Version < 1 {
		errs = append(errs, ValidationError{
			Field:   "version",
			Message: "must be >= 1",
		})
	}

	errs = append(errs, validateNetwork(&p.Network)...)
	errs = append(errs, validateFilesystem(&p.Filesystem)...)
	errs = append(errs, validateTools(&p.Tools)...)
	errs = append(errs, validateResources(&p.Resources)...)
	errs = append(errs, validateRuntime(&p.Runtime)...)
	errs = append(errs, validateCredentials(&p.Credentials)...)

	return errs
}

func validateNetwork(n *NetworkPolicy) []ValidationError {
	var errs []ValidationError

	if n.Mode != "" && n.Mode != "deny-by-default" {
		errs = append(errs, ValidationError{
			Field:   "network.mode",
			Message: fmt.Sprintf("must be %q, got %q", "deny-by-default", n.Mode),
		})
	}

	for i, entry := range n.Allow {
		prefix := fmt.Sprintf("network.allow[%d]", i)

		if entry.ID == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".id",
				Message: "is required",
			})
		}

		if len(entry.Hosts) == 0 {
			errs = append(errs, ValidationError{
				Field:   prefix + ".hosts",
				Message: "must have at least one host",
			})
		}

		for _, h := range entry.Hosts {
			if h == "*" {
				errs = append(errs, ValidationError{
					Field:   prefix + ".hosts",
					Message: "wildcard host \"*\" is not permitted",
				})
			}
		}

		if len(entry.Ports) == 0 {
			errs = append(errs, ValidationError{
				Field:   prefix + ".ports",
				Message: "must have at least one port",
			})
		}

		for _, p := range entry.Ports {
			if p < 1 || p > 65535 {
				errs = append(errs, ValidationError{
					Field:   prefix + ".ports",
					Message: fmt.Sprintf("invalid port %d", p),
				})
			}
		}
	}

	return errs
}

func validateFilesystem(f *FilesystemPolicy) []ValidationError {
	var errs []ValidationError

	for i, path := range f.Deny {
		if !strings.HasPrefix(path, "/") {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("filesystem.deny[%d]", i),
				Message: fmt.Sprintf("must be absolute path, got %q", path),
			})
		}
	}

	return errs
}

func validateTools(t *ToolsPolicy) []ValidationError {
	var errs []ValidationError

	validRisks := map[string]bool{
		RiskSafe:             true,
		RiskReviewRequired:   true,
		RiskBlockedByDefault: true,
	}

	for i, rule := range t.Rules {
		prefix := fmt.Sprintf("tools.rules[%d]", i)

		if len(rule.Match) == 0 {
			errs = append(errs, ValidationError{
				Field:   prefix + ".match",
				Message: "must have at least one pattern",
			})
		}

		if rule.Risk != "" && !validRisks[rule.Risk] {
			errs = append(errs, ValidationError{
				Field:   prefix + ".risk",
				Message: fmt.Sprintf("invalid risk class %q", rule.Risk),
			})
		}
	}

	return errs
}

func validateResources(r *ResourcePolicy) []ValidationError {
	var errs []ValidationError

	if r.CPU != "" && parseResourceBytes(r.CPU) == 0 {
		errs = append(errs, ValidationError{
			Field:   "resources.cpu",
			Message: fmt.Sprintf("invalid resource value %q", r.CPU),
		})
	}

	if r.Memory != "" && parseResourceBytes(r.Memory) == 0 {
		errs = append(errs, ValidationError{
			Field:   "resources.memory",
			Message: fmt.Sprintf("invalid resource value %q", r.Memory),
		})
	}

	if r.Disk != "" && parseResourceBytes(r.Disk) == 0 {
		errs = append(errs, ValidationError{
			Field:   "resources.disk",
			Message: fmt.Sprintf("invalid resource value %q", r.Disk),
		})
	}

	return errs
}

func validateRuntime(r *RuntimePolicy) []ValidationError {
	var errs []ValidationError

	validEngines := map[string]bool{
		"gvisor": true,
		"kata":   true,
	}

	if r.Engine != "" && !validEngines[r.Engine] {
		errs = append(errs, ValidationError{
			Field:   "runtime.engine",
			Message: fmt.Sprintf("must be %q or %q, got %q", "gvisor", "kata", r.Engine),
		})
	}

	return errs
}

func validateCredentials(c *CredentialPolicy) []ValidationError {
	var errs []ValidationError

	ttls := map[string]string{
		"credentials.git_token_ttl":    c.GitTokenTTL,
		"credentials.llm_api_key_ttl":  c.LLMKeyTTL,
		"credentials.mirror_token_ttl": c.MirrorTokenTTL,
	}

	for field, val := range ttls {
		if val != "" {
			if _, err := time.ParseDuration(val); err != nil {
				errs = append(errs, ValidationError{
					Field:   field,
					Message: fmt.Sprintf("invalid duration %q", val),
				})
			}
		}
	}

	return errs
}

// ValidateMerge checks that a three-level policy hierarchy satisfies
// the tighten-only invariant.
func ValidateMerge(org, team, project *Policy) []ValidationError {
	var errs []ValidationError

	// First validate each policy individually.
	if org != nil {
		for _, e := range ValidatePolicy(org) {
			e.Field = "org." + e.Field
			errs = append(errs, e)
		}
	}
	if team != nil {
		for _, e := range ValidatePolicy(team) {
			e.Field = "team." + e.Field
			errs = append(errs, e)
		}
	}
	if project != nil {
		for _, e := range ValidatePolicy(project) {
			e.Field = "project." + e.Field
			errs = append(errs, e)
		}
	}

	// Then check the merge invariant.
	_, err := MergePolicies(org, team, project)
	if mergeErr, ok := err.(*MergeError); ok {
		for _, v := range mergeErr.Violations {
			errs = append(errs, ValidationError{
				Field:   "merge",
				Message: v,
			})
		}
	} else if err != nil {
		errs = append(errs, ValidationError{
			Field:   "merge",
			Message: err.Error(),
		})
	}

	return errs
}
