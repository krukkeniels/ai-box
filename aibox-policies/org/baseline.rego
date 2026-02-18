package aibox.org.baseline

# AI-Box Organization Baseline Policy
# This policy is immutable and enforced on all sandboxes.
# Team and project policies can only tighten these rules, never loosen them.

default allow_sandbox_start = false

# Require gVisor runtime
allow_sandbox_start {
    input.runtime == "runsc"
}

# Deny wildcard network access
deny_network[msg] {
    some rule in input.network.allow
    rule.host == "*"
    msg := "Wildcard network access is not permitted by org baseline"
}

# Require image signature verification
deny_image[msg] {
    not input.image.signed
    msg := "Unsigned images are not permitted"
}

# Require rate limiting on LLM API
deny_llm[msg] {
    input.llm.rate_limit_rpm > 60
    msg := sprintf("LLM rate limit %d exceeds org maximum of 60 rpm", [input.llm.rate_limit_rpm])
}

deny_llm[msg] {
    input.llm.rate_limit_tpm > 100000
    msg := sprintf("LLM token limit %d exceeds org maximum of 100000 tpm", [input.llm.rate_limit_tpm])
}

# Enforce non-root execution
deny_sandbox[msg] {
    input.user.uid == 0
    msg := "Running as root is not permitted"
}

# Enforce read-only rootfs
deny_sandbox[msg] {
    not input.filesystem.read_only_root
    msg := "Read-only root filesystem is required"
}

# Enforce capability drop
deny_sandbox[msg] {
    count(input.security.capabilities) > 0
    msg := "No Linux capabilities are permitted"
}
