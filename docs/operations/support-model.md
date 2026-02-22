# AI-Box Tiered Support Model

**Version**: 1.0
**Last Updated**: 2026-02-21
**Owner**: Platform Engineering Team

---

## Overview

The AI-Box support model is designed to scale support for ~200 developers without the platform team becoming a bottleneck. Support is structured in four tiers, each handling a progressively smaller percentage of issues with increasing specialization.

```
+------------------------------------------------------------------+
|                                                                  |
|   Developer encounters issue                                     |
|          |                                                       |
|          v                                                       |
|   [Tier 0: Self-Service]  -----> 70% resolved here              |
|   aibox doctor, FAQ, docs                                        |
|          |                                                       |
|          | unresolved                                            |
|          v                                                       |
|   [Tier 1: Champion]  ---------> 20% resolved here              |
|   Team champion, handbook                                        |
|          |                                                       |
|          | unresolved                                            |
|          v                                                       |
|   [Tier 2: Platform Team]  ----> 9% resolved here               |
|   #aibox-help, SLA-backed                                        |
|          |                                                       |
|          | infrastructure/security                               |
|          v                                                       |
|   [Tier 3: Escalation]  -------> 1% resolved here               |
|   Infra team, security team                                      |
|                                                                  |
+------------------------------------------------------------------+
```

---

## Tier 0: Self-Service

**Target**: 70% of all issues resolved without human assistance.

### Tools

| Tool | What It Does | When to Use |
|------|-------------|-------------|
| `aibox doctor` | Runs 10+ automated checks (Podman, gVisor, WSL2 memory, proxy, DNS, Harbor, image signature, policy) | First thing to try for any issue |
| `aibox network test` | Tests connectivity to all required endpoints (Harbor, Nexus, LLM gateway, Git server) | Network/proxy/build dependency failures |
| `aibox policy explain --log-entry <id>` | Explains why a specific action was blocked by policy | After any "blocked by policy" message |
| `aibox status` | Shows sandbox state, resource usage, tool packs, port mappings | General "is it running?" checks |
| `aibox repair cache` | Clears and rebuilds build caches, fixes permission issues | Build cache corruption, permission errors |

### Documentation

| Resource | Location | Content |
|----------|----------|---------|
| VS Code Quickstart | `docs/phase6/training/quickstart-vscode.md` | Setup through first build, troubleshooting |
| JetBrains Quickstart | `docs/phase6/training/quickstart-jetbrains.md` | Gateway setup through first build, troubleshooting |
| Troubleshooting FAQ | `docs/phase6/training/troubleshooting-faq.md` | 30+ common issues with step-by-step resolutions |
| Architecture Overview | `docs/phase6/training/architecture-overview.md` | How the system works (conceptual) |
| Building Tool Packs | `docs/phase6/training/building-toolpacks-guide.md` | For power users creating custom packs |
| "AI-Box in 5 Minutes" | Internal video platform | Narrated walkthrough of the full developer flow |

### Knowledge Base

Maintain a searchable knowledge base (Confluence or wiki) of known issues. Each entry includes:
- Symptoms (searchable keywords)
- Root cause
- Resolution steps
- Related `aibox doctor` check (if applicable)

Update the knowledge base whenever Tier 1 or Tier 2 resolves a new issue type.

### Success Metrics for Tier 0

| Metric | Target |
|--------|--------|
| Self-service resolution rate | 70% of all issues |
| `aibox doctor` check coverage | Covers top 20 known issue types |
| Documentation freshness | Updated within 1 week of new issue type discovery |
| Knowledge base search success | Developer finds relevant article >80% of searches |

---

## Tier 1: Champions

**Target**: 20% of all issues resolved by team champions.

### Responsibilities

- Handle team-level troubleshooting using the Champions Handbook
- Run `aibox doctor` on behalf of teammates if needed
- Check the Troubleshooting FAQ before escalating
- Provide context-rich escalations when they cannot resolve

### What Champions Can Resolve

- Environment-specific setup issues (WSL2 config, IDE settings)
- IDE connection problems (VS Code Remote SSH, JetBrains Gateway)
- Tool pack installation and configuration
- Build cache issues
- Explaining policy violation messages
- Onboarding walkthroughs

### What Champions Should Escalate

- Issues affecting multiple developers (systemic)
- Problems not in the handbook or FAQ
- Suspected platform bugs
- Tool pack or policy change requests
- Image signature or security-related failures

### Escalation Format

