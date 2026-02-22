# Phase 7: Distribution, Installation & Extensibility

**Phase**: 7 (follows Phase 6: Rollout & Operations)
**Estimated Effort**: 8-12 engineer-weeks
**Team Size**: 2-3 engineers
**Dependencies**: Phases 0-5 complete; Phase 6 deliverables provide docs/training context
**Spec Sections**: Appendix B (Developer Quickstart), Section 15 (Tool Packs), Section 21 (Rollout), Section 22 (Operations)
**Status**: Not Started

---

## Overview

Phases 0-6 built a fully functional AI-Box platform with 18 CLI commands, 24 internal packages, 12 tool packs, container images with CI/CD, and comprehensive rollout documentation. However, **there is no way for users to actually install the CLI**. There is no release pipeline for the 4 Go binaries, no install script, no package manager integration, no self-update mechanism, and no plugin system for community extensions. The `aibox setup` command assumes the binary already exists on the system.

Phase 7 closes the "last mile" gap: from source code in a Git repo to a tool that developers can install in under 60 seconds and extend without rebuilding.

### Current State

| Area | Status |
|------|--------|
| Go binaries | 4 separate modules (`aibox`, `aibox-credential-helper`, `aibox-llm-proxy`, `aibox-git-remote-helper`), each with its own `go.mod`. Only `cmd/aibox/` has a Makefile. |
| CI/CD | No `.github/workflows/` exist. No automated testing, building, or releasing. |
| Installation | None. Users must `go build` from source or use a pre-built binary placed manually. |
| Config defaults | Hardcoded to `harbor.internal` and enterprise infrastructure. No open-source-friendly defaults. |
| Self-update | `aibox update` updates container images only. No CLI binary self-update. |
| Tool pack registry | Filesystem-based (`aibox-toolpacks/packs/`). No remote registry, no signing, no versioning. |
| Plugin system | Does not exist. No extension mechanism beyond modifying source. |

---

## Deliverables

| # | Deliverable | Work Stream |
|---|-------------|-------------|
| D1 | Root Makefile coordinating all 4 Go binaries | WS1: Build System |
| D2 | `.goreleaser.yaml` for multi-binary, multi-platform builds with signing + SBOM | WS1: Build System |
| D3 | GitHub Actions release workflow (test → build → sign → publish) | WS1: Build System |
| D4 | GitHub Actions CI workflow (lint → test → build on PRs) | WS1: Build System |
| D5 | Install script (`scripts/install.sh`) with platform detection, verification, PATH setup | WS2: Installation |
| D6 | Enhanced `aibox setup` with pre-flight checks, progress reporting, post-validation | WS2: Installation |
| D7 | Homebrew tap repository (`krukkeniels/homebrew-aibox`) | WS2: Installation |
| D8 | APT repository for WSL2 Ubuntu users | WS2: Installation |
| D9 | Config schema validation on load | WS3: Configuration |
| D10 | Config migration system for version upgrades | WS3: Configuration |
| D11 | Config templates (`--template enterprise\|dev\|minimal`) | WS3: Configuration |
| D12 | Generic defaults for open-source use (not hardcoded `harbor.internal`) | WS3: Configuration |
| D13 | `aibox update --self` binary self-update mechanism | WS4: Self-Update |
| D14 | Version check on startup with 24h cache | WS4: Self-Update |
| D15 | Self-update rollback (`aibox update --rollback`) | WS4: Self-Update |
| D16 | OCI-based tool pack distribution (push/pull to GHCR) | WS5: Tool Pack Registry |
| D17 | Tool pack signing + checksum verification | WS5: Tool Pack Registry |
| D18 | `aibox pack publish/search/info` commands | WS5: Tool Pack Registry |
| D19 | Tool pack semver versioning with range resolution | WS5: Tool Pack Registry |
| D20 | Binary plugin system (`aibox-plugin-*` convention) | WS6: Plugin System |
| D21 | Plugin management commands (install/uninstall/list/init) | WS6: Plugin System |
| D22 | Plugin signing and policy enforcement | WS6: Plugin System |
| D23 | `docs/installation.md` — all install methods per platform | WS7: Documentation |
| D24 | `docs/configuration.md` — all config options, templates, migration | WS7: Documentation |
| D25 | `docs/plugin-development.md` — how to build plugins and tool packs | WS7: Documentation |
| D26 | `docs/contributing.md` — contribution guide for code, packs, plugins | WS7: Documentation |
| D27 | Updated `README.md` with install instructions and quickstart | WS7: Documentation |

---

## Implementation Steps

### Work Stream 1: Build System & Release Pipeline

