# Phase 4: Developer Experience

**Phase**: 4 of 6
**Estimated Effort**: 8-10 engineer-weeks
**Team Size**: 3 engineers
**Calendar**: Weeks 11-14
**Dependencies**: Phase 1 (CLI/Runtime), Phase 2 (Network Security), Phase 3 (Policy Engine & Credentials)

---

## Overview

Phase 4 transforms AI-Box from a secure but bare container into a development environment that feels better than local development. This is the adoption gate -- developers will reject AI-Box regardless of its security properties if the daily workflow is painful.

This phase delivers five work streams: IDE integration (VS Code, JetBrains, debugging, port forwarding), the tool pack system with 10 initial packs, the MCP pack system for AI agent tool discovery, dotfiles and shell customization, and the non-blocking `git push` approval flow. Every work stream targets the spec's "invisible security" principle (Section 4): if developers notice friction, the phase has failed.

---

## Deliverables

1. **IDE integration** -- VS Code Remote SSH (pre-installed server, SSH config, extensions, telemetry blocking), JetBrains Gateway (SSH backend, resource guidance), debugging (JDWP, Node inspector, debugpy, port forwarding), hot reload via native inotify
2. **Tool pack system** -- manifest schema, `aibox install <pack>` command, runtime install into `/opt/toolpacks`, signature verification, registry
3. **Initial tool packs** -- `java@21`, `node@20`, `python@3.12`, `bazel@7`, `scala@3`, `angular@18`, `dotnet@8`, `powershell@7`, `angularjs@1`, `ai-tools`
4. **Pre-built image variants** -- `aibox-java:21-24.04`, `aibox-node:20-24.04`, `aibox-dotnet:8-24.04`, `aibox-full:24.04`
5. **MCP pack system** -- `aibox mcp enable/list/disable` commands, auto-generated MCP config for AI agent discovery
6. **Initial MCP packs** -- `filesystem-mcp`, `git-mcp`
7. **Dotfiles and shell customization** -- dotfiles repo cloning into persistent home volume, bash + zsh + tmux, persistent shell history
8. **Git push non-blocking approval flow** -- staging ref mechanism, webhook notifications, async approve/reject, merge-on-approval
9. **Documentation** -- IDE quickstarts, tool pack authoring guide, MCP pack guide, custom CLI migration guide

---

## Implementation Steps

### Work Stream 1: IDE Integration

**What to build**:

**VS Code Remote SSH**:

- Pre-install VS Code Server in the base image during the image build pipeline. This eliminates the 30-60 second first-connect download. Pre-install latest stable in weekly image rebuilds; VS Code handles minor version mismatches gracefully.
- Configure SSH server in container on port 22, mapped to host 2222 via Podman port forwarding.
- Add `aibox` CLI hook that writes/updates the user's `~/.ssh/config` with an entry for the container:
  ```
  Host aibox
    HostName localhost
    Port 2222
    User dev
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
  ```
- Pre-install curated VS Code extensions into the server image (language support, Git, debugging). Maintain list in `aibox-images` repo.
- Disable all telemetry: `"telemetry.telemetryLevel": "off"` in pre-installed settings. Known telemetry endpoints never added to Squid allowlist.
- Allow VS Code Marketplace access via proxy for extension installation. Lock down to baked-in only if security review requires it.
- Generate a dedicated SSH key pair during `aibox setup`; inject public key into container. Do not reuse developer's personal SSH keys.

**JetBrains Gateway**:

- Ensure container SSH server supports JetBrains Gateway (standard SSH + SFTP subsystem -- no special requirements).
- JetBrains Gateway downloads its backend (~500MB-1GB) on first connect. Route through Squid proxy; add `download.jetbrains.com` (or internal mirror) to allowlist.
- Store JetBrains backend in persistent home volume (`/home/dev/.cache/JetBrains`) to survive container restarts.
- Set minimum resource requirements: 4 CPU cores, 8GB RAM (spec Section 13.2). The `aibox` CLI warns or auto-adjusts when JetBrains is configured (e.g., `aibox start --profile jetbrains`).
- Coordinate with org's JetBrains license admin for license server endpoint (add to egress allowlist).

**Debugging & Port Forwarding**:

- Ensure standard debugging protocols work without extra configuration:
  - **Node.js**: `--inspect` flag, VS Code connects via SSH tunnel automatically.
  - **Java (JDWP)**: JVM debug port, JetBrains Gateway backend connects directly.
  - **Python**: `debugpy` inside container, VS Code Python extension via SSH tunnel.
  - **Browser DevTools**: Port-forward dev server (e.g., 3000, 4200) to host.