Champions escalate by posting in `#aibox-help` using this template:

```
**Issue**: [One-line summary]
**Affected**: [Number of developers, team name]
**Tried**: [What was already attempted]
**aibox doctor**: [output or "all checks passed"]
**aibox status**: [relevant output]
**Reproducible**: [Yes/No/Sometimes]
```

### Success Metrics for Tier 1

| Metric | Target |
|--------|--------|
| Champion resolution rate | 80% of issues that reach them |
| Time to first response | Same business day |
| Escalation quality | 90% of escalations include complete template |
| Team coverage | 100% of teams have an active champion |

---

## Tier 2: Platform Team

**Target**: 9% of all issues.

### Channel

**`#aibox-help`** on Slack. This is the single point of contact for all escalations from champions and direct developer requests.

### Staffing

| Phase | Staffing | Hours |
|-------|----------|-------|
| Pilot (Weeks 5-8) | 2 engineers, dedicated | Business hours |
| Early Adopter (Weeks 9-14) | 1.5 engineers | Business hours |
| General Rollout (Weeks 15-22) | 1 engineer | Business hours |
| Steady State (Week 23+) | 0.5 FTE | Business hours |

**No formal on-call rotation.** Coverage is business hours only (9am-5pm local, Monday-Friday). Critical after-hours issues are handled on a best-effort basis by the platform team.

### SLAs

| Priority | Acknowledge | Resolve/Workaround | Examples |
|----------|------------|-------------------|----------|
| **Critical** | 1 hour | 4 hours | Platform-wide outage, security incident, >10 devs blocked |
| **High** | 2 hours | 1 business day | Multiple devs blocked, broken tool pack, build system failure |
| **Medium** | 4 hours | 2 business days | Single dev blocked with no workaround, feature request |
| **Low** | 1 business day | 5 business days | Minor inconvenience, documentation gap, cosmetic issue |

SLAs apply during business hours (9am-5pm local time, Monday-Friday). There is no formal on-call rotation. Outside business hours, issues are handled on next business day.

### Triage Process

1. Issue arrives in `#aibox-help`
2. On-duty engineer acknowledges with a reaction emoji (within SLA)
3. Engineer assigns priority based on impact (devs affected, workaround exists?)
4. Engineer resolves or provides workaround
5. If resolution reveals a new common issue, update:
   - Troubleshooting FAQ
   - Champions Handbook
   - `aibox doctor` checks (if automatable)
6. Close with summary of root cause and fix

### What Tier 2 Handles

- Platform bugs and regressions
- Tool pack creation, updates, and fixes
- Policy change requests (coordinate with security team)
- Image build and signing issues
- Network/proxy configuration changes
- Performance investigations
- Complex environment-specific issues

### What Tier 2 Escalates to Tier 3

- Central infrastructure failures (Harbor, Nexus, Vault down)
- Security incidents (container escape, unauthorized access, data exfiltration)
- Network infrastructure issues (corporate proxy, firewall rules)
- Zero-day vulnerabilities in platform dependencies
- Compliance/audit findings requiring immediate action

### Success Metrics for Tier 2

| Metric | Target |
|--------|--------|
| SLA adherence | >95% of issues acknowledged within SLA |
| Resolution time (median) | <4 hours for High, <1 day for Medium |
| Ticket volume | <3 tickets/week during steady state |
| Repeat issue rate | <10% (same issue type recurring) |
| Knowledge base updates | New article within 1 week of new issue type |

---

## Tier 3: Escalation

**Target**: 1% of all issues.

### Scope

Tier 3 handles issues that require expertise or access beyond the platform team:

| Category | Responsible Team | Examples |
|----------|-----------------|----------|
| Infrastructure failures | Infrastructure/SRE team | Harbor, Nexus, or Vault outage; storage issues; network infrastructure |
| Security incidents | Security team | Suspected data exfiltration, container escape, unauthorized access |
| Compliance issues | Compliance/Security team | Audit findings, policy violations at org level |
| Vendor issues | Platform team + vendor | gVisor bugs, Podman regressions, Squid issues |

### Escalation Process

1. Platform team (Tier 2) identifies issue requires Tier 3
2. Platform team opens an incident ticket in the organization's incident management system
3. Platform team provides:
   - Full issue description and timeline
   - Impact assessment (developers affected, data risk)
   - Actions already taken
   - Relevant logs and diagnostics
4. Tier 3 team acknowledges and takes ownership
5. Platform team remains involved as liaison and communicates status to affected developers

