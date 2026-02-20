# AI-Box Code Audit Report

**Date:** 2026-02-19
**Auditor:** code-auditor (automated)
**Scope:** All code files in the repository (excluding `docs/`)

> **Note:** Polyglot gaps (dotnet, PowerShell, AngularJS) are tracked in the
> phase documents, not here.

---

## 1. Executive Summary

The AI-Box codebase is a well-structured Go CLI application backed by container
infrastructure scripts, container image definitions, OPA policies, and a tool
pack schema. The code quality is generally high with consistent style and
reasonable test coverage for a Phase 1 implementation.

However, several structural issues prevent the codebase from being
production-ready:

- **Critical:** `container.go` builds mount and security arguments inline,
  completely ignoring the dedicated `mounts` and `security` packages, creating
  two divergent sources of truth for container configuration.
- **High:** Go toolchain is not present in the development environment, so
  compilation and test execution could not be verified.
- **High:** Multiple workspace validation implementations exist with different
  capabilities (`container/workspace.go` vs `mounts/validate.go`).
- **Medium:** Infrastructure scripts contain hardcoded placeholder credentials.
- **Low:** Duplicated utility functions, empty schema templates, and minor
  inconsistencies.

**Verdict:** The foundation is solid. A focused cleanup sprint addressing the
mount/security integration and workspace validation consolidation would bring
the codebase to a shippable state for Phase 1.

---

## 2. Build & Test Status

### 2.1 Compilation

```
$ go build ./...
ERROR: go: command not found
```

**Finding:** Go 1.23.0 is not installed on the development host. The `go.mod`
file is well-formed and declares reasonable dependencies (cobra v1.10.2, viper
v1.21.0). The module path `github.com/aibox/aibox` is consistent across all
source files. No obvious import cycle issues were detected by manual inspection.

**Indirect validation:** A pre-built binary exists at `cmd/aibox/bin/aibox`,
suggesting the code has compiled successfully at some point.

### 2.2 Unit Tests

```
$ go test ./...
ERROR: go: command not found
```

Could not execute. There are **11 unit test files** covering:

| Package | Tests | Coverage Areas |
|---------|-------|----------------|
| `internal/security` | `flags_test.go`, `apparmor_test.go` | 14 tests: flag validation, AppArmor detection |
| `internal/mounts` | `validate_test.go`, `layout_test.go`, `volumes_test.go` | 22 tests: workspace FS detection, mount layout, cache volumes |
| `internal/config` | `config_test.go`, `validate_test.go` | 15 tests: config loading, env overrides, validation |
| `internal/host` | `detect_test.go` | 4 tests: OS/WSL2 detection |
| `internal/runtime` | `detect_test.go` | 3 tests: runtime probing |
| `internal/container` | `names_test.go`, `workspace_test.go` | 10 tests: naming, workspace validation |

**Assessment:** ~68 unit tests across 11 files. Coverage looks reasonable for
core packages. Notable gap: no unit tests for `container.go` (the Manager
struct and its Start/Stop/Shell/Status methods).

### 2.3 Integration Tests

There are **4 integration/security test files** (build-tag gated):

| File | Build Tag | Description |
|------|-----------|-------------|
| `tests/integration_test.go` | (none) | CLI lifecycle (start/stop/status/doctor) |
| `tests/integration/integration_test.go` | `integration` | Identical to above |
| `tests/filesystem_test.go` | (none) | Mount layout verification (read-only root, tmpfs, workspace) |
| `tests/security/security_test.go` | `security` | Runtime security (cap-drop, no-new-privs, network isolation) |
| `tests/security_test.go` | (none) | Seccomp/AppArmor profile validation |

**Finding:** `tests/integration_test.go` and `tests/integration/integration_test.go`
appear to be duplicates in different directories. The Makefile targets
`test-integration` and `test-security` point to the `tests/integration/` and
`tests/security/` subdirectories respectively, suggesting the top-level
`tests/integration_test.go` and `tests/filesystem_test.go` are orphaned or
misplaced files.

### 2.4 Benchmarks

`tests/benchmark/run.sh` uses `hyperfine` to benchmark:
- Container start time (cold)
- Container start time (warm)
- Shell exec latency
- File I/O throughput