- Implement `aibox port-forward <container-port> [<host-port>]` for manual port forwarding.
- Set `fs.inotify.max_user_watches=524288` inside container for hot reload with large projects.
- Document that gVisor blocks `ptrace`, so `strace`/GDB will not work. Recommend IDE-integrated debug adapters (which use debug protocols, not ptrace).

**Spec references**: Sections 13.1-13.4, 18.1, 18.2.

---

### Work Stream 2: Tool Pack System & Initial Packs

**What to build**:

**Tool Pack System**:

- Implement tool pack manifest schema (spec Section 15.2) as YAML: `name`, `version`, `description`, `maintainer`, `install`, `network`, `filesystem`, `resources`, `security`, `tags`.
- Implement `aibox install <pack>@<version>`:
  1. Fetch manifest from tool pack registry (Harbor OCI artifacts).
  2. Verify manifest signature (Cosign).
  3. Check network requirements against current policy.
  4. Install into `/opt/toolpacks/<pack>` via docker-layer method (primary), with script method as fallback for org-approved scripts only.
  5. Update `$PATH` and environment variables.
  6. Register pack so `aibox status` shows installed packs.
- Implement `aibox install --toolpacks java@21,node@20` flag on `aibox start`.
- Build tool pack registry as Harbor OCI artifacts (binary packs, signature verification built in) with Git repo for manifests and build scripts.
- Implement `aibox list packs` to show available packs.
- Version pinning: `@21` shorthand resolves to latest patch. Manifests immutable once published.
- Ship both image variants (top stacks) and runtime install (everything else).

**Initial Tool Packs (10 packs)**:

| Pack | Contents | Target Size | Notes |
|------|----------|-------------|-------|
| `java@21` | OpenJDK 21 (Temurin), Gradle 8.5, Maven 3.9.6 | ~400MB | `eclipse-temurin:21-jdk` base layer |
| `node@20` | Node.js 20 LTS, npm 10.x, yarn 1.22.x | ~150MB | Official Node.js binaries |
| `python@3.12` | Python 3.12, pip, venv, setuptools | ~100MB | Ubuntu `python3.12` packages |
| `bazel@7` | Bazel 7.x | ~200MB | Bazelisk binary |
| `scala@3` | Scala 3.x, sbt 1.x, Metals LSP | ~300MB | Coursier-installed tools |
| `angular@18` | Angular CLI 18.x (requires `node@20`) | ~50MB | npm global install |
| `dotnet@8` | .NET SDK 8, NuGet CLI, MSBuild | ~500MB | `mcr.microsoft.com/dotnet/sdk:8.0` base layer |
| `powershell@7` | PowerShell Core 7.x | ~150MB | `pwsh` also in base image; pack adds modules/tooling |
| `angularjs@1` | AngularJS 1.x build tools (Node 18, Bower, Grunt/Gulp, Karma/Protractor) | ~200MB | Legacy; may need Node 18 not Node 20 |
| `ai-tools` | Claude Code CLI, Codex CLI | ~100MB | Auto-configured to use LLM sidecar proxy |

For each pack: write manifest YAML, create Dockerfile, build and push to Harbor as signed OCI artifact, test install + basic build/run, configure named volumes for build caches.

**`dotnet@8` Details**:

```yaml
name: dotnet
version: "8.0"
description: ".NET SDK 8, NuGet CLI, MSBuild"
maintainer: "platform-team"

install:
  method: "docker-layer"
  base_image: "mcr.microsoft.com/dotnet/sdk:8.0"

network:
  requires:
    - id: "nuget-feed"
      hosts: ["nexus.internal"]
      ports: [443]

filesystem:
  creates: ["/usr/share/dotnet"]
  caches:
    - "$HOME/.nuget/packages"
    - "$HOME/.dotnet/tools"

resources:
  min_memory: "4GB"
  recommended_memory: "6GB"

tags: ["language", "clr", "dotnet"]
```

Validate that `dotnet restore` works through Nexus proxy. Include pre-configured `nuget.config` pointing to Nexus mirror.

**`powershell@7` Details**:

- PowerShell Core 7.x (`pwsh`) is also being added to the base image as a shell. The tool pack provides additional PowerShell modules and tooling beyond what the base image ships.
- Include compatibility matrix documentation for Windows-origin scripts:

| PowerShell Pattern | Linux Support | Migration Path |
|-------------------|---------------|---------------|
| `Get-ChildItem`, `Invoke-WebRequest` | Yes | No change |
| `Get-WmiObject` | No | Use `Get-CimInstance` |
| `[System.Windows.Forms]`, COM objects | No | Rewrite with CLI alternatives or .NET APIs |
| Registry access (`HKLM:\...`) | No | Use environment variables or config files |

