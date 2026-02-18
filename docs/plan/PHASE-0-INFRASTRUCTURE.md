# Phase 0: Infrastructure Foundation

**Phase**: 0 of 6
**Estimated Duration**: Weeks 1-2
**Team**: 2 platform engineers
**Status**: Not Started

---

## Overview

Phase 0 establishes the central infrastructure services that every subsequent phase depends on. Nothing else can proceed without a working image registry, artifact mirrors, a reproducible base image build pipeline, and image signing. This phase is first because it produces the container images that Phase 1 needs to launch sandboxes, the package mirrors that Phase 2 needs for proxied builds, and the signing infrastructure that ensures supply chain integrity from day one.

The outputs of this phase are purely infrastructure -- no developer-facing features yet. The audience is the platform engineering team and the systems that consume images and packages.

---

## Deliverables

### D1: Harbor Registry Deployment

**Output**: A running Harbor 2.x instance at `harbor.internal:443` with RBAC, Trivy scanning, and replication configured.

**Acceptance Criteria**:
- Harbor is reachable over HTTPS with a valid internal CA certificate.
- RBAC is configured: `aibox-push` role (CI pipeline), `aibox-pull` role (developer machines), `aibox-admin` role (platform team).
- Trivy scanning is enabled and runs automatically on every push.
- At least one replication rule is configured (for air-gapped site distribution or DR).
- Garbage collection is scheduled weekly.
- OCI artifact support is enabled (for storing tool pack manifests and policy bundles alongside images).

### D2: Nexus Repository Mirror Configuration

**Output**: Nexus Repository 3.x configured as a caching proxy for npm, Maven Central, PyPI, NuGet, Go modules, and Cargo (crates.io).

**Acceptance Criteria**:
- Each supported format has a proxy repository pointing to its upstream registry.
- Each format has a group repository that clients will use as their single endpoint.
- A `pip install requests`, `npm install express`, and `mvn dependency:resolve` (for a common artifact) succeed via the Nexus mirror.
- Cleanup policies are configured to manage disk usage.
- Anonymous read access enabled for proxy repositories (or a shared read-only token).

### D3: Base Image Build Pipeline

**Output**: A CI/CD pipeline that builds `aibox-base:24.04` from Ubuntu 24.04 LTS, produces image variants, tags with date stamps, and pushes to Harbor.

**Acceptance Criteria**:
- `aibox-base:24.04` builds reproducibly from a Containerfile in the `aibox-images` Git repository.
- Base image includes: Ubuntu 24.04 minimal, ca-certificates, git, ssh-server, jq, yq, vim, nano, bash, zsh, tmux, build-essential, python3, curl.
- Base image includes placeholder directories and configs for aibox-agent and aibox-llm-proxy (binaries added in later phases).
- Image variants build: `aibox-java:21-24.04`, `aibox-node:20-24.04`, `aibox-full:24.04`.
- All images are tagged with both a stable tag (e.g., `24.04`) and a date tag (e.g., `24.04-20260218`).
- Trivy scan passes with zero critical CVEs; high CVEs documented with justification if accepted.
- Pipeline runs on commit to `main` branch of `aibox-images` repo.
- Weekly scheduled rebuild pipeline produces fresh date-tagged images.

### D4: Cosign Image Signing

**Output**: All images are signed at build time using Cosign (Sigstore). Client-side verification is configured.

**Acceptance Criteria**:
- A Cosign key pair is generated and the private key is stored securely (Vault, CI secrets, or HSM).
- The CI pipeline signs every image immediately after push to Harbor.
- A Podman `policy.json` configuration file is produced that rejects unsigned images from `harbor.internal`.
- `podman pull harbor.internal/aibox/base:24.04` succeeds when the image is signed and fails when the signature is missing or invalid.
- The public key is distributed to developer machines (via `aibox setup` in Phase 1, but the key itself is produced now).

### D5: Podman Client-Side Verification Policy

**Output**: A `/etc/containers/policy.json` file that enforces signature verification for images pulled from Harbor.

**Acceptance Criteria**:
- The policy rejects unsigned or incorrectly signed images from `harbor.internal`.
- The policy allows unsigned images from localhost (for local development builds, if applicable).
- The policy is tested: pulling a tampered image fails with a clear error.

