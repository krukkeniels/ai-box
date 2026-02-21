package aibox.policy.runtime

# Require gVisor runtime
deny contains msg if {
    input.runtime.engine != "gvisor"
    msg := "gVisor runtime is required by org baseline"
}

# Require rootless mode
deny contains msg if {
    input.runtime.rootless == false
    msg := "Rootless mode is required by org baseline"
}

# Prevent runtime downgrade to runc
deny contains msg if {
    input.runtime.engine == "runc"
    msg := "runc runtime is not permitted; use gvisor or kata"
}
