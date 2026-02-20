# Phase 3: Policy Engine & Credentials

**Phase**: 3 of 6
**Status**: Not Started
**Estimated Effort**: 6-7 engineer-weeks
**Team Size**: 2-3 engineers
**Calendar**: Weeks 8-11
**Dependencies**: Phase 1 (Core Runtime & CLI), Phase 2 (Network Security)

---

## Overview

Phase 3 transforms AI-Box from a secured container with network controls into a policy-driven platform with cryptographic identity and dynamic credential management. It delivers two tightly coupled capabilities:

1. **Policy Engine (OPA/Rego)**: A declarative, policy-as-code system that enforces security rules across network access, filesystem boundaries, tool invocations, and resource limits. Policies follow a strict hierarchy -- org baseline (immutable, signed by security) > team policy > project policy -- where each level can only tighten, never loosen.

2. **Credential Management (Vault + SPIFFE/SPIRE)**: A zero-standing-credential system where all secrets (Git tokens, LLM API keys, package mirror tokens) are short-lived, scoped to specific workloads, and automatically revoked when the sandbox terminates. Workload identity is cryptographically attested via SPIFFE SVIDs, eliminating pre-shared secrets.

These two systems reinforce each other: the policy engine decides *what* a workload may do; the credential system ensures it can only authenticate as *who* it claims to be, and only for as long as it needs to.

**Spec Sections Covered**:
- Section 5.2 (Policy Engine) -- OPA/Rego, policy hierarchy, decision logging
- Section 11 (Credential Management) -- Vault, SPIFFE/SPIRE, credential types, Git auth, LLM key injection, token lifecycle, simplified fallback
- Section 12 (Policy Engine) -- policy hierarchy, policy spec, OPA validation, tool permission model

---

## Deliverables

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

---

## Implementation Steps

### Work Stream 1: OPA Policy Engine

**What to build**:
- An OPA instance (embedded library, sidecar, or external process) that the `aibox-agent` process queries for every policy decision.
- A policy bundle loading mechanism that pulls signed Rego bundles from the `aibox-policies` Git repository at container start.
- A decision logging backend that records every evaluation (input document, policy version, decision, reason) to a structured log file that Phase 5 will ship to central storage.
- Hot-reload support so policy updates can be applied without container restart (pull on interval or webhook trigger).

**Key decisions**:
- **Integration mode**: OPA runs as an **embedded Go library** (`github.com/open-policy-agent/opa/rego`) since `aibox-agent` is written in Go. This provides sub-millisecond evaluation and avoids inter-process overhead. No sidecar or external service needed.
- **Bundle distribution**: Rego bundles are stored in the `aibox-policies` Git repository, signed with Cosign, and pulled at container start. The `aibox-agent` verifies signatures before loading. Updates can be pulled periodically (every 5 minutes) or on-demand via `aibox policy update`.
- **Decision log format**: Structured JSON, one line per decision, compatible with Vector/Fluentd for shipping. Include: timestamp, policy version hash, input hash, decision (allow/deny), rule that fired, human-readable reason.

**Spec references**: Section 5.2, Section 12.3, Section 19.1 (policy decision logging)

---

### Work Stream 2: Policy Hierarchy & Merge

**What to build**:
- A policy merge engine that combines three levels of policy (org baseline, team, project) into a single effective policy for a given sandbox session.
- The merge algorithm enforces the **tighten-only invariant**: a lower-level policy can only add restrictions, never remove them. Specifically:
  - Network allowlist: lower levels can remove entries, never add entries not in the parent.
  - Filesystem deny list: lower levels can add paths, never remove paths from the parent.
  - Tool rules: lower levels can change `safe` to `review-required` or `blocked-by-default`, but never the reverse.
  - Resource limits: lower levels can reduce CPU/memory/disk, never increase beyond the parent.
  - Runtime settings: lower levels cannot change `gvisor` to `runc` or `rootless: true` to `false`.
- A `policy.yaml` schema validator that rejects malformed or policy-violating configurations at authoring time.
- The `aibox policy validate` CLI command that runs the merge locally and reports violations.