**`angularjs@1` Details**:

- Legacy AngularJS 1.x build tooling for projects that have not migrated to Angular 2+.
- Key risk: may need Node 16 or 18, not Node 20. The tool pack system must support multiple Node versions side-by-side (via nvm or separate packs).
- Includes: Bower (if used), Grunt/Gulp task runners, Karma/Protractor test runners.

**Dependency handling**: Tool pack system checks for and auto-installs dependencies (e.g., `angular@18` requires `node@20`), or reports a clear error if missing.

**Spec references**: Sections 5.4, 15 (all subsections), 17.2.

---

### Work Stream 3: MCP Pack System

**What to build**:

- Implement MCP pack manifest schema: `name`, `version`, `description`, `command`, `args`, `network_requires`, `filesystem_requires`, `permissions`.
- Implement `aibox mcp enable <pack> [<pack>...]`:
  1. Validate network and filesystem requirements against current policy (via OPA).
  2. Install MCP server binary if not present.
  3. Generate MCP configuration file for AI agent discovery. Write canonical config at `~/.config/aibox/mcp.json` and symlink/copy to agent-specific locations (e.g., `~/.config/claude/claude_desktop_config.json`).
  4. MCP servers launch on-demand by the AI agent (standard MCP pattern -- config declares how to start them).
- Implement `aibox mcp list` and `aibox mcp disable <pack>`.
- Auto-regenerate MCP configuration on `aibox start` based on enabled packs.
- MCP packs requiring network access (e.g., `jira-mcp`) must have endpoints in policy allowlist. `aibox mcp enable jira-mcp` fails with a clear message if policy doesn't allow it.

**Spec references**: Sections 5.5, 14.4, 16 (all subsections).

---

### Work Stream 4: Dotfiles & Shell

**What to build**:

- Implement dotfiles sync: developer configures repo URL (`aibox config set dotfiles.repo <url>`), CLI clones/pulls on `aibox start` into persistent home volume. If repo contains `install.sh` or `Makefile`, run it; otherwise symlink standard dotfiles (`.bashrc`, `.zshrc`, `.vimrc`, `.gitconfig`, `.tmux.conf`).
- Ensure `bash`, `zsh`, and `pwsh` available in base image. Default shell configurable via `aibox config set shell zsh`.
- Pre-install `tmux` with sensible defaults.
- Ensure shell history persists in home volume across container recreation.
- Set standard environment variables (`$EDITOR`, `$TERM`, `$LANG`); allow user override via dotfiles.
- Source `aibox-env.sh` last in shell rc files to ensure proxy vars and PATH entries are correct, overriding any conflicts from user dotfiles.
- Dotfiles repo auth uses same `aibox-credential-helper` (Vault-backed, Phase 3).

**Spec references**: Sections 13.5, 18.4.

---

### Work Stream 5: Git Push Approval Flow

**What to build**:

- Implement non-blocking `git push` approval for repos where policy sets `git push` to `review-required`.
- Flow:
  1. Developer runs `git push origin main`.
  2. Custom Git remote helper (`aibox-git-remote-helper`) intercepts and redirects to staging ref: `refs/aibox/staging/<user>/<timestamp>`.
  3. Approval request created (source branch, target branch, commit range, developer identity).
  4. Webhook fires to notify approver(s) via Slack webhook (primary) or email (fallback).
  5. Developer sees: "Push staged for approval. Continue working. Check status: `aibox push status`."
  6. Developer is NOT blocked.
  7. On approval: staging ref merged/fast-forwarded to target branch, developer notified.
  8. On rejection: staging ref deleted, developer notified with reason.
- Implement `aibox push status` and `aibox push cancel`.
- For repos where `git push` is `safe`, push proceeds normally.
- Staged pushes not approved within configurable timeout (default: 48h) are auto-rejected.
- Protect `refs/aibox/staging/` namespace on Git server so only the approval system can promote refs.

**Spec references**: Sections 12.4, 18.5.

---

## Additional Deliverables

### Custom CLI Migration Guide

Documentation deliverable: **"Migrating Project-Specific CLIs to AI-Box"**.

1. **Inventory**: Classify each command as cross-platform (no change), PowerShell-based (ensure `pwsh` compatibility), or Windows-native (requires rewrite or wrapper).
2. **Wrapper pattern**: For Windows-native commands, provide a translation script:
   ```bash
   case "$1" in
     build-dotnet) dotnet build "${@:2}" ;;
     build-angular) ng build "${@:2}" ;;
     *) echo "Unknown command: $1" >&2; exit 1 ;;
   esac
   ```