Well-structured with JSON output option. Depends on `hyperfine` being installed.

---

## 3. Go CLI Assessment

### 3.1 Architecture

```
cmd/aibox/
  main.go              -- entry point
  cmd/                 -- cobra subcommands (root, start, stop, shell, status, doctor, setup, update, repair)
  internal/
    config/            -- viper-based config loading and validation
    container/         -- container lifecycle (Manager, workspace validation, naming)
    doctor/            -- diagnostic checks
    host/              -- OS/WSL2 detection
    logging/           -- slog configuration
    mounts/            -- mount layout, cache volumes, workspace FS validation
    runtime/           -- container runtime detection
    security/          -- security flags, AppArmor
    setup/             -- host setup (Linux, WSL2)
  configs/
    seccomp.json       -- seccomp profile
    apparmor/          -- AppArmor profile
  tests/               -- integration and security tests
```

The internal package structure follows Go conventions well. Packages are
cohesive with clear responsibilities.

### 3.2 Critical: Mount/Security Integration Gap

**Severity: Critical**

The `internal/mounts` package provides `Layout()` and `RuntimeArgs()` to
generate mount arguments. The `internal/security` package provides `BuildArgs()`
to generate security flags. However, **`container.go` uses neither package**.

`container.go:99-167` builds all mount and security arguments inline:

```go
// container.go builds mounts inline (lines 149-162)
args = append(args, "--mount", fmt.Sprintf("type=bind,source=%s,target=/workspace", workspace))
args = append(args, "--mount", fmt.Sprintf("type=volume,source=aibox-home-%s,target=/home/dev", ...))
args = append(args, "--mount", "type=volume,source=aibox-toolpacks,target=/opt/toolpacks")
args = append(args, "--tmpfs", fmt.Sprintf("/tmp:rw,noexec,nosuid,size=%s", tmpSize))
args = append(args, "--tmpfs", "/var/tmp:rw,noexec,nosuid,size=1g")
```

Meanwhile, `mounts/layout.go:32-85` builds a proper `[]Mount` slice with the
same targets plus cache volumes and proper options.

**Consequences:**
1. Mount options differ: `container.go` uses `--tmpfs` syntax while
   `mounts.RuntimeArgs()` uses `--mount type=tmpfs` syntax
2. `container.go` omits `nosuid,nodev` options on bind and volume mounts that
   `mounts.Layout()` includes
3. `container.go` omits cache volume mounts entirely (Maven, Gradle, npm, Yarn,
   Bazel)
4. Volume naming differs: `container.go` uses `aibox-home-<sanitized-username>`
   while `mounts.Layout()` uses `aibox-<username>-home`
5. Security flags in `container.go` are hardcoded inline instead of using
   `security.BuildArgs()`
6. Any future change to mount policy must be made in two places

**Current usage:**
- `mounts` package: only imported by `cmd/repair.go` (for cache cleanup)
- `security` package: only imported by `doctor/checks.go` and `setup/linux.go`
  (for validation, not container creation)

**Recommendation:** Refactor `container.go` Start() to use `mounts.Layout()` +
`mounts.RuntimeArgs()` and `security.DefaultFlags()` + `security.BuildArgs()`.

### 3.3 High: Duplicate Workspace Validation

**Severity: High**

Two implementations exist:

| Location | Signature | Capabilities |
|----------|-----------|--------------|
| `container/workspace.go` | `ValidateWorkspace(path) (string, error)` | Exists check, directory check, NTFS prefix heuristic (`/mnt/[a-z]`) |
| `mounts/validate.go` | `ValidateWorkspace(path, validateFS) error` | Exists check, directory check, `/proc/mounts` FS detection with allow/block lists |

The `mounts/validate.go` version is strictly more capable (reads actual
filesystem type from `/proc/mounts` rather than guessing from path prefix). The
`container/workspace.go` version is what `container.go` Start() actually calls.

**Recommendation:** Remove `container/workspace.go`, have `container.go` use
`mounts.ValidateWorkspace()`.

### 3.4 Medium: Duplicated Utility Functions

**`firstLine()`** is defined identically in two files:
- `internal/doctor/checks.go:408`
- `internal/setup/linux.go:271`

