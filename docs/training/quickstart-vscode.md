# AI-Box Quickstart: VS Code on Windows 11

**Deliverable**: D9
**Audience**: VS Code users on Windows 11 + WSL2
**Prerequisites**: Windows 11 (build 22000+), administrator access for initial setup

---

## Overview

This guide takes you from zero to a working AI-Box sandbox connected to VS Code. Total time: 5-10 minutes (first time), under 1 minute (subsequent starts).

---

## Step 1: Install AI-Box

Open **PowerShell** (or Windows Terminal) and run:

```powershell
winget install aibox
```

Or download from the internal portal: `https://portal.internal/aibox/releases`

Verify the installation:

```powershell
aibox version
```

You should see the version number and build date.

---

## Step 2: Run Initial Setup

```powershell
aibox setup
```

This command performs one-time setup:
- Installs and configures Podman in WSL2
- Configures the gVisor (runsc) runtime
- Sets up the egress proxy (Squid) and DNS (CoreDNS)
- Configures nftables network rules
- Pulls the base AI-Box image from Harbor
- Validates your machine meets minimum requirements

**Expected duration**: 3-5 minutes (first time only).

If setup fails, run `aibox doctor` to see what went wrong. Common issues:

| Issue | Fix |
|-------|-----|
| WSL2 not installed | Run `wsl --install` in PowerShell as admin, restart |
| WSL2 outdated | Run `wsl --update` in PowerShell as admin |
| Insufficient RAM | AI-Box needs 16GB minimum. Check `.wslconfig` (see Step 2a) |

### Step 2a: Configure WSL2 Memory (if needed)

If you have 16GB RAM, WSL2 may need explicit memory allocation. Create or edit `%USERPROFILE%\.wslconfig`:

```ini
[wsl2]
memory=12GB
processors=6
swap=4GB
```

For polyglot workloads (JVM + Node + AI tools), 32GB total RAM with 24GB allocated to WSL2 is recommended.

Restart WSL2 after editing:

```powershell
wsl --shutdown
```

---

## Step 3: Start Your Sandbox

```powershell
aibox start --workspace ~/projects/my-service
```

Add tool packs for your project's language:

```powershell
# Java project
aibox start --workspace ~/projects/my-service --toolpacks java@21

# Node.js project
aibox start --workspace ~/projects/my-app --toolpacks node@20

# .NET project
aibox start --workspace ~/projects/my-api --toolpacks dotnet@8

# Multiple languages
aibox start --workspace ~/projects/fullstack --toolpacks java@21,node@20
```

**First start** (cold): up to 90 seconds (image layers download).
**Subsequent starts** (warm): ~15 seconds.

When ready, you will see:

```
Sandbox ready.
  SSH:       localhost:2222
  Workspace: /workspace
  Tools:     java@21
  Status:    aibox status
```

---

## Step 4: Connect VS Code

### Option A: Automatic (Recommended)

AI-Box configures your SSH config automatically. In VS Code:

1. Open the **Command Palette** (`Ctrl+Shift+P`)
2. Type **"Remote-SSH: Connect to Host..."**
3. Select **`aibox`** from the list
4. VS Code connects and opens a new window

### Option B: Manual

If `aibox` does not appear in the host list:

1. Open the **Command Palette** (`Ctrl+Shift+P`)
2. Type **"Remote-SSH: Connect to Host..."**
3. Enter: `aibox@localhost -p 2222`
4. VS Code connects

### Option C: Command Line

```powershell
code --remote ssh-remote+aibox /workspace
```

### First Connection

On first connection, VS Code Server installs inside the container. This takes 30-60 seconds. Subsequent connections are instant.

After connecting:
- The **Explorer** panel shows your project files under `/workspace`
- The **Terminal** opens inside the sandbox
- All extensions run inside the container

---

## Step 5: Use AI Tools

