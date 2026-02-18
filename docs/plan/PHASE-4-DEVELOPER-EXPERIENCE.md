# Phase 4: Developer Experience

**Phase**: 4 of 6
**Estimated Effort**: 6-7 engineer-weeks
**Team Size**: 3 engineers
**Calendar**: Weeks 11-14
**Dependencies**: Phase 1 (CLI/Runtime), Phase 2 (Network Security), Phase 3 (Policy Engine & Credentials)

---

## Overview

Phase 4 transforms AI-Box from a secure but bare container into a development environment that feels better than local development. This is the adoption gate -- developers will reject AI-Box regardless of its security properties if the daily workflow is painful.

This phase delivers eight work streams: VS Code and JetBrains IDE integration via SSH remote development, the tool pack system with initial language/framework packs, the MCP pack system for AI agent tool discovery, dotfiles and shell customization, the non-blocking `git push` approval flow, and debugging support. Every work stream targets the spec's "invisible security" principle (Section 4): if developers notice friction, the phase has failed.

The phase depends on all three prior phases. IDE connections traverse the SSH server inside the container (Phase 1). Tool downloads route through the Squid proxy and Nexus mirror (Phase 2). AI tool API keys come from the credential broker or Vault (Phase 3). The `git push` approval flow relies on the policy engine's `review-required` risk class (Phase 3).

---

## Deliverables

1. **VS Code Remote SSH integration** -- pre-installed VS Code Server in base image, SSH config auto-generation, extension pre-loading, telemetry blocking
2. **JetBrains Gateway integration** -- SSH-based backend connection, resource allocation guidance, backend pre-cache support
3. **SSH server configuration** -- OpenSSH in container (port 22 mapped to host 2222), key-based auth from host
4. **Tool pack system** -- manifest schema implementation, `aibox install <pack>` command, runtime installation into `/opt/toolpacks`, signature verification
5. **Initial tool packs** -- `java@21`, `node@20`, `python@3.12`, `bazel@7`, `scala@3`, `angular@18`, `ai-tools`
6. **Pre-built image variants** -- `aibox-java:21-24.04`, `aibox-node:20-24.04`, `aibox-full:24.04` updated to include tool pack contents
7. **MCP pack system** -- `aibox mcp enable/list/disable` commands, auto-generated MCP config for AI agent discovery
8. **Initial MCP packs** -- `filesystem-mcp`, `git-mcp`
9. **Dotfiles and shell customization** -- dotfiles repo cloning into persistent home volume, bash + zsh + tmux, persistent shell history
10. **Git push non-blocking approval flow** -- staging ref mechanism, webhook notifications, async approve/reject, merge-on-approval
11. **Debugging support** -- port forwarding for debug adapters, JDWP/Node inspector configuration, hot reload via native inotify
12. **Documentation** -- VS Code quickstart, JetBrains quickstart, tool pack authoring guide, MCP pack guide

---

## Implementation Steps

### Work Stream 1: VS Code Remote SSH

**What to build**:

- Pre-install VS Code Server (`code-server` or the official `vscode-server` binary) into the base image during the image build pipeline (Phase 0 infrastructure). This eliminates the 30-60 second first-connect download that occurs when VS Code Remote SSH connects to a host without a pre-installed server.
- Configure the SSH server in the container to accept connections on port 22, mapped to host port 2222 via Podman port forwarding.
- Add an `aibox` CLI hook that writes/updates the user's `~/.ssh/config` on the host with an entry for the AI-Box container:
  ```
  Host aibox
    HostName localhost
    Port 2222
    User dev
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
  ```
- Pre-install a curated set of VS Code extensions into the server image (language support, Git, debugging). Maintain the list in the `aibox-images` repo.
- Disable all telemetry: set `"telemetry.telemetryLevel": "off"` in the pre-installed VS Code Server settings. Block known telemetry endpoints (e.g., `dc.services.visualstudio.com`, `vortex.data.microsoft.com`) at the Squid proxy level (Phase 2 allowlist -- these are simply never added).
- Optionally proxy VS Code Marketplace access through the egress allowlist for extension installation. Decide whether marketplace access is allowed per-org or blocked entirely (extensions baked into image only).

**Key decisions**:

1. **Which VS Code Server binary to pre-install?** The official `vscode-server` binary (used by Remote SSH) is versioned and tied to the client VS Code version. Options: (a) pre-install the latest stable and accept minor version mismatch prompts, (b) pre-install multiple recent versions, (c) allow first-connect download through the proxy. Recommendation: (a) pre-install latest stable in weekly image rebuilds; VS Code handles minor version mismatches gracefully.
2. **Extension marketplace access**: Allow via proxy (more flexible, devs can install extensions) vs. baked-in only (more controlled, no marketplace egress). Recommendation: allow marketplace via proxy for initial rollout; lock down if security review requires it.
3. **SSH key management**: Generate a key pair during `aibox setup` and inject the public key into the container. The private key stays on the host only. Do not reuse the developer's personal SSH keys.

**Spec references**: Section 13.1 (VS Code), Section 13.3 (Connection Model), Section 18.1 (Performance SLAs -- reconnect < 5s).

---

### Work Stream 2: JetBrains Gateway

**What to build**:

- Ensure the container's SSH server supports the JetBrains Gateway connection protocol (standard SSH, no special requirements beyond a working SFTP subsystem).
- Document the JetBrains Gateway connection flow: open Gateway, select "SSH Connection", enter `localhost:2222`, user `dev`, authenticate via key.
- JetBrains Gateway downloads its backend (IDE backend for IntelliJ, WebStorm, etc.) into the container on first connect. This is a ~500MB-1GB download. Ensure the download routes through the Squid proxy and that `download.jetbrains.com` (or the internal mirror) is on the allowlist.
- Store the JetBrains backend in the persistent home volume (`/home/dev/.cache/JetBrains`) so it survives container restarts. This avoids re-downloading after `aibox stop && aibox start`.
- Set minimum resource requirements for containers running JetBrains backend: 4 CPU cores, 8GB RAM (spec Section 13.2). The `aibox` CLI should warn or auto-adjust resource allocation when JetBrains is detected or configured.
- Optionally pre-download the JetBrains backend into the base image or a dedicated tool pack (`jetbrains-backend@2025.1`) to eliminate the first-connect download entirely.

**Key decisions**:

1. **Pre-download vs. on-demand**: Pre-downloading the JetBrains backend eliminates first-connect latency but increases image size by ~1GB and ties the image to a specific IDE version. Recommendation: on-demand download for initial release (cached in persistent volume), with a `jetbrains-backend` tool pack as a follow-up optimization.
2. **Resource auto-adjustment**: Should `aibox start` auto-increase memory when a JetBrains project is detected (e.g., `.idea/` directory present)? Recommendation: yes, with a `--profile` flag (e.g., `aibox start --profile jetbrains`) that sets 8GB+ RAM, 4+ cores.
3. **License validation**: JetBrains requires license validation. The license server endpoint must be added to the egress allowlist (or an on-prem license server used). Coordinate with the organization's JetBrains license admin.

**Spec references**: Section 13.2 (JetBrains Gateway), Section 18.2 (Resource Allocation -- 4 cores, 8-12GB for JetBrains backend workloads).

---

### Work Stream 3: Tool Pack System

**What to build**:

- Implement the tool pack manifest schema (spec Section 15.2) as a YAML format with fields: `name`, `version`, `description`, `maintainer`, `install` (method, base_image, packages), `network` (required endpoints), `filesystem` (created paths, caches), `resources` (min/recommended memory), `security` (checksum, signature), `tags`.
- Implement `aibox install <pack>@<version>` command that:
  1. Fetches the manifest from a tool pack registry (Harbor OCI artifacts or a Git repo).
  2. Verifies the manifest signature (Cosign).
  3. Checks that the pack's network requirements are satisfied by the current policy.
  4. Installs the pack into `/opt/toolpacks/<pack>` using the declared method.
  5. Updates `$PATH` and environment variables.
  6. Registers the pack so `aibox status` shows installed packs.
- Support three install methods:
  - **docker-layer**: Pull an OCI layer containing the pre-built tool binaries and extract into `/opt/toolpacks`. Fastest, most deterministic.
  - **nix**: Use Nix package manager to install (if Nix is available in the image). Most reproducible.
  - **script**: Run a shell script with network access to download and install. Most flexible, least deterministic. Only allowed for org-approved scripts.
- Implement `aibox install --toolpacks java@21,node@20` flag on `aibox start` for specifying packs at launch time.
- Build the tool pack registry: either a dedicated Harbor project storing OCI artifacts, or a Git repo (`aibox-toolpacks`) with manifest files and build scripts.
- Implement `aibox list packs` to show available packs from the registry.

**Key decisions**:

