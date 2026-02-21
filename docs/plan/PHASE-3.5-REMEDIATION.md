# Phase 3.5: Remediation & Integration Hardening

**Phase**: 3.5 (between Phase 3 and Phase 4)
**Status**: Required
**Estimated Effort**: 2-3 engineer-weeks (1-2 engineers, ~2 calendar weeks)
**Dependencies**: Phases 0-3 complete
**Triggered by**: Post-Phase 3 implementation review (2026-02-21)

---

## Overview

Phases 0-3 are substantially complete. All three Go binaries compile (`aibox`, `aibox-llm-proxy`, `aibox-credential-helper`), all 34 OPA/Rego policy tests pass, and the core architecture is sound. However, the review identified **test failures, cross-phase integration gaps, and missing configuration** that must be fixed before Phase 4 (Developer Experience) can proceed safely.

Phase 3.5 is not new feature work. It is hardening, fixing, and integration-testing the work already done in Phases 0-3.

---

## Issues Found

### Category A: Test Failures (5 failing tests)

| # | Test | Package | Root Cause |
|---|------|---------|------------|
| A1 | `TestDefaultValues` (6 sub-tests) | `internal/config` | Test calls `Load("")` which discovers `~/.config/aibox/config.yaml` (a user test config with `image: ubuntu:24.04`, `gvisor.enabled: false`, etc.). Test does not isolate itself from the real user config file. |
| A2 | `TestAppArmorProfile_Exists` | `tests` | Test expects string `deny /var/run/docker.sock` but the AppArmor profile uses glob pattern `deny /**/docker.sock rw,`. The glob is actually more secure (blocks docker.sock at any path), but the test assertion is wrong. |
| A3 | `TestMountLayout_WorkspaceWritable` | `tests` | Integration test runs `touch /workspace/test-file` inside a container but fails with `Permission denied`. Likely a rootless Podman / gVisor permission issue with the bind mount, or the workspace directory doesn't exist pre-container-start. |
| A4 | `TestContainerLifecycle` (tests/) | `tests` | Fails because AppArmor profile is not loaded (`apparmor_parser` requires root/sudo). The test attempts `aibox start` which gates on AppArmor being active. |
| A5 | `TestContainerLifecycle` (tests/integration/) | `tests/integration` | Same root cause as A4 -- AppArmor profile load requires elevated privileges. |

### Category B: Cross-Phase Integration Gaps

| # | Issue | Severity | Detail |
|---|-------|----------|--------|
| B1 | `vault.internal` missing from Squid proxy allowlist | **Critical** | Phase 3 requires containers to reach Vault for credential management. The org `policy.yaml` lists `vault.internal` as an allowed host, but the actual Squid config (`configs/squid.conf`), CoreDNS Corefile (`configs/Corefile`), and Go `DefaultAllowedDomains` list only contain 4 domains: `harbor.internal`, `nexus.internal`, `foundry.internal`, `git.internal`. Containers **cannot reach Vault** through the network stack. |
| B2 | `vault.internal` missing from CoreDNS hosts | **Critical** | Same gap as B1 but at the DNS layer. CoreDNS has no entry for `vault.internal`, so DNS resolution returns NXDOMAIN even if Squid were to allow it. |
| B3 | AppArmor profile not auto-loaded without sudo | **High** | `aibox start` fails if the AppArmor profile hasn't been loaded via `aibox setup` (which requires root). There's no graceful degradation path when the user hasn't run setup with elevated privileges. The doctor check warns but `start` hard-fails. |
| B4 | Policy engine not wired into container start | **Medium** | `cmd/aibox/cmd/start.go` uses the credential broker for env var injection, but the policy engine (`policy.Engine`) is not invoked during container startup to validate the effective policy or enforce tool permissions at runtime. The OPA engine is built but not integrated into the container lifecycle. |
| B5 | Decision logger not started in container | **Medium** | The `DecisionLogger` is implemented but not instantiated during `aibox start`. No decision log file is being written. The `policy explain` command reads from a log that doesn't exist yet. |

### Category C: Configuration & Hardening Gaps

| # | Issue | Severity | Detail |
|---|-------|----------|--------|
| C1 | Config test pollution | **Medium** | The `TestDefaultValues` test is not hermetic. Any developer with a `~/.config/aibox/config.yaml` will see failures. The test must use `t.TempDir()` or `t.Setenv("HOME", ...)` to isolate. |
| C2 | AppArmor test assertion mismatch | **Low** | The test checks for `deny /var/run/docker.sock` but the profile uses the more secure `deny /**/docker.sock rw,`. Either update the test to match the profile's glob pattern or add the explicit path as well. |
| C3 | No end-to-end network + policy integration test | **Medium** | There are unit tests for network components and unit tests for policy components, but no test that validates: container starts -> network controls active -> credential fetch from Vault succeeds -> policy decisions are logged. |
| C4 | Seccomp profile not validated against spec in tests | **Low** | The seccomp profile correctly blocks all 14 spec-mandated syscalls (verified manually), but no automated test validates this. A regression could silently allow a dangerous syscall. |
| C5 | Missing `setup` integration for non-root users | **Medium** | `aibox setup` installs nftables, Squid, CoreDNS, and AppArmor -- all requiring root. There's no split between `aibox setup --system` (root, run once by admin) and `aibox setup --user` (non-root, run by developer). This creates friction for the Phase 4/6 developer onboarding flow. |

