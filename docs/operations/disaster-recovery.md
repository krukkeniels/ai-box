# Disaster Recovery Procedures

**Owner**: Platform Engineering Team
**Last Updated**: 2026-02-21
**Review Cadence**: Quarterly (aligned with tabletop exercises)

---

## Table of Contents

1. [RTO/RPO Targets](#1-rtorpo-targets)
2. [HA Requirements and Backup Specifications](#2-ha-requirements-and-backup-specifications)
3. [Scenario 1: Developer Machine Failure](#3-scenario-1-developer-machine-failure)
4. [Scenario 2: Harbor Registry Failure](#4-scenario-2-harbor-registry-failure)
5. [Scenario 3: Nexus Repository Failure](#5-scenario-3-nexus-repository-failure)
6. [Scenario 4: Vault Failure](#6-scenario-4-vault-failure)
7. [Tabletop Exercise Templates](#7-tabletop-exercise-templates)
8. [Recovery Verification Checklists](#8-recovery-verification-checklists)

---

## 1. RTO/RPO Targets

| Service | RTO (Recovery Time Objective) | RPO (Recovery Point Objective) | Justification |
|---------|------|------|---------------|
| Developer Machine | 1-2 hours | 0 (code in Git, images in Harbor) | Developer productivity loss |
| Harbor Registry | 4 hours | 24 hours | Devs work with cached images; only new pulls fail |
| Nexus Repository | 2 hours | 24 hours | Builds fail for uncached deps; high dev impact |
| Vault | 1 hour | 0 (HA replication) | Credential issuance blocked; cached creds expire |

---

## 2. HA Requirements and Backup Specifications

### 2.1 Harbor Registry

| Aspect | Specification |
|--------|---------------|
| Deployment | HA pair with shared PostgreSQL and Redis |
| Storage | S3-compatible object storage (MinIO cluster) |
| Backup frequency | Daily full backup of PostgreSQL DB and config |
| Backup retention | 30 days |
| Replication | Async replication to secondary site for DR |
| Failover | Manual failover to secondary (automated monitoring detects failure) |
| Storage capacity | 500 GB initial, monitored via alert at 80% |

### 2.2 Nexus Repository

| Aspect | Specification |
|--------|---------------|
| Deployment | Single node with hot standby |
| Storage | Local SSD + nightly rsync to DR node |
| Backup frequency | Nightly full backup of blob store and OrientDB |
| Backup retention | 14 days |
| Cache behavior | Proxy repositories cache all downloaded artifacts locally |
| Failover | Manual DNS switch to standby node |
| Storage capacity | 1 TB initial (cached artifacts grow over time) |

### 2.3 Vault

| Aspect | Specification |
|--------|---------------|
| Deployment | HA cluster (3 nodes, Raft storage backend) |
| Backup frequency | Hourly Raft snapshots |
| Backup retention | 7 days |
| Auto-unseal | AWS KMS or Transit-based auto-unseal |
| Failover | Automatic leader election via Raft consensus |
| Seal recovery | Auto-unseal; manual unseal as fallback |

---

## 3. Scenario 1: Developer Machine Failure

### Symptoms
- Machine unresponsive, hardware failure, OS corruption, or theft/loss.
- Developer cannot access their local development environment.

### Impact Assessment
- **Scope**: Single developer.
- **Data at risk**: Local build caches (named Podman volumes), uncommitted code changes, local IDE configuration.
- **No data loss for**: Source code (in Git), container images (in Harbor), credentials (in Vault), policies (in Git).

### Immediate Actions
1. Developer reports issue to IT and team lead.
2. IT provides replacement machine or re-images existing machine.
3. Developer follows recovery procedure below.

### Recovery Steps

```bash
# Step 1: Install AI-Box on new/re-imaged machine (5-10 minutes)
winget install aibox
# Or download from internal portal

# Step 2: Run setup (10-15 minutes)
aibox setup
# Installs Podman, configures WSL2, proxy, DNS, gVisor
# Downloads verification key (cosign.pub)

# Step 3: Verify setup
aibox doctor
# All checks should pass

# Step 4: Pull container images (5-15 minutes depending on bandwidth)
aibox update
# Pulls latest signed image from Harbor

# Step 5: Clone project repositories (5-30 minutes)
git clone <repo-url> ~/projects/<project>
# Repeat for each project

# Step 6: Start sandbox and verify (1-2 minutes)
aibox start --workspace ~/projects/<project> --toolpacks java@21,node@22
# IDE connects automatically

# Step 7: Restore IDE settings
# VS Code: Settings sync via Microsoft account
# IntelliJ: Settings sync via JetBrains account

# Step 8: Re-warm build caches (first build will be slower)
gradle build   # or mvn compile, npm install, dotnet restore
# Subsequent builds use restored caches
```

### Cache Re-Warming
Build caches (`.gradle/`, `node_modules/`, `~/.m2/`, `.nuget/`) are stored in named Podman volumes which are local to the machine. After machine failure, these are lost. First builds will take longer:

| Stack | First Build (no cache) | Subsequent Builds |
|-------|----------------------|-------------------|
| Java (Gradle) | 3-8 minutes | 30-90 seconds |
| Java (Maven) | 5-10 minutes | 1-2 minutes |
| Node.js | 2-5 minutes | 10-30 seconds |
| .NET | 3-7 minutes | 30-60 seconds |
| Bazel | 5-15 minutes | 1-3 minutes |

### Verification
- [ ] `aibox doctor` passes all checks.
- [ ] `aibox start` launches sandbox successfully.
- [ ] IDE connects and indexes project.
- [ ] `git push` works (requires credential from Vault).
- [ ] Build completes successfully.
- [ ] AI tools (Claude Code / Codex) functional.

### Post-Incident Review
- Confirm developer is fully operational.
- Document any issues encountered during recovery.
- Update this runbook if recovery took longer than RTO.

---

## 4. Scenario 2: Harbor Registry Failure

### Symptoms
- `aibox update` fails with connection error to `harbor.internal`.
- New `aibox start` on machines without cached images fails.
- CI pipeline cannot push newly built images.
- Harbor health check alert fires (see alerting-rules.md).

### Impact Assessment
- **Scope**: All developers (potential), but mitigated by local caching.
- **Developers with cached images**: Continue working normally. Cannot pull updates.
- **Developers without cached images** (new setup, new machine): Cannot start sandboxes.
- **CI pipeline**: Cannot publish new images. Weekly rebuild blocked.

### Immediate Actions
1. Confirm Harbor is down (not a network/DNS issue):
   ```bash
   curl -v https://harbor.internal/api/v2.0/health
   ping harbor.internal
   nslookup harbor.internal
   ```
2. Notify `#aibox-help`:
   ```
   INCIDENT: Harbor registry is currently unreachable. Developers with cached images
   can continue working normally. New setups are blocked. Team is investigating.
   ```
3. Check Harbor service logs and infrastructure.

### Recovery Steps

**If Harbor service is down but data is intact:**
```bash
# On Harbor server
systemctl status harbor
docker-compose -f /opt/harbor/docker-compose.yml logs --tail=100

# Common fixes:
# - Restart Harbor services
docker-compose -f /opt/harbor/docker-compose.yml down
docker-compose -f /opt/harbor/docker-compose.yml up -d

# - Check PostgreSQL connectivity
docker exec harbor-db pg_isready

# - Check Redis connectivity
docker exec harbor-redis redis-cli ping

# - Check storage backend (MinIO)
mc admin info minio-cluster
```

**If Harbor data is corrupted or lost:**
```bash
# Restore from latest backup
# 1. Stop Harbor
docker-compose -f /opt/harbor/docker-compose.yml down

# 2. Restore PostgreSQL from backup
pg_restore -d registry /backups/harbor/latest/harbor-db.dump

# 3. Restore configuration
cp /backups/harbor/latest/harbor.yml /opt/harbor/harbor.yml

# 4. Restart Harbor
docker-compose -f /opt/harbor/docker-compose.yml up -d

# 5. Verify
curl https://harbor.internal/api/v2.0/health
```

**If primary Harbor is unrecoverable (failover to secondary):**
```bash
# Update DNS to point harbor.internal to secondary site
# Verify replication status -- secondary may be up to 24h behind
curl https://harbor-dr.internal/api/v2.0/health

# Update DNS record
# (infrastructure team action)
```

### Verifying Local Image Integrity Without Registry
Developers can verify their cached images are untampered:
```bash
# Check local image digest
podman inspect harbor.internal/aibox/base:latest --format '{{.Digest}}'

# Compare with last known good digest (published in #aibox-announce)
# If digests match, image is safe to use
```

### Verification
- [ ] `curl https://harbor.internal/api/v2.0/health` returns `healthy`.
- [ ] `aibox update` succeeds.
- [ ] New `aibox start` can pull images.
- [ ] CI pipeline can push images.
- [ ] Image signatures verify correctly.

### Post-Incident Review
- Root cause analysis.
- Duration of outage.
- Number of developers impacted.
- Update HA/backup strategy if needed.
- Update this runbook with lessons learned.

---

## 5. Scenario 3: Nexus Repository Failure

### Symptoms
- `npm install`, `gradle build`, `mvn compile`, `dotnet restore` fail with dependency resolution errors.
- Proxy error logs in Squid show failed connections to `nexus.internal`.
- Nexus health check alert fires.

### Impact Assessment
- **Scope**: All developers performing builds that require dependency downloads.
- **Developers with cached dependencies**: Builds succeed using local build caches in named volumes.
- **Developers pulling new dependencies**: Builds fail.
- **Severity**: High -- directly impacts developer productivity.

### Immediate Actions
1. Confirm Nexus is down:
   ```bash
   curl -v https://nexus.internal/service/rest/v1/status
   ```
2. Notify `#aibox-help`:
   ```
   INCIDENT: Nexus repository is unreachable. Builds using cached dependencies will
   continue to work. New dependency downloads will fail. Team is investigating.
   ```
3. Advise developers:
   ```
   WORKAROUND: If your build fails with dependency errors, try:
   1. Check if the dep is already cached: builds that succeeded recently should work.
   2. Avoid 'clean build' commands that discard caches.
   3. If you must add a new dependency, wait for Nexus restoration.
   ```

### Recovery Steps

**If Nexus service is down:**
```bash
# On Nexus server
systemctl status nexus
journalctl -u nexus --tail=100

# Common fixes:
# - JVM out of memory: increase heap
# - Disk full: clear /tmp, expand storage
# - Corrupted index: rebuild index
#   nexus-cli rebuild-index --repository=npm-proxy

# Restart
systemctl restart nexus
```

**If Nexus data is lost (restore from backup):**
```bash
# Stop Nexus
systemctl stop nexus

# Restore blob store from backup
rsync -av /backups/nexus/latest/blobs/ /opt/nexus/sonatype-work/blobs/

# Restore OrientDB
cp -r /backups/nexus/latest/db/ /opt/nexus/sonatype-work/db/

# Start Nexus
systemctl start nexus

# Note: Proxy caches will re-populate automatically from upstream on first request
```

**If Nexus is unrecoverable (failover to standby):**
```bash
# Switch DNS to standby node
# Standby has nightly rsync of blob store
# Some proxy cache entries may be stale -- they'll refresh from upstream
```

### Build Cache Mitigation
Build caches inside sandboxes persist across sessions via named Podman volumes:
- Gradle: `~/.gradle/caches/` (dependencies cached locally)
- Maven: `~/.m2/repository/` (full mirror of downloaded deps)
- npm: `~/.npm/` and `node_modules/` (if preserved)
- NuGet: `~/.nuget/packages/`
- Bazel: `~/.cache/bazel/` (build outputs and deps)

Developers who have built recently will have most dependencies cached.

### Verification
- [ ] `curl https://nexus.internal/service/rest/v1/status` returns OK.
- [ ] `npm install` resolves dependencies through Nexus.
- [ ] `gradle build` downloads from Nexus proxy.
- [ ] `dotnet restore` connects to NuGet proxy.
- [ ] Proxy repositories are populating cache.

### Post-Incident Review
- Duration of outage and developer impact.
- Number of builds that failed.
- Evaluate increasing cache retention or adding Nexus HA.

---

## 6. Scenario 4: Vault Failure

### Symptoms
- `aibox start` fails at credential injection step.
- AI tools report authentication failures.
- `git push` fails with auth errors.
- Vault health check alert fires.

### Impact Assessment
- **Scope**: All developers starting new sandboxes or whose cached credentials have expired.
- **Developers with valid cached credentials**: Continue working until TTLs expire.
- **Credential TTLs**:
  | Credential | TTL | Grace Period |
  |-----------|-----|-------------|
  | Git token | 4 hours | 30 minutes |
  | LLM API key | 8 hours | 1 hour |
  | Package mirror token | 8 hours | 1 hour |
- **Running sandboxes**: Continue working with cached credentials until TTL expiry.

### Immediate Actions
1. Confirm Vault is down:
   ```bash
   vault status
   curl https://vault.internal:8200/v1/sys/health
   ```
2. Check if Vault is sealed (common after restart):
   ```bash
   vault status | grep Sealed
   # If sealed and auto-unseal is configured, it should unseal automatically
   # If auto-unseal failed, manual unseal required
   ```
3. Notify `#aibox-help`:
   ```
   INCIDENT: Vault is currently unreachable. Running sandboxes with valid credentials
   continue to work. New sandbox starts will fail. Credentials expire on a rolling basis:
   Git tokens: ~4h, LLM/package tokens: ~8h. Team is investigating.
   ```

### Recovery Steps

**If Vault is sealed:**
```bash
# Check auto-unseal status
vault status

# If auto-unseal failed, manual unseal (requires 3 of 5 key shares)
vault operator unseal <key-share-1>
vault operator unseal <key-share-2>
vault operator unseal <key-share-3>

# Verify
vault status
```

**If Vault leader node is down (HA failover):**
```bash
# Raft consensus should automatically elect new leader
# Check cluster status
vault operator raft list-peers

# If automatic failover did not occur:
# 1. Identify healthy nodes
vault operator raft list-peers

# 2. Remove failed node from cluster
vault operator raft remove-peer <failed-node-id>

# 3. New leader election happens automatically
```

**If Vault data is corrupted (restore from Raft snapshot):**
```bash
# On a healthy Vault node
vault operator raft snapshot restore /backups/vault/latest/raft.snap

# Verify
vault status
vault secrets list
```

### Graceful Degradation
The `aibox` CLI implements graceful degradation for Vault outages:
1. If Vault is unreachable during `aibox start`:
   - Check if cached credentials exist and are still within TTL.
   - If cached creds are valid: start sandbox with cached credentials, warn user.
   - If no valid cached creds: fail with clear error message directing to `#aibox-help`.
2. Credential renewal attempts continue in the background.
3. When Vault recovers, credentials are refreshed silently.

### Extended Outage Fallback
If Vault is down for > 8 hours (all credential TTLs expired):
1. Activate simplified credential broker:
   - Pre-generated, time-limited tokens stored in a sealed emergency credential store.
   - Platform engineer manually distributes emergency tokens.
   - Tokens are single-use and expire in 24 hours.
2. This is a last resort -- prioritize Vault restoration.

### Verification
- [ ] `vault status` shows unsealed and healthy.
- [ ] `vault token lookup` succeeds.
- [ ] `aibox start` injects credentials successfully.
- [ ] `git push` authenticates correctly.
- [ ] AI tools authenticate to LLM API.
- [ ] Package mirror tokens work for dependency downloads.

### Post-Incident Review
- Root cause (seal issue, node failure, network, storage).
- Duration of outage.
- Number of developers who lost credential access.
- Evaluate Vault HA topology and auto-unseal configuration.

---

## 7. Tabletop Exercise Templates

### 7.1 Schedule
Quarterly tabletop exercises, rotating through scenarios:

| Quarter | Scenario | Participants |
|---------|----------|-------------|
| Q1 | Harbor Registry Failure | Platform team + 1 champion |
| Q2 | Vault Failure | Platform team + security team |
| Q3 | Developer Machine Failure | Platform team + 2 developers |
| Q4 | Combined: Nexus + Vault | Platform team + security team |

### 7.2 Exercise Format

**Duration**: 90 minutes.

| Phase | Duration | Activity |
|-------|----------|----------|
| Setup | 10 min | Facilitator presents scenario, hands out role cards |
| Detection | 15 min | Team discusses: how would we detect this? What alerts fire? |
| Response | 30 min | Walk through runbook step-by-step. Identify gaps. |
| Recovery | 20 min | Verify recovery steps. Test verification checklist. |
| Debrief | 15 min | Document findings, action items, runbook updates |

### 7.3 Scenario Cards

**Harbor Failure Scenario Card:**
```
SCENARIO: Harbor Registry Failure
TIME: Tuesday 10:30 AM

The HarborRegistryDown alert has fired. curl to harbor.internal times out.
You check the Harbor server and find the PostgreSQL container has crashed
due to disk full. The last successful backup was 18 hours ago.

Questions to work through:
1. What is the immediate developer impact?
2. What do you communicate to developers?
3. How do you free disk space and restart PostgreSQL?
4. If the database is corrupted, how do you restore from backup?
5. What data might be lost from the 18-hour gap?
6. How do you verify full recovery?
```

**Vault Failure Scenario Card:**
```
SCENARIO: Vault Failure
TIME: Friday 4:00 PM

The VaultConnectivityDown alert fires. Vault cluster status shows all 3 nodes
are sealed. Auto-unseal is failing because the KMS endpoint is unreachable.
It's Friday afternoon and the AWS team is not available until Monday.

Questions to work through:
1. How long until developer credentials start expiring?
2. Can you manually unseal without KMS?
3. Should you activate the emergency credential broker?
4. What's the communication plan for the weekend?
5. How do you prevent this in the future?
```

### 7.4 Exercise Output Template

```markdown
# DR Tabletop Exercise Report - [Date]

## Scenario: [scenario name]
## Participants: [names and roles]

## Findings
1. [Finding: description, severity]
2. ...

## Runbook Gaps Identified
1. [Gap: what was missing from the runbook]
2. ...

## Action Items
| # | Action | Owner | Due Date |
|---|--------|-------|----------|
| 1 | Update runbook section X | [name] | [date] |
| 2 | Add alert for Y | [name] | [date] |

## Runbook Updates Made
- [List of changes applied to this document]
```

---

## 8. Recovery Verification Checklists

### 8.1 Harbor Recovery Verification
- [ ] Harbor API health endpoint returns `healthy`.
- [ ] All 5 image repositories are accessible.
- [ ] `latest` tags exist for all variants.
- [ ] Image signatures verify with `cosign verify`.
- [ ] Trivy scanning is operational.
- [ ] `aibox update` succeeds from a developer machine.
- [ ] New `aibox start` (no cached images) pulls and starts successfully.
- [ ] CI pipeline can push a test image.
- [ ] Harbor GC cron is scheduled.
- [ ] Replication to DR site is active.

### 8.2 Nexus Recovery Verification
- [ ] Nexus status API returns OK.
- [ ] Maven proxy repository responds.
- [ ] npm proxy repository responds.
- [ ] NuGet proxy repository responds.
- [ ] PyPI proxy repository responds.
- [ ] `gradle build` resolves dependencies through Nexus.
- [ ] `npm install` resolves packages through Nexus.
- [ ] `dotnet restore` resolves NuGet packages.
- [ ] Proxy cache is populating.
- [ ] Blob store integrity check passes.

### 8.3 Vault Recovery Verification
- [ ] `vault status` shows initialized, unsealed, active.
- [ ] Raft cluster has quorum (2 of 3 nodes healthy).
- [ ] Auto-unseal is functional.
- [ ] Secret engines are accessible (`vault secrets list`).
- [ ] New token issuance works for Git, LLM, and package mirror.
- [ ] `aibox start` injects credentials successfully.
- [ ] `git push` authenticates with new tokens.
- [ ] AI tool API calls authenticate.
- [ ] Audit logging is active.
- [ ] Token TTLs are correct (Git 4h, LLM 8h, packages 8h).

### 8.4 Developer Machine Recovery Verification
- [ ] `aibox doctor` passes all checks.
- [ ] `aibox start` launches sandbox within 90s.
- [ ] IDE connects (VS Code Remote SSH or JetBrains Gateway).
- [ ] `git clone` and `git push` work.
- [ ] Build completes (with cache miss, first time).
- [ ] AI tools launch and authenticate.
- [ ] Network connectivity (`aibox network test` passes).
- [ ] Dotfiles sync works (if configured).
