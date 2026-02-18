# Phase 1: Core Runtime & CLI

**Version**: 1.0
**Date**: 2026-02-18
**Status**: Draft
**Phase Dependencies**: Phase 0 (Infrastructure Foundation) must be complete
**Estimated Calendar**: Weeks 3-5 (after Phase 0)

---

## Overview

Phase 1 delivers the foundational container runtime and the `aibox` CLI that developers interact with daily. By the end of this phase, a developer can run `aibox setup && aibox start --workspace ~/project && aibox shell` and land inside a gVisor-isolated, security-hardened container with the correct filesystem layout, persistent build caches, and all mandatory security controls applied.

This phase does **not** include network security (Phase 2), credential injection (Phase 3), or IDE integration (Phase 4). The container produced here has no outbound network access and no injected credentials -- it is a locked-down shell environment. Subsequent phases layer security and developer experience on top of this foundation.

The CLI is the single abstraction layer that shields developers from infrastructure complexity. Every subsequent phase extends the CLI rather than introducing new interfaces. Getting this right is critical for adoption.

---

## Deliverables

1. **`aibox` CLI binary** -- A statically-linked CLI with commands: `setup`, `start`, `stop`, `shell`, `status`, `update`, `doctor`. Must detect host OS (native Linux vs Windows 11/WSL2) and adapt behavior.
   - *Acceptance*: `aibox --help` displays all commands. Binary runs on x86_64 Linux and inside WSL2 without additional dependencies.

2. **Podman + gVisor integration** -- Containers launch with `--runtime=runsc` by default. Docker fallback supported when Podman is unavailable.
   - *Acceptance*: `aibox start` produces a container where `/proc/self/status` shows the process running under gVisor's Sentry. `podman inspect` confirms `runsc` runtime.

3. **Seccomp profile** (`/etc/aibox/seccomp.json`) -- Allowlist-based (default-deny) seccomp profile blocking `ptrace`, `mount`, `umount2`, `pivot_root`, `chroot`, `bpf`, `userfaultfd`, `unshare`, `setns`, `init_module`, `finit_module`, `kexec_load`, `keyctl`, `add_key`, plus all other syscalls not required by development tooling.
   - *Acceptance*: Attempting a blocked syscall (e.g., `ptrace`) from inside the container returns `EPERM`. Profile passes `seccomp-tools` validation.

4. **AppArmor profile** (`aibox-sandbox`) -- Enforces read-only root, writable `/workspace` and `/tmp`, denies access to sensitive paths (`/home`, `/root`, `/.ssh`, `docker.sock`, `/proc/*/mem`, `/proc/kcore`).
   - *Acceptance*: `cat /proc/1/attr/current` inside the container shows `aibox-sandbox` profile loaded. Writing to `/etc/` fails. Writing to `/workspace` succeeds.

5. **Mandatory security flags** -- Every container launch applies: `--cap-drop=ALL`, `--security-opt=no-new-privileges:true`, `--read-only`, `--user=1000:1000`, plus the seccomp and AppArmor profiles above.
   - *Acceptance*: `capsh --print` inside the container shows zero capabilities. Attempting `setuid` execution fails. Root filesystem writes fail.

6. **Filesystem mount layout** -- Implements the spec's mount table:
   - `/` -- read-only (image filesystem)
   - `/workspace` -- rw,nosuid,nodev (bind mount or volume)
   - `/home/dev` -- rw,nosuid,nodev (named volume, persistent)
   - `/opt/toolpacks` -- rw,nosuid,nodev (named volume, for Phase 4 tool packs)
   - `/tmp` -- tmpfs, rw,noexec,nosuid, size=2G
   - `/var/tmp` -- tmpfs, rw,noexec,nosuid, size=1G
   - *Acceptance*: `mount` output inside the container matches the layout above. `findmnt` confirms mount options. Files written to `/home/dev` persist across `stop`/`start` cycles. Files in `/tmp` do not.

