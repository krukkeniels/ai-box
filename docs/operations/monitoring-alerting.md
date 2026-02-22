# Monitoring Dashboards and Alerting

**Owner**: Platform Engineering Team
**Last Updated**: 2026-02-21
**Infrastructure**: Prometheus + Grafana (provisioned as code)

---

## 1. Dashboard Overview

AI-Box uses five Grafana dashboards. Three were created in Phase 5 (Platform Operations, Security Posture, Executive Summary -- see `cmd/aibox/internal/dashboards/dashboards.go`). Phase 6 adds two operational dashboards:

| Dashboard | UID | Audience | Refresh |
|-----------|-----|----------|---------|
| Platform Operations | `aibox-platform-ops` | Platform team | 30s |
| Security Posture | `aibox-security-posture` | Security team | 30s |
| Executive Summary | `aibox-executive` | Leadership | 1h |
| **Fleet Health & Adoption** | `aibox-fleet-health` | Platform team + leads | 5m |
| **Support & KPI** | `aibox-support-kpi` | Platform team + leads | 15m |

---

## 2. Fleet Health & Adoption Dashboard

### Row 1: Fleet Overview (y=0, h=4)

| Panel | Type | W | X | Query |
|-------|------|---|---|-------|
| Active Sandboxes | stat | 4 | 0 | `count(aibox_sandbox_status{status="running"})` |
| Adoption Rate (7d) | gauge | 4 | 4 | `aibox_active_users_7d / aibox_total_developers * 100` |
| Teams Migrated | gauge | 4 | 8 | `count(aibox_team_migrated==1) / count(aibox_team_migrated) * 100` |
| Rollout Phase | stat | 4 | 12 | `aibox_rollout_phase` (1=Pilot, 2=Early, 3=General, 4=Mandatory) |
| Fleet Image Freshness | gauge | 4 | 16 | `1 - (aibox_fleet_outdated / aibox_fleet_total)` |
| Fallback Rate (7d) | gauge | 4 | 20 | `increase(aibox_fallback_events_total[7d]) / increase(aibox_sandbox_starts_total[7d]) * 100` |

### Row 2: Adoption Trends (y=4, h=8)

| Panel | Type | W | X | Query |
|-------|------|---|---|-------|
| Weekly Adoption Trend | timeseries | 12 | 0 | `aibox_active_users_7d / aibox_total_developers * 100` over 12 weeks |
| Adoption by Team | bargauge | 12 | 12 | `count by (team) (aibox_sandbox_status{active_7d="true"}) / on(team) group_left aibox_team_size * 100` |

### Row 3: Image & Resource Distribution (y=12, h=8)

| Panel | Type | W | X | Query |
|-------|------|---|---|-------|
| Image Age Distribution | bargauge | 8 | 0 | `(time() - aibox_sandbox_image_timestamp) / 86400` bucketed |
| Image Versions in Fleet | table | 8 | 8 | `count by (image, version) (aibox_sandbox_status{status="running"})` |
| Resource Utilization | timeseries | 8 | 16 | `avg(aibox_sandbox_cpu_usage)`, `avg(aibox_sandbox_memory_usage)` |

### Row 4: Startup Performance (y=20, h=8)

| Panel | Type | W | X | Query |
|-------|------|---|---|-------|
| Cold Start p50/p95/p99 | timeseries | 12 | 0 | `histogram_quantile(0.50/0.95/0.99, rate(aibox_sandbox_startup_seconds_bucket{type="cold"}[1h]))` |
| Warm Start p50/p95/p99 | timeseries | 12 | 12 | `histogram_quantile(0.50/0.95/0.99, rate(aibox_sandbox_startup_seconds_bucket{type="warm"}[1h]))` |

---

## 3. Support & KPI Dashboard

### Row 1: Support Volume (y=0, h=6)

| Panel | Type | W | X | Query |
|-------|------|---|---|-------|
| Open Threads (7d) | stat | 6 | 0 | `increase(aibox_support_threads_total[7d])` |
| Resolution Time (avg) | stat | 6 | 6 | `avg(aibox_support_resolution_seconds) / 3600` (hours) |
| Support Volume Trend | timeseries | 12 | 12 | `increase(aibox_support_threads_total[7d])` over 12 weeks |

Note: Support is tracked via Slack thread counts. Formal ticketing only activates if > 20 threads/week (per PO decision).

### Row 2: Support Breakdown (y=6, h=8)

| Panel | Type | W | X | Query |
|-------|------|---|---|-------|
| By Category | piechart | 8 | 0 | `sum by (category) (increase(aibox_support_threads_total[30d]))` |
| By Tier | bargauge | 8 | 8 | `sum by (tier) (increase(aibox_support_threads_total[30d]))` |
| Repeat Issues | table | 8 | 16 | Top recurring categories in 30 days |

