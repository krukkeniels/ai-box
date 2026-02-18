# AI-Box Developer Experience & Workflow Review

**Reviewer**: Developer Experience (DevEx) Platform Engineer
**Date**: 2026-02-18
**Scope**: Developer workflow mapping, IDE integration, Windows/Linux support, transition strategy, tool pack design, performance, AI tool integration, and platform recommendation for ~200 developers.

---

## Executive Summary

The AI-Box spec is architecturally sound for its security goals, but it significantly underspecifies the developer experience. Security without usability leads to shadow IT -- developers will bypass the sandbox if it adds more than ~30 seconds of friction to their core loop. This review maps every friction point and provides concrete recommendations to make AI-Box something developers *want* to use, not something they're forced to use.

**Top-level verdict**: The spec needs a dedicated "Developer Experience" section that is treated as a first-class requirement alongside security. I recommend **Coder (self-hosted)** as the platform, **Dev Containers** as the workspace definition format, and a **phased rollout** starting with a 10-person pilot.

---

## 1. Developer Workflow Mapping

### 1.1 Current State (Local Development)

A typical developer's day today:

```
Morning:  git pull -> open IDE -> code
Coding:   edit -> save -> hot-reload (instant feedback)
Build:    local build tool (Gradle/npm/Bazel) -> fast incremental builds
Test:     run tests locally -> instant results
AI:       Claude Code / Codex CLI in terminal -> direct API access
Review:   git push -> open PR -> CI runs
```

Total friction from "open laptop" to "writing code": **< 2 minutes**.

### 1.2 Proposed AI-Box State (Without Optimization)

```
Morning:  start sandbox -> wait for container -> attach IDE -> wait for sync -> code
Coding:   edit in remote IDE -> network round-trip -> save -> hot-reload (delayed)
Build:    build inside container -> possible volume mount penalty -> slower
Test:     tests inside container -> same penalty
AI:       Claude Code in sandbox terminal -> traffic routed through egress proxy -> latency
Review:   git push from sandbox -> review-required gate -> approval -> push completes
```

Total friction from "open laptop" to "writing code": **5-10 minutes** if not optimized.

### 1.3 Friction Point Analysis

| Friction Point | Severity | Mitigation |
|---|---|---|
| Container startup time | HIGH | Pre-built images, workspace snapshots, keep-alive with auto-sleep |
| IDE connection latency | HIGH | Use SSH-based remoting, not volume mounts from host |
| File sync lag | CRITICAL on Windows | Store code inside container (ext4), not on host NTFS |
| Build performance | MEDIUM | Persistent build caches, tmpfs for temp dirs |
| AI tool proxy latency | MEDIUM | Low-latency proxy sidecar, connection pooling |
| `git push` approval gate | LOW-MEDIUM | Make approval async, not blocking (comment-based) |
| "Where are my dotfiles?" | MEDIUM | Dotfile sync mechanism (chezmoi, yadm, or Coder built-in) |
| "I can't install X" | HIGH | Self-service tool packs with fast approval process |

### 1.4 Critical Spec Gap: No Startup Time Target

**The spec must define a startup SLA.** Recommendation:

- Cold start (new workspace): < 90 seconds
- Warm start (existing workspace, stopped): < 15 seconds
- Reconnect (workspace running, IDE disconnected): < 5 seconds

Without these targets, implementations will drift toward "it works eventually" and developers will revolt.

---

## 2. IDE Integration Deep-Dive

### 2.1 VS Code

**Recommended approach: Remote - SSH** (not Remote Containers, not Dev Containers extension from host)

| Approach | Pros | Cons | Verdict |
|---|---|---|---|
| Remote - SSH | Best performance, full extension support, SSH is the universal protocol | Requires SSH server in container | **Recommended** |
| Remote - Containers | Docker socket required on host, tight Docker coupling | Requires Docker socket exposure (security risk) | Not recommended |
| Dev Containers (host) | Good DX, devcontainer.json standard | Requires Docker on host, defeats sandboxing purpose | Not recommended |
| code-server (browser) | Zero local install, works from any device | No native VS Code extensions, no local keybindings | Fallback option |

**How it works with AI-Box**:
1. Coder/DevPod starts the container with an SSH server (or uses its own tunnel)
2. Developer opens VS Code locally, connects via Remote - SSH
3. VS Code Server runs **inside** the container -- extensions, terminals, language servers all run inside the sandbox
4. Files are on the container's local filesystem (ext4), no cross-OS sync penalty

