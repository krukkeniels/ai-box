# Phase 1: Core Runtime & CLI

**Phase**: 1 of 6
**Estimated Duration**: 6-8 engineer-weeks (2-3 engineers, ~3-4 calendar weeks)
**Phase Dependencies**: Phase 0 (Infrastructure Foundation) must be complete
**Status**: Not Started

---

## Overview

Phase 1 delivers the container runtime and the `aibox` CLI. By the end, a developer can run `aibox setup && aibox start --workspace ~/project && aibox shell` and land inside a gVisor-isolated, security-hardened container with the correct filesystem layout, persistent build caches, and all mandatory security controls applied.

This phase does **not** include network security (Phase 2), credential injection (Phase 3), or IDE integration (Phase 4). The container has no outbound network access and no injected credentials -- it is a locked-down shell environment.

The CLI is the single abstraction layer that shields developers from infrastructure complexity. Every subsequent phase extends it rather than introducing new interfaces.

---

## Deliverables

### D1: `aibox` CLI Binary

Statically-linked Go binary with commands: `setup`, `start`, `stop`, `shell`, `status`, `update`, `doctor`. Detects host OS (native Linux vs Windows 11/WSL2) and adapts behavior.

**Language decision: Go.** The Podman ecosystem is Go-native, OPA embeds as a Go library, cross-compilation is trivial, and platform teams are more likely to have Go experience. This is decided, not research.

*Acceptance*: `aibox --help` displays all commands. Binary runs on x86_64 Linux and inside WSL2 without additional dependencies.

### D2: Podman + gVisor Integration

Containers launch with `--runtime=runsc` by default. Docker fallback when Podman is unavailable.

*Acceptance*: `aibox start` produces a container where `/proc/self/status` shows gVisor's Sentry. `podman inspect` confirms `runsc` runtime.

### D3: Seccomp Profile

Allowlist-based (default-deny) seccomp profile blocking `ptrace`, `mount`, `umount2`, `pivot_root`, `chroot`, `bpf`, `userfaultfd`, `unshare`, `setns`, `init_module`, `finit_module`, `kexec_load`, `keyctl`, `add_key`, plus all other syscalls not required by development tooling.

*Acceptance*: Blocked syscall (e.g., `ptrace`) returns `EPERM`. Profile passes `seccomp-tools` validation.

### D4: AppArmor Profile

Enforces read-only root, writable `/workspace` and `/tmp`, denies access to sensitive paths.

*Acceptance*: `cat /proc/1/attr/current` shows `aibox-sandbox` loaded. Writing to `/etc/` fails. Writing to `/workspace` succeeds.

### D5: Mandatory Security Flags

Every container launch applies: `--cap-drop=ALL`, `--security-opt=no-new-privileges:true`, `--read-only`, `--user=1000:1000`, plus seccomp and AppArmor profiles.

*Acceptance*: `capsh --print` shows zero capabilities. Setuid execution fails. Root filesystem writes fail.

### D6: Filesystem Mount Layout

Per spec Section 10.1:
- `/` -- read-only
- `/workspace` -- rw,nosuid,nodev (bind mount)
- `/home/dev` -- rw,nosuid,nodev (named volume, persistent)
- `/opt/toolpacks` -- rw,nosuid,nodev (named volume)
- `/tmp` -- tmpfs, rw,noexec,nosuid, size=2G
- `/var/tmp` -- tmpfs, rw,noexec,nosuid, size=1G

*Acceptance*: `mount` output matches layout. Files in `/home/dev` persist across stop/start. NTFS-mounted workspaces blocked with clear error.

### D7: Build Cache Persistence

Named volumes for Maven, Gradle, npm, NuGet, Bazel caches that survive container recreation.

*Acceptance*: Build, stop, start, rebuild -- second build uses cached artifacts.

### D8: WSL2 Detection and Setup

`aibox setup` on Windows 11 configures WSL2, creates Podman machine, applies `.wslconfig`, validates environment.