7. **Build cache persistence** -- Named volumes for Maven, Gradle, npm, Bazel caches that survive container recreation.
   - *Acceptance*: Run a build, `aibox stop && aibox start`, run the same build -- second build uses cached artifacts and completes faster.

8. **WSL2 detection and setup automation** -- `aibox setup` on Windows 11 configures WSL2, creates Podman machine, applies `.wslconfig` optimization, and validates the environment.
   - *Acceptance*: On a clean Windows 11 machine, `aibox setup` produces a working environment. `aibox doctor` reports all-green.

9. **`aibox doctor` diagnostics** -- Health check command verifying: Podman installed, gVisor available, WSL2 memory allocation (if applicable), image signature valid, disk space sufficient.
   - *Acceptance*: Deliberately misconfigure gVisor. `aibox doctor` reports the specific failure with remediation guidance.

10. **Performance benchmarks validated** -- Cold start < 90s, warm start < 15s.
    - *Acceptance*: Automated benchmark script measures start times across 10 runs. p95 meets SLA targets.

---

## Implementation Steps

### Work Stream 1: `aibox` CLI Skeleton

**What to build**:
- CLI framework with subcommand routing: `setup`, `start`, `stop`, `shell`, `status`, `update`, `doctor`
- Configuration file format and default locations (`~/.config/aibox/config.yaml` on Linux, equivalent on Windows)
- Host OS detection logic (native Linux, WSL2, macOS unsupported with clear error)
- Runtime detection logic (Podman preferred, Docker fallback)
- Structured logging (JSON for machine consumption, human-readable for terminal)
- Version management and self-update stub (actual update mechanism in later phase)

**Key decisions**:
- Language choice: Go vs Rust (see Research Required section below)
- Configuration format: YAML for human authoring, JSON for machine interchange
- Distribution: single static binary, no runtime dependencies
- Workspace specification: `--workspace` flag accepts absolute paths; CLI resolves and validates

**Spec references**: Section 5.6, Appendix A, Appendix B

### Work Stream 2: Podman + gVisor Integration

**What to build**:
- Container launch logic: translate `aibox start` flags into the correct `podman run` invocation
- gVisor runtime configuration: install `runsc` binary, register as OCI runtime in Podman config (`/etc/containers/containers.conf` or user-level config)
- Docker fallback: detect when Podman is absent, fall back to Docker with rootless mode validation
- Container lifecycle management: start, stop, restart, destroy, status query
- Container naming convention: `aibox-<username>-<workspace-hash>` for multi-workspace support
- Image pull with signature verification: leverage Phase 0's `/etc/containers/policy.json`

**Key decisions**:
- Use `podman` CLI invocation vs Podman Go bindings (libpod). CLI invocation is simpler and avoids tight coupling to Podman internals. Go bindings are faster but require version-matched library.
- Container networking mode for Phase 1: `--network=none` (no network until Phase 2 adds proxy routing). This is the most secure default.
- gVisor platform: `systrap` (default, best performance on modern kernels) vs `ptrace` (broader compatibility, slower)

**Spec references**: Section 5.1, Section 7

### Work Stream 3: Security Profiles

**What to build**:
- **Seccomp profile** (`/etc/aibox/seccomp.json`): Start from Docker's default seccomp profile, then tighten. Block the specific syscalls listed in spec Section 9.2. Validate against real workloads (Java builds, Node builds, Python execution, Git operations) to ensure nothing breaks.
- **AppArmor profile** (`aibox-sandbox`): Implement the profile from spec Section 9.3. Install and load via `apparmor_parser`. Handle systems without AppArmor gracefully (warn, continue with reduced protection on distros that use SELinux).
- **Mandatory container flags**: Codify the flag set from spec Section 9.4 so they cannot be omitted. The CLI must refuse to launch a container without these flags.

