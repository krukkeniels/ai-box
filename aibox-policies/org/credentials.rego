package aibox.policy.credentials

# Require credential revocation on stop
deny contains msg if {
    not input.credentials.revoke_on_stop
    msg := "Credentials must be revoked when sandbox stops"
}

# Prevent credential persistence to workspace
deny contains msg if {
    not input.credentials.no_persist_to_workspace
    msg := "Credentials must not be persisted to workspace"
}

# Enforce maximum git token TTL
deny contains msg if {
    ttl_hours := parse_duration_hours(input.credentials.git_token_ttl)
    ttl_hours > 4
    msg := sprintf("Git token TTL '%s' exceeds maximum of 4h", [input.credentials.git_token_ttl])
}

# Enforce maximum LLM API key TTL
deny contains msg if {
    ttl_hours := parse_duration_hours(input.credentials.llm_api_key_ttl)
    ttl_hours > 8
    msg := sprintf("LLM API key TTL '%s' exceeds maximum of 8h", [input.credentials.llm_api_key_ttl])
}

# Helper to parse duration strings like "4h" to hours
parse_duration_hours(s) := hours if {
    endswith(s, "h")
    hours := to_number(trim_suffix(s, "h"))
}