1. **Primary install method**: Docker layers (OCI artifacts) vs. Nix vs. scripts. Recommendation: docker-layer as the primary method (pre-built, fast, deterministic). Nix as a secondary option for teams that already use it. Scripts only as a last resort with security review.
2. **Tool pack registry location**: Harbor OCI artifacts vs. Git repo. Recommendation: Harbor OCI artifacts for binary packs (fast pull, signature verification built in), Git repo for manifests and build scripts.
3. **Runtime install vs. image variants**: Tool packs installed at runtime are slower on first use but more flexible. Image variants are faster but less composable. Recommendation: ship both. Image variants for the top 3 stacks (Java, Node, Full), runtime install for everything else.
4. **Version pinning**: Tool packs declare exact versions (e.g., `java@21.0.2`, not `java@21`). The `@21` shorthand resolves to the latest patch. Manifests are immutable once published.

**Spec references**: Section 5.4 (Tool Packs), Section 15 (Tool Packs -- all subsections), Section 17.2 (Image Variants).

---

### Work Stream 4: Initial Tool Packs

**What to build**:

Build and publish the initial set of tool packs. Each requires a manifest, build script, CI integration, and testing.

| Pack | Contents | Target Size | Build Source |
|------|----------|-------------|-------------|
| `java@21` | OpenJDK 21.0.2 (Temurin), Gradle 8.5, Maven 3.9.6 | ~400MB | `eclipse-temurin:21-jdk` base layer |
| `node@20` | Node.js 20 LTS, npm 10.x, yarn 1.22.x | ~150MB | Official Node.js binaries |
| `python@3.12` | Python 3.12, pip, venv, setuptools | ~100MB | Ubuntu `python3.12` packages |
| `bazel@7` | Bazel 7.x | ~200MB | Bazelisk binary |
| `scala@3` | Scala 3.x, sbt 1.x, Metals LSP | ~300MB | Coursier-installed tools |
| `angular@18` | Angular CLI 18.x (requires `node@20`) | ~50MB | npm global install |
| `ai-tools` | Claude Code CLI, Codex CLI | ~100MB | Official install scripts |

For each pack:
1. Write the manifest YAML per the schema.
2. Create the build script (Dockerfile for docker-layer method).
3. Build and push to Harbor as a signed OCI artifact.
4. Test: `aibox install <pack>` succeeds, binaries are on `$PATH`, a basic build/run works.
5. Configure named volumes for build caches (e.g., `aibox-maven-cache -> /home/dev/.m2/repository`).

**Key decisions**:

1. **ai-tools pack contents**: Include Claude Code and Codex CLI. Future agents added as they become available. The AI tools in the pack must be configured to use the LLM sidecar proxy (environment variables set automatically).
2. **Dependency handling**: `angular@18` depends on `node@20`. The tool pack system should check for and auto-install dependencies, or at minimum report a clear error if a dependency is missing.
3. **Cache volume names**: Use a consistent naming convention: `aibox-<tool>-cache`. Caches persist across container recreations and image updates.

**Spec references**: Section 15.3 (Available Tool Packs), Section 15.4 (Governance), Section 10.4 (Build Cache Persistence).

---

### Work Stream 5: MCP Pack System

**What to build**:

- Implement the MCP pack manifest schema. Each MCP pack declares: `name`, `version`, `description`, `command` (binary path), `args`, `network_requires` (endpoints needed), `filesystem_requires` (paths needed), `permissions` (what the MCP server can do).
- Implement `aibox mcp enable <pack> [<pack>...]` command that:
  1. Validates the pack's network and filesystem requirements against the current policy (via OPA).
  2. Installs the MCP server binary if not already present.
  3. Generates the MCP configuration file that AI agents discover. For Claude Code: `~/.config/claude/claude_desktop_config.json`. For other agents: their respective config locations.
  4. Starts the MCP server process (or configures it for on-demand start by the AI agent).
- Implement `aibox mcp list` to show available and enabled MCP packs.
- Implement `aibox mcp disable <pack>` to remove an MCP pack from the agent config.
- Auto-generate the MCP configuration on `aibox start` based on enabled packs. The configuration must be in the format each AI agent expects (spec Section 14.4).

**Key decisions**:

1. **MCP config file location**: Different AI agents look for MCP config in different locations. The system needs to generate config files for all supported agents, or use a convention that agents are configured to read from. Recommendation: generate a canonical `~/.config/aibox/mcp.json` and symlink or copy to agent-specific locations.
2. **MCP server lifecycle**: Start all enabled MCP servers on container boot vs. on-demand start by the AI agent. Recommendation: on-demand (agent launches the MCP server process when it needs it), which is the standard MCP pattern. The config file just declares how to start them.
3. **Permission enforcement**: MCP packs that need network access (e.g., `jira-mcp`) must have their required endpoints in the policy allowlist. If the policy doesn't allow the endpoint, `aibox mcp enable jira-mcp` should fail with a clear message explaining which policy change is needed.

