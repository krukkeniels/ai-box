# AI-Box Implementation Plan

**Version**: 1.0
**Date**: 2026-02-18
**Status**: Draft
**Audience**: Implementation teams, project management, engineering leadership

---

## 1. Project Overview

AI-Box is a secure, tool-agnostic development sandbox that enables ~200 developers to use agentic AI tools (Claude Code, Codex CLI, and future agents) for software engineering while preventing code leakage from classified/sensitive environments.

The system runs AI agents inside policy-enforced, Podman-based containers with gVisor isolation, default-deny networking (nftables + Squid proxy + CoreDNS), credential injection via Vault/SPIFFE, and OPA-based policy enforcement. Developers interact through their existing IDEs (VS Code, JetBrains) via SSH remote development, and a unified `aibox` CLI abstracts all infrastructure concerns.

The primary deployment model is **local-first**: containers run on developer workstations with minimal central infrastructure (Harbor registry, Nexus mirrors, Vault). A centralized Coder-on-K8s option is available as a future Phase 2 expansion.

This plan covers the full build-out from infrastructure foundation through general rollout, organized into seven phases (0-6).

---

## 2. Implementation Phases

### Phase 0: Infrastructure Foundation

**Scope**: Stand up the central infrastructure services that all subsequent phases depend on. This includes the container image registry, artifact mirrors, the base image build pipeline, and image signing.

**Spec Sections Covered**:
- Section 6 (Deployment Model) -- central infrastructure sizing and components
- Section 17 (Image Strategy) -- Harbor, base image, Cosign signing, update cadence
- Section 23 (Tech Stack Summary) -- Harbor, Cosign, Nexus, Buildah, Trivy

**Dependencies**: None (this is the foundation).

**Estimated Effort**: 3-4 engineer-weeks

**Key Deliverables**:
1. Harbor registry deployed and configured (RBAC, Trivy scanning enabled, replication for air-gapped sites)
2. Nexus Repository configured as mirror for npm, Maven Central, PyPI, NuGet, Go modules, Cargo
3. Base image build pipeline (CI/CD) producing `aibox-base:24.04` from Ubuntu 24.04 LTS
4. Cosign key generation and image signing integrated into the build pipeline
5. Podman client-side policy (`/etc/containers/policy.json`) to reject unsigned images
6. Automated weekly rebuild pipeline with date-tagged images
7. Image variant builds for `aibox-java:21-24.04`, `aibox-node:20-24.04`, `aibox-full:24.04`

**Exit Criteria**:
- Harbor is reachable, images can be pushed and pulled with signature verification
- Nexus proxies at least npm, Maven Central, and PyPI successfully
- Base image builds pass Trivy scan with zero critical/high CVEs
- `podman pull harbor.internal/aibox/base:24.04` succeeds and signature is verified
- Weekly rebuild pipeline runs unattended and produces signed, tagged images

---

### Phase 1: Core Runtime & CLI

**Scope**: Build the container runtime environment and the `aibox` CLI that developers use daily. This phase produces a working sandbox that can start, stop, and provide a shell -- without network security or credential injection yet.

**Spec Sections Covered**:
- Section 5.1 (Runtime Sandbox) -- Podman + gVisor, read-only rootfs, volumes
- Section 5.6 (CLI) -- `aibox` commands
- Section 7 (Container Runtime) -- Podman rootless, gVisor, Docker fallback, Windows/WSL2
- Section 9 (Container Isolation) -- gVisor, seccomp, AppArmor, mandatory security settings
- Section 10 (Filesystem Controls) -- mount layout, denied paths, build cache persistence
- Section 18 (Developer Experience) -- performance SLAs, resource allocation
- Appendix A (CLI Reference)
- Appendix B (Developer Quickstart)

**Dependencies**: Phase 0 (images must be pullable from Harbor).

**Estimated Effort**: 5-6 engineer-weeks

