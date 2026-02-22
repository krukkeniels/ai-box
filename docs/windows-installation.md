# Windows 11 Installation Guide

Complete walkthrough from a fresh Windows 11 machine to a working AI-Box setup.

**Audience**: Developers on Windows 11 with no prior WSL2 experience.
**Estimated time**: 15-20 minutes.

**Prerequisites summary**:

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| OS | Windows 11 23H2 | Windows 11 24H2 |
| RAM | 16 GB | 32 GB |
| CPU | 8 cores | 8+ cores |
| Storage | 256 GB SSD | 512 GB NVMe SSD |

AI-Box does not run natively on Windows. It requires WSL2 (Windows Subsystem for Linux).

---

## Step 1: Enable WSL2

Open **PowerShell as Administrator** and run:

```powershell
wsl --install -d Ubuntu-24.04
```

This enables the WSL2 feature, installs the Linux kernel, and sets up Ubuntu 24.04. **Reboot when prompted.**

After rebooting, Ubuntu will finish setup and ask you to create a username and password.

### Manual fallback

If `wsl --install` fails (e.g., on managed machines), enable the features manually:

```powershell
dism.exe /online /enable-feature /featurename:Microsoft-Windows-Subsystem-Linux /all /norestart
dism.exe /online /enable-feature /featurename:VirtualMachinePlatform /all /norestart
```

Reboot, then:

```powershell
wsl --set-default-version 2
wsl --install -d Ubuntu-24.04
```

### Verify

```powershell
wsl --list --verbose
```

You should see Ubuntu-24.04 with VERSION 2:

```
  NAME            STATE           VERSION
* Ubuntu-24.04    Running         2
```

Update the WSL kernel to the latest version:

```powershell
wsl --update
```

---

## Step 2: Configure .wslconfig

Create (or edit) the file `%USERPROFILE%\.wslconfig` on the **Windows side**. Open it in Notepad:

```powershell
notepad "$env:USERPROFILE\.wslconfig"
```

### Recommended config (32 GB machines)

```ini
[wsl2]
memory=24GB
swap=8GB
processors=8
localhostForwarding=true
nestedVirtualization=true

[experimental]
autoMemoryReclaim=dropcache
sparseVhd=true
```

### Light config (16 GB machines)

```ini
[wsl2]
memory=12GB
swap=4GB
processors=8
localhostForwarding=true

[experimental]
autoMemoryReclaim=dropcache
sparseVhd=true
```

### What each setting does

| Setting | Purpose |
|---------|---------|
| `memory` | Maximum RAM WSL2 can use. Polyglot workloads peak at 18-22 GB; 24 GB provides headroom. |
| `swap` | Swap space prevents OOM kills during memory spikes (e.g., Gradle + Bazel). |
| `processors` | Number of CPU cores. Match your host core count. |
| `localhostForwarding` | Allows accessing WSL2 services (dev servers) from Windows via `localhost`. |
| `nestedVirtualization` | Required for some container runtimes. |
| `autoMemoryReclaim` | Reclaims page cache when WSL2 is idle. Prevents the "vmmem eating all my RAM" problem. |
| `sparseVhd` | Allows the WSL2 virtual disk to shrink. Without this, disk usage only grows. |

After saving, restart WSL:

```powershell
wsl --shutdown
```

Then reopen your Ubuntu terminal.

---

## Step 3: First steps in WSL2

Open the Ubuntu terminal (from Start menu or Windows Terminal) and update packages:

```bash
sudo apt update && sudo apt upgrade -y
```

Verify your environment:

```bash
# Kernel version (should be 5.15+, ideally 6.x)
uname -r

# Available memory (should match your .wslconfig setting)
free -h

# Available disk space
df -h /
```

---

## Step 4: Install prerequisites

AI-Box requires Podman (container runtime) and gVisor (kernel isolation). Run these inside WSL2.

**Podman:**

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

gVisor requires WSL2 kernel 5.15+. If `uname -r` shows an older version, update WSL from PowerShell:

```powershell
wsl --update
```

---

## Step 5: Install AI-Box

**All commands in this section must run inside WSL2, not in PowerShell.**

### APT (recommended for Ubuntu / WSL2)

```bash
# Add repository key and source
curl -fsSL https://krukkeniels.github.io/ai-box/apt/gpg.key | sudo gpg --dearmor -o /usr/share/keyrings/aibox-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/aibox-archive-keyring.gpg] https://krukkeniels.github.io/ai-box/apt stable main" | sudo tee /etc/apt/sources.list.d/aibox.list

# Install
sudo apt-get update
sudo apt-get install -y aibox
```

### Alternative: install script

```bash
curl -fsSL https://raw.githubusercontent.com/krukkeniels/ai-box/main/scripts/install.sh | bash
```