**Extension considerations**:
- Extensions run inside the container automatically with Remote - SSH
- Language servers (TypeScript, Java LSP, etc.) run inside the container -- this is actually *better* than local because the environment matches the build environment exactly
- The Copilot extension needs network access to GitHub/API -- must be in the egress allowlist
- Custom extensions that phone home need to be audited and allowlisted

### 2.2 JetBrains (IntelliJ, WebStorm, etc.)

**Recommended approach: JetBrains Gateway with SSH**

| Approach | Pros | Cons | Verdict |
|---|---|---|---|
| Gateway + SSH | Official remote dev, full IDE features | Requires ~4GB RAM on remote for backend | **Recommended** |
| Gateway + Coder plugin | Seamless workspace selection, auto-connect | Coder-specific dependency | **Recommended (if using Coder)** |
| Thin Client (Projector) | Browser-based, zero install | Deprecated in favor of Gateway, laggy | Not recommended |
| Local IDE + remote build | Familiar workflow | Requires file sync, defeats sandboxing | Not recommended |

**How it works with AI-Box**:
1. Developer opens JetBrains Gateway locally
2. Selects workspace from Coder dashboard (or connects via SSH)
3. Gateway downloads the JetBrains backend IDE into the container
4. Frontend (thin client) runs locally, backend runs in container
5. All indexing, code analysis, debugging happens inside the sandbox

**Critical JetBrains consideration**: The JetBrains backend requires **significant RAM** (2-4GB for IntelliJ with a large Java project). The spec must account for this in container resource allocation. A container running IntelliJ backend + build tools + AI agent needs at minimum **8GB RAM, 4 CPU cores**.

### 2.3 Language Server Performance

Language servers running inside containers perform **the same or better** than local, provided:
- The code is on the container's native filesystem (not a mounted volume)
- Sufficient RAM is allocated (LSP can be memory-hungry)
- File watching works correctly (inotify inside container works natively)

**Spec gap**: The spec mentions `/workspace` as writable but does not specify the filesystem type. **Recommendation**: mandate ext4 or overlayfs for `/workspace`, never NFS or FUSE-mounted host paths for primary development.

### 2.4 Debugging Experience

| Debugger Scenario | How It Works in AI-Box |
|---|---|
| VS Code + Node.js debug | Debug adapter runs inside container, VS Code connects over SSH tunnel. Works natively. |
| IntelliJ + Java debug | JDWP runs inside container, Gateway frontend connects to backend. Works natively. |
| Browser DevTools (frontend dev) | Port forwarding from container to host. Coder/DevPod handle this automatically. |
| Attach to running process | Works because debugger and process are in the same container. No special config needed. |
| Hot reload (React, Spring Boot, etc.) | File watcher inside container detects changes. No cross-OS latency. Works well. |

**Verdict**: Debugging is actually a non-issue with SSH-based remote development. The key is that *everything runs inside the container*.

### 2.5 Terminal & Shell Experience

- Developers get a shell inside the container via IDE terminal or `ssh`/`coder ssh`
- Shell customization: support dotfiles sync (`.bashrc`, `.zshrc`, starship prompt, etc.)
- Tmux/screen should be available in the base image for session persistence
- **Spec gap**: No mention of shell customization. Developers are very attached to their shell configs. Add a `dotfiles` mechanism to the spec.

### 2.6 Git Integration

- VS Code Git pane and IntelliJ VCS work natively because Git runs inside the container
- SSH keys for Git: injected via credential broker (spec section 6.3) -- this is correct
- GPG signing: needs a mechanism to forward GPG agent or inject signing keys
- **The `git push` review-required gate (spec section 6.4) needs UX design**: How does the approval prompt surface? In the IDE? In a browser? Via CLI? This must be non-blocking for the developer -- e.g., push goes to a staging ref and a webhook notifies the developer when approved.

---

## 3. Windows 11 + Linux Host Requirements

### 3.1 Container Runtime Options

| Runtime | License Cost (200 devs) | Performance | Ease of Setup | Verdict |
|---|---|---|---|---|
| Docker Desktop | ~$19/user/month Business = **$45,600/year** | Good | Easy | Expensive, but most compatible |
| Rancher Desktop | Free (Apache 2.0) | Good | Medium | **Recommended for cost-conscious orgs** |
| Podman Desktop | Free (Apache 2.0) | Good | Medium-Hard | Rootless by default (security win), but less ecosystem compat |
| WSL2 + Docker CE | Free | Best | Hard (manual setup) | Best performance, worst onboarding |

