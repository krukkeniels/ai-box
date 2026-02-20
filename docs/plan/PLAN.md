# AI-Box Implementation Plan

**Version**: 2.0
**Date**: 2026-02-19
**Status**: Draft
**Audience**: Implementation teams, project management, engineering leadership

---

## 1. Project Overview

AI-Box is a secure, tool-agnostic development sandbox that enables ~200 developers to use agentic AI tools (Claude Code, Codex CLI, and future agents) for software engineering while preventing code leakage from classified/sensitive environments.

The system runs AI agents inside policy-enforced, Podman-based containers with gVisor isolation, default-deny networking (nftables + Squid proxy + CoreDNS), credential injection via Vault/SPIFFE, and OPA-based policy enforcement. Developers interact through their existing IDEs (VS Code, JetBrains) via SSH remote development. A unified `aibox` CLI abstracts all infrastructure concerns.

Deployment is **local-first**: containers run on developer workstations with minimal central infrastructure (Harbor registry, Nexus mirrors, Vault). A centralized Coder-on-K8s option is available as a future expansion.

> **Pre-Phase-0 Recommendation**: Conduct a stack audit of all target teams (1-2 days) before Phase 0 begins. Survey actual technology stacks, build systems, scripting languages, and dev environments. Size the tool pack backlog from real data, not assumptions.

---

## 2. Implementation Phases

### Phase 0: Infrastructure Foundation

**Scope**: Stand up central infrastructure: container image registry, artifact mirrors, base image build pipeline, and image signing. Includes NuGet mirror validation and the `aibox-dotnet` image variant.

**Spec Sections**: 6 (Deployment Model), 17 (Image Strategy), 23 (Tech Stack Summary)

**Dependencies**: None (foundation phase).

**Estimated Effort**: 4-5 engineer-weeks

**Key Deliverables**:
1. Harbor registry deployed (RBAC, Trivy scanning, air-gapped replication)
2. Nexus mirrors for npm, Maven Central, PyPI, NuGet, Go modules, Cargo -- with explicit `dotnet restore` validation through the NuGet proxy
3. Base image pipeline producing `aibox-base:24.04` from Ubuntu 24.04 LTS
4. Image variants: `aibox-java:21-24.04`, `aibox-node:20-24.04`, `aibox-dotnet:8-24.04`, `aibox-full:24.04`
5. Cosign signing integrated into the pipeline; Podman client-side policy rejects unsigned images
6. Automated weekly rebuild pipeline with date-tagged images

**Exit Criteria**:
- Harbor reachable; images push/pull with signature verification
- Nexus proxies npm, Maven Central, PyPI, and NuGet successfully (`dotnet restore` validated)
- Base image passes Trivy scan with zero critical/high CVEs
- Weekly rebuild runs unattended, producing signed images

---

### Phase 1: Core Runtime & CLI

**Scope**: Build the container runtime and `aibox` CLI. Produce a working sandbox that can start, stop, and provide a shell -- without network security or credential injection. Includes expanded gVisor compatibility testing for polyglot stacks and WSL2 validation as an explicit deliverable (not just a risk item).

**Spec Sections**: 5.1, 5.6, 7, 9, 10, 18, Appendix A, Appendix B

**Dependencies**: Phase 0.

**Estimated Effort**: 6-8 engineer-weeks

**Key Deliverables**:
1. `aibox` CLI binary (Go or Rust): `setup`, `start`, `stop`, `shell`, `status`, `update`, `doctor`
2. Podman + gVisor integration (`--runtime=runsc`)
3. Seccomp profile (spec Section 9.2 allowlist) and AppArmor profile (`aibox-sandbox`)
4. Mandatory security flags: cap-drop=ALL, no-new-privileges, read-only rootfs, non-root user
5. Filesystem mount layout: read-only root, writable `/workspace`, persistent home volume, build caches, tmpfs `/tmp`
6. **WSL2 deliverable**: `aibox setup` automates WSL2 configuration on Windows 11; validated with full polyglot stack (not just Java/Node). Known-issues registry maintained from this phase forward.
7. **gVisor compatibility matrix** validated for:

   | Component | Risk Level |
   |-----------|------------|
   | .NET CLR (memory-mapped files, JIT) | High |
   | Bazel sandbox-in-gVisor-sandbox | High |
   | Combined .NET + JVM + Node under gVisor | High |
   | PowerShell Core (CLR-based syscall surface) | Medium |
   | Gradle daemon (long-lived JVM, file watchers) | Medium |