**`currentUsername()`** and **`sanitize()`** in `container/names.go` duplicate
functionality that `mounts.volumePrefix()` does differently (different naming
convention).

**Recommendation:** Extract `firstLine()` into a shared `internal/util` package
or inline it where used. Consolidate username/sanitization helpers.

### 3.5 Medium: Missing Unit Tests for container.go

The `container` package has tests for `names.go` and `workspace.go` but none
for `container.go` itself. The Manager struct's Start/Stop/Shell/Status methods
are only tested via integration tests which require a running container runtime.

**Recommendation:** Add unit tests using a mock runtime command or at minimum
test the argument-building logic in isolation.

### 3.6 Low: Inconsistent Container Command Syntax

`container.go:167` hardcodes `sleep infinity` as the container entrypoint:

```go
args = append(args, image, "sleep", "infinity")
```

The comment acknowledges the real aibox image has an init/SSH server, but this
is not configurable. The `aibox-images/base/Containerfile` sets up an SSH
server, which would be overridden by this `sleep infinity` command.

### 3.7 Low: Hardcoded Network Isolation

`container.go:140` hardcodes `--network=none` with comment "Phase 1". This
should be driven by configuration for Phase 2+ network policies.

### 3.8 Low: go.mod Version

Go 1.23.0 is specified. Current stable is Go 1.23.x or 1.24.x. The version
is recent enough but should be kept updated.

---

## 4. Infrastructure Assessment

### 4.1 Harbor (`infra/harbor/`)

**Quality: Good**

Well-organized set of scripts covering the full Harbor lifecycle:
- `install.sh`: Comprehensive with preflight checks, TLS, post-install validation
- `rbac-setup.sh`: Creates project, robot accounts, scan-on-push policy
- `replication-setup.sh`: DR replication to secondary instance
- `gc-schedule.sh`: Weekly garbage collection
- `docker-compose.override.yml`: Resource limits for all services

**Issues:**
1. **Medium:** `harbor.yml` contains `CHANGE_ME` placeholder passwords
   (lines 10, 95, 98). These should be parameterized with environment variables
   or a secrets manager reference.
2. **Low:** `harbor.yml:3` hardcodes `hostname: harbor.internal` which may not
   match all deployment environments. Should be templated.
3. **Low:** `install.sh` downloads a specific Harbor version (v2.12.2) without
   checksum verification of the downloaded tarball.

### 4.2 Nexus (`infra/nexus/`)

**Quality: Good**

- `docker-compose.yml`: Production-ready with JVM tuning, healthcheck, volume persistence
- `configure-repos.sh`: Creates proxy+hosted+group repos for npm, Maven, Gradle,
  PyPI, NuGet, Go, Cargo
- `cleanup-policies.sh`: 90-day eviction with weekly task scheduling
- `test-mirrors.sh`: Integration tests for all mirror formats

**Issues:**
1. **Low:** `configure-repos.sh` notes Cargo proxy support is limited in Nexus.
   No workaround documented.
2. **Low:** `nexus.properties` enables `nexus.scripts.allowCreation=true` which
   is a security consideration (allows arbitrary Groovy scripts via API).

### 4.3 Cosign (`infra/cosign/`)

**Quality: Good**

Complete signing pipeline:
- `setup-keys.sh`: Key generation with overwrite protection
- `verify-image.sh` / `test-verification.sh`: TAP-format test suite
- `policy.json`: Podman policy requiring signatures from Harbor
- `install-policy.sh`: System-level policy installation with backup

**Issues:**
1. **Low:** `setup-keys.sh` reads the Cosign password from `COSIGN_PASSWORD` env
   var or prompts interactively. No integration with a secrets manager for CI.

### 4.4 Infrastructure Summary

The infrastructure scripts are well-written and operationally sound. The main
gap is credential management (hardcoded placeholders in Harbor, env-var
passwords in Cosign). For a Phase 1 deployment this is acceptable with the
understanding that secrets management will be addressed before production.

---

## 5. Container Images Assessment

### 5.1 Base Image (`aibox-images/base/`)

**Quality: Good**

- Ubuntu 24.04 base with comprehensive dev tools
- Non-root user `dev` (UID 1000)
- SSH server with key-only auth, no root login
- `yq` for YAML processing
- `policy-default.yaml` placeholder for network policy

