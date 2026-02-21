package aibox.policy.filesystem

# Require read-only root filesystem
deny contains msg if {
    not input.filesystem.read_only_root
    msg := "Read-only root filesystem is required"
}

# Deny access to sensitive paths
deny_path contains msg if {
    some denied in input.filesystem.deny
    startswith(input.target, denied)
    msg := sprintf("Access to '%s' is denied by filesystem policy (matches deny rule '%s')", [input.target, denied])
}

# Workspace must be under /workspace
deny contains msg if {
    input.filesystem.workspace_root != "/workspace"
    msg := sprintf("Workspace root must be '/workspace', got '%s'", [input.filesystem.workspace_root])
}
