package aibox.policy

import data.aibox.policy.runtime
import data.aibox.policy.network
import data.aibox.policy.filesystem
import data.aibox.policy.credentials

# Aggregate all deny rules from sub-policies.
deny contains msg if {
    runtime.deny[msg]
}

deny contains msg if {
    network.deny[msg]
}

deny contains msg if {
    filesystem.deny[msg]
}

deny contains msg if {
    credentials.deny[msg]
}

# Overall allow decision: allowed when no deny rules fire.
default allow := false

allow if {
    count(deny) == 0
}
