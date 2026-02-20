# Phase 5: Audit, Monitoring & Compliance

## Overview

Phase 5 builds the observability and compliance backbone for AI-Box. Every security control deployed in Phases 1-4 generates events -- container lifecycle, network connections, DNS queries, policy decisions, credential access, LLM API traffic -- but without a collection pipeline, immutable storage, and alerting, those events are invisible to the security team.

This phase delivers six work streams: a log collection pipeline that gathers events from every component, immutable storage that ensures logs cannot be tampered with, Falco runtime monitoring for real-time threat detection, SIEM integration for correlation and alerting, optional session recording for classified environments, and dashboards that give both platform operators and security teams visibility into the fleet.

Phase 5 can begin as soon as Phase 2 completes (network/proxy/DNS logs are available) and runs in parallel with Phases 3-4. For most environments, it does not block rollout. For classified environments where audit is a hard compliance gate, it must complete before Phase 6 pilot.

**Spec Sections Covered**:
- Section 19 (Audit and Compliance) -- event categories, immutability, session recording, Falco, SIEM
- Section 20 (Threat Model) -- detection controls for residual risks (LLM covert channels, timing attacks)
- Section 24 (Residual Risks) -- monitoring-based mitigations for accepted risks

**Estimated Calendar**: Weeks 8-14 (parallel with Phases 3-4)

---

## Deliverables

1. Log collection pipeline shipping events from all AI-Box components to a central store
2. Structured event logging covering all categories defined in spec Section 19.1
3. Immutable, tamper-evident log storage with cryptographic hash chain
4. Falco deployment on developer workstations with AI-Box-specific detection rules
5. SIEM integration with correlation rules for AI-Box-specific threat patterns
6. Session recording system for terminal I/O capture (optional, for classified environments)
7. Operational dashboards for fleet health, security posture, and policy compliance
8. Alert routing and escalation configuration
9. Log retention automation (2+ years lifecycle/policy events, 1+ year network/LLM events)
10. Audit log schema documentation and SIEM integration guide

---

## Implementation Steps

### Work Stream 1: Log Collection Pipeline

**What to build**: A Vector-based log collection agent running on each developer workstation that collects events from all AI-Box components, normalizes them into a common schema, and ships them to the central immutable store.

**Sources to collect from**:

| Source | Log Type | Location |
|--------|----------|----------|
| `aibox` CLI / `aibox-agent` | Sandbox lifecycle (create, start, stop, destroy, config changes) | Container stdout/journal |
| Squid proxy | Network connections (allowed + denied), destination, bytes, duration | `/var/log/aibox/proxy-access.log` |
| CoreDNS | DNS queries and responses | CoreDNS log output / Prometheus metrics |
| `aibox-agent` | Tool invocations (command, args, exit code, risk class, approver) | Container-internal event bus |
| Vault audit log | Credential issuance, use, rotation, revocation | Vault audit device |
| OPA decision log | Every policy evaluation (input, decision, reason) | OPA decision log API |
| `aibox-llm-proxy` | LLM API request/response payloads (or content hashes) | Sidecar structured log |
| `aibox-agent` | File access events on sensitive paths | Container-internal audit |
| Falco | Runtime security alerts | Falco gRPC/file output |
| Host auditd | Host-level security events relevant to AI-Box | auditd log |

**Key implementation steps**:
1. Define a common event schema (JSON) with fields: `timestamp`, `event_type`, `sandbox_id`, `user_id`, `source`, `severity`, `details`, `hash_prev`
2. Deploy Vector as a systemd service on each dev machine (installed by `aibox setup`)
3. Configure Vector sources for each log producer (file tailing, journald, socket)
4. Configure Vector transforms for schema normalization and enrichment (add sandbox identity, user context)
5. Configure Vector sinks to ship to the central immutable store
6. Add buffering and retry logic for resilience (disk-backed buffer, at-least-once delivery)
7. Implement health checks: Vector reports status via `aibox doctor`