**Recommendation**: **Rancher Desktop** as the default for Windows developers. It is free, uses containerd or dockerd under the hood, provides a Docker CLI-compatible interface, and avoids the Docker Desktop licensing fee. For organizations that can absorb the cost, Docker Desktop remains the easiest path.

**If using Coder (recommended platform)**: Developers on Windows don't need a local container runtime at all if workspaces run on a central Kubernetes cluster. They only need the Coder CLI and an IDE. This completely eliminates the Docker Desktop licensing question for the developer workstation.

### 3.2 WSL2 Considerations

If workspaces run locally (not on a central cluster):

- **WSL2 is mandatory** for acceptable container performance on Windows
- Default WSL2 memory limit is 50% of host RAM or 8GB (whichever is less) -- this is often too low
- **Must configure `.wslconfig`**:
  ```ini
  [wsl2]
  memory=16GB
  processors=8
  swap=4GB
  ```
- WSL2 networking: uses a NAT by default, which can complicate proxy configuration. Consider `networkingMode=mirrored` (available in recent Windows 11 builds)

### 3.3 File System Performance (CRITICAL)

This is the single biggest performance trap on Windows:

| Scenario | Relative Performance | Notes |
|---|---|---|
| Code on ext4 inside WSL2/container | **1x (baseline)** | Native Linux I/O |
| Code on NTFS, mounted into WSL2 | **3-10x slower** | 9P protocol overhead |
| Code on NTFS, mounted into Docker | **5-20x slower** | Layered translation |

**Hard rule**: Source code MUST live inside the container's native filesystem, never on the Windows NTFS host. The `aibox mount`/`aibox sync` commands in the spec (section 7.2) must default to cloning the repo *inside* the container, not mounting from the host.

**Spec gap**: Section 7.2 mentions `aibox mount` and `aibox sync` but does not specify the direction or mechanism. This must be clarified:
- `aibox sync` should do a `git clone` inside the container, not a bind mount
- Two-way file sync (like mutagen) should be available but not the default
- Document clearly: "Your code lives inside the sandbox. This is by design."

### 3.4 Linux Host Setup

Linux developers have a simpler path:
- Docker CE or Podman installed natively (no WSL2 needed)
- Containers run at native speed
- File system performance is not an issue (code inside container on ext4 either way)
- Same IDE remote development workflow (VS Code Remote SSH, JetBrains Gateway)

**The spec should not assume all developers are on the same OS.** Provide OS-specific quickstart guides.

---

## 4. Transition Strategy for 200 Developers

### 4.1 Phased Rollout Plan

```
Phase 0: Foundation       (Weeks 1-4)    - 3-5 platform engineers
Phase 1: Pilot            (Weeks 5-8)    - 10 volunteer developers
Phase 2: Early Adopters   (Weeks 9-14)   - 30-40 developers
Phase 3: General Rollout  (Weeks 15-22)  - Remaining developers
Phase 4: Mandatory        (Week 23+)     - Local dev deprecated
```

#### Phase 0: Foundation (3-5 platform engineers)
- Stand up Coder instance on Kubernetes
- Build base image + top 3 tool packs (Java, Node, Python)
- Configure egress proxy with LLM endpoint allowlist
- Create workspace templates for 2-3 major project types
- Build monitoring dashboards (startup time, resource usage)
- Write quickstart documentation
- **Exit criteria**: Platform team can go from zero to coding in < 90 seconds

#### Phase 1: Pilot (10 volunteer developers)
- Select developers across teams, OS types, and IDE preferences
- Pair each pilot dev with a platform engineer for their first day
- Daily feedback surveys for first 2 weeks, then weekly
- Fix the top 5 pain points before moving to Phase 2
- **Exit criteria**: 8/10 pilot devs rate experience as "acceptable or better"

#### Phase 2: Early Adopters (30-40 developers)
- Open to volunteers + nominated team leads
- Launch self-service onboarding (guided setup wizard)
- Establish Champions program (see 4.3)
- Begin building additional tool packs based on demand
- Run AI-Box and local dev in parallel -- no mandate yet
- **Exit criteria**: < 3 support tickets per week, startup time < 90s at p95

