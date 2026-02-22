# AI-Box Architecture Overview

**Deliverable**: D13
**Audience**: Developers who want to understand how AI-Box works
**Last Updated**: 2026-02-21

---

## What Is AI-Box?

AI-Box is a secure development sandbox that lets you use AI coding tools (Claude Code, Codex CLI, and others) while preventing source code from leaving controlled boundaries. It runs on your local machine as a container, managed by the `aibox` CLI.

The key idea: AI tools need to see your source code to help you write it. AI-Box ensures that while the AI can see your code inside the sandbox, neither the AI nor anything else can send that code to an unauthorized destination.

---

## Architecture Diagram

```
+------------------------------------------------------------------+
| DEVELOPER WORKSTATION (Windows 11)                               |
|                                                                  |
|  +------------------+    +--------------------+                  |
|  | VS Code /        |    | aibox CLI          |                  |
|  | JetBrains        |    | (manages sandbox)  |                  |
|  | (native frontend)|    +--------------------+                  |
|  +--------+---------+                                            |
|           | SSH (localhost:2222)                                  |
|           |                                                      |
| +=========|=== WSL2 VM ========================================+ |
| |         v                                                    | |
| |  +------------------------------------------------------+   | |
| |  | Podman Container (gVisor runtime)                     |   | |
| |  |                                                       |   | |
| |  |  /workspace/         Your project files (native ext4) |   | |
| |  |  IDE backend          VS Code Server or JB backend    |   | |
| |  |  AI agent             Claude Code / Codex CLI         |   | |
| |  |  MCP servers          filesystem-mcp, git-mcp, etc.   |   | |
| |  |  SSH server (:22)     IDE connection endpoint         |   | |
| |  |  LLM proxy (:8443)    Injects API keys, logs payloads |   | |
| |  |                                                       |   | |
| |  |  Read-only rootfs, seccomp locked, no capabilities    |   | |
| |  +----+------+---------+---+-----------------------------+   | |
| |       |      |         |   |                                 | |
| |       |      |         |   +------- No direct egress -----X | |
| |       |      |         |                                     | |
| |  +----v-+ +--v------+  +--v-----------+                     | |
| |  |Squid | |CoreDNS  |  |nftables      |                     | |
| |  |Proxy | |(:53)    |  |(host kernel)  |                     | |
| |  |(:3128)| |         |  |              |                     | |
| |  +---+--+ +---------+  +--------------+                     | |
| |      |     Allowlist     DROP all traffic                    | |
| |      |     only          except -> proxy                     | |
| +======|======================================================+ |
|        |                                                         |
+--------|------ Allowed Egress Only ------------------------------|
         |                                                         |
         v
+------------------------------------------------------------------+
| CENTRAL INFRASTRUCTURE                                           |
|                                                                  |
|  Harbor (harbor.internal)     Signed container images            |
|  Nexus  (nexus.internal)      Package mirrors (Maven, npm, etc.) |
|  Vault  (vault.internal)      Dynamic credentials                |
|  Git    (git.internal)        Source repos + policy repo         |
|  LLM GW (foundry.internal)   LLM API (Anthropic/OpenAI)         |
+------------------------------------------------------------------+
```

---

## Six Layers

AI-Box is composed of six layers, each serving a specific purpose.

### 1. Runtime Sandbox

Your code runs inside a **Podman rootless container** with **gVisor** as the runtime. Podman is similar to Docker but runs without a privileged daemon and is free (saving the organization ~$50K/year in Docker Desktop licenses).

gVisor provides an extra isolation layer by intercepting system calls in user-space. Instead of your code talking directly to the host operating system kernel, gVisor provides a separate kernel that limits what the container can do. This dramatically reduces the attack surface even if something inside the container is compromised.

**What this means for you**: Your development environment feels like a normal Linux machine, but it is heavily sandboxed. Performance is comparable to native for most workloads.

### 2. Policy Engine

Every action inside the sandbox is governed by policies written in **OPA/Rego** (Open Policy Agent). Policies control:

- Which network endpoints can be reached
- Which files can be accessed
- Which commands and tools are allowed
- Whether `git push` requires approval

Policies follow a hierarchy:
- **Org baseline**: Set by the security team. Cannot be loosened.
- **Team policy**: Set by your team lead. Can tighten the org baseline.
- **Project policy**: In your repo as `/aibox/policy.yaml`. Can tighten further.

If a policy blocks you, `aibox policy explain --log-entry <id>` tells you exactly why.

### 3. Connectivity Layer

Network security is enforced at the **host level**, outside the container. The container cannot bypass these controls:

- **Squid proxy**: An HTTP proxy that allowlists specific domains. Only approved destinations (Nexus mirrors, LLM API, Git server) can be reached. Everything else is blocked.
- **nftables**: Firewall rules that force all container traffic through the proxy. Direct egress is dropped.
- **CoreDNS**: A DNS server that only resolves allowlisted domains. Unapproved domains return NXDOMAIN. This prevents DNS tunneling.

**What this means for you**: `npm install`, `gradle build`, and `git push` work because they go through approved mirrors. `curl` to random websites does not. This is by design.

