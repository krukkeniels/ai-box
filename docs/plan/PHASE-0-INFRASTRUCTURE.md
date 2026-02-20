# Phase 0: Infrastructure Foundation

**Phase**: 0 of 6
**Estimated Duration**: 4-5 engineer-weeks (2 engineers, ~2.5 calendar weeks)
**Status**: Not Started

---

## Overview

Phase 0 establishes the central infrastructure that every subsequent phase depends on: image registry, artifact mirrors, base image pipeline, and image signing. Nothing else can proceed without these.

Outputs are purely infrastructure -- no developer-facing features yet.

---

## Deliverables

### D1: Harbor Registry

Deploy Harbor 2.x at `harbor.internal:443` with RBAC, Trivy scanning, and replication.

**Acceptance Criteria**:
- Harbor reachable over HTTPS with valid internal CA certificate.
- RBAC configured: `aibox-ci` (push + pull), `aibox-pull` (pull-only), `aibox-admin`.
- Trivy scan-on-push enabled. Vulnerability thresholds: block critical, warn on high.
- At least one replication rule configured for air-gapped distribution or DR.
- Garbage collection scheduled weekly.
- OCI artifact support enabled (for tool pack manifests and policy bundles).

### D2: Nexus Repository Mirrors

Configure Nexus 3.x as caching proxy for npm, Maven Central, PyPI, NuGet, Go modules, and Cargo.

**Acceptance Criteria**:
- Proxy + group repositories created for each format.
- Validation commands succeed through Nexus: `pip install requests`, `npm install express`, `mvn dependency:resolve`.
- Cleanup policies configured (90-day unused artifact eviction).
- Anonymous read access enabled for proxy repositories.

### D3: NuGet Mirror Validation

Explicit validation that .NET package restoration works through the Nexus proxy.

**Acceptance Criteria**:
- `dotnet restore` succeeds through Nexus for a real .NET project with multiple NuGet dependencies.
- `nuget.config` template produced with Nexus mirror URL pre-configured for use in the `dotnet@8` tool pack.
- NuGet feed discovery and authentication work correctly through the proxy.

### D4: Base Image Build Pipeline

CI/CD pipeline that builds, scans, signs, and pushes container images from the `aibox-images` Git repository.

**Acceptance Criteria**:
- `aibox-base:24.04` builds reproducibly from a Containerfile.
- Base image includes: Ubuntu 24.04 minimal, ca-certificates, git, ssh-server, jq, yq, vim, nano, bash, zsh, tmux, build-essential, python3, curl.
- Placeholder directories for aibox-agent and aibox-llm-proxy.
- Image variants built: `aibox-java:21-24.04`, `aibox-node:20-24.04`, `aibox-dotnet:8-24.04`, `aibox-full:24.04`.
- All images tagged with stable tag (`24.04`) and date tag (`24.04-20260218`).
- Trivy scan passes with zero critical CVEs; high CVEs documented with justification.
- Pipeline triggers on commit to `main` and on weekly schedule.

### D5: Cosign Image Signing

All images signed at build time using Cosign. Client-side verification configured.

**Acceptance Criteria**:
- Cosign key pair generated; private key stored in CI secrets (upgrade to Vault/HSM later).
- CI pipeline signs every image after push.
- Podman `policy.json` rejects unsigned images from `harbor.internal`.
- `podman pull harbor.internal/aibox/base:24.04` succeeds when signed, fails when unsigned.
- Public key packaged for distribution to developer machines.

### D6: Weekly Rebuild Pipeline

Scheduled CI pipeline that rebuilds all image variants weekly with latest OS patches.

**Acceptance Criteria**:
- Triggers weekly (Monday 02:00 UTC, configurable) and on manual dispatch.
- Pulls fresh Ubuntu 24.04 base layers.
- New images receive date tag and are signed.
- Critical CVE failures block the push.

### D7: Git Repository Structure

**Acceptance Criteria**:
- `aibox-images` repository with Containerfiles and CI pipeline definitions.
- `aibox-policies` repository with placeholder org baseline policy.
- `aibox-toolpacks` repository with placeholder manifest structure.
- All repos require PR review and signed commits.