*Acceptance*: On clean Windows 11, `aibox setup` produces a working environment. `aibox doctor` reports all-green.

### D9: `aibox doctor` Diagnostics

Health check verifying: Podman installed, gVisor available, WSL2 memory allocation (if applicable), image signature valid, disk space sufficient.

*Acceptance*: Deliberately misconfigure gVisor. `aibox doctor` reports specific failure with remediation guidance.

### D10: WSL2 Polyglot Validation

Dedicated WSL2 validation for the full polyglot stack (.NET + JVM + Node + Bazel + AI agent) under WSL2 + Podman + gVisor.

*Acceptance*:
- Full gVisor compatibility matrix passes under WSL2.
- Expanded `.wslconfig` recommendations documented (24GB memory for polyglot workloads).
- WSL2 known-issues registry created (living document, updated throughout all phases).
- Performance benchmarks captured on WSL2 vs native Linux.

### D11: Performance Benchmarks

Cold start < 90s, warm start < 15s.

*Acceptance*: Automated benchmark script measures start times across 10 runs. p95 meets SLA targets.

---

## Implementation Steps

### Work Stream 1: CLI Skeleton (1.5 weeks)

- CLI framework with subcommand routing: `setup`, `start`, `stop`, `shell`, `status`, `update`, `doctor`
- Config file format (`~/.config/aibox/config.yaml`)
- Host OS detection (native Linux, WSL2, macOS unsupported with clear error)
- Runtime detection (Podman preferred, Docker fallback)
- Structured logging (JSON for machines, human-readable for terminal)

**Spec references**: Section 5.6, Appendix A, Appendix B

### Work Stream 2: Podman + gVisor Integration (1.5 weeks)

- Container launch: translate `aibox start` into `podman run` invocation
- gVisor runtime: install `runsc`, register as OCI runtime
- Docker fallback with rootless mode validation
- Container lifecycle: start, stop, restart, destroy, status
- Naming: `aibox-<username>-<workspace-hash>`
- Image pull with signature verification via Phase 0's `policy.json`
- Network mode: `--network=none` (no network until Phase 2)

**Spec references**: Section 5.1, Section 7

### Work Stream 3: Security Profiles (1 week)

- **Seccomp**: Start from Docker's default, tighten per spec Section 9.2. Validate against real workloads (Java, Node, Python, .NET, Git) using `strace`.
- **AppArmor**: Implement spec Section 9.3 profile. Handle systems without AppArmor gracefully (warn, continue with reduced protection).
- **Mandatory flags**: Codify flag set so CLI refuses to launch without them.

Strategy: launch with Docker's default seccomp first, tighten iteratively using `strace` data from real workloads.

**Spec references**: Section 9.1, 9.2, 9.3, 9.4

### Work Stream 4: Filesystem Layout & Build Caches (1 week)

- Mount point configuration per spec's mount table
- Workspace validation: block NTFS-mounted paths (check `/proc/mounts` for `9p`/`drvfs`)
- Named volume management: `aibox-<username>-<purpose>` for Maven, Gradle, npm, NuGet, Bazel caches
- Persistent `/home/dev` volume
- `/opt/toolpacks` placeholder volume
- `aibox repair cache` command

**Spec references**: Section 10.1, 10.2, 10.3, 10.4

### Work Stream 5: WSL2 Support & Polyglot Validation (1.5-2 weeks)

- WSL2 detection (`/proc/version` check)
- `aibox setup` on Windows: WSL2 enablement, `.wslconfig` with recommended settings, Podman machine creation
- Path translation (Windows paths to WSL2 paths)
- gVisor validation inside WSL2 kernel
- `aibox doctor` WSL2-specific checks: WSL2 version, allocated memory, Podman machine status, kernel version
- **Dedicated WSL2 polyglot validation**: test full stack (.NET + JVM + Node + Bazel) under WSL2 + Podman + gVisor
- Expanded `.wslconfig` recommendations:
  - Standard workloads: 16GB memory, 4GB swap
  - Polyglot workloads: 24GB memory, 8GB swap