8. `aibox doctor` health checks (Podman, gVisor, WSL2 memory, image signature)
9. Cold start < 90s, warm start < 15s benchmarks validated

**Exit Criteria**:
- `aibox setup && aibox start --workspace ~/project` produces a running gVisor-isolated container
- Security settings verified: no capabilities, no privilege escalation, seccomp active, AppArmor loaded
- Build caches persist across stop/start cycles
- Works on native Linux and Windows 11 + WSL2 with polyglot stack
- gVisor compatibility matrix documented; `runc` fallback tested with compensating controls for any failures

---

### Phase 2: Network Security

**Scope**: Implement host-level network security to prevent data exfiltration. This is the core security differentiator: enforcement outside the container that cannot be bypassed.

**Spec Sections**: 5.3, 8 (all subsections)

**Dependencies**: Phases 0, 1.

**Estimated Effort**: 5-6 engineer-weeks

**Key Deliverables**:
1. nftables ruleset: container traffic allowed only to proxy + DNS, all else dropped
2. Squid proxy with domain allowlist (Harbor, Nexus, Git, LLM gateway)
3. CoreDNS: allowlist-only resolution, NXDOMAIN for all else, query logging
4. DNS tunneling mitigations: block TXT/NULL/CNAME, rate limiting, entropy monitoring
5. LLM API sidecar proxy: credential injection, payload logging, rate limiting (60 req/min, 100K tokens/min)
6. Package manager proxy: npm/Maven/pip/NuGet/etc. routed through Nexus via Squid
7. `aibox network test` command
8. Anti-bypass: block DoH (443 to known resolvers), block DoT (853)

**Exit Criteria**:
- `curl https://google.com` fails from inside the container
- `git clone` from `git.internal` succeeds
- Non-allowlisted DNS returns NXDOMAIN
- LLM API calls via sidecar succeed; agent cannot read the API key
- `npm install` / `gradle build` / `dotnet restore` succeed via Nexus
- nftables rules survive container restart; cannot be modified from inside

---

### Phase 3: Policy Engine & Credentials

**Scope**: OPA-based policy engine for declarative security enforcement and Vault/SPIFFE credential management for short-lived, scoped secrets.

**Spec Sections**: 5.2, 11, 12

**Dependencies**: Phases 1, 2.

**Estimated Effort**: 6-7 engineer-weeks

**Key Deliverables**:
1. OPA integration: policies loaded at container start, evaluated on tool invocations and network requests
2. Rego policy library: org baseline rules (deny wildcards, require gVisor, require rate limiting)
3. Policy hierarchy: org baseline (immutable) > team > project; tighten-only merge
4. `aibox policy validate` and `aibox policy explain --log-entry <id>` commands
5. Decision logging: every OPA evaluation recorded with input, decision, reason
6. Tool permission model: `safe`, `review-required`, `blocked-by-default` risk classes
7. SPIRE server + agent for workload identity attestation
8. Vault integration: dynamic secrets for Git tokens (4h TTL), LLM API keys (8h TTL), package mirror tokens (8h TTL)
9. `aibox-credential-helper` for Git HTTPS auth via Vault
10. Token lifecycle: mint on start, revoke on stop, prevent persistence to workspace
11. Simplified credential broker fallback (env var injection) for orgs without Vault

**Exit Criteria**:
- Policy that loosens org baseline is rejected by `aibox policy validate`
- `blocked-by-default` tools denied with clear explanation
- `review-required` tools generate audit entries and approval notifications
- Git operations use short-lived Vault tokens; tokens revoked within seconds of `aibox stop`
- Graceful degradation if Vault is temporarily unreachable

---

### Phase 4: Developer Experience

**Scope**: Make the sandbox feel like a better development environment. IDE integration, tool packs (including polyglot packs), MCP packs, dotfiles, shell customization, and `git push` approval flow. Includes custom CLI migration documentation.

**Spec Sections**: 13, 14, 15, 16, 18

**Dependencies**: Phases 1, 2, 3.

**Estimated Effort**: 8-10 engineer-weeks