**Key decisions**:
- Vector over Fluentd: Rust-based, lower resource footprint on developer workstations, better performance per watt. See Research section for full comparison.
- Structured JSON logging from all components (not free-form text parsing).
- Local disk buffer (configurable, default 500MB) to survive network interruptions.
- Events enriched with sandbox identity at collection time (not at query time).

**Spec references**: Section 19.1 (event categories table), Section 19.5 (SIEM integration diagram), Section 23 (Vector in tech stack).

---

### Work Stream 2: Immutable Storage

**What to build**: A tamper-evident, append-only log storage system that ensures audit logs cannot be modified or deleted, even by administrators with access to the storage infrastructure.

**Key implementation steps**:
1. Select storage backend based on environment (see Research section for options)
2. Implement cryptographic hash chain: each log entry includes `hash_prev = SHA-256(previous_entry)`, creating a verifiable chain
3. Deploy storage with write-once/append-only enforcement at the storage layer
4. Implement dual-control access: no single administrator can access or modify raw logs
5. Configure log rotation and lifecycle policies (2+ year retention for lifecycle/policy/credential/tool events, 1+ year for network/DNS/LLM/file events)
6. Build a verification tool (`aibox audit verify`) that walks the hash chain and reports any gaps or tampering
7. Separate log storage operator role from sandbox operator role (different credentials, different access paths)

**Storage options by environment**:

| Environment | Backend | Mechanism |
|-------------|---------|-----------|
| AWS/cloud-connected | S3 with Object Lock (Compliance mode) | WORM at storage API level |
| On-premises with S3-compatible storage | MinIO with Object Lock | Same as S3, self-hosted |
| Air-gapped / no object store | Local append-only filesystem + signed batches | OS-level append-only (`chattr +a`), periodic signed batch export |

**Key decisions**:
- Hash chain provides application-level tamper evidence regardless of storage backend.
- Storage-level immutability (Object Lock / WORM) provides infrastructure-level protection.
- Both layers together: even if one is compromised, the other detects tampering.
- Retention policies enforced by storage lifecycle rules, not application logic.
- Log search/query via a read-only index (e.g., OpenSearch/Elasticsearch) that is populated from the immutable store. The index is rebuildable from the immutable source.

**Spec references**: Section 19.2 (immutability requirements), Section 4 (Reproducible and auditable principle).

---

### Work Stream 3: Falco Runtime Monitoring

**What to build**: Falco deployed on developer workstations to detect runtime security threats in real time -- container escape attempts, unexpected network activity, privilege escalation, and access to sensitive files.

**Key implementation steps**:
1. Package Falco installation into `aibox setup` (or as an optional component for environments that require it)
2. Write AI-Box-specific Falco rules targeting the threat model in spec Section 20.1
3. Tune rules to minimize false positives in developer workstation context (see Research section)
4. Configure Falco output to Vector pipeline for centralized alerting
5. Implement alert severity levels: `critical` (immediate page), `warning` (review within 4h), `info` (dashboard only)
6. Test rules against simulated attack scenarios (red team validation)

**AI-Box-specific Falco rules**:

| Rule | Detects | Severity |
|------|---------|----------|
| Container process writes outside `/workspace`, `/tmp`, `/home/dev` | Unexpected filesystem write (possible escape) | Critical |
| Container process opens raw socket | Attempt to bypass proxy (network escape) | Critical |
| Container process reads `/proc/*/environ` of another process | Credential harvesting attempt | Critical |
| Container process executes `ptrace` syscall | Debugging/escape attempt (should be blocked by seccomp, this is defense-in-depth) | Critical |
| Container process connects to IP not in allowlist | Proxy bypass attempt | Warning |
| Container process accesses `/etc/shadow`, `/etc/passwd` with write | Privilege escalation attempt | Critical |
| Unexpected binary execution in container (not in base image or tool packs) | Supply chain / persistence | Warning |
| Container spawns shell via unusual parent process | Possible reverse shell | Warning |
| High-frequency DNS queries with high subdomain entropy | DNS tunneling attempt | Warning |
| LLM API request size exceeds threshold (>1MB payload) | Possible bulk exfiltration via LLM channel | Warning |