**Spec references**: Section 5.5 (MCP Packs), Section 14.4 (MCP Server Discovery), Section 16 (MCP Packs -- all subsections).

---

### Work Stream 6: Dotfiles & Shell

**What to build**:

- Implement dotfiles sync in the `aibox` CLI. The developer configures a dotfiles Git repo URL (e.g., `aibox config set dotfiles.repo https://git.internal/user/dotfiles.git`). On `aibox start`, the CLI clones or pulls the repo into the persistent home volume (`/home/dev`).
- Support a standard dotfiles install mechanism: if the dotfiles repo contains an `install.sh` or `Makefile`, run it. Otherwise, symlink standard dotfiles (`.bashrc`, `.zshrc`, `.vimrc`, `.gitconfig`, `.tmux.conf`) from the repo into `$HOME`.
- Ensure `bash` and `zsh` are both available in the base image. The default shell is configurable via `aibox config set shell zsh`.
- Pre-install `tmux` for terminal multiplexing and session persistence. Include a sensible default `.tmux.conf`.
- Ensure shell history (`.bash_history`, `.zsh_history`) is stored in the persistent home volume and survives container recreation.
- Ensure `$HOME` (`/home/dev`) is on a named volume that persists across `aibox stop/start` cycles and across image updates.
- Set up standard environment variables: `$EDITOR`, `$TERM`, `$LANG`, locale settings. Allow user override via dotfiles.

**Key decisions**:

1. **Dotfiles repo auth**: The dotfiles repo clone needs Git credentials. Use the same `aibox-credential-helper` (Vault-backed, Phase 3) for authentication. The dotfiles repo must be on the Git server allowlist.
2. **Dotfiles conflict resolution**: If the user's dotfiles conflict with AI-Box required settings (e.g., proxy environment variables), AI-Box settings take precedence. Source an `aibox-env.sh` at the end of shell rc files to ensure proxy vars and PATH entries are correct.
3. **Shell startup performance**: Avoid bloating shell startup. The `aibox-env.sh` sourced at the end should be minimal (set proxy vars, PATH for tool packs, MCP config path).

**Spec references**: Section 13.5 (Shell and Dotfiles), Section 18.4 (Shell and Dotfiles).

---

### Work Stream 7: Git Push Approval Flow

**What to build**:

- Implement the non-blocking `git push` approval flow for repositories where the policy sets `git push` to `review-required` risk class.
- The flow:
  1. Developer runs `git push origin main` inside the container.
  2. The `aibox-agent` (policy enforcement process in the container) intercepts the push via a Git `pre-push` hook or a custom Git remote helper.
  3. Instead of pushing directly to `main`, the push goes to a staging ref: `refs/aibox/staging/<user>/<timestamp>` on the target remote.
  4. The agent creates an approval request: records the source branch, target branch, commit range, and developer identity.
  5. A webhook fires to notify the approver(s) via the configured channel (Slack webhook, email, or a web dashboard).
  6. The developer sees a message: "Push staged for approval. You can continue working. Approval status: `aibox push status`."
  7. The developer is NOT blocked -- they continue coding.
  8. The approver reviews the staged commits (via a web UI, Git diff, or Slack interactive message).
  9. On approval: the staging ref is merged/fast-forwarded to the target branch. The developer is notified.
  10. On rejection: the staging ref is deleted. The developer is notified with the reason.
- Implement `aibox push status` command to check pending push approvals.
- Implement `aibox push cancel` to withdraw a pending push.
- For repos where `git push` is `safe` (no review required), the push proceeds normally without interception.

**Key decisions**:

1. **Interception mechanism**: Git `pre-push` hook vs. custom Git remote helper vs. Git wrapper script. Recommendation: custom Git remote helper (`aibox-git-remote-helper`) configured via `remote.<name>.pushurl` that rewrites the target ref. This is transparent to the developer and works with all Git workflows. Fallback: a `pre-push` hook that rewrites refs.
2. **Approval notification channel**: Slack webhook (fastest, most visible), email (universal, slower), web dashboard (most detail, requires building a UI). Recommendation: Slack webhook for MVP, with email as fallback. Dashboard as a Phase 6 enhancement.
3. **Approver selection**: Who approves? Options: team lead, any team member, security team. This should be configurable in the team policy. Default: team lead.
4. **Staging ref location**: Push to the same remote but under a special ref namespace. This avoids needing a separate staging server. The ref namespace (`refs/aibox/staging/`) should be protected on the Git server so only the approval system can promote refs out of it.
5. **Timeout**: Staged pushes that are not approved within a configurable timeout (default: 48 hours) are auto-rejected and the developer is notified.

