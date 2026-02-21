package aibox.policy.tools_test

import data.aibox.policy.tools

test_default_safe if {
    result := tools.tool_decision with input as {
        "command": ["echo", "hello"],
        "tools": {"rules": []}
    }
    result.allowed == true
    result.risk_class == "safe"
}

test_exact_match_blocked if {
    result := tools.tool_decision with input as {
        "command": ["rm", "-rf", "/workspace"],
        "tools": {"rules": [
            {"match": ["rm", "-rf", "/workspace"], "allow": false, "risk": "blocked-by-default"}
        ]}
    }
    result.allowed == false
    result.risk_class == "blocked-by-default"
}

test_review_required if {
    result := tools.tool_decision with input as {
        "command": ["git", "push"],
        "tools": {"rules": [
            {"match": ["git", "push"], "allow": true, "risk": "review-required"}
        ]}
    }
    result.allowed == true
    result.risk_class == "review-required"
}

test_no_match_returns_default if {
    result := tools.tool_decision with input as {
        "command": ["ls", "-la"],
        "tools": {"rules": [
            {"match": ["git", "push"], "allow": true, "risk": "review-required"}
        ]}
    }
    result.allowed == true
    result.risk_class == "safe"
}

test_blocked_commands_set if {
    cmds := tools.blocked_commands with input as {
        "tools": {"rules": [
            {"match": ["rm", "-rf"], "allow": false, "risk": "blocked-by-default"},
            {"match": ["git", "push"], "allow": true, "risk": "review-required"}
        ]}
    }
    count(cmds) == 1
}

test_review_required_commands_set if {
    cmds := tools.review_required_commands with input as {
        "tools": {"rules": [
            {"match": ["rm", "-rf"], "allow": false, "risk": "blocked-by-default"},
            {"match": ["git", "push"], "allow": true, "risk": "review-required"}
        ]}
    }
    count(cmds) == 1
}