**Issues:**
1. **Low:** `sshd_config` allows `AllowTcpForwarding yes` which enables SSH
   tunneling. This may conflict with `--network=none` Phase 1 policy.

### 5.2 Language Variants

| Variant | Stack | Status |
|---------|-------|--------|
| `java/Containerfile` | JDK 21 + Maven 3.9.9 + Gradle 8.12 | Complete |
| `node/Containerfile` | Node.js 20 LTS + Yarn | Complete |
| `full/Containerfile` | Java + Node + Python venv + Poetry | Complete |

### 5.3 CI Pipelines

- `build-and-publish.yml`: GitHub Actions with lint, build, sign, scan matrix
- `weekly-rebuild.yml`: Weekly `--no-cache` rebuild for fresh base layers

**Issues:**
1. **Low:** `build.sh` uses `buildah` which may not be available in all CI
   environments. No fallback to `docker build`.

### 5.4 Scripts

- `build.sh`: Build + tag + push via Buildah
- `sign.sh`: Cosign sign + verify
- `scan-check.sh`: Trivy scan with CRITICAL=0 threshold
- `lint.sh`: hadolint for all Containerfiles

Well-structured and consistent.

---

## 6. Policies & Toolpacks Assessment

### 6.1 OPA Policy (`aibox-policies/org/baseline.rego`)

**Quality: Good**

Enforces:
- gVisor runtime requirement
- No wildcard network access
- Signed images only
- LLM rate limiting (100 req/min, 10k tokens/req)
- Non-root, read-only rootfs, no capabilities

**Issues:**
1. **Low:** Policy references `input.container.runtime` which must match the
   container runtime's inspection output format. No documentation on how OPA
   integrates with the container lifecycle.

### 6.2 Organization Policy (`aibox-policies/org/policy.yaml`)

**Quality: Good**

Comprehensive YAML policy covering:
- Network allow rules (npm, Maven, PyPI, NuGet, Go, Cargo, Rust registries)
- Credential TTLs (4h short, 24h long)
- Tool risk classification (low/medium/high/critical)
- Resource limits per tier

**Issues:**
1. **Medium:** No `.NET`/NuGet-specific network rules listed explicitly (though
   `nuget.org` appears in the allowed registries).

---

## 7. Recommended Actions

### 7.1 Priority 1 (Must Fix -- Structural)

| # | Action | Files | Effort |
|---|--------|-------|--------|
| 1 | Refactor `container.go` Start() to use `mounts.Layout()` + `mounts.RuntimeArgs()` instead of inline mount building | `internal/container/container.go` | Medium |
| 2 | Refactor `container.go` Start() to use `security.DefaultFlags()` + `security.BuildArgs()` instead of inline security flags | `internal/container/container.go` | Medium |
| 3 | Remove `container/workspace.go` and use `mounts.ValidateWorkspace()` in container.go | `internal/container/workspace.go`, `internal/container/container.go` | Small |
| 4 | Reconcile volume naming: `container.go` uses `aibox-home-<user>` while `mounts` uses `aibox-<user>-home` | `internal/container/container.go`, `internal/mounts/layout.go` | Small |

### 7.2 Priority 2 (Should Fix -- Quality)

| # | Action | Files | Effort |
|---|--------|-------|--------|
| 5 | Extract duplicated `firstLine()` into shared utility or inline | `internal/doctor/checks.go`, `internal/setup/linux.go` | Small |
| 6 | Add unit tests for `container.go` Manager methods (at least argument building) | `internal/container/container_test.go` (new) | Medium |
| 7 | Parameterize Harbor credentials (replace CHANGE_ME placeholders) | `infra/harbor/harbor.yml` | Small |
| 8 | Add NuGet and pip cache volumes to `mounts/volumes.go` CacheVolumes() | `internal/mounts/volumes.go` | Small |
| 9 | Make container entrypoint configurable (not hardcoded `sleep infinity`) | `internal/container/container.go` | Small |
| 10 | Make `--network=none` configurable for Phase 2+ | `internal/container/container.go`, `internal/config/config.go` | Small |

### 7.3 Priority 3 (Nice to Have)