**Objective**: Go from `git tag v1.0.0 && git push --tags` to signed binaries on GitHub Releases automatically.

**Current state**: 4 separate Go modules with different Go versions (1.23, 1.24.6). Only `cmd/aibox/` has a Makefile (42 lines with build/test/lint/clean targets). No GitHub Actions workflows exist. Container image CI is referenced in docs but no workflow files are present.

**What to build**:

#### 1. Root Makefile (`Makefile`)

Coordinate all 4 binaries from a single entry point.

| Target | Description |
|--------|-------------|
| `build` | Compile all 4 binaries for host platform into `bin/` |
| `build-<name>` | Compile a single binary (e.g., `build-aibox`) |
| `test` | Run `go test ./...` across all 4 modules |
| `lint` | Run `golangci-lint` across all 4 modules |
| `fmt` | Run `gofmt` and `goimports` across all modules |
| `release-local` | Cross-compile for all supported platforms (testing only) |
| `clean` | Remove all build artifacts |
| `install` | Build and copy binaries to `~/.local/bin` |

Implementation details:
- Version injection via `-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"` — matches the existing `main.go` ldflags pattern
- `VERSION` derived from `git describe --tags --always --dirty`
- `CGO_ENABLED=0` for static binaries (matches existing `cmd/aibox/Makefile`)
- Module directories defined as variables for DRY iteration

#### 2. GoReleaser Config (`.goreleaser.yaml`)

Multi-binary, multi-platform release automation.

**Build targets** (4 builds):

| Binary | Source Dir | Main |
|--------|-----------|------|
| `aibox` | `cmd/aibox` | `./` |
| `aibox-credential-helper` | `cmd/aibox-credential-helper` | `./` |
| `aibox-llm-proxy` | `cmd/aibox-llm-proxy` | `./` |
| `aibox-git-remote-helper` | `cmd/aibox-git-remote-helper` | `./` |

**Platform matrix**:

| OS | Architectures |
|----|--------------|
| Linux | amd64, arm64 |
| macOS | amd64, arm64 |
| Windows | amd64 |

**Archive format**: `tar.gz` for Linux/macOS, `zip` for Windows.

**Additional outputs**:
- SHA256 checksums (`checksums.txt`)
- Cosign keyless signing of checksums (GitHub OIDC identity)
- Syft SBOM generation per binary
- Auto-generated changelog from conventional commits
- Homebrew formula pushed to `krukkeniels/homebrew-aibox`
- `.deb` and `.rpm` packages for Linux

#### 3. GitHub Actions Release Workflow (`.github/workflows/release.yaml`)

Triggered on tag push matching `v*`.

```
Job 1: test
  - Checkout code
  - Setup Go 1.24
  - golangci-lint across all 4 modules
  - go test ./... across all 4 modules

Job 2: release (needs: test)
  - Checkout code
  - Setup Go 1.24
  - Install cosign, syft
  - goreleaser release --clean
  - Permissions: contents: write, id-token: write (cosign OIDC)
```

#### 4. GitHub Actions CI Workflow (`.github/workflows/test.yaml`)

Triggered on pull requests and pushes to `main`.

```
Matrix: Go [1.23, 1.24], OS [ubuntu-latest]

Steps:
  - Checkout
  - Setup Go (matrix version)
  - golangci-lint
  - go test ./... (all 4 modules)
  - go build (all 4 binaries — verify compilation)
```

**Key files**:
| File | Action |
|------|--------|
| `Makefile` | New |
| `.goreleaser.yaml` | New |
| `.github/workflows/release.yaml` | New |
| `.github/workflows/test.yaml` | New |

---

### Work Stream 2: Installation & First-Run

**Objective**: A developer on Windows 11 + WSL2 can go from zero to `aibox start` in under 5 minutes.

**What to build**:

#### 1. Install Script (`scripts/install.sh`)

A single `curl | bash` installer.

**Flow**:
1. Detect platform: `uname -s` → `linux`/`darwin`, `uname -m` → normalize `x86_64` → `amd64`, `aarch64` → `arm64`
2. Determine latest version from GitHub Releases API (`/repos/krukkeniels/ai-box/releases/latest`)
3. Download archive + `checksums.txt` from release assets
4. Verify checksum with `sha256sum --check`
5. Optionally verify cosign signature if `cosign` is in PATH
6. Extract and install 4 binaries to install directory
7. Add to PATH if needed (append to `~/.bashrc` or `~/.zshrc`)
8. Print success message and next steps

**Install locations** (in priority order):
1. `--dir <path>` if specified
2. `/usr/local/bin` if writable or sudo available
3. `~/.local/bin` as fallback

