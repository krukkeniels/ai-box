# Phase 0: Infrastructure Setup Guide

This guide walks platform engineers through deploying the AI-Box Phase 0 infrastructure end-to-end: Harbor registry, Nexus package mirrors, Cosign image signing, base image pipeline, and weekly rebuild automation.

---

## 1. Prerequisites

### Server Requirements

| Service | CPU | RAM | Disk | Purpose |
|---------|-----|-----|------|---------|
| Harbor  | 4 cores | 16 GB | 2 TB SSD | Container image registry |
| Nexus   | 4 cores | 16 GB | 1 TB SSD | Package manager caching proxy |

A single CI runner with Buildah, Cosign, and Trivy is also required (can be a shared runner with those tools installed).

### Software Requirements

Install the following on the appropriate machines:

| Tool | Where Needed | Purpose |
|------|-------------|---------|
| Docker or Podman | Harbor server | Container runtime for Harbor components |
| docker-compose v2+ | Harbor server | Orchestrate Harbor services |
| Buildah | CI runner | Build container images (rootless) |
| Cosign | CI runner, admin workstation | Sign and verify container images |
| Trivy | CI runner | Vulnerability scanning |
| hadolint | CI runner | Containerfile linting |
| curl, jq | All servers | API scripting |

### Network Requirements

- **Outbound access** from Harbor server to Docker Hub (`docker.io`) to pull Ubuntu base images.
- **Outbound access** from Nexus server to upstream registries: `registry.npmjs.org`, `repo1.maven.org`, `plugins.gradle.org`, `pypi.org`, `api.nuget.org`, `proxy.golang.org`, `static.crates.io`.
- **DNS records**: `harbor.internal` pointing to the Harbor server, `nexus.internal` pointing to the Nexus server.
- **Ports**: Harbor uses 80 and 443. Nexus uses 8081 (HTTP) and optionally 8443 (HTTPS).
- If servers are behind a corporate proxy, configure the proxy settings in `harbor.yml` (proxy section) and in Nexus's JVM options.

### TLS Certificates

Harbor requires TLS certificates from the internal CA:

- Place the certificate at `/etc/harbor/tls/cert.pem`
- Place the private key at `/etc/harbor/tls/key.pem`

The internal CA root certificate must also be distributed to all machines that pull images from Harbor, or `podman pull` will fail with TLS errors.

### Git Repository Access

Three Git repositories must be created with branch protection and signed-commit requirements:

- `aibox-images` -- Containerfiles and CI pipeline definitions
- `aibox-policies` -- Organization baseline policy (placeholder)
- `aibox-toolpacks` -- Tool pack manifest structure (placeholder)

The repository structure for `aibox-images` is:

```
aibox-images/
  base/Containerfile
  base/sshd_config
  base/policy-default.yaml
  java/Containerfile
  node/Containerfile
  full/Containerfile
  ci/build-and-publish.yml
  ci/weekly-rebuild.yml
  scripts/build.sh
  scripts/sign.sh
  scripts/scan-check.sh
  scripts/lint.sh
```

---

## 2. Deployment Order

Follow this sequence. Each step depends on the previous ones completing.

1. **Git repositories** -- Create `aibox-images`, `aibox-policies`, `aibox-toolpacks` with branch protection.
2. **Harbor** -- Deploy the registry so images have a destination.
3. **Nexus** -- Deploy the package mirror (can run in parallel with Harbor).
4. **Cosign keys** -- Generate the signing key pair and store the private key securely.
5. **Base images** -- Build, sign, and push `aibox-base:24.04` and all variants.
6. **CI pipeline** -- Configure the build-and-publish workflow for automated builds on push to `main`.
7. **Weekly rebuild** -- Enable the scheduled weekly-rebuild workflow.
8. **Verification** -- Run the exit criteria checklist to confirm everything works.

---

## 3. Harbor Deployment

All Harbor configuration files are in `infra/harbor/`.

### 3.1 Install Harbor

Review and customize `infra/harbor/harbor.yml` before running the installer. Key settings to change:

- `harbor_admin_password` -- replace `CHANGE_ME` with a strong password
- `database.password` -- replace `CHANGE_ME_DB_PASSWORD`
- TLS certificate paths (defaults to `/etc/harbor/tls/cert.pem` and `key.pem`)
- Storage backend (local filesystem by default; uncomment the S3/MinIO section for object storage)
- Proxy settings (uncomment if outbound access goes through a corporate proxy)