---

## Tasks

### Task 1: Fix `vault.internal` in Network Stack [Critical]

**What**: Add `vault.internal` to all three network layers so containers can reach Vault for credential management.

**Files to change**:
- `cmd/aibox/configs/squid.conf` -- add `acl aibox_allowed dstdomain .vault.internal`
- `cmd/aibox/configs/Corefile` -- add `10.0.0.14 vault.internal` to the hosts block
- `cmd/aibox/internal/network/squid.go` -- add `"vault.internal"` to `DefaultAllowedDomains`
- `cmd/aibox/internal/config/config.go` -- add `"vault.internal"` to default `allowed_domains` list
- `cmd/aibox/configs/nftables.conf` -- no change needed (traffic routes through proxy)
- Update Squid and CoreDNS tests to expect `vault.internal`

**Acceptance**: `dig vault.internal` from inside a container returns the correct IP. `curl https://vault.internal:8200/v1/sys/health` succeeds through the proxy.

**Effort**: 0.5 days

---

### Task 2: Fix All Failing Tests [High]

**What**: Make all Go tests pass cleanly.

**Sub-tasks**:

**T2a: Fix `TestDefaultValues` (config test isolation)**
- Modify `TestDefaultValues` to set `HOME` to a temp directory via `t.Setenv("HOME", t.TempDir())` so `Load("")` doesn't discover the user's real config file.
- File: `cmd/aibox/internal/config/config_test.go`

**T2b: Fix `TestAppArmorProfile_Exists` (string assertion)**
- Change the assertion from `deny /var/run/docker.sock` to `deny /**/docker.sock` to match the actual profile glob pattern.
- File: `cmd/aibox/tests/security_test.go:130`

**T2c: Fix `TestMountLayout_WorkspaceWritable` (container integration)**
- Ensure the test creates the workspace directory before bind-mounting, or skip if no container runtime is available.
- File: `cmd/aibox/tests/filesystem_test.go`

**T2d: Fix `TestContainerLifecycle` (AppArmor gate)**
- Make the test skip gracefully when AppArmor is not available (no root), rather than hard-failing.
- Alternatively, have `aibox start` degrade gracefully when AppArmor is unavailable (warn + continue with gVisor + seccomp as compensating controls).
- Files: `cmd/aibox/tests/integration_test.go`, `cmd/aibox/internal/container/container.go`

**Acceptance**: `go test ./...` from within `cmd/aibox/` passes with 0 failures.

**Effort**: 1-2 days

---

### Task 3: Wire Policy Engine into Container Lifecycle [Medium]

**What**: Connect the OPA policy engine and decision logger to the container start/stop lifecycle.

**Sub-tasks**:

**T3a: Load and validate effective policy at start**
- During `aibox start`, load the policy hierarchy (org + team + project), run merge validation, and reject startup if the policy is invalid.
- If no project policy exists, use org baseline only (no error).
- File: `cmd/aibox/cmd/start.go`

**T3b: Start decision logger**
- Instantiate `DecisionLogger` during container setup.
- Ensure the log directory `/var/log/aibox/` exists (create if not).
- Flush and close on `aibox stop`.

**T3c: Log container lifecycle decisions**
- Log "container_start" and "container_stop" events to the decision log.
- Include: user, workspace, image, policy version hash, timestamp.

**Acceptance**: After `aibox start` + `aibox stop`, `/var/log/aibox/decisions.jsonl` contains at least 2 entries (start + stop). `aibox policy explain --log-entry 1` displays the start event.

**Effort**: 2-3 days

---

### Task 4: AppArmor Graceful Degradation [High]

**What**: `aibox start` should not hard-fail when AppArmor is unavailable. Instead, it should warn and continue with gVisor + seccomp as compensating controls.

**Rationale**: AppArmor requires `apparmor_parser` with root privileges. Developers who haven't run `aibox setup` with sudo (or are on systems without AppArmor) are completely blocked from starting containers. Since gVisor + seccomp already provide strong isolation, AppArmor is defense-in-depth, not a hard requirement.

**Implementation**:
- Change the AppArmor check in container launch from fatal error to warning.
- Add a config flag `gvisor.require_apparmor: false` (default false) for organizations that want to enforce it.
- Log a `WARN` when AppArmor is not loaded but gVisor + seccomp are active.
- `aibox doctor` continues to report AppArmor status but doesn't gate on it.

**Files to change**:
- `cmd/aibox/internal/container/container.go` -- change AppArmor gate from error to warning
- `cmd/aibox/internal/config/config.go` -- add `require_apparmor` config field

**Acceptance**: `aibox start` succeeds on a system without AppArmor loaded (but with gVisor + seccomp). `aibox doctor` warns about missing AppArmor.

**Effort**: 0.5 days

---