**Key decisions**:
- Seccomp profile must be validated against all target tool stacks before locking down. A syscall used by `javac` or `node` that is accidentally blocked will break the developer experience. Strategy: run workloads under `strace` to capture required syscalls, then build an allowlist.
- AppArmor vs SELinux: AppArmor is specified in the spec and is native to Ubuntu 24.04 (the base image OS). SELinux support is a non-goal for Phase 1 but should not be architecturally excluded.
- Profile distribution: profiles ship inside the base image (baked in at build time) and are also available via the `aibox-policies` Git repository for host-side loading.

**Spec references**: Section 9.1, 9.2, 9.3, 9.4

### Work Stream 4: Filesystem Layout & Build Caches

**What to build**:
- Mount point configuration: translate the spec's mount table into `podman run` volume and tmpfs flags
- Workspace handling: validate workspace path is on a native Linux filesystem (ext4/xfs/btrfs), block NTFS-mounted paths with a clear error message
- Named volume management: create, list, prune named volumes for build caches (`aibox-maven-cache`, `aibox-gradle-cache`, `aibox-npm-cache`, `aibox-bazel-cache`)
- Home volume: persistent `/home/dev` volume that survives container recreation
- Toolpack volume: `/opt/toolpacks` placeholder volume (used in Phase 4)
- `aibox repair cache` command: allows clearing and rebuilding cache volumes

**Key decisions**:
- Bind mount vs named volume for `/workspace`: Bind mount gives the developer direct host filesystem access to the workspace (they can edit files from the host). Named volume isolates files inside Podman storage. Bind mount is the pragmatic choice for local-first (matches spec language "bind mount or clone").
- NTFS detection on WSL2: check `/proc/mounts` for `9p` or `drvfs` filesystem type on the workspace path. These indicate a Windows NTFS mount.
- Volume naming: include username to support multi-user machines. Format: `aibox-<username>-<purpose>`.

**Spec references**: Section 10.1, 10.2, 10.3, 10.4

### Work Stream 5: WSL2 Support

**What to build**:
- WSL2 detection: determine if running inside WSL2 vs native Linux
- `aibox setup` on Windows: automate WSL2 enablement (check `wsl --status`), configure `.wslconfig` with recommended settings (16GB RAM, 8 processors, 4GB swap), create Podman machine
- Podman machine management: `podman machine init` and `podman machine start` with correct settings
- Path translation: convert Windows paths (`C:\Users\...`) to WSL2 paths (`/mnt/c/Users/...`) and warn when workspace is on Windows filesystem
- gVisor inside WSL2: validate `runsc` works inside the WSL2 kernel (Linux 5.15+ in WSL2 should support it, but needs testing)
- `aibox doctor` WSL2-specific checks: WSL2 version, allocated memory, Podman machine status, kernel version compatibility