Run the installer:

```bash
cd infra/harbor
HARBOR_PASS='<your-admin-password>' ./install.sh
```

The script will:
1. Check prerequisites (container runtime, docker-compose, disk space, ports, TLS certs)
2. Download the Harbor v2.11.2 offline installer
3. Apply the `harbor.yml` configuration
4. Install Harbor with Trivy scanning enabled (`--with-trivy`)
5. Wait for Harbor to become ready
6. Run RBAC setup and GC scheduling automatically

To skip the download (if the tarball is already present):

```bash
HARBOR_PASS='<password>' SKIP_DOWNLOAD=true ./install.sh
```

### 3.2 Verify Harbor is Running

```bash
# Check the API
curl -sSf -k https://harbor.internal/api/v2.0/systeminfo | jq .

# Check the web UI
# Navigate to https://harbor.internal in a browser
# Log in with admin / <HARBOR_PASS>
```

### 3.3 RBAC Setup

The installer runs `rbac-setup.sh` automatically. If you need to run it separately:

```bash
HARBOR_URL=https://harbor.internal \
HARBOR_USER=admin \
HARBOR_PASS='<password>' \
  ./rbac-setup.sh
```

This creates:
- Project `aibox` with auto-scan enabled (scan-on-push, block critical CVEs)
- Robot account `robot$aibox-ci` (push + pull, for CI pipeline)
- Robot account `robot$aibox-pull` (pull-only, for developer machines)

**Save the robot account secrets** printed during creation. They are shown only once.

### 3.4 Configure Replication (Optional)

For DR or air-gapped distribution, set up replication to a secondary Harbor instance:

```bash
HARBOR_URL=https://harbor.internal \
HARBOR_USER=admin \
HARBOR_PASS='<password>' \
TARGET_URL=https://harbor-dr.internal \
TARGET_USER=admin \
TARGET_PASS='<dr-password>' \
  ./replication-setup.sh
```

Supported trigger modes (set via `REPLICATION_TRIGGER`):
- `event_based` (default) -- replicate on every push
- `scheduled` -- replicate on a cron schedule
- `manual` -- replicate only when triggered

### 3.5 Garbage Collection

The installer schedules weekly GC automatically (Sunday 03:00 UTC). To configure manually:

```bash
HARBOR_URL=https://harbor.internal \
HARBOR_USER=admin \
HARBOR_PASS='<password>' \
  ./gc-schedule.sh
```

To run a dry-run first: `GC_DRY_RUN=true ./gc-schedule.sh`

Verify the schedule:

```bash
curl -sSf -u admin:'<password>' \
  https://harbor.internal/api/v2.0/system/gc/schedule | jq .
```

---

## 4. Nexus Deployment

All Nexus configuration files are in `infra/nexus/`.

### 4.1 Start Nexus

```bash
cd infra/nexus
docker compose up -d
```

Nexus runs as `sonatype/nexus3:3.75.1` with 4g-12g heap allocation. First startup takes 1-2 minutes. Wait for it to become healthy:

```bash
# Check health
curl -sf http://nexus.internal:8081/service/rest/v1/status
```

On first startup, retrieve the admin password:

```bash
docker exec nexus cat /nexus-data/admin.password
```

Change this password immediately through the Nexus UI or API.

### 4.2 Configure Repositories

```bash
NEXUS_URL=http://nexus.internal:8081 \
NEXUS_USER=admin \
NEXUS_PASS='<admin-password>' \
  ./configure-repos.sh
```

This creates proxy, hosted, and group repositories for:
- **npm**: `npm-proxy`, `npm-hosted`, `npm-group`
- **Maven**: `maven-central-proxy`, `maven-hosted`, `maven-group`
- **Gradle Plugin Portal**: `gradle-plugins-proxy`
- **PyPI**: `pypi-proxy`, `pypi-hosted`, `pypi-group`
- **NuGet**: `nuget-proxy`, `nuget-group`
- **Go modules**: `go-proxy` (raw format)
- **Cargo**: `cargo-proxy` (raw format, limited -- see note below)

