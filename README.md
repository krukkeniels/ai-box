[![Release](https://img.shields.io/badge/release-v1.0.0-blue)](https://github.com/krukkeniels/ai-box/releases/tag/v1.0.0)
[![Go](https://img.shields.io/badge/go-1.24-00ADD8)](https://go.dev)
[![Tests](https://img.shields.io/badge/tests-635+-success)](https://github.com/krukkeniels/ai-box)
[![License](https://img.shields.io/badge/license-Apache%202.0-orange)](LICENSE)

# AI-Box

**Policy-enforced sandbox platform for organizations safely adopting agentic AI tools across their developer fleet.**

## The problem

AI coding agents (Claude Code, Codex CLI, Copilot, Cursor) need full access to source code to be useful. This creates a fundamental security tension: the same access that makes agents productive also enables code exfiltration — whether through prompt injection, supply chain compromise, or a misconfigured model endpoint.

Individual developer guardrails don't scale. Telling 200 engineers to "be careful" with their API keys and network access doesn't produce auditable compliance. Restrictive security policies that block everything get bypassed with personal hotspots and shadow IT.

Organizations need a platform-level answer: let developers use any AI tool they want, inside a container that enforces what leaves the network.

## How AI-Box solves it

AI-Box wraps every AI development session in a hardened sandbox with **defense in depth**: 10 independent security layers from kernel-level syscall filtering to application-level policy evaluation. The network is **default-deny** — only explicitly allowlisted domains are reachable. **Policy-as-code** (OPA/Rego) lets security teams define and enforce rules declaratively. Every action produces an **immutable audit trail** with hash-chain integrity. The platform is **tool-agnostic** — Claude Code, Codex, Copilot, or any CLI tool works unmodified inside the sandbox.

## Architecture

```
 Developer workstation                          Central infrastructure
┌─────────────────────────────────────┐        ┌──────────────────────┐
│  IDE / Terminal                     │        │  Harbor (images)     │
│    │                                │        │  Nexus (packages)    │
│    ▼                                │        │  Vault (secrets)     │
│  aibox CLI                          │        │  LLM Gateway         │
│    │                                │        └──────────┬───────────┘
│    ▼                                │                   │
│  ┌───────────────────────────────┐  │                   │
│  │  Podman + gVisor container    │  │                   │
│  │  ┌─────────────────────────┐  │  │                   │
│  │  │  AI agent + dev tools   │  │  │                   │
│  │  │  (Claude Code, Codex…)  │  │  │                   │
│  │  └────────────┬────────────┘  │  │                   │
│  │  Seccomp │ AppArmor │ rootless│  │                   │
│  └───────────────┼───────────────┘  │                   │
│                  │                  │                   │
│  ┌───────────────┼───────────────┐  │                   │
│  │  Network security stack       │  │                   │
│  │  Squid (L7) ← nftables (L3)  │◄─┼───────────────────┘
│  │  CoreDNS ← OPA policy engine │  │
│  └───────────────────────────────┘  │
└─────────────────────────────────────┘
```

## Feature matrix

| Category | Feature | Description |
|----------|---------|-------------|
| **Container Isolation** | gVisor (runsc) | User-space kernel intercepts all syscalls |
| | Seccomp profile | Custom syscall allowlist (200+ rules) |
| | AppArmor profile | Mandatory access control for files, network, capabilities |
| | Read-only rootfs | Immutable container filesystem |
| | Capability drop | `--cap-drop=ALL` — no privileged operations |
| | No-new-privileges | Prevents suid/sgid escalation |
| | Rootless container | Runs as UID 1000, no root inside container |
| **Network Security** | Squid L7 proxy | Domain-level allowlist with TLS inspection |
| | nftables L3/L4 | IP and port filtering, anti-bypass rules |
| | CoreDNS | DNS resolution control and tunneling prevention |
| | LLM proxy | API key injection, request logging, model routing |
| **Policy Engine** | OPA/Rego | Declarative policy evaluation for every tool invocation |
| | 3-tier hierarchy | Org baseline → Team → Project (tighten-only) |
| | Tool risk classes | safe / review-required / blocked-by-default |
| | Decision logging | Every policy decision recorded to audit trail |
| **Credentials** | Vault + SPIFFE/SPIRE | Short-lived tokens with workload identity |
| | Keychain fallback | OS keychain with AES-256-GCM encrypted file fallback |
| | Auto-inject | Credentials appear as environment variables in sandbox |
| | Auto-revoke | All tokens revoked on `aibox stop` |
| **Audit & Compliance** | 25 event types | Structured audit events covering full session lifecycle |
| | Hash chain | SHA-256 chain ensures tamper-evident log integrity |
| | Falco eBPF | 10 runtime detection rules for anomalous behavior |
| | SIEM integration | 10 detection rules, 4 sink types, MITRE ATT&CK mapping |
| | Session recording | AES-256-GCM encrypted terminal recordings |
| | Grafana dashboards | 3 dashboards with 10 alert rules |
| | Vector pipeline | Log collection with 4 sources and 4 sinks |
| | Immutable storage | Append-only log storage with batch verification |
| **Developer Experience** | 10 tool packs | Java, Node, Python, Bazel, Scala, Angular, .NET, PowerShell, AngularJS, AI tools |
| | 2 MCP packs | Filesystem MCP, Git MCP for agent integrations |
| | Dotfiles sync | Developer config synced into sandbox |
| | IDE integration | SSH key generation for remote IDE access |
| | Git push approval | Non-blocking async approval for gated pushes |
| | Port forwarding | Forward container ports to host for local dev servers |

## What developers experience

```
1. aibox start --workspace ~/my-project     Start a sandbox (~15s warm start)
2. aibox shell                               Open a terminal inside it
3. Use AI tools normally                     Claude Code, Codex — API keys injected automatically
4. Build and test as usual                   Persistent caches, fast incremental builds
5. git push                                  Works — gated for review if policy requires it
6. aibox stop                                Credentials revoked, sandbox stopped
```

The sandbox should feel like a *better* development environment, not a restricted one.

## Defense in depth: 10 security layers

AI-Box applies defense in depth — every layer is independent:

| Layer | What it does |
|-------|-------------|
| **gVisor (runsc)** | User-space kernel intercepts all syscalls — host kernel never directly exposed |
| **Seccomp** | Syscall allowlist — blocks dangerous syscalls even if gVisor is bypassed |
| **AppArmor** | MAC profile restricting file access, network, and capabilities |
| **Read-only rootfs** | Container filesystem is immutable — only `/workspace`, `/tmp`, `/home` are writable |
| **Capability drop** | `--cap-drop=ALL` — no privileged operations possible |
| **No-new-privileges** | Prevents suid/sgid escalation |
| **Rootless container** | Runs as UID 1000 — no root even inside the container |
| **Network proxy** | Squid + nftables + CoreDNS — only allowlisted domains reachable |
| **Policy engine** | OPA evaluates every tool invocation against declarative rules |
| **Credential broker** | Short-lived tokens, auto-revoked on stop, never persisted in container |

## Policy engine

AI-Box enforces security policies using OPA (Open Policy Agent) with a three-level hierarchy:

```
Org baseline (immutable)  →  Team policy  →  Project policy
         Most restrictive wins (tighten-only)
```

Policies control what AI agents can do: which commands are allowed, what network destinations are reachable, filesystem access rules, and credential TTLs.

**Tool risk classes:**

| Class | Behavior | Example |
|-------|----------|---------|
| `safe` | Runs immediately | `cat`, `ls`, `grep` |
| `review-required` | Runs with async approval logging | `git push`, `npm publish` |
| `blocked-by-default` | Denied unless explicitly allowed | `curl`, `rm -rf /` |

```bash
# Validate your policy files
aibox policy validate --org /etc/aibox/org-policy.yaml

# Validate full hierarchy
aibox policy validate \
  --org /etc/aibox/org-policy.yaml \
  --team ./team-policy.yaml \
  --project ./aibox/policy.yaml

# Understand why a decision was made
aibox policy explain --log-entry 42
```

Every policy decision is logged to `/var/log/aibox/decisions.jsonl` for audit.

## Audit and compliance

AI-Box provides a complete audit and monitoring stack for organizations that need to demonstrate compliance and detect threats in real time.

**Event categories** — 25 structured event types covering session lifecycle, policy decisions, credential operations, network activity, and file access. Every event includes timestamp, session ID, user identity, and risk classification.

**Tamper-evident logging** — SHA-256 hash chain links every audit event to its predecessor. Any modification, deletion, or reordering of log entries is cryptographically detectable.

**Runtime threat detection** — 10 Falco eBPF rules monitor for anomalous container behavior: unexpected process execution, privilege escalation attempts, sensitive file access, network policy violations, and cryptominer signatures.

**SIEM integration** — 10 detection rules with MITRE ATT&CK references, 4 Vector sink types (Elasticsearch, Splunk HEC, S3, Syslog), and pre-built correlation patterns for Splunk, Elastic, Microsoft Sentinel, IBM QRadar, and CrowdStrike.

**Session recording** — Terminal sessions are recorded and encrypted with AES-256-GCM. Recordings are stored in immutable append-only storage with batch integrity verification.

**Dashboards** — 3 Grafana dashboards (security overview, session activity, policy decisions) with 10 configurable alert rules.

See [`docs/audit-operations-guide.md`](docs/audit-operations-guide.md) for the full operations guide including SIEM integration for 5 platforms.

## Installation

### Prerequisites

- **Linux** (native or WSL2 with kernel 5.15+)
- **Podman** (rootless) or Docker
- **gVisor** (runsc) — recommended for full kernel isolation

### Download from GitHub release

```bash
# Using gh CLI
gh release download v1.0.0 --repo krukkeniels/ai-box

# Or clone the repository
git clone https://github.com/krukkeniels/ai-box.git
cd ai-box
```

### Build from source

Requires Go 1.24+:

```bash
cd cmd/aibox
go build -o bin/aibox .
sudo cp bin/aibox /usr/local/bin/
```

### First-time setup

```bash
# Install required security profiles
sudo mkdir -p /etc/aibox
sudo cp configs/seccomp.json /etc/aibox/seccomp.json

# AppArmor (Ubuntu — recommended)
sudo apparmor_parser -r configs/apparmor/aibox-sandbox

# Generate default config
aibox setup

# Verify everything is ready
aibox doctor
```

`aibox doctor` checks your system and tells you exactly what to fix if anything is missing.

## Commands

| Command | Description |
|---------|-------------|
| `aibox start -w <path>` | Start a sandbox with workspace mounted at `/workspace` |
| `aibox stop` | Stop the running sandbox (revokes credentials) |
| `aibox shell` | Open bash inside the running sandbox |
| `aibox status` | Show sandbox state, policy, and credential info |
| `aibox doctor` | Check system health and prerequisites |
| `aibox setup` | Auto-configure the host environment |
| `aibox auth add <type>` | Store a credential (git-token, llm-api-key, mirror-token) |
| `aibox auth list` | List stored credentials |
| `aibox auth remove <type>` | Remove a stored credential |
| `aibox policy validate` | Validate policy files for correctness |
| `aibox policy explain` | Explain a policy decision from the audit log |
| `aibox network test` | Verify network security stack is operational |
| `aibox install <pack>` | Install a tool pack (e.g. `bazel@7`, `node@20`) |
| `aibox list packs` | List available and installed tool packs |
| `aibox mcp list` | List available MCP packs and their status |
| `aibox mcp enable <pack>` | Enable MCP packs for AI agent integrations |
| `aibox mcp disable <pack>` | Disable one or more MCP packs |
| `aibox config set <key> <value>` | Set a configuration value |
| `aibox config get <key>` | Get a configuration value |
| `aibox port-forward <port>` | Forward a container port to the host |
| `aibox push status` | Show pending git push approval requests |
| `aibox push cancel <id>` | Cancel a pending push approval |
| `aibox update` | Update container images and components |
| `aibox repair cache` | Clear and rebuild build cache volumes |

## Configuration

Config lives at `~/.config/aibox/config.yaml`. Generated by `aibox setup`, or create manually:

```yaml
runtime: podman                    # podman or docker
image: harbor.internal/aibox/base:24.04

gvisor:
  enabled: true                    # gVisor kernel isolation
  platform: systrap                # systrap (fast) or ptrace (compat)

resources:
  cpus: 4
  memory: 8g

network:
  enabled: true                    # proxy-controlled egress
  allowed_domains:
    - harbor.internal
    - git.internal
    - foundry.internal

credentials:
  mode: fallback

logging:
  level: info
```

All settings can be overridden with `AIBOX_` environment variables (e.g. `AIBOX_RUNTIME=docker`).

## Credentials

AI-Box injects credentials into the sandbox as environment variables so AI tools work without manual configuration.

```bash
# Store your credentials (one-time)
aibox auth add git-token        # Git PAT for pushing code
aibox auth add llm-api-key      # Claude/OpenAI API key
aibox auth add mirror-token     # Package registry token

# Credentials are injected automatically on next `aibox start`
# Inside the sandbox they appear as:
#   AIBOX_GIT_TOKEN, AIBOX_LLM_API_KEY, AIBOX_MIRROR_TOKEN
```

**Storage modes:**

- **fallback** (default) — OS keychain (GNOME Keyring / libsecret) with AES-256-GCM encrypted file fallback
- **vault** — HashiCorp Vault with SPIFFE/SPIRE workload identity for short-lived, auto-rotating tokens

Set the mode in `~/.config/aibox/config.yaml`:

```yaml
credentials:
  mode: fallback    # or "vault"
  vault_addr: "https://vault.internal:8200"
```

## Quick start

```bash
# Start a sandbox with your project mounted
aibox start --workspace ~/my-project

# Open a shell inside
aibox shell

# Inside the sandbox, AI tools just work:
#   claude-code, codex, git, build tools — all available
#   API keys injected automatically from your credential store

# When done
aibox stop
```

## Project structure

```
ai-box/
  cmd/
    aibox/                         CLI binary (Go + Cobra)
      cmd/                         Subcommands (start, stop, shell, doctor, auth, policy, ...)
      internal/
        audit/                     Audit event schema, hash chain, file logger
        config/                    Config loading, validation, defaults
        container/                 Container lifecycle (start, stop, shell, status)
        credentials/               Credential broker, Vault, SPIFFE, keychain, file store
        dashboards/                Grafana dashboard and alert rule generation
        doctor/                    System health checks
        dotfiles/                  Developer dotfiles sync into sandbox
        falco/                     Falco eBPF rule management and runtime detection
        host/                      Host detection (Linux, WSL2)
        mcppacks/                  MCP pack manifest, config generator, policy validator
        mounts/                    Filesystem mount layout
        network/                   nftables, Squid, CoreDNS management
        policy/                    OPA engine, merge, toolgate, decision logging
        recording/                 AES-256-GCM encrypted session recording
        security/                  Seccomp, AppArmor, arg validation
        setup/                     Host setup automation, SSH key generation
        siem/                      SIEM detection rules, Vector sinks, MITRE mapping
        storage/                   Immutable append-only log storage
        toolpacks/                 Tool pack manifest, registry, installer, dependency resolver
        vector/                    Vector log collection pipeline
      configs/                     Security profiles (seccomp, AppArmor, Squid, CoreDNS, nftables)
      tests/                       Integration and exit criteria tests
    aibox-credential-helper/       Git credential helper binary
    aibox-git-remote-helper/       Non-blocking git push approval helper
    aibox-llm-proxy/               LLM API proxy sidecar
  aibox-policies/                  Policy-as-code (Rego + YAML)
    org/                           Org baseline (immutable, 34 tests)
    teams/                         Team-level overrides
    project/                       Project-level overrides
  aibox-images/                    Container image definitions (base, java, node, dotnet, full)
  aibox-toolpacks/                 Tool pack definitions (10 packs)
  infra/                           Infrastructure configs (Vault, SPIRE)
  docs/                            Design docs, plans, operations guides
  SPEC-FINAL.md                    Full specification (v1.1)
```

## Testing

Requires Go 1.24+ and the project source.

```bash
# Unit tests (all packages)
cd cmd/aibox
go test ./internal/...

# Integration tests (needs podman + seccomp installed)
go test ./tests/integration/ -v

# OPA policy tests (needs opa CLI)
opa test ../../aibox-policies/org/ -v

# Full suite
go test ./...
```

**Test coverage:** 635 tests across 60 test files (~15K lines of test code), plus 34 OPA policy tests.

## Documentation

| Document | Description |
|----------|-------------|
| [`SPEC-FINAL.md`](SPEC-FINAL.md) | Complete specification v1.1 — architecture, threat model, API |
| [`docs/audit-operations-guide.md`](docs/audit-operations-guide.md) | Audit operations guide with SIEM integration for 5 platforms |
| [`docs/plan/`](docs/plan/) | Phase implementation plans (Phase 0 through Phase 6) |
| [`aibox-policies/org/`](aibox-policies/org/) | Org baseline policies with 34 Rego tests |

## License

Apache 2.0 — see [LICENSE](LICENSE).