**Key decisions**:
- Falco runs on the host (not inside the container) so it cannot be disabled by a compromised sandbox.
- Use Falco's eBPF driver (not kernel module) for easier deployment on developer workstations.
- Rules ship via the signed `aibox-policies` Git repository alongside OPA policies.
- Performance budget: Falco should consume < 2% CPU on average on a developer workstation. Rules must be tuned to meet this.
- Falco is optional in the initial deployment but recommended. Classified environments should mandate it.

**Spec references**: Section 19.4 (Falco deployment), Section 20.1 (threat model), Section 9.1 (gVisor defense-in-depth).

---

### Work Stream 4: SIEM Integration

**What to build**: Integration between the AI-Box log pipeline and the organization's existing SIEM platform, including pre-built detection rules for AI-Box-specific threat patterns.

**Key implementation steps**:
1. Survey existing SIEM platform (Splunk, Elastic SIEM, Microsoft Sentinel, QRadar, etc.)
2. Configure Vector sink for the target SIEM (syslog, HTTP/JSON, S3, Kafka -- Vector supports all major SIEMs)
3. Develop AI-Box-specific detection/correlation rules
4. Configure alert routing: security team channels (Slack, PagerDuty, email) by severity
5. Build saved searches / investigation templates for common incident types
6. Validate end-to-end: simulate attack -> Falco/log detection -> Vector shipping -> SIEM alert -> notification

**Detection rules to implement**:

| Rule Name | Trigger | Sources Correlated | Severity |
|-----------|---------|-------------------|----------|
| Anomalous outbound data volume | Squid logs show >X MB transferred in Y minutes from a single sandbox | Squid proxy logs | High |
| DNS query spike | CoreDNS logs show >N queries/minute from a sandbox | CoreDNS logs | Medium |
| Off-hours credential access | Vault audit shows token issuance outside configured business hours | Vault audit log | Medium |
| Repeated blocked network attempts | nftables/Squid deny >N requests in Y minutes from same sandbox | Squid + nftables logs | High |
| LLM payload size anomaly | `aibox-llm-proxy` logs show request payload >95th percentile by >3x | LLM proxy logs | Medium |
| Container escape indicator | Falco fires critical-severity rule | Falco alerts | Critical |
| Policy violation burst | OPA decision log shows >N denials in Y minutes from same sandbox | OPA decision logs | High |
| Credential access after sandbox stop | Vault audit shows token use after sandbox lifecycle shows `stop` event | Vault + lifecycle logs | Critical |
| Base64 payload anomaly | LLM proxy detects base64-encoded blocks in unusual request fields | LLM proxy logs | Medium |
| Git push to unexpected remote | Git operation logs show push to non-allowlisted remote | Tool invocation logs | High |

**Key decisions**:
- Vector handles all SIEM shipping (no direct log forwarders per component).
- Detection rules are maintained as code in the `aibox-policies` repository.
- Alert fatigue is a real risk; start with high-confidence rules and tune thresholds based on baseline data from the pilot phase.
- SIEM integration must work with whatever the organization already has. Vector's sink flexibility is key.

**Spec references**: Section 19.5 (SIEM integration diagram), Section 20.1 (threat mitigations), Section 24 (residual risks).

---

### Work Stream 5: Session Recording

**What to build**: An optional terminal session recording system that captures all terminal I/O within the sandbox for post-incident investigation and compliance in classified environments.

**Key implementation steps**:
1. Integrate `script` wrapper (or equivalent) into the container entrypoint to capture terminal I/O
2. Encrypt recordings at rest (AES-256-GCM, key from Vault)
3. Ship encrypted recordings to immutable storage via Vector pipeline
4. Build a playback tool (`aibox audit playback --session <id>`) for incident investigators
5. Implement access control: only designated security/compliance roles can access recordings
6. Display session recording notice to developers at sandbox start (legal requirement + deterrent)
7. Configure recording scope: full terminal I/O by default, or filtered (exclude password prompts, etc.)