### Task 5: Add Seccomp Blocked-Syscall Validation Test [Low]

**What**: Automated test that verifies the seccomp profile blocks all 14 spec-mandated syscalls.

**Implementation**: Parse `configs/seccomp.json`, extract the allowlist, and verify that `ptrace`, `mount`, `umount2`, `pivot_root`, `chroot`, `bpf`, `userfaultfd`, `unshare`, `setns`, `init_module`, `finit_module`, `kexec_load`, `keyctl`, and `add_key` are NOT in the allowlist.

**File**: `cmd/aibox/tests/security_test.go` (add new test function)

**Acceptance**: Test passes and would catch a regression if someone accidentally adds a blocked syscall to the allowlist.

**Effort**: 0.5 days

---

### Task 6: Add End-to-End Integration Test [Medium]

**What**: A single integration test that validates the full Phase 0-3 stack works together.

**Test flow**:
1. `aibox start --workspace /tmp/e2e-test` succeeds
2. Container is running with gVisor runtime
3. nftables rules are active
4. DNS resolves allowlisted domains, returns NXDOMAIN for others
5. `curl https://google.com` fails from inside container
6. Policy validation works (`aibox policy validate`)
7. Credential helper returns credentials (fallback mode)
8. `aibox stop` succeeds, container removed, volumes preserved
9. Decision log exists and is valid JSON

**File**: `cmd/aibox/tests/integration/e2e_test.go` (new file)

**Note**: This test requires a running Podman + gVisor environment. Tag it with `//go:build integration` so it doesn't run in CI without explicit opt-in.

**Acceptance**: Test passes on a properly configured development machine. Skips gracefully in CI environments without Podman/gVisor.

**Effort**: 2-3 days

---

### Task 7: Split `aibox setup` into System and User Phases [Medium]

**What**: Separate privileged setup (nftables, Squid, CoreDNS, AppArmor) from unprivileged setup (config file, image pull, volume creation).

**Implementation**:
- `aibox setup --system` (requires root): installs nftables rules, Squid, CoreDNS, AppArmor profile, gVisor runtime. Run once per machine by an admin.
- `aibox setup` (no root): creates config directory, pulls images, creates named volumes, writes default config. Run by each developer.
- `aibox setup` without `--system` checks if system setup has been done and warns if not, but doesn't fail.

**Rationale**: Phase 4/6 developer onboarding requires a smooth `aibox setup` that doesn't demand root. System-level components should be pre-installed by IT or provisioned via config management (Ansible/Puppet).

**Files to change**:
- `cmd/aibox/cmd/setup.go` -- split into system/user flows
- `cmd/aibox/internal/setup/linux.go` -- separate privileged and unprivileged operations

**Acceptance**: `aibox setup` (no root) succeeds and creates a usable environment on a machine where `aibox setup --system` was previously run by an admin.

**Effort**: 1-2 days

---

## Summary Table

| # | Task | Severity | Effort | Blocks Phase 4? |
|---|------|----------|--------|-----------------|
| T1 | Fix `vault.internal` in network stack | Critical | 0.5 days | Yes -- credential fetch from Vault fails |
| T2 | Fix all failing tests | High | 1-2 days | Yes -- CI gate broken |
| T3 | Wire policy engine into container lifecycle | Medium | 2-3 days | Yes -- policy enforcement is not active |
| T4 | AppArmor graceful degradation | High | 0.5 days | Yes -- developers can't start containers without root |
| T5 | Seccomp blocked-syscall validation test | Low | 0.5 days | No |
| T6 | End-to-end integration test | Medium | 2-3 days | No, but strongly recommended |
| T7 | Split `aibox setup` system/user | Medium | 1-2 days | Partially -- affects onboarding in Phase 6 |

**Critical path for Phase 4**: T1, T2, T3, T4 (total: 4.5-6 days, ~1 calendar week with 1-2 engineers)

---

## Exit Criteria

Phase 3.5 is complete when ALL of the following are true:

1. `go test ./...` passes with 0 failures in `cmd/aibox/`
2. `opa test aibox-policies/org/ -v` passes (34/34 -- already passing, regression gate)
3. `vault.internal` is resolvable from inside a container via CoreDNS
4. `vault.internal` is reachable through the Squid proxy
5. `aibox start` succeeds without root (when gVisor + seccomp are available, AppArmor optional)
6. Policy engine loads and validates the effective policy during `aibox start`
7. Decision log entries are written to `/var/log/aibox/decisions.jsonl` during container lifecycle
8. Seccomp profile blocks all 14 spec-mandated syscalls (automated test)

---

## Dependency on Phase 4

Phase 4 (Developer Experience) requires:
- Working `aibox start` without root -- **blocked by T4**
- Credential injection for AI tools (Vault connectivity) -- **blocked by T1**
- Policy enforcement for tool permissions (review-required, blocked-by-default) -- **blocked by T3**
- Clean CI test suite for ongoing development -- **blocked by T2**

**Recommendation**: Complete T1, T2, T3, T4 before beginning any Phase 4 work. T5, T6, T7 can run in parallel with early Phase 4 tasks.
