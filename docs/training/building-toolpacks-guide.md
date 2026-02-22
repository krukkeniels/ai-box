# Building Tool Packs for AI-Box

**Deliverable**: D12
**Audience**: Power users, champions, and contributors
**Last Updated**: 2026-02-21

---

## Overview

Tool packs add language runtimes, build tools, and utilities to AI-Box sandboxes without rebuilding the base image. This guide covers creating, testing, and submitting tool packs.

---

## How Tool Packs Work

A tool pack is a directory containing:
- `manifest.yaml` -- metadata, dependencies, network requirements, install instructions
- `install.sh` -- installation script executed inside the container

When a developer runs `aibox install <pack>@<version>`, the CLI:
1. Fetches the manifest from the tool pack registry
2. Validates the manifest against the project's OPA policy
3. Runs the install script inside the running container
4. Verifies the installation using the `verify` checks in the manifest

Tool packs are signed by the maintainer and verified by Cosign before installation.

---

## Manifest Schema

Every tool pack requires a `manifest.yaml`. Here is the complete schema:

```yaml
# Required fields
name: string            # Pack name (lowercase, alphanumeric + hyphens)
version: string         # Version (semver or upstream version)
description: string     # One-line description
maintainer: string      # Maintainer identity (team or email)

# Installation
install:
  method: string        # "script" (required for custom packs)
  script: string        # Relative path to install script (default: install.sh)

# Network requirements (optional)
network:
  requires:             # List of network endpoints the pack needs
    - id: string        # Unique identifier for this endpoint
      hosts: [string]   # Hostnames (must be Nexus mirrors, not external)
      ports: [int]      # Port numbers

# Filesystem (optional)
filesystem:
  creates: [string]     # Directories the pack creates
  caches: [string]      # Directories to persist as cache volumes

# Resource requirements (optional)
resources:
  min_memory: string    # e.g., "2GB"
  recommended_memory: string  # e.g., "4GB"

# Security (required for submission)
security:
  signed_by: string     # Signer identity

# Environment variables set by the pack (optional)
environment:
  KEY: "value"          # Environment variables added to shell

# Verification checks (recommended)
verify:
  - command: string     # Command to run after install
    expect_exit_code: int  # Expected exit code (usually 0)

# Metadata
tags: [string]          # Categorization tags (e.g., "language", "jvm", "build-tool")
```

---

## Worked Example 1: Simple Pack (Single Binary)

Let's create a tool pack for the `ripgrep` search tool.

### Directory Structure

```
aibox-toolpacks/packs/ripgrep/
  manifest.yaml
  install.sh
```

### manifest.yaml

```yaml
name: ripgrep
version: "14.1.0"
description: "ripgrep (rg) - fast recursive search tool"
maintainer: platform-team

install:
  method: script
  script: install.sh

network:
  requires: []

filesystem:
  creates:
    - "/opt/toolpacks/ripgrep"

resources:
  min_memory: "0"
  recommended_memory: "0"

security:
  signed_by: platform-team@aibox.internal

environment:
  PATH: "/opt/toolpacks/ripgrep/bin:${PATH}"

verify:
  - command: "rg --version"
    expect_exit_code: 0

tags:
  - tool
  - search
```

### install.sh

```bash
#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="/opt/toolpacks/ripgrep"
VERSION="14.1.0"
ARCH="$(uname -m)"

case "${ARCH}" in
  x86_64)  ARCH_SUFFIX="x86_64-unknown-linux-musl" ;;
  aarch64) ARCH_SUFFIX="aarch64-unknown-linux-gnu" ;;
  *) echo "Unsupported architecture: ${ARCH}" >&2; exit 1 ;;
esac

TARBALL="ripgrep-${VERSION}-${ARCH_SUFFIX}.tar.gz"
URL="https://nexus.internal/repository/raw-hosted/ripgrep/${VERSION}/${TARBALL}"

mkdir -p "${INSTALL_DIR}/bin"

# Download from internal Nexus mirror (not external)
curl -fsSL "${URL}" | tar xz -C "${INSTALL_DIR}" --strip-components=1
ln -sf "${INSTALL_DIR}/rg" "${INSTALL_DIR}/bin/rg"

echo "ripgrep ${VERSION} installed to ${INSTALL_DIR}"
```

**Key points**:
- Downloads from `nexus.internal`, never from external sources directly
- Uses `set -euo pipefail` for safe scripting
- Handles multiple architectures
- Creates the directory structure declared in the manifest

