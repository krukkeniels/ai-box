# Alerting Rules for AI-Box Operations

**Owner**: Platform Engineering Team
**Last Updated**: 2026-02-21

---

## 1. Overview

This document defines operational alerts for AI-Box infrastructure health. These supplement the security-focused alerts defined in Phase 5 (see `cmd/aibox/internal/dashboards/dashboards.go`).

All alerts use Prometheus alerting rules evaluated by Grafana or Alertmanager.

---

## 2. Infrastructure Health Alerts

### 2.1 Harbor Registry Downtime

```yaml
alert: HarborRegistryDown
expr: probe_success{job="harbor"} == 0
for: 5m
labels:
  severity: critical
  team: platform
annotations:
  summary: "Harbor registry is unreachable"
  description: "Harbor at harbor.internal has been unreachable for 5 minutes. Developers cannot pull new images or updates."
  runbook: "docs/phase6/operations/disaster-recovery.md#harbor-registry-failure"
  impact: "Image pulls fail. Existing cached images continue to work."
  action: "Check Harbor service health, DNS resolution, and network connectivity."
```

**Channel**: Existing infrastructure team on-call
**SLA**: 15 minutes response (business hours); best-effort after hours

### 2.2 Nexus Repository Downtime

```yaml
alert: NexusRepositoryDown
expr: probe_success{job="nexus"} == 0
for: 5m
labels:
  severity: critical
  team: platform
annotations:
  summary: "Nexus repository is unreachable"
  description: "Nexus at nexus.internal has been unreachable for 5 minutes. Builds will fail if dependencies are not cached locally."
  runbook: "docs/phase6/operations/disaster-recovery.md#nexus-repository-failure"
  impact: "Dependency resolution fails for uncached packages."
  action: "Check Nexus service, disk space, and JVM health."
```

**Channel**: Existing infrastructure team on-call
**SLA**: 15 minutes response (business hours); best-effort after hours

### 2.3 Vault Connectivity Failure

```yaml
alert: VaultConnectivityDown
expr: probe_success{job="vault"} == 0
for: 5m
labels:
  severity: critical
  team: platform
annotations:
  summary: "Vault is unreachable"
  description: "Vault at vault.internal has been unreachable for 5 minutes. New credential issuance will fail."
  runbook: "docs/phase6/operations/disaster-recovery.md#vault-failure"
  impact: "New credentials cannot be issued. Cached credentials valid until TTL expires (Git: 4h, LLM: 8h, packages: 8h)."
  action: "Check Vault seal status, HA failover, and network connectivity."
```

**Channel**: Existing infrastructure team on-call
**SLA**: 15 minutes response (business hours); best-effort after hours

---

## 3. Image Lifecycle Alerts

### 3.1 Image Signing Failure

```yaml
alert: ImageSigningFailure
expr: increase(aibox_image_signing_failures_total[1h]) > 0
for: 0m
labels:
  severity: high
  team: platform
annotations:
  summary: "Image signing failed during CI build"
  description: "Cosign image signing failed during the CI build pipeline. Unsigned images will be rejected by developer machines."
  runbook: "docs/phase6/operations/runbook-image-lifecycle.md#image-signing"
  action: "Check Cosign key validity, Vault connectivity (if key is in Vault), and CI pipeline logs."
```

**Channel**: Slack `#aibox-platform`
**SLA**: 4 hours

### 3.2 Image Age Exceeds 30 Days

```yaml
alert: ImageAgeExceeded
expr: (time() - aibox_image_build_timestamp) / 86400 > 30
for: 1h
labels:
  severity: high
  team: platform
annotations:
  summary: "AI-Box image is older than 30 days"
  description: "The latest AI-Box image in Harbor is {{ $value | humanizeDuration }} old. Weekly rebuilds may be failing."
  runbook: "docs/phase6/operations/runbook-image-lifecycle.md#weekly-image-rebuild"
  action: "Check weekly CI rebuild pipeline. Investigate build failures."
```

**Channel**: Slack `#aibox-platform`
**SLA**: Same business day

### 3.3 Mandatory Update Not Applied