**Key decisions**:
- Session recording is **optional** -- enabled per policy, not globally by default. Classified environments enable it; standard environments may not need it.
- `script` is the simplest approach (built into coreutils). For richer features (web playback, search), evaluate Teleport Session Recording or asciinema.
- Recordings are encrypted before leaving the container. The sidecar or Vector agent handles encryption.
- Storage cost estimate: ~1-5 MB/hour of active terminal use per developer. At 200 developers, 8 hours/day, this is ~3-8 GB/day uncompressed, ~500MB-1.5GB compressed.
- Retention: same as lifecycle events (2+ years for classified, configurable for others).
- Developers are informed that sessions are recorded (displayed at login). This is both a legal requirement and a deterrent.

**Spec references**: Section 19.3 (session recording), Section 4 (Reproducible and auditable principle).

---

### Work Stream 6: Dashboards & Alerting

**What to build**: Operational dashboards that give platform operators, security teams, and engineering leadership real-time visibility into AI-Box fleet health, security posture, and usage patterns.

**Key implementation steps**:
1. Deploy Grafana (or equivalent) connected to the log/metrics store
2. Build dashboard panels for each audience
3. Configure alert rules with routing to appropriate channels
4. Integrate with existing on-call/paging systems

**Dashboard: Platform Operations**

| Panel | Data Source | Purpose |
|-------|------------|---------|
| Active sandboxes (count, by team) | Lifecycle events | Fleet size tracking |
| Startup time distribution (p50, p95, p99) | Lifecycle events | SLA compliance (cold < 90s, warm < 15s) |
| Image versions in use | Lifecycle events | Patch compliance, outdated image detection |
| `aibox doctor` failure rate by check | Doctor telemetry | Proactive issue detection |
| Tool pack installation frequency | Tool invocation logs | Demand signals for pre-built images |
| Support ticket volume trend | External ticketing system | Operational health |

**Dashboard: Security Posture**

| Panel | Data Source | Purpose |
|-------|------------|---------|
| Blocked network attempts (rate, by sandbox) | Squid/nftables logs | Exfiltration attempt detection |
| DNS query patterns (rate, entropy score) | CoreDNS logs | Tunneling detection |
| Policy violations (rate, by rule, by team) | OPA decision logs | Policy effectiveness |
| Falco alerts (count, by severity, by rule) | Falco output | Runtime threat detection |
| LLM API usage (requests, tokens, payload sizes) | LLM proxy logs | Anomaly baseline + covert channel detection |
| Credential lifecycle (issuance, revocation, TTL adherence) | Vault audit log | Credential hygiene |
| Session recording coverage | Recording metadata | Compliance verification |

**Dashboard: Executive Summary**

| Panel | Data Source | Purpose |
|-------|------------|---------|
| Adoption rate (% of developers using AI-Box) | Lifecycle events + HR data | Rollout progress |
| Security incident count (by severity, trend) | SIEM | Risk posture |
| Developer satisfaction (from surveys) | External survey tool | Adoption health |
| Cost per developer (compute, storage, ops) | Infrastructure metrics | Budget tracking |

**Alert routing**:

| Severity | Channel | SLA |
|----------|---------|-----|
| Critical (container escape, credential misuse) | PagerDuty / on-call page | 15 minutes |
| High (repeated blocked attempts, anomalous data volume) | Security team Slack channel | 4 hours |
| Medium (DNS spikes, off-hours access, payload anomalies) | Security team email digest | Next business day |
| Info (policy violations, outdated images) | Dashboard only | Weekly review |

**Key decisions**:
- Grafana is the recommended dashboard tool (open source, broad data source support, alerting built in).
- Dashboards are provisioned as code (Grafana provisioning API or Terraform) and stored in the `aibox-policies` repository.
- Alert thresholds are calibrated during pilot phase using baseline data. Start permissive, tighten after baselining.
- Executive dashboard is deliberately simple -- 4-5 panels, updated daily, no operational noise.

**Spec references**: Section 19.1 (event categories for dashboard content), Section 22.1 (day-2 operations activities that dashboards support).

---

## Research Required

### Vector vs Fluentd for Log Shipping