### D6: Automated Weekly Rebuild Pipeline

**Output**: A scheduled CI pipeline that rebuilds all image variants weekly, incorporating OS security patches.

**Acceptance Criteria**:
- Pipeline triggers on a weekly schedule (configurable, default: Monday 02:00 UTC).
- Rebuilds pull fresh Ubuntu 24.04 base layers (picks up security patches).
- New images receive a date tag and are signed.
- Trivy scan results are recorded; critical CVE failures block the push.
- Pipeline can also be triggered manually for emergency patches.

### D7: Git Repository Structure

**Output**: Git repositories for images, policies, and tool packs are created with branch protection and signed-commit requirements.

**Acceptance Criteria**:
- `aibox-images` repository exists with Containerfiles and CI pipeline definitions.
- `aibox-policies` repository exists with placeholder org baseline policy.
- `aibox-toolpacks` repository exists with placeholder manifest structure.
- All repos require PR review before merge to `main`.
- All repos require signed commits (GPG or SSH signing).

---

## Implementation Steps

### Work Stream 1: Harbor Setup

**What to build**: Deploy Harbor 2.x as the central OCI image registry for all AI-Box images.

**Steps**:

1. **Provision infrastructure**: Allocate a server or VM meeting the spec's sizing guidance (4 CPU, 16GB RAM, 2TB SSD). Determine whether to deploy on bare metal, a VM, or Kubernetes (Harbor supports both docker-compose and Helm chart deployment).

2. **Install Harbor**: Use the Harbor offline installer or Helm chart. Configure HTTPS with an internal CA certificate. Set the external URL to `harbor.internal:443`.

3. **Configure storage backend**: Decide between local filesystem and S3-compatible object storage (MinIO for on-prem). S3-compatible storage is recommended for scalability and air-gapped replication.

4. **Set up RBAC**:
   - Create project `aibox` for all AI-Box images.
   - Create robot accounts: `aibox-ci` (push + pull, used by CI pipeline), `aibox-pull` (pull-only, distributed to developer machines).
   - Configure LDAP/AD integration if the org uses centralized identity.

5. **Enable Trivy scanning**: Activate the built-in Trivy scanner. Set scan-on-push to enabled. Configure vulnerability severity thresholds (block critical, warn on high).

6. **Configure replication**: Set up at least one replication rule targeting a secondary Harbor instance or an export endpoint for air-gapped distribution.

7. **Schedule garbage collection**: Configure weekly GC to reclaim space from untagged/deleted images.

8. **Enable OCI artifact support**: Ensure Harbor is configured to store arbitrary OCI artifacts (for policy bundles and tool pack manifests in future phases).

**Key Configuration Decisions**:
- Deployment method: docker-compose (simpler, suitable for single-server) vs Helm on K8s (HA, suitable if K8s exists).
- Storage backend: local filesystem vs S3/MinIO.
- Authentication: local users vs LDAP/AD integration.
- TLS: internal CA certificate provisioning and distribution.

**Spec References**: Section 6 (Deployment Model -- central infrastructure sizing), Section 17.4 (Harbor features), Section 23 (Tech Stack Summary).

---

### Work Stream 2: Nexus Configuration

**What to build**: Configure Nexus Repository 3.x as a caching proxy for all package manager registries that developers will need.

**Steps**:

1. **Assess existing infrastructure**: Determine if the organization already has a Nexus or Artifactory instance. If so, add the required proxy repositories. If not, deploy a new Nexus instance (4 CPU, 16GB RAM, 1TB SSD per spec).

2. **Create proxy repositories** for each supported format:
   - **npm**: proxy to `https://registry.npmjs.org`
   - **Maven**: proxy to `https://repo1.maven.org/maven2/`
   - **Gradle Plugin Portal**: proxy to `https://plugins.gradle.org/m2/`
   - **PyPI**: proxy to `https://pypi.org/`
   - **NuGet**: proxy to `https://api.nuget.org/v3/index.json`
   - **Go modules**: proxy to `https://proxy.golang.org/`
   - **Cargo (crates.io)**: proxy to `https://crates.io/` (if Nexus supports it; otherwise document as a gap)