**Flags**:
| Flag | Description |
|------|-------------|
| `--version <tag>` | Install specific version (default: latest) |
| `--dir <path>` | Custom install directory |
| `--no-verify` | Skip checksum verification (not recommended) |
| `--help` | Show usage |

**Idempotent**: Safe to re-run. Overwrites existing binaries. Skips PATH modification if already present.

**Usage**:
```bash
curl -fsSL https://raw.githubusercontent.com/krukkeniels/ai-box/main/scripts/install.sh | bash
```

#### 2. Enhanced `aibox setup`

Modify `cmd/aibox/cmd/setup.go` and `cmd/aibox/internal/setup/linux.go` to add pre-flight checks, progress reporting, and post-validation.

**Pre-flight checks** (new file: `cmd/aibox/internal/setup/preflight.go`):

| Check | Warning Threshold | Error Threshold |
|-------|-------------------|-----------------|
| OS version | — | Fail if not Ubuntu 22.04+ or non-Linux |
| WSL2 detection | Inform if native Linux | — |
| Available RAM | Warn < 16 GB, suggest `.wslconfig` | Fail < 4 GB |
| Disk space | Warn < 20 GB free | Fail < 5 GB free |
| Podman installed | — | Fail if not found |
| gVisor (`runsc`) | Warn if not found (non-fatal) | — |
| Network connectivity | Skip image pull if offline | — |

**Progress reporting**: Replace current silent multi-step flow with step-by-step output:
```
[1/7] Checking prerequisites...         ✓
[2/7] Detecting container runtime...     ✓ podman 4.9.3
[3/7] Verifying gVisor sandbox...        ✓ runsc 2024.04
[4/7] Creating configuration...          ✓ ~/.config/aibox/config.yaml
[5/7] Generating SSH keys...             ✓ ~/.config/aibox/ssh/
[6/7] Pulling base image...              ✓ ghcr.io/krukkeniels/aibox/base:24.04
[7/7] Running health checks...           ✓ all 8 checks passed
```

**Post-setup validation**: Automatically run `aibox doctor` and display results.

**Non-destructive behavior**:
- Detect existing `config.yaml` → prompt: keep / overwrite / merge
- Detect existing SSH keys → skip regeneration
- Detect existing system setup → skip unless `--force`

**Error recovery**: If any step fails, print what succeeded and what needs manual fix.

#### 3. Homebrew Tap (`krukkeniels/homebrew-aibox`)

Auto-generated by GoReleaser on each release.

- Formula installs all 4 binaries
- Includes bash, zsh, and fish completions
- Usage: `brew install krukkeniels/aibox/aibox`
- Caveats message points to `aibox setup`

#### 4. APT Repository

For WSL2 Ubuntu users (primary target platform).

- GoReleaser produces `.deb` packages
- Static APT repo hosted via GitHub Pages
- GPG-signed `Release` file
- Usage:
  ```bash
  curl -fsSL https://krukkeniels.github.io/ai-box/gpg.key | sudo gpg --dearmor -o /usr/share/keyrings/aibox.gpg
  echo "deb [signed-by=/usr/share/keyrings/aibox.gpg] https://krukkeniels.github.io/ai-box/apt stable main" | sudo tee /etc/apt/sources.list.d/aibox.list
  sudo apt update && sudo apt install aibox
  ```

**Key files**:
| File | Action |
|------|--------|
| `scripts/install.sh` | New |
| `cmd/aibox/cmd/setup.go` | Modify |
| `cmd/aibox/internal/setup/linux.go` | Modify |
| `cmd/aibox/internal/setup/preflight.go` | New |
| `cmd/aibox/internal/setup/preflight_test.go` | New |

---

### Work Stream 3: Configuration & Upgrade

**Objective**: Config is validated, versioned, migratable, and works for both enterprise (internal registries) and open-source (public registries) use.

**Current state**: Viper-based config at `~/.config/aibox/config.yaml` with 48 default keys defined in `config.go` (427 lines). Defaults are hardcoded to enterprise values (`harbor.internal:443`, Vault URLs, SPIFFE trust domains). No schema validation, no versioning, no migration, no templates.

**What to build**:

#### 1. Config Schema Validation (`cmd/aibox/internal/config/validation.go`)

Validate config on every load in `LoadConfig()`.

| Validation | Example |
|-----------|---------|
| Required fields present | `registry.url` must not be empty |
| Value ranges | `resources.cpu` > 0, `resources.memory` parseable (e.g., `8g`) |
| Valid enum values | `logging.format` ∈ {`text`, `json`} |
| Hostname format | `registry.url`, `network.squid_addr` must be valid |
| Port range | All ports ∈ [1, 65535] |
| Path existence | `workspace.path` must be a valid directory |
| Unknown field warnings | Warn on keys not in schema (likely typos) |