**Why this matters**: The log shipper runs on every developer workstation. It must be lightweight, reliable, and support all the output formats the organization needs.

| Factor | Vector | Fluentd |
|--------|--------|---------|
| Language | Rust | Ruby + C |
| Resource usage | Lower CPU/memory footprint | Higher, especially with many plugins |
| Performance | ~10x throughput advantage in benchmarks | Adequate for most workloads |
| Plugin ecosystem | Growing, covers major sinks | Mature, very broad |
| Configuration | TOML (declarative) | Ruby-flavored config |
| Reliability | Built-in disk-backed buffering | Requires buffer plugin configuration |
| Developer workstation fit | Better (lower impact on dev machine) | Acceptable but heavier |

**Recommendation**: Vector. The spec already lists it in the tech stack (Section 23). Its lower resource footprint is important since it runs on developer machines alongside the IDE, build tools, and AI agents.

**Action items**:
- [ ] Benchmark Vector resource consumption on target developer hardware (Windows 11 + WSL2 and native Linux)
- [ ] Validate Vector sinks for the organization's specific SIEM platform
- [ ] Test Vector on Windows/WSL2 (confirm it runs as a systemd service inside WSL2)

### Immutable Storage Options

**Why this matters**: The storage backend must provide true write-once semantics and be deployable in the organization's environment (which may be air-gapped or on-premises).

| Option | Immutability Mechanism | On-Premises | Air-Gapped | Operational Complexity |
|--------|----------------------|-------------|-----------|----------------------|
| S3 Object Lock (Compliance mode) | Storage API enforced, cannot be disabled even by root | No (AWS only) | No | Low (managed service) |
| MinIO with Object Lock | Same API as S3, self-hosted | Yes | Yes | Medium (operate MinIO cluster) |
| Local append-only + signed batches | `chattr +a` + periodic GPG-signed archive | Yes | Yes | High (custom tooling, manual verification) |

**Recommendation**: MinIO with Object Lock for on-premises/air-gapped. S3 Object Lock if cloud-connected. Local append-only as fallback for minimal infrastructure environments.

**Action items**:
- [ ] Determine target environment constraints (cloud-connected, on-premises, air-gapped)
- [ ] If MinIO: size the cluster (estimate log volume: ~200 devs, 8 hours/day, all event categories)
- [ ] If local: prototype the signed batch export and verification workflow
- [ ] Estimate storage costs: ~1-5 GB/day for 200 developers (compressed), retained for 1-2 years

### Falco on Developer Workstations

**Why this matters**: Falco is designed for server/Kubernetes environments. Running it on developer workstations introduces concerns around performance impact, driver compatibility, and rule tuning to avoid false positives in a development context.