**Spec references**: Section 12.4 (Tool Permission Model -- `review-required` risk class), Section 18.5 (Git Push Approval Flow).

---

### Work Stream 8: Debugging Support

**What to build**:

- Ensure all standard debugging protocols work inside the container without extra configuration:
  - **Node.js**: `--inspect` flag starts the debug adapter on a port inside the container. VS Code connects via the SSH tunnel automatically (Remote SSH handles port forwarding).
  - **Java (JDWP)**: JVM debug port inside the container. JetBrains Gateway backend connects to it directly (same host). VS Code Java extension connects via SSH tunnel.
  - **Python**: `debugpy` runs inside the container. VS Code Python extension connects via SSH tunnel.
  - **Browser DevTools**: For frontend debugging, port-forward from the container's dev server (e.g., port 3000, 4200) to the host. `aibox` CLI or VS Code Remote SSH handles this automatically.
- Implement `aibox port-forward <container-port> [<host-port>]` for manual port forwarding when needed (e.g., debugging a web app, connecting to a database UI inside the container).
- Ensure hot reload works correctly: file watchers (inotify) operate on native ext4 inside the container with no cross-OS latency. This is inherently fast because the workspace is on a Linux filesystem (spec Section 10.2). No additional work needed beyond ensuring inotify limits are set correctly in the container (`fs.inotify.max_user_watches`).
- Verify that gVisor's inotify implementation supports the watch patterns used by common build tools (webpack, Vite, Gradle continuous build, Spring Boot DevTools). Document any known limitations.
- Pre-configure VS Code launch configurations (`.vscode/launch.json` templates) for common debugging scenarios. Ship these as part of the quickstart documentation rather than injecting them into projects.

**Key decisions**:

1. **Port forwarding mechanism**: Podman port forwarding (`-p`) at container start vs. SSH tunnel-based forwarding vs. dynamic forwarding via `aibox port-forward`. Recommendation: SSH tunnel-based for IDE debug adapters (handled automatically by VS Code Remote SSH and JetBrains Gateway). `aibox port-forward` as a CLI utility for manual use cases (web apps, database UIs).
2. **inotify limits**: gVisor has its own inotify implementation. Default limits may be too low for large projects (Angular/React with thousands of files). Set `fs.inotify.max_user_watches=524288` inside the container.
3. **gVisor compatibility**: gVisor's syscall coverage may affect some debuggers (e.g., `ptrace` is blocked by seccomp). Debuggers that use `ptrace` (like `strace`, GDB) will not work under gVisor. Document this limitation and recommend using IDE-integrated debug adapters (which use debug protocols, not ptrace) instead.

**Spec references**: Section 13.4 (Debugging), Section 9.2 (Seccomp Profile -- ptrace blocked), Section 10.2 (Source Code Location -- native ext4 performance).

---

## Research Required

### 1. VS Code Server Pre-install Approach

**Question**: What is the best way to pre-install VS Code Server in the base image so that Remote SSH connections do not trigger a download?

**Research tasks**:
- Determine if the official VS Code Server binary (`vscode-server`) can be extracted and pre-installed. Check licensing terms for redistribution in internal images.
- Test whether pre-installing a specific VS Code Server version causes issues when the developer's local VS Code client is a different (newer/older) version. Document version compatibility ranges.
- Evaluate using `code-server` (open source alternative) vs. the official binary. `code-server` is freely redistributable but may have feature gaps.
- Measure the first-connect time with and without pre-installed server to quantify the improvement.

**Why this matters**: First-connect experience is critical for adoption. A 60-second wait on first connection will generate support tickets and frustration.

### 2. JetBrains Gateway Backend Resource Requirements

**Question**: What are the actual resource requirements for JetBrains Gateway backend running inside a container with gVisor, and how does this interact with AI agent memory usage?

**Research tasks**:
- Benchmark JetBrains backend (IntelliJ IDEA, WebStorm) memory and CPU usage inside a gVisor container for representative project sizes (small: 50 files, medium: 500 files, large: 5000+ files / monorepo).
- Measure the combined memory footprint: JetBrains backend + AI agent (Claude Code) + build tools (Gradle/Node) + MCP servers. Determine if 8GB is sufficient or if 12-16GB is the realistic minimum.
- Test JetBrains backend startup time inside gVisor vs. standard runc. Quantify any overhead.
- Verify that JetBrains backend indexing does not trigger gVisor syscall compatibility issues.
- Test the persistent volume approach for JetBrains backend cache (`/home/dev/.cache/JetBrains`). Verify that backend updates work correctly with cached versions.