**Key Deliverables**:
1. `aibox` CLI binary (Go or Rust) with commands: `setup`, `start`, `stop`, `shell`, `status`, `update`, `doctor`
2. Podman + gVisor integration: containers launch with `--runtime=runsc`
3. Seccomp profile (`/etc/aibox/seccomp.json`) implementing the allowlist from spec Section 9.2
4. AppArmor profile (`aibox-sandbox`) per spec Section 9.3
5. Mandatory security flags applied on every launch (cap-drop=ALL, no-new-privileges, read-only rootfs, non-root user)
6. Filesystem mount layout: read-only root, writable `/workspace`, persistent home volume, persistent build caches, tmpfs `/tmp`
7. WSL2 detection and setup automation (`aibox setup` on Windows 11)
8. `aibox doctor` health checks (Podman, gVisor, WSL2 memory, image signature)
9. Cold start < 90s, warm start < 15s benchmarks validated

**Exit Criteria**:
- `aibox setup && aibox start --workspace ~/project` produces a running gVisor-isolated container
- `aibox shell` drops into a shell inside the container with correct mount layout
- Security settings verified: no capabilities, no privilege escalation, seccomp active, AppArmor loaded
- Build caches persist across `aibox stop && aibox start` cycles
- `aibox doctor` reports all-green on a correctly configured machine
- Works on both native Linux and Windows 11 + WSL2

---

### Phase 2: Network Security

**Scope**: Implement the full network security stack that prevents data exfiltration. This is the core security differentiator of AI-Box: host-level enforcement that the container cannot bypass.

**Spec Sections Covered**:
- Section 5.3 (Connectivity Layer) -- Squid, nftables, CoreDNS, LLM sidecar
- Section 8 (Network Security) -- all subsections (8.1-8.6)
- Section 8.5 (LLM API Sidecar Proxy) -- credential injection, payload logging, rate limiting
- Section 8.6 (Package Manager Proxying) -- Nexus integration

**Dependencies**: Phase 0 (Nexus for package proxying), Phase 1 (running container to route traffic through).

**Estimated Effort**: 5-6 engineer-weeks

**Key Deliverables**:
1. nftables ruleset installed by `aibox setup`: container traffic allowed only to proxy + DNS, all else dropped
2. Squid proxy configuration with domain allowlist (Harbor, Nexus, Git, LLM gateway)
3. CoreDNS configuration: allowlist-only resolution, NXDOMAIN for everything else, query logging
4. DNS tunneling mitigations: block TXT/NULL/CNAME, rate limiting, entropy monitoring
5. LLM API sidecar proxy (`aibox-llm-proxy`): credential injection, payload logging, rate limiting (60 req/min, 100K tokens/min)
6. Package manager proxy configuration: npm/Maven/pip/etc. routed through Nexus via Squid
7. `aibox network test` command for connectivity verification
8. Anti-bypass protections: block DoH (443 to known resolvers), block DoT (853)

**Exit Criteria**:
- From inside the container, `curl https://google.com` fails (blocked)
- From inside the container, `git clone` from `git.internal` succeeds (allowed)
- DNS resolution for non-allowlisted domains returns NXDOMAIN
- LLM API calls via sidecar proxy succeed; agent process cannot read the API key
- `npm install` / `gradle build` succeed via Nexus mirror
- nftables rules survive container restart and cannot be modified from inside the container
- Payload logging captures full LLM API request/response bodies

---

### Phase 3: Policy Engine & Credentials

**Scope**: Implement the OPA-based policy engine for declarative security enforcement and the Vault/SPIFFE credential management system for short-lived, scoped secrets.

**Spec Sections Covered**:
- Section 5.2 (Policy Engine) -- OPA/Rego, policy hierarchy, decision logging
- Section 11 (Credential Management) -- Vault, SPIFFE/SPIRE, credential types, Git auth, LLM key injection, token lifecycle
- Section 12 (Policy Engine) -- policy hierarchy, policy spec, OPA validation, tool permission model

**Dependencies**: Phase 1 (CLI and container runtime), Phase 2 (network layer for credential fetching and policy distribution).

**Estimated Effort**: 6-7 engineer-weeks