**New command**: `aibox config validate` — explicitly validate current config and report all issues.

**Error output format**:
```
config: 2 errors, 1 warning
  ERROR  registry.url: "harbor.internal:443" is not reachable (DNS resolution failed)
  ERROR  resources.cpu: value "0" must be > 0
  WARN   loggging.format: unknown field (did you mean "logging.format"?)
```

#### 2. Config Versioning & Migration (`cmd/aibox/internal/config/migration.go`)

Add `config_version: 1` field to config files.

**Migration system**:
- Migration functions registered as `v1 → v2`, `v2 → v3`, etc.
- On load: detect version, run all pending migrations in sequence
- Before migration: backup to `config.yaml.backup.v<old_version>`
- After migration: validate migrated config

**Example migration (v1 → v2)**:
- Rename `registry.harbor_url` → `registry.url`
- Add new field `update.check: true`
- Remove deprecated `network.legacy_proxy` field

**New command**: `aibox config migrate` — explicitly run migrations (with `--dry-run` flag to preview changes).

#### 3. Generic Defaults (modify `cmd/aibox/internal/config/config.go`)

Replace enterprise-specific defaults with open-source-friendly values.

| Config Key | Current Default | New Default |
|-----------|----------------|-------------|
| `registry.url` | `harbor.internal:443` | `ghcr.io/krukkeniels/aibox` |
| `registry.verify_signatures` | `true` | `false` |
| `network.squid_enabled` | (implied) | `false` |
| `network.coredns_enabled` | (implied) | `false` |
| `credentials.vault_addr` | `https://vault.internal:8200` | ` ` (empty — disabled) |
| `credentials.spiffe_socket` | `/run/spire/...` | ` ` (empty — disabled) |
| `audit.falco_enabled` | `true` | `false` |
| `gvisor.required` | `true` | `false` |

Enterprise users activate full security via `aibox config init --template enterprise`.

#### 4. Config Templates (`cmd/aibox/configs/templates/`)

Embedded in the binary via Go `embed.FS`.

| Template | Use Case | Key Settings |
|----------|----------|-------------|
| `minimal.yaml` | Open-source / personal use | Public images, no proxy, no Vault, no gVisor requirement, no Falco |
| `dev.yaml` | Development / testing | Public images, local Squid proxy, basic security, gVisor recommended |
| `enterprise.yaml` | Production enterprise | Private registry, Vault, Squid, CoreDNS, gVisor required, Falco, full audit |

**New command**: `aibox config init --template <name>` — generate config from template.
- Refuses to overwrite existing config without `--force`
- Prints summary of what was configured
- Runs validation on generated config

**Key files**:
| File | Action |
|------|--------|
| `cmd/aibox/internal/config/validation.go` | New |
| `cmd/aibox/internal/config/validation_test.go` | New |
| `cmd/aibox/internal/config/migration.go` | New |
| `cmd/aibox/internal/config/migration_test.go` | New |
| `cmd/aibox/internal/config/config.go` | Modify (generic defaults, `config_version`, embed templates) |
| `cmd/aibox/configs/templates/minimal.yaml` | New |
| `cmd/aibox/configs/templates/dev.yaml` | New |
| `cmd/aibox/configs/templates/enterprise.yaml` | New |
| `cmd/aibox/cmd/config_cmd.go` | Modify (add `validate`, `migrate`, `init` subcommands) |

---

### Work Stream 4: CLI Self-Update

**Objective**: `aibox update --self` updates the CLI binary from GitHub Releases without requiring package manager interaction.

**Current state**: `cmd/aibox/cmd/update.go` (165 lines) handles container image updates only. It checks for running containers, pulls the latest image, verifies signatures, and reports digest changes. No binary self-update capability exists.

**What to build**:

#### 1. Self-Update Command (extend `cmd/aibox/cmd/update.go`)

Extend the existing `update` command with new flags:

| Flag | Behavior |
|------|----------|
| `aibox update` | Existing behavior — update container images |
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

**Library**: `creativeprojects/go-selfupdate` — handles GitHub release discovery, platform detection, and binary replacement.

#### 2. Version Check on Startup (modify `cmd/aibox/cmd/root.go`)

Non-blocking background version check in `PersistentPreRunE`.

- Runs in a goroutine with 2-second timeout
- Caches result in `~/.config/aibox/.version-check` for 24 hours
- If newer version available, prints one-line notice after command output:
  ```
  A new version of aibox is available (v1.2.0 → v1.3.0). Run 'aibox update --self' to upgrade.
  ```
