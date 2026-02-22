# Phase 8: Self-Update, Tool Pack Registry & Plugin System

**Phase**: 8 (follows Phase 7: Distribution, Installation & Extensibility)
**Estimated Effort**: 3-5 engineer-weeks
**Team Size**: 1-2 engineers
**Dependencies**: Phase 7 complete (build system, installation, configuration, CI/CD, documentation)
**Spec Sections**: Section 15 (Tool Packs), Appendix B (Developer Quickstart)
**Status**: Not Started

---

## Overview

Phase 7 established the foundation for distributing and installing AI-Box: a multi-binary build system with GoReleaser, cross-platform installation (curl, Homebrew, APT), configuration schema validation with templates and migrations, CI/CD pipelines, and developer documentation. These four work streams (WS1-WS3, WS7) were prioritized because every subsequent extensibility feature depends on a working release pipeline, installable binaries, and validated configuration.

Phase 8 picks up the three remaining Phase 7 work streams that were deferred:

| Work Stream | What It Delivers | Why It Was Deferred |
|-------------|-----------------|---------------------|
| WS4: CLI Self-Update | Binary self-update from GitHub Releases | Requires working release pipeline (WS1) and install infrastructure (WS2) |
| WS5: Tool Pack Registry | OCI-based remote distribution of tool packs | Requires build system for signing artifacts (WS1) and config templates for registry URLs (WS3) |
| WS6: Plugin System | Binary plugin discovery, management, and signing | Requires release pipeline for plugin distribution (WS1) and config system for policy enforcement (WS3) |

These three work streams transform AI-Box from a static tool into a self-maintaining, extensible platform. Users get automatic updates, teams publish and share tool packs through OCI registries, and the community extends AI-Box without modifying core code.

---

## Dependencies

### Blocking (Phase 7 Must Be Complete)

| Phase 7 Deliverable | Required By | Reason |
|---------------------|-------------|--------|
| D1: Root Makefile | WS4, WS5, WS6 | Coordinates multi-binary builds that self-update replaces |
| D2: `.goreleaser.yaml` | WS4, WS5 | Self-update downloads from GoReleaser-produced releases; signing infrastructure reused for tool packs |
| D3: Release workflow | WS4 | Self-update fetches from GitHub Releases created by this workflow |
| D5: Install script | WS4 | Self-update must preserve install location detected by the installer |
| D9: Config schema validation | WS5, WS6 | Registry URLs and plugin policies validated on config load |
| D10: Config migration | WS5, WS6 | New config keys (`registry.packs_url`, `plugins.require_signed`) added via migration |
| D11: Config templates | WS5, WS6 | Enterprise/dev/minimal templates set signing enforcement levels |
| D12: Generic defaults | WS5 | Default pack registry URL (`ghcr.io/krukkeniels/aibox-packs`) |

### Non-Blocking

| Dependency | Source | Impact If Not Ready |
|------------|--------|-------------------|
| Phase 6 training docs | Phase 6 | Can cross-reference; not required |
| Harbor registry | Phase 0 | Enterprise tool pack registry; GHCR works as alternative |
| Vault infrastructure | Phase 3 | Enterprise credential flow; simplified broker works |
| Falco integration | Phase 5 | Plugin audit logging; can add later |

### External

| Dependency | Owner | Risk |
|------------|-------|------|
| `creativeprojects/go-selfupdate` | Community | Low -- stable, well-maintained Go library for GitHub Release-based updates |
| `oras.land/oras-go/v2` | ORAS project (CNCF) | Low -- CNCF sandbox, actively maintained OCI artifact library |
| GHCR (GitHub Container Registry) | GitHub | Low -- OCI artifact hosting for tool packs |
| Cosign / Sigstore | Sigstore project | Low -- stable, CNCF graduated; already used in Phase 7 release signing |

---

## Work Stream 4: CLI Self-Update

**Objective**: `aibox update --self` updates the CLI binary from GitHub Releases without requiring package manager interaction.

**Current state** (after Phase 7): `cmd/aibox/cmd/update.go` (165 lines) handles container image updates only. It checks for running containers, pulls the latest image, verifies signatures, and reports digest changes. No binary self-update capability exists. Phase 7 delivers the release pipeline that produces signed binaries on GitHub Releases -- this work stream consumes those releases.

