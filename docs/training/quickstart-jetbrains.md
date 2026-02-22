# AI-Box Quickstart: JetBrains IDEs on Windows 11

**Deliverable**: D10
**Audience**: IntelliJ IDEA, WebStorm, PyCharm, and Rider users on Windows 11 + WSL2
**Prerequisites**: Windows 11 (build 22000+), JetBrains Gateway installed, administrator access for initial setup

---

## Overview

JetBrains IDEs connect to AI-Box through **JetBrains Gateway**, which runs a backend IDE process inside the sandbox while keeping the thin-client frontend on your local machine. This guide covers setup through first build.

---

## Step 1: Install AI-Box

Open **PowerShell** (or Windows Terminal) and run:

```powershell
winget install aibox
```

Or download from the internal portal: `https://portal.internal/aibox/releases`

Verify:

```powershell
aibox version
```

---

## Step 2: Run Initial Setup

```powershell
aibox setup
```

This performs one-time setup (Podman, gVisor, proxy, DNS, image pull). Takes 3-5 minutes.

If setup fails, run `aibox doctor` for diagnostics.

### WSL2 Memory Configuration

JetBrains backends are memory-intensive. Edit `%USERPROFILE%\.wslconfig`:

```ini
[wsl2]
memory=16GB
processors=6
swap=4GB
```

**Minimum**: 12GB for WSL2 (JetBrains backend needs 2-4GB + build tools + AI agent).
**Recommended**: 16GB+ for WSL2 (especially for Java/Scala projects with large indexes).

Restart WSL2 after editing:

```powershell
wsl --shutdown
```

---

## Step 3: Install JetBrains Gateway

If you do not already have JetBrains Gateway:

1. Download from **JetBrains Toolbox** (recommended) or `https://www.jetbrains.com/remote-development/gateway/`
2. Install and launch Gateway
3. Sign in with your JetBrains license

Gateway is a lightweight launcher -- it does not include a full IDE. The IDE backend installs inside the sandbox on first connection.

---

## Step 4: Start Your Sandbox

```powershell
# Java project
aibox start --workspace ~/projects/my-service --toolpacks java@21

# Node.js project
aibox start --workspace ~/projects/my-app --toolpacks node@20

# Scala project
aibox start --workspace ~/projects/my-scala --toolpacks scala@3,java@21

# .NET project (use Rider)
aibox start --workspace ~/projects/my-api --toolpacks dotnet@8
```

Wait for the "Sandbox ready" message:

```
Sandbox ready.
  SSH:       localhost:2222
  Workspace: /workspace
  Tools:     java@21
  Status:    aibox status
```

---

## Step 5: Connect JetBrains Gateway

### Option A: SSH Connection (Recommended)

1. Open **JetBrains Gateway**
2. Click **"SSH"** under "Remote Development"
3. Click **"New Connection"**
4. Configure:
   - **Host**: `localhost`
   - **Port**: `2222`
   - **User**: `aibox`
   - **Authentication**: Key-based (AI-Box configures keys during setup)