**Key Deliverables**:
1. OPA integration: policies loaded at container start, evaluated on tool invocations and network requests
2. Rego policy library: org baseline rules (deny wildcards, require gVisor, require rate limiting)
3. Policy hierarchy enforcement: org baseline (immutable) > team policy > project policy; tighten-only merge
4. `aibox policy validate` command
5. `aibox policy explain --log-entry <id>` command for developer-friendly policy explanations
6. Decision logging: every OPA evaluation recorded with input, decision, and reason
7. Tool permission model: `safe`, `review-required`, `blocked-by-default` risk classes enforced
8. SPIRE server + agent deployment for workload identity attestation
9. Vault integration: dynamic secret generation for Git tokens (4h TTL), LLM API keys (8h TTL), package mirror tokens (8h TTL)
10. `aibox-credential-helper` for Git HTTPS authentication via Vault
11. Token lifecycle management: mint on start, revoke on stop, prevent persistence to workspace
12. Simplified credential broker fallback (env var injection) for orgs without Vault

**Exit Criteria**:
- A project `policy.yaml` that attempts to loosen the org baseline is rejected by `aibox policy validate`
- Tool invocations matching `blocked-by-default` are denied with a clear explanation
- `review-required` tools (e.g., `git push`) generate audit entries and approval notifications
- Git operations inside the container use short-lived tokens from Vault (no static credentials)
- Tokens are revoked within seconds of `aibox stop`
- Decision logs are queryable and contain full evaluation context
- System degrades gracefully if Vault is temporarily unreachable (cached creds valid until TTL)

---

### Phase 4: Developer Experience

**Scope**: Make the sandbox feel like a better development environment, not a restricted one. This phase covers IDE integration, tool packs, MCP packs, dotfiles, shell customization, and the `git push` approval flow.

**Spec Sections Covered**:
- Section 13 (IDE Integration) -- VS Code Remote SSH, JetBrains Gateway, debugging, shell/dotfiles
- Section 14 (AI Tool Integration) -- Claude Code, Codex CLI, future agents, MCP discovery
- Section 15 (Tool Packs) -- design, manifest schema, initial packs, governance
- Section 16 (MCP Packs) -- design, enabling MCP packs
- Section 18 (Developer Experience) -- performance SLAs, shell/dotfiles, git push approval flow

**Dependencies**: Phase 1 (CLI/runtime), Phase 2 (network for IDE connections and tool downloads), Phase 3 (credentials for AI tool API keys).

**Estimated Effort**: 6-7 engineer-weeks

**Key Deliverables**:
1. VS Code Remote SSH integration: pre-installed VS Code Server in base image, auto-connect configuration
2. JetBrains Gateway integration: SSH-based backend connection, resource guidance
3. SSH server configuration in container (port 22, mapped to host 2222)
4. Tool pack system: manifest schema, `aibox install <pack>` command, runtime installation
5. Initial tool packs: `java@21`, `node@20`, `python@3.12`, `bazel@7`, `scala@3`, `angular@18`, `ai-tools`
6. Pre-built image variants for common stack combinations
7. MCP pack system: `aibox mcp enable/list` commands, auto-generated MCP config for agent discovery
8. Initial MCP packs: `filesystem-mcp`, `git-mcp`
9. Dotfiles sync: `aibox` clones configured dotfiles repo into persistent home volume
10. Shell setup: bash + zsh + tmux, persistent history
11. `git push` non-blocking approval flow (staging ref, webhook notification, async approve/reject)
12. Debugging support: port forwarding, debug adapter configuration, hot reload

**Exit Criteria**:
- VS Code connects to the sandbox via Remote SSH and provides full IDE functionality (editing, terminal, debugging, extensions)
- JetBrains Gateway connects and runs backend inside the container
- `aibox install java@21` adds JDK, Maven, and Gradle to a running container
- Claude Code and Codex CLI work inside the sandbox with auto-injected API keys
- MCP servers are discoverable by AI agents via auto-generated config
- Dotfiles and shell history persist across sessions
- `git push` with `review-required` policy creates staging ref and notifies approver without blocking the developer
- Build performance within 20% of local baseline

---

### Phase 5: Audit, Monitoring & Compliance

**Scope**: Build the logging pipeline, runtime security monitoring, SIEM integration, and session recording capabilities needed for classified environment compliance.