---

## Implementation Steps

### Work Stream 1: Harbor Setup (3-4 days)

1. Provision server (4 CPU, 16GB RAM, 2TB SSD).
2. Install Harbor via offline installer or Helm chart. Configure HTTPS with internal CA cert.
3. Create project `aibox`. Create robot accounts: `aibox-ci` (push+pull), `aibox-pull` (pull-only).
4. Enable Trivy scanning with scan-on-push.
5. Configure replication rule for DR/air-gapped distribution.
6. Schedule weekly garbage collection.
7. Enable OCI artifact support.

**Spec References**: Section 6, Section 17.4, Section 23.

### Work Stream 2: Nexus Configuration (3-4 days)

1. Assess existing infrastructure -- reuse existing Nexus/Artifactory if available.
2. Create proxy repositories: npm, Maven Central, Gradle Plugin Portal, PyPI, NuGet (`https://api.nuget.org/v3/index.json`), Go modules, Cargo.
3. Create group repositories for each format.
4. Configure cleanup policies (90-day unused eviction).
5. Configure access control (anonymous read for proxy repos).
6. Test each mirror from a clean machine. For NuGet specifically: create a test .NET project and run `dotnet restore` through the proxy to validate feed discovery and package resolution.
7. Produce `nuget.config` template for the dotnet tool pack.
8. Document mirror URLs for injection into container configs in Phase 2.

**Spec References**: Section 6, Section 8.6, Section 23.

### Work Stream 3: Base Image Pipeline (4-5 days)

1. Create `aibox-images` Git repository:
   ```
   aibox-images/
     base/Containerfile
     java/Containerfile        # FROM harbor.internal/aibox/base:24.04
     node/Containerfile
     dotnet/Containerfile      # .NET SDK 8
     full/Containerfile
     ci/pipeline.yaml
     scripts/build.sh
     scripts/sign.sh
     scripts/scan-check.sh
   ```
2. Write base Containerfile (Ubuntu 24.04 + essential tools + non-root user + SSH + placeholder dirs).
3. Write variant Containerfiles (Java 21, Node 20, .NET SDK 8, full).
4. Configure CI pipeline: lint, build with Buildah (rootless), push to Harbor, sign with Cosign, Trivy scan, fail on critical CVEs.
5. Test end-to-end: push a change, verify image in Harbor, signed, Trivy-scanned.
6. Validate image pull on developer machine.

**Spec References**: Section 17.1-17.3, Section 17.6, Section 22.2.

### Work Stream 4: Image Signing (2-3 days)

1. Generate Cosign key pair. Store private key in CI secrets.
2. Integrate signing into CI pipeline (sign after every `buildah push`).
3. Create Podman client-side policy (`/etc/containers/policy.json`) enforcing sigstore verification for `harbor.internal`.
4. Create registries configuration (`/etc/containers/registries.d/harbor.yaml`).
5. Test: signed image pull succeeds, unsigned/tampered pull fails.
6. Document key rotation and backup procedures.

**Spec References**: Section 17.5, Section 23.

---

## Research Required

### R1: Existing Artifact Mirror Infrastructure
Audit whether the org already has Nexus/Artifactory. Reuse if possible.

### R2: CI/CD Platform Selection
Determine which CI/CD platform to use. Must support Buildah (rootless), Cosign signing, and scheduled triggers.

### R3: Cosign Key Management
Key-based signing (works air-gapped) vs keyless (requires Sigstore infrastructure). Key-based recommended for classified environments.

### R4: Internal CA Certificate Distribution
How will Harbor's TLS cert be distributed to developer machines? Leverage existing org cert distribution if available.

### R5: Network Connectivity to Upstream Registries
Verify Harbor/Nexus have outbound access to upstream registries. Plan for pre-loading via approved media if restricted.

### R6: NuGet Proxy Specifics
NuGet has specific feed configuration requirements that differ from npm/Maven. Validate that Nexus NuGet proxy supports V3 feed protocol and service index discovery.

---

## Open Questions