**Why this matters**: Underestimating resource requirements leads to poor performance, which leads to developer rejection. The spec allocates 8-12GB for backend workloads (Section 18.2), but this has not been validated under gVisor with concurrent AI agent use.

### 3. Tool Pack Install Methods

**Question**: What is the optimal method for installing tool packs at runtime without rebuilding the base image?

**Research tasks**:
- **Docker layers / OCI artifacts**: Prototype extracting an OCI layer into `/opt/toolpacks`. Measure install time for the Java tool pack (~400MB). Test that extracted binaries work correctly (library paths, shared objects).
- **Nix**: Evaluate Nix-in-container feasibility. Nix store can be a named volume. Measure install time and disk usage for the Java tool pack. Assess whether Nix's daemon requirements conflict with the rootless/read-only container constraints.
- **Scripts**: Baseline comparison. Download and untar binaries via script. Fastest to author, hardest to reproduce.
- Compare all three methods on: install time, disk usage, reproducibility, security (signature verification), compatibility with gVisor, and maintenance burden.
- Prototype the manifest schema and `aibox install` command with the recommended method.

**Why this matters**: The tool pack system is a core extensibility mechanism. If installation is slow or fragile, teams will resist using it and demand custom images instead, defeating the composable design.

### 4. MCP Server Discovery by Different AI Agents

**Question**: How do different AI agents (Claude Code, Codex CLI, Aider, Continue, custom agents) discover and connect to MCP servers, and how can AI-Box generate a universal configuration?

**Research tasks**:
- Document the MCP config file location and format for each supported agent: Claude Code (`~/.config/claude/`), Codex CLI, Continue (`.continue/` config), Aider, and any other agents the organization plans to support.
- Determine if there is a standard MCP discovery mechanism (e.g., a well-known path or environment variable) that multiple agents support, or if each agent has its own config format.
- Prototype auto-generating MCP config for at least Claude Code and one other agent. Test that the agents correctly discover and use the configured MCP servers.
- Evaluate whether MCP servers should run as long-lived daemons or be launched on-demand by the agent. Different agents may have different expectations.
- Research MCP server stdio vs. HTTP transport. Determine which transport each agent supports and which is preferred for the sandbox use case.

**Why this matters**: AI-Box is explicitly "assistant/tool/MCP agnostic" (spec Section 3). If MCP discovery only works for one agent, the system fails its design goal.

### 5. Non-Blocking Git Push Approval Implementation

**Question**: What is the most reliable mechanism to intercept `git push`, redirect to a staging ref, and implement async approval without blocking the developer?

**Research tasks**:
- **Git remote helper approach**: Prototype a custom Git remote helper that rewrites push targets. Test with `git push`, `git push --force`, `git push origin feature:main`, and other push variations. Verify it handles all Git push semantics correctly.
- **Pre-push hook approach**: Prototype a `pre-push` hook that stages commits to a different ref. Test edge cases: concurrent pushes, push with multiple refs, push with tags.
- **Git server-side approach**: Evaluate server-side hooks (pre-receive, update) as an alternative. This moves complexity to the Git server but simplifies the client. May not be feasible if the org uses a hosted Git service without custom hooks.
- Research notification delivery mechanisms: Slack incoming webhooks (reliability, rate limits), email (SMTP access through proxy), custom webhook to an internal approval service.
- Prototype the full approval flow end-to-end: push to staging ref, notification sent, approval received, staging ref promoted to target branch. Measure total latency from push to branch update.
- Research how GitHub/GitLab protected branches interact with staging refs. Ensure the staging namespace does not conflict with branch protection rules.

**Why this matters**: This is a novel workflow that no existing tool provides out of the box. If the implementation is buggy (lost pushes, failed promotions, stuck approvals), developers will lose trust in the entire system.

---

## Open Questions

1. **VS Code Marketplace access**: Should the egress allowlist include `marketplace.visualstudio.com` for extension installation, or should all extensions be baked into the image? Trade-off: flexibility vs. attack surface. This needs a decision from the security team.

2. **JetBrains license server**: Does the organization have an on-premises JetBrains license server, or does license validation need to reach external JetBrains servers? This affects the egress allowlist.