Anonymous read access is enabled for all proxy/group repositories.

**Note on Cargo**: Nexus 3.x does not natively support the Cargo sparse registry protocol. The `cargo-proxy` is a raw proxy for `.crate` file caching only. See [nexus-mirror-urls.md](nexus-mirror-urls.md) for workarounds.

### 4.3 Configure Cleanup Policies

```bash
NEXUS_URL=http://nexus.internal:8081 \
NEXUS_USER=admin \
NEXUS_PASS='<admin-password>' \
  ./cleanup-policies.sh
```

This creates:
- Cleanup policy `evict-unused-proxy-90d` (evict cached artifacts unused for 90 days)
- Applies the policy to all proxy repositories
- Schedules weekly cleanup task (Sunday 01:00 UTC)
- Schedules weekly compact blob store task (Sunday 03:00 UTC)

### 4.4 Test Mirrors

```bash
NEXUS_URL=http://nexus.internal:8081 ./test-mirrors.sh
```

This verifies each package format by performing real installs through the Nexus proxy. Tests run for npm, Maven, PyPI, Go, and NuGet (requires the respective CLI tools). Set `SKIP_NPM=1`, `SKIP_MAVEN=1`, etc. to skip formats where the CLI is unavailable.

Expected output: `PASS` for each tested format.

### 4.5 Mirror URL Reference

For the full list of mirror URLs and client configuration examples (npm, yarn, Maven, Gradle, pip, NuGet, Go), see [nexus-mirror-urls.md](nexus-mirror-urls.md).

---

## 5. Cosign Setup

All Cosign configuration files are in `infra/cosign/`.

### 5.1 Generate Key Pair

On a trusted workstation:

```bash
cd infra/cosign
./setup-keys.sh --output-dir /tmp/cosign-keygen
```

You will be prompted for a password to encrypt the private key. Choose a strong password (20+ characters).

### 5.2 Store the Private Key Securely

Choose one of the following storage options:

**Option 1: CI Secrets** (simplest, acceptable for initial deployment)
- Store `cosign.key` contents as a CI secret variable `COSIGN_KEY`
- Store the password as `COSIGN_PASSWORD`

**Option 2: HashiCorp Vault** (recommended for production)
```bash
vault kv put secret/aibox/cosign \
  private_key=@/tmp/cosign-keygen/cosign.key \
  password="<key-password>"
```

**Option 3: HSM / Cloud KMS** (highest security, for classified environments)
```bash
cosign generate-key-pair --kms awskms:///alias/aibox-cosign
```

After storing the private key, delete the local copy:

```bash
shred -u /tmp/cosign-keygen/cosign.key
```

### 5.3 Distribute the Public Key

1. Commit `cosign.pub` to the `aibox-images` repository.
2. Install the verification policy on developer machines:

```bash
sudo ./install-policy.sh
```

This installs three files:
- `/etc/containers/policy.json` -- Podman verification policy requiring `sigstoreSigned` for `harbor.internal`
- `/etc/containers/registries.d/harbor.yaml` -- Tells Podman to look for sigstore attachments
- `/etc/aibox/cosign.pub` -- The public key for signature verification

Use `--dry-run` to preview changes without writing files.

### 5.4 Key Rotation and Recovery

For key rotation procedures, emergency revocation, backup/recovery plans, and access control policies, see [cosign-key-management.md](cosign-key-management.md).

Summary of rotation triggers:
- Regular schedule (annually recommended, quarterly for high-security)
- Immediately on suspected key compromise
- When personnel with key access leave the organization

---

## 6. Base Image Pipeline

### 6.1 Image Variants

| Image | Tag | Base | Additions |
|-------|-----|------|-----------|
| `aibox/base` | `24.04` | Ubuntu 24.04 | git, ssh-server, jq, yq, vim, nano, bash, zsh, tmux, build-essential, python3, curl, wget |
| `aibox/java` | `21-24.04` | `aibox/base:24.04` | Eclipse Temurin JDK 21, Maven 3.9.9, Gradle 8.12 |
| `aibox/node` | `20-24.04` | `aibox/base:24.04` | Node.js 20 LTS, Yarn |
| `aibox/full` | `24.04` | `aibox/base:24.04` | JDK 21, Maven, Gradle, Node.js 20, Yarn, python3-venv, Poetry |