3. **Gradual migration**: Critical paths (build, test, serve) first; convenience commands follow.

### Polyglot Resource Profile

For polyglot monolith workloads (.NET + JVM + Node + Bazel + AI agent), recommend:

| Component | Memory Budget |
|-----------|--------------|
| .NET SDK | ~4GB |
| JVM (Gradle/sbt) | ~4GB |
| Node.js | ~2GB |
| Bazel | ~4GB |
| AI agent | ~2GB |
| OS overhead | ~2GB |
| **Total** | **~18GB** |

Recommend 24GB WSL2 allocation for polyglot developers:

```ini
[wsl2]
memory=24GB
processors=8
swap=8GB
```

---

## Research Required

### 1. MCP Server Discovery Across AI Agents

**Question**: How do different AI agents (Claude Code, Codex CLI, Aider, Continue, custom agents) discover and connect to MCP servers?

**Research tasks**:
- Document MCP config location and format for each supported agent.
- Determine if a standard MCP discovery mechanism exists (well-known path, environment variable) or if each agent has its own format.
- Prototype auto-generating MCP config for Claude Code and one other agent.
- Research MCP server stdio vs. HTTP transport per agent.

**Why this matters**: AI-Box is "assistant/tool/MCP agnostic" (spec Section 3). MCP discovery must work for multiple agents.

### 2. Non-Blocking Git Push Approval Implementation

**Question**: What is the most reliable mechanism to intercept `git push`, redirect to a staging ref, and implement async approval?

**Research tasks**:
- Prototype custom Git remote helper that rewrites push targets. Test with `git push`, `git push --force`, multi-ref pushes, and tag pushes.
- Research how GitHub/GitLab protected branches interact with staging refs.
- Prototype end-to-end flow: push to staging ref -> notification -> approval -> branch promotion.

**Why this matters**: This is a novel workflow. If buggy, developers lose trust in the system.

### 3. gVisor Compatibility for Polyglot Stacks

**Question**: Do .NET CLR, Bazel sandboxing (sandbox-within-sandbox), and PowerShell Core work correctly under gVisor?

**Research tasks**:
- Test .NET CLR memory-mapped files, JIT, and thread pool patterns under gVisor.
- Test Bazel sandbox-within-gVisor-sandbox; determine if `--spawn_strategy=local` fallback is needed.
- Test PowerShell Core syscall surface beyond bash/zsh.
- Test combined stack (JVM + .NET + Node) under gVisor memory pressure.

**Why this matters**: Polyglot stacks expand gVisor compatibility surface beyond what has been validated.

---

## Open Questions

1. **VS Code Marketplace access**: Allow marketplace via proxy for extension installation, or bake all extensions into the image? Trade-off: flexibility vs. attack surface. Needs security team decision.

2. **Tool pack self-service**: Can team leads publish tool packs without platform team review, or does every new pack require security sign-off? The spec says "known/registered tools: 1 business day" -- clarify the boundary.

3. **Multiple AI agents simultaneously**: Can a developer run Claude Code and Codex CLI simultaneously? If so, do they share MCP servers or get separate instances? The spec is silent on multi-agent scenarios.

---

## Dependencies

### Upstream (what Phase 4 needs)

| Dependency | Source | Requirement |
|-----------|--------|-------------|
| Running container with SSH server | Phase 1 | OpenSSH on port 22, mapped to host 2222 |
| Base image build pipeline | Phase 0 | Able to add VS Code Server and tool packs |
| Squid proxy with allowlist | Phase 2 | Marketplace, JetBrains, package mirror endpoints |
| Nexus mirrors | Phase 0 | npm, Maven, PyPI, NuGet mirrors operational |
| nftables + CoreDNS | Phase 2 | IDE connections (localhost) and tool downloads (via proxy) allowed |
| LLM sidecar proxy | Phase 2 | AI tools reach LLM API via `localhost:8443` |
| Credential broker / Vault | Phase 3 | API keys, Git tokens, dotfiles repo auth |
| OPA policy engine | Phase 3 | `review-required` risk class for git push approval |
| Image signing (Cosign) | Phase 0 | Tool pack manifests and images signed and verified |

### Downstream (what depends on Phase 4)

| Dependent | Target | What It Needs |
|-----------|--------|---------------|
| Pilot rollout | Phase 6 | IDE integration and tool packs working reliably |
| Training materials | Phase 6 | IDE quickstarts depend on Phase 4 deliverables |
| Additional tool/MCP packs | Phase 6+ | Systems must be stable and documented for self-service |

