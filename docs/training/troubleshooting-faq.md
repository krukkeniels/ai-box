# AI-Box Troubleshooting FAQ

**Deliverable**: D11
**Audience**: All developers
**Last Updated**: 2026-02-21

---

## How to Use This FAQ

1. Find your symptom in the table of contents below
2. Follow the resolution steps
3. If the issue persists, run `aibox doctor` and share the output with your team champion or `#aibox-help`

**Before you read further**: run `aibox doctor`. It automatically detects and explains the most common problems.

---

## Table of Contents

- [Setup and Installation](#setup-and-installation)
- [Startup Failures](#startup-failures)
- [IDE Connection: VS Code](#ide-connection-vs-code)
- [IDE Connection: JetBrains Gateway](#ide-connection-jetbrains-gateway)
- [Network and Proxy](#network-and-proxy)
- [Build Failures](#build-failures)
- [AI Tool Issues](#ai-tool-issues)
- [Policy Violations](#policy-violations)
- [Performance](#performance)
- [Git and Push](#git-and-push)
- [Tool Packs and MCP](#tool-packs-and-mcp)
- [General](#general)

---

## Setup and Installation

### Q: `aibox setup` fails with "WSL2 not found"

**A**: WSL2 must be installed and enabled on Windows 11.

1. Open PowerShell as Administrator
2. Run: `wsl --install`
3. Restart your computer
4. Run: `wsl --update`
5. Retry: `aibox setup`

If WSL2 is installed but `aibox` cannot find it, ensure you are running `aibox setup` from a PowerShell or Windows Terminal window (not from inside WSL).

### Q: `aibox setup` fails with "Podman installation failed"

**A**: Podman installs inside WSL2. If installation fails:

1. Open WSL2: `wsl`
2. Check the distro: `cat /etc/os-release` (should be Ubuntu 24.04 or Fedora)
3. Try manual install: `sudo apt update && sudo apt install podman`
4. Run `aibox setup` again

If Podman is already installed but an old version, update it:

```bash
sudo apt update && sudo apt upgrade podman
```

### Q: `aibox setup` fails with "insufficient resources"

**A**: AI-Box requires:
- 16GB RAM minimum (32GB recommended for polyglot)
- SSD storage
- 8+ CPU cores

Check your WSL2 allocation in `%USERPROFILE%\.wslconfig`. See the quickstart guides for recommended settings.

### Q: How do I completely reset AI-Box?

**A**:

```bash
aibox stop                    # Stop running sandbox
aibox repair cache            # Clear caches
```

For a full reset (re-downloads everything):

```bash
aibox stop
# In WSL2:
podman system reset           # Removes all images and containers
# Then re-run:
aibox setup
```

---

## Startup Failures

### Q: `aibox start` hangs with no output

**A**: The image is likely downloading (first run or after update). Wait up to 90 seconds. Check progress:

```bash
aibox status
```

If it hangs for more than 2 minutes, cancel (Ctrl+C) and check:
1. Network connectivity: `aibox network test`
2. Harbor availability: `aibox doctor`
3. Disk space: `df -h` in WSL2

### Q: `aibox start` fails with "image signature invalid"

**A**: The image signature could not be verified. This is a security check.

1. Run `aibox update` to pull the latest signed image
2. If the error persists, the Harbor signing key may have rotated
3. Run `aibox setup` to refresh signing keys
4. If still failing, report to `#aibox-help` (possible supply chain issue)

### Q: `aibox start` fails with "image too old, mandatory update required"

**A**: A critical security update requires a newer image.

```bash
aibox update
aibox start --workspace <path>
```

This is enforced when a critical CVE has been patched. You cannot bypass this check.

### Q: `aibox start` fails with "port 2222 already in use"

**A**: Another process is using the SSH port.

1. Find the process: `ss -tlnp | grep 2222` (in WSL2) or `netstat -an | findstr 2222` (PowerShell)
2. Stop the conflicting process, or
3. Use a different port: `aibox start --ssh-port 2223 --workspace <path>`
4. Update your IDE SSH config to use the new port

### Q: `aibox start` fails with "gVisor runtime not available"

**A**: The gVisor (runsc) runtime is not installed or not configured.

1. Run `aibox doctor` to check gVisor status
2. Run `aibox setup` to reinstall
3. Check that your CPU supports virtualization (required for gVisor under WSL2)

---

## IDE Connection: VS Code

### Q: VS Code cannot find the "aibox" host in Remote SSH

**A**: AI-Box should configure your SSH config automatically. If it is missing:

1. Check `~/.ssh/config` for an `aibox` host entry
2. Run `aibox start` again (it updates SSH config on start)
3. Manual fallback: connect to `aibox@localhost -p 2222`

### Q: VS Code shows "Could not establish connection"

**A**:

1. Verify sandbox is running: `aibox status`
2. Test SSH manually: `ssh aibox@localhost -p 2222` (from PowerShell or WSL2)
3. If SSH works but VS Code fails, try: Command Palette > "Remote-SSH: Kill VS Code Server on Host" > reconnect
4. If SSH itself fails, restart the sandbox: `aibox stop && aibox start --workspace <path>`

### Q: VS Code keeps disconnecting and reconnecting

**A**: This usually indicates VS Code Server crashing inside the container.

1. Check container memory: `aibox status` (look for memory pressure)
2. Increase WSL2 memory in `.wslconfig`
3. Clear VS Code Server: `aibox repair cache`
4. Restart: `aibox stop && aibox start --workspace <path>`
5. Reconnect VS Code

### Q: VS Code extensions are not available

**A**: Extensions run inside the container. The extension marketplace may be restricted by proxy policy.

- **Pre-approved extensions** are baked into the image and available immediately
- **Other extensions**: request via your champion or `#aibox-help`
- **Extension telemetry** is disabled by default. Extensions requiring telemetry endpoints may not function

---

## IDE Connection: JetBrains Gateway

### Q: Gateway says "Connection refused"

**A**:

1. Verify sandbox is running: `aibox status`
2. Check SSH port in `aibox status` output
3. Ensure Gateway SSH settings match (host: `localhost`, port: `2222`, user: `aibox`)
4. Test SSH manually: `ssh aibox@localhost -p 2222`

### Q: Gateway is stuck downloading the IDE backend

**A**: The IDE backend downloads through the egress proxy.

1. Check proxy connectivity: `aibox network test`
2. If JetBrains CDN is blocked, contact `#aibox-help` to request allowlist addition
3. Large backends (IntelliJ, Rider) may take 3-5 minutes on first download

### Q: Indexing is extremely slow or never completes

**A**:

1. JetBrains backend needs 2-4GB RAM. Check WSL2 memory allocation
2. Exclude generated directories: `build/`, `target/`, `node_modules/`, `.gradle/`
3. Close the project and re-open to restart indexing
4. Check `aibox status` for overall container memory usage
5. For large monorepos: consider indexing only the relevant module

### Q: Gateway shows "Backend terminated unexpectedly"

**A**: The IDE backend ran out of memory.

1. Increase WSL2 memory (minimum 12GB, recommended 16GB+)
2. Reduce open projects/tabs
3. Reconnect -- Gateway will restart the backend

---

## Network and Proxy

### Q: `npm install` / `pip install` / `gradle dependencies` fail

**A**: Package downloads go through internal Nexus mirrors.

1. Run `aibox network test` to check Nexus connectivity
2. Verify the correct tool pack is installed for your package manager
3. Check for cached dependencies: retry the build (cache may satisfy)
4. If Nexus is down, builds using cached dependencies still work

**For npm**: Ensure `.npmrc` uses the Nexus registry (configured automatically by the tool pack).

**For pip**: Ensure `pip.conf` uses the Nexus mirror.

**For Gradle/Maven**: Ensure `build.gradle` or `pom.xml` uses the Nexus mirror. Hardcoded repository URLs to external registries will fail.

**For NuGet (.NET)**: Ensure `nuget.config` points to `nexus.internal`.

### Q: `git clone` fails for an external repository

**A**: External Git hosts must be in the egress allowlist.

1. Check which hosts are allowed: `aibox policy validate`
2. Your Git server (`git.internal`) is always allowed
3. External hosts (e.g., github.com) may not be allowed by policy
4. Request allowlist additions through your team lead or `#aibox-help`

### Q: DNS resolution fails inside the sandbox

**A**: AI-Box uses CoreDNS with allowlist-only resolution.

1. Run `aibox doctor` to check DNS status
2. Only allowlisted domains resolve. Unlisted domains return NXDOMAIN
3. This is by design -- not a bug
4. If a required domain is not resolving, request allowlist addition

### Q: `curl` does not work inside the sandbox

**A**: `curl` to arbitrary endpoints is disabled by default network policy. Use package managers through Nexus mirrors instead. `curl` works only to allowlisted hosts.

---

## Build Failures

### Q: First build is much slower than local

**A**: The first build in a fresh sandbox downloads all dependencies through Nexus. Subsequent builds use persistent caches and should be comparable to local builds.

- Build caches persist in named volumes between sandbox restarts
- If caches are corrupted: `aibox repair cache`

### Q: Java/Gradle build fails with OutOfMemoryError

**A**:

1. Check WSL2 memory allocation: need 8GB+ for JVM builds
2. Set Gradle JVM args in `gradle.properties`:
   ```
   org.gradle.jvmargs=-Xmx4g
   ```
3. Check `aibox status` for container memory usage
4. Close other memory-heavy processes (IDE backend, AI tools) temporarily

### Q: Bazel build fails with sandbox errors

**A**: Bazel's own sandboxing can conflict with gVisor.

Use the `--spawn_strategy=local` flag:

```bash
bazel build --spawn_strategy=local //...
```

Or add to `.bazelrc`:

```
build --spawn_strategy=local
```

This is a known limitation of running Bazel inside a gVisor sandbox.

### Q: .NET restore fails

**A**:

1. Ensure the `dotnet@8` tool pack is installed: `aibox install dotnet@8`
2. Verify NuGet source points to Nexus: `dotnet nuget list source`
3. Run `aibox network test` to check Nexus connectivity
4. Clear NuGet cache: `dotnet nuget locals all --clear`

### Q: Hot reload is not picking up file changes

**A**: File watching uses `inotify` inside the container. If it is not working:

1. Check the inotify watch limit: `cat /proc/sys/fs/inotify/max_user_watches`
2. If low, it may need increasing (contact `#aibox-help`)
3. Ensure you are editing files inside `/workspace`, not on the host

---

## AI Tool Issues

### Q: `claude` or `codex` fails to start

**A**:

1. Verify the `ai-tools` pack is installed: `aibox install ai-tools`
2. Check that the LLM API endpoint is reachable: `aibox network test`
3. API keys are injected automatically. If injection fails, restart the sandbox

### Q: AI tool says "API key not found" or authentication fails

**A**: Credentials are injected by the credential broker.

1. Restart the sandbox: `aibox stop && aibox start --workspace <path>`
2. If the issue persists, the credential broker (Vault) may be down
3. Run `aibox doctor` -- it checks Vault connectivity
4. Cached credentials are valid for 4-8 hours. If recently started, wait a moment for injection

### Q: AI tool is slow or times out

**A**:

1. Check LLM API connectivity: `aibox network test`
2. The LLM API proxy adds minimal latency (<50ms)
3. If the LLM service itself is slow, this is outside AI-Box's control
4. Check if rate limits are being hit (visible in `aibox status`)

---

## Policy Violations

### Q: I got a "blocked by policy" error. What do I do?

**A**:

1. Note the log entry ID in the error message
2. Run: `aibox policy explain --log-entry <id>`
3. This tells you which policy rule blocked the action and why
4. Common reasons:
   - Network access to a non-allowlisted host
   - File access outside `/workspace`
   - Running a tool not in the project allowlist
   - `npm publish` or similar sensitive operations blocked by default

### Q: How do I request a policy change?

**A**:

- **Team policy changes** (tighten only): ask your team lead to submit a PR to the team policy file
- **Project policy changes**: submit a PR to your project's `/aibox/policy.yaml`
- **Org baseline changes**: these require security team approval. Route through `#aibox-help`

Policy can only be tightened by teams/projects, never loosened below the org baseline.

### Q: `aibox policy validate` shows errors

**A**:

1. Check your project's `/aibox/policy.yaml` for syntax errors
2. Ensure the policy does not try to loosen org baseline restrictions
3. Common mistakes: allowlisting a host that the org baseline blocks, granting permissions the baseline denies

---

## Performance

### Q: Everything is slow

**A**: Systematic diagnosis:

1. Run `aibox doctor` for automated checks
2. Check WSL2 memory: `wsl -- free -h` from PowerShell
3. Check container resources: `aibox status`
4. Check disk space: `df -h` inside the sandbox
5. Common cause: WSL2 default memory is too low. Edit `.wslconfig`

### Q: WSL2 is consuming too much memory

**A**: WSL2 can grow memory but not shrink it. To reclaim:

```powershell
wsl --shutdown
# Wait 10 seconds
# Start aibox again
```

Also consider setting a memory limit in `.wslconfig` to prevent WSL2 from consuming all available RAM.

---

## Git and Push

### Q: `git push` is pending approval

**A**: Your project policy has push gating enabled.

1. Check status: `aibox push status`
2. An approver has been notified
3. You can continue working while waiting
4. To cancel: `aibox push cancel`

### Q: `git push` fails with "permission denied"

**A**:

1. Git credentials are managed by the credential broker
2. Restart the sandbox to refresh credentials
3. Ensure you are pushing to an allowed remote (usually `git.internal`)
4. Pushing to external remotes may be blocked by policy

---

## Tool Packs and MCP

### Q: How do I see what tool packs are available?

**A**:

```bash
aibox install --list
```

### Q: How do I install a tool pack into a running sandbox?

**A**:

```bash
aibox install python@3.12
```

The tool pack installs without restarting the sandbox.

### Q: A tool I need is not available as a pack

**A**:

1. Ask your champion to submit a tool pack request
2. Or file directly in `#aibox-help` with: tool name, version, why you need it, how many devs would use it
3. SLA: known tools in 1 business day, new tools requiring security review in 3-5 business days

### Q: How do I enable MCP servers?

**A**:

```bash
aibox mcp enable filesystem-mcp git-mcp
aibox mcp list
```

MCP servers are auto-discovered by AI agents inside the sandbox.

---

## General

### Q: Where is my data stored?

**A**:

- **Workspace files**: `/workspace` inside the container (bind mount or clone of your project)
- **Build caches**: Named Podman volumes (persist between restarts)
- **Home directory**: Named volume (shell history, dotfiles, IDE settings persist)
- **Container filesystem**: Read-only base image (ephemeral, rebuilt weekly)

### Q: Can I customize my shell?

**A**: Yes. `bash`, `zsh`, and `pwsh` are available. Your home directory persists in a named volume. You can also configure dotfiles sync:

```bash
# Set your dotfiles repo
aibox config set dotfiles.repo git@git.internal:you/dotfiles.git
```

### Q: Can I run multiple sandboxes at once?

**A**: Each `aibox start` with a different workspace creates a separate sandbox. Ensure your machine has sufficient resources (each sandbox needs 4-8GB RAM).

### Q: How do I file a bug report?

**A**: Use the bug report template in the Champions Handbook, or post in `#aibox-help` with:

- What you were doing
- What happened (error message, screenshot)
- Output of `aibox doctor`
- Output of `aibox status`

---

*FAQ not covering your issue? Run `aibox doctor`, then contact your team champion or post in `#aibox-help`.*