3. **Create group repositories**: For each format, create a group repository that aggregates the proxy + any hosted repositories. This gives clients a single URL.

4. **Configure cleanup policies**: Set up cleanup policies to evict unused cached artifacts after a configurable retention period (e.g., 90 days unused).

5. **Configure access control**: Enable anonymous read access for proxy/group repositories (simplifies container configuration) or create a shared read-only service account.

6. **Test each mirror**: From a clean machine, configure package managers to use the Nexus mirror and verify that installs succeed.

7. **Document mirror URLs**: Produce a reference document listing the Nexus URL for each package format. These URLs will be injected into container configurations in Phase 2.

**Key Configuration Decisions**:
- New Nexus instance vs adding to existing.
- Anonymous read access vs authenticated access (impacts container config complexity).
- Disk allocation and cleanup policies.
- Whether to enable vulnerability scanning in Nexus (Nexus IQ) or rely solely on Trivy in Harbor for image-level scanning.

**Spec References**: Section 6 (Deployment Model -- Nexus sizing), Section 8.6 (Package Manager Proxying), Section 23 (Tech Stack Summary).

---

### Work Stream 3: Base Image Pipeline

**What to build**: A CI/CD pipeline that produces signed, scanned container images from Containerfiles stored in the `aibox-images` Git repository.

**Steps**:

1. **Create the `aibox-images` Git repository**: Initialize with a directory structure:
   ```
   aibox-images/
     base/
       Containerfile
     java/
       Containerfile       # FROM harbor.internal/aibox/base:24.04
     node/
       Containerfile
     full/
       Containerfile
     ci/
       pipeline.yaml       # CI/CD definition (GitLab CI, GitHub Actions, Jenkins, etc.)
     scripts/
       build.sh
       sign.sh
       scan-check.sh
   ```

2. **Write the base Containerfile** (`base/Containerfile`):
   ```dockerfile
   FROM docker.io/library/ubuntu:24.04

   # OS packages
   RUN apt-get update && apt-get install -y --no-install-recommends \
       ca-certificates git openssh-server jq vim nano bash zsh tmux \
       build-essential python3 python3-pip curl wget \
       && rm -rf /var/lib/apt/lists/*

   # Install yq
   RUN curl -sSL https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 \
       -o /usr/local/bin/yq && chmod +x /usr/local/bin/yq

   # Create non-root user
   RUN useradd -m -s /bin/bash -u 1000 dev

   # SSH server config
   RUN mkdir -p /run/sshd
   COPY sshd_config /etc/ssh/sshd_config

   # Placeholder directories for aibox components (binaries added in later phases)
   RUN mkdir -p /etc/aibox /opt/toolpacks /workspace
   COPY policy-default.yaml /etc/aibox/policy.yaml

   # Set workspace as working directory
   WORKDIR /workspace
   USER dev

   EXPOSE 22
   ```
   *Note: This is a starting point. Actual Containerfile will be refined during implementation.*

3. **Write variant Containerfiles**: Each variant extends the base image and adds language-specific tooling (JDK, Node.js, etc.).

4. **Configure the CI pipeline**:
   - Trigger: on push to `main` branch, on weekly schedule, on manual dispatch.
   - Steps: lint Containerfile, build with Buildah (rootless), push to Harbor, sign with Cosign, run Trivy scan, fail on critical CVEs.
   - Tags: apply both stable tag and date tag.

5. **Integrate Cosign signing** into the pipeline (see Work Stream 4).

6. **Test the pipeline end-to-end**: Push a change, verify the image appears in Harbor, is signed, and passes the Trivy scan.

7. **Validate image pull on a developer machine**: `podman pull harbor.internal/aibox/base:24.04` succeeds, signature is verified, image runs.

**Key Configuration Decisions**:
- CI/CD platform: GitLab CI, GitHub Actions, Jenkins, or other. Must support Buildah (rootless) or Podman for building.
- Build tool: Buildah (recommended by spec) vs Podman build vs Kaniko.
- Whether to use multi-stage builds to minimize image size.
- Whether to pre-install VS Code Server in the base image (spec recommends it; decide if it belongs in Phase 0 or Phase 4).

**Spec References**: Section 17.1-17.3 (Base image, variants, contents), Section 17.6 (Update cadence), Section 22.2 (Image updates).