#### Phase 3: General Rollout (remaining developers)
- All new projects start in AI-Box by default
- Existing projects migrate on a team-by-team schedule
- Dedicated "migration office hours" 3x per week
- **Exit criteria**: > 90% of active developers using AI-Box

#### Phase 4: Mandatory
- Local development access deprecated (not removed, just unsupported)
- Remaining holdouts get 1:1 migration support
- Policy enforcement activated for all repos

### 4.2 Training Materials Needed

| Material | Format | Audience |
|---|---|---|
| "AI-Box in 5 minutes" video | Screen recording | All developers |
| Quickstart guide (VS Code) | Written + screenshots | VS Code users |
| Quickstart guide (IntelliJ) | Written + screenshots | JetBrains users |
| "Troubleshooting AI-Box" FAQ | Wiki/Confluence | All developers |
| "Building Tool Packs" guide | Written + examples | Power users / team leads |
| "AI-Box for Team Leads" | Slide deck | Engineering managers |
| Architecture overview | Diagram + written | Curious developers |

### 4.3 Champions / Ambassador Program

- Recruit 1 champion per team (15-20 champions total)
- Champions get:
  - Early access to new features
  - Direct Slack channel with platform team
  - Monthly "Champions Sync" meeting
  - Recognition (internal blog post, etc.)
- Champions are responsible for:
  - Being the first point of contact for their team
  - Surfacing pain points to the platform team
  - Testing new tool packs before general release

### 4.4 Support Model

```
Tier 0: Self-service (docs, FAQ, quickstart)         - 70% of issues
Tier 1: Champions (team-level troubleshooting)        - 20% of issues
Tier 2: Platform team (Slack channel + ticket queue)  - 9% of issues
Tier 3: Escalation (infrastructure/security team)     - 1% of issues
```

Dedicated Slack channel: `#ai-box-help` with platform team rotation during business hours.

### 4.5 Fallback Plan

**Critical**: Developers MUST be able to fall back to local development during the transition period (Phases 1-3).

- Keep local dev tooling functional and documented
- AI-Box should be additive, not a gate, during early phases
- If a developer hits a blocking issue in AI-Box, they should be able to `git clone` locally and continue working within 5 minutes
- Track fallback frequency as a metric -- it tells you what's broken

### 4.6 Day-1 MVP

The minimum that makes a developer productive on day 1:

1. Working IDE connection (VS Code Remote SSH or JetBrains Gateway)
2. Git clone of their primary repo inside the sandbox
3. Build and test working for their project type
4. Claude Code / Codex CLI functional with API access through proxy
5. `git push` working (even if gated by review-required policy)
6. Their shell with basic dotfiles

Everything else (tool pack customization, MCP servers, advanced policies) can come later.

---

## 5. Tool Pack Design

### 5.1 Implementation Approach

**Recommended: Docker multi-stage layers + Nix for complex dependencies**

| Approach | Pros | Cons | Verdict |
|---|---|---|---|
| Docker layers | Simple, well-understood, good caching | Layer bloat, hard to compose at runtime | Base tool packs |
| Nix | Reproducible, composable, per-user profiles | Steep learning curve, large store | Complex/versioned tools |
| apt/dnf at runtime | Simple, familiar | Non-reproducible, slow, network-dependent | Not recommended |
| Homebrew/Linuxbrew | Familiar to macOS devs | Slow, not reproducible | Not recommended |

**Recommended hybrid approach**:
- **Base image** includes: Git, SSH, shell tools, tmux, common CLI utilities
- **Language tool packs** are Docker layers composed at image build time: `java@21`, `node@20`, `python@3.12`
- **Complex/niche tool packs** use Nix for precise version control: `bazel@7`, `scala@3.4`
- **Runtime extensions** via a lightweight package manager for quick additions that don't need reproducibility

### 5.2 How Developers Request New Tools

```
Request Flow:
1. Developer opens issue in ai-box-toolpacks repo (template provided)
2. Issue template collects: tool name, version, use case, urgency
3. Auto-label based on tool category (language, build, security, etc.)
4. If tool exists in registry: auto-approve, fast-track (< 1 business day)
5. If new tool: review by platform team + security review
6. Approval -> PR with tool pack definition -> CI builds + tests -> merge -> available
```

**SLA**:
- Known/registered tools: available within 1 business day
- New tools requiring security review: 3-5 business days
- Emergency requests: same-day with platform team approval