- Disable: `aibox config set update.check false` or `AIBOX_UPDATE_CHECK=false`
- Respects `--quiet` / non-interactive mode (no notice in CI)

#### 3. Rollback (`cmd/aibox/internal/selfupdate/rollback.go`)

Simple single-version rollback:
- Before replacing: copy current binary to `~/.local/share/aibox/aibox.previous`
- `aibox update --rollback`: swap current ↔ previous
- Keep only 1 previous version (no deep history)
- Print version info after rollback: `Rolled back from v1.3.0 to v1.2.0`

**Key files**:
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

### Work Stream 5: Tool Pack Registry & Distribution

**Objective**: Tool packs are discoverable, versioned, signed, and installable from a central registry — not just local filesystem.

**Current state**: 12 tool packs defined as `manifest.yaml` + `install.sh` in `aibox-toolpacks/packs/`. Filesystem-based registry in `cmd/aibox/internal/toolpacks/` (7 files: manifest, registry, installer, dependency resolver, and tests). No remote registry, no signing, no versioning, no discovery commands.

**What to build**:

#### 1. OCI-Based Tool Pack Distribution (extend `cmd/aibox/internal/toolpacks/registry.go`)

Package tool packs as OCI artifacts using the ORAS (OCI Registry As Storage) library.

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

#### 2. Tool Pack Signing (`cmd/aibox/internal/toolpacks/signing.go`)

- Sign OCI artifacts with cosign on `pack push`
- Verify signatures on `aibox install <pack>` before running `install.sh`
- Checksum verification of `install.sh` content before execution
- Configurable enforcement:
  - `enterprise` template: `require_signed: true` — reject unsigned packs
  - `dev`/`minimal` templates: `require_signed: false` — warn on unsigned packs

#### 3. Discovery Commands (`cmd/aibox/cmd/pack.go`)

New `aibox pack` command group:

| Command | Description |
|---------|-------------|
| `aibox pack search <query>` | Search registry for tool packs matching query |
| `aibox pack info <name>` | Show manifest details, versions, dependencies, size |
| `aibox pack list` | List installed packs with version info (enhances existing `aibox list`) |
| `aibox pack publish <dir>` | Validate manifest → package → sign → push to registry |

#### 4. Semver Versioning

Add `version` field to tool pack manifests (currently absent).

**Resolution rules**:
| Input | Resolution |
|-------|-----------|
| `aibox install java` | Latest stable version |
| `aibox install java@21` | Latest `21.x.x` |
| `aibox install java@21.0.2` | Exact version |
| `aibox install java@latest` | Same as no version |

**Rollback**: `aibox install java@21.0.1` installs the older version, effectively downgrading.

**Key files**:
| File | Action |
|------|--------|
| `cmd/aibox/internal/toolpacks/registry.go` | Modify (add OCI push/pull) |
| `cmd/aibox/internal/toolpacks/oci.go` | New (OCI packaging/unpacking) |
| `cmd/aibox/internal/toolpacks/oci_test.go` | New |
| `cmd/aibox/internal/toolpacks/signing.go` | New |
| `cmd/aibox/internal/toolpacks/signing_test.go` | New |
| `cmd/aibox/cmd/pack.go` | New (pack subcommands) |
| `aibox-toolpacks/packs/*/manifest.yaml` | Modify (add `version` field) |
| `cmd/aibox/go.mod` | Modify (add `oras.land/oras-go/v2`) |

---

### Work Stream 6: Plugin System

**Objective**: Community and teams can extend AI-Box with custom commands without modifying core code.

**What to build**:

#### 1. Plugin Discovery & Execution (`cmd/aibox/internal/plugins/`)

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

**Execution context** — plugins receive environment variables:

| Variable | Value |
|----------|-------|
| `AIBOX_WORKSPACE` | Active workspace path |
| `AIBOX_CONFIG` | Path to config file |
| `AIBOX_SANDBOX_ID` | Current sandbox container ID |
| `AIBOX_VERSION` | CLI version |
| `AIBOX_PLUGIN_DIR` | Plugin data directory |

**Policy enforcement**: Plugin declared permissions are checked against OPA policy engine before execution. Enterprise config can restrict which plugins are allowed.

**Root command integration**: Modify `cmd/aibox/cmd/root.go` to scan for `aibox-plugin-*` binaries and register them as Cobra subcommands dynamically.

#### 2. Plugin Management Commands (`cmd/aibox/cmd/plugin.go`)

| Command | Description |
|---------|-------------|
| `aibox plugin install <source>` | Download binary from GitHub release or URL, verify signature, create manifest |
| `aibox plugin uninstall <name>` | Remove binary and manifest |
| `aibox plugin list` | List installed plugins with versions and status |
| `aibox plugin init <name>` | Scaffold a new plugin project (Go template with Makefile, main.go, manifest) |
| `aibox plugin verify <name>` | Check signature validity of installed plugin |