Open the integrated terminal in VS Code (`Ctrl+``) and start an AI tool:

```bash
# Claude Code (API key auto-injected)
claude

# Codex CLI (API key auto-injected)
codex
```

API keys are injected automatically by the credential broker. You do not need to configure them.

AI tools can:
- Read and edit files in `/workspace`
- Run commands in the terminal
- Access allowed network endpoints (LLM API, package mirrors, Git server)

AI tools cannot:
- Access files outside `/workspace`
- Reach unauthorized network endpoints
- Escalate privileges or escape the container

---

## Step 6: Build, Test, and Push

All development workflows work normally inside the sandbox:

```bash
# Java
gradle build
gradle test

# Node.js
npm install
npm test

# .NET
dotnet build
dotnet test

# Git
git add .
git commit -m "feat: add feature"
git push
```

Build caches persist between sessions in named volumes. Incremental builds are fast.

Dependencies are resolved through internal mirrors (Nexus) automatically. No direct internet access.

---

## Step 7: Stop When Done

```bash
# From the host terminal (PowerShell)
aibox stop
```

Your workspace and build caches persist. Next time you run `aibox start`, everything is where you left it.

---

## Day-to-Day Workflow

After initial setup, your daily workflow is:

```
1. aibox start --workspace ~/projects/my-service    (15 seconds)
2. Open VS Code, connect to aibox                    (automatic)
3. Code, build, test, use AI tools                   (normal workflow)
4. git push                                          (as usual)
5. aibox stop                                        (when done)
```

---

## Working on Multiple Projects

Each project gets its own sandbox:

```bash
aibox start --workspace ~/projects/service-a --project service-a --toolpacks java@21
aibox start --workspace ~/projects/frontend  --project frontend  --toolpacks node@20
```

The CLI warns at 80% resource usage when running multiple sandboxes.

---

## Installing Additional Tool Packs

Need a tool that is not in your current sandbox?

```bash
# List available packs
aibox install --list

# Install into running sandbox
aibox install bazel@7
aibox install python@3.12

# Enable MCP servers
aibox mcp enable filesystem-mcp git-mcp
aibox mcp list
```

---

## Telemetry

AI-Box collects operational metrics only (startup times, success/fail, image versions). No command history, file paths, or AI prompts are ever collected.

```bash
# See what telemetry is collected
aibox telemetry show

# Opt out of optional telemetry
aibox telemetry opt-out
```

---

## Keeping Up to Date

```bash
# Check for updates
aibox update

# Update pulls the latest signed image
# Running containers are not disrupted; update takes effect on next start
```

If `aibox start` refuses to launch with a message about an outdated image, a critical security update is required. Run `aibox update` first.

---

## Troubleshooting

### "Could not establish connection to host"

1. Check that the sandbox is running: `aibox status`
2. If not running, start it: `aibox start --workspace ~/projects/my-service`
3. If running, check the SSH port: output of `aibox status` shows the port
4. Ensure no other process is using port 2222: `netstat -an | findstr 2222`
5. Try restarting: `aibox stop && aibox start --workspace ~/projects/my-service`

### VS Code reconnect loop

1. Stop the sandbox: `aibox stop`
2. Clear VS Code Server cache: delete `~/.vscode-server` inside the container (or `aibox repair cache`)
3. Restart: `aibox start --workspace ~/projects/my-service`
4. Reconnect VS Code

### Extensions not working

Extensions run inside the container. If an extension needs network access (e.g., extension marketplace):
- Check if the extension is pre-approved (baked into the image)
- If not, request it via your team champion or `#aibox-help`

All telemetry is disabled by default. Extensions that require telemetry endpoints may not function.

### Slow performance

1. Check WSL2 memory allocation: `wsl -- free -h` (should show 12GB+)
2. Check container resource usage: `aibox status`
3. For JVM projects: ensure at least 8GB allocated to WSL2
4. For polyglot (JVM + Node + AI): ensure 16GB+ allocated to WSL2
5. Check disk space: `aibox doctor` reports disk status

### Build dependencies fail to download

1. Run `aibox network test` to verify connectivity to Nexus
2. Check that the correct tool pack is installed (e.g., `java@21` for Maven)
3. Build caches from previous sessions may resolve the issue on retry
4. If Nexus is down, builds using cached dependencies still work

### "Blocked by policy" message

1. Note the log entry ID in the message
2. Run `aibox policy explain --log-entry <id>`
3. The output explains which policy rule blocked the action and why
4. If you believe the block is incorrect, contact your champion or post in `#aibox-help`

### Nothing works

Run the full diagnostic:

```bash
aibox doctor
```

Share the output with your team champion or post in `#aibox-help`.

---

## Quick Reference

```
aibox setup                              # One-time setup
aibox start --workspace <path>           # Start sandbox
aibox start --toolpacks java@21,node@20  # Start with tool packs
aibox stop                               # Stop sandbox
aibox status                             # Check status
aibox doctor                             # Diagnose issues
aibox update                             # Pull latest image
aibox install <pack>@<version>           # Add tool pack
aibox shell                              # Open shell
aibox network test                       # Test connectivity
aibox policy validate                    # Validate policy
aibox policy explain --log-entry <id>    # Explain block
aibox mcp enable <pack>                  # Enable MCP server
aibox repair cache                       # Fix cache issues
```

---

*Having trouble? Run `aibox doctor` first, then check the [Troubleshooting FAQ](troubleshooting-faq.md). Your team's AI-Box champion can also help.*
