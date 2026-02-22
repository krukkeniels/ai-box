# Installation

> **Windows 11 users**: See the [Windows Installation Guide](windows-installation.md)
> for a complete walkthrough from enabling WSL2 to a working AI-Box setup.

## Quick install

The install script detects your OS, downloads the latest release, and places the binaries on your PATH:

```bash
curl -fsSL https://raw.githubusercontent.com/krukkeniels/ai-box/main/scripts/install.sh | bash
```

This installs four binaries: `aibox`, `aibox-credential-helper`, `aibox-llm-proxy`, and `aibox-git-remote-helper`.

Options:

```bash
# Install a specific version
curl -fsSL https://raw.githubusercontent.com/krukkeniels/ai-box/main/scripts/install.sh | bash -s -- --version v1.0.0

# Install to a custom directory
curl -fsSL https://raw.githubusercontent.com/krukkeniels/ai-box/main/scripts/install.sh | bash -s -- --dir ~/.local/bin
```

## Homebrew

Available for macOS and Linux:

```bash
brew install krukkeniels/aibox/aibox
```

This installs all four binaries and adds them to your PATH automatically.

## APT (Ubuntu / WSL2)

For Ubuntu and WSL2 systems, add the APT repository:

```bash
# Add repository key and source
curl -fsSL https://krukkeniels.github.io/ai-box/apt/gpg.key | sudo gpg --dearmor -o /usr/share/keyrings/aibox-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/aibox-archive-keyring.gpg] https://krukkeniels.github.io/ai-box/apt stable main" | sudo tee /etc/apt/sources.list.d/aibox.list

# Install
sudo apt-get update
sudo apt-get install -y aibox
```

## Manual download