---

## Worked Example 2: Complex Pack (Multi-Dependency)

Let's create a tool pack for a Rust development environment.

### Directory Structure

```
aibox-toolpacks/packs/rust/
  manifest.yaml
  install.sh
```

### manifest.yaml

```yaml
name: rust
version: "1.78.0"
description: "Rust 1.78 toolchain (rustc, cargo, clippy, rustfmt)"
maintainer: platform-team

install:
  method: script
  script: install.sh

network:
  requires:
    - id: crates-mirror
      hosts: ["nexus.internal"]
      ports: [443]

filesystem:
  creates:
    - "/opt/toolpacks/rust"
    - "/opt/toolpacks/rust/bin"
  caches:
    - "$HOME/.cargo/registry"
    - "$HOME/.cargo/git"
    - "$HOME/.rustup"

resources:
  min_memory: "2GB"
  recommended_memory: "4GB"

security:
  signed_by: platform-team@aibox.internal

environment:
  RUSTUP_HOME: "/opt/toolpacks/rust/rustup"
  CARGO_HOME: "/opt/toolpacks/rust/cargo"
  PATH: "/opt/toolpacks/rust/cargo/bin:${PATH}"
  CARGO_REGISTRIES_INTERNAL_INDEX: "sparse+https://nexus.internal/repository/crates-group/"

verify:
  - command: "rustc --version"
    expect_exit_code: 0
  - command: "cargo --version"
    expect_exit_code: 0
  - command: "clippy-driver --version"
    expect_exit_code: 0
  - command: "rustfmt --version"
    expect_exit_code: 0

tags:
  - language
  - systems
```

### install.sh

```bash
#!/usr/bin/env bash
set -euo pipefail

VERSION="1.78.0"
INSTALL_DIR="/opt/toolpacks/rust"
RUSTUP_HOME="${INSTALL_DIR}/rustup"
CARGO_HOME="${INSTALL_DIR}/cargo"

export RUSTUP_HOME CARGO_HOME

mkdir -p "${INSTALL_DIR}" "${RUSTUP_HOME}" "${CARGO_HOME}"

# Download rustup-init from internal mirror
RUSTUP_URL="https://nexus.internal/repository/raw-hosted/rust/rustup-init"
curl -fsSL "${RUSTUP_URL}" -o /tmp/rustup-init
chmod +x /tmp/rustup-init

# Install specific toolchain version
/tmp/rustup-init -y \
  --default-toolchain "${VERSION}" \
  --component clippy rustfmt \
  --no-modify-path

rm /tmp/rustup-init

# Configure Cargo to use internal crates mirror
mkdir -p "${CARGO_HOME}"
cat > "${CARGO_HOME}/config.toml" << 'TOML'
[registries.internal]
index = "sparse+https://nexus.internal/repository/crates-group/"

[source.crates-io]
replace-with = "internal-mirror"

[source.internal-mirror]
registry = "sparse+https://nexus.internal/repository/crates-group/"
TOML

echo "Rust ${VERSION} installed to ${INSTALL_DIR}"
echo "Components: rustc, cargo, clippy, rustfmt"
```

**Key points for complex packs**:
- Declares cache directories that persist between sessions
- Configures the package manager to use internal mirrors
- Installs multiple components in a single script
- Sets environment variables so tools are on `PATH`
- Verification checks cover all installed components

---

## Testing Locally

### Step 1: Validate the manifest

```bash
# From the aibox-toolpacks directory
aibox policy validate --toolpack packs/rust/manifest.yaml
```

This checks:
- Required fields are present
- Network requirements are satisfiable under current policy
- No policy violations in filesystem paths

### Step 2: Test the install script in a sandbox

```bash
# Start a sandbox
aibox start --workspace ~/projects/test-project

# Shell into the sandbox
aibox shell

# Run the install script manually
cd /tmp
cp -r /workspace/aibox-toolpacks/packs/rust .
chmod +x rust/install.sh
bash rust/install.sh

# Verify
rustc --version
cargo --version
```

### Step 3: Test the full install flow

```bash
# From a clean sandbox
aibox install rust@1.78.0 --local-pack ./packs/rust

# This simulates the full install: manifest validation, script execution, verification
```

### Step 4: Test with a real project