All images:
- Run as non-root user `dev` (UID 1000)
- Expose SSH on port 22 (key-only auth, no root login)
- Include placeholder directories `/etc/aibox`, `/opt/toolpacks`, `/workspace`
- Include a default policy at `/etc/aibox/policy.yaml`

### 6.2 Build Scripts

All build scripts are in `aibox-images/scripts/`.

**Build an image manually:**

```bash
./scripts/build.sh <variant> <stable_tag> <registry> [--no-cache]

# Examples:
./scripts/build.sh base 24.04 harbor.internal/aibox
./scripts/build.sh java 21-24.04 harbor.internal/aibox --no-cache
```

The script builds with Buildah, applies both the stable tag and a date tag (`<stable_tag>-YYYYMMDD`), and pushes both tags to the registry.

**Sign an image:**

```bash
COSIGN_KEY=/path/to/cosign.key ./scripts/sign.sh harbor.internal/aibox/base:24.04
```

**Scan an image:**

```bash
./scripts/scan-check.sh harbor.internal/aibox/base:24.04 --report /tmp/trivy-report.json
```

Exit code 0 means no critical CVEs. Exit code 1 means critical CVEs were found (blocks the pipeline).

**Lint all Containerfiles:**

```bash
./scripts/lint.sh
```

Downloads hadolint automatically if not installed.

### 6.3 CI Pipeline (Push to Main)

The workflow at `aibox-images/ci/build-and-publish.yml` triggers on:
- Push to `main` when files in `base/`, `java/`, `node/`, `full/`, or `scripts/` change
- Manual dispatch (can select a single variant or all)

Pipeline steps:
1. **Lint** -- hadolint checks all Containerfiles
2. **Build base** -- Buildah builds `aibox/base:24.04`, pushes to Harbor
3. **Sign base** -- Cosign signs both stable and date tags
4. **Scan base** -- Trivy scan, fail on critical CVEs
5. **Build variants** (parallel: java, node, full) -- same build/sign/scan cycle

Required CI secrets:
- `HARBOR_URL` -- e.g. `harbor.internal`
- `HARBOR_USER` -- e.g. `robot$aibox-ci`
- `HARBOR_PASSWORD` -- the robot account secret from RBAC setup
- `COSIGN_KEY` -- path to the Cosign private key

### 6.4 Weekly Rebuild (Scheduled)

The workflow at `aibox-images/ci/weekly-rebuild.yml` runs:
- **Scheduled**: Monday 02:00 UTC (cron `0 2 * * 1`)
- **Manual dispatch**: with optional `--no-cache` for all builds

The weekly rebuild uses `--no-cache` on the base image to pull fresh Ubuntu 24.04 layers with the latest security patches. Variant images are rebuilt on top of the fresh base.

### 6.5 Verify Images

After a pipeline run, verify images are available and properly signed:

```bash
# Pull an image
podman pull harbor.internal/aibox/base:24.04

# Verify signature
./infra/cosign/verify-image.sh harbor.internal/aibox/base:24.04

# Check Trivy scan results in Harbor UI
# Navigate to: https://harbor.internal/harbor/projects/aibox/repositories
```

Run the full verification suite:

```bash
./infra/cosign/test-verification.sh --image harbor.internal/aibox/base:24.04
```

This tests: signature verification with correct key, rejection with wrong key, policy.json installation, registries.d configuration, public key installation, and Podman policy enforcement.

---

## 7. Verification Checklist

This checklist maps to the Phase 0 exit criteria. All items must pass before moving to Phase 1.

### Exit Criterion 1: Harbor is Operational

```bash
podman pull harbor.internal/aibox/base:24.04
```

Expected: image pulls successfully. If it fails, check DNS resolution, TLS certificate trust, and Harbor service status.

### Exit Criterion 2: Image Signature Verification Works

```bash
# Verify a signed image succeeds
cosign verify --key /etc/aibox/cosign.pub harbor.internal/aibox/base:24.04

# Verify an unsigned image is rejected by policy
# (attempt to pull an image that is not signed -- should fail with policy error)
```

Expected: signed images pull successfully; unsigned/tampered images from `harbor.internal` are rejected.

### Exit Criterion 3: Nexus Mirrors are Functional

