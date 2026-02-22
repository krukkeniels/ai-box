# Runbook: Image Lifecycle Management

**Owner**: Platform Engineering Team
**Frequency**: Weekly (automated) + on-demand for CVE response
**Last Updated**: 2026-02-21

---

## 1. Weekly Image Rebuild

### Trigger
Automated CI pipeline runs every Sunday at 02:00 UTC.

### Process
1. CI pipeline pulls latest Ubuntu 24.04 LTS base.
2. Applies all pending OS security patches (`apt-get upgrade`).
3. Rebuilds all image variants:
   - `aibox-base:24.04-YYYYMMDD`
   - `aibox-java:21-24.04-YYYYMMDD`
   - `aibox-node:20-24.04-YYYYMMDD`
   - `aibox-dotnet:8-24.04-YYYYMMDD`
   - `aibox-full:24.04-YYYYMMDD`
4. Trivy scans each image; build fails if Critical CVEs are found.
5. Cosign signs each image with the platform signing key.
6. Images pushed to Harbor with date tag.
7. `latest` tag updated to point to new build.

### Verification
```bash
# Confirm new images in Harbor
curl -s https://harbor.internal/api/v2.0/projects/aibox/repositories | jq '.[].name'

# Verify signatures
cosign verify --key cosign.pub harbor.internal/aibox/base:latest

# Check Trivy scan results
curl -s https://harbor.internal/api/v2.0/projects/aibox/repositories/base/artifacts/latest/additions/vulnerabilities
```

### Failure Handling
- If CI build fails: platform engineer on-call investigates within 4 hours.
- If Trivy finds Critical CVE in base image: escalate to CVE triage (see runbook-cve-triage.md).
- If Cosign signing fails: check signing key expiry, re-generate if needed.

---

## 2. CVE Response

### Severity-Based Response

| Severity | CVSS Score | Response Time | Action |
|----------|-----------|---------------|--------|
| Critical | 9.0-10.0 | Immediate (< 4 hours) | Emergency rebuild, mandatory update enforced |
| High | 7.0-8.9 | Same business day | Emergency rebuild, mandatory update enforced |
| Medium | 4.0-6.9 | Next weekly rebuild | Batched into weekly cycle |
| Low | 0.1-3.9 | Next weekly rebuild | Batched into weekly cycle |

### Emergency Rebuild Process (Critical/High)
1. Platform engineer triggers emergency CI build.
2. Build applies targeted patch or package update.
3. Trivy re-scan confirms CVE is resolved.
4. Cosign signs the emergency image.
5. Push to Harbor with emergency tag: `24.04-YYYYMMDD-cve-XXXX`.
6. Update `latest` tag.
7. Set mandatory update flag in image metadata.

### Mandatory Update Enforcement (CVSS 9+)
When a CVSS 9+ CVE is patched:
1. Image metadata includes `mandatory-update: true` and `min-image-date: YYYY-MM-DD`.
2. `aibox start` checks image date against `min-image-date`.
3. If local image is older than `min-image-date`, `aibox start` refuses to launch and prompts:
   ```
   SECURITY UPDATE REQUIRED: A critical vulnerability (CVE-XXXX-XXXXX, CVSS 9.X) has been
   patched. Run 'aibox update' to pull the latest image before starting your sandbox.
   ```
4. Developer runs `aibox update` to pull the patched image.
5. `aibox start` succeeds with the updated image.

---

## 3. `aibox update` Flow

### Developer Experience
```bash
aibox update
# Checking for updates...
# Current image: aibox-base:24.04-20260215
# Latest image:  aibox-base:24.04-20260221
# Pulling aibox-base:24.04-20260221... done (45s)
# Verifying signature... valid
# Update complete. Changes take effect on next 'aibox start'.
```

### Internal Process
1. Query Harbor for latest tag matching current image variant.
2. Compare local image digest with remote digest.
3. If different: pull new image via Podman.
4. Verify Cosign signature against trusted key.
5. Running containers are NOT disrupted -- update applies on next `aibox start`.

---

## 4. Harbor Garbage Collection

### Schedule
Weekly cron job runs every Monday at 04:00 UTC.

### Process
1. Harbor GC removes untagged blobs and old image layers.
2. Retention policy keeps:
   - Last 8 weekly builds per variant (rolling 2-month window).
   - All images tagged with `emergency` or `cve-*` for 90 days.
   - The `latest` tag is never garbage-collected.

### Configuration
```yaml
# Harbor GC schedule (configured in Harbor admin)
schedule: "0 4 * * 1"  # Monday 04:00 UTC
retention:
  rules:
    - tag_pattern: "latest"
      action: retain
    - tag_pattern: "*-cve-*"
      retain_days: 90
    - tag_pattern: "*"
      retain_count: 8
      scope: per_repository
```

### Monitoring
- Alert if GC fails to run for 2 consecutive weeks.
- Alert if Harbor storage exceeds 80% capacity.
- Dashboard panel: "Harbor Storage Usage" in Platform Operations dashboard.

---

## 5. Image Signing

### Key Management
- Signing key stored in HashiCorp Vault at `secret/aibox/cosign-key`.
- Key rotation: annually, or immediately if compromise is suspected.
- Verification key (`cosign.pub`) distributed to all developer machines via `aibox setup`.

### Verification in `aibox start`
Every `aibox start` verifies the image signature before launching:
```
Verifying image signature... valid (signed by platform-ci@aibox.internal, 2026-02-21)
```

If signature verification fails:
```
ERROR: Image signature verification failed for aibox-base:24.04-20260221.
This image may have been tampered with. Contact the platform team.
Run 'aibox update' to pull a fresh image from Harbor.
```

---

## 6. Rollback Procedure

If a newly published image causes issues:
1. Identify the last known good image tag (e.g., `24.04-20260214`).
2. Update the `latest` tag in Harbor to point to the known good image.
3. Notify developers via Slack `#aibox-announce`:
   ```
   Image rollback: aibox images have been rolled back to 24.04-20260214 due to
   [brief description]. Run 'aibox update' to get the stable image.
   ```
4. Investigate root cause in the failed build.
5. Fix and re-publish a corrected image.

---

## 7. Metrics and Monitoring

| Metric | Alert Threshold | Dashboard |
|--------|----------------|-----------|
| Image age (days since build) | > 30 days | Platform Operations |
| Weekly rebuild success rate | < 100% for 2 weeks | Platform Operations |
| Harbor storage usage | > 80% capacity | Platform Operations |
| Unsigned image pull attempts | > 0 | Security Posture |
| Mandatory update pending (developers) | > 10% of fleet after 24h | Platform Operations |