### 5.3 Governance Model

| Role | Responsibility |
|---|---|
| Platform Team | Maintains base image, core tool packs, registry |
| Security Team | Reviews new tool packs for supply chain risk, network requirements |
| Team Leads | Can approve team-specific tool packs within policy |
| Champions | Can submit and test tool pack PRs |

### 5.4 Tool Pack Manifest (Recommended Format)

The spec shows `toolpacks/<name>/manifest.yaml` -- here's a concrete schema:

```yaml
# toolpacks/java/manifest.yaml
name: java
version: "21.0.2"
description: "OpenJDK 21 + Gradle + Maven"
maintainer: "platform-team"

install:
  method: "docker-layer"        # or "nix", "script"
  base_image: "eclipse-temurin:21-jdk"
  packages:
    - gradle:8.5
    - maven:3.9.6

network:
  requires:
    - id: "maven-central"
      hosts: ["repo1.maven.org"]
      ports: [443]
    - id: "gradle-plugins"
      hosts: ["plugins.gradle.org"]
      ports: [443]

filesystem:
  creates:
    - "/opt/java"
    - "/opt/gradle"
    - "/opt/maven"
  caches:
    - "$HOME/.gradle/caches"
    - "$HOME/.m2/repository"

resources:
  min_memory: "2GB"
  recommended_memory: "4GB"

tags: ["language", "jvm"]
```

### 5.5 Version Management

- Tool packs are version-pinned in the manifest
- Projects can pin tool pack versions in their `aibox/policy.yaml`
- Platform team maintains an LTS channel (stable, tested combinations) and a Latest channel
- Breaking version upgrades require opt-in per project

### 5.6 Custom Tool Packs by Teams

Teams should be able to create their own tool packs:
- Fork the tool pack template
- Define their manifest + install script
- Submit PR to the tool pack registry
- After security review, it becomes available to their team (or org-wide)
- Custom tool packs are namespaced: `team-payments/kafka-tools`

---

## 6. Performance Considerations

### 6.1 Build Times: Container vs Local

| Scenario | Local | Container (optimized) | Container (naive) |
|---|---|---|---|
| Gradle full build (Java, 500 modules) | 4 min | 4-5 min | 8-12 min |
| npm install + build (large React app) | 2 min | 2-3 min | 5-8 min |
| Bazel build (incremental) | 30s | 30-45s | 2-5 min |

**Key optimizations that close the gap**:
1. **Persistent build caches**: Mount build cache directories as persistent volumes (`~/.gradle/caches`, `~/.m2`, `node_modules/.cache`)
2. **tmpfs for temp directories**: Mount `/tmp` as tmpfs for fast temporary I/O
3. **Code on native filesystem**: Never build from a mounted host volume
4. **Pre-warmed images**: Pre-pull and cache base images + dependencies
5. **Resource allocation**: Allocate adequate CPU/RAM (see 6.2)

### 6.2 Resource Requirements Per Developer

| Workload Profile | CPU | RAM | Disk | Notes |
|---|---|---|---|---|
| Frontend (React/Angular) | 2 cores | 4-6 GB | 20 GB | Lower resource needs |
| Backend (Java/Kotlin + IntelliJ) | 4 cores | 8-12 GB | 40 GB | JetBrains backend is hungry |
| Full-stack + AI agent | 4-6 cores | 10-16 GB | 50 GB | AI agent + build + IDE |
| Monorepo (Bazel/Nx) | 6-8 cores | 12-16 GB | 60 GB | Large index, many targets |

**Spec gap**: The spec does not mention resource allocation at all. This is critical for capacity planning. Add a `resources` section to the workspace template specification.

### 6.3 Disk I/O for Large Repos

- Monorepos with 100K+ files: initial clone takes 2-5 minutes
- Solution: use `git clone --filter=blob:none` (partial clone) + `git sparse-checkout`
- Persistent workspaces avoid re-cloning on every start
- Build cache volumes should use SSD-backed persistent storage, not network-attached

### 6.4 Network Latency

- Egress proxy adds 1-5ms per request (negligible for API calls)
- LLM API calls (Claude, OpenAI): proxy latency is irrelevant vs. inference time
- Artifact download (Maven, npm): proxy may add latency if doing TLS inspection; prefer allowlist-only without inspection
- IDE remote protocol (SSH): needs < 50ms RTT for acceptable experience -- this means workspaces should run on infrastructure *close to* developers (same region/datacenter)

