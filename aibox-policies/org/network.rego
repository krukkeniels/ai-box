package aibox.policy.network

# Deny wildcard host allowlists
deny contains msg if {
    some entry in input.network.allow
    some host in entry.hosts
    host == "*"
    msg := sprintf("Wildcard host '%s' in allowlist entry '%s' is prohibited", [host, entry.id])
}

# Require deny-by-default mode
deny contains msg if {
    input.network.mode != "deny-by-default"
    msg := sprintf("Network mode must be 'deny-by-default', got '%s'", [input.network.mode])
}

# Require rate limiting on LLM gateway
deny contains msg if {
    some entry in input.network.allow
    entry.id == "llm-gateway"
    not entry.rate_limit
    msg := "LLM gateway must have rate_limit configured"
}

# Enforce max rate limits
deny contains msg if {
    some entry in input.network.allow
    entry.rate_limit
    entry.rate_limit.requests_per_min > 120
    msg := sprintf("Rate limit %d rpm exceeds org maximum of 120 rpm for entry '%s'", [entry.rate_limit.requests_per_min, entry.id])
}