See [installation.md](installation.md) for all install methods (Homebrew, manual download, build from source).

---

## Step 6: Setup and verify

Run the one-time setup (auto-detects WSL2):

```bash
aibox setup
```

Then verify everything is working:

```bash
aibox doctor
```

All checks should show `PASS`. If any fail, see [Troubleshooting](#troubleshooting) below.

---

## Step 7: Connect your IDE

### VS Code (Remote - WSL)

1. Install [VS Code](https://code.visualstudio.com/) on Windows.
2. Install the **WSL** extension.
3. Click the bottom-left `><` icon and select **Connect to WSL**.
4. Open your project folder inside WSL2.

See the [VS Code quickstart](training/quickstart-vscode.md) for AI-Box-specific IDE setup.

### JetBrains (Gateway)

1. Install [JetBrains Gateway](https://www.jetbrains.com/remote-development/gateway/) on Windows.
2. Connect to your WSL2 instance.
3. Open your project folder.

See the [JetBrains quickstart](training/quickstart-jetbrains.md) for AI-Box-specific IDE setup.

---

## Step 8: Launch your first sandbox

```bash
# Start a sandbox with your project
aibox start --workspace ~/my-project

# Open a terminal inside the sandbox
aibox shell

# When finished, stop the sandbox (credentials are auto-revoked)
aibox stop
```

You now have a working AI-Box setup on Windows 11.

---

## Troubleshooting

Windows-specific issues. For general issues see the [troubleshooting FAQ](training/troubleshooting-faq.md).

### VHD grows but never shrinks

WSL2's virtual hard disk grows as you use it but does not shrink automatically.

**Fix**: Add `sparseVhd=true` to `[experimental]` in `.wslconfig` (see [Step 2](#step-2-configure-wslconfig)).

### vmmem process consuming all RAM

WSL2 holds page cache and appears to "leak" memory on the Windows side.

**Fix**: Add `autoMemoryReclaim=dropcache` to `[experimental]` in `.wslconfig`. Then restart WSL:

```powershell
wsl --shutdown
```

### DNS breaks after VPN connect

WSL2 DNS can stop working when the Windows host connects to a VPN.

**Fix**: Disable automatic DNS generation and set a static resolver:

```bash
# Inside WSL2
sudo bash -c 'echo "[network]
generateResolvConf=false" > /etc/wsl.conf'

sudo bash -c 'echo "nameserver 8.8.8.8
nameserver 8.8.4.4" > /etc/resolv.conf'

sudo chattr +i /etc/resolv.conf
```

Then restart WSL (`wsl --shutdown` from PowerShell). Replace `8.8.8.8` with your corporate DNS if needed.

### Clock drift after sleep/hibernate

WSL2's clock can drift from the host after sleep or hibernate, causing TLS certificate errors and build timestamp issues.

**Fix**:

```bash
sudo hwclock -s
```

Or restart WSL entirely (`wsl --shutdown` from PowerShell).

### localhost port forwarding not working

Dev servers running inside WSL2 are not reachable from Windows via `localhost`.

**Fix**: Ensure `localhostForwarding=true` is set in `.wslconfig` under `[wsl2]` (see [Step 2](#step-2-configure-wslconfig)).

### gVisor not working

gVisor requires WSL2 kernel 5.15+. Check your kernel version:

```bash
uname -r
```

If it's below 5.15, update from PowerShell:

```powershell
wsl --update
```

If gVisor still fails after updating, AI-Box falls back to seccomp-only isolation (still strong).

### Slow first file access after WSL2 start

Initial file access after WSL2 starts can be slow (10-15 seconds) while the virtual filesystem warms up.

**Workaround**: Wait a few seconds after opening the terminal before running `aibox start`.

---

## Known limitations

Some stack-specific issues exist when running under gVisor in WSL2:

- **Bazel**: Requires `--spawn_strategy=local` (sandbox-within-gVisor not supported)
- **JVM profiling**: JFR / `perf_event_open` not available under gVisor
- **AngularJS (legacy)**: PhantomJS does not work under gVisor; migrate to headless Chrome
- **.NET watch**: Increased latency; use `--no-hot-reload` if watch mode is slow
- **PowerShell**: PSReadLine reduced features; set `$env:TERM = "xterm-256color"`

See [WSL2 Validation Report](wsl2-validation.md) for full details, per-stack resource budgets, and test cases.

---

## Next steps

- [Installation guide](installation.md) -- alternative install methods, upgrading, uninstalling
- [Configuration guide](configuration.md) -- all config options and environment variables
- [VS Code quickstart](training/quickstart-vscode.md) -- IDE-specific setup
- [JetBrains quickstart](training/quickstart-jetbrains.md) -- IDE-specific setup
- [Troubleshooting FAQ](training/troubleshooting-faq.md) -- general issues and solutions