```bash
# npm
npm install --registry=http://nexus.internal:8081/repository/npm-group/ express

# pip
pip install --index-url=http://nexus.internal:8081/repository/pypi-group/simple/ \
  --trusted-host=nexus.internal requests

# Maven
mvn -s settings.xml dependency:resolve
# (using settings.xml with nexus mirror -- see nexus-mirror-urls.md)
```

Or run the automated test suite:

```bash
NEXUS_URL=http://nexus.internal:8081 ./infra/nexus/test-mirrors.sh
```

Expected: all tested formats pass.

### Exit Criterion 4: Base Image Builds are Automated

Push a trivial change to `main` in the `aibox-images` repository and verify:
1. The CI pipeline triggers automatically
2. The image is built, scanned, signed, and pushed to Harbor
3. The image appears in the Harbor UI under project `aibox`

### Exit Criterion 5: Weekly Rebuild Pipeline is Scheduled

Verify the weekly rebuild workflow exists and has run at least once:

```bash
# Check workflow configuration
cat aibox-images/ci/weekly-rebuild.yml | grep cron
# Expected: "0 2 * * 1" (Monday 02:00 UTC)

# Trigger a manual run to verify it works
# Use workflow_dispatch in the CI platform
```

### Exit Criterion 6: Trivy Scan Passes

```bash
./scripts/scan-check.sh harbor.internal/aibox/base:24.04
```

Expected: exit code 0, zero critical CVEs. Any high CVEs should be documented with justification.

### Exit Criterion 7: Image Variants Exist

```bash
podman pull harbor.internal/aibox/java:21-24.04
podman pull harbor.internal/aibox/node:20-24.04
podman pull harbor.internal/aibox/full:24.04

# Verify signatures on all variants
cosign verify --key /etc/aibox/cosign.pub harbor.internal/aibox/java:21-24.04
cosign verify --key /etc/aibox/cosign.pub harbor.internal/aibox/node:20-24.04
cosign verify --key /etc/aibox/cosign.pub harbor.internal/aibox/full:24.04
```

Expected: all three variants pull and verify successfully.

### Exit Criterion 8: Git Repositories are Created

Verify the following repositories exist with branch protection and signed-commit requirements:
- `aibox-images`
- `aibox-policies`
- `aibox-toolpacks`

### Exit Criterion 9: Documentation is Written

You are reading it. Confirm that these companion documents also exist:
- [cosign-key-management.md](cosign-key-management.md) -- Key rotation, revocation, backup, access control
- [nexus-mirror-urls.md](nexus-mirror-urls.md) -- Mirror URLs and client configuration examples

---

## 8. Troubleshooting

### TLS Certificate Errors

**Symptom**: `podman pull` or `curl` fails with `x509: certificate signed by unknown authority`.

**Resolution**:
1. Install the internal CA root certificate on the client machine:
   ```bash
   sudo cp internal-ca.crt /usr/local/share/ca-certificates/
   sudo update-ca-certificates
   ```
2. For Podman specifically, you may also need to add the CA cert to `/etc/containers/certs.d/harbor.internal/ca.crt`.
3. Restart any running container runtimes after updating certs.

### Harbor Unreachable

**Symptom**: `curl https://harbor.internal/api/v2.0/systeminfo` fails.

**Resolution**:
1. Check DNS resolution: `dig harbor.internal` or `nslookup harbor.internal`
2. Check Harbor services are running:
   ```bash
   cd /opt/harbor && docker compose ps
   ```
3. Check ports 80 and 443 are not blocked: `ss -tlnp | grep -E ':80|:443'`
4. Check Harbor logs:
   ```bash
   docker compose logs core
   docker compose logs proxy
   ```
5. If Harbor was recently installed, wait up to 120 seconds for it to become ready.

### Harbor Login Fails

**Symptom**: `podman login harbor.internal` returns authentication error.

**Resolution**:
1. Verify the robot account secret was saved correctly during RBAC setup.
2. Re-run RBAC setup to check if accounts exist:
   ```bash
   curl -sSf -u admin:'<password>' \
     https://harbor.internal/api/v2.0/robots | jq '.[].name'
   ```
3. If credentials are lost, delete and recreate the robot account through the Harbor UI.

### Nexus Proxy Failures