3. **Tool pack self-service**: Can team leads approve and publish tool packs without platform team involvement, or does every new pack require security review? The spec (Section 15.4) says "known/registered tools: 1 business day" -- does this mean pre-approved tools are instant, and only truly new tools need review?

4. **MCP pack scope**: Should Phase 4 include the `jira-mcp` and `docs-mcp` packs (which require additional network endpoints), or limit initial scope to `filesystem-mcp` and `git-mcp` (which work within existing allowlists)? Recommendation: start with filesystem and git only; Jira/docs are Phase 6 enhancements.

5. **Git push approval UX**: When a push is rejected by the approver, what is the developer's recovery path? Do they amend and re-push (which creates a new staging ref and new approval request)? Is there a "re-request review" mechanism?

6. **Multiple AI agents simultaneously**: Can a developer run Claude Code and Codex CLI simultaneously inside the same container? If so, do they share MCP servers or each get their own instances? The spec is silent on multi-agent scenarios.

7. **Debugger compatibility with gVisor**: Which debuggers and profilers are known to fail under gVisor's syscall restrictions? Need to build a compatibility matrix before rollout.

---

## Dependencies

### Upstream (what Phase 4 needs from earlier phases)

| Dependency | Source Phase | Specific Requirement |
|-----------|-------------|---------------------|
| Running container with SSH server | Phase 1 | Container launches with OpenSSH on port 22, mapped to host 2222 |
| Base image build pipeline | Phase 0 | Able to add VS Code Server and tool packs to image builds |
| Squid proxy with allowlist | Phase 2 | Marketplace, JetBrains download, and package mirror endpoints accessible |
| Nexus mirrors | Phase 0 | npm, Maven, PyPI mirrors operational for tool pack dependencies |
| nftables + CoreDNS | Phase 2 | Network rules allow IDE connections (localhost only) and tool downloads (via proxy) |
| LLM sidecar proxy | Phase 2 | AI tools (Claude Code, Codex) can reach LLM API via `localhost:8443` |
| Credential broker / Vault | Phase 3 | API keys for AI tools, Git tokens for push flow, dotfiles repo auth |
| OPA policy engine | Phase 3 | `review-required` risk class enforcement for git push approval |
| Policy hierarchy | Phase 3 | Tool permission model operational for MCP pack permission validation |
| Image signing (Cosign) | Phase 0 | Tool pack manifests and images are signed and verified |

### Downstream (what depends on Phase 4)

| Dependent | Target Phase | What It Needs |
|-----------|-------------|---------------|
| Pilot rollout | Phase 6 | IDE integration and tool packs must work reliably before pilots begin |
| Training materials | Phase 6 | VS Code and JetBrains quickstarts depend on Phase 4 deliverables |
| Champions program | Phase 6 | Champions need tool pack authoring guide to support their teams |
| Additional tool packs | Phase 6+ | Tool pack system must be stable and documented for self-service pack creation |
| Additional MCP packs | Phase 6+ | MCP pack system must be extensible for Jira, docs, and custom MCP servers |

---

## Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|-----------|--------|-----------|
| R1 | VS Code Server version mismatch causes connection failures | Medium | High | Pre-install latest stable in weekly rebuilds. Allow download fallback through proxy. Test with VS Code Stable, Insiders, and previous 2 releases. |
| R2 | JetBrains backend consumes too much memory, OOM-kills container | Medium | High | Benchmark actual usage early. Set memory limits with headroom. Provide `--profile jetbrains` with 12GB+ RAM. Document minimum requirements prominently. |
| R3 | gVisor syscall gaps break debuggers or build tools | Medium | High | Build compatibility matrix during research phase. Document known limitations (no ptrace = no strace/GDB). Provide runc escape hatch for debugging-only sessions with compensating controls (no network access). |
| R4 | Tool pack installation is too slow for acceptable DX | Low | Medium | Prioritize docker-layer method (fast extraction). Pre-build image variants for top 3 stacks. Cache tool packs in persistent volumes. Target < 30s install for pre-built packs. |
| R5 | Git push approval flow loses commits or fails silently | Low | Critical | Extensive testing of staging ref mechanism. Implement retry logic. `aibox push status` for visibility. Alert on stuck approvals. Never delete a staging ref without explicit action. |
| R6 | MCP config generation breaks when AI agents update their config format | Medium | Medium | Pin MCP config schema versions. Test with each agent's latest release in CI. Provide `aibox mcp regenerate` command for recovery. |
| R7 | Dotfiles repo conflicts with AI-Box required settings (proxy, PATH) | High | Low | Source `aibox-env.sh` last in shell rc. Document which environment variables AI-Box manages. Provide clear error messages when conflicts are detected. |
| R8 | Tool pack supply chain attack (compromised tool binary) | Low | Critical | All packs signed with Cosign. Security review for new packs. Checksum verification on install. Harbor vulnerability scanning of pack images. |
| R9 | Developer adoption resistance due to accumulated small frictions | High | High | Extensive beta testing with pilot developers (Phase 6 overlap). Fix top 5 pain points before expanding. Measure startup time, build time, and developer satisfaction weekly. |