- **Q1**: Does the org already have a Nexus or Artifactory instance for reuse? -- *Infrastructure/DevOps team lead*
- **Q2**: What CI/CD platform should the build pipeline use? -- *Engineering leadership*
- **Q3**: Can Harbor/Nexus reach upstream registries directly, or through a corporate proxy? -- *Network/security team*
- **Q4**: What is the internal CA process for issuing TLS certificates? -- *IT/PKI team*

---

## Dependencies

### External Dependencies

| Dependency | Provider | Needed By |
|-----------|----------|-----------|
| Server/VM for Harbor (4 CPU, 16GB RAM, 2TB SSD) | Infrastructure team | Week 1 start |
| Server/VM for Nexus if new (4 CPU, 16GB RAM, 1TB SSD) | Infrastructure team | Week 1 start |
| DNS records for `harbor.internal`, `nexus.internal` | Network/DNS team | Before go-live |
| TLS certificates | PKI/IT team | Before go-live |
| Internal CA root cert bundle | PKI/IT team | Before client testing |
| Git server access for new repos | Git/DevOps team | Week 1 start |
| CI/CD runner capacity (must support Buildah/Podman) | DevOps team | Before pipeline development |
| Outbound network access from Harbor/Nexus | Network/security team | Before mirror testing |

### Internal Dependencies

Phase 0 is the foundation. It depends on no other phase. It must be completed before Phase 1 can begin.

---

## Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|-----------|--------|-----------|
| P0-R1 | Server provisioning delayed | Medium | High (blocks project) | Request early. Fallback: run Harbor/Nexus as containers temporarily. |
| P0-R2 | DNS/TLS provisioning delayed | Medium | Medium | Pre-request in parallel. Use self-signed certs for initial testing. |
| P0-R3 | Harbor/Nexus cannot reach upstream registries | Medium | High | Research network topology early (R5). Plan for pre-loading via approved media. |
| P0-R4 | Trivy finds critical CVEs in Ubuntu 24.04 base | Low | Medium | Document and accept with justification. Use Ubuntu's security pocket. |
| P0-R5 | CI/CD platform cannot run Buildah rootless | Medium | Medium | Research early (R2). Alternatives: Podman build, Kaniko, dedicated build server. |
| P0-R6 | NuGet proxy configuration issues | Medium | Medium | NuGet V3 feed discovery through proxy needs explicit testing. Validate early with `dotnet restore`. |

---

## Exit Criteria

Phase 0 is complete when ALL of the following are true:

1. `podman pull harbor.internal/aibox/base:24.04` succeeds from a developer machine.
2. Signed image pull succeeds; unsigned/tampered image pull is rejected.
3. Nexus mirrors functional: npm, Maven, PyPI proxying works.
4. `dotnet restore` succeeds through Nexus NuGet proxy.
5. Commit to `aibox-images` triggers automated build, scan, sign, push pipeline.
6. Weekly rebuild pipeline scheduled and has run at least once.
7. Trivy scan passes with zero critical CVEs.
8. Image variants exist in Harbor: `aibox-java:21-24.04`, `aibox-node:20-24.04`, `aibox-dotnet:8-24.04`, `aibox-full:24.04`.
9. Git repos created with branch protection and signed-commit requirements.
10. Infrastructure setup documentation written.

---

## Estimated Effort

| Work Stream | Effort | Engineers |
|-------------|--------|-----------|
| Harbor Setup (WS1) | 3-4 days | 1 |
| Nexus Configuration + NuGet Validation (WS2) | 3-4 days | 1 |
| Base Image Pipeline incl. .NET variant (WS3) | 4-5 days | 1 |
| Image Signing (WS4) | 2-3 days | 1 |
| Git Repository Setup (D7) | 0.5 day | 1 |
| Testing & Validation | 2-3 days | 1-2 |
| Documentation | 1-2 days | 1 |
| **Total** | **~4-5 engineer-weeks** | **2 engineers, ~2.5 calendar weeks** |

**Parallelization**: WS1 (Harbor) and WS2 (Nexus) run in parallel from day one. WS3 (pipeline) depends on Harbor. WS4 (signing) begins key generation in parallel but needs pipeline for integration.