### 6.5 Developer Machine Specs

**Minimum for remote workspace model (Coder/central cluster)**:
- Any modern laptop with 8GB+ RAM, decent network
- IDE runs locally, compute is remote
- This is a significant advantage: developer hardware requirements drop dramatically

**Minimum for local workspace model (Docker on developer machine)**:
- Windows: 16GB RAM minimum (32GB recommended), SSD, 8+ CPU cores
- Linux: 16GB RAM minimum, SSD, 8+ CPU cores
- WSL2 needs at least 12GB allocated

**Recommendation**: The remote workspace model (Coder on Kubernetes) is strongly preferred because it decouples developer machine specs from workspace requirements.

---

## 7. AI Tool Integration Specifics

### 7.1 Claude Code Inside the Sandbox

Claude Code is a CLI tool that runs in the terminal. Inside AI-Box:

```
Developer -> IDE Terminal (inside container) -> claude (CLI)
                                                   |
                                                   v
                                            Egress Proxy
                                                   |
                                                   v
                                          api.anthropic.com
                                          (or internal gateway)
```

**Setup requirements**:
1. `claude` binary installed in the base image or via a tool pack
2. API key injected via environment variable (`ANTHROPIC_API_KEY`) using the credential broker (spec 6.3)
3. Egress allowlist entry for the Anthropic API endpoint (or internal LLM gateway like Foundry)
4. Claude Code's filesystem access is naturally sandboxed -- it can only see `/workspace`
5. Claude Code's tool execution (bash, file edits) is confined to the container

**Coder AgentAPI integration**: Coder has built an `agentapi` that wraps Claude Code (and other AI agents) with an HTTP API, enabling:
- Web-based chat UI for AI agents running in workspaces
- Status monitoring (is the agent idle or working?)
- Multi-agent orchestration
- This is a strong signal that Coder is investing in AI-in-sandbox workflows

```bash
# Inside a Coder workspace, launching Claude Code via AgentAPI
agentapi server -- claude
```

### 7.2 Codex CLI

Same pattern as Claude Code:
1. `codex` binary in base image or tool pack
2. API key via credential broker (`OPENAI_API_KEY`)
3. Egress allowlist for OpenAI API endpoint
4. Runs in terminal, filesystem access confined to container

```bash
# Launch Codex via AgentAPI
agentapi server --type=codex -- codex
```

### 7.3 MCP Server Configuration

MCP servers inside the sandbox need careful configuration:

| MCP Server | Runs Where | Network Needs | Notes |
|---|---|---|---|
| filesystem-mcp | Inside container | None (local) | Naturally sandboxed to `/workspace` |
| git-mcp | Inside container | Git remote (allowlisted) | Needs SSH key or token |
| jira-mcp | Inside container | Jira endpoint (allowlisted) | Needs API token |
| docs-mcp | Inside container | Docs endpoint (allowlisted) | Read-only access |
| browser-mcp (Playwright) | Inside container | Allowlisted URLs only | Needs headless browser in image |

**MCP Pack manifest should declare network requirements** (the spec already suggests this in section 5, "Each MCP pack declares required permissions and endpoints" -- good).

**Recommended MCP configuration flow**:
```bash
# Enable MCP packs for a workspace
aibox mcp enable filesystem-mcp git-mcp jira-mcp

# This updates the MCP configuration inside the container
# Claude Code / other agents can then discover and use these MCP servers
```

**Spec gap**: The spec does not describe how MCP servers are *discovered* by AI agents inside the container. Recommendation: AI-Box should write a standard MCP configuration file (e.g., `~/.config/claude/claude_desktop_config.json` for Claude Code, or a generic `mcp.json`) that agents can read to discover available MCP servers.

### 7.4 API Key / Token Management

**This is one of the most sensitive UX decisions in the spec.**

Options:

| Approach | Security | UX | Recommendation |
|---|---|---|---|
| Developer pastes API key into env var | Low (key persists in shell history) | Easy | Not recommended |
| Credential broker injects short-lived token | High | Transparent | **Recommended** |
| Vault/secrets manager integration | High | Medium (requires auth) | Good for enterprise |
| OAuth device flow for LLM gateway | High | Good (browser-based auth) | **Best for internal gateway** |