### D13: `aibox update --self` Binary Self-Update

Extend the existing `update` command with new flags:

| Flag | Behavior |
|------|----------|
| `aibox update` | Existing behavior -- update container images |
| `aibox update --self` | Update the CLI binary itself |
| `aibox update --all` | Update both CLI binary and container images |
| `aibox update --rollback` | Restore the previous CLI version |
| `aibox update --check` | Check for updates without installing |

**Self-update flow**:
1. Query GitHub Releases API for latest version
2. Compare with current version (`main.version` from ldflags)
3. If newer: download archive, verify checksum, extract
4. Backup current binary to `~/.local/share/aibox/aibox.previous`
5. Atomic replace: write to temp file, `os.Rename()` over current binary
6. Print changelog summary for new version

**Library**: `creativeprojects/go-selfupdate` -- handles GitHub release discovery, platform detection, and binary replacement.

### D14: Version Check on Startup with 24h Cache

Non-blocking background version check in `PersistentPreRunE` (modify `cmd/aibox/cmd/root.go`).

- Runs in a goroutine with 2-second timeout
- Caches result in `~/.config/aibox/.version-check` for 24 hours
- If newer version available, prints one-line notice after command output:
  ```
  A new version of aibox is available (v1.2.0 -> v1.3.0). Run 'aibox update --self' to upgrade.
  ```
- Disable: `aibox config set update.check false` or `AIBOX_UPDATE_CHECK=false`
- Respects `--quiet` / non-interactive mode (no notice in CI)

### D15: `aibox update --rollback` Mechanism

Simple single-version rollback:
- Before replacing: copy current binary to `~/.local/share/aibox/aibox.previous`
- `aibox update --rollback`: swap current <-> previous
- Keep only 1 previous version (no deep history)
- Print version info after rollback: `Rolled back from v1.3.0 to v1.2.0`

### Key Files

| File | Action |
|------|--------|
| `cmd/aibox/cmd/update.go` | Modify (add `--self`, `--all`, `--rollback`, `--check` flags) |
| `cmd/aibox/internal/selfupdate/selfupdate.go` | New (update logic, GitHub release client) |
| `cmd/aibox/internal/selfupdate/selfupdate_test.go` | New |
| `cmd/aibox/internal/selfupdate/rollback.go` | New (backup/restore logic) |
| `cmd/aibox/internal/selfupdate/rollback_test.go` | New |
| `cmd/aibox/cmd/root.go` | Modify (background version check) |
| `cmd/aibox/go.mod` | Modify (add `creativeprojects/go-selfupdate`) |

---

## Work Stream 5: Tool Pack Registry & Distribution

**Objective**: Tool packs are discoverable, versioned, signed, and installable from a central registry -- not just local filesystem.

**Current state** (after Phase 7): 12 tool packs defined as `manifest.yaml` + `install.sh` in `aibox-toolpacks/packs/`. Filesystem-based registry in `cmd/aibox/internal/toolpacks/` (7 files: manifest, registry, installer, dependency resolver, and tests). Phase 7 delivers config templates with `registry.packs_url` and signing infrastructure via cosign. No remote registry, no signing of packs, no versioning, no discovery commands.

### D16: OCI-Based Tool Pack Distribution via GHCR

Package tool packs as OCI artifacts using the ORAS (OCI Registry As Storage) library. Extend `cmd/aibox/internal/toolpacks/registry.go`.

**Pack format** (OCI artifact):
```
mediaType: application/vnd.aibox.toolpack.v1+tar
layers:
  - manifest.yaml
  - install.sh
  - (optional) additional files
annotations:
  org.aibox.pack.name: java
  org.aibox.pack.version: 21.0.2
  org.aibox.pack.description: "Java 21 JDK and build tools"
```

**Commands**:
| Command | Description |
|---------|-------------|
| `aibox pack push <name>` | Package local tool pack and push to OCI registry |
| `aibox pack pull <name>@<version>` | Download from registry to local cache |

**Default registry**: `ghcr.io/krukkeniels/aibox-packs` (configurable via `registry.packs_url`).