**Symptom**: Package installs through Nexus return 502 or timeout.

**Resolution**:
1. Check Nexus health:
   ```bash
   curl -sf http://nexus.internal:8081/service/rest/v1/status
   ```
2. Check proxy repository status in the Nexus UI: Administration > Repositories. A green checkmark means the upstream is reachable.
3. If the upstream is unreachable, check outbound network access from the Nexus server.
4. Check Nexus logs:
   ```bash
   docker compose logs nexus
   ```
5. If a proxy has auto-blocked due to upstream failures, unblock it in the Nexus UI: click the repository, toggle the "Blocked" setting.

### Nexus Disk Space

**Symptom**: Nexus returns errors or runs slowly.

**Resolution**:
1. Check blob store usage in the Nexus UI: Administration > Blob Stores.
2. Manually trigger cleanup: Nexus UI > Administration > System > Tasks > Run "Cleanup unused proxy artifacts".
3. After cleanup, run the compact task to reclaim disk space.
4. Cleanup policies evict artifacts unused for 90 days automatically (Sunday 01:00 UTC).

### Cosign Verification Failures

**Symptom**: `cosign verify` returns `no matching signatures` or `signature mismatch`.

**Resolution**:
1. Verify the public key matches the key used to sign the image:
   ```bash
   cosign verify --key /etc/aibox/cosign.pub harbor.internal/aibox/base:24.04
   ```
2. Check that the registries.d configuration is installed:
   ```bash
   cat /etc/containers/registries.d/harbor.yaml
   # Should contain: use-sigstore-attachments: true
   ```
3. If the key was recently rotated, ensure the new public key has been distributed and old images have been re-signed.
4. Check that the image was actually signed during the CI pipeline by reviewing the pipeline logs for the signing step.

### Podman Policy Rejects a Signed Image

**Symptom**: `podman pull harbor.internal/aibox/base:24.04` fails with policy error even though the image is signed.

**Resolution**:
1. Verify policy.json is correct:
   ```bash
   jq '.transports.docker["harbor.internal"]' /etc/containers/policy.json
   ```
   Should show `type: sigstoreSigned` with `keyPath: /etc/aibox/cosign.pub`.
2. Verify the public key exists at the path referenced in the policy:
   ```bash
   ls -la /etc/aibox/cosign.pub
   ```
3. Verify the registries.d config exists:
   ```bash
   cat /etc/containers/registries.d/harbor.yaml
   ```
4. Re-install the policy files:
   ```bash
   sudo ./infra/cosign/install-policy.sh
   ```

### Buildah Permission Issues

**Symptom**: `buildah bud` fails with permission errors in CI.

**Resolution**:
1. Ensure the CI runner supports rootless Buildah. Check:
   ```bash
   buildah --version
   buildah info
   ```
2. For rootless builds, the user needs entries in `/etc/subuid` and `/etc/subgid`:
   ```bash
   grep $(whoami) /etc/subuid
   grep $(whoami) /etc/subgid
   ```
3. If running in a container-based CI runner, ensure `--privileged` or appropriate seccomp/apparmor profiles are set.
4. Alternative: use `podman build` instead of `buildah bud` if Buildah rootless is not available.

### CI Pipeline Fails at Signing Step

**Symptom**: Pipeline fails with `no signing key provided` or `cosign: command not found`.

**Resolution**:
1. Verify CI secrets are configured: `COSIGN_KEY`, `COSIGN_PASSWORD`, `HARBOR_URL`, `HARBOR_USER`, `HARBOR_PASSWORD`.
2. Verify Cosign is installed on the CI runner:
   ```bash
   cosign version
   ```
3. If using Vault for key storage, verify the CI runner has network access to Vault and the correct auth token.

### Trivy Scan Blocks a Build

**Symptom**: Pipeline fails at scan step with critical CVE findings.

**Resolution**:
1. Review the Trivy report output to identify the CVEs.
2. Check if patches are available: look at the "Fixed Version" column.
3. If patches are available, rebuild with `--no-cache` to pull fresh base layers:
   ```bash
   ./scripts/build.sh base 24.04 harbor.internal/aibox --no-cache
   ```
4. If no patch is available and the CVE is a false positive or not applicable, document the justification and consider adding it to Trivy's ignore list.
