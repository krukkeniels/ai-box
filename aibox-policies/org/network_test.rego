package aibox.policy.network_test

import data.aibox.policy.network

test_deny_by_default_required if {
    network.deny["Network mode must be 'deny-by-default', got 'allow-all'"] with input as {
        "network": {"mode": "allow-all", "allow": []}
    }
}

test_deny_by_default_passes if {
    count(network.deny) == 0 with input as {
        "network": {"mode": "deny-by-default", "allow": []}
    }
}

test_wildcard_host_denied if {
    count(network.deny) > 0 with input as {
        "network": {
            "mode": "deny-by-default",
            "allow": [{"id": "bad-entry", "hosts": ["*"], "ports": [443]}]
        }
    }
}

test_specific_host_allowed if {
    count(network.deny) == 0 with input as {
        "network": {
            "mode": "deny-by-default",
            "allow": [{"id": "git", "hosts": ["git.internal"], "ports": [443]}]
        }
    }
}

test_llm_gateway_requires_rate_limit if {
    network.deny["LLM gateway must have rate_limit configured"] with input as {
        "network": {
            "mode": "deny-by-default",
            "allow": [{"id": "llm-gateway", "hosts": ["llm.internal"], "ports": [443]}]
        }
    }
}

test_llm_gateway_with_rate_limit_passes if {
    count(network.deny) == 0 with input as {
        "network": {
            "mode": "deny-by-default",
            "allow": [{
                "id": "llm-gateway",
                "hosts": ["llm.internal"],
                "ports": [443],
                "rate_limit": {"requests_per_min": 60}
            }]
        }
    }
}

test_rate_limit_exceeds_max if {
    count(network.deny) > 0 with input as {
        "network": {
            "mode": "deny-by-default",
            "allow": [{
                "id": "llm-gateway",
                "hosts": ["llm.internal"],
                "ports": [443],
                "rate_limit": {"requests_per_min": 200}
            }]
        }
    }
}

test_rate_limit_at_max_passes if {
    count(network.deny) == 0 with input as {
        "network": {
            "mode": "deny-by-default",
            "allow": [{
                "id": "llm-gateway",
                "hosts": ["llm.internal"],
                "ports": [443],
                "rate_limit": {"requests_per_min": 120}
            }]
        }
    }
}