### D17: Tool Pack Signing + Checksum Verification

- Sign OCI artifacts with cosign on `pack push`
- Verify signatures on `aibox install <pack>` before running `install.sh`
- Checksum verification of `install.sh` content before execution
- Configurable enforcement:
  - `enterprise` template: `require_signed: true` -- reject unsigned packs
  - `dev`/`minimal` templates: `require_signed: false` -- warn on unsigned packs

### D18: `aibox pack publish/search/info` Commands

New `aibox pack` command group (`cmd/aibox/cmd/pack.go`):

| Command | Description |
|---------|-------------|
| `aibox pack search <query>` | Search registry for tool packs matching query |
| `aibox pack info <name>` | Show manifest details, versions, dependencies, size |
| `aibox pack list` | List installed packs with version info (enhances existing `aibox list`) |
| `aibox pack publish <dir>` | Validate manifest -> package -> sign -> push to registry |

### D19: Semver Versioning with Range Resolution

Add `version` field to tool pack manifests (currently absent).

**Resolution rules**:
| Input | Resolution |
|-------|-----------|
| `aibox install java` | Latest stable version |
| `aibox install java@21` | Latest `21.x.x` |
| `aibox install java@21.0.2` | Exact version |
| `aibox install java@latest` | Same as no version |

**Rollback**: `aibox install java@21.0.1` installs the older version, effectively downgrading.

### Key Files

| File | Action |
|------|--------|
| `cmd/aibox/internal/toolpacks/registry.go` | Modify (add OCI push/pull) |
| `cmd/aibox/internal/toolpacks/oci.go` | New (OCI packaging/unpacking) |
| `cmd/aibox/internal/toolpacks/oci_test.go` | New |
| `cmd/aibox/internal/toolpacks/signing.go` | New |
| `cmd/aibox/internal/toolpacks/signing_test.go` | New |
| `cmd/aibox/cmd/pack.go` | New (pack subcommands) |
| `aibox-toolpacks/packs/*/manifest.yaml` | Modify (add `version` field to all 12 manifests) |
| `cmd/aibox/go.mod` | Modify (add `oras.land/oras-go/v2`) |

---

## Work Stream 6: Plugin System

**Objective**: Community and teams can extend AI-Box with custom commands without modifying core code.

### D20: Binary Plugin System (`aibox-plugin-*` Convention)

**Convention**: Any executable named `aibox-plugin-<name>` in PATH becomes available as `aibox <name>`.

**Plugin manifest** (`~/.config/aibox/plugins/<name>.yaml`):
```yaml
name: example
version: 1.0.0
description: "Example plugin that does X"
binary: /usr/local/bin/aibox-plugin-example
permissions:
  network:
    - api.example.com:443
  filesystem:
    - /tmp/example
```

**Execution context** -- plugins receive environment variables:

| Variable | Value |
|----------|-------|
| `AIBOX_WORKSPACE` | Active workspace path |
| `AIBOX_CONFIG` | Path to config file |
| `AIBOX_SANDBOX_ID` | Current sandbox container ID |
| `AIBOX_VERSION` | CLI version |
| `AIBOX_PLUGIN_DIR` | Plugin data directory |

**Policy enforcement**: Plugin declared permissions are checked against OPA policy engine before execution. Enterprise config can restrict which plugins are allowed.

**Root command integration**: Modify `cmd/aibox/cmd/root.go` to scan for `aibox-plugin-*` binaries and register them as Cobra subcommands dynamically.

### D21: Plugin Management Commands (install/uninstall/list/init)

New `aibox plugin` command group (`cmd/aibox/cmd/plugin.go`):

| Command | Description |
|---------|-------------|
| `aibox plugin install <source>` | Download binary from GitHub release or URL, verify signature, create manifest |
| `aibox plugin uninstall <name>` | Remove binary and manifest |
| `aibox plugin list` | List installed plugins with versions and status |
| `aibox plugin init <name>` | Scaffold a new plugin project (Go template with Makefile, main.go, manifest) |
| `aibox plugin verify <name>` | Check signature validity of installed plugin |