---

### Work Stream 4: Image Signing (Cosign)

**What to build**: Cosign key infrastructure and integration into the build pipeline, plus client-side verification configuration.

**Steps**:

1. **Generate Cosign key pair**:
   ```bash
   cosign generate-key-pair
   ```
   Store the private key securely: in the CI system's secret store, in Vault (if available), or in an HSM. The public key will be distributed to all developer machines.

2. **Integrate signing into CI pipeline**: After each `buildah push`, run:
   ```bash
   cosign sign --key <private-key> harbor.internal/aibox/<image>:<tag>
   ```

3. **Create Podman client-side policy** (`/etc/containers/policy.json`):
   ```json
   {
     "default": [{"type": "insecureAcceptAnything"}],
     "transports": {
       "docker": {
         "harbor.internal": [
           {
             "type": "sigstoreSigned",
             "keyPath": "/etc/aibox/cosign.pub"
           }
         ]
       }
     }
   }
   ```

4. **Create registries configuration** (`/etc/containers/registries.d/harbor.yaml`):
   ```yaml
   docker:
     harbor.internal:
       use-sigstore-attachments: true
   ```

5. **Test verification**: Pull a signed image (should succeed). Tamper with or remove the signature and pull again (should fail).

6. **Document key management procedures**: How to rotate the Cosign key pair. Who has access. Backup and recovery plan.

**Key Configuration Decisions**:
- Key storage: CI secrets vs Vault vs HSM. HSM is most secure but adds complexity; CI secrets are acceptable for initial deployment.
- Keyless signing (Sigstore Fulcio + Rekor) vs key-based signing. Keyless requires network access to Sigstore public infrastructure, which may not be available in classified environments. **Key-based is recommended for classified environments.**
- Whether to use a transparency log (Rekor). Requires network access; skip for air-gapped.

**Spec References**: Section 17.5 (Image Signing), Section 23 (Tech Stack Summary -- Cosign/Sigstore).

---

## Research Required

### R1: Harbor Deployment Method
**What**: Evaluate docker-compose vs Helm chart deployment for Harbor. Consider whether the organization has existing Kubernetes infrastructure.
**Why**: Impacts HA, operational complexity, and alignment with existing infrastructure patterns.
**Owner**: Platform engineer assigned to Work Stream 1.

### R2: Existing Artifact Mirror Infrastructure
**What**: Audit whether the organization already has a Nexus or Artifactory instance. Determine if it can be reused or if a dedicated instance is needed.
**Why**: Reusing existing infrastructure avoids duplication and leverages existing knowledge. A dedicated instance avoids contention and policy conflicts.
**Owner**: Platform engineer assigned to Work Stream 2.

### R3: CI/CD Platform Selection
**What**: Determine which CI/CD platform to use for the image build pipeline. Evaluate GitLab CI, GitHub Actions, Jenkins, and any org-standard platform.
**Why**: The pipeline must support Buildah (rootless), Cosign signing, and scheduled triggers. Not all platforms support all of these natively.
**Owner**: Platform team lead.

### R4: Cosign Key Management Strategy
**What**: Decide between key-based signing (simpler, works air-gapped) and keyless signing (requires Sigstore infrastructure). If key-based, decide where to store the private key.
**Why**: Classified environments likely cannot use public Sigstore infrastructure. Key rotation and backup procedures must be defined before keys are generated.
**Owner**: Security team + platform team lead.

### R5: Internal CA Certificate Distribution
**What**: Determine how the Harbor HTTPS certificate (signed by the internal CA) will be distributed to developer machines. Evaluate whether the org already has a certificate distribution mechanism.
**Why**: Every developer machine needs to trust Harbor's certificate. Without this, `podman pull` fails with TLS errors.
**Owner**: Platform engineer assigned to Work Stream 1, in coordination with IT/infrastructure team.

### R6: Cargo/crates.io Mirror Support
**What**: Verify whether Nexus Repository 3.x supports Cargo (crates.io) proxying natively or if an alternative is needed.
**Why**: The spec lists Cargo as a supported registry. Nexus support for Cargo may be limited or require a plugin.
**Owner**: Platform engineer assigned to Work Stream 2.

