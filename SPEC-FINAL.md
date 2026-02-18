# AI-Box: Final Specification

*A secure, tool-agnostic development sandbox enabling agentic AI software engineering at scale.*

**Version**: 1.0
**Date**: 2026-02-18
**Status**: Final Draft
**Audience**: Platform engineering, security, engineering leadership

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Problem Statement](#2-problem-statement)
3. [Goals and Non-Goals](#3-goals-and-non-goals)
4. [Design Principles](#4-design-principles)
5. [Architecture Overview](#5-architecture-overview)
6. [Deployment Model](#6-deployment-model)
7. [Container Runtime](#7-container-runtime)
8. [Network Security](#8-network-security)
9. [Container Isolation](#9-container-isolation)
10. [Filesystem Controls](#10-filesystem-controls)
11. [Credential Management](#11-credential-management)
12. [Policy Engine](#12-policy-engine)
13. [IDE Integration](#13-ide-integration)
14. [AI Tool Integration](#14-ai-tool-integration)
15. [Tool Packs](#15-tool-packs)
16. [MCP Packs](#16-mcp-packs)
17. [Image Strategy](#17-image-strategy)
18. [Developer Experience Requirements](#18-developer-experience-requirements)
19. [Audit and Compliance](#19-audit-and-compliance)
20. [Threat Model](#20-threat-model)
21. [Transition and Rollout Plan](#21-transition-and-rollout-plan)
22. [Operations](#22-operations)
23. [Tech Stack Summary](#23-tech-stack-summary)
24. [Residual Risks](#24-residual-risks)
25. [Appendices](#25-appendices)

---

## 1. Executive Summary

AI-Box enables ~200 developers to use agentic AI tools (Claude Code, Codex CLI, and future agents) for software engineering while preventing code leakage from classified/sensitive environments.

**The core insight**: AI tools need to see source code to function. AI-Box contains this by running agents inside a policy-enforced sandbox with default-deny networking, so code never reaches an unauthorized destination -- even if the AI agent is compromised via prompt injection.

### Key Architectural Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Deployment model** | Local-first (containers on dev machines) with optional centralized (Coder on K8s) as Phase 2 | Lowest cost, best classified env fit, best performance, no centralized server dependency |
| **Container runtime** | Podman (rootless) | Free ($50K/year savings vs Docker Desktop), rootless-by-default, daemonless, OCI-compliant |
| **Container isolation** | gVisor (runsc) default, Kata Containers for highest classification | User-space kernel intercepts syscalls, dramatically reduces host kernel attack surface |
| **Egress proxy** | Squid (L7) + nftables (L3/L4) on host | Host-level enforcement, container cannot bypass. Squid for allowlists, nftables for enforcement |
| **DNS control** | CoreDNS (allowlist-only resolution) | Prevents DNS tunneling, only resolves approved domains |
| **Secret management** | HashiCorp Vault + SPIFFE/SPIRE | Short-lived tokens, cryptographic workload identity, no static credentials |
| **Policy engine** | OPA (Open Policy Agent) with Rego | Declarative policy-as-code, admission control, decision logging |
| **IDE integration** | SSH-based remote development | VS Code Remote SSH + JetBrains Gateway. Code lives in container, IDE UI on host |
| **Image registry** | Harbor (self-hosted) | Image signing/scanning, RBAC, replication for air-gapped sites |
| **Platform** (Phase 2) | Coder (self-hosted on K8s) | Best AI agent integration (AgentAPI), IDE plugins, centralized policy, eliminates Docker Desktop need |

### What Developers Experience

```
1. Run `aibox start` (or click IDE button)           ~15 seconds (warm start)
2. IDE connects automatically                         VS Code / IntelliJ, same as today
3. Code is on native ext4 inside sandbox              No file sync lag
4. AI tools (Claude Code, Codex) just work            API keys injected automatically
5. Build and test as normal                           Persistent caches, fast incremental builds
6. git push works (gated for review if policy says)   Non-blocking approval flow
```

The sandbox should feel like a *better* development environment, not a restricted one.

---

## 2. Problem Statement

Engineering teams want to use agentic AI tools to write and change software faster. In classified/sensitive environments, the risks block adoption:

- **Source code leakage**: Accidental or malicious exfiltration via network, logs, plugins, tools, or the LLM API channel itself.
- **Prompt injection & tool misuse**: Untrusted text causes the agent to run unsafe commands, fetch secrets, or modify sensitive files.
- **Inconsistent environments**: Each assistant/tool requires a different image, setup, and policy configuration.
- **Hard to extend**: Teams need Java/Scala/Node/Angular/Bazel/etc. without maintaining a zoo of bespoke containers.
- **Developer friction**: Security controls that slow developers down get bypassed. Shadow IT is the real risk.

AI-Box solves this by providing a **policy-enforced sandbox** that is **assistant/tool/MCP agnostic**, supports **pluggable tool packs**, prevents exfiltration by default, and is designed to be as fast (or faster) than local development.

---

## 3. Goals and Non-Goals

### Core Goals

1. **Prevent code leakage by design**
   - Default-deny network egress, enforced at the host level (not inside the container).
   - Allowlist-only outbound endpoints via authenticated proxy.
   - No implicit access to host filesystem, secrets, or credentials.

2. **Minimize prompt-injection blast radius**
   - gVisor runtime isolates syscalls in userspace.
   - Tool gating + least privilege for every tool and command.
   - Explicit approval for risky operations (configurable per policy).

3. **Assistant/tool/MCP agnostic**
   - Stable runtime and policy layer regardless of agent (Claude Code, Codex CLI, Continue, Aider, custom bots).
   - Standard interfaces: terminal, SSH, MCP.

4. **Easy extension**
   - Tool packs installed without rebuilding the base image.
   - Self-service requests with fast approval.

5. **Developer experience as a first-class requirement**
   - Startup SLA: cold < 90s, warm < 15s, reconnect < 5s.
   - IDE experience equivalent to local development.
   - Persistent build caches, dotfiles sync, shell customization.

### Non-Goals

- AI-Box is not an AI model provider (it consumes LLM APIs).
- AI-Box does not attempt to "prove" zero data exfiltration (it enforces strong controls and creates auditable evidence).
- AI-Box is not a full enterprise DLP replacement.
- AI-Box does not replace CI/CD (it complements it).

---

## 4. Design Principles

| Principle | Meaning |
|-----------|---------|
| **Secure by default** | Safest behavior on first run. No network, no credentials, no privilege. |
| **Policy over convention** | Enforcement is not optional. Devs can tighten policy, never loosen below org baseline. |
| **Least privilege everywhere** | Filesystem, network, credentials, tools -- minimal access for each. |
| **Host-level enforcement** | Security controls enforced outside the container where they cannot be bypassed. |
| **Invisible security** | Developers should not feel the sandbox. If they notice friction, we've failed. |
| **Reproducible and auditable** | Pinned versions, signed images, immutable logs, deterministic builds. |
| **Composable** | Base runtime + tool packs + MCP packs + policies. Mix and match. |
| **Graceful degradation** | If a central service is down, devs can keep working with cached resources. |

---

## 5. Architecture Overview

AI-Box is composed of six layers:

### 5.1 Runtime Sandbox
- Podman rootless container with gVisor (runsc) as the OCI runtime.
- Read-only base filesystem, writable `/workspace`.
- Persistent named volumes for build caches and user home.
- Configurable ephemeral mode (auto-wipe on exit, caches preserved).

### 5.2 Policy Engine
- OPA/Rego policies distributed via signed Git repository.
- Enforces network allowlists, filesystem boundaries, command allow/deny rules, tool permissions.
- Decision logs record every policy evaluation.
- Hierarchy: org baseline (immutable) -> team policy (can tighten) -> project policy (can tighten further).

### 5.3 Connectivity Layer
- Host-level Squid proxy for L7 domain allowlisting.
- Host-level nftables for L3/L4 enforcement (container traffic -> proxy only).
- CoreDNS for controlled DNS resolution (allowlisted domains only).
- LLM API sidecar proxy for credential injection and payload logging.

### 5.4 Tool Packs
- Installable bundles (e.g., `java@21`, `node@20`, `bazel@7`, `python@3.12`).
- Pre-built image variants for common stacks + runtime install for custom combinations.
- Versioned, pinned, checksummed, signed.

### 5.5 MCP Packs
- Optional MCP server bundles (e.g., `filesystem-mcp`, `git-mcp`, `jira-mcp`).
- Each MCP pack declares required permissions and endpoints.
- Auto-configured for discovery by AI agents inside the sandbox.

### 5.6 CLI (`aibox`)
- Single entry point for developers. Abstracts all infrastructure.
- Commands: `setup`, `start`, `stop`, `shell`, `status`, `update`, `install`, `doctor`, `policy validate`.
- Detects host OS (Windows/WSL2 vs Linux) and adapts automatically.

### Architecture Diagram

```
[DEVELOPER WORKSTATION]
 |
 |-- [Host OS: Windows 11 / Linux]
 |     |-- VS Code / IntelliJ (native IDE frontend)
 |     |-- aibox CLI
 |     |-- SSH tunnel to container (localhost:2222)
 |
 |-- [WSL2 VM or Native Linux]
       |
       |-- [Podman (rootless)]
       |     |-- [AI-Box Container (gVisor runtime)]
       |           |-- /workspace (bind mount or clone, ext4)
       |           |-- IDE backend (VS Code Server / JB backend)
       |           |-- AI agent (Claude Code / Codex CLI)
       |           |-- MCP servers
       |           |-- aibox-agent (policy enforcement)
       |           |-- LLM sidecar proxy (localhost:8443)
       |           |-- SSH server (:22)
       |           |-- Read-only rootfs, seccomp locked
       |           |-- No capabilities, no privilege escalation
       |
       |-- [Squid Proxy (:3128)] -- on host network
       |     |-- Domain allowlist enforcement
       |     |-- Access logging
       |
       |-- [CoreDNS (:53)] -- on host network
       |     |-- Allowlisted domain resolution only
       |     |-- NXDOMAIN for everything else
       |
       |-- [nftables rules] -- host kernel
             |-- Container -> proxy only (DROP all else)
             |-- Blocks direct egress, DNS tunneling, DoH

[CENTRAL INFRASTRUCTURE]
 |
 |-- [Harbor Registry]     harbor.internal:443
 |     Signed images, CVE scanning, replication
 |
 |-- [Nexus Repository]    nexus.internal:443
 |     Maven/npm/PyPI/NuGet mirrors
 |
 |-- [LLM Gateway]         foundry.internal:443
 |     Anthropic/OpenAI API proxy (or self-hosted models)
 |
 |-- [Git Server]          git.internal:443
 |     Source repos + AI-Box policy repo
 |
 |-- [Vault]               vault.internal:443
 |     Dynamic secrets, credential broker
```

---

## 6. Deployment Model

### Primary: Local-First

Every developer runs AI-Box containers on their own machine. The `aibox` CLI manages the lifecycle.

**Why local-first**:
- Best performance (no network latency for file I/O, builds, IDE).
- Lowest cost (no centralized compute infrastructure, minimal central servers).
- Best classified-environment fit (no dependency on centralized servers, works air-gapped).
- Compromise of one dev machine does not affect others.
- Works offline after initial image pull.

**Central infrastructure required**:

| Component | Sizing | Purpose |
|-----------|--------|---------|
| Harbor registry | 4 CPU, 16GB RAM, 2TB SSD | Signed image distribution |
| Nexus/Artifactory | 4 CPU, 16GB RAM, 1TB SSD | Package mirrors (likely already exists) |
| Vault | 2 CPU, 4GB RAM (HA pair) | Credential broker |
| Git server | Existing | Policy distribution, source repos |

**Total central infra**: 1-2 servers. Most organizations already have registry and artifact servers.

### Phase 2 Option: Centralized (Coder on Kubernetes)

For organizations that later need centralized audit, thin-client support, or >500 developers:

- Coder OSS or Enterprise on Kubernetes.
- Workspace pods with network policies, resource quotas.
- Developers connect via Coder CLI + IDE (no Docker needed on dev machines).
- Coder AgentAPI provides first-class AI agent integration.

**When to consider**:
- Security mandate for centralized session recording/audit.
- Thin-client deployment (locked-down desktops, Chromebooks).
- Organization exceeds 500 developers.
- Compliance requires centralized data-at-rest controls.

**Cost estimate (centralized, 200 devs)**: $715K-$1.2M year 1, $405K-$605K/year ongoing. This is why local-first is the default recommendation.

The `aibox` CLI abstraction means migrating from local to centralized does not change the developer-facing interface.

---

## 7. Container Runtime

### Primary: Podman 5.x (Rootless)

| Feature | Detail |
|---------|--------|
| License | Free (Apache 2.0) -- **saves ~$50K/year** vs Docker Desktop Business for 200 devs |
| Rootless | Default. Container processes map to unprivileged host UIDs. |
| Daemonless | Fork-exec model. No privileged daemon to attack. |
| OCI compliant | All Docker/OCI images work unmodified. |
| CLI compatible | `alias docker=podman` -- minimal retraining. |
| Windows support | `podman machine` creates Fedora CoreOS VM in WSL2/Hyper-V. Production-ready as of 2025. |
| Linux support | Native, first-class. |
| Build tool | Buildah (rootless, daemonless OCI image builds). |

### Docker Fallback

The `aibox` CLI detects available runtimes and prefers Podman. Teams with existing Docker Desktop licenses are supported as a fallback. Docker must be configured for rootless mode.

### OCI Runtime: gVisor (runsc)

gVisor is the OCI runtime used within Podman. It provides a user-space kernel (Sentry) that intercepts ~400 syscalls, dramatically reducing the host kernel attack surface.

```bash
# Container launch (abstracted by aibox CLI)
podman run \
  --runtime=runsc \
  --cap-drop=ALL \
  --security-opt=no-new-privileges:true \
  --security-opt=seccomp=/etc/aibox/seccomp.json \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=2g \
  -v workspace:/workspace:rw,nosuid,nodev \
  harbor.internal/aibox/java:21-24.04
```

For highest-classification workloads, Kata Containers (full VM isolation) is available as an alternative runtime.

### Windows 11 Setup

- WSL2 is required for container runtime.
- `aibox setup` automates: WSL2 configuration, Podman machine creation, `.wslconfig` optimization.

Recommended `.wslconfig`:
```ini
[wsl2]
memory=16GB
processors=8
swap=4GB
```

---

## 8. Network Security

Network controls are the primary exfiltration prevention mechanism. They are enforced at the **host level** -- the container cannot modify, disable, or bypass them.

### 8.1 Layered Architecture

| Layer | Technology | Purpose |
|-------|-----------|---------|
| L3/L4 | nftables on host | Block all container traffic except to proxy + DNS |
| L7 | Squid proxy on host | Domain allowlist enforcement, access logging |
| DNS | CoreDNS on host | Resolve only allowlisted domains, NXDOMAIN everything else |
| Application | LLM sidecar proxy in container | Inject API keys, log payloads, rate-limit |

### 8.2 nftables Rules (Host-Level)

```bash
# Set by `aibox setup` -- container cannot modify these
# Block all direct egress from container network
nft add rule inet filter forward iifname "podman*" drop
# Allow container -> proxy
nft add rule inet filter forward iifname "podman*" ip daddr <proxy_ip> tcp dport 3128 accept
# Allow container -> DNS
nft add rule inet filter forward iifname "podman*" ip daddr <dns_ip> udp dport 53 accept
# Block outbound DNS to anywhere else (prevents DNS tunneling)
nft add rule inet filter forward iifname "podman*" udp dport 53 drop
nft add rule inet filter forward iifname "podman*" tcp dport 53 drop
# Block DoT (DNS-over-TLS)
nft add rule inet filter forward iifname "podman*" tcp dport 853 drop
```

For rootless Podman with `pasta` networking, the `aibox` CLI configures equivalent restrictions via `pasta` options and host-level rules.

### 8.3 Squid Proxy Configuration

```
# /etc/aibox/squid.conf (simplified)
acl aibox_allowed dstdomain harbor.internal
acl aibox_allowed dstdomain nexus.internal
acl aibox_allowed dstdomain foundry.internal
acl aibox_allowed dstdomain git.internal

http_access allow aibox_allowed
http_access deny all

# Logging
access_log /var/log/aibox/proxy-access.log squid
```

The proxy supports HTTPS via `CONNECT` (SNI-based filtering). **No TLS interception by default** -- classified environments often prohibit it, and SNI-based filtering is sufficient for domain allowlisting.

### 8.4 DNS Control (CoreDNS)

```
# CoreDNS Corefile
.:53 {
    hosts {
        <harbor_ip> harbor.internal
        <nexus_ip> nexus.internal
        <foundry_ip> foundry.internal
        <git_ip> git.internal
        fallthrough
    }
    # Forward only allowlisted domains to upstream
    forward harbor.internal nexus.internal foundry.internal git.internal <upstream_dns>
    # Block everything else
    template IN A . {
        rcode NXDOMAIN
    }
    log
    prometheus :9153
}
```

Additional protections:
- Block TXT/NULL/CNAME record types unless specifically needed (commonly used for DNS tunneling).
- Rate-limit DNS queries per source (detect tunneling attempts).
- Monitor query patterns for high subdomain entropy (tunneling signature).

### 8.5 LLM API Sidecar Proxy

The LLM API channel legitimately carries source code. This is the hardest channel to secure because the LLM needs to see code to function.

```
Agent (in container)
  |
  | HTTP to localhost:8443 (NO auth header)
  v
aibox-llm-proxy (sidecar process in container)
  |
  | - Reads API key from Vault-mounted secret
  | - Injects Authorization header (agent never sees key)
  | - Logs full request/response payloads to audit store
  | - Enforces rate limits (60 req/min, 100K tokens/min)
  | - Enforces payload size limits
  | - Adds sandbox identity headers for audit correlation
  |
  | HTTPS to egress proxy -> LLM gateway
  v
```

**Detection-focused controls** (prevention would break functionality):
- Log ALL LLM API payloads to immutable audit store.
- Alert on anomalous patterns: request size spikes, base64-encoded data in unusual fields, unusual request frequency.
- Rate-limit to prevent bulk exfiltration.
- For highest-sensitivity: run LLM inside the security boundary (self-hosted models via vLLM/TGI).

### 8.6 Package Manager Proxying

All package installations go through Nexus/Artifactory mirrors. Direct access to upstream registries (npmjs.org, Maven Central, PyPI) is blocked.

```
Container -> Squid proxy -> Nexus/Artifactory -> Upstream registries
                                  |
                           [vulnerability scan]
                           [license check]
                           [policy check]
```

Supported registries: npm, Maven Central, Gradle Plugin Portal, PyPI, NuGet, Go modules, Cargo (crates.io).

---

## 9. Container Isolation

### 9.1 Runtime Isolation (gVisor)

gVisor provides a user-space kernel that intercepts syscalls. Even if an attacker achieves code execution inside the container, they attack gVisor's Sentry (a Go application in a constrained sandbox) rather than the host kernel.

| Runtime | Isolation | Performance Overhead | Compatibility |
|---------|-----------|---------------------|---------------|
| runc (standard) | Namespaces + cgroups | ~1-2% | Excellent |
| **gVisor (runsc)** | **User-space kernel** | **5-10% typical** | **Good (dev workflows work)** |
| Kata Containers | Full VM | Higher (VM boot, ~256MB/sandbox) | Good |

gVisor is the default. Kata Containers available for highest-classification workloads.

### 9.2 Seccomp Profile

AI-Box uses an **allowlist-based seccomp profile** (default-deny, explicitly allow needed syscalls). Key denials beyond Docker defaults:

| Blocked Syscall | Reason |
|----------------|--------|
| `ptrace` | Prevents debugging/tracing other processes (escape vector) |
| `mount`, `umount2` | Prevents filesystem manipulation |
| `pivot_root`, `chroot` | Prevents filesystem escape |
| `bpf` | Prevents eBPF program loading (could bypass network controls) |
| `userfaultfd` | Commonly used in kernel exploits |
| `unshare`, `setns` | Prevents namespace manipulation |
| `init_module`, `finit_module` | Prevents kernel module loading |
| `kexec_load` | Prevents kernel replacement |
| `keyctl`, `add_key` | Prevents kernel keyring access |

Full seccomp profile is maintained in the `aibox-policies` repository and signed.

### 9.3 AppArmor Profile

```
profile aibox-sandbox flags=(attach_disconnected,mediate_deleted) {
  # Read-only root, writable workspace
  / r,
  /** r,
  /workspace/** rw,
  /tmp/** rw,
  /dev/null rw,
  /dev/zero r,
  /dev/urandom r,
  /dev/pts/* rw,
  /proc/self/** r,

  # Deny sensitive paths
  deny /home/** rw,
  deny /root/** rw,
  deny /**/.ssh/** rw,
  deny /**/docker.sock rw,
  deny /proc/*/mem rw,
  deny /proc/kcore r,
  deny /proc/sysrq-trigger rw,

  # Deny dangerous operations
  deny mount,
  deny ptrace,
  deny capability sys_admin,
  deny capability sys_ptrace,
  deny capability net_admin,
  deny capability net_raw,
}
```

### 9.4 Mandatory Security Settings

Every AI-Box container runs with:

```bash
--cap-drop=ALL                            # Drop all Linux capabilities
--security-opt=no-new-privileges:true     # No privilege escalation via setuid
--security-opt=seccomp=/etc/aibox/seccomp.json
--security-opt=apparmor=aibox-sandbox
--runtime=runsc                           # gVisor
--read-only                               # Read-only root filesystem
--user=1000:1000                          # Non-root user
```

No `sudo`, no `su`, no setuid binaries in the image. These cannot be overridden by developers.

---

## 10. Filesystem Controls

### 10.1 Mount Layout

```
/                   read-only (image filesystem)
/workspace          rw,nosuid,nodev (source code -- bind mount or clone)
/home/dev           rw,nosuid,nodev (named volume -- user config, dotfiles, shell history)
/opt/toolpacks      rw,nosuid,nodev (named volume -- runtime-installed tools)
/tmp                tmpfs, rw,noexec,nosuid,size=2G (ephemeral)
/var/tmp            tmpfs, rw,noexec,nosuid,size=1G (ephemeral)
```

### 10.2 Source Code Location

**Source code MUST live on a native Linux filesystem (ext4) inside the container or WSL2.**

| Scenario | Performance | Status |
|----------|------------|--------|
| Code on ext4 inside container/WSL2 | 1x (baseline) | **Required** |
| Code on NTFS mounted into WSL2 | 3-10x slower | **Blocked by `aibox` CLI** |
| Code on NTFS mounted into Docker | 5-20x slower | **Blocked** |

The `aibox` CLI detects and warns if workspace is on a Windows NTFS path. Default behavior: `git clone` inside the WSL2 filesystem.

### 10.3 Denied Paths

| Path | Reason |
|------|--------|
| `/home` (host) | Prevents access to host user data |
| `/root` | Prevents root home access |
| `~/.ssh` | Prevents SSH key theft |
| `/var/run/docker.sock` | Prevents Docker socket access |
| `/proc/*/environ` | Prevents reading other process environments |
| `/proc/kcore` | Prevents kernel memory access |

### 10.4 Build Cache Persistence

Build caches are critical for productivity. Named volumes persist across sessions:

```
aibox-maven-cache    -> /home/dev/.m2/repository
aibox-gradle-cache   -> /home/dev/.gradle/caches
aibox-npm-cache      -> /home/dev/.npm
aibox-bazel-cache    -> /home/dev/.cache/bazel
```

These are per-user, persist across container recreations (image updates), and can be pre-warmed.

---

## 11. Credential Management

### 11.1 Design Principle

**Default: no credentials present.** All credentials are injected at runtime, short-lived, scoped, and automatically revoked when the sandbox terminates.

### 11.2 Architecture: Vault + SPIFFE/SPIRE

```
SPIRE Server (management host)
  |
  | Issues SVID (SPIFFE Verifiable Identity Document)
  v
SPIRE Agent (on dev machine / WSL2)
  |
  | Attests workload identity via container metadata
  v
Sandbox workload presents SVID to Vault
  |
  v
Vault validates SVID, returns scoped, short-lived credential
```

### 11.3 Credential Types

| Credential | Scope | TTL | Injection Method |
|-----------|-------|-----|-----------------|
| Git token | Single repo, read+write | 4 hours | Git credential helper -> Vault |
| LLM API key | Rate-limited, specific model | 8 hours | Sidecar proxy (agent never sees key) |
| Package mirror token | Read-only downloads | 8 hours | Environment variable via Vault |
| IDE license | Validation only | Session-length | Environment variable |

### 11.4 Git Authentication

**HTTPS tokens via credential helper (not SSH keys).**

```bash
# Container git config
git config --global credential.helper '/usr/local/bin/aibox-credential-helper'

# The helper:
# 1. Requests short-lived token from Vault (4-hour TTL)
# 2. Returns in git-credential format
# 3. Token scoped to specific repo (read+write)
# 4. Auto-expires; helper fetches new token when needed
```

### 11.5 LLM API Key Injection

The agent must NEVER see the API key directly. The LLM sidecar proxy:
1. Reads API key from Vault-mounted secret.
2. Strips any auth headers the agent might add.
3. Injects the correct `Authorization` header.
4. Agent talks to `localhost:8443` without credentials.

If the agent is compromised via prompt injection, it cannot exfiltrate the API key.

### 11.6 Token Lifecycle

- Tokens minted on sandbox start.
- Tokens revoked immediately on sandbox stop/destroy.
- All token issuance and use is logged.
- Tokens cannot be persisted to workspace (git commit of tokens is blocked by policy).

### 11.7 Simplified Alternative

For organizations without Vault infrastructure, a simpler credential broker is acceptable for initial deployment:

```bash
# aibox CLI injects credentials at container start
aibox start --workspace ~/projects/my-service
# Internally: reads token from secure host storage, passes as env var
# Env var visible only to container init process, not written to disk
```

This is less secure (env vars visible via `/proc`) but acceptable as a bootstrap before Vault is deployed. gVisor's `/proc` implementation provides additional protection.

---

## 12. Policy Engine

### 12.1 Policy Hierarchy

```
Org Baseline Policy (immutable, signed by security team)
  |
  v
Team Policy (can tighten, managed by team leads)
  |
  v
Project Policy (can tighten, in repo at /aibox/policy.yaml)
```

Developers can **never loosen** policy below the org baseline. The `aibox` CLI enforces the merge.

### 12.2 Policy Specification

```yaml
# /aibox/policy.yaml
version: 1

network:
  mode: "deny-by-default"
  allow:
    - id: "llm-gateway"
      hosts: ["foundry.internal"]
      ports: [443]
      rate_limit: { requests_per_min: 60, tokens_per_min: 100000 }
    - id: "artifact-repo"
      hosts: ["nexus.internal"]
      ports: [443]
    - id: "git-server"
      hosts: ["git.internal"]
      ports: [443]

filesystem:
  workspace_root: "/workspace"
  deny:
    - "/home"
    - "/root"
    - "/.ssh"
    - "/var/run/docker.sock"

tools:
  rules:
    - match: ["git", "status"]
      allow: true
      risk: "safe"
    - match: ["git", "push"]
      allow: true
      risk: "review-required"
    - match: ["curl", "*"]
      allow: false
      risk: "blocked-by-default"
    - match: ["rm", "-rf", "/workspace"]
      allow: false
      risk: "blocked-by-default"

resources:
  cpu: "4"
  memory: "8Gi"
  disk: "40Gi"

runtime:
  engine: "gvisor"        # or "kata" for highest classification
  rootless: true
  ephemeral: false        # true = auto-wipe on exit (caches preserved)
```

### 12.3 OPA Validation

All policies are validated by OPA before application:

```rego
# Deny wildcard host allowlists
deny[msg] {
    input.network.allow[_].hosts[_] == "*"
    msg := "Wildcard host allowlist is prohibited"
}

# Require gVisor for classified workloads
deny[msg] {
    input.classification == "classified"
    input.runtime.engine != "gvisor"
    msg := "Classified workloads must use gVisor runtime"
}

# Require LLM rate limiting
deny[msg] {
    input.network.allow[i].id == "llm-gateway"
    not input.network.allow[i].rate_limit
    msg := "LLM gateway must have rate_limit configured"
}
```

### 12.4 Tool Permission Model

Every tool/command declares a risk class:

| Risk Class | Behavior | Example |
|-----------|----------|---------|
| `safe` | Allowed, no prompt | `git status`, `ls`, `cat` |
| `review-required` | Allowed with async approval | `git push`, `npm publish` |
| `blocked-by-default` | Denied unless explicit policy exception | `curl`, `wget`, `ssh` |

For `review-required` actions: the `aibox` agent logs the intent, allows the action, and creates an audit entry. For interactive approval: a notification surfaces in the IDE or CLI.

---

## 13. IDE Integration

### 13.1 VS Code

**Approach: Remote - SSH**

1. AI-Box container runs an SSH server (port 22, mapped to host port 2222).
2. Developer opens VS Code, connects via Remote - SSH.
3. VS Code Server installs and runs **inside** the container.
4. Extensions, language servers, terminals, debuggers -- all inside the sandbox.
5. Files on native ext4 -- no cross-OS sync penalty.

Pre-install VS Code Server in the base image to avoid first-connect download.

**Extension management**:
- Pre-approved extensions baked into the image.
- All telemetry disabled: `"telemetry.telemetryLevel": "off"`.
- Extension marketplace access optionally proxied through egress allowlist.
- Known telemetry endpoints blocked at proxy level.

### 13.2 JetBrains (IntelliJ, WebStorm, etc.)

**Approach: JetBrains Gateway with SSH**

1. Developer opens JetBrains Gateway locally.
2. Selects workspace (or connects via SSH to localhost:2222).
3. Gateway downloads JetBrains backend into the container.
4. Frontend (thin client) runs locally, backend runs in container.
5. All indexing, code analysis, debugging inside the sandbox.

**Resource requirement**: JetBrains backend needs 2-4GB RAM. Containers running IntelliJ backend + build tools + AI agent need **minimum 8GB RAM, 4 CPU cores**.

### 13.3 Connection Model

```
IDE (host) ---SSH (localhost:2222)---> Container SSH server (:22)
```

The IDE connection does NOT traverse the egress proxy. It is localhost-only. IDE traffic is not an exfiltration vector (stays on the local machine).

### 13.4 Debugging

All debugging works natively because everything runs inside the container:

| Scenario | How It Works |
|----------|-------------|
| Node.js debug | Debug adapter inside container, VS Code connects over SSH tunnel |
| Java debug (JDWP) | JDWP inside container, Gateway frontend connects to backend |
| Browser DevTools | Port forwarding from container to host |
| Hot reload | File watcher inside container, native inotify, no cross-OS latency |

### 13.5 Shell and Dotfiles

- `bash` and `zsh` available in base image.
- `tmux` available for session persistence.
- Dotfiles sync mechanism: `aibox` CLI can clone a dotfiles repo into the container home volume.
- Shell history persists in the named home volume.

---

## 14. AI Tool Integration

### 14.1 Claude Code

Claude Code is a CLI that runs in the terminal inside the sandbox:

```
Developer -> IDE Terminal (in container) -> claude (CLI)
                                              |
                                              v
                                     LLM Sidecar Proxy (localhost:8443)
                                              |
                                              v
                                     Squid Proxy -> foundry.internal
```

**Setup**:
1. `claude` binary installed via tool pack or in base image.
2. API key injected via sidecar proxy (agent never sees it). Or via `ANTHROPIC_API_KEY` env var from credential broker.
3. Egress allowlist: Anthropic API endpoint or internal LLM gateway.
4. Claude Code's filesystem access naturally confined to `/workspace`.
5. Claude Code's tool execution (bash, file edits) confined to the container.

### 14.2 Codex CLI

Same architecture as Claude Code:
1. `codex` binary in tool pack.
2. `OPENAI_API_KEY` via credential broker.
3. Egress allowlist: OpenAI API endpoint or internal gateway.

### 14.3 Future Agents

The architecture is agent-agnostic. Any CLI-based agent that needs:
- Terminal access: available via SSH.
- LLM API access: via sidecar proxy.
- File system access: confined to `/workspace`.
- Tool execution: confined to container, gated by policy.

### 14.4 MCP Server Discovery

AI-Box writes a standard MCP configuration file that agents discover:

```json
// ~/.config/claude/claude_desktop_config.json (auto-generated by aibox)
{
  "mcpServers": {
    "filesystem": {
      "command": "aibox-mcp-filesystem",
      "args": ["--root", "/workspace"]
    },
    "git": {
      "command": "aibox-mcp-git",
      "args": ["--repo", "/workspace"]
    }
  }
}
```

### 14.5 Coder AgentAPI (Phase 2)

When using Coder (centralized mode), the AgentAPI provides:
- Web-based chat UI for AI agents running in workspaces.
- Status monitoring (agent idle/working).
- Multi-agent orchestration.

```bash
# Inside a Coder workspace
agentapi server -- claude
```

---

## 15. Tool Packs

### 15.1 Design

Tool packs are installable bundles that add language runtimes, build tools, and utilities to the sandbox without rebuilding the base image.

**Dual approach**:
- **Pre-built image variants** for common stacks (fast pull, deterministic).
- **Runtime install** for custom combinations (flexible, slower first install).

### 15.2 Manifest Schema

```yaml
# toolpacks/java/manifest.yaml
name: java
version: "21.0.2"
description: "OpenJDK 21 + Gradle 8.5 + Maven 3.9.6"
maintainer: "platform-team"

install:
  method: "docker-layer"      # or "nix", "script"
  base_image: "eclipse-temurin:21-jdk"
  packages:
    - gradle:8.5
    - maven:3.9.6

network:
  requires:
    - id: "maven-central"
      hosts: ["nexus.internal"]   # Via mirror, not direct
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

security:
  checksum: "sha256:abc123..."
  signed_by: "platform-team@org.internal"

tags: ["language", "jvm"]
```

### 15.3 Available Tool Packs (Initial)

| Pack | Contents | Target Size |
|------|----------|------------|
| `java@21` | OpenJDK 21, Gradle 8.5, Maven 3.9.6 | +400MB |
| `node@20` | Node.js 20 LTS, npm, yarn | +150MB |
| `python@3.12` | Python 3.12, pip, venv | +100MB |
| `bazel@7` | Bazel 7.x | +200MB |
| `scala@3` | Scala 3.x, sbt, Metals LSP | +300MB |
| `angular@18` | Angular CLI 18 | +50MB (requires node) |
| `ai-tools` | Claude Code CLI, Codex CLI | +100MB |

### 15.4 Governance

| Role | Responsibility |
|------|---------------|
| Platform Team | Maintains base image, core tool packs, registry |
| Security Team | Reviews new tool packs for supply chain risk |
| Team Leads | Can approve team-specific tool packs within policy |
| Champions | Can submit and test tool pack PRs |

**Request SLA**:
- Known/registered tools: available within 1 business day.
- New tools requiring security review: 3-5 business days.
- Emergency requests: same-day with platform team approval.

---

## 16. MCP Packs

### 16.1 Design

MCP packs are optional server bundles that AI agents can discover and use inside the sandbox.

| MCP Pack | Network Needs | Purpose |
|----------|--------------|---------|
| `filesystem-mcp` | None (local) | File operations, naturally sandboxed to `/workspace` |
| `git-mcp` | Git server (allowlisted) | Git operations |
| `jira-mcp` | Jira endpoint (allowlisted) | Issue tracking |
| `docs-mcp` | Docs endpoint (allowlisted) | Internal documentation access |

### 16.2 Enabling MCP Packs

```bash
aibox mcp enable filesystem-mcp git-mcp
# Updates MCP config inside container
# AI agents auto-discover available MCP servers
```

Each MCP pack declares its required network endpoints and filesystem permissions in its manifest. The policy engine validates these against the project policy before enabling.

---

## 17. Image Strategy

### 17.1 Base Image

**Ubuntu 24.04 LTS** -- largest ecosystem, best tool compatibility, 5-year LTS, developers know it.

### 17.2 Image Variants

```
aibox-base:24.04          ~500MB   OS + essential tools + aibox-agent
aibox-java:21-24.04       ~900MB   base + JDK 21 + Maven + Gradle
aibox-node:20-24.04       ~650MB   base + Node 20 + npm/yarn
aibox-full:24.04          ~1.5GB   base + Java + Node + Python + Bazel
```

### 17.3 Base Image Contents

```
# OS layer
Ubuntu 24.04 LTS (minimal), ca-certificates, git, ssh-client,
jq, yq, vim, nano, bash, zsh, tmux, build-essential, python3,
curl (disabled by network policy)

# AI-Box layer
aibox-agent (policy enforcement)
aibox-llm-proxy (LLM sidecar)
SSH server (for IDE remote)
/etc/aibox/policy.yaml (default deny-all)
/etc/aibox/seccomp.json
```

### 17.4 Image Registry: Harbor

- Self-hosted, CNCF graduated.
- Built-in Trivy vulnerability scanning.
- Cosign image signing verification.
- Replication for multi-site / air-gapped distribution.
- RBAC and audit logging.
- OCI artifact support (store tool packs, policies alongside images).

### 17.5 Image Signing

All images signed with Cosign (Sigstore) at build time. Podman configured to reject unsigned images:

```bash
# Build pipeline
cosign sign --key cosign.key harbor.internal/aibox/base:24.04

# Client verification (/etc/containers/policy.json)
# Rejects unsigned or tampered images automatically
```

### 17.6 Update Cadence

| Component | Cadence | Trigger |
|-----------|---------|---------|
| OS security patches | Weekly | CVE publication |
| Tool pack versions | Monthly | Upstream releases |
| Base image rebuild | Monthly | Accumulate patches |
| Emergency patches | Immediate | Critical CVE (CVSS 9+) |

Process: Automated CI pipeline rebuilds images weekly. New images pushed with date tag (`24.04-20260218`). `aibox update` pulls latest. Running containers not disrupted -- updates apply on next start. Emergency CVEs trigger mandatory update within 24 hours.

---

## 18. Developer Experience Requirements

### 18.1 Performance SLAs

| Metric | Target |
|--------|--------|
| Cold start (new workspace) | < 90 seconds |
| Warm start (existing workspace, stopped) | < 15 seconds |
| Reconnect (workspace running, IDE disconnected) | < 5 seconds |
| Build performance vs local | Within 20% of local builds |
| AI tool response latency (proxy overhead) | < 50ms added |

### 18.2 Resource Allocation Per Developer

| Workload Profile | CPU | RAM | Disk |
|-----------------|-----|-----|------|
| Frontend (React/Angular) | 2 cores | 4-6 GB | 20 GB |
| Backend (Java/Kotlin + IntelliJ) | 4 cores | 8-12 GB | 40 GB |
| Full-stack + AI agent | 4-6 cores | 10-16 GB | 50 GB |
| Monorepo (Bazel/Nx) | 6-8 cores | 12-16 GB | 60 GB |

### 18.3 Developer Machine Requirements

**For local-first model**:
- Windows 11: 16GB RAM minimum (32GB recommended), SSD, 8+ CPU cores.
- Linux: 16GB RAM minimum, SSD, 8+ CPU cores.
- WSL2 needs at least 12GB allocated.

**For centralized model (Phase 2)**: Any modern laptop with 8GB+ RAM (compute is remote).

### 18.4 Shell and Dotfiles

- Bash and Zsh available.
- Tmux for session persistence.
- Dotfiles sync: `aibox` clones a configured dotfiles repo into the persistent home volume.
- Shell history persists across sessions.

### 18.5 `git push` Approval Flow

When `git push` is `review-required` in policy, the flow is **non-blocking**:

1. Developer runs `git push`.
2. Push goes to a staging ref.
3. Webhook notifies approver (Slack, email, or dashboard).
4. Developer continues working (not blocked).
5. Approver reviews and approves/rejects.
6. On approval, staging ref merged to target branch.

---

## 19. Audit and Compliance

### 19.1 What to Log

| Event Category | Specific Events | Retention |
|---------------|----------------|-----------|
| Sandbox lifecycle | Create, start, stop, destroy, configuration | 2+ years |
| Network | All connections (allowed + denied), destination, bytes, duration | 1+ year |
| DNS | All queries and responses | 1+ year |
| Tool invocations | Every command, arguments, exit code, approver | 2+ years |
| Credential access | Token issuance, use, rotation, revocation | 2+ years |
| Policy decisions | Every OPA evaluation: input, decision, reason | 2+ years |
| LLM API traffic | Full request/response payloads (or hashes) | 1+ year |
| File access | Reads/writes to sensitive paths | 1+ year |

### 19.2 Immutability

- Append-only storage (S3 Object Lock, WORM-compliant storage, or MinIO on-prem).
- Cryptographic hash chain to detect tampering.
- Log storage separated from sandbox operators.
- Dual-control access to raw logs.

### 19.3 Session Recording (Optional)

For classified environments:
- Terminal I/O recording via `script` wrapper or Teleport.
- Encrypted, append-only storage.
- On-demand playback for incident investigation.
- Developers informed that sessions are recorded (legal + deterrent).

### 19.4 Runtime Security Monitoring

**Falco** (optional but recommended) deployed on dev machines for runtime detection:
- Detects container escape attempts.
- Alerts on unexpected network connections.
- Detects privilege escalation attempts.
- Detects access to sensitive files.

### 19.5 SIEM Integration

```
Sandbox logs ----+
Host auditd -----+--> Vector/Fluentd --> Enterprise SIEM
Squid proxy -----+
OPA decisions ---+
Vault audit -----+
Falco alerts ----+
```

Detection rules: anomalous outbound data volume, DNS query spikes, credential access outside hours, repeated blocked network attempts, LLM API request size anomalies.

---

## 20. Threat Model

### 20.1 In-Scope Threats

| Threat | Mitigation |
|--------|-----------|
| Prompt injection causing secret reading | gVisor `/proc` isolation, no secrets in container (sidecar proxy) |
| Prompt injection causing exfiltration (curl/wget) | Default-deny network, host-level enforcement |
| Prompt injection causing destructive commands | Policy engine tool gating, `review-required` for dangerous ops |
| Prompt injection causing lateral movement | Isolated container network, no LAN access |
| DNS tunneling for data exfiltration | CoreDNS allowlist-only, block alt DNS (DoH/DoT), query monitoring |
| Supply chain attack via package managers | Nexus/Artifactory mirrors with scanning, optional curated lists |
| IDE plugin/extension data leakage | Telemetry disabled, curated extensions, telemetry endpoints blocked |
| Container escape | gVisor user-space kernel, seccomp, AppArmor, rootless, no capabilities |
| Clipboard-based exfiltration (hybrid mode) | Clipboard sharing disabled or size-limited (per sensitivity level) |
| Covert channels via LLM API | Rate limiting, payload logging, anomaly detection, payload size limits |
| Steganographic exfiltration via git metadata | Normalize commit metadata, scan for anomalies |
| Build artifact exfiltration | Scan artifacts before export, strip debug symbols/metadata |
| Malicious tool packs | Signed packs, security review process, image signing |

### 20.2 Out-of-Scope

| Threat | Rationale |
|--------|-----------|
| Physical host compromise | Requires physical security measures |
| Insider with host admin access | Mitigate with PAM and separation of duties |
| Developer screenshots of IDE | Physical/visual exfiltration cannot be stopped by software |
| Zero-day kernel vulnerabilities | Defense-in-depth (gVisor reduces surface), rapid patching |

---

## 21. Transition and Rollout Plan

### 21.1 Phased Rollout

```
Phase 0: Foundation       (Weeks 1-4)    3-5 platform engineers
Phase 1: Pilot            (Weeks 5-8)    10 volunteer developers
Phase 2: Early Adopters   (Weeks 9-14)   30-40 developers
Phase 3: General Rollout  (Weeks 15-22)  Remaining developers
Phase 4: Mandatory        (Week 23+)     Local dev deprecated
```

#### Phase 0: Foundation (3-5 platform engineers)
- Deploy Harbor registry, configure Nexus mirrors.
- Build base image + top 3 tool packs (Java, Node, Python).
- Develop `aibox` CLI with core commands.
- Configure egress proxy with LLM endpoint allowlist.
- Build workspace templates for 2-3 major project types.
- Build monitoring dashboards.
- Write quickstart documentation.
- **Exit criteria**: Platform team goes from zero to coding in < 90 seconds.

#### Phase 1: Pilot (10 volunteer developers)
- Select across teams, OS types, IDE preferences.
- Pair each pilot dev with a platform engineer for first day.
- Daily feedback surveys for 2 weeks, then weekly.
- Fix top 5 pain points before Phase 2.
- **Exit criteria**: 8/10 pilot devs rate experience as "acceptable or better".

#### Phase 2: Early Adopters (30-40 developers)
- Open to volunteers + nominated team leads.
- Launch self-service onboarding.
- Establish Champions program (1 per team, 15-20 total).
- Build additional tool packs based on demand.
- AI-Box and local dev run in parallel.
- **Exit criteria**: < 3 support tickets/week, startup time < 90s at p95.

#### Phase 3: General Rollout (remaining developers)
- All new projects start in AI-Box by default.
- Existing projects migrate team-by-team.
- Dedicated "migration office hours" 3x/week.
- **Exit criteria**: > 90% of active developers using AI-Box.

#### Phase 4: Mandatory
- Local development unsupported (not removed, but no assistance).
- 1:1 migration support for holdouts.
- Full policy enforcement activated.

### 21.2 Training Materials

| Material | Format | Audience |
|----------|--------|----------|
| "AI-Box in 5 minutes" | Screen recording | All developers |
| Quickstart (VS Code) | Written + screenshots | VS Code users |
| Quickstart (IntelliJ) | Written + screenshots | JetBrains users |
| Troubleshooting FAQ | Wiki/Confluence | All developers |
| "Building Tool Packs" | Written + examples | Power users |
| Architecture overview | Diagram + written | Curious developers |

### 21.3 Champions Program

- 1 champion per team (15-20 total).
- Early access to features, direct Slack channel with platform team.
- First point of contact for their team.
- Surface pain points, test new tool packs.

### 21.4 Support Model

```
Tier 0: Self-service (docs, FAQ, `aibox doctor`)     70% of issues
Tier 1: Champions (team-level troubleshooting)         20% of issues
Tier 2: Platform team (Slack #ai-box-help)              9% of issues
Tier 3: Escalation (infrastructure/security)            1% of issues
```

### 21.5 Fallback Plan

During Phases 1-3, developers MUST be able to fall back to local development:
- Keep local dev tooling functional.
- AI-Box is additive, not a gate, during early phases.
- Track fallback frequency as a metric (it tells you what's broken).

---

## 22. Operations

### 22.1 Day-2 Operations

| Activity | Frequency | Owner | Automation |
|----------|-----------|-------|-----------|
| Image rebuild (patches) | Weekly | CI pipeline | Fully automated |
| Image signing | Every build | CI pipeline | Fully automated |
| CVE triage | Daily review | Platform team | Alert-driven |
| Policy updates | As needed | Security team | Git PR + review |
| Tool pack updates | Monthly | Platform team | Semi-automated |
| Dev support | Ongoing | Platform team (0.5 FTE) | `aibox doctor` handles most |
| Nexus mirror sync | Continuous | Nexus | Automatic |
| Harbor GC | Weekly | Harbor cron | Automatic |
| Compatibility testing | Monthly | Platform team | Manual test matrix |

**Staffing**: 0.5-1 FTE ongoing for local-first model. 2 FTEs during initial rollout (Phases 0-2).

### 22.2 Image Updates

1. Automated CI pipeline rebuilds weekly (or immediately for critical CVEs).
2. New image pushed to Harbor with date tag.
3. `aibox update` pulls latest signed image.
4. Running containers NOT disrupted (update on next start).
5. Critical CVEs: mandatory update within 24 hours (`aibox start` refuses outdated image).

### 22.3 `aibox doctor` (Self-Service Diagnostics)

```bash
aibox doctor
# Checks:
#  [OK] Podman installed and running
#  [OK] gVisor runtime available
#  [OK] WSL2 memory allocation (16GB)
#  [OK] Proxy reachable (localhost:3128)
#  [OK] DNS resolver responding
#  [OK] Harbor registry accessible
#  [OK] Image signature valid
#  [OK] Policy file valid
#  [WARN] Image is 15 days old -- run `aibox update`
```

### 22.4 Disaster Recovery

| Scenario | Recovery |
|----------|----------|
| Dev machine dies | Re-image, `aibox setup`, pull images, clone repos. ~1-2 hours. |
| Harbor down | Devs continue with cached local images. Fix registry. |
| Nexus down | Builds fail if deps not cached. Priority fix. Build caches mitigate. |
| Vault down | Cached credentials valid until TTL expires. Graceful degradation. |

---

## 23. Tech Stack Summary

| Control | Technology | Rationale |
|---------|-----------|-----------|
| Container runtime | **Podman 5.x** (rootless) | Free, rootless-by-default, daemonless, OCI-compliant |
| OCI runtime | **gVisor (runsc)** | User-space kernel, eliminates host kernel attack surface |
| Image build | **Buildah** | Rootless, daemonless OCI image builds |
| L3/L4 enforcement | **nftables** on host | Host-level, container cannot bypass |
| L7 proxy | **Squid 6.x** | Mature, domain allowlisting, HTTPS CONNECT, logging |
| DNS control | **CoreDNS** | Allowlist-only resolution, logging, rate limiting |
| Image registry | **Harbor 2.x** | Signing, scanning, replication, RBAC, air-gap support |
| Image signing | **Cosign (Sigstore)** | Keyless signing, transparency log |
| Package mirror | **Nexus Repository 3.x** | All major formats, vulnerability scanning, caching |
| Secret management | **HashiCorp Vault** | Dynamic secrets, short-lived tokens, audit logging |
| Workload identity | **SPIFFE/SPIRE** | Cryptographic identity without pre-shared secrets |
| Policy engine | **OPA (Rego)** | Declarative policy-as-code, admission control, decision logs |
| Runtime monitoring | **Falco** (optional) | Syscall monitoring, container escape detection |
| Log shipping | **Vector** | High-performance, Rust-based, all output formats |
| Vulnerability scan | **Trivy** (via Harbor) + **Grype** | Image + runtime scanning |
| Base image | **Ubuntu 24.04 LTS** | Largest ecosystem, LTS, developer familiarity |
| CLI | **`aibox`** (Go or Rust) | Single entry point, abstracts all infrastructure |
| Platform (Phase 2) | **Coder** (self-hosted on K8s) | AgentAPI for AI tools, IDE plugins, centralized policy |

---

## 24. Residual Risks

Even with all controls, the following risks remain and are accepted:

| Risk | Severity | Mitigation |
|------|----------|-----------|
| LLM API as exfiltration path | Medium | Detection-focused: payload logging, rate limiting, anomaly detection. LLM must see code to function. |
| Zero-day container/gVisor escape | Low | Defense-in-depth: host-level controls persist after escape. Rapid patching. |
| Timing-based covert channels | Low | Rate limiting, anomaly detection. Cannot fully eliminate without destroying utility. |
| Insider with host admin access | Low | PAM, separation of duties, audit logging. Explicitly out of scope. |
| Developer screenshots/photos | Low | Physical security, DLP policies. Cannot be stopped by software. |
| Clipboard exfiltration (hybrid mode) | Medium | Disable clipboard sharing for highest sensitivity. Size-limit + log for moderate. |

---

## 25. Appendices

### Appendix A: `aibox` CLI Reference

```bash
aibox setup                              # Initial setup (Podman, proxy, DNS, nftables)
aibox start --workspace ~/project        # Start sandbox
aibox start --toolpacks java@21,node@20  # Start with specific tool packs
aibox stop                               # Stop sandbox
aibox shell                              # Open shell in sandbox
aibox status                             # Check sandbox status
aibox update                             # Pull latest image
aibox install bazel@7                    # Install tool pack in running sandbox
aibox doctor                             # Run diagnostics
aibox policy validate                    # Validate project policy
aibox policy explain --log-entry <id>    # Explain why something was blocked
aibox mcp enable filesystem-mcp git-mcp  # Enable MCP servers
aibox mcp list                           # List available MCP packs
aibox network test                       # Test egress connectivity
aibox repair cache                       # Clear and rebuild build caches
```

### Appendix B: Developer Quickstart (VS Code + Windows 11)

```bash
# 1. Install AI-Box (one-time)
winget install aibox        # or download from internal portal
aibox setup                 # Installs Podman, configures WSL2, proxy, DNS

# 2. Start sandbox for your project
aibox start --workspace ~/projects/my-service --toolpacks java@21

# 3. Connect VS Code
# VS Code auto-detects the sandbox (Remote - SSH)
# Or: code --remote ssh-remote+aibox /workspace

# 4. Use AI tools
# In VS Code terminal (inside sandbox):
claude                      # Claude Code -- API key auto-injected
codex                       # Codex CLI -- API key auto-injected

# 5. Build, test, push
gradle build                # Build caches persist between sessions
git push                    # Approved per policy
```

### Appendix C: Policy Hierarchy Example

```yaml
# ORG BASELINE (immutable, signed by security team)
# /etc/aibox/org-policy.yaml
network:
  mode: "deny-by-default"
  max_rate_limit: { requests_per_min: 120 }
runtime:
  engine: "gvisor"
  rootless: true
filesystem:
  deny: ["/home", "/root", "/.ssh", "/var/run/docker.sock"]

# TEAM POLICY (can tighten, managed by team lead)
# /etc/aibox/team-policy.yaml
network:
  allow:
    - id: "team-specific-api"
      hosts: ["api.internal"]
      ports: [443]

# PROJECT POLICY (can tighten further, in repo)
# /aibox/policy.yaml
tools:
  rules:
    - match: ["npm", "publish"]
      allow: false
      risk: "blocked-by-default"
```

### Appendix D: Air-Gapped Deployment

For fully air-gapped classified environments:

1. **Image distribution**: Harbor replication from connected staging environment. Or export images via `podman save` to approved media.
2. **Package mirrors**: Nexus configured with cached/pre-loaded packages. No upstream connectivity.
3. **LLM**: Self-hosted models (vLLM, TGI) inside the security boundary. Code never leaves the enclave.
4. **Policy distribution**: Git repo mirrored to air-gapped network.
5. **Updates**: Periodic sync via approved media transfer process.

### Appendix E: Comparison -- Local-First vs Centralized

| Factor | Local-First (Recommended) | Centralized (Coder on K8s) |
|--------|--------------------------|---------------------------|
| Year 1 cost | ~$50K (central infra only) | $715K - $1.2M |
| Ongoing cost | ~$10K/year | $405K - $605K/year |
| Dev machine requirement | 16GB+ RAM, SSD | 8GB+ RAM (any laptop) |
| IDE latency | None (localhost) | 2-20ms (network dependent) |
| Offline capability | Yes (after image pull) | No |
| Air-gap support | Excellent | Possible but complex |
| Security enforcement | Host-level (per machine) | Cluster-level (centralized) |
| Audit centralization | Optional (forward logs) | Built-in |
| Ops staffing | 0.5-1 FTE | 2+ FTE |
| Migration to other | Can add Coder later | Difficult to go back |

---

*End of AI-Box Final Specification v1.0*