**Source formats for install**:
- `github.com/user/aibox-plugin-example` -- download latest release from GitHub
- `https://example.com/plugin.tar.gz` -- direct URL download
- `./path/to/binary` -- local file install

### D22: Plugin Signing and Policy Enforcement

Reuse cosign infrastructure from WS5:

- Enterprise config: require signed plugins (`plugins.require_signed: true`)
- Dev/minimal config: warn on unsigned plugins
- `aibox plugin verify <name>` -- manually check signature
- Signing uses same cosign keyless flow as tool packs and release artifacts

### D25: `docs/plugin-development.md`

| Section | Content |
|---------|---------|
| Architecture | How plugins are discovered and executed |
| Getting started | `aibox plugin init` walkthrough |
| Environment variables | Available context from host CLI |
| Permissions | How to declare and what the policy engine enforces |
| Signing | How to sign plugins for enterprise deployment |
| Publishing | Making plugins available to others |
| Examples | Simple plugin (add a command) + complex plugin (interact with sandbox) |

### Key Files

| File | Action |
|------|--------|
| `cmd/aibox/internal/plugins/manager.go` | New (install, uninstall, list, verify) |
| `cmd/aibox/internal/plugins/manager_test.go` | New |
| `cmd/aibox/internal/plugins/discovery.go` | New (scan PATH, load manifests) |
| `cmd/aibox/internal/plugins/discovery_test.go` | New |
| `cmd/aibox/internal/plugins/scaffold.go` | New (plugin init template) |
| `cmd/aibox/cmd/plugin.go` | New (plugin subcommands) |
| `cmd/aibox/cmd/root.go` | Modify (dynamic plugin registration) |
| `docs/plugin-development.md` | New |

---

## Execution Order

```
Week 1:    [WS4] CLI Self-Update (quick win, 0.5-1 week)
              |
Week 2-4:  [WS5] Tool Pack Registry ───────┐
           [WS6] Plugin System (parallel) ──┘
```

**WS4 first**: Self-update is the smallest work stream (4 new files + 3 modifications) and delivers immediate user value. It depends only on the Phase 7 release pipeline and can be completed in under a week.

**WS5 + WS6 in parallel**: Both work streams are independent of each other but share cosign signing infrastructure. WS5 adds OCI distribution to the existing tool pack system. WS6 builds a new plugin subsystem. A single engineer can tackle them sequentially, or two engineers can work them in parallel.

**D25 (plugin docs) written alongside WS6**: The plugin development guide is best authored while building the plugin system itself.

---

## Exit Criteria

### Self-Update (WS4)
- [ ] `aibox update --self` downloads and installs the latest version from GitHub Releases
- [ ] `aibox update --rollback` restores the previous version
- [ ] Version check notice appears at most once per 24 hours when a new version is available
- [ ] `aibox update --check` reports current and latest version without installing

### Tool Pack Registry (WS5)
- [ ] `aibox pack search java` finds the java tool pack from GHCR
- [ ] `aibox pack publish ./my-pack` packages, signs, and pushes to registry
- [ ] `aibox install java@21` resolves semver range and installs from OCI registry
- [ ] Enterprise config rejects unsigned tool packs

### Plugin System (WS6)
- [ ] `aibox plugin install github.com/user/aibox-plugin-example` installs and registers a plugin
- [ ] `aibox example-command` executes the installed plugin
- [ ] `aibox plugin init my-plugin` scaffolds a buildable plugin project
- [ ] `aibox plugin list` shows installed plugins with versions and signature status
- [ ] Enterprise config rejects unsigned plugins
- [ ] `docs/plugin-development.md` enables a developer to build and publish a plugin

---

## Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|-----------|--------|-----------|
| R1 | Self-update fails mid-flight leaving corrupted binary | Low | High | Atomic rename pattern (write to temp, `os.Rename`). Keep previous version for `--rollback`. Verify checksum before replace. |
| R2 | OCI-based tool pack registry adds complexity vs filesystem | Medium | Medium | Start with GHCR (free, already used for images). Filesystem fallback always available. OCI is industry standard. |
| R3 | Plugin system security -- untrusted code execution in user context | Medium | High | Require signing for enterprise. Plugin permissions declared in manifest and enforced by OPA. Plugins run outside sandbox (cannot access container internals directly). |
| R4 | ORAS library breaking changes or API instability | Low | Medium | Pin to specific version in `go.mod`. ORAS is CNCF sandbox with stable v2 API. |
| R5 | Cosign keyless signing requires GitHub OIDC -- not available in all environments | Low | Medium | Support both keyless (CI) and key-based (manual) signing. Dev/minimal templates don't require signing. |