| # | Action | Files | Effort |
|---|--------|-------|--------|
| 11 | Fix `wsl2.go:92` inconsistent indentation | `internal/setup/wsl2.go` | Trivial |
| 12 | Add checksum verification to Harbor installer | `infra/harbor/install.sh` | Small |
| 13 | Document OPA policy integration with container lifecycle | New docs file | Medium |
| 14 | Install Go toolchain in development environment | System setup | Small |

---

## 8. Files to Delete

| File | Reason |
|------|--------|
| `cmd/aibox/internal/container/workspace.go` | Duplicates `mounts/validate.go` with less functionality. After refactoring container.go to use `mounts.ValidateWorkspace()`, this file is dead code. |
| `cmd/aibox/internal/container/workspace_test.go` | Tests for the removed file. Equivalent coverage exists in `mounts/validate_test.go`. |
| `cmd/aibox/tests/integration_test.go` | Duplicate of `tests/integration/integration_test.go`. Makefile targets point to the `integration/` subdirectory. |
| `cmd/aibox/tests/filesystem_test.go` | Orphaned in top-level `tests/` but not referenced by any Makefile target. Should be moved to `tests/integration/` or deleted. |
| `cmd/aibox/bin/aibox` | Pre-built binary should not be checked into version control. Add `bin/` to `.gitignore`. |

---

## 9. Files to Modify

| File | Changes Needed |
|------|----------------|
| `cmd/aibox/internal/container/container.go` | (P1) Replace inline mounts with `mounts.Layout()` + `mounts.RuntimeArgs()`. Replace inline security flags with `security.BuildArgs()`. Use `mounts.ValidateWorkspace()` instead of `container.ValidateWorkspace()`. Make entrypoint and network mode configurable. |
| `cmd/aibox/internal/mounts/volumes.go` | (P2) Add NuGet cache (`/home/dev/.nuget/packages`) and pip cache (`/home/dev/.cache/pip`) volumes. |
| `cmd/aibox/internal/mounts/layout.go` | (P1) Verify volume naming matches what container.go expects after refactor. |
| `cmd/aibox/internal/doctor/checks.go` | (P2) Remove `firstLine()`, import shared utility. |
| `cmd/aibox/internal/setup/linux.go` | (P2) Remove `firstLine()`, import shared utility. |
| `cmd/aibox/internal/config/config.go` | (P2) Add `Network.Mode` config field for Phase 2 network policy support. |
| `infra/harbor/harbor.yml` | (P2) Replace `CHANGE_ME` passwords with `${HARBOR_DB_PASSWORD}` etc. |

---

## 10. Files to Create

| File | Purpose |
|------|---------|
| `cmd/aibox/internal/container/container_test.go` | (P2) Unit tests for Manager.Start() argument building, containerState(), findRunningContainer(). |
| `cmd/aibox/.gitignore` | (P3) Add `bin/` directory to prevent committing built binaries. |

---

## 11. Dead Code and Unused Exports

| Symbol | Location | Status |
|--------|----------|--------|
| `mounts.Layout()` | `internal/mounts/layout.go:19` | Not used by container.go -- should be after refactor |
| `mounts.RuntimeArgs()` | `internal/mounts/layout.go:89` | Not used by container.go -- should be after refactor |
| `security.BuildArgs()` | `internal/security/flags.go` | Only used by doctor, not by container start |
| `security.ValidateArgs()` | `internal/security/flags.go` | Only used by doctor/tests, not by container start |
| `container.ValidateWorkspace()` | `internal/container/workspace.go` | Duplicated, should be replaced by `mounts.ValidateWorkspace()` |
| `mounts.VolumePrefix()` (exported) | `internal/mounts/layout.go:116` | Only used by `cmd/repair.go` -- fine |

---

## 12. Summary Statistics

| Metric | Count |
|--------|-------|
| Go source files (non-test) | 19 |
| Go test files | 15 |
| Estimated unit tests | ~68 |
| Integration test files | 4 |
| Infrastructure scripts | 14 |
| Container images | 4 (base, java, node, full) |
| CI pipeline files | 2 |
| Policy files | 2 (Rego + YAML) |
| Config files | 2 (seccomp + AppArmor) |
| **Total code files audited** | **~60** |

---

*End of audit report.*