### 4. Tool Packs

Instead of one massive container image with every tool, AI-Box uses **tool packs**: installable bundles that add language runtimes and build tools.

Available packs include `java@21`, `node@20`, `python@3.12`, `dotnet@8`, `bazel@7`, `scala@3`, `angular@18`, `powershell@7`, and more.

Install a pack with `aibox install <pack>@<version>`. No container rebuild needed.

### 5. MCP Packs

MCP (Model Context Protocol) packs provide additional capabilities to AI agents. For example:
- `filesystem-mcp`: Enhanced file operations
- `git-mcp`: Git operations

Enable them with `aibox mcp enable <pack>`. AI agents discover available MCP servers automatically.

### 6. CLI (`aibox`)

The `aibox` CLI is the single entry point. It abstracts all the infrastructure above into simple commands:

```
aibox setup     -- one-time installation
aibox start     -- start a sandbox
aibox stop      -- stop it
aibox doctor    -- diagnose problems
aibox install   -- add tool packs
aibox update    -- pull latest image
```

You should never need to interact with Podman, gVisor, Squid, or nftables directly.

---

## How Your IDE Connects

Your IDE (VS Code or JetBrains) connects to the sandbox via SSH over localhost:

```
IDE frontend (your machine) --> SSH (localhost:2222) --> Container
```

The IDE frontend runs natively on your machine for full performance. The IDE backend (VS Code Server or JetBrains backend) runs inside the container, so extensions, language servers, and debuggers all operate on the sandboxed files.

This SSH connection is localhost-only. It does not go through the proxy and is not an exfiltration path.

---

## How Credentials Work

AI-Box uses **HashiCorp Vault** to manage credentials dynamically:

- **LLM API keys** (Anthropic, OpenAI): Injected automatically via the LLM sidecar proxy. You never see or handle these keys.
- **Git credentials**: Provided by the credential broker. Scoped to allowed repositories.
- **Package mirror tokens**: Automatically configured for Nexus access.

All credentials are short-lived:
- Git tokens: 4 hours
- LLM API keys: 8 hours
- Package mirror tokens: 8 hours

Credentials refresh automatically. If Vault is temporarily unreachable, cached credentials remain valid until their TTL expires.

---

## What Happens When You `git push`

Depending on your project's policy, `git push` either works immediately or goes through a review flow:

1. You run `git push`
2. AI-Box pushes to a staging ref on the Git server
3. An approver is notified (if push gating is enabled)
4. Once approved, the push lands on the target branch
5. You can check status with `aibox push status`

This flow is non-blocking: you can keep working while a push is pending.

---

## Security Model (Conceptual)

AI-Box follows a **defense-in-depth** approach with multiple independent layers:

| Layer | What It Prevents |
|-------|-----------------|
| gVisor runtime | Container escape, host kernel exploitation |
| Read-only rootfs | Persistent tampering of the container OS |
| Seccomp profile | Dangerous system calls |
| No capabilities | Privilege escalation |
| Squid proxy (L7) | Unauthorized HTTP/HTTPS traffic |
| nftables (L3/L4) | Direct network egress bypassing the proxy |
| CoreDNS (allowlist) | DNS tunneling, DNS-over-HTTPS exfiltration |
| OPA policies | Unauthorized tool execution, file access |
| Vault credentials | Static credential theft |
| Image signing | Tampered images |

No single layer is sufficient on its own. Together, they make exfiltration extremely difficult even if one layer is compromised.

**Design principle**: Security should be invisible. If you notice friction from these controls, something is wrong and should be reported.

---

## Graceful Degradation

AI-Box is designed to keep you working even when central services have issues:

| Service Down | Impact | Mitigation |
|-------------|--------|------------|
| Harbor (image registry) | Cannot pull new images | Existing cached images continue to work |
| Nexus (package mirrors) | Builds fail if deps not cached | Local build caches satisfy most builds |
| Vault (credentials) | New credential requests fail | Cached credentials valid for 4-8 hours |
| Git server | Cannot push/pull | Continue working locally, push when restored |

---

## What AI-Box Is NOT

- **Not an AI model provider**: AI-Box consumes LLM APIs. It does not host models (though self-hosted models can be used in air-gapped environments).
- **Not a replacement for CI/CD**: It complements your build pipeline. CI/CD runs outside the sandbox.
- **Not a DLP system**: It enforces strong controls and creates audit evidence, but does not claim to prevent all possible exfiltration.
- **Not a virtual desktop**: It is a development container. Your IDE runs natively on your machine.

---

## Further Reading

- [VS Code Quickstart](quickstart-vscode.md) -- Get started with VS Code
- [JetBrains Quickstart](quickstart-jetbrains.md) -- Get started with JetBrains Gateway
- [Troubleshooting FAQ](troubleshooting-faq.md) -- Common issues and fixes
- [Building Tool Packs](building-toolpacks-guide.md) -- Create your own tool packs
- Full specification: `SPEC-FINAL.md` (for platform engineers and security reviewers)

---

*Questions about the architecture? Post in `#aibox-help` or ask your team's AI-Box champion.*