Download binaries directly from [GitHub Releases](https://github.com/krukkeniels/ai-box/releases):

```bash
# Download the latest release archive
gh release download --repo krukkeniels/ai-box --pattern 'aibox_*_linux_amd64.tar.gz'

# Extract and install
tar xzf aibox_*_linux_amd64.tar.gz
sudo install -m 755 aibox /usr/local/bin/
sudo install -m 755 aibox-credential-helper /usr/local/bin/
sudo install -m 755 aibox-llm-proxy /usr/local/bin/
sudo install -m 755 aibox-git-remote-helper /usr/local/bin/
```

Available archives per platform:

| Platform | Archive |
|----------|---------|
| Linux amd64 | `aibox_<version>_linux_amd64.tar.gz` |
| Linux arm64 | `aibox_<version>_linux_arm64.tar.gz` |
| macOS amd64 | `aibox_<version>_darwin_amd64.tar.gz` |
| macOS arm64 | `aibox_<version>_darwin_arm64.tar.gz` |

## Build from source

Requires Go 1.24+:

```bash
git clone https://github.com/krukkeniels/ai-box.git
cd ai-box
make build       # Builds all 4 binaries
make install     # Installs to /usr/local/bin (may require sudo)
```

To build individual binaries:

```bash
make build-aibox
make build-credential-helper
make build-llm-proxy
make build-git-remote-helper
```

---

## Prerequisites

AI-Box requires:

| Requirement | Minimum | Recommended |
|-------------|---------|-------------|
| OS | Linux (native or WSL2 with kernel 5.15+) | Ubuntu 22.04+ |
| Container runtime | Podman 4.x (rootless) or Docker 24+ (rootless) | Podman 5.x |
| gVisor | runsc (any recent release) | release-20240401.0+ |
| Disk space | 10 GB free | 20+ GB |
| RAM | 8 GB | 16+ GB |

### Installing prerequisites

**Podman (Ubuntu/Debian):**

```bash
sudo apt-get update
sudo apt-get install -y podman
```

**gVisor:**

```bash
curl -fsSL https://gvisor.dev/archive.key | sudo gpg --dearmor -o /usr/share/keyrings/gvisor-archive-keyring.gpg
echo 'deb [signed-by=/usr/share/keyrings/gvisor-archive-keyring.gpg] https://storage.googleapis.com/gvisor/releases release main' | sudo tee /etc/apt/sources.list.d/gvisor.list
sudo apt-get update && sudo apt-get install -y runsc
sudo runsc install
```

---

## Post-install setup

After installing the binaries, run the one-time setup:

```bash
aibox setup
```

This detects your container runtime, creates config and SSH keys, and pulls the base image. For enterprise environments, an admin should also run `sudo aibox setup --system` to install security profiles (seccomp, AppArmor, network stack).

### Verification

```bash
aibox doctor
```

All checks should show `PASS`. If any fail, see [Troubleshooting](#troubleshooting) below.

---

## Troubleshooting

### `aibox: command not found`

The binary is not on your PATH. The install script places binaries in `~/.local/bin` by default.

```bash
# Add to your shell profile (~/.bashrc or ~/.zshrc)
export PATH="$HOME/.local/bin:$PATH"

# Then reload
source ~/.bashrc
```

### `podman: command not found`

Install Podman:

```bash
# Ubuntu/Debian
sudo apt-get install -y podman

# Fedora
sudo dnf install -y podman
```

### `runsc (gVisor) not found`

gVisor is optional but strongly recommended. Without it, AI-Box uses seccomp-only isolation. See [Installing prerequisites](#installing-prerequisites) above for install commands.

### Permission denied during `aibox setup`

User setup does not require root. If you see permission errors:

- Make sure you are **not** running with `sudo` (user setup should run as your normal user)
- System setup (`sudo aibox setup --system`) requires root for installing security profiles

### WSL2: not enough memory

WSL2 defaults to half your system RAM. If builds run out of memory:

1. Create or edit `%USERPROFILE%\.wslconfig`:

```ini
[wsl2]
memory=16GB
processors=8
```

2. Restart WSL: `wsl --shutdown` then reopen your terminal.

### WSL2: gVisor not working

WSL2 kernel must be 5.15+ for gVisor systrap mode. Update WSL:

```bash
wsl --update
```

If gVisor still fails, AI-Box falls back to seccomp-only isolation (still strong).

### Image pull fails (403 Forbidden or timeout)

If the base image cannot be pulled (GHCR package not yet published, air-gapped network, registry unreachable), build it locally from source:

```bash
# Clone the repo (if you haven't already)
git clone https://github.com/krukkeniels/ai-box.git
cd ai-box

# Build the base image locally (~5 minutes)
make image-base

# Verify
podman images | grep aibox
```

To build a language-specific variant instead:

```bash
make image-java    # JDK 21 + Maven + Gradle
make image-node    # Node.js 20 + Yarn
make image-dotnet  # .NET SDK 8
make image-full    # All of the above + Python + Bazel
```

Enterprise users with an internal Harbor registry:

```bash
make image-base IMAGE_REGISTRY=harbor.internal/aibox
```

### Seccomp profile not found

```bash
# Install via setup
sudo aibox setup --system

# Or install manually
sudo mkdir -p /etc/aibox
sudo cp configs/seccomp.json /etc/aibox/seccomp.json
```

---

## Upgrading

### Install script

Re-run the install script to get the latest version:

```bash
curl -fsSL https://raw.githubusercontent.com/krukkeniels/ai-box/main/scripts/install.sh | bash
```

Or specify a version:

```bash
curl -fsSL https://raw.githubusercontent.com/krukkeniels/ai-box/main/scripts/install.sh | bash -s -- --version v1.1.0
```

### Homebrew

```bash
brew upgrade krukkeniels/aibox/aibox
```

### APT

```bash
sudo apt-get update
sudo apt-get upgrade aibox
```

### Container images

After upgrading the CLI, update container images:

```bash
aibox update
```

---

## Uninstall

### Install script

```bash
sudo rm /usr/local/bin/aibox /usr/local/bin/aibox-credential-helper \
        /usr/local/bin/aibox-llm-proxy /usr/local/bin/aibox-git-remote-helper

# Or if installed to ~/.local/bin
rm ~/.local/bin/aibox ~/.local/bin/aibox-credential-helper \
   ~/.local/bin/aibox-llm-proxy ~/.local/bin/aibox-git-remote-helper
```

### Homebrew

```bash
brew uninstall krukkeniels/aibox/aibox
```

### APT

```bash
sudo apt-get remove aibox
```

### Clean up configuration and data

```bash
# Remove user config
rm -rf ~/.config/aibox

# Remove system config (requires root)
sudo rm -rf /etc/aibox

# Remove cached container images
podman rmi ghcr.io/krukkeniels/aibox/base:24.04

# Remove build cache volumes
podman volume rm aibox-build-cache 2>/dev/null
```