---

## Exit Criteria

All of the following must be true before Phase 4 is considered complete:

### IDE Integration
- [ ] VS Code connects to the sandbox via Remote SSH within 5 seconds (reconnect) or 15 seconds (warm start with pre-installed server)
- [ ] VS Code provides full functionality: editing, terminal, debugging, extensions, Git integration
- [ ] JetBrains Gateway connects and runs backend inside the container
- [ ] JetBrains backend persists in the home volume across container restarts (no re-download)

### Tool Packs
- [ ] `aibox install java@21` adds JDK, Maven, and Gradle to a running container in < 60 seconds
- [ ] All 7 initial tool packs build, publish, and install successfully
- [ ] Tool pack signatures are verified on install; unsigned packs are rejected
- [ ] Build caches (Maven, Gradle, npm) persist across container restarts

### MCP Packs
- [ ] `aibox mcp enable filesystem-mcp git-mcp` generates valid MCP configuration
- [ ] Claude Code discovers and uses the configured MCP servers
- [ ] MCP config regenerates correctly on container restart

### Shell & Dotfiles
- [ ] Dotfiles repo clones into the persistent home volume on first start
- [ ] Shell history persists across `aibox stop && aibox start` cycles
- [ ] Both bash and zsh work with correct PATH and proxy settings

### Git Push Approval
- [ ] `git push` with `review-required` policy pushes to staging ref and notifies approver
- [ ] Developer is NOT blocked after push (can continue working immediately)
- [ ] Approved push merges staging ref to target branch
- [ ] Rejected push notifies developer with reason
- [ ] `aibox push status` shows pending approvals

### Debugging
- [ ] Node.js debugging works via VS Code Remote SSH (attach to running process)
- [ ] Java JDWP debugging works via JetBrains Gateway
- [ ] Port forwarding works for web app dev servers (localhost:3000 accessible from host)
- [ ] Hot reload (inotify-based) works with no perceptible delay

### Performance
- [ ] Build performance within 20% of local baseline for Java (Gradle), Node (npm), and Python (pip) projects
- [ ] AI tool (Claude Code) response latency overhead from sidecar proxy < 50ms

### Documentation
- [ ] VS Code quickstart guide published and reviewed
- [ ] JetBrains quickstart guide published and reviewed
- [ ] Tool pack authoring guide published and reviewed
- [ ] MCP pack guide published and reviewed

---

## Estimated Effort

| Work Stream | Effort | Engineers | Notes |
|-------------|--------|-----------|-------|
| VS Code Remote SSH | 1 week | 1 | Includes server pre-install, SSH config, extension management |
| JetBrains Gateway | 0.5 weeks | 1 | Largely configuration; more effort if pre-download is pursued |
| Tool Pack System | 1.5 weeks | 1-2 | Manifest schema, install command, registry integration, signature verification |
| Initial Tool Packs | 1 week | 1-2 | Build and test 7 packs in parallel; some are straightforward (Node), some complex (Scala) |
| MCP Pack System | 1 week | 1 | Manifest, enable/list/disable commands, config generation |
| Dotfiles & Shell | 0.5 weeks | 1 | Dotfiles clone, shell config, tmux, persistent history |
| Git Push Approval Flow | 1 week | 1 | Most complex work stream; novel mechanism requiring thorough testing |
| Debugging Support | 0.5 weeks | 1 | Mostly verification and configuration; port forwarding utility |
| Documentation | 0.5 weeks | 1 | Quickstarts, authoring guides, reference docs |
| **Total** | **~7 weeks** | **3 engineers** | **~2.5 weeks calendar with 3 engineers in parallel** |

**Parallelization**: VS Code + JetBrains + Debugging can be done by one engineer. Tool Pack System + Initial Packs by another. MCP Packs + Git Push Approval + Dotfiles by a third. Documentation is distributed across all three.

**Research effort** (pre-implementation): 1-2 engineer-weeks for the five research items above. This can overlap with late Phase 3 work.