**Spec Sections Covered**:
- Section 19 (Audit and Compliance) -- event categories, immutability, session recording, Falco, SIEM
- Section 20 (Threat Model) -- detection controls for residual risks
- Section 24 (Residual Risks) -- monitoring-based mitigations

**Dependencies**: Phase 1 (container lifecycle events), Phase 2 (network/DNS/proxy logs), Phase 3 (policy decision logs, credential access logs).

**Estimated Effort**: 4-5 engineer-weeks

**Key Deliverables**:
1. Log aggregation pipeline: Vector/Fluentd collectors on dev machines shipping to central store
2. Event logging for all categories: sandbox lifecycle, network connections, DNS queries, tool invocations, credential access, policy decisions, LLM API traffic, file access
3. Immutable log storage: append-only with cryptographic hash chain (S3 Object Lock or MinIO WORM)
4. Falco deployment on dev machines: rules for container escape detection, unexpected network connections, privilege escalation, sensitive file access
5. SIEM integration: detection rules for anomalous outbound volume, DNS spikes, off-hours credential access, repeated blocked attempts, LLM payload size anomalies
6. Session recording (optional): terminal I/O capture via `script` wrapper, encrypted storage, playback capability
7. Audit dashboards: sandbox usage metrics, policy violation trends, security event timeline
8. Log retention enforcement: 2+ years for lifecycle/tool/credential/policy events, 1+ year for network/DNS/LLM/file events

**Exit Criteria**:
- All event categories from spec Section 19.1 are captured and shipped to central store
- Logs are tamper-evident (hash chain verified) and append-only
- Falco alerts fire on simulated container escape and privilege escalation attempts
- SIEM rules trigger on simulated anomalous patterns (bulk data transfer, DNS tunneling attempt)
- Session recordings can be played back for incident investigation
- Dashboards show real-time sandbox fleet health and security posture
- Log retention policies are configured and automated

---

### Phase 6: Rollout & Operations

**Scope**: Execute the phased rollout from pilot through general availability, build training materials, establish the champions program, and set up day-2 operations.

**Spec Sections Covered**:
- Section 21 (Transition and Rollout Plan) -- phased rollout, training, champions, support model, fallback
- Section 22 (Operations) -- day-2 ops, image updates, `aibox doctor`, disaster recovery

**Dependencies**: Phases 0-4 complete (Phase 5 can proceed in parallel with early rollout stages).

**Estimated Effort**: 8-10 engineer-weeks (spread over 15-22 weeks calendar time)

**Key Deliverables**:
1. Pilot (10 developers): selection, pairing with platform engineers, daily feedback surveys, fix top 5 pain points
2. Early Adopter program (30-40 developers): self-service onboarding, volunteer + nominated leads
3. Champions program: 1 per team (15-20 total), early access, direct Slack channel, first-line support
4. Training materials: "AI-Box in 5 minutes" screencast, VS Code quickstart, IntelliJ quickstart, troubleshooting FAQ, tool pack authoring guide, architecture overview
5. General rollout: all new projects default to AI-Box, team-by-team migration, migration office hours 3x/week
6. Support model: Tier 0 self-service (docs + `aibox doctor`), Tier 1 champions, Tier 2 platform team, Tier 3 escalation
7. Day-2 operations runbooks: image patching, CVE triage, policy updates, tool pack updates, compatibility testing
8. Disaster recovery procedures: dev machine re-image, Harbor/Nexus/Vault failure recovery
9. Metrics and KPIs: adoption rate, startup time p95, support ticket volume, fallback frequency, security event count

**Exit Criteria**:
- Pilot: 8/10 developers rate experience "acceptable or better"
- Early Adopters: < 3 support tickets/week, startup time < 90s at p95
- General Rollout: > 90% of active developers using AI-Box
- Champions program active with at least 1 champion per team
- All training materials published and reviewed
- Day-2 runbooks tested via tabletop exercises
- Fallback to local dev remains available during rollout phases

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

**Detailed dependency edges**:

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

The critical path runs through:

**Phase 0 --> Phase 1 --> Phase 2 --> Phase 3 --> Phase 4 --> Phase 6**

This is the critical path because:

1. **Phase 0 (Infrastructure)** blocks everything. No images, no containers. This is the absolute first thing that must ship.

2. **Phase 1 (Runtime & CLI)** is the next bottleneck. Nothing else can be tested or developed without a working container and CLI. The CLI is also the primary developer interface for all subsequent phases.

3. **Phase 2 (Network Security)** is the core security value proposition. Without host-level network enforcement, AI-Box does not meet its primary goal of preventing exfiltration. This also blocks credential management (Vault needs network to reach).

4. **Phase 3 (Policy & Credentials)** enables the "invisible security" principle. Without short-lived credentials and policy enforcement, the system either has no credentials (broken) or static credentials (insecure).

5. **Phase 4 (Developer Experience)** is the adoption gate. Without IDE integration, tool packs, and a polished workflow, developers will reject the system regardless of its security properties.

6. **Phase 6 (Rollout)** is the final gate to value delivery.

**Phase 5 (Audit)** is important but not on the critical path because the system is functional and secure without centralized audit. Audit can proceed in parallel with Phase 4 and early Phase 6 stages. However, for classified environments, audit capabilities may be a hard requirement before rollout, which would make Phase 5 critical-path.

**Parallelization opportunities**:
- Phase 5 can begin as soon as Phase 2 completes (network logs available), and continue in parallel with Phases 3-4.
- Within Phase 4, IDE integration and tool pack development can run in parallel.
- Phase 6 pilot can begin once Phases 0-4 are complete, even if Phase 5 is still in progress.

---

## 5. Cross-Cutting Concerns

### Testing Strategy

Testing spans all phases and must be established early (Phase 0/1):

| Test Type | Scope | Automation |
|-----------|-------|-----------|
| **Unit tests** | CLI commands, policy evaluation, credential helper | CI on every commit |
| **Integration tests** | Container launch + gVisor, proxy + DNS, Vault + SPIRE | CI on merge to main |
| **Security tests** | Escape attempts, network bypass attempts, privilege escalation | Weekly + pre-release |
| **Performance tests** | Cold/warm start time, build performance, LLM proxy latency | Weekly |
| **Compatibility tests** | Windows 11 + WSL2, native Linux, VS Code, JetBrains | Monthly test matrix |
| **End-to-end tests** | Full developer workflow: start -> code -> build -> push | Pre-release gate |

A security test suite should be written that actively attempts to bypass every control (exfiltrate data via DNS, escape container, read credentials from `/proc`, etc.). This suite runs on every release.

### Documentation

Documentation is a deliverable in every phase, not a follow-up activity:

- **Phase 0**: Infrastructure setup guide, image build pipeline docs
- **Phase 1**: CLI reference, developer quickstart, `aibox setup` guide
- **Phase 2**: Network architecture diagram, proxy configuration reference, troubleshooting network issues
- **Phase 3**: Policy authoring guide, credential management overview, Rego rule reference
- **Phase 4**: IDE setup guides (VS Code, JetBrains), tool pack authoring guide, MCP pack guide
- **Phase 5**: Audit log schema reference, SIEM integration guide, incident response playbook
- **Phase 6**: Training materials, FAQ, champions handbook, operations runbooks

### Security Review

Each phase undergoes security review before merging to the next:

- **Phase 0**: Image supply chain review (signing, scanning, provenance)
- **Phase 1**: Container isolation review (seccomp profile audit, AppArmor policy audit, gVisor configuration)
- **Phase 2**: Network security review (penetration test of egress controls, DNS tunneling test, proxy bypass test)
- **Phase 3**: Credential management review (token lifecycle, Vault configuration, SPIFFE trust domain)
- **Phase 4**: Developer workflow review (IDE extension audit, tool pack supply chain, MCP permission model)
- **Phase 5**: Audit completeness review (event coverage, tamper resistance, retention compliance)

### Configuration Management

All configuration is version-controlled, signed, and reproducible:

- Infrastructure-as-code for Harbor, Nexus, Vault (Terraform or Ansible)
- Policy-as-code in a dedicated `aibox-policies` Git repository
- Image definitions in a dedicated `aibox-images` Git repository
- Tool pack manifests in a `aibox-toolpacks` Git repository
- All repos require signed commits and PR review

---

## 6. Risk Register

| # | Risk | Likelihood | Impact | Phase | Mitigation |
|---|------|-----------|--------|-------|-----------|
| R1 | gVisor compatibility breaks developer tooling (debuggers, profilers, build tools) | Medium | High | 1 | Early compatibility testing with target tool stacks. Maintain escape hatch to runc for debugging, with compensating controls. |
| R2 | WSL2 + Podman + gVisor triple-stack instability on Windows | Medium | High | 1 | Dedicate testing effort to Windows path. Document known issues. Consider Hyper-V backend as fallback. |
| R3 | Network controls cause false-positive blocks during development | High | Medium | 2 | `aibox policy explain` for clear error messages. Easy allowlist request process. Fast turnaround (< 1 day). |
| R4 | LLM sidecar proxy adds unacceptable latency | Low | High | 2 | Benchmark early. Sidecar is localhost-only (no network hop). Target < 50ms overhead. |
| R5 | Vault/SPIRE complexity delays Phase 3 | Medium | Medium | 3 | Implement simplified credential broker first (env var injection). Vault integration as enhancement. |
| R6 | Developer adoption resistance ("I was faster without the sandbox") | High | High | 4, 6 | Invest heavily in DX. Warm start < 15s. Build cache persistence. Champions program. Fix top pain points before expanding rollout. |
| R7 | Tool pack maintenance becomes unsustainable | Medium | Medium | 4 | Automate updates. Clear governance model. Enable self-service with guardrails. Limit initial pack count. |
| R8 | Classified environment compliance requirements not fully met by audit system | Medium | High | 5 | Engage compliance team early (Phase 0). Map spec controls to compliance framework. Gap analysis before Phase 5 design. |
| R9 | Central infrastructure (Harbor, Nexus) becomes single point of failure | Low | High | 0 | Local image caching. Graceful degradation. HA deployment for Harbor/Nexus. Nexus proxy caching. |
| R10 | Scope creep from stakeholders wanting features beyond spec | High | Medium | All | Strict change control. Features go through spec amendment process. Maintain a backlog, not an expanding scope. |

---

## 7. Phase Summary Table

| Phase | Name | Dependencies | Effort | Team Size | Calendar | Status |
|-------|------|-------------|--------|-----------|----------|--------|
| 0 | Infrastructure Foundation | None | 3-4 eng-weeks | 2 engineers | Weeks 1-2 | Not Started |
| 1 | Core Runtime & CLI | Phase 0 | 5-6 eng-weeks | 2-3 engineers | Weeks 3-5 | Not Started |
| 2 | Network Security | Phases 0, 1 | 5-6 eng-weeks | 2 engineers | Weeks 5-8 | Not Started |
| 3 | Policy Engine & Credentials | Phases 1, 2 | 6-7 eng-weeks | 2-3 engineers | Weeks 8-11 | Not Started |
| 4 | Developer Experience | Phases 1, 2, 3 | 6-7 eng-weeks | 3 engineers | Weeks 11-14 | Not Started |
| 5 | Audit, Monitoring & Compliance | Phases 1, 2, 3 | 4-5 eng-weeks | 2 engineers | Weeks 8-14 (parallel) | Not Started |
| 6 | Rollout & Operations | Phases 0-4 | 8-10 eng-weeks | 2-3 engineers | Weeks 15-22+ | Not Started |
| **Total** | | | **37-45 eng-weeks** | **3-5 engineers** | **~22 weeks** | |

**Notes**:
- Calendar estimates assume 3-5 engineers working in parallel where dependencies allow.
- Phase 5 runs in parallel with Phases 3-4, not sequentially after them.
- Phase 6 calendar time is longer because it involves organizational change, not just engineering.
- Total effort of 37-45 engineer-weeks aligns with the spec's recommendation of 2 FTEs during rollout phases and 3-5 platform engineers for foundation work.