**Key Deliverables**:
1. VS Code Remote SSH and JetBrains Gateway integration
2. SSH server configuration (container :22 mapped to host :2222)
3. Tool pack system: manifest schema, `aibox install <pack>`, runtime installation
4. Initial tool packs: `java@21`, `node@20`, `python@3.12`, `bazel@7`, `scala@3`, `angular@18`, **`dotnet@8`**, **`powershell@7`**, **`angularjs@1`**, `ai-tools`
5. Pre-built image variants for common stack combinations
6. MCP pack system: `aibox mcp enable/list`, auto-generated config for agent discovery
7. Initial MCP packs: `filesystem-mcp`, `git-mcp`
8. Dotfiles sync, shell setup (bash + zsh + pwsh + tmux), persistent history
9. `git push` non-blocking approval flow (staging ref, webhook, async approve/reject)
10. **Polyglot resource profile** documented and validated: 8 cores, 16-24 GB RAM, 80 GB disk for .NET + JVM + Node + Bazel + AI agent
11. **Custom CLI migration docs**: inventory, wrapper patterns, gradual migration path
12. Debugging support: port forwarding, debug adapters, hot reload

**Exit Criteria**:
- VS Code and JetBrains connect to sandbox with full IDE functionality
- `aibox install dotnet@8` adds .NET SDK to a running container; `dotnet build` works
- Claude Code and Codex CLI work with auto-injected API keys
- MCP servers discoverable by AI agents
- Dotfiles and shell history persist across sessions
- `git push` with `review-required` creates staging ref without blocking developer
- Build performance within 20% of local baseline

---

### Phase 5: Audit, Monitoring & Compliance

**Scope**: Logging pipeline, runtime security monitoring, SIEM integration, and session recording for classified environment compliance.

**Spec Sections**: 19, 20, 24

**Dependencies**: Phases 1, 2, 3.

**Estimated Effort**: 4-5 engineer-weeks

**Key Deliverables**:
1. Log aggregation pipeline (Vector/Fluentd) shipping to central store
2. Event logging for all categories: lifecycle, network, DNS, tool invocations, credentials, policy decisions, LLM traffic, file access
3. Immutable log storage: append-only with cryptographic hash chain
4. Falco on dev machines: container escape detection, unexpected network, privilege escalation
5. SIEM integration with detection rules for anomalous patterns
6. Session recording (optional): terminal I/O capture, encrypted storage, playback
7. Audit dashboards and log retention enforcement (2+ years lifecycle/policy, 1+ year network/LLM)

**Exit Criteria**:
- All event categories from spec Section 19.1 captured and shipped
- Logs tamper-evident and append-only
- Falco alerts on simulated escape and privilege escalation
- SIEM rules trigger on simulated anomalous patterns
- Log retention policies configured and automated

---

### Phase 6: Rollout & Operations

**Scope**: Phased rollout from pilot through GA, training materials, champions program, and day-2 operations. Includes polyglot stack rollout testing and Windows-specific rollout procedures.

**Spec Sections**: 21, 22

**Dependencies**: Phases 0-4 complete (Phase 5 can proceed in parallel with early rollout).

**Estimated Effort**: 9-11 engineer-weeks (spread over 15-22+ weeks calendar time)

**Key Deliverables**:
1. Pilot (10 devs): pair with platform engineers, daily feedback, fix top 5 pain points
2. Early Adopters (30-40 devs): self-service onboarding, champions program launched
3. Champions program: 1 per team (15-20 total), early access, direct Slack channel
4. Training materials: "AI-Box in 5 minutes" screencast, VS Code/IntelliJ quickstarts, troubleshooting FAQ, tool pack authoring guide
5. General rollout: new projects default to AI-Box, team-by-team migration, office hours 3x/week
6. **Windows-specific rollout procedures**: WSL2 setup validation, `.wslconfig` guidance per workload profile, polyglot stack testing on Windows
7. Support model: Tier 0 self-service, Tier 1 champions, Tier 2 platform team, Tier 3 escalation
8. Day-2 runbooks: image patching, CVE triage, policy updates, tool pack updates
9. Disaster recovery procedures and operational KPIs

**Exit Criteria**:
- Pilot: 8/10 developers rate experience "acceptable or better"
- Early Adopters: < 3 support tickets/week, startup time < 90s at p95
- General Rollout: > 90% of active developers using AI-Box
- Champions program active with at least 1 champion per team
- Day-2 runbooks tested via tabletop exercises

---

## 3. Dependency Graph