**Key questions**:
- [ ] eBPF driver compatibility with WSL2 kernel (WSL2 ships a Microsoft-maintained kernel; verify eBPF support for Falco's needs)
- [ ] CPU overhead of Falco's eBPF probes under developer workload (builds, IDE indexing, AI agent activity generate high syscall rates)
- [ ] False positive rate of default Falco rules in a container development context (developers legitimately do things that look suspicious on a server)
- [ ] Interaction between Falco and gVisor (gVisor intercepts syscalls in userspace; Falco's eBPF hooks may see different syscall patterns)
- [ ] Can Falco rules be scoped to only AI-Box containers (avoid monitoring unrelated workloads on the dev machine)?

**Action items**:
- [ ] Deploy Falco on 2-3 representative developer machines (Windows/WSL2 + native Linux)
- [ ] Run a typical development workload for 1 week, measure CPU/memory overhead
- [ ] Catalog false positives and tune rules iteratively
- [ ] Test Falco + gVisor interaction specifically (does gVisor's syscall interception affect Falco visibility?)
- [ ] Document performance budget: target < 2% average CPU overhead

### Existing SIEM Platform Compatibility

**Why this matters**: The organization likely already has a SIEM. AI-Box logs must integrate with it, not create a parallel monitoring silo.

**Action items**:
- [ ] Identify the organization's current SIEM platform (Splunk, Elastic SIEM, Microsoft Sentinel, QRadar, etc.)
- [ ] Validate Vector has a mature sink for that platform
- [ ] Determine required log format (CEF, LEEF, JSON, syslog RFC 5424)
- [ ] Identify existing correlation rules that could apply to AI-Box events
- [ ] Determine if the SIEM team has capacity to onboard new log sources and write custom detection rules
- [ ] Confirm network path from developer workstations to SIEM ingestion endpoint (firewall rules, VPN, etc.)

---

## Open Questions

1. **Is audit a hard gate for rollout?** For classified environments, Phase 5 may need to complete before Phase 6 pilot begins. For standard environments, basic logging may suffice for pilot, with full audit completing in parallel. Need decision from security/compliance stakeholders.

2. **LLM payload logging: full content or hashes?** Logging full LLM request/response payloads provides the best forensic capability but creates large volumes of potentially sensitive data in the log store. Can this be configured per classification level (full content for classified, hashes for standard)?

3. **Session recording opt-in or opt-out?** The spec says "optional for classified environments." Should this be a per-environment policy setting or a per-team setting? Legal/HR review needed for jurisdictions with employee monitoring regulations.

4. **Log storage sizing and budget.** Estimated ~1-5 GB/day compressed for 200 developers. With 1-2 year retention, this is ~0.5-3.5 TB. With session recordings, potentially 2-5x more. Need budget approval for storage infrastructure.

5. **Falco on Windows/WSL2: is it viable?** Falco's eBPF driver may not work in all WSL2 kernel versions. If it doesn't, the options are: (a) Falco on native Linux only, (b) host-level auditd as a substitute on Windows, (c) defer Falco to centralized (K8s) mode. Research must answer this before implementation begins.

---

## Dependencies

### Depends On (inputs from earlier phases)

| Dependency | Source Phase | What It Provides |
|------------|-------------|-----------------|
| Container lifecycle events | Phase 1 | Sandbox create/start/stop/destroy events to collect |
| Network, proxy, and DNS logs | Phase 2 | Squid access logs, nftables counters, CoreDNS query logs |
| LLM proxy payload logs | Phase 2 | Request/response payloads from `aibox-llm-proxy` |
| OPA decision logs | Phase 3 | Policy evaluation records with input/decision/reason |
| Vault audit logs | Phase 3 | Credential issuance, use, and revocation records |
| Tool invocation logs | Phase 3 | Command execution records with risk class and approval |
| `aibox-agent` in container | Phase 1 | Process that generates and exposes structured events |
| Signed policies repository | Phase 3 | Distribution channel for Falco rules alongside OPA policies |

### Depended On By (outputs consumed by later phases)

| Consumer | What It Needs |
|----------|--------------|
| Phase 6 (Rollout) | Dashboards for pilot monitoring, security sign-off for classified rollout |
| Phase 6 (Operations) | Operational dashboards, alerting, day-2 runbooks referencing log queries |
| Security team | SIEM integration, detection rules, incident investigation tools |
| Compliance team | Retention proof, tamper-evidence verification, audit reports |

### External Dependencies

| Dependency | Owner | Risk |
|------------|-------|------|
| SIEM platform access and ingestion capacity | Security/SOC team | May require SIEM team capacity for onboarding |
| Storage infrastructure for immutable logs | Infrastructure team | MinIO cluster or S3 bucket provisioning |
| Compliance framework requirements | Compliance/legal team | Must be clarified before finalizing event categories and retention |
| Developer workstation permissions for Falco install | IT/endpoint team | Falco eBPF driver may require elevated install permissions |

---

## Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|-----------|--------|-----------|
| R1 | Falco eBPF driver incompatible with WSL2 kernel | Medium | Medium | Research early (week 1). Fallback: host auditd on Windows, Falco on Linux only. Falco becomes mandatory only in centralized (K8s) mode. |
| R2 | Falco causes unacceptable performance degradation on developer workstations | Medium | High | Performance budget of < 2% CPU. Benchmark during research phase. Aggressive rule pruning. Disable non-critical rules. Make Falco optional with compensating controls (enhanced proxy logging). |
| R3 | Log volume overwhelms storage budget | Medium | Medium | Start with high-value events only (lifecycle, policy, credential, network deny). Add lower-priority events (all network allow, all file access) incrementally. Compress aggressively. Sample LLM payloads if volume is excessive. |
| R4 | Alert fatigue from too many false positives | High | High | Start with high-confidence rules only. Baseline normal behavior during pilot before enabling alerting. Tune thresholds using real data, not estimates. Implement alert suppression for known-good patterns. |
| R5 | SIEM team lacks capacity to onboard AI-Box logs | Medium | Medium | Engage SIEM team in Phase 0/1 planning. Provide pre-built detection rules and dashboards. Minimize custom work required from SIEM team. |
| R6 | Session recording creates legal/privacy concerns | Medium | High | Engage legal/HR before implementation. Ensure recording notice is displayed. Provide per-policy toggle. Do not enable by default. |
| R7 | Hash chain verification is too slow at scale | Low | Medium | Verify in batches (daily). Use Merkle tree structure for efficient partial verification. Store periodic checkpoints. |
| R8 | Vector agent on developer machine interferes with development workflow | Low | High | Benchmark Vector resource usage. Configure CPU/memory limits. Use disk-backed buffer with size cap. Implement graceful degradation (if Vector fails, development continues). |

---

## Exit Criteria

All of the following must be verified before Phase 5 is considered complete:

1. **Event coverage**: All event categories from spec Section 19.1 are captured and arrive in the central store within 60 seconds of generation (at p95).

2. **Tamper evidence**: The hash chain verification tool (`aibox audit verify`) successfully detects a deliberately injected tampered log entry.

3. **Immutability**: Attempting to modify or delete a stored log entry fails at the storage layer (Object Lock / WORM / append-only enforcement verified).

4. **Falco detection**: Simulated container escape attempt, privilege escalation attempt, and unexpected network connection all trigger Falco alerts that arrive in the SIEM within 2 minutes.

5. **SIEM correlation**: At least 5 of the 10 detection rules fire correctly when their trigger conditions are simulated end-to-end (event generation -> Vector -> SIEM -> alert).

6. **Session recording** (if enabled): A recorded session can be played back with full fidelity, and recordings are encrypted at rest.

7. **Dashboard operational**: Platform operations dashboard shows real-time fleet status. Security dashboard shows Falco alerts and policy violations. Both refresh within 30 seconds.

8. **Retention automation**: Log lifecycle policies are configured and verified (old test data expires as expected per retention rules).

9. **Performance**: Vector agent consumes < 1% average CPU on developer workstations during normal development activity. Falco (if deployed) consumes < 2% average CPU.

10. **Documentation**: Audit log schema reference, SIEM integration guide, and incident investigation playbook are published and reviewed by the security team.

---

## Estimated Effort

| Work Stream | Effort | Skills Needed |
|-------------|--------|--------------|
| Log Collection Pipeline | 1-1.5 engineer-weeks | Vector configuration, systemd, structured logging |
| Immutable Storage | 1-1.5 engineer-weeks | MinIO/S3, cryptography (hash chains), storage operations |
| Falco Runtime Monitoring | 0.5-1 engineer-week | Falco rule authoring, eBPF, Linux security |
| SIEM Integration | 0.5-1 engineer-week | Target SIEM platform expertise, detection engineering |
| Session Recording | 0.5 engineer-week | Terminal I/O, encryption, storage |
| Dashboards & Alerting | 0.5-1 engineer-week | Grafana, data visualization, alert routing |
| **Total** | **4-5 engineer-weeks** | **2 engineers, ~3 calendar weeks** |

**Team composition**: 2 engineers -- one focused on the data pipeline (Work Streams 1, 2, 4) and one focused on security monitoring (Work Streams 3, 5, 6). Both collaborate on schema design and testing.

**Calendar**: Weeks 8-14, running in parallel with Phases 3-4. Research items (Falco/WSL2 compatibility, SIEM platform survey) should begin in Week 6-7 to de-risk.