---

## Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|-----------|--------|-----------|
| R1 | VS Code Server version mismatch causes connection failures | Medium | High | Pre-install latest stable in weekly rebuilds. Allow download fallback. Test with Stable, Insiders, and previous 2 releases. |
| R2 | JetBrains backend OOM-kills container | Medium | High | Benchmark early. Provide `--profile jetbrains` with 12GB+ RAM. |
| R3 | gVisor breaks polyglot debuggers or build tools (.NET CLR, Bazel sandboxing) | Medium | High | Build compatibility matrix. Document limitations (no ptrace). Provide runc fallback with compensating controls. |
| R4 | Tool pack install too slow | Low | Medium | Docker-layer method (fast extraction). Image variants for top stacks. Cache in persistent volumes. Target < 30s for pre-built packs. |
| R5 | Git push approval loses commits or fails silently | Low | Critical | Extensive testing. Retry logic. `aibox push status` for visibility. Never delete staging ref without explicit action. |
| R6 | Tool pack supply chain attack | Low | Critical | All packs signed with Cosign. Security review for new packs. Harbor vulnerability scanning. |
| R7 | Developer adoption resistance from accumulated friction | High | High | Beta testing with pilot developers. Fix top 5 pain points before expanding. Measure startup time, build time, satisfaction weekly. |

---

## Exit Criteria

### IDE Integration
- [ ] VS Code connects via Remote SSH within 5s (reconnect) or 15s (warm start)
- [ ] VS Code provides full functionality: editing, terminal, debugging, extensions, Git
- [ ] JetBrains Gateway connects and runs backend inside container
- [ ] JetBrains backend persists across container restarts
- [ ] Node.js, Java, and Python debugging works via IDE
- [ ] Port forwarding works for web app dev servers
- [ ] Hot reload (inotify) works with no perceptible delay

### Tool Packs
- [ ] `aibox install java@21` completes in < 60 seconds
- [ ] All 10 initial tool packs build, publish, and install successfully
- [ ] `dotnet restore` works through Nexus proxy
- [ ] Tool pack signatures verified on install; unsigned packs rejected
- [ ] Build caches (Maven, Gradle, npm, NuGet) persist across restarts

### MCP Packs
- [ ] `aibox mcp enable filesystem-mcp git-mcp` generates valid config
- [ ] Claude Code discovers and uses configured MCP servers
- [ ] MCP config regenerates correctly on container restart

### Shell & Dotfiles
- [ ] Dotfiles repo clones into persistent home on first start
- [ ] Shell history persists across `aibox stop && aibox start`
- [ ] bash, zsh, and pwsh work with correct PATH and proxy settings

### Git Push Approval
- [ ] `git push` with `review-required` policy pushes to staging ref and notifies approver
- [ ] Developer is NOT blocked after push
- [ ] Approved push merges to target branch; rejected push notifies with reason
- [ ] `aibox push status` shows pending approvals

### Performance
- [ ] Build performance within 20% of local baseline for Java, Node, .NET, and Python projects
- [ ] AI tool response latency overhead from sidecar proxy < 50ms

---

## Estimated Effort

| Work Stream | Effort | Engineers | Notes |
|-------------|--------|-----------|-------|
| IDE Integration (VS Code + JetBrains + Debugging) | 1.5 weeks | 1 | Server pre-install, SSH config, extensions, port forwarding, debug verification |
| Tool Pack System + Initial Packs (10 packs) | 3 weeks | 1-2 | Manifest schema, install command, registry, build/test 10 packs (3 new polyglot packs add ~1 week) |
| MCP Pack System | 1 week | 1 | Manifest, commands, config generation |
| Dotfiles & Shell | 0.5 weeks | 1 | Dotfiles clone, shell config, tmux, persistent history |
| Git Push Approval Flow | 1 week | 1 | Novel mechanism requiring thorough testing |
| Documentation (incl. CLI migration guide) | 1 week | 1 | IDE quickstarts, authoring guides, PowerShell migration, CLI migration |
| **Total** | **~8-10 weeks** | **3 engineers** | **~3-3.5 weeks calendar with 3 engineers in parallel** |

**Parallelization**: IDE Integration + Dotfiles & Shell by one engineer. Tool Pack System + Initial Packs by one or two engineers. MCP Packs + Git Push Approval + Documentation by a third. Documentation distributed across all three.

**Research effort** (pre-implementation): 1-2 engineer-weeks for the three research items. Can overlap with late Phase 3 work.