**Recommended flow for internal LLM gateway (Foundry/similar)**:
1. Developer starts workspace
2. Workspace init script calls credential broker
3. Broker authenticates developer (SSO/Kerberos/certificate)
4. Broker mints a scoped, short-lived token for the LLM gateway
5. Token is injected as environment variable
6. Claude Code / Codex CLI use the token transparently
7. Token expires after session ends (or configurable TTL)

**Spec improvement**: Section 6.3 mentions "optional credential broker" -- this should be **required** for LLM API access, not optional. Manual key management at scale (200 devs) is a security incident waiting to happen.

### 7.5 Can AI Tools Read the Full Repo?

Yes, and this is a key advantage of the sandbox model:
- The entire repo is cloned inside `/workspace`
- Claude Code / Codex can read all files (within the filesystem policy)
- Context window limits are an AI model constraint, not a sandbox constraint
- For monorepos, partial clone + sparse checkout can limit what's visible (also a security benefit -- principle of least privilege for code access)

---

## 8. Platform Recommendation

### 8.1 Evaluation Matrix

| Criterion | Coder (OSS) | DevPod | Custom Docker Compose | Gitpod (Self-Hosted) |
|---|---|---|---|---|
| Self-hosted | Yes (Kubernetes) | Yes (client-side) | Yes | Yes (Kubernetes) |
| IDE support (VS Code) | Excellent | Excellent | Manual | Good |
| IDE support (JetBrains) | Excellent (Gateway plugin) | Good | Manual | Limited |
| AI agent integration | **Best** (AgentAPI, built-in) | None | Manual | None |
| Workspace templates | Terraform-based, flexible | devcontainer.json | docker-compose.yml | .gitpod.yml |
| Network policy control | Via Kubernetes NetworkPolicy | Limited (client-side) | Manual (iptables) | Via Kubernetes |
| Multi-user management | Built-in (RBAC, audit logs) | None (client-side) | None | Built-in |
| Cost management | Auto-stop, quotas | Local resources | Manual | Auto-stop |
| Maturity | High (enterprise users) | Medium (growing) | Low (bespoke) | Medium (OSS declining) |
| Windows client support | CLI + browser + IDE | Desktop app | Docker required | Browser only |
| License | AGPL v3 (OSS) + Enterprise | MPL 2.0 | N/A | AGPL + Enterprise |

### 8.2 Recommendation: Coder (Self-Hosted on Kubernetes)

**Coder is the clear winner for this use case.** Here's why:

1. **AI agent integration is a first-class concern**: Coder's AgentAPI directly supports Claude Code, Codex, Aider, and other agents. No other platform has this.

2. **Central infrastructure means simpler security**: Workspaces run on a Kubernetes cluster that the platform team controls. Network policies, egress rules, and resource quotas are enforced server-side, not on developer machines. This aligns perfectly with AI-Box's security model.

3. **No Docker Desktop on developer machines**: Developers only need the Coder CLI and their IDE. This eliminates the Docker Desktop licensing cost ($45K+/year for 200 devs) and removes a major Windows pain point.

4. **JetBrains Gateway integration is mature**: Coder has a dedicated JetBrains Gateway plugin that makes connecting IntelliJ to a workspace seamless.

5. **Workspace templates via Terraform**: More flexible than devcontainer.json for complex setups (custom networking, sidecars for egress proxy, resource constraints).

6. **Enterprise features**: RBAC, audit logging, workspace quotas, auto-stop/auto-delete, OIDC/SAML SSO. All needed for 200-dev deployment.

7. **Open source core**: AGPL v3 for the core, with an enterprise tier for premium features. The OSS version is sufficient for a pilot.

### 8.3 Architecture with Coder

```
Developer Workstation                    Kubernetes Cluster
+------------------------+              +----------------------------------+
| VS Code / IntelliJ    |   SSH/WSS    | Coder Control Plane              |
| (local IDE frontend)  |<------------>|   - Workspace provisioner        |
|                        |              |   - Template engine              |
| Coder CLI              |              |   - RBAC / Auth (SSO)           |
| (coder ssh, coder open)|              |   - Audit logging               |
+------------------------+              +----------------------------------+
                                        |                                  |
                                        v                                  v
                                 +------------------+    +------------------+
                                 | Workspace Pod    |    | Workspace Pod    |
                                 | (Developer A)    |    | (Developer B)    |
                                 |                  |    |                  |
                                 | - IDE backend    |    | - IDE backend    |
                                 | - Claude Code    |    | - Codex CLI      |
                                 | - Build tools    |    | - Build tools    |
                                 | - MCP servers    |    | - MCP servers    |
                                 | - /workspace     |    | - /workspace     |
                                 +--------+---------+    +--------+---------+
                                          |                       |
                                          v                       v
                                 +------------------------------------------+
                                 | Egress Proxy (sidecar or cluster-level)  |
                                 | - Allowlist: LLM API, artifact repos     |
                                 | - Deny: everything else                  |
                                 | - Logging: all decisions                 |
                                 +------------------------------------------+
```