### Incident Response (Security)

For suspected security incidents:

1. **Contain**: Isolate affected sandbox(es) immediately
2. **Assess**: Determine scope (single dev, team, org-wide)
3. **Escalate**: Notify security team via incident channel
4. **Communicate**: Inform affected developers with guidance
5. **Remediate**: Fix root cause, update defenses
6. **Review**: Post-incident review within 5 business days

### Success Metrics for Tier 3

| Metric | Target |
|--------|--------|
| Tier 3 escalation rate | <1% of all issues |
| Infrastructure recovery time | Within documented DR targets |
| Security incident response | Contained within 4 hours |
| Post-incident review completion | 100% of incidents reviewed |

---

## Support Metrics Tracking

### What to Track

| Metric | Source | Frequency | Dashboard |
|--------|--------|-----------|-----------|
| Total issue volume | `#aibox-help` + champion reports | Weekly | Support dashboard |
| Issues by tier | Triage data | Weekly | Support dashboard |
| Issues by category | Triage tags | Weekly | Support dashboard |
| Resolution time by priority | Ticket timestamps | Weekly | Support dashboard |
| SLA adherence rate | Ticket timestamps | Weekly | Support dashboard |
| Repeat issue rate | Issue categorization | Monthly | Trend report |
| Self-service resolution rate | Survey + `aibox doctor` telemetry | Monthly | Adoption dashboard |
| Champion resolution rate | Champion weekly reports | Monthly | Champions dashboard |
| Developer satisfaction | Quarterly survey (1-5 scale) | Quarterly | Executive report |

### Reporting

| Report | Audience | Frequency |
|--------|----------|-----------|
| Weekly support summary | Platform team | Weekly |
| Monthly support trends | Platform team + champions | Monthly (at champions sync) |
| Quarterly support review | Engineering leadership | Quarterly |

### Using Metrics to Improve

- **High repeat issue rate**: Add to FAQ and `aibox doctor` checks
- **High Tier 1 escalation rate**: Update Champions Handbook, provide additional champion training
- **High Tier 2 volume**: Invest in self-service tooling and documentation
- **Low self-service rate**: Improve discoverability of docs, enhance `aibox doctor`
- **SLA misses**: Review staffing, consider adding rotation

---

## Tooling Recommendations

### All Phases

| Tool | Purpose | Justification |
|------|---------|---------------|
| Slack `#aibox-help` (public) | Primary support channel for all developers | Low friction, real-time, searchable history |
| Slack `#aibox-champions` (private) | Champion coordination, early feature access | Direct platform team access for champions |
| Slack bookmarks/pins | FAQ and common answers | Quick reference for recurring questions |
| Git-hosted markdown (`docs/`) | Primary documentation | Versioned alongside code, PR-reviewed |
| Org knowledge base mirror | FAQ mirror for discoverability | Developers who do not check Git docs |

**No formal ticketing system** unless Slack thread volume exceeds 20 threads/week consistently. If that threshold is reached, evaluate Jira Service Management or equivalent.

### Steady State Additions

| Tool | Purpose | Justification |
|------|---------|---------------|
| Grafana dashboard | Support metrics visualization | Connected to telemetry data, real-time |
| `aibox telemetry show` | Developer views what telemetry is collected | Transparency requirement |
| `aibox telemetry opt-out` | Developer opts out of optional telemetry | Privacy requirement |

---

## Transition Plan

### Rollout Support Mode (Weeks 5-22)

- Platform team actively monitors `#aibox-help`
- Daily triage of all open issues
- Weekly support summary shared with team
- Champions onboarded and active by Week 12

### Steady-State Support Mode (Week 23+)

- Champion-first model: most issues resolved at Tier 0-1
- Platform team monitors `#aibox-help` during business hours
- No formal on-call -- business hours only, best-effort after hours
- Monthly review of support metrics, quarterly process review
- Transition criteria: <3 tickets/week for 4 consecutive weeks

**Mandatory cutover is metric-gated, not calendar-driven.** Three conditions must all be met before local dev becomes unsupported:
1. >90% adoption sustained for 2 consecutive weeks
2. <3 support tickets/week sustained for 4 consecutive weeks
3. Every team has a validated tool pack configuration

If conditions are not met by Week 30, a remediation plan is required. The transition is framed as "unsupported" (local dev at your own risk), not "prohibited."

---

*This support model is reviewed quarterly and updated based on support metrics and feedback.*
