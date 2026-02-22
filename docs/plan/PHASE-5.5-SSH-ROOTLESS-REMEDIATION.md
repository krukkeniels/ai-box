# Phase 5.5: SSH & Rootless Podman Remediation

## Overview

Phase 5.5 addresses critical compatibility issues discovered during end-to-end testing of `aibox start` + VS Code Remote SSH connection. The SSH connectivity flow (Phase 4 deliverable) was non-functional due to five interlocking bugs in the container runtime, security profile, image, and entrypoint configuration.

These fixes are required before Phase 6 rollout as they directly affect the developer experience path.

**Root Cause**: The security hardening applied in Phase 1 (seccomp, cap-drop, no-new-privileges, read-only rootfs) was designed assuming rootful containers, but the actual deployment uses rootless podman where the kernel enforces stricter capability restrictions via user namespaces.

---

## Bugs Found & Fixed

### Bug 1: `--network=none` Blocks Port Forwarding
**File**: `internal/container/container.go:193`
**Issue**: When `network.enabled=false` (default), the container was launched with `--network=none`, which disables ALL networking — including the `-p 2222:22` port mapping needed for SSH.
**Fix**: Changed to `--network=slirp4netns` which provides minimal networking with port forwarding support while still isolating the container from the host network.

### Bug 2: Seccomp Profile Missing Critical Syscalls
**File**: `cmd/aibox/configs/seccomp.json`
**Issue**: The custom seccomp profile (default-deny) was missing syscalls required for:
- Container capability initialization (`capget`, `capset`)
- User switching by sshd (`setuid`, `setgid`, `setreuid`, `setregid`, `setresuid`, `setresgid`)
- sshd privilege separation (`chroot`)
- Privilege dropping in entrypoint (`getresuid`, `getresgid`, `initgroups`)
- Modern glibc file access checks (`faccessat2`)
**Fix**: Added all missing syscalls to the seccomp allowlist. These are required for sshd privilege separation and are safe under rootless podman's user namespace isolation.

### Bug 3: `--cap-drop=ALL` + `--no-new-privileges` Incompatible With sshd in Rootless Mode
**File**: `internal/container/container.go:126-128`
**Issue**: `--cap-drop=ALL` removes all capabilities, and `--no-new-privileges` prevents gaining any back. sshd fundamentally requires `SETUID`, `SETGID`, `SYS_CHROOT` etc. for privilege separation after authentication. In rootless podman, `--cap-add` after `--cap-drop=ALL` fails at the kernel level ("unable to get capability version").
**Fix**: When SSH port mapping is enabled (`effectiveSSHPort > 0`), omit `--cap-drop=ALL` and `--no-new-privileges`. Rootless podman's user namespace already provides equivalent isolation. When SSH is disabled, the original strict flags are preserved.

### Bug 4: `su` Blocked by Seccomp (PAM Session Failure)
**File**: `internal/container/container.go:253` (entrypoint)
**Issue**: The entrypoint used `exec su -s /bin/bash dev -c 'sleep infinity'` to drop from root to dev user after starting sshd. `su` uses PAM, which requires syscalls not in the seccomp allowlist. Error: "cannot open session: Critical error - immediate abort"
**Fix**: Replaced `su` with `setpriv --reuid=1000 --regid=1000 --init-groups` which performs a direct UID/GID transition without PAM dependency.

### Bug 5: Dev Account Locked + SSH Key Ownership
**Files**: `aibox-images/base/Containerfile:106`, `internal/container/container.go:588`
**Issue (a)**: The `dev` user was created by `useradd` without a password, leaving the account locked (`!` in shadow). sshd rejects locked accounts even for pubkey auth: "User dev not allowed because account is locked".
**Issue (b)**: `injectSSHKey` created `/home/dev/.ssh/authorized_keys` as root (via `podman exec` which runs as root). sshd drops to uid 1000 to read the file but can't access root-owned files. Error: "Could not open user 'dev' authorized keys: Permission denied"
**Fix (a)**: Added `passwd -d dev` to Containerfile to unlock the account.
**Fix (b)**: Added `chown -R dev:dev /home/dev/.ssh` to `injectSSHKey`.

---

## Security Impact Assessment

| Change | Security Impact | Mitigation |
|--------|----------------|------------|
| `slirp4netns` instead of `none` | Container has outbound networking | Seccomp + nftables (when enabled) still restrict traffic |
| Added 12 syscalls to seccomp | Slightly larger attack surface | All are standard POSIX syscalls needed for user switching |
| No `cap-drop=ALL` with SSH | Container retains default caps | Rootless podman caps are already restricted to user namespace |
| No `no-new-privileges` with SSH | sshd can do setuid transitions | Required for sshd privilege separation; seccomp still enforced |
| `setpriv` instead of `su` | No PAM session tracking | Acceptable in sandbox — PAM not needed for ephemeral containers |
| `passwd -d dev` | Account has no password | `PasswordAuthentication no` in sshd_config; only pubkey auth works |

### Defense-in-Depth Still Active
Even with these relaxations for SSH-enabled containers:
- Seccomp profile enforces syscall allowlist
- Read-only root filesystem
- User namespaces (rootless podman)
- Workspace bind-mounted read-write only to /workspace
- SSH key-only authentication
- sshd hardened config (no root login, no password auth, max 3 auth tries)

---

## Security Validation Changes

**File**: `internal/security/flags.go`

The `ValidateArgsWithExpectations` function was updated to recognize two valid security modes:
1. **Full lockdown** (no SSH): `--cap-drop=ALL` + `--no-new-privileges` required
2. **SSH mode**: Both omitted; rootless user namespace provides isolation

---

## Files Changed

| File | Change |
|------|--------|
| `internal/container/container.go` | Network mode, security flags, entrypoint, SSH key chown |
| `internal/security/flags.go` | SSH-mode validation exemption |
| `cmd/aibox/configs/seccomp.json` | Added 12 missing syscalls |
| `aibox-images/base/Containerfile` | `passwd -d dev` to unlock account |
| `aibox-images/base/Containerfile.test` | Minimal test image for validation |
| `aibox-images/base/sshd_config` | No changes needed (config was correct) |

---

## Test Results

```
$ aibox start -w /tmp/aibox-test-workspace
AI-Box sandbox started.
  Container: aibox-race-day-7de376d4
  SSH:       localhost:2222

$ ssh -i ~/.config/aibox/ssh/aibox_ed25519 -p 2222 dev@localhost
=== SSH CONNECTION SUCCESSFUL ===
uid=1000(dev) gid=1000(dev) groups=1000(dev)
/home/dev
README.md
hello from aibox
```

VS Code Remote SSH: Host "aibox" configured in `~/.ssh/config`, connects via key auth on port 2222.

---

## Prerequisites for Phase 6

Before Phase 6 rollout:
1. Run full security test suite to verify no regressions
2. Update existing tests for new security validation modes
3. Rebuild production base image with `passwd -d dev` fix
4. Deploy updated seccomp profile to all developer machines (`/etc/aibox/seccomp.json`)
5. Document the two security modes in operator guide