### 8.4 Why Not the Alternatives?

**DevPod**: Excellent for individual developers or small teams, but client-side architecture means:
- No centralized policy enforcement (each developer's machine is the trust boundary)
- No centralized audit logging
- Network egress control depends on developer's local Docker/Podman config
- Harder to enforce consistent environments at scale
- Would work well as a "local fallback" option during transition

**Custom Docker Compose**: Too much maintenance burden for a 200-dev org. You'd end up rebuilding Coder's features (SSH tunneling, IDE integration, auto-stop, RBAC, audit) from scratch.

**Gitpod (Self-Hosted)**: The open-source self-hosted version has seen declining investment. JetBrains support is less mature than Coder's. No AI agent integration story.

---

## 9. Spec Change Recommendations (Summary)

### Must-Have Additions

| # | Recommendation | Spec Section |
|---|---|---|
| 1 | Add a "Developer Experience" section as a first-class requirement | New section |
| 2 | Define workspace startup SLA (cold < 90s, warm < 15s, reconnect < 5s) | Section 7.1 |
| 3 | Specify that code lives on native container filesystem, not host mounts | Section 6.2 |
| 4 | Make credential broker required (not optional) for LLM API access | Section 6.3 |
| 5 | Add resource allocation requirements per workspace profile | New section |
| 6 | Define MCP server discovery mechanism for AI agents | Section 5 |
| 7 | Add dotfiles/shell customization support | Section 7.2 |
| 8 | Specify IDE integration approach (SSH-based remote development) | Section 6 / 7.1 |
| 9 | Add tool pack manifest schema with network/resource declarations | Section 7.3 |
| 10 | Define `git push` approval UX (non-blocking) | Section 6.4 |

### Should-Have Additions

| # | Recommendation | Spec Section |
|---|---|---|
| 11 | Add Windows/Linux host-specific guidance | New section |
| 12 | Add transition/rollout strategy as an appendix | Appendix |
| 13 | Define support model and SLAs for the platform | Appendix |
| 14 | Add monitoring/observability requirements (workspace health, startup times) | New section |
| 15 | Define workspace persistence model (ephemeral vs persistent, auto-delete policy) | Section 4.1 |

---

## 10. Risk Register

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Developers bypass sandbox for productivity | HIGH | HIGH | Make sandbox faster/easier than local dev. Invest in UX. |
| JetBrains Gateway performance issues with large projects | MEDIUM | HIGH | Pre-allocate adequate resources; test with largest project during pilot |
| Egress proxy becomes a bottleneck | LOW | HIGH | Deploy proxy as sidecar per workspace, not as shared service |
| Tool pack sprawl (too many packs, hard to maintain) | MEDIUM | MEDIUM | Governance model, ownership requirements, automated testing |
| Windows developers have worse experience than Linux | HIGH | MEDIUM | Central Kubernetes model eliminates most OS differences |
| Credential broker becomes single point of failure | MEDIUM | HIGH | HA deployment, graceful degradation, cached tokens |
| Build cache corruption causes hard-to-debug failures | MEDIUM | MEDIUM | Per-workspace caches with easy "nuke cache" command |

---

## 11. Final Verdict

The AI-Box spec is a **strong security architecture** that is **underspecified on developer experience**. The security controls are well-designed and necessary. But without equal investment in the developer workflow, these controls will be circumvented.

**The core insight**: The sandbox should feel like a *better* development environment, not a restricted one. With the right platform (Coder), proper resource allocation, and native-filesystem code storage, it is entirely possible to make the sandboxed experience as good as -- or better than -- local development. The AI tool integration actually becomes *easier* inside the sandbox because the credential broker handles API keys, the egress proxy handles routing, and the policy engine handles safety, all transparently to the developer.

**If developers have to think about the sandbox, you've already lost.** The goal is invisibility.