```yaml
alert: MandatoryUpdatePending
expr: (aibox_fleet_outdated_mandatory / aibox_fleet_total) * 100 > 10
for: 24h
labels:
  severity: high
  team: platform
annotations:
  summary: "More than 10% of fleet has not applied mandatory update"
  description: "{{ $value }}% of developers have not applied the mandatory security update after 24 hours."
  action: "Send direct notifications to affected developers. Escalate to team leads."
```

**Channel**: Slack `#aibox-platform`
**SLA**: Same business day

---

## 4. Policy Violation Alerts

### 4.1 Policy Violation Spike

```yaml
alert: PolicyViolationSpike
expr: sum(increase(aibox_policy_violations_total[1h])) > 50
for: 0m
labels:
  severity: high
  team: platform
annotations:
  summary: "Abnormal spike in policy violations"
  description: "More than 50 policy violations recorded in the last hour across the fleet. This may indicate a misconfigured policy or attempted policy bypass."
  action: "Review policy violation logs. Check if a recent policy change is causing false positives."
```

**Channel**: Slack `#aibox-platform`
**SLA**: 4 hours

### 4.2 Single User Repeated Violations

```yaml
alert: UserRepeatedPolicyViolations
expr: sum by (user) (increase(aibox_policy_violations_total[1h])) > 20
for: 0m
labels:
  severity: medium
  team: platform
annotations:
  summary: "User {{ $labels.user }} has excessive policy violations"
  description: "User {{ $labels.user }} triggered {{ $value }} policy violations in 1 hour. May indicate confusion or intentional bypass attempt."
  action: "Review user's recent activity. Reach out to offer help if it appears to be a configuration issue."
```

**Channel**: Email to platform team
**SLA**: Next business day

---

## 5. Performance Alerts

### 5.1 Startup Time SLA Breach

```yaml
alert: StartupTimeSLABreach
expr: histogram_quantile(0.95, rate(aibox_sandbox_startup_seconds_bucket{type="cold"}[1h])) > 90
for: 30m
labels:
  severity: high
  team: platform
annotations:
  summary: "Cold start p95 exceeds 90-second SLA"
  description: "Cold start p95 is {{ $value }}s, exceeding the 90-second SLA target."
  action: "Check Harbor pull times, image sizes, gVisor startup, and host resource availability."
```

**Channel**: Slack `#aibox-platform`
**SLA**: 4 hours

### 5.2 Warm Start Degradation

```yaml
alert: WarmStartDegraded
expr: histogram_quantile(0.95, rate(aibox_sandbox_startup_seconds_bucket{type="warm"}[1h])) > 15
for: 30m
labels:
  severity: medium
  team: platform
annotations:
  summary: "Warm start p95 exceeds 15-second target"
  description: "Warm start p95 is {{ $value }}s. Expected < 15 seconds."
  action: "Check Podman container resume performance, disk I/O, and WSL2 memory pressure."
```

**Channel**: Email to platform team
**SLA**: Next business day

---

## 6. Alert Routing Summary

| Severity | Channel | Response SLA | Escalation |
|----------|---------|-------------|------------|
| Critical | Existing infra team on-call | 15 minutes | Infra team on-call handles Harbor/Nexus/Vault |
| High | Slack `#aibox-help` | 4 hours (business hours) | Platform team lead |
| Medium | Email to platform team | Next business day | Weekly review meeting |
| Info | Dashboard only | Weekly review | None |

---

## 7. Monitoring Coverage

- **Business hours**: Platform team monitors Slack `#aibox-help` and dashboards during business hours.
- **After hours**: Best-effort response. Critical infrastructure alerts (Harbor, Nexus, Vault) route to the existing infrastructure team on-call -- no new AI-Box-specific on-call rotation.
- Platform engineers have access to Harbor admin, Nexus admin, Vault admin, and CI pipeline during business hours.
- Escalation path: platform team (business hours) -> infra team on-call (critical infra) -> security team lead (security incidents).

---

## 8. Alert Silencing

To prevent alert fatigue during planned maintenance:

```bash
# Silence alerts during Harbor maintenance window
amtool silence add --alertname HarborRegistryDown \
  --start "2026-03-01T02:00:00Z" --end "2026-03-01T04:00:00Z" \
  --comment "Planned Harbor maintenance"
```

All silences must be documented in `#aibox-platform` with:
- Reason for silencing
- Expected duration
- Who authorized it