5. Click **"Check Connection and Continue"**
6. Select the **IDE** to install in the container:
   - IntelliJ IDEA (Java/Scala/Kotlin)
   - WebStorm (JavaScript/TypeScript)
   - PyCharm (Python)
   - Rider (.NET/C#)
7. Set **Project Directory**: `/workspace`
8. Click **"Download IDE and Connect"**

### Option B: Re-use Saved Connection

After the first connection, Gateway saves your configuration. On subsequent connections:

1. Open JetBrains Gateway
2. Click your saved `aibox` connection under "Recent"
3. Gateway reconnects automatically

### First Connection

On first connection, Gateway downloads and installs the JetBrains backend inside the container. This takes **2-5 minutes** depending on the IDE.

After installation:
- The IDE frontend opens on your local machine
- The backend (indexing, code analysis, compilation) runs inside the sandbox
- All files are in `/workspace`

---

## Step 6: Initial Indexing

JetBrains IDEs index your project on first open. This is normal and takes 1-5 minutes depending on project size.

**Tips to speed up indexing**:
- Indexing uses persistent storage -- subsequent opens are faster
- Mark directories as "Excluded" if they contain generated files
- The index persists between sandbox restarts (stored in named volume)

**If indexing seems stuck**:
1. Check memory: the JetBrains backend needs 2-4GB free RAM
2. Open the IDE's "Power Save Mode" indicator (bottom-right) -- ensure it is off
3. Check `aibox status` for container resource usage

---

## Step 7: Use AI Tools

Open the **Terminal** in your JetBrains IDE (Alt+F12) and start an AI tool:

```bash
# Claude Code (API key auto-injected)
claude

# Codex CLI (API key auto-injected)
codex
```

AI tools run in the terminal inside the sandbox. API keys are injected automatically.

You can also use AI tools alongside the JetBrains AI Assistant plugin if it is installed in your IDE backend.

---

## Step 8: Build, Test, Debug

### Building

Use the IDE's built-in build tools or the terminal:

```bash
# Java (IntelliJ)
gradle build
mvn package

# Scala (IntelliJ)
sbt compile

# Node.js (WebStorm)
npm install && npm run build

# .NET (Rider)
dotnet build
```

### Debugging

Debugging works natively because the debug adapter runs inside the container:

1. Set breakpoints in the editor
2. Click **Run > Debug** (or Shift+F9)
3. The debugger attaches inside the container
4. Variables, call stack, watches -- all work as expected

**Port forwarding for web apps**:

If your application serves HTTP on a port (e.g., 8080):

```bash
# From host terminal
aibox port-forward 8080
```

Then open `http://localhost:8080` in your local browser.

### Hot Reload

Hot reload works with native `inotify` inside the container. No cross-OS file sync latency.

---

## Step 9: Push Code

```bash
git add .
git commit -m "feat: add feature"
git push
```

If push gating is enabled by policy, check status with:

```bash
aibox push status
```

---

## Step 10: Stop When Done

```powershell
# From host terminal
aibox stop
```

JetBrains index, build caches, and workspace state persist in named volumes.

---

## Day-to-Day Workflow

```
1. aibox start --workspace ~/projects/my-service    (15 seconds)
2. Open JetBrains Gateway, click saved connection    (seconds)
3. Code, build, debug, use AI tools                  (normal workflow)
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

The CLI warns at 80% resource usage when running multiple sandboxes. Each sandbox appears as a separate connection in JetBrains Gateway.

---

## Telemetry

AI-Box collects operational metrics only (startup times, success/fail, image versions). No command history, file paths, or AI prompts are ever collected.

```bash
aibox telemetry show       # See what is collected
aibox telemetry opt-out    # Opt out of optional telemetry
```

---

## Troubleshooting

### "Connection refused" in Gateway

1. Verify sandbox is running: `aibox status`
2. Check the SSH port matches (default: 2222)
3. Restart the sandbox: `aibox stop && aibox start --workspace <path>`
4. Check for port conflicts: `netstat -an | findstr 2222`

### Gateway cannot download IDE backend

1. The IDE backend download goes through the egress proxy
2. Run `aibox network test` to check proxy connectivity
3. If JetBrains CDN is not in the allowlist, contact `#aibox-help`
4. Workaround: pre-installed IDE backends may be available in future image versions

### Indexing never completes or is very slow

1. Check available memory: JetBrains backend needs 2-4GB
2. Increase WSL2 memory in `.wslconfig` (see Step 2)
3. Exclude generated directories (e.g., `build/`, `node_modules/`, `target/`)
4. Check `aibox status` for memory pressure

### IDE feels sluggish

1. JetBrains frontend is a thin client -- it should be responsive locally
2. If the backend is slow, check memory allocation
3. Reduce indexing scope by excluding large directories
4. Close unused editor tabs (each tab consumes backend memory)

### "Cannot resolve dependencies"

1. Run `aibox network test` to verify Nexus connectivity
2. Ensure the correct tool pack is installed for your build system
3. Check `.gradle/gradle.properties` or `pom.xml` for hardcoded repository URLs (should use Nexus mirrors)
4. For NuGet (.NET): ensure `nuget.config` points to `nexus.internal`

### Debugging does not attach

1. JetBrains debug adapter runs inside the container -- ensure the process started correctly
2. For Java: check that JDWP port is not conflicting
3. For Node.js: use `--inspect` flag and verify port forwarding
4. Restart the IDE backend: close Gateway connection and reconnect

### "Blocked by policy" message

1. Note the log entry ID
2. Run `aibox policy explain --log-entry <id>` in the terminal
3. Contact your champion or `#aibox-help` if the block seems incorrect

### Nothing works

```bash
aibox doctor
```

Share the output with your team champion or `#aibox-help`.

---

## Quick Reference

```
aibox setup                              # One-time setup
aibox start --workspace <path>           # Start sandbox
aibox start --toolpacks java@21          # Start with tool packs
aibox stop                               # Stop sandbox
aibox status                             # Check status
aibox doctor                             # Diagnose issues
aibox update                             # Pull latest image
aibox install <pack>@<version>           # Add tool pack
aibox shell                              # Open shell
aibox network test                       # Test connectivity
aibox port-forward <port>                # Forward port to host
aibox policy explain --log-entry <id>    # Explain block
aibox repair cache                       # Fix cache issues
```

---

*Having trouble? Run `aibox doctor` first, then check the [Troubleshooting FAQ](troubleshooting-faq.md). Your team's AI-Box champion can also help.*