---

## Estimated Effort

| Work Stream | Effort | Calendar | Notes |
|-------------|--------|----------|-------|
| WS4: CLI Self-Update | 0.5-1 eng-week | Week 1 | Quick win -- builds on Phase 7 release infrastructure |
| WS5: Tool Pack Registry | 1.5-2 eng-weeks | Weeks 2-4 | OCI distribution, signing, discovery commands |
| WS6: Plugin System | 1-1.5 eng-weeks | Weeks 2-4 | Discovery, management, signing, scaffold, docs |
| **Total** | **3-5 eng-weeks** | **~4 weeks calendar** | 1-2 engineers |

---

## Verification Plan

| # | Test | Description |
|---|------|-------------|
| V1 | Self-update test | Install v0.1.0 -> `aibox update --self` -> verify upgraded to latest, changelog shown |
| V2 | Rollback test | Install v0.2.0 -> `aibox update --rollback` -> verify reverts to v0.1.0 |
| V3 | Version check test | Start aibox with outdated version -> verify notice printed -> run again within 24h -> verify no notice (cached) |
| V4 | Tool pack registry test | Publish pack to GHCR -> install on different machine -> verify signature check |
| V5 | Semver resolution test | `aibox install java@21` -> verify resolves to latest `21.x.x` from registry |
| V6 | Plugin lifecycle test | `aibox plugin init` -> build -> `aibox plugin install` -> verify in `aibox plugin list` -> execute via `aibox <name>` -> `aibox plugin uninstall` |
| V7 | Enterprise signing enforcement | Set `require_signed: true` -> attempt to install unsigned pack/plugin -> verify rejection |

---

## File Summary

### New Files (15)

| File | Work Stream |
|------|-------------|
| `cmd/aibox/internal/selfupdate/selfupdate.go` | WS4 |
| `cmd/aibox/internal/selfupdate/selfupdate_test.go` | WS4 |
| `cmd/aibox/internal/selfupdate/rollback.go` | WS4 |
| `cmd/aibox/internal/selfupdate/rollback_test.go` | WS4 |
| `cmd/aibox/internal/toolpacks/oci.go` | WS5 |
| `cmd/aibox/internal/toolpacks/oci_test.go` | WS5 |
| `cmd/aibox/internal/toolpacks/signing.go` | WS5 |
| `cmd/aibox/internal/toolpacks/signing_test.go` | WS5 |
| `cmd/aibox/cmd/pack.go` | WS5 |
| `cmd/aibox/internal/plugins/manager.go` | WS6 |
| `cmd/aibox/internal/plugins/manager_test.go` | WS6 |
| `cmd/aibox/internal/plugins/discovery.go` | WS6 |
| `cmd/aibox/internal/plugins/discovery_test.go` | WS6 |
| `cmd/aibox/internal/plugins/scaffold.go` | WS6 |
| `cmd/aibox/cmd/plugin.go` | WS6 |

### Modified Files (7)

| File | Work Stream | Changes |
|------|-------------|---------|
| `cmd/aibox/cmd/update.go` | WS4 | Add `--self`, `--all`, `--rollback`, `--check` flags |
| `cmd/aibox/cmd/root.go` | WS4, WS6 | Background version check, dynamic plugin registration |
| `cmd/aibox/go.mod` | WS4, WS5 | Add `creativeprojects/go-selfupdate`, `oras.land/oras-go/v2` |
| `cmd/aibox/internal/toolpacks/registry.go` | WS5 | Add OCI push/pull support |
| `aibox-toolpacks/packs/*/manifest.yaml` | WS5 | Add `version` field to all 12 manifests |
| `docs/plugin-development.md` | WS6 | New (plugin development guide) |
| `README.md` | WS6 | Add plugin development section |