**Key decisions**:
- **Merge semantics**: Define precise merge rules for each policy section. Network allowlists use set intersection (lower level is a subset of parent). Filesystem denies use set union (lower level adds to parent's deny list). Tool rules use most-restrictive-wins. Resource limits use min(parent, child).
- **Org baseline immutability**: The org baseline is signed by the security team's key and distributed as a read-only file inside the container image at `/etc/aibox/org-policy.yaml`. It cannot be overridden or modified inside the container.
- **Team policy location**: Team policies reside in the `aibox-policies` repo under `teams/<team-name>/policy.yaml`. Team leads have merge permissions to their team's directory.
- **Project policy location**: Project policies live in the source repository at `/aibox/policy.yaml` (spec Appendix C).

**Spec references**: Section 12.1, Section 12.2, Appendix C

---

### Work Stream 3: Tool Permission Model

**What to build**:
- An interceptor in the `aibox-agent` that evaluates tool/command invocations against the effective policy before execution.
- Three risk classes with distinct enforcement behaviors:
  - `safe`: Allowed immediately, logged silently. Examples: `git status`, `ls`, `cat`.
  - `review-required`: Allowed but generates an audit entry and surfaces a notification (IDE or CLI). Examples: `git push`, `npm publish`. For interactive approval mode: blocks until approver responds.
  - `blocked-by-default`: Denied unless an explicit policy exception exists. Returns a clear error message referencing the blocking rule. Examples: `curl`, `wget`, `ssh`.
- A pattern matching engine for tool rules. The spec uses a `match` array format (e.g., `["git", "push"]`) that supports wildcards (`*`).
- Integration with the decision logging system from Work Stream 1.

**Key decisions**:
- **Interception point**: The `aibox-agent` process wraps command execution. For shell commands, this can be implemented as a shell wrapper (`/usr/local/bin/aibox-exec`) that the container's shell invokes before running any command. For MCP tool calls, the MCP server implementations include policy checks. The design must minimize latency for `safe` commands (target: < 5ms overhead).
- **Approval flow for `review-required`**: Two modes should be supported: (a) async/non-blocking (log and proceed, reviewer checks after the fact) and (b) sync/blocking (pause until approved via IDE notification or CLI prompt). The mode is configurable per rule in the policy.
- **Default risk class**: Any command not explicitly matched by a rule defaults to `safe` (to avoid breaking normal development workflows) or `blocked-by-default` (maximum security). The spec implies a whitelist approach for known-dangerous commands. Recommendation: default to `safe` for common development commands, with the org baseline explicitly blocking known-dangerous commands.

**Spec references**: Section 12.4, Section 18.5 (`git push` approval flow)

---

### Work Stream 4: SPIFFE/SPIRE Deployment

**What to build**:
- A SPIRE Server deployed on a management host (or the same host running Vault) that acts as the trust root for workload identity.
- A SPIRE Agent installed on each developer workstation (inside WSL2 on Windows, natively on Linux) that attests container workloads.
- Workload registration entries that assign a SPIFFE ID to each AI-Box sandbox based on container metadata (container ID, image hash, user UID).
- The SPIRE Agent exposes a Workload API (Unix domain socket) that processes inside the container use to obtain SVIDs (SPIFFE Verifiable Identity Documents).
- SVID format: X.509 certificates with short TTLs (1 hour), auto-rotated by the SPIRE Agent.

**Key decisions**:
- **SPIRE Server topology**: For local-first, each developer machine runs its own SPIRE Agent, but a shared SPIRE Server handles registration. For air-gapped environments, a local SPIRE Server per machine is also viable (with manual registration). Evaluate: upstream authority (shared root CA) vs. self-signed per-machine.
- **Workload attestation method**: Use the Unix workload attestor (based on process UID/GID, binary path) combined with container metadata from Podman. The SPIRE Agent must be able to distinguish AI-Box containers from other processes on the developer machine.
- **Trust domain**: Define a SPIFFE trust domain (e.g., `spiffe://aibox.org.internal/`) with a structured ID format: `spiffe://aibox.org.internal/sandbox/<user>/<workspace>`.
- **SPIRE Agent installation**: Bundled with `aibox setup`. The agent runs as a system service (systemd on Linux, Windows service via WSL2 init).

**Spec references**: Section 11.2, Section 23 (tech stack: SPIFFE/SPIRE)

---

### Work Stream 5: Vault Integration

**What to build**:
- Vault server configuration (assumes Vault is pre-deployed or deployed as part of this phase).
- A SPIFFE auth method in Vault that validates SVIDs from the SPIRE trust domain and maps them to Vault policies.
- Vault policies that scope credentials per workload identity:
  - `aibox/sandbox/<user>/*` can request Git tokens for repos the user has access to.
  - `aibox/sandbox/<user>/*` can request LLM API keys with rate limits.
  - `aibox/sandbox/<user>/*` can request read-only package mirror tokens.
- Dynamic secret backends:
  - Git tokens: Vault generates short-lived HTTPS tokens (4-hour TTL) scoped to a specific repository via the Git server's API (GitHub/GitLab/Bitbucket token generation endpoint).
  - LLM API keys: Vault stores a pool of API keys and leases them to sandboxes with rate-limit metadata. Or Vault generates per-session API keys if the LLM provider supports it.
  - Package mirror tokens: Vault generates read-only Nexus tokens (8-hour TTL).
- A credential request library (`aibox-vault-client`) used by `aibox-credential-helper` and `aibox-llm-proxy` to authenticate with Vault and fetch secrets.

**Key decisions**:
- **Vault deployment model**: For local-first, Vault runs centrally (spec Section 6: 2 CPU, 4GB RAM, HA pair). All developer machines reach Vault through the network (allowed via proxy). This is the one central dependency that matters for credential management.
- **Auth method**: The SPIFFE auth method (`vault auth enable jwt` with SPIFFE SVID validation) is the recommended approach. The SVID's X.509 certificate is validated against the SPIRE Server's trust bundle. This means no static Vault tokens or passwords are needed on developer machines.
- **Git token generation**: Depends on the Git server platform. For GitHub Enterprise, use GitHub App installation tokens. For GitLab, use personal access tokens via API (or project access tokens). For Bitbucket, use repository access tokens. The credential helper must abstract the platform.
- **Graceful degradation**: When Vault is unreachable, existing credentials remain valid until their TTL expires. The `aibox-agent` caches credentials in memory (never disk) and retries Vault on a backoff schedule. The credential helper returns cached credentials transparently.

**Spec references**: Section 11.2, Section 11.3 (credential types and TTLs), Section 11.6 (token lifecycle), Section 22.4 (disaster recovery -- Vault down)

---

### Work Stream 6: Credential Helper

**What to build**:
- `aibox-credential-helper`: A Git credential helper binary that implements the `git-credential` protocol. When Git needs credentials (clone, push, fetch), it calls this helper, which:
  1. Obtains an SVID from the SPIRE Agent.
  2. Authenticates to Vault using the SVID.
  3. Requests a short-lived Git token scoped to the target repository (4-hour TTL).
  4. Returns the token in `git-credential` format (`protocol=https\nhost=git.internal\nusername=x-token\npassword=<token>\n`).
  5. Caches the token in memory for the TTL duration (avoids round-trip to Vault on every git operation).
- Token revocation on sandbox stop: the `aibox stop` flow calls Vault's lease revocation endpoint for all active leases associated with the sandbox's SVID.
- Token persistence prevention: a policy check (or file-system watch) that blocks commits containing token-like strings in `/workspace`.
- Configuration: `git config --global credential.helper '/usr/local/bin/aibox-credential-helper'` set in the container image.
- Integration with `aibox-llm-proxy` for LLM API key injection (the proxy uses the same Vault client to obtain LLM API keys and inject them into outbound requests).

**Key decisions**:
- **Caching strategy**: Cache tokens in memory only. Never write to disk or environment variables (beyond the initial env-var fallback path). Use a token cache keyed by (repo URL, operation type) with TTL-based expiry.
- **Token scope**: Tokens should be scoped as narrowly as possible. Ideal: per-repository, read+write for the developer's own repos, read-only for dependencies. The scoping depends on the Git server's token model.
- **Revocation timing**: Revocation must happen synchronously during `aibox stop`. If Vault is unreachable during stop, queue the revocation and retry. Tokens expire naturally via TTL regardless, so this is a defense-in-depth measure.

**Spec references**: Section 11.4 (Git authentication), Section 11.5 (LLM API key injection), Section 11.6 (token lifecycle)

---

### Work Stream 7: Simplified Fallback

**What to build**:
- A simplified credential broker for organizations that do not yet have Vault infrastructure. This is a stepping stone, not the long-term solution.
- The fallback mechanism:
  1. `aibox start` reads credentials from a secure host-side store (OS keychain on Linux via `secret-tool`, Windows Credential Manager via WSL2 interop, or an encrypted file at `~/.config/aibox/credentials.enc`).
  2. Credentials are passed into the container as environment variables visible only to the container init process (PID 1).
  3. The `aibox-credential-helper` and `aibox-llm-proxy` read from environment variables instead of Vault.
  4. Credentials are NOT written to disk inside the container.
- `aibox setup` includes a credential configuration step: `aibox auth add git-token <token>` stores the token in the host keychain.
- Clear documentation of the security trade-offs: env vars are visible via `/proc/<pid>/environ` to processes with the same UID inside the container (gVisor mitigates this partially), and credentials are longer-lived (no dynamic minting/revocation).

**Key decisions**:
- **Host-side storage**: Prefer the OS keychain (`libsecret` on Linux, Credential Manager on Windows). Fall back to an AES-256-encrypted file if keychain is unavailable (e.g., headless Linux server without a keyring daemon).
- **Token refresh**: Without Vault, tokens must be manually rotated. The `aibox` CLI warns when tokens are approaching expiry and prompts the user to update them.
- **Migration path**: The credential helper should abstract the backend (Vault vs. env var). When the organization deploys Vault, switching is a configuration change, not a code change.

**Spec references**: Section 11.7 (simplified alternative)

---

## Research Required

### OPA Bundle Signing and Decision Log Performance
- **Bundle signing and verification**: OPA supports signed bundles natively. Evaluate whether OPA's built-in bundle signing (using JWS) is sufficient or whether Cosign signatures on the Git repository provide a better trust chain.
- **Decision log performance**: OPA's built-in decision logging can impact performance at high volumes. Evaluate async logging (buffered writes) vs. synchronous logging. Target: < 1ms overhead per decision.

### SPIFFE/SPIRE Deployment on Developer Workstations
- **WSL2 compatibility**: SPIRE Agent runs as a Linux binary. On Windows, it runs inside WSL2. Evaluate whether the SPIRE Agent can attest Podman containers running inside WSL2 (container metadata access via Podman socket).
- **Workload attestation for rootless Podman**: The standard Docker workload attestor may not work with rootless Podman. Evaluate the Unix attestor as a fallback and determine what metadata is available for distinguishing AI-Box containers.
- **SPIRE Server scaling**: For 200 developers, each with 1-2 active sandboxes, the SPIRE Server handles ~200-400 SVIDs with 1-hour rotation. Evaluate server sizing and HA requirements.
- **Offline/air-gapped SVID renewal**: If the SPIRE Server is unreachable, SVIDs expire after their TTL. Evaluate longer SVID TTLs for graceful degradation vs. security trade-off.

### Vault Auth Methods for SPIFFE SVIDs
- **JWT auth method with SVID**: Vault's JWT auth method can validate SPIFFE SVIDs if the SVID includes a JWT-SVID. Evaluate whether X.509-SVID or JWT-SVID is more appropriate for Vault authentication.
- **Vault SPIFFE auth plugin**: There is a community Vault plugin for direct SPIFFE authentication. Evaluate its maturity, maintenance status, and suitability for production use.
- **Vault policy granularity**: Map SPIFFE IDs to Vault policies. Evaluate whether per-user policies (one policy per developer) or role-based policies (per-team or per-project) are more maintainable at scale (200 developers).

### Rego Policy Testing Frameworks
- **OPA test framework**: OPA includes a built-in `opa test` command for unit-testing Rego policies. Evaluate its CI integration and coverage reporting capabilities.
- **Conftest**: A tool for testing structured data against Rego policies. Evaluate whether Conftest is useful for testing `policy.yaml` files in addition to OPA's native test framework.
- **Policy simulation**: Evaluate tools for simulating policy decisions against real-world workloads (replay production decision logs through updated policies to detect regressions).

---

## Open Questions

**Q1**: What is the Git server platform (GitHub Enterprise, GitLab, Bitbucket) and what token generation APIs does it expose? -- *Who should answer*: Infrastructure/DevOps team

**Q2**: Is an existing Vault cluster available, or does one need to be deployed as part of this phase? -- *Who should answer*: Infrastructure/security team

**Q3**: What SPIFFE trust domain should be used, and should it be shared with other organizational workloads or dedicated to AI-Box? -- *Who should answer*: Security architecture team

**Q4**: For the `review-required` tool class, should the default behavior be async (log and proceed) or sync (block until approved)? -- *Who should answer*: Security team + developer experience team (jointly)

**Q5**: What is the acceptable credential TTL range? Shorter TTLs (1-2 hours) increase Vault load and break graceful degradation; longer TTLs (8-12 hours) increase exposure window. -- *Who should answer*: Security team

**Q6**: How granular should Git token scoping be? Per-organization, per-repository, or per-branch? -- *Who should answer*: Security team + Git server administrators

---

## Dependencies

### Upstream (What Phase 3 Requires)

| Dependency | Source Phase | What Is Needed | Impact If Missing |
|-----------|------------|----------------|-------------------|
| Running container with `aibox-agent` | Phase 1 | A container lifecycle that the policy engine can hook into | Cannot enforce policies without a running sandbox |
| `aibox` CLI framework | Phase 1 | CLI extension points for `policy validate`, `policy explain` | CLI commands cannot be added |
| Network connectivity to Vault | Phase 2 | Squid proxy allowlist includes `vault.internal:443` | Containers cannot fetch credentials from Vault |
| Network connectivity to Git server | Phase 2 | Squid proxy allowlist includes `git.internal:443` | Credential helper cannot validate token scoping |
| LLM sidecar proxy | Phase 2 | The sidecar must be modified to use Vault-issued keys instead of static keys | LLM API key injection requires Phase 2 sidecar |
| Harbor registry | Phase 0 | Signed images that embed the org baseline policy | Org baseline cannot be distributed immutably |

### Downstream (What Depends on Phase 3)

| Dependent | Phase | What They Need | Impact of Delay |
|----------|-------|---------------|-----------------|
| Tool packs with credential needs | Phase 4 | Dynamic credential injection for tool pack network endpoints | Tool packs that need authenticated endpoints are blocked |
| MCP packs with API access | Phase 4 | Credential injection for MCP servers (e.g., Jira token for `jira-mcp`) | Authenticated MCP packs are blocked |
| `git push` approval flow | Phase 4 | Tool permission model (`review-required` class) must be operational | Approval flow has no enforcement mechanism |
| AI tool API key injection | Phase 4 | Vault-issued LLM API keys via sidecar proxy | AI tools fall back to static keys or manual env vars |
| Policy decision logging | Phase 5 | Structured decision logs in a shippable format | Audit system has no policy events to collect |
| Credential access logging | Phase 5 | Token issuance/revocation events logged | Audit system has no credential events to collect |

---

## Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|-----------|--------|-----------|
| R1 | Vault/SPIRE complexity delays the phase beyond 7 weeks | Medium | Medium | Build the simplified fallback (Work Stream 7) first. This provides a working credential system while Vault integration proceeds in parallel. The credential helper abstracts the backend, so switching is seamless. |
| R2 | SPIRE Agent does not reliably attest rootless Podman containers on WSL2 | Medium | High | Research this in the first week. If attestation is unreliable, fall back to Unix attestor with UID/GID matching. Maintain a test matrix of WSL2 + Podman versions. |
| R3 | OPA policy evaluation adds perceptible latency to developer workflows | Low | High | Use embedded library or Unix socket sidecar (not HTTP). Benchmark early with realistic policy sets. Target < 5ms per decision for `safe` commands. Cache decisions for repeated identical inputs. |
| R4 | Policy hierarchy merge logic has edge cases that either block legitimate work or allow policy loosening | Medium | High | Write comprehensive unit tests for the merge engine covering all policy sections. Use property-based testing to verify the tighten-only invariant holds for arbitrary policy combinations. Security review the merge logic before deployment. |
| R5 | Git server token API does not support the required scoping granularity | Medium | Medium | Research the Git server's token API early (Q2). If per-repo scoping is not available, fall back to per-organization tokens with shorter TTLs. Document the residual risk. |
| R6 | Vault becomes a single point of failure for all sandbox operations | Low | High | Implement graceful degradation: cache credentials in memory with TTL. Sandbox continues operating with cached credentials when Vault is down. Alert operators. Vault should be deployed in HA mode. |
| R7 | Decision log volume overwhelms local storage before Phase 5 log shipping is operational | Low | Medium | Implement log rotation with size limits. Keep the most recent 7 days locally. Decision logs for `safe` commands can be sampled (log 1 in 10) rather than exhaustive. |

---

## Exit Criteria

1. **Policy merge correctness**: A project `policy.yaml` that attempts to loosen the org baseline (e.g., adding a host to the network allowlist, changing a `blocked-by-default` tool to `safe`) is rejected by `aibox policy validate` with a clear error message.

2. **Tool permission enforcement**: Tool invocations matching `blocked-by-default` rules are denied. The developer sees a human-readable explanation referencing the specific rule and how to request an exception.

3. **Review-required flow**: `review-required` tools (e.g., `git push`) generate an audit entry. In async mode, the action proceeds and the entry is logged. In sync mode, the action blocks until approved.

4. **Dynamic Git credentials**: Git operations inside the container use short-lived tokens from Vault. `git clone` and `git push` to `git.internal` succeed. No static Git tokens exist anywhere in the container.

5. **Token revocation**: Tokens are revoked within 5 seconds of `aibox stop`. Verification: after stopping a sandbox, attempt to use a previously-issued token -- it must be rejected by the Git server.

6. **Decision logging**: Decision logs are queryable and contain the full evaluation context (timestamp, input, policy version, decision, firing rule, reason). Logs are written in structured JSON.

7. **Graceful degradation**: If Vault is temporarily unreachable, the sandbox continues operating with cached credentials until their TTL expires. The `aibox status` command reports the degraded state.

8. **Simplified fallback**: For environments without Vault, `aibox start` successfully injects credentials from the host keychain. The credential helper and LLM proxy function correctly with env-var-based credentials.

9. **Policy explain**: `aibox policy explain --log-entry <id>` returns a developer-friendly explanation of why a specific action was allowed or denied.

10. **SPIFFE identity**: The sandbox workload obtains a valid SVID from the SPIRE Agent and uses it to authenticate to Vault without any static tokens or passwords.

---

## Estimated Effort

| Work Stream | Effort | Engineers | Notes |
|------------|--------|-----------|-------|
| 1. OPA Policy Engine | 1-1.5 weeks | 1 | Includes OPA integration, bundle loading, decision logging |
| 2. Policy Hierarchy & Merge | 1-1.5 weeks | 1 | Merge algorithm, schema validation, CLI commands |
| 3. Tool Permission Model | 1 week | 1 | Interceptor, pattern matching, approval flow |
| 4. SPIFFE/SPIRE Deployment | 1-1.5 weeks | 1 | SPIRE Server, Agent, workload registration |
| 5. Vault Integration | 1-1.5 weeks | 1 | Auth method, policies, dynamic secret backends |
| 6. Credential Helper | 1 week | 1 | Git credential helper, LLM proxy integration, revocation |
| 7. Simplified Fallback | 0.5-1 week | 1 | Keychain storage, env var injection, migration path |
| **Total** | **6-7 weeks** | **2-3** | Work streams 1-3 can run in parallel with 4-7 |

**Parallelization**: The phase naturally splits into two tracks:
- **Track A (Policy)**: Work Streams 1, 2, 3 -- can be done by one engineer.
- **Track B (Credentials)**: Work Streams 4, 5, 6, 7 -- can be done by one or two engineers.

Track B starts with Work Stream 7 (simplified fallback) first -- this provides a working credential system immediately and unblocks other work. Then proceed to Work Streams 4, 5, 6 for the full Vault/SPIRE integration. Track A and Track B converge when the tool permission model (Work Stream 3) needs credential-gated decisions.

A third engineer is valuable for writing the Rego test suite, security testing, and documentation in parallel.