**Key decisions**:
- Podman machine backend: WSL2 backend (default on Windows) vs Hyper-V backend. WSL2 is the recommended path per the spec. Hyper-V is a potential fallback if WSL2 + gVisor proves unstable.
- The `aibox` CLI itself runs inside WSL2 (installed via the WSL2 distro's package manager), not as a native Windows binary. A thin Windows-side wrapper or PowerShell alias can invoke it from the Windows terminal.
- `.wslconfig` modification: `aibox setup` should modify `.wslconfig` only with user consent (prompt before changing), as other tools may depend on current settings.

**Spec references**: Section 7 (Windows 11 Setup), Section 18.3 (Developer Machine Requirements)

---

## Research Required

### R1: Language Choice for CLI (Go vs Rust)

| Factor | Go | Rust |
|--------|-----|------|
| **Podman ecosystem** | Podman is written in Go. `libpod` bindings available. Containers/image libraries in Go. | Must shell out to `podman` CLI or use REST API. No native Podman library. |
| **Static binary** | `CGO_ENABLED=0` produces fully static binary. Easy cross-compilation. | Static linking via musl. Slightly more complex cross-compilation. |
| **Build speed** | Fast (seconds). Good developer iteration speed. | Slower (minutes for full builds). |
| **OPA/Rego integration** | OPA is written in Go. Can embed the OPA engine as a library (relevant for Phase 3). | Must use OPA REST API or WASM-compiled Rego. |
| **Performance** | Adequate for CLI. GC pauses irrelevant for short-lived commands. | Slightly faster, but CLI is not performance-critical. |
| **Team familiarity** | Broader adoption in platform/infrastructure tooling. | Growing in security-critical tooling (ripgrep, nushell). |
| **Error handling** | Verbose but explicit (`if err != nil`). | Type-system enforced (`Result<T, E>`). More robust at compile time. |
| **Binary size** | ~10-20MB typical | ~5-10MB typical |

**Recommendation**: Go is the pragmatic choice. The Podman ecosystem is Go-native, OPA embeds as a Go library, cross-compilation is trivial, and platform/infrastructure teams are more likely to have Go experience. Rust is technically superior for safety-critical code but introduces friction with the Podman ecosystem.

**Action**: Decide before sprint 1 starts. Prototype the `podman run` invocation in both languages (1 day each) to validate integration ergonomics.

### R2: gVisor Compatibility with Target Workloads

gVisor's user-space kernel implements ~400 of Linux's ~450 syscalls, but some implementations are incomplete or have behavioral differences. Known risk areas:

- **Java (JDK 21)**: JIT compiler uses `mmap` with exotic flags, `clone3` for threading. gVisor added `clone3` support in 2023 -- verify with JDK 21 specifically.
- **Node.js**: V8 engine uses `mmap`, `madvise`, `futex` extensively. Generally works, but `--enable-source-maps` and certain debug modes may use unsupported `ptrace` features.
- **Gradle/Maven builds**: Fork-heavy. Test full builds of representative projects (largest internal project).
- **Bazel**: Uses sandboxing features that may conflict with gVisor's sandboxing. Test `bazel build` and `bazel test` with `--sandbox_debug`.
- **Git operations**: Generally compatible. Test `git clone` of a large repository (>1GB).
- **Python**: Generally compatible. Test `pip install` with C extensions that compile native code.

**Action**: Create a compatibility test matrix. Run representative builds for each target stack under gVisor. Document any failures and workarounds. Timeline: 2-3 days of focused testing.

### R3: WSL2 + Podman + gVisor Stability

This is a triple-layer virtualization stack (Hyper-V -> WSL2 Linux kernel -> Podman -> gVisor). Risk areas:

- **Nested virtualization**: gVisor's `systrap` platform requires certain CPU features. Verify these are exposed through WSL2's virtual kernel.
- **Memory pressure**: WSL2 has a fixed memory allocation. Podman machine adds another layer. A 16GB allocation with gVisor overhead may leave insufficient memory for builds.
- **Filesystem performance**: WSL2's ext4 vhdx is fast, but Podman's overlay storage adds a layer. Benchmark file I/O.
- **Podman machine**: `podman machine` on WSL2 creates an additional Fedora CoreOS distribution. This adds complexity but is the official supported path.

**Action**: Set up a test rig on a Windows 11 machine with 32GB RAM. Run the full compatibility matrix (R2) inside WSL2. Measure performance overhead vs native Linux. Document failure modes. Timeline: 3-4 days.

### R4: Performance Benchmarking Approach

**Metrics to measure**:
- Cold start time (no cached image layers)
- Warm start time (image cached, container stopped)
- Hot reconnect time (container running, CLI reconnects)
- File I/O throughput (sequential and random)
- Build time for reference projects (Java/Gradle, Node/webpack, Python/pip)
- Memory overhead of gVisor vs runc

**Benchmarking approach**:
- Use `hyperfine` for CLI operation timing
- Use `fio` for file I/O benchmarks inside the container
- Use representative project builds with build cache cold and warm
- Compare: runc baseline -> gVisor overhead -> WSL2 additional overhead
- Automate benchmarks so they can run in CI and track regressions

**Action**: Define benchmark suite as part of Phase 1 development. Run baseline benchmarks on native Linux with runc, then measure each layer of overhead. Timeline: ongoing throughout phase, 2 days for initial setup.

---

## Open Questions

**Q1**: Should the `aibox` CLI binary run natively on Windows (Go/Rust cross-compiled for Windows) or only inside WSL2?
*Who should answer*: Platform team + developer experience lead. Running inside WSL2 simplifies the codebase (Linux-only) but adds a step for Windows developers. A thin Windows wrapper that invokes the WSL2 binary could bridge this.

**Q2**: What is the fallback behavior when gVisor is unavailable or incompatible with a specific workload?
*Who should answer*: Security team. Options: (a) refuse to start (most secure), (b) fall back to runc with compensating controls and an audit log entry (most flexible), (c) offer Kata Containers as an alternative isolation layer. The spec says gVisor is the default but does not explicitly address failure modes.

**Q3**: Should `aibox start` automatically `git clone` the workspace repo into the container if the workspace path does not exist?
*Who should answer*: Developer experience lead. This simplifies the first-run experience but introduces complexity around Git authentication (which is Phase 3). Phase 1 could assume the workspace directory already exists on the host.

**Q4**: How should the CLI handle Podman version skew? Podman 4.x vs 5.x have behavioral differences, especially in rootless networking (`slirp4netns` vs `pasta`).
*Who should answer*: Platform team. Options: (a) require Podman 5.x minimum, (b) detect version and adapt behavior, (c) bundle Podman in the `aibox` distribution.

**Q5**: Should the seccomp profile be validated against actual workloads before or after the CLI is functional?
*Who should answer*: Platform team lead. Sequencing question: building the seccomp profile requires running workloads, which requires a working container launch. Suggest: launch with Docker's default seccomp first, then tighten iteratively using `strace` data.

**Q6**: What is the expected behavior when `/workspace` is empty (no source code present)?
*Who should answer*: Developer experience lead. Options: (a) start with an empty workspace (user clones manually), (b) prompt for a repo URL, (c) refuse to start. Spec does not specify.

---

## Dependencies

### From Phase 0

| Dependency | What Phase 1 Needs | Blocking? |
|-----------|-------------------|-----------|
| Harbor registry running | Base image pullable: `harbor.internal/aibox/base:24.04` | Yes -- cannot start containers without images |
| Image signing operational | `podman pull` must succeed with signature verification via `/etc/containers/policy.json` | Yes -- unsigned images must be rejected |
| Base image built | Ubuntu 24.04 base image with aibox-agent stub, SSH server, essential tools | Yes -- this is the container we launch |
| Nexus mirrors configured | Not required for Phase 1 (no network access in containers yet) | No |

### External Dependencies

| Dependency | Source | Risk |
|-----------|--------|------|
| Podman 5.x packages | Fedora/Ubuntu repos or upstream | Low -- well-maintained, stable releases |
| gVisor (runsc) binary | gVisor GitHub releases | Low -- active project, regular releases. Verify license (Apache 2.0) is compatible. |
| AppArmor on target systems | Ubuntu 24.04 (host OS for developers or WSL2) | Low -- AppArmor is default on Ubuntu. Medium risk on non-Ubuntu hosts. |
| WSL2 kernel version | Microsoft | Medium -- WSL2 kernel updates are controlled by Microsoft. Need kernel 5.15+ for gVisor `systrap`. |
| Developer machines meeting hardware requirements | IT procurement | Medium -- 16GB RAM minimum, SSD required. Machines not meeting spec will have degraded experience. |

---

## Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|-----------|--------|-----------|
| R1 | gVisor breaks specific developer tooling (Java debugger, Bazel sandbox, Node profiler) | Medium | High | Run compatibility matrix early (Research R2). Maintain an "escape hatch" to runc with compensating controls and mandatory audit entry. Document known incompatibilities. |
| R2 | WSL2 + Podman + gVisor instability on Windows | Medium | High | Dedicate 3-4 days to WSL2 testing (Research R3). Document minimum WSL2 kernel version. Have Hyper-V backend as fallback for Podman machine. |
| R3 | Seccomp profile too restrictive, breaks normal development operations | Medium | Medium | Start with Docker's default profile, tighten iteratively. Run `strace` on representative workloads to identify required syscalls before locking down. |
| R4 | Cold start time exceeds 90s SLA | Low | Medium | Image pull is the bottleneck. Mitigate with image layer optimization (shared base layers), pre-pulling images during `aibox setup`, and lazy-pulling (eStargz/zstd:chunked) if available in Podman 5.x. |
| R5 | CLI language choice delays development | Low | Low | Make decision in sprint 0 (before Phase 1 starts). 1-day prototype in each language to validate Podman integration. Do not bikeshed. |
| R6 | AppArmor unavailable on some developer machines (non-Ubuntu, custom kernels) | Low | Medium | Detect AppArmor availability at `aibox setup` time. Warn if unavailable but do not block (gVisor + seccomp still provide strong isolation). Document as reduced-security mode. |
| R7 | Named volume data corruption across Podman upgrades | Low | Medium | Document tested Podman version matrix. Include volume backup/restore in `aibox` CLI. Test volume migration path. |

---

## Exit Criteria

All of the following must be demonstrated before Phase 1 is considered complete:

1. **Functional**: `aibox setup && aibox start --workspace ~/project` produces a running gVisor-isolated container on both native Linux and Windows 11 + WSL2.

2. **Shell access**: `aibox shell` drops the user into a bash shell inside the container with the correct mount layout visible (`mount` output matches spec).

3. **Security hardened**: Container has zero Linux capabilities, `no-new-privileges` set, seccomp profile loaded and blocking `ptrace`/`mount`/`bpf`, AppArmor profile loaded, read-only root filesystem, non-root user (UID 1000).

4. **Filesystem correct**: `/workspace` is writable, `/home/dev` is writable and persistent, `/tmp` is tmpfs with `noexec`, root filesystem is read-only, NTFS-mounted workspaces are blocked with a clear error.

5. **Persistence works**: Build cache volumes persist across `aibox stop && aibox start` cycles. Home directory contents survive container recreation (image update).

6. **Diagnostics work**: `aibox doctor` reports all-green on a correctly configured machine and identifies specific failures (with remediation) on a misconfigured machine.

7. **Performance meets SLA**: Cold start < 90s at p95 (image cached locally), warm start < 15s at p95, measured across 10 runs on reference hardware.

8. **WSL2 tested**: Full test pass on Windows 11 with WSL2, including gVisor runtime verification.

9. **Documentation**: CLI reference, setup guide (Linux + Windows), and troubleshooting guide published.

10. **Security review passed**: Seccomp profile, AppArmor profile, and container launch configuration reviewed and approved by security team.

---

## Estimated Effort

| Work Stream | Effort | Parallelizable? |
|-------------|--------|----------------|
| CLI skeleton (commands, config, OS detection) | 1.5 weeks | Yes -- independent |
| Podman + gVisor integration | 1.5 weeks | Yes -- independent after CLI skeleton has `start` stub |
| Security profiles (seccomp + AppArmor) | 1 week | Partially -- needs running container for validation |
| Filesystem layout & build caches | 1 week | Partially -- needs running container |
| WSL2 support & testing | 1 week | After Podman integration works on Linux |
| Performance benchmarking & optimization | 0.5 weeks | After everything else works |
| Documentation & security review | 0.5 weeks | After implementation complete |

**Total**: 5-6 engineer-weeks with 2-3 engineers working in parallel.

**Suggested team allocation**:
- **Engineer A**: CLI skeleton + Podman/gVisor integration (3 weeks)
- **Engineer B**: Security profiles + filesystem layout (2 weeks), then WSL2 testing (1 week)
- **Engineer C** (part-time): Compatibility testing (Research R2), benchmarking, documentation

**Calendar time**: ~3 weeks with 2 full-time engineers + 1 part-time.