### R7: Network Connectivity to Upstream Registries
**What**: Verify that the Nexus server has outbound network access to upstream package registries (npmjs.org, Maven Central, PyPI, etc.) and that Harbor can pull base images from Docker Hub.
**Why**: In some classified environments, even the mirror servers may have restricted outbound access. Alternative: pre-load packages via approved media transfer.
**Owner**: Platform team + network/security team.

---

## Open Questions

- **Q1**: Does the organization already have a Nexus or Artifactory instance that can be reused for AI-Box package mirroring? -- *Who should answer*: Infrastructure/DevOps team lead

- **Q2**: What CI/CD platform should the image build pipeline use? Is there an organizational standard? -- *Who should answer*: Engineering leadership / DevOps team

- **Q3**: Does the Harbor server need HA (active-active or active-passive) from day one, or is a single-instance deployment acceptable for Phase 0 with HA deferred? -- *Who should answer*: Platform team lead + security team

- **Q4**: Can the Nexus and Harbor servers reach upstream registries (Docker Hub, npmjs.org, Maven Central, PyPI) directly, or do they need to go through an existing corporate proxy? -- *Who should answer*: Network/security team

- **Q5**: What is the internal CA process for issuing TLS certificates for `harbor.internal` and `nexus.internal`? Is there an automated mechanism (ACME, cert-manager) or is it a manual request? -- *Who should answer*: IT/PKI team

- **Q6**: Where should the Cosign private key be stored? Is there an existing HSM or secrets management system (Vault) available for this purpose? -- *Who should answer*: Security team

- **Q7**: Are there any compliance or change-management gates that must be passed before deploying new infrastructure services (Harbor, Nexus) to the production network? -- *Who should answer*: Compliance/change management team

- **Q8**: What base OS hardening standards (CIS benchmarks, STIGs) apply to the Harbor and Nexus servers themselves? -- *Who should answer*: Security team

---

## Dependencies

### External Dependencies (Things We Need From Others)

| Dependency | Provider | Needed By | Notes |
|-----------|----------|-----------|-------|
| Server/VM allocation for Harbor | Infrastructure team | Week 1 start | 4 CPU, 16GB RAM, 2TB SSD minimum |
| Server/VM allocation for Nexus (if new) | Infrastructure team | Week 1 start | 4 CPU, 16GB RAM, 1TB SSD minimum |
| DNS records for `harbor.internal`, `nexus.internal` | Network/DNS team | Before Harbor/Nexus go live | Internal DNS zone entries |
| TLS certificates for Harbor and Nexus | PKI/IT team | Before Harbor/Nexus go live | Signed by internal CA |
| Internal CA root certificate bundle | PKI/IT team | Before client testing | Needed on developer machines for trust |
| Git server access for new repositories | Git/DevOps team | Week 1 start | `aibox-images`, `aibox-policies`, `aibox-toolpacks` repos |
| CI/CD pipeline access and runner capacity | DevOps team | Before pipeline development | Runner must support Buildah/Podman |
| Outbound network access from Harbor/Nexus to upstream registries | Network/security team | Before mirror testing | Docker Hub, npmjs.org, Maven Central, PyPI, etc. |
| Cosign private key storage location | Security team | Before first image signing | Vault, CI secrets, or HSM |

### Internal Dependencies (Phase 0 Has No Upstream Phase Dependencies)

Phase 0 is the foundation. It depends on no other phase. However, it must be completed before Phase 1 can begin (Phase 1 needs to pull images from Harbor).

---

## Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|-----------|--------|-----------|
| P0-R1 | Server provisioning delayed by infrastructure team | Medium | High (blocks entire project) | Request servers immediately. Have fallback: run Harbor/Nexus as containers on a developer machine temporarily. |
| P0-R2 | DNS and TLS certificate provisioning delayed | Medium | Medium (can work with IP addresses temporarily) | Pre-request DNS and certs in parallel with server provisioning. Use self-signed certs for initial testing. |
| P0-R3 | Harbor or Nexus cannot reach upstream registries due to network restrictions | Medium | High (mirrors cannot populate) | Research network topology early (R7). Have a plan for pre-loading packages via approved media transfer. |
| P0-R4 | Trivy scan finds critical CVEs in Ubuntu 24.04 base that cannot be patched | Low | Medium (delays base image approval) | Document and accept with justification. Use Ubuntu's security pocket. Consider minimal/distroless base as alternative. |
| P0-R5 | Cosign key management process not established, blocks signing | Low | Medium (images can be built but not signed) | Generate keys early. Even a simple key-in-CI-secrets approach works for Phase 0. Improve key management later. |
| P0-R6 | Nexus does not support all required package formats (e.g., Cargo) | Low | Low (Cargo is niche; can be added later) | Research Nexus format support early (R6). Document gaps. Find alternatives for unsupported formats. |
| P0-R7 | CI/CD platform cannot run Buildah rootless | Medium | Medium (must find alternative build method) | Research CI platform capabilities early (R3). Alternatives: Podman build, Kaniko, or a dedicated build server. |

---

## Exit Criteria

Phase 0 is complete when ALL of the following are true:

1. **Harbor is operational**: `podman pull harbor.internal/aibox/base:24.04` succeeds from a developer machine (or a test machine on the same network).

2. **Image signature verification works**: Pulling a signed image succeeds. Pulling an unsigned or tampered image from `harbor.internal` is rejected by client-side policy.

3. **Nexus mirrors are functional**: At minimum, npm, Maven Central, and PyPI proxying works. A `pip install requests`, `npm install express`, and `mvn dependency:resolve` (for a well-known artifact) all succeed via the Nexus mirror URLs.

4. **Base image builds are automated**: A commit to the `aibox-images` repository triggers a pipeline that builds, scans, signs, and pushes the base image to Harbor without manual intervention.

5. **Weekly rebuild pipeline is scheduled**: A scheduled pipeline exists and has run at least once successfully, producing a date-tagged image.

6. **Trivy scan passes**: The latest base image has zero critical CVEs. Any high CVEs are documented with justification.

7. **Image variants exist**: `aibox-java:21-24.04`, `aibox-node:20-24.04`, and `aibox-full:24.04` are built, signed, and available in Harbor.

8. **Git repositories are created**: `aibox-images`, `aibox-policies`, and `aibox-toolpacks` repos exist with branch protection and signed-commit requirements.

9. **Documentation is written**: Infrastructure setup guide covers Harbor deployment, Nexus configuration, build pipeline operation, and Cosign key management.

---

## Estimated Effort

| Work Stream | Estimated Effort | Engineers | Notes |
|-------------|-----------------|-----------|-------|
| Harbor Setup (WS1) | 3-4 days | 1 | Includes provisioning, install, RBAC, Trivy, replication |
| Nexus Configuration (WS2) | 2-3 days | 1 | Faster if reusing existing Nexus; longer if deploying new |
| Base Image Pipeline (WS3) | 4-5 days | 1 | Containerfiles + CI pipeline + testing |
| Image Signing (WS4) | 2-3 days | 1 | Key generation, CI integration, client policy, testing |
| Git Repository Setup (D7) | 0.5 day | 1 | Quick but must be done early |
| Testing & Validation | 2-3 days | 1-2 | End-to-end testing of all deliverables |
| Documentation | 1-2 days | 1 | Setup guide, mirror URLs reference, key management docs |
| **Total** | **~3-4 engineer-weeks** | **2 engineers** | **~2 calendar weeks with 2 engineers working in parallel** |

**Parallelization**: Work Streams 1 and 2 (Harbor and Nexus) can proceed in parallel from day one. Work Stream 3 (build pipeline) depends on Harbor being ready to receive pushes. Work Stream 4 (signing) can begin key generation in parallel but needs the pipeline from WS3 for integration testing.

**Suggested schedule**:
- **Days 1-2**: Git repos created (D7). Harbor install begins (WS1). Nexus install begins (WS2). Cosign key generation (WS4).
- **Days 3-5**: Harbor RBAC/Trivy/replication configured (WS1). Nexus mirrors tested (WS2). Base Containerfile written (WS3).
- **Days 6-8**: CI pipeline development (WS3). Cosign integration into pipeline (WS4). Harbor and Nexus validated.
- **Days 9-10**: End-to-end testing. Image variants built. Documentation written. Exit criteria verified.
