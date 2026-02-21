package aibox.policy.tools

# Default tool decision
default tool_decision := {"allowed": true, "risk_class": "safe", "rule": "default", "reason": "No matching rule; default safe"}

# Evaluate tool/command against rules
tool_decision := result if {
    some i, rule in input.tools.rules
    command_matches(input.command, rule.match)
    result := {
        "allowed": rule.allow,
        "risk_class": rule.risk,
        "rule": sprintf("tools.rules[%d]", [i]),
        "reason": sprintf("Command %v matches rule pattern %v", [input.command, rule.match]),
    }
}

# Command matching: exact match on each position
command_matches(cmd, pattern) if {
    count(cmd) >= count(pattern)
    every i, p in pattern {
        cmd[i] == p
    }
}

# Command matching with wildcard support
command_matches(cmd, pattern) if {
    count(cmd) >= count(pattern)
    every i, p in pattern {
        _position_matches(cmd[i], p)
    }
}

_position_matches(_, "*") if true
_position_matches(actual, expected) if {
    expected != "*"
    actual == expected
}

# Blocked commands
blocked_commands contains cmd if {
    some rule in input.tools.rules
    rule.risk == "blocked-by-default"
    cmd := rule.match
}

# Review-required commands
review_required_commands contains cmd if {
    some rule in input.tools.rules
    rule.risk == "review-required"
    cmd := rule.match
}