- Create and maintain WSL2 known-issues registry

**Spec references**: Section 7, Section 18.2, Section 18.3

### Work Stream 6: gVisor Compatibility Matrix (1-1.5 weeks)

Test representative workloads under gVisor and document results:

| Component | Risk Level | What to Test |
|-----------|-----------|-------------|
| Java (JDK 21) | Medium | JIT, `clone3`, full Gradle build |
| Node.js | Medium | V8 engine, npm install, webpack build |
| .NET CLR | High | Memory-mapped files, thread pool, JIT patterns, `dotnet build` |
| Bazel sandboxing | High | Sandbox-within-gVisor-sandbox, `--spawn_strategy=local` fallback |
| PowerShell Core | Medium | Uses .NET CLR internally, additional syscall surface |
| Gradle daemon | Medium | Long-lived JVM with file watchers and IPC |
| Combined stack | High | JVM + .NET + Node simultaneously under gVisor memory pressure |
| Python | Low | pip install with C extensions |
| Git operations | Low | Clone of large repo (>1GB) |

Document any failures and `runc` escape hatch with compensating controls.

### Work Stream 7: Benchmarking & Documentation (0.5-1 week)

- Benchmark suite: cold start, warm start, reconnect, file I/O, build times
- Use `hyperfine` for CLI timing, `fio` for I/O
- Compare: runc baseline -> gVisor overhead -> WSL2 additional overhead
- CLI reference, setup guide (Linux + Windows), troubleshooting guide

---

## Research Required

### R1: gVisor Compatibility with Target Workloads
Run compatibility matrix (Work Stream 6). This is the highest-risk research item. 2-3 days of focused testing per platform (native Linux, WSL2).

### R2: WSL2 + Podman + gVisor Stability
Triple-layer virtualization (Hyper-V -> WSL2 -> Podman -> gVisor). Test on Windows 11 with 32GB RAM. Verify `systrap` platform works through WSL2's virtual kernel. Measure performance overhead. 3-4 days.

### R3: Performance Benchmarking
Define metrics, automate benchmarks, establish baselines. Ongoing throughout phase, 2 days initial setup.

---

## Open Questions

- **Q1**: What is the fallback when gVisor is incompatible with a specific workload? Options: (a) refuse to start (most secure), (b) fall back to runc with compensating controls + audit entry. -- *Security team*

- **Q2**: How should the CLI handle Podman version skew (4.x vs 5.x)? Recommend requiring Podman 5.x minimum. -- *Platform team*

- **Q3**: Should `aibox start` auto-clone a repo if workspace path doesn't exist? Recommend deferring to Phase 3 (needs Git auth). Phase 1 assumes workspace directory exists. -- *Developer experience lead*

---

## Dependencies

### From Phase 0

| Dependency | What Phase 1 Needs | Blocking? |
|-----------|-------------------|-----------|
| Harbor registry running | Base image pullable | Yes |
| Image signing operational | Signature verification via `policy.json` | Yes |
| Base image built | Ubuntu 24.04 base + variants including dotnet | Yes |
| Nexus mirrors configured | Not required (no network in containers yet) | No |

### External Dependencies

| Dependency | Source | Risk |
|-----------|--------|------|
| Podman 5.x packages | Fedora/Ubuntu repos | Low |
| gVisor (runsc) binary | gVisor GitHub releases | Low |
| AppArmor on target systems | Ubuntu 24.04 | Low (default on Ubuntu, medium on others) |
| WSL2 kernel 5.15+ | Microsoft | Medium |
| Developer machines (16GB+ RAM, SSD) | IT procurement | Medium |

---

## Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|-----------|--------|-----------|
| R1 | gVisor breaks developer tooling (.NET CLR, Bazel sandbox, Java debugger, Node profiler) | Medium | High | Run compatibility matrix early (WS6). Maintain `runc` escape hatch with compensating controls + mandatory audit entry. |
| R2 | WSL2 + Podman + gVisor instability | Medium | High | Dedicated WSL2 validation sprint (WS5). Document minimum kernel version. Hyper-V backend as Podman machine fallback. |
| R3 | Seccomp profile too restrictive | Medium | Medium | Start with Docker default, tighten iteratively via `strace` on real workloads. |
| R4 | Cold start exceeds 90s SLA | Low | Medium | Image layer optimization, pre-pull during `aibox setup`, lazy-pulling if available. |
| R5 | AppArmor unavailable on some machines | Low | Medium | Detect at `aibox setup`. Warn but don't block (gVisor + seccomp still provide strong isolation). |
| R6 | Combined polyglot stack exceeds memory under gVisor | Medium | High | Document polyglot resource profile (24GB). Test combined-stack scenario explicitly in WS6. |

---

## Exit Criteria

All of the following must pass before Phase 1 is complete:

1. **Functional**: `aibox setup && aibox start --workspace ~/project` produces a running gVisor-isolated container on both native Linux and Windows 11 + WSL2.

2. **Shell access**: `aibox shell` drops into bash with correct mount layout.

3. **Security hardened**: Zero capabilities, `no-new-privileges`, seccomp blocking `ptrace`/`mount`/`bpf`, AppArmor loaded, read-only root, non-root user (UID 1000).

4. **Filesystem correct**: `/workspace` writable, `/home/dev` writable and persistent, `/tmp` tmpfs with `noexec`, root read-only, NTFS workspaces blocked.

5. **Persistence works**: Build caches persist across stop/start. Home survives container recreation.

6. **Diagnostics work**: `aibox doctor` reports all-green on correct config, identifies failures with remediation on misconfigured machine.

7. **Performance**: Cold start < 90s at p95, warm start < 15s at p95.

8. **WSL2 validated**: Full test pass on Windows 11 with WSL2, including polyglot stack (.NET + JVM + Node + Bazel).

9. **gVisor compatibility documented**: Matrix tested for all target workloads. Failures documented with workarounds.

10. **Validation checks pass** (absorbed from Phase 1.5):
    - `capsh --print` shows zero capabilities
    - Write to `/` fails, write to `/workspace` succeeds
    - `cat /proc/1/attr/current` shows AppArmor loaded
    - `make test-unit`, `make test-integration`, `make test-security` pass

11. **Documentation**: CLI reference, setup guide (Linux + Windows), troubleshooting guide, WSL2 known-issues registry.

12. **Security review passed**: Seccomp, AppArmor, and container launch config reviewed by security team.

---

## Estimated Effort

| Work Stream | Effort | Parallelizable? |
|-------------|--------|----------------|
| CLI skeleton (WS1) | 1.5 weeks | Yes |
| Podman + gVisor integration (WS2) | 1.5 weeks | Yes (after CLI has `start` stub) |
| Security profiles (WS3) | 1 week | Partially (needs running container) |
| Filesystem layout & caches (WS4) | 1 week | Partially (needs running container) |
| WSL2 support & polyglot validation (WS5) | 1.5-2 weeks | After Podman integration works on Linux |
| gVisor compatibility matrix (WS6) | 1-1.5 weeks | Partially parallel with WS5 |
| Benchmarking & documentation (WS7) | 0.5-1 week | After implementation complete |

**Total**: 6-8 engineer-weeks with 2-3 engineers working in parallel.

**Team allocation**:
- **Engineer A**: CLI skeleton + Podman/gVisor integration (3 weeks)
- **Engineer B**: Security profiles + filesystem layout (2 weeks), then WSL2 validation (1.5 weeks)
- **Engineer C** (part-time): gVisor compatibility matrix, benchmarking, documentation

**Calendar time**: ~3-4 weeks with 2 full-time + 1 part-time engineer.