**Source formats for install**:
- `github.com/user/aibox-plugin-example` — download latest release from GitHub
- `https://example.com/plugin.tar.gz` — direct URL download
- `./path/to/binary` — local file install

#### 3. Plugin Signing (reuse cosign infrastructure from WS5)

- Enterprise config: require signed plugins (`plugins.require_signed: true`)
- Dev/minimal config: warn on unsigned plugins
- `aibox plugin verify <name>` — manually check signature
- Signing uses same cosign keyless flow as tool packs and release artifacts

**Key files**:
| File | Action |
|------|--------|
| `cmd/aibox/internal/plugins/manager.go` | New (install, uninstall, list, verify) |
| `cmd/aibox/internal/plugins/manager_test.go` | New |
| `cmd/aibox/internal/plugins/discovery.go` | New (scan PATH, load manifests) |
| `cmd/aibox/internal/plugins/discovery_test.go` | New |
| `cmd/aibox/internal/plugins/scaffold.go` | New (plugin init template) |
| `cmd/aibox/cmd/plugin.go` | New (plugin subcommands) |
| `cmd/aibox/cmd/root.go` | Modify (dynamic plugin registration) |

---

### Work Stream 7: Developer Documentation

**Objective**: Complete documentation for installation, configuration, extension, and contribution.

#### 1. `docs/installation.md` (D23)

| Section | Content |
|---------|---------|
| Quick install | `curl` one-liner |
| Homebrew | `brew install` instructions for macOS and Linux |
| APT | Repository setup + `apt install` for WSL2 Ubuntu |
| Manual download | Direct binary download from GitHub Releases |
| Build from source | `git clone`, `make build`, `make install` |
| Post-install | `aibox setup` walkthrough with expected output |
| Verification | `aibox doctor` explanation of each check |
| Troubleshooting | Common issues: PATH, permissions, WSL2, Podman |
| Uninstall | Clean removal instructions per install method |

#### 2. `docs/configuration.md` (D24)

| Section | Content |
|---------|---------|
| Config file location | `~/.config/aibox/config.yaml` |
| All config keys | Table with key, type, default, description |
| Templates | `minimal`, `dev`, `enterprise` explained |
| Environment overrides | `AIBOX_*` naming convention |
| Config migration | How versioned migration works |
| Per-profile examples | Full config files for each deployment scenario |
| Validation | How to validate and fix config errors |

#### 3. `docs/plugin-development.md` (D25)

| Section | Content |
|---------|---------|
| Architecture | How plugins are discovered and executed |
| Getting started | `aibox plugin init` walkthrough |
| Environment variables | Available context from host CLI |
| Permissions | How to declare and what the policy engine enforces |
| Signing | How to sign plugins for enterprise deployment |
| Publishing | Making plugins available to others |
| Examples | Simple plugin (add a command) + complex plugin (interact with sandbox) |

#### 4. `docs/contributing.md` (D26)

| Section | Content |
|---------|---------|
| Development setup | Go, Podman, gVisor, golangci-lint |
| Repository structure | Map of all directories and packages |
| Adding a CLI command | Step-by-step with code examples |
| Creating a tool pack | Manifest + install.sh anatomy |
| Creating an MCP pack | Config + policy requirements |
| Testing | Unit, integration, security, e2e test suites |
| PR process | Branch naming, commit conventions, review checklist |

#### 5. README.md Update (D27)

Add to existing README:
- **Installation** section with 3 install methods (curl, brew, apt)
- **Quick Start** section: install → setup → start → shell (4 commands)
- **Contributing** link to `docs/contributing.md`
- **Plugin Development** link to `docs/plugin-development.md`
- Badge: latest release version from GitHub

**Key files**:
| File | Action |
|------|--------|
| `docs/installation.md` | New |
| `docs/configuration.md` | New |
| `docs/plugin-development.md` | New |
| `docs/contributing.md` | New |
| `README.md` | Modify |

---

## Dependencies

### Blocking

| Dependency | Source | What It Provides |
|------------|--------|-----------------|
| All 4 Go binaries compile | Phases 0-5 | Working code to package |
| Existing CLI commands | Phases 1-4 | Commands to document and extend |
| Config system | Phase 1 | Viper-based config to add validation/migration to |
| OPA policy engine | Phase 3 | Policy enforcement for plugins |
| Cosign signing | Phase 3 | Signing infrastructure to reuse |

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
| GitHub Actions runners | GitHub | Low — widely available |
| GHCR (GitHub Container Registry) | GitHub | Low — OCI artifact hosting |
| Cosign / Sigstore | Sigstore project | Low — stable, CNCF graduated |
| GoReleaser | GoReleaser project | Low — mature, widely used |
| ORAS (OCI artifacts) | ORAS project | Low — CNCF sandbox, actively maintained |
| `creativeprojects/go-selfupdate` | Community | Low — stable, well-maintained |