```bash
# Start a sandbox with your new pack
aibox start --workspace ~/projects/rust-project --toolpacks rust@1.78.0

# Build a real project to verify everything works
cd /workspace
cargo build
cargo test
```

---

## Submitting a Tool Pack

### Submission Process

1. **Create a branch** in the `aibox-toolpacks` repository:
   ```bash
   git checkout -b add-rust-toolpack
   ```

2. **Add your pack** under `packs/<name>/`:
   ```
   packs/rust/manifest.yaml
   packs/rust/install.sh
   ```

3. **Upload binaries to Nexus**: Tool pack binaries must be hosted on `nexus.internal`, not fetched from external sources at install time. Work with the platform team to upload binaries.

4. **Submit a PR** with:
   - Description of what the pack installs
   - Why it is needed (which teams, how many developers)
   - Testing evidence (output of install + verification)

5. **Review process**:
   - Platform team reviews manifest and install script
   - Security team reviews if the pack introduces new network endpoints or elevated permissions
   - Champion testing before merge (for packs used by multiple teams)

### PR Template

```markdown
## New Tool Pack: <name>

**What**: <brief description>
**Why**: <which teams need it, how many developers>
**Version**: <upstream version>

### Checklist
- [ ] manifest.yaml passes `aibox policy validate`
- [ ] install.sh tested in sandbox
- [ ] All verify checks pass
- [ ] Binaries hosted on nexus.internal
- [ ] No external network endpoints required (or documented)
- [ ] Cache directories declared for persistent data
- [ ] Environment variables set correctly
```

### Review SLA

| Type | SLA |
|------|-----|
| Known/registered tools | 1 business day |
| New tools (no new network endpoints) | 3 business days |
| New tools (new network endpoints) | 5 business days (security review required) |
| Emergency request | Same day (platform team approval) |

---

## Governance

### Who Can Do What

| Role | Permissions |
|------|------------|
| **Platform team** | Create, update, delete any pack. Approve all PRs |
| **Security team** | Review packs with new network requirements. Approve/block based on supply chain risk |
| **Team leads** | Approve team-specific packs within policy |
| **Champions** | Submit and test PRs. Review is advisory, not approving |
| **Developers** | Request packs via champion or `#aibox-help` |

### Pack Maintenance

- **Monthly updates**: Platform team updates pack versions monthly (upstream releases)
- **Security patches**: Critical CVEs trigger immediate pack updates
- **Deprecation**: Old versions deprecated with 30-day notice. Developers using deprecated packs see a warning on `aibox start`

### Supply Chain Security

All tool packs must:
- Download binaries only from `nexus.internal` (pre-uploaded, not proxied at install time for security-critical tools)
- Be signed by the maintainer using Cosign
- Have checksums verified during installation
- Not execute arbitrary code from the network at install time
- Be reviewed by the security team if they introduce new network endpoints

---

## Common Patterns

### Adding a PATH entry

```yaml
environment:
  PATH: "/opt/toolpacks/<name>/bin:${PATH}"
```

### Persisting cache between sessions

```yaml
filesystem:
  caches:
    - "$HOME/.<tool>/cache"
```

### Requiring another tool pack

There is no formal dependency declaration yet. Document prerequisites in the `description`:

```yaml
description: "Angular CLI 18 (requires node@20 tool pack)"
```

And check in the install script:

```bash
if ! command -v node &> /dev/null; then
  echo "ERROR: node@20 tool pack must be installed first" >&2
  exit 1
fi
```

### Multi-architecture support

```bash
ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64)  SUFFIX="amd64" ;;
  aarch64) SUFFIX="arm64" ;;
  *) echo "Unsupported: ${ARCH}" >&2; exit 1 ;;
esac
```

---

## Troubleshooting Pack Development

### Install script fails with "permission denied"

- Install scripts run as the sandbox user (not root)
- Install to `/opt/toolpacks/<name>` (writable by the sandbox user)
- Do not install to `/usr/bin` or `/usr/local` (read-only rootfs)

### Verification check fails

- The verify command runs after install completes
- Ensure environment variables (PATH, etc.) are set before verify runs
- Test the verify command manually in the sandbox

### Network endpoint blocked by policy

- All downloads must go through `nexus.internal`
- If you need a new external mirror in Nexus, request it via `#aibox-help`
- Direct downloads from external URLs will be blocked by the proxy

---

*Questions about building tool packs? Ask in `#aibox-champions` or `#aibox-help`.*