### Row 3: KPI Summary (y=14, h=6)

| Panel | Type | W | X | Query |
|-------|------|---|---|-------|
| Adoption Rate | gauge | 4 | 0 | See Section 2 |
| Startup p95 | stat | 4 | 4 | Cold start p95 |
| Ticket Volume | stat | 4 | 8 | 7-day thread count |
| Fallback Rate | gauge | 4 | 12 | Fallback percentage |
| Security Events (7d) | stat | 4 | 16 | `sum(increase(aibox_security_incidents_total[7d]))` |
| Mandatory Cutover Readiness | stat | 4 | 20 | "READY" if all 3 PO conditions met |

Mandatory cutover conditions (all must be true for 2+ weeks):
- Adoption > 90%
- Tickets < 3/week for 4 consecutive weeks
- Every team has validated tool pack config

### Row 4: Fallback Analysis (y=20, h=8)

| Panel | Type | W | X | Query |
|-------|------|---|---|-------|
| Fallback Trend | timeseries | 12 | 0 | `increase(aibox_fallback_events_total[7d]) / increase(aibox_sandbox_starts_total[7d]) * 100` over 12 weeks |
| Fallback Reasons | bargauge | 12 | 12 | `sum by (reason) (increase(aibox_fallback_events_total[30d]))` |

---

## 4. Alert Rules

### 4.1 Infrastructure Health

| Alert | Expression | For | Severity | Channel |
|-------|-----------|-----|----------|---------|
| Harbor Down | `probe_success{job="harbor"} == 0` | 5m | critical | Infra team on-call |
| Nexus Down | `probe_success{job="nexus"} == 0` | 5m | critical | Infra team on-call |
| Vault Down | `probe_success{job="vault"} == 0` | 5m | critical | Infra team on-call |
| Harbor Storage > 80% | `harbor_storage_usage_percent > 80` | 30m | high | `#aibox-help` |

### 4.2 Image Lifecycle

| Alert | Expression | For | Severity | Channel |
|-------|-----------|-----|----------|---------|
| Image Signing Failure | `increase(aibox_image_signing_failures_total[1h]) > 0` | 0m | high | `#aibox-help` |
| Image Age > 30 days | `(time() - aibox_image_build_timestamp) / 86400 > 30` | 1h | high | `#aibox-help` |
| Mandatory Update Pending > 10% | `aibox_fleet_outdated_mandatory / aibox_fleet_total > 0.1` | 24h | high | `#aibox-help` |

### 4.3 Policy & Security

| Alert | Expression | For | Severity | Channel |
|-------|-----------|-----|----------|---------|
| Policy Violation Spike | `sum(increase(aibox_policy_violations_total[1h])) > 50` | 0m | high | `#aibox-help` |
| User Repeated Violations | `sum by (user) (increase(aibox_policy_violations_total[1h])) > 20` | 0m | medium | email |

### 4.4 Performance

| Alert | Expression | For | Severity | Channel |
|-------|-----------|-----|----------|---------|
| Cold Start SLA Breach | `histogram_quantile(0.95, rate(aibox_sandbox_startup_seconds_bucket{type="cold"}[1h])) > 90` | 30m | high | `#aibox-help` |
| Warm Start Degraded | `histogram_quantile(0.95, rate(aibox_sandbox_startup_seconds_bucket{type="warm"}[1h])) > 15` | 30m | medium | email |

### 4.5 KPI Alerts

| Alert | Expression | For | Severity | Channel |
|-------|-----------|-----|----------|---------|
| Adoption Stall | Adoption rate decreases 2 consecutive weeks | 14d | medium | email |
| Ticket Volume Spike | > 2x previous week average | 0m | medium | email |
| Fallback Rate Spike | Fallback rate > 10% for 1 week | 7d | high | `#aibox-help` |

### 4.6 Alert Routing Summary

| Severity | Channel | Response SLA | Escalation |
|----------|---------|-------------|------------|
| Critical | Existing infra team on-call | 15 minutes | Infra team handles Harbor/Nexus/Vault |
| High | Slack `#aibox-help` | 4 hours (business hours) | Platform team lead |
| Medium | Email | Next business day | Weekly review |
| Info | Dashboard only | Weekly review | None |

Note: Per PO decision, there is no AI-Box-specific on-call rotation. Platform team provides business-hours monitoring with after-hours best-effort. Critical infrastructure alerts (Harbor, Nexus, Vault) route to the existing infrastructure team on-call.

### 4.7 Alert Silencing

During planned maintenance:
```bash
amtool silence add --alertname HarborRegistryDown \
  --start "2026-03-01T02:00:00Z" --end "2026-03-01T04:00:00Z" \
  --comment "Planned Harbor maintenance"
```
All silences documented in `#aibox-help` with reason, duration, and authorizer.