---

## Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|-----------|--------|-----------|
| R1 | 4 separate Go modules with different Go versions complicate coordinated releases | High | Medium | GoReleaser supports multiple builds from different directories. Root Makefile coordinates. Standardize all modules on Go 1.24. Consider consolidating to monorepo module long-term. |
| R2 | WSL2-specific install issues (PATH, permissions, systemd) | Medium | High | Install script tested on clean WSL2 Ubuntu 24.04. Extensive troubleshooting section in docs. `aibox doctor` catches post-install problems. |
| R3 | Config migration breaks existing users during upgrade | Medium | High | Always backup before migration. Validate migrated config. `--dry-run` flag for migration preview. Rollback to backup if migration fails. |
| R4 | OCI-based tool pack registry adds complexity vs filesystem | Medium | Medium | Start with GHCR (free, already used for images). Filesystem fallback always available. OCI is industry standard. |
| R5 | Plugin system security — untrusted code execution in user context | Medium | High | Require signing for enterprise. Plugin permissions declared in manifest and enforced by OPA. Plugins run outside sandbox (cannot access container internals directly). |
| R6 | Self-update fails mid-flight leaving corrupted binary | Low | High | Atomic rename pattern (write to temp, `os.Rename`). Keep previous version for `--rollback`. Verify checksum before replace. |
| R7 | Homebrew/APT repo maintenance overhead | Low | Medium | GoReleaser automates Homebrew tap on every release. APT repo is static files served from GitHub Pages (near-zero maintenance). |

---

## Exit Criteria

### Build System
- [ ] `make build` compiles all 4 binaries for the host platform
- [ ] `make test` runs all tests across all 4 modules and passes
- [ ] `git tag v0.1.0 && git push --tags` triggers GitHub Actions and produces a GitHub Release with signed binaries, checksums, and SBOMs

### Installation
- [ ] `curl -fsSL .../install.sh | bash` installs aibox on a clean WSL2 Ubuntu 24.04 in under 60 seconds
- [ ] `brew install krukkeniels/aibox/aibox` installs successfully on macOS and Linux
- [ ] `aibox setup` completes with clear progress output and passes `aibox doctor` validation
- [ ] `aibox doctor` reports all green on a fresh install

### Configuration
- [ ] Loading a config with invalid values produces clear, actionable error messages
- [ ] `aibox config init --template minimal` generates a working config for open-source use
- [ ] `aibox config validate` reports all issues with severity levels
- [ ] Upgrading from config v1 to v2 automatically migrates and backs up the original

### Self-Update
- [ ] `aibox update --self` downloads and installs the latest version from GitHub Releases
- [ ] `aibox update --rollback` restores the previous version
- [ ] Version check notice appears at most once per 24 hours when a new version is available
- [ ] `aibox update --check` reports current and latest version without installing

### Extensibility
- [ ] `aibox pack search java` finds the java tool pack from GHCR
- [ ] `aibox pack publish ./my-pack` packages, signs, and pushes to registry
- [ ] `aibox plugin install github.com/user/aibox-plugin-example` installs and registers a plugin
- [ ] `aibox example-command` executes the installed plugin
- [ ] Enterprise config rejects unsigned plugins and tool packs

### Documentation
- [ ] `docs/installation.md` covers all install methods with verified commands
- [ ] `docs/configuration.md` documents all 48+ config keys with defaults and valid values
- [ ] `docs/plugin-development.md` enables a developer to build and publish a plugin
- [ ] `docs/contributing.md` enables a new contributor to build and test locally
- [ ] `README.md` includes install instructions and quickstart

---

## Estimated Effort

| Work Stream | Effort | Calendar | Notes |
|-------------|--------|----------|-------|
| WS1: Build System & Release Pipeline | 1.5-2 eng-weeks | Weeks 1-2 | Foundation — unblocks all other streams |
| WS2: Installation & First-Run | 1.5-2 eng-weeks | Weeks 2-4 | Install script + enhanced setup + package repos |
| WS3: Configuration & Upgrade | 1-1.5 eng-weeks | Weeks 3-5 | Validation, migration, templates, generic defaults |
| WS4: CLI Self-Update | 0.5-1 eng-week | Weeks 4-5 | Builds on WS1 release infrastructure |
| WS5: Tool Pack Registry | 1.5-2 eng-weeks | Weeks 4-7 | OCI distribution, signing, discovery commands |
| WS6: Plugin System | 1-1.5 eng-weeks | Weeks 6-8 | Discovery, management, signing, scaffold |
| WS7: Documentation | 1-1.5 eng-weeks | Weeks 6-9 | Written throughout, finalized last |
| **Total** | **8-12 eng-weeks** | **~9 weeks calendar** | 2-3 engineers in parallel |