```
Phase 0: Infrastructure Foundation
   |
   v
Phase 1: Core Runtime & CLI
   |
   v
Phase 2: Network Security --------+
   |                               |
   v                               |
Phase 3: Policy Engine & Creds ---+---> Phase 4: Developer Experience
   |                               |         |
   |                               |         v
   +-------------------------------+--> Phase 6: Rollout & Operations
   |
   v
Phase 5: Audit, Monitoring & Compliance
   |
   +---> (feeds into Phase 6, can run in parallel with Phase 4)
```

**Dependency edges**:

```
0 --> 1         (images must exist before runtime can pull them)
1 --> 2         (container must run before network controls are meaningful)
0 --> 2         (Nexus must exist for package proxying)
1 --> 3         (CLI and runtime needed for policy enforcement)
2 --> 3         (network layer needed for Vault communication)
1 --> 4         (runtime needed for IDE connection)
2 --> 4         (network needed for tool downloads)
3 --> 4         (credentials needed for AI tool API keys)
1 --> 5         (container events to log)
2 --> 5         (network/proxy/DNS logs to collect)
3 --> 5         (policy decisions and credential events to log)
0-4 --> 6       (all functional phases before rollout)
5 ||--> 6       (audit can run in parallel with early rollout)
```

---

## 4. Critical Path

**Phase 0 --> Phase 1 --> Phase 2 --> Phase 3 --> Phase 4 --> Phase 6**

- **Phase 0** blocks everything. No images, no containers.
- **Phase 1** is the next bottleneck. Nothing else can be tested without a working container and CLI.
- **Phase 2** is the core security value proposition. Without host-level network enforcement, AI-Box doesn't meet its exfiltration-prevention goal.
- **Phase 3** enables "invisible security." Without short-lived credentials and policy enforcement, the system is either broken or insecure.
- **Phase 4** is the adoption gate. Without IDE integration and tool packs, developers will reject the system.
- **Phase 6** delivers value to the organization.

**Phase 5** is not on the critical path -- the system is functional and secure without centralized audit. It can proceed in parallel with Phases 4 and early Phase 6. However, for classified environments, audit may be a hard requirement before rollout.

**Parallelization**:
- Phase 5 can begin after Phase 2 completes.
- Within Phase 4, IDE integration and tool pack development can run in parallel.
- Phase 6 pilot can begin once Phases 0-4 are complete, even if Phase 5 is in progress.

---

## 5. Cross-Cutting Concerns

### Testing Strategy

| Test Type | Scope | Automation |
|-----------|-------|-----------|
| Unit | CLI commands, policy evaluation, credential helper | CI on every commit |
| Integration | Container launch + gVisor, proxy + DNS, Vault + SPIRE | CI on merge to main |
| Security | Escape attempts, network bypass, privilege escalation | Weekly + pre-release |
| Performance | Cold/warm start time, build performance, LLM proxy latency | Weekly |
| Compatibility | Windows 11 + WSL2, native Linux, VS Code, JetBrains, polyglot stack | Monthly test matrix |
| End-to-end | Full developer workflow: start -> code -> build -> push | Pre-release gate |

A security test suite must actively attempt to bypass every control (DNS exfiltration, container escape, `/proc` credential read, etc.). This suite runs on every release.

### Documentation

Documentation is a deliverable in every phase:

- **Phase 0**: Infrastructure setup guide, image build pipeline docs
- **Phase 1**: CLI reference, developer quickstart, `aibox setup` guide, WSL2 known-issues registry
- **Phase 2**: Network architecture diagram, proxy configuration reference
- **Phase 3**: Policy authoring guide, credential management overview, Rego rule reference
- **Phase 4**: IDE setup guides, tool pack authoring guide, MCP pack guide, custom CLI migration guide
- **Phase 5**: Audit log schema reference, SIEM integration guide, incident response playbook
- **Phase 6**: Training materials, FAQ, champions handbook, operations runbooks

### Security Review

Each phase undergoes security review before the next begins:

- **Phase 0**: Image supply chain (signing, scanning, provenance)
- **Phase 1**: Container isolation (seccomp audit, AppArmor audit, gVisor config)
- **Phase 2**: Network security (egress pen-test, DNS tunneling test, proxy bypass test)
- **Phase 3**: Credential management (token lifecycle, Vault config, SPIFFE trust domain)
- **Phase 4**: Developer workflow (IDE extension audit, tool pack supply chain, MCP permissions)
- **Phase 5**: Audit completeness (event coverage, tamper resistance, retention compliance)

### Configuration Management

All configuration is version-controlled, signed, and reproducible:

- Infrastructure-as-code for Harbor, Nexus, Vault (Terraform or Ansible)
- Policy-as-code in a dedicated `aibox-policies` Git repository
- Image definitions in `aibox-images`, tool pack manifests in `aibox-toolpacks`
- All repos require signed commits and PR review

---

## 6. Risk Register

| # | Risk | Likelihood | Impact | Phase | Mitigation |
|---|------|-----------|--------|-------|-----------|
| R1 | gVisor compatibility breaks developer tooling | Medium | High | 1 | Expanded testing matrix: JVM, **.NET CLR**, **Bazel sandbox-in-sandbox**, **PowerShell Core**, combined polyglot stack. Maintain `runc` escape hatch with compensating controls. |
| R2 | WSL2 + Podman + gVisor instability on Windows | **High** | High | 1 | **100% of target team is on Windows -- this is the primary deployment path, not an edge case.** Dedicated WSL2 validation sprint with polyglot stack. Known-issues registry. Hyper-V backend as fallback. |
| R3 | Network controls cause false-positive blocks | High | Medium | 2 | `aibox policy explain` for clear errors. Easy allowlist request process (< 1 day turnaround). |
| R4 | LLM sidecar proxy adds unacceptable latency | Low | High | 2 | Benchmark early. Sidecar is localhost-only. Target < 50ms overhead. |
| R5 | Vault/SPIRE complexity delays Phase 3 | Medium | Medium | 3 | Implement simplified credential broker first. Vault as enhancement. |
| R6 | Developer adoption resistance | High | High | 4, 6 | Invest heavily in DX. Warm start < 15s. Build cache persistence. Champions program. Fix pain points before expanding. |
| R7 | Tool pack maintenance unsustainable | Medium | Medium | 4 | Automate updates. Clear governance. Self-service with guardrails. |
| R8 | Compliance requirements not met by audit system | Medium | High | 5 | Engage compliance team in Phase 0. Map controls to compliance framework. |
| R9 | Central infrastructure SPOF (Harbor, Nexus) | Low | High | 0 | Local image caching. Graceful degradation. HA deployment. |
| R10 | Scope creep | High | Medium | All | Strict change control. Spec amendment process. |
| **R11** | **Polyglot resource pressure (OOM)** | **Medium** | **High** | **1, 4** | **.NET + JVM + Node + Bazel + AI agent can exceed 16 GB simultaneously. Polyglot resource profile (24 GB RAM). `.wslconfig` guidance. `aibox doctor` memory checks.** |

---

## 7. Phase Summary Table

| Phase | Name | Dependencies | Effort | Team Size | Calendar | Status |
|-------|------|-------------|--------|-----------|----------|--------|
| -- | Stack Audit (pre-Phase-0) | None | 1-2 days | 1-2 engineers | Before Week 1 | Not Started |
| 0 | Infrastructure Foundation | None | 4-5 eng-weeks | 2 engineers | Weeks 1-3 | Not Started |
| 1 | Core Runtime & CLI | Phase 0 | 6-8 eng-weeks | 2-3 engineers | Weeks 3-6 | Not Started |
| 2 | Network Security | Phases 0, 1 | 5-6 eng-weeks | 2 engineers | Weeks 6-9 | Not Started |
| 3 | Policy Engine & Credentials | Phases 1, 2 | 6-7 eng-weeks | 2-3 engineers | Weeks 9-12 | Not Started |
| 4 | Developer Experience | Phases 1, 2, 3 | 8-10 eng-weeks | 3 engineers | Weeks 12-16 | Not Started |
| 5 | Audit, Monitoring & Compliance | Phases 1, 2, 3 | 4-5 eng-weeks | 2 engineers | Weeks 9-16 (parallel) | Not Started |
| 6 | Rollout & Operations | Phases 0-4 | 9-11 eng-weeks | 2-3 engineers | Weeks 17-25+ | Not Started |
| **Total** | | | **42-52 eng-weeks** | **3-5 engineers** | **~24-25 weeks** | |

**Notes**:
- Calendar estimates assume 3-5 engineers working in parallel where dependencies allow.
- Phase 5 runs in parallel with Phases 3-4, not sequentially.
- Phase 6 calendar time is longer because it involves organizational change, not just engineering.
- Total of 42-52 engineer-weeks reflects polyglot stack support (NuGet validation, .NET/PowerShell/AngularJS tool packs, expanded gVisor testing, deeper WSL2 validation).
