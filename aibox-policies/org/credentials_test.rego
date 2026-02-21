package aibox.policy.credentials_test

import data.aibox.policy.credentials

test_revoke_on_stop_required if {
    credentials.deny["Credentials must be revoked when sandbox stops"] with input as {
        "credentials": {
            "revoke_on_stop": false,
            "no_persist_to_workspace": true,
            "git_token_ttl": "4h",
            "llm_api_key_ttl": "8h"
        }
    }
}

test_no_persist_required if {
    credentials.deny["Credentials must not be persisted to workspace"] with input as {
        "credentials": {
            "revoke_on_stop": true,
            "no_persist_to_workspace": false,
            "git_token_ttl": "4h",
            "llm_api_key_ttl": "8h"
        }
    }
}

test_git_token_ttl_too_long if {
    count(credentials.deny) > 0 with input as {
        "credentials": {
            "revoke_on_stop": true,
            "no_persist_to_workspace": true,
            "git_token_ttl": "12h",
            "llm_api_key_ttl": "8h"
        }
    }
}

test_git_token_ttl_at_limit_passes if {
    ttl_denials := {msg |
        some msg in credentials.deny
        contains(msg, "Git token TTL")
    } with input as {
        "credentials": {
            "revoke_on_stop": true,
            "no_persist_to_workspace": true,
            "git_token_ttl": "4h",
            "llm_api_key_ttl": "8h"
        }
    }
    count(ttl_denials) == 0
}

test_llm_key_ttl_too_long if {
    count(credentials.deny) > 0 with input as {
        "credentials": {
            "revoke_on_stop": true,
            "no_persist_to_workspace": true,
            "git_token_ttl": "4h",
            "llm_api_key_ttl": "24h"
        }
    }
}

test_llm_key_ttl_at_limit_passes if {
    ttl_denials := {msg |
        some msg in credentials.deny
        contains(msg, "LLM API key TTL")
    } with input as {
        "credentials": {
            "revoke_on_stop": true,
            "no_persist_to_workspace": true,
            "git_token_ttl": "4h",
            "llm_api_key_ttl": "8h"
        }
    }
    count(ttl_denials) == 0
}

test_all_valid if {
    count(credentials.deny) == 0 with input as {
        "credentials": {
            "revoke_on_stop": true,
            "no_persist_to_workspace": true,
            "git_token_ttl": "2h",
            "llm_api_key_ttl": "4h"
        }
    }
}