### Recommended Execution Order

```
Week 1-2:  [WS1] Build System & Release Pipeline
              ↓
Week 2-4:  [WS2] Installation ─────────────┐
           [WS3] Configuration (parallel) ──┤
              ↓                             │
Week 4-5:  [WS4] Self-Update ──────────────┤
              ↓                             │
Week 4-7:  [WS5] Tool Pack Registry ───────┤
              ↓                             │
Week 6-8:  [WS6] Plugin System ───────────┤
              ↓                             │
Week 6-9:  [WS7] Documentation ───────────┘
```

WS1 must be first — everything depends on releases working. WS2/WS3 run in parallel after WS1. WS4 is a quick win after WS1. WS5/WS6 run in parallel. WS7 is continuous and finalized last.

---

## Verification Plan

| # | Test | Description |
|---|------|-------------|
| V1 | Clean install test | Fresh WSL2 Ubuntu 24.04 → `curl \| bash` → `aibox setup` → `aibox start` → verify end-to-end |
| V2 | Upgrade test | Install v0.1.0, create config, install packs → upgrade to v0.2.0 via `aibox update --self` → verify config migrated, packs intact |
| V3 | Cross-platform build | Verify binaries produced for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 |
| V4 | Plugin test | `aibox plugin init` → build → `aibox plugin install` → verify in `aibox plugin list` → execute via `aibox <name>` |
| V5 | Tool pack registry test | Publish pack to GHCR → install on different machine → verify signature check |
| V6 | Air-gapped test | Download release artifacts to USB → install without internet → verify offline setup with local images |
| V7 | Config template test | `aibox config init --template minimal` → `aibox config validate` → `aibox start` → verify minimal config works |
| V8 | Rollback test | Install v0.2.0 → `aibox update --rollback` → verify reverts to v0.1.0 |

---

## File Summary

### New Files (25)

| File | Work Stream |
|------|-------------|
| `Makefile` (root) | WS1 |
| `.goreleaser.yaml` | WS1 |
| `.github/workflows/release.yaml` | WS1 |
| `.github/workflows/test.yaml` | WS1 |
| `scripts/install.sh` | WS2 |
| `cmd/aibox/internal/setup/preflight.go` | WS2 |
| `cmd/aibox/internal/setup/preflight_test.go` | WS2 |
| `cmd/aibox/internal/config/validation.go` | WS3 |
| `cmd/aibox/internal/config/validation_test.go` | WS3 |
| `cmd/aibox/internal/config/migration.go` | WS3 |
| `cmd/aibox/internal/config/migration_test.go` | WS3 |
| `cmd/aibox/configs/templates/minimal.yaml` | WS3 |
| `cmd/aibox/configs/templates/dev.yaml` | WS3 |
| `cmd/aibox/configs/templates/enterprise.yaml` | WS3 |
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
| `docs/installation.md` | WS7 |
| `docs/configuration.md` | WS7 |
| `docs/plugin-development.md` | WS7 |
| `docs/contributing.md` | WS7 |

### Modified Files (10)

| File | Work Stream | Changes |
|------|-------------|---------|
| `cmd/aibox/cmd/setup.go` | WS2 | Pre-flight checks, progress reporting |
| `cmd/aibox/internal/setup/linux.go` | WS2 | Enhanced progress output, post-validation |
| `cmd/aibox/internal/config/config.go` | WS3 | Generic defaults, `config_version`, embed templates |
| `cmd/aibox/cmd/config_cmd.go` | WS3 | Add `validate`, `migrate`, `init` subcommands |
| `cmd/aibox/cmd/update.go` | WS4 | Add `--self`, `--all`, `--rollback`, `--check` flags |
| `cmd/aibox/cmd/root.go` | WS4, WS6 | Background version check, dynamic plugin registration |
| `cmd/aibox/go.mod` | WS4, WS5 | Add `go-selfupdate`, `oras-go` dependencies |
| `cmd/aibox/internal/toolpacks/registry.go` | WS5 | Add OCI push/pull support |
| `aibox-toolpacks/packs/*/manifest.yaml` | WS5 | Add `version` field to all 12 manifests |
| `README.md` | WS7 | Install instructions, quickstart, badges |
