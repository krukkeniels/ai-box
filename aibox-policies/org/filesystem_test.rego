package aibox.policy.filesystem_test

import data.aibox.policy.filesystem

test_read_only_root_required if {
    filesystem.deny["Read-only root filesystem is required"] with input as {
        "filesystem": {"read_only_root": false, "workspace_root": "/workspace", "deny": []}
    }
}

test_read_only_root_passes if {
    count(filesystem.deny) == 0 with input as {
        "filesystem": {"read_only_root": true, "workspace_root": "/workspace", "deny": []}
    }
}

test_workspace_root_must_be_workspace if {
    count(filesystem.deny) > 0 with input as {
        "filesystem": {"read_only_root": true, "workspace_root": "/home/user", "deny": []}
    }
}

test_workspace_root_correct_passes if {
    count(filesystem.deny) == 0 with input as {
        "filesystem": {"read_only_root": true, "workspace_root": "/workspace", "deny": []}
    }
}

test_deny_path_blocks_sensitive if {
    count(filesystem.deny_path) > 0 with input as {
        "target": "/etc/shadow",
        "filesystem": {"read_only_root": true, "workspace_root": "/workspace", "deny": ["/etc/shadow", "/proc/kcore"]}
    }
}

test_deny_path_prefix_match if {
    count(filesystem.deny_path) > 0 with input as {
        "target": "/sys/firmware/efi",
        "filesystem": {"read_only_root": true, "workspace_root": "/workspace", "deny": ["/sys/firmware"]}
    }
}

test_allowed_path_passes if {
    count(filesystem.deny_path) == 0 with input as {
        "target": "/workspace/src/main.go",
        "filesystem": {"read_only_root": true, "workspace_root": "/workspace", "deny": ["/etc/shadow"]}
    }
}
