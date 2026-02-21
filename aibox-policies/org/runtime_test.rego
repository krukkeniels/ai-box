package aibox.policy.runtime_test

import data.aibox.policy.runtime

test_gvisor_required if {
    runtime.deny["gVisor runtime is required by org baseline"] with input as {"runtime": {"engine": "runc", "rootless": true}}
}

test_gvisor_allowed if {
    count(runtime.deny) == 0 with input as {"runtime": {"engine": "gvisor", "rootless": true}}
}

test_rootless_required if {
    runtime.deny["Rootless mode is required by org baseline"] with input as {"runtime": {"engine": "gvisor", "rootless": false}}
}

test_rootless_and_gvisor_pass if {
    count(runtime.deny) == 0 with input as {"runtime": {"engine": "gvisor", "rootless": true}}
}

test_runc_denied if {
    runtime.deny["runc runtime is not permitted; use gvisor or kata"] with input as {"runtime": {"engine": "runc", "rootless": true}}
}

test_kata_requires_gvisor if {
    runtime.deny["gVisor runtime is required by org baseline"] with input as {"runtime": {"engine": "kata", "rootless": true}}
}
