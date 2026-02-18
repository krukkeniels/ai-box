# Phase 6: Rollout & Operations

**Phase**: 6 of 6
**Estimated Effort**: 8-10 engineer-weeks (spread over 15-22+ weeks calendar time)
**Team Size**: 2-3 engineers + champions + support rotation
**Dependencies**: Phases 0-4 complete; Phase 5 can proceed in parallel with early rollout stages
**Spec Sections**: Section 21 (Transition and Rollout Plan), Section 22 (Operations)
**Status**: Not Started

---

## Overview

Phase 6 executes the organizational rollout of AI-Box from a working platform (delivered by Phases 0-5) to a system used by all ~200 developers. This is primarily an **organizational change management** phase, not a pure engineering phase. The work spans pilot validation, early adopter expansion, champion program activation, training material production, general rollout execution, support model establishment, day-2 operations handoff, and disaster recovery procedures.

The rollout follows the spec's phased approach: Pilot (10 devs) -> Early Adopters (30-40 devs) -> General Rollout (remaining devs) -> Mandatory adoption. Each stage has explicit exit criteria that must be met before advancing to the next. A fallback to local development must remain available throughout Phases 1-3 of the rollout so that AI-Box is additive, never a gate.

The overarching principle is **invisible security**: if developers notice friction, the rollout has failed. Feedback loops at every stage drive iteration on the platform itself, not just the rollout process.

---

## Deliverables

| # | Deliverable | Stage |
|---|-------------|-------|
| D1 | Pilot cohort selected and onboarded (10 developers) | Pilot Program |
| D2 | Feedback collection system (surveys, metrics) operational | Pilot Program |
| D3 | Top 5 pilot pain points identified and fixed | Pilot Program |
| D4 | Self-service onboarding flow validated and published | Early Adopter Expansion |
| D5 | Early adopter cohort onboarded (30-40 developers) | Early Adopter Expansion |
| D6 | Champions program launched (15-20 champions, 1 per team) | Champions Program |
| D7 | Champions handbook and direct communication channel | Champions Program |
| D8 | "AI-Box in 5 minutes" screen recording | Training Materials |
| D9 | VS Code quickstart guide (written + screenshots) | Training Materials |
| D10 | IntelliJ quickstart guide (written + screenshots) | Training Materials |
| D11 | Troubleshooting FAQ (wiki/Confluence) | Training Materials |
| D12 | "Building Tool Packs" guide (written + examples) | Training Materials |
| D13 | Architecture overview document (diagram + written) | Training Materials |
| D14 | All remaining developers onboarded | General Rollout |
| D15 | Migration office hours schedule and process | General Rollout |
| D16 | Tiered support model operational (Tier 0-3) | Support Model |
| D17 | Day-2 operations runbooks (image patching, CVE triage, policy updates, tool pack updates, compatibility testing) | Day-2 Operations |
| D18 | Monitoring dashboards and alerting for fleet health | Day-2 Operations |
| D19 | Disaster recovery procedures documented and tested | Disaster Recovery |
| D20 | KPI tracking dashboard (adoption rate, startup p95, tickets, fallback frequency, security events) | Day-2 Operations |

---

## Implementation Steps

### Work Stream 1: Pilot Program

**Objective**: Validate AI-Box with 10 hand-picked developers across diverse teams, OS types, and IDE preferences. Identify and fix critical friction before broader exposure.

**What to do**:

1. **Select pilot cohort** (10 volunteer developers):
   - Ensure diversity: at least 2 teams represented, both Windows 11/WSL2 and native Linux, both VS Code and JetBrains users, mix of project types (Java, Node, Python, monorepo).
   - Prioritize developers who are enthusiastic but also honest about friction.

2. **Pair each pilot developer with a platform engineer** for their first day:
   - Walk through `aibox setup`, first `aibox start`, IDE connection, and first AI tool use.
   - Platform engineer observes friction points in real time and documents them.

3. **Deploy feedback collection**:
   - Daily feedback surveys for the first 2 weeks (short: 3-5 questions, < 2 minutes).
   - Transition to weekly surveys after week 2.
   - Track quantitative metrics: startup time, build time delta vs local, support requests, fallback-to-local frequency.

4. **Run pilot for 4 weeks** (spec Weeks 5-8):
   - Week 1-2: intensive support, daily feedback.
   - Week 3-4: reduced support, weekly feedback, pilot devs self-sufficient.

5. **Identify and fix top 5 pain points** before moving to Early Adopters:
   - Triage all feedback into categories (performance, usability, missing tools, bugs, policy friction).
   - Fix the top 5 most impactful issues.
   - Validate fixes with pilot cohort.

**Key decisions**:
- Cohort selection criteria and process (volunteer vs nominated, team coverage requirements).
- Survey tooling choice (Google Forms, Typeform, internal tool).
- Threshold for "acceptable" rating: the spec requires 8/10 pilot devs rating experience as "acceptable or better."
- What constitutes a "pain point" severe enough to block Early Adopter expansion.

**Spec references**: Section 21.1 Phase 1 (Pilot), Section 21.5 (Fallback Plan).

---

### Work Stream 2: Early Adopter Expansion

**Objective**: Scale from 10 to 30-40 developers with self-service onboarding. Validate that the platform scales without 1:1 support.

**What to do**:

1. **Open enrollment to volunteers and nominated team leads**:
   - Announce via internal comms (email, Slack, team meetings).
   - Provide clear enrollment process (self-service, not gated by platform team).

2. **Build and validate self-service onboarding flow**:
   - Written quickstart guide (VS Code and IntelliJ paths).
   - `aibox setup` must complete without manual intervention on supported machines.
   - `aibox doctor` must catch and explain all common setup issues.
   - First-run experience must result in a working sandbox within the 90-second cold start SLA.

3. **Run Early Adopter phase for 6 weeks** (spec Weeks 9-14):
   - Weekly feedback surveys.
   - Track support ticket volume (target: < 3 tickets/week).
   - Track startup time at p95 (target: < 90 seconds).
   - Build additional tool packs based on demand from early adopters.

4. **Maintain parallel local dev**:
   - AI-Box and local development run side by side.
   - Track fallback frequency as a signal of what is broken.

5. **Iterate on platform based on early adopter feedback**:
   - Prioritize issues by frequency and severity.
   - Ship fixes and improvements continuously.

**Key decisions**:
- Whether to cap early adopter enrollment or allow open signup.
- Criteria for nominating team leads vs accepting all volunteers.
- Threshold for support ticket volume that would pause expansion.
- Which additional tool packs to build based on demand signals.

**Spec references**: Section 21.1 Phase 2 (Early Adopters), Section 21.5 (Fallback Plan), Section 18.1 (Performance SLAs).

---

### Work Stream 3: Champions Program

**Objective**: Establish a distributed support and advocacy network of 15-20 champions (1 per team) who serve as the first point of contact for their team and as a feedback channel to the platform team.

**What to do**:

1. **Recruit champions** (1 per team, 15-20 total):
   - Identify candidates from early adopter cohort (enthusiastic, technically strong, respected by peers).
   - Ensure every team has at least one champion.
   - Champions should be volunteers, not conscripted.

2. **Set up champions infrastructure**:
   - Dedicated Slack channel (e.g., `#aibox-champions`) with direct platform team access.
   - Early access to new features and tool packs before general release.
   - Monthly champions sync meeting with platform team.

3. **Define champion responsibilities**:
   - First point of contact for their team's AI-Box questions (Tier 1 support).
   - Surface pain points and feature requests to platform team.
   - Test new tool packs and features before wider release.
   - Assist with team onboarding during general rollout.
   - Submit and review tool pack PRs (power users).

4. **Create champions handbook**:
   - Common issues and resolutions.
   - Escalation path (when to escalate to platform team).
   - How to submit tool pack requests.
   - How to provide structured feedback.

5. **Launch during Early Adopter phase** (spec Week 9-14):
   - Champions active before general rollout begins.
   - Champions participate in training material review.

**Key decisions**:
- Time commitment expected from champions (estimate: 1-2 hours/week).
- Whether champions receive any formal recognition (title, performance review credit).
- Champion rotation policy (fixed term or ongoing).
- How to handle teams without a willing champion.

**Spec references**: Section 21.3 (Champions Program), Section 21.4 (Support Model -- Tier 1).

---

### Work Stream 4: Training Materials

**Objective**: Produce the full set of training materials specified in the spec so that developers can self-serve onboarding and troubleshooting.

**What to do**:

1. **"AI-Box in 5 minutes" screen recording**:
   - Record a screencast showing the full developer flow: `aibox start`, IDE connect, AI tool use, build, push.
   - Keep it under 5 minutes. Narrated, captioned.
   - Hosted on internal video platform or wiki.

2. **Quickstart guide: VS Code**:
   - Written guide with screenshots.
   - Covers: `aibox setup`, `aibox start`, VS Code Remote SSH connection, terminal use, AI tool invocation.
   - Troubleshooting section for common VS Code Remote SSH issues.

3. **Quickstart guide: IntelliJ/JetBrains**:
   - Written guide with screenshots.
   - Covers: JetBrains Gateway setup, SSH connection to sandbox, backend installation, indexing, debugging.
   - Troubleshooting section for common Gateway issues.

4. **Troubleshooting FAQ**:
   - Compiled from pilot and early adopter feedback.
   - Covers: startup failures, network issues (`aibox network test`), IDE connection problems, build failures, policy violations.
   - Linked from `aibox doctor` output where applicable.

5. **"Building Tool Packs" guide**:
   - For power users and champions.
   - Covers: manifest schema, testing locally, submitting PRs, governance and approval process.
   - Includes worked examples for a simple and a complex tool pack.

6. **Architecture overview**:
   - For curious developers and new team members.
   - Includes the architecture diagram from spec Section 5.
   - Explains the security model at a conceptual level without implementation details that could aid bypass.

**Key decisions**:
- Documentation platform (Confluence, internal wiki, Git-hosted markdown, or dedicated docs site).
- Whether training materials need security review (architecture overview should be reviewed).
- Screen recording tooling and hosting.
- Localization requirements (if any).

**Spec references**: Section 21.2 (Training Materials), Appendix A (CLI Reference), Appendix B (Developer Quickstart).

---

### Work Stream 5: General Rollout

**Objective**: Migrate all remaining developers (~130-160) from local development to AI-Box as the default development environment.

**What to do**:

1. **Default all new projects to AI-Box**:
   - Update project creation templates/tooling to include `aibox` configuration.
   - New projects start with a `/aibox/policy.yaml` in the repo.

2. **Migrate existing projects team by team**:
   - Work with each team lead and their champion to plan migration.
   - Identify team-specific tool pack or policy requirements before migration.
   - Create per-team migration checklist.

3. **Establish migration office hours** (3x/week during general rollout):
   - Platform engineers available for live troubleshooting.
   - Scheduled at different times to accommodate different team schedules.

4. **Track and report adoption metrics**:
   - Active AI-Box users / total developers (target: > 90%).
   - Teams fully migrated / total teams.
   - Fallback-to-local frequency (should trend toward zero).

5. **Handle stragglers and edge cases**:
   - Identify developers who have not adopted and understand why.
   - Provide 1:1 migration support for holdouts (spec Phase 4: Mandatory).
   - Document edge cases that require special configuration.

6. **Transition to mandatory** (spec Week 23+):
   - Local development unsupported (not removed, but no assistance).
   - Full policy enforcement activated.
   - Communication is clear: AI-Box is the standard, local dev is at your own risk.

**Key decisions**:
- Team migration order (by willingness, by project complexity, by security sensitivity).
- Timeline for mandatory cutover.
- How to handle projects with unique requirements that existing tool packs do not cover.
- Communication plan for the "mandatory" transition (avoid antagonizing holdouts).
- Whether to enforce via technical controls (e.g., AI tool licenses only work inside sandbox) or organizational policy.

**Spec references**: Section 21.1 Phase 3 (General Rollout), Section 21.1 Phase 4 (Mandatory), Section 21.5 (Fallback Plan).

---

### Work Stream 6: Support Model

**Objective**: Establish the tiered support model so that the platform team is not the bottleneck for every question.

**What to do**:

1. **Tier 0: Self-service** (target: 70% of issues):
   - Published documentation (quickstarts, FAQ, architecture overview).
   - `aibox doctor` self-diagnostics command.
   - `aibox policy explain --log-entry <id>` for policy violation understanding.
   - `aibox network test` for connectivity troubleshooting.
   - Searchable knowledge base of known issues.

2. **Tier 1: Champions** (target: 20% of issues):
   - Champions handle team-level troubleshooting.
   - Champions have access to champions handbook with common resolutions.
   - Escalation path to Tier 2 is well-defined and low-friction.

3. **Tier 2: Platform team** (target: 9% of issues):
   - Dedicated Slack channel (`#aibox-help` or similar).
   - Platform team monitors during business hours.
   - SLA: acknowledge within 4 hours, resolve or provide workaround within 1 business day.
   - Tool pack and policy requests routed through this tier.

4. **Tier 3: Escalation** (target: 1% of issues):
   - Infrastructure or security issues requiring specialized expertise.
   - Involves infrastructure team, security team, or vendor support.
   - Incident response process for security events.

5. **Track support metrics**:
   - Ticket volume by tier.
   - Resolution time by tier.
   - Repeat issue frequency (identifies documentation gaps).
   - Categories of issues (guides platform improvement priorities).

**Key decisions**:
- Support tooling (Slack channel, ticketing system, or both).
- On-call rotation for platform team during rollout.
- SLA definitions for each tier.
- When to shift from rollout support mode to steady-state support mode.

**Spec references**: Section 21.4 (Support Model).

---

### Work Stream 7: Day-2 Operations

**Objective**: Establish ongoing operational processes that keep AI-Box healthy, secure, and current after rollout completes.

**What to do**:

1. **Image lifecycle management**:
   - Automated weekly image rebuild via CI pipeline (already built in Phase 0).
   - Validate that `aibox update` pulls latest signed image and prompts developers.
   - Critical CVE response: mandatory update within 24 hours, `aibox start` refuses outdated image.
   - Harbor garbage collection: weekly cron to reclaim storage from old tags.

2. **CVE triage process**:
   - Daily review of Trivy scan results from Harbor.
   - Severity-based response: Critical/High CVEs trigger immediate rebuild; Medium/Low batched into weekly rebuild.
   - Documented escalation path for zero-day vulnerabilities.

3. **Policy update process**:
   - Policy changes via Git PR to `aibox-policies` repository.
   - Require security team review for org baseline changes.
   - Team leads can merge team-level policy changes (tighten only).
   - Changes propagate to developers on next `aibox start`.

4. **Tool pack update process**:
   - Monthly update cadence for tool pack versions (upstream releases).
   - Automated build and test pipeline for tool packs.
   - Champions test updated packs before general release.
   - Emergency tool pack updates for critical security issues.

5. **Compatibility testing**:
   - Monthly test matrix: Windows 11 + WSL2, native Linux, VS Code (latest), JetBrains (latest).
   - Automated where possible; manual for IDE-specific workflows.
   - Results published to platform team and champions.

6. **Monitoring and alerting**:
   - Dashboards for: sandbox fleet health, image age distribution, policy violation trends, support ticket trends, adoption metrics.
   - Alerts for: Harbor/Nexus downtime, image signing failures, abnormal policy violation spikes, Vault connectivity issues.

7. **Staffing**:
   - 0.5-1 FTE ongoing for local-first model (spec Section 22.1).
   - 2 FTEs during rollout phases (Phases 0-2 of rollout).
   - Reduce to steady-state staffing once general rollout is complete.

**Key decisions**:
- Monitoring tooling (Prometheus + Grafana, Datadog, internal platform).
- Alert routing and on-call rotation.
- Image retention policy (how many old tags to keep).
- Process for deprecating old tool pack versions.
- Handoff plan from rollout team to steady-state operations team.

**Spec references**: Section 22.1 (Day-2 Operations), Section 22.2 (Image Updates), Section 22.3 (`aibox doctor`), Section 17.6 (Update Cadence).

---

### Work Stream 8: Disaster Recovery

**Objective**: Document and test recovery procedures for all failure scenarios so that developer downtime is minimized.

**What to do**:

1. **Developer machine failure**:
   - Procedure: re-image machine, run `aibox setup`, pull images from Harbor, clone repos from Git server.
   - Recovery time target: 1-2 hours.
   - Build caches are lost (named volumes are local); document re-warming process.

2. **Harbor registry failure**:
   - Impact: developers cannot pull new images; existing cached images continue to work.
   - Procedure: developers continue working with local images. Platform team restores Harbor from backup.
   - Mitigation: HA deployment of Harbor. Image replication to secondary site.
   - Document: how to verify local image integrity without registry access.

3. **Nexus repository failure**:
   - Impact: builds fail if dependencies are not cached locally.
   - Procedure: priority restoration of Nexus. Build caches mitigate impact for most developers.
   - Mitigation: Nexus proxy caching retains previously-downloaded artifacts.
   - Document: how to use local build caches to continue working during Nexus outage.

4. **Vault failure**:
   - Impact: new credential issuance fails; cached credentials valid until TTL expires.
   - Procedure: developers continue with cached creds. Platform team restores Vault (HA pair).
   - Mitigation: graceful degradation built into `aibox` CLI. Fallback to simplified credential broker if Vault is down for extended period.
   - TTL grace period: Git tokens (4h), LLM API keys (8h), package mirror tokens (8h).

5. **Tabletop exercises**:
   - Run tabletop disaster recovery exercises quarterly.
   - Simulate each failure scenario and walk through the recovery procedure.
   - Update runbooks based on exercise findings.

6. **Runbook format**:
   - Each scenario: symptoms, impact assessment, immediate actions, recovery steps, verification, post-incident review.
   - Stored alongside operations runbooks in versioned documentation.

**Key decisions**:
- HA requirements for Harbor, Nexus, and Vault (single node vs HA pair vs cluster).
- Backup frequency and retention for each central service.
- Acceptable downtime targets (RTO) for each service.
- Whether to implement automated failover or rely on manual recovery.
- Frequency of DR tabletop exercises.

**Spec references**: Section 22.4 (Disaster Recovery), Section 4 (Design Principles -- Graceful Degradation).

---

## Research Required

### Developer Survey and Feedback Tooling

- Evaluate lightweight survey tools for daily/weekly developer feedback (Google Forms, Typeform, Microsoft Forms, custom Slack bot).
- Requirements: quick to complete (< 2 minutes), quantitative scoring (1-5 scale), free-text field, exportable data for trend analysis.
- Decide whether to build a custom feedback mechanism integrated into `aibox` CLI (e.g., `aibox feedback`).

### Notification Systems for Git Push Approvals

- The spec defines a non-blocking `git push` approval flow (Section 18.5) where pushes go to a staging ref and an approver is notified.
- Research notification delivery mechanisms: Slack webhook, email notification, custom dashboard, IDE notification.
- Evaluate integration with existing code review tools (GitHub/GitLab review flows, custom webhook-based systems).
- Determine approver assignment logic (team lead, rotating reviewer, self-service after review).

### Metric Collection for KPIs

- Identify infrastructure for collecting and visualizing rollout KPIs:
  - **Adoption rate**: active `aibox` users per week / total developers.
  - **Startup time p95**: telemetry from `aibox start` command.
  - **Support ticket volume**: integration with support tooling.
  - **Fallback frequency**: telemetry from developers falling back to local dev.
  - **Security event count**: from SIEM/Falco alerts (Phase 5 output).
- Evaluate: Prometheus + Grafana, Datadog, internal BI tooling, or simple spreadsheet tracking.
- Privacy considerations: what telemetry is acceptable to collect from developer machines.

### Migration Patterns for Existing Projects

- Survey existing projects to identify:
  - Language/runtime distribution (Java, Node, Python, Scala, mixed).
  - Build system distribution (Gradle, Maven, npm, Bazel, other).
  - Special dependencies (databases, message queues, external services used in dev).
  - Custom development tooling or scripts that may need adaptation.
- Develop per-project-type migration checklists.
- Identify projects with unique requirements that need custom tool packs or policy exceptions.
- Estimate migration effort per project type to inform rollout scheduling.

---

## Open Questions

1. **Mandatory cutover timeline**: What is the target date for Phase 4 (Mandatory) where local dev is unsupported? Is this a hard deadline or flexible based on adoption metrics?

2. **Feedback tool selection**: Should we build a custom feedback mechanism into the `aibox` CLI or use an external survey tool? Integration is better for adoption, external is faster to deploy.

3. **Champion incentives**: What formal recognition do champions receive? Performance review credit, team allocation adjustment, or purely voluntary?

4. **Support tooling**: Slack channel alone or a ticketing system? Slack is faster but loses history; tickets are trackable but add friction.

5. **Telemetry privacy**: What usage telemetry can we collect from developer machines? Startup time and crash reports are likely acceptable; command history or file access patterns are not.

6. **Rollout blocking on Phase 5**: Does the classified environment compliance requirement mean audit capabilities (Phase 5) must be complete before the pilot begins, or can audit run in parallel with early rollout?

7. **Multi-project developers**: How do developers who work on multiple projects manage multiple sandbox instances? One per project, or a single sandbox with multiple workspaces?

8. **Machine provisioning**: Who is responsible for upgrading developer machines that do not meet the minimum spec (16GB RAM, SSD, 8+ cores)? Hardware procurement timelines could delay rollout.

9. **Notification mechanism for git push approvals**: Slack, email, dashboard, or IDE plugin? Needs to be low-latency and reliable.

10. **Team migration order**: Do we migrate by team willingness (easiest first), by project complexity (simplest first), or by security sensitivity (most sensitive first)?

---

## Dependencies

### Blocking Dependencies (must be complete before Phase 6 begins)

| Dependency | Source Phase | What It Provides |
|------------|-------------|-----------------|
| Base images and tool packs in Harbor | Phase 0 | Developers need images to pull |
| `aibox` CLI with all commands | Phase 1 | Developers need the CLI to interact with the system |
| Network security stack operational | Phase 2 | Security controls must be enforced during pilot |
| Policy engine and credential management | Phase 3 | Policy enforcement and credential injection needed for real use |
| IDE integration and tool packs | Phase 4 | Developers need their IDE and tools to work |

### Non-Blocking Dependencies (can proceed in parallel)

| Dependency | Source Phase | Impact If Not Ready |
|------------|-------------|-------------------|
| Audit and monitoring pipeline | Phase 5 | Pilot can proceed without centralized audit; logs exist locally. May block rollout in classified environments. |
| SIEM integration and alerting | Phase 5 | Security posture monitoring deferred; compensated by local logging |
| Session recording | Phase 5 | Optional capability; not needed for pilot or early adopter phases |

### External Dependencies

| Dependency | Owner | Risk |
|------------|-------|------|
| Developer machine hardware meets minimum spec | IT/Procurement | Machines below 16GB RAM will have poor experience |
| Internal communication channels (Slack, email) for notifications | IT | Needed for support model and git push approvals |
| Security team availability for compliance validation | Security | May gate rollout in classified environments |
| Team lead availability for migration planning | Engineering leadership | Team-by-team migration requires lead coordination |

---

## Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|-----------|--------|-----------|
| R1 | Developer adoption resistance ("slower than local") | High | High | Invest in DX before rollout. Warm start < 15s. Build cache persistence. Fix top 5 pain points from pilot before expanding. Champions advocate within teams. |
| R2 | Support volume overwhelms platform team during general rollout | Medium | High | Tiered support model reduces direct platform team load. Champions handle 20% of issues. Self-service docs and `aibox doctor` handle 70%. Stagger team migrations. |
| R3 | Training materials are insufficient or out of date | Medium | Medium | Review materials with pilot cohort and champions before publishing. Version materials alongside code. Assign documentation ownership. |
| R4 | Champions burn out or disengage | Medium | Medium | Keep champion time commitment low (1-2 hrs/week). Provide recognition. Rotate champions annually. Ensure platform team remains responsive to champion feedback. |
| R5 | Tool pack gaps block specific teams from migrating | Medium | High | Survey all teams for tool requirements during early adopter phase. Prioritize tool pack development based on migration schedule. Allow self-service tool pack contributions with review. |
| R6 | Hardware constraints prevent some developers from running AI-Box | Low | Medium | Identify machines below spec early. Work with IT/procurement on upgrades. Centralized Coder option (Phase 2 of deployment model) as long-term fallback for thin clients. |
| R7 | Central infrastructure failure during rollout damages trust | Low | High | HA deployment for Harbor, Nexus, Vault. Graceful degradation in `aibox` CLI. Communicate transparently about outages. DR procedures tested before general rollout. |
| R8 | Mandatory cutover creates backlash | Medium | Medium | Generous timeline. Clear communication. 1:1 support for holdouts. Frame as enablement, not restriction. Ensure fallback period is long enough. |
| R9 | Classified environment compliance blocks rollout | Medium | High | Engage compliance team during pilot phase. Phase 5 (audit) runs in parallel. Map controls to compliance framework early. |
| R10 | Scope creep during rollout (feature requests from adopters) | High | Medium | Strict change control. Feature requests go to backlog, not sprint. Rollout focuses on stability and adoption, not new features. |

---

## Exit Criteria

### Pilot Program Exit Criteria
- 10 developers onboarded and using AI-Box for daily work.
- 8/10 pilot developers rate experience as "acceptable or better" (spec requirement).
- Top 5 pain points identified, fixed, and validated.
- Feedback collection system operational and producing actionable data.

### Early Adopter Exit Criteria
- 30-40 developers onboarded via self-service.
- < 3 support tickets/week (spec requirement).
- Startup time < 90 seconds at p95 (spec requirement).
- Champions program launched with at least 1 champion per team.
- All training materials published and reviewed by champions.

### General Rollout Exit Criteria
- \> 90% of active developers using AI-Box (spec requirement).
- All teams have completed migration or have a documented exception.
- Migration office hours wound down (demand drops to near zero).
- Fallback-to-local frequency < 5% of sessions.

### Operations Readiness Exit Criteria
- Day-2 operations runbooks documented and tested via tabletop exercise.
- Disaster recovery procedures documented and tested via tabletop exercise.
- Monitoring dashboards operational with alerting configured.
- Steady-state support model operational (Tier 0-3).
- Staffing transitioned from rollout mode (2 FTE) to steady-state (0.5-1 FTE).

### Phase 6 Overall Exit Criteria
All of the above, plus:
- KPI tracking dashboard operational and reporting: adoption rate, startup p95, ticket volume, fallback frequency, security events.
- Champions program active and self-sustaining.
- No critical unresolved issues from any rollout stage.

---

## Estimated Effort

| Work Stream | Effort | Calendar Time | Notes |
|-------------|--------|---------------|-------|
| Pilot Program | 2-3 eng-weeks | Weeks 15-18 | Heavy support load; 1:1 pairing in week 1 |
| Early Adopter Expansion | 2-3 eng-weeks | Weeks 19-22 | Self-service onboarding reduces per-dev effort |
| Champions Program | 1 eng-week | Weeks 19-20 | Setup overlaps with early adopter phase |
| Training Materials | 1-2 eng-weeks | Weeks 17-20 | Draft during pilot, finalize during early adopter |
| General Rollout | 2-3 eng-weeks | Weeks 22-28+ | Calendar time longer than effort due to team-by-team pacing |
| Support Model | 0.5 eng-week | Week 18-19 | Define process and tooling; execution is ongoing |
| Day-2 Operations | 1-2 eng-weeks | Weeks 20-22 | Runbooks + dashboards + alerting setup |
| Disaster Recovery | 0.5-1 eng-week | Week 21-22 | Documentation + tabletop exercise |
| **Total** | **8-10 eng-weeks** | **~14 weeks calendar** | Spread over Weeks 15-28+, with 2-3 engineers |

**Staffing model**:
- Weeks 15-22 (pilot through early adopter): 2-3 engineers, 0.5 FTE on support rotation.
- Weeks 22-28+ (general rollout): 1-2 engineers, support shifts to champions + Tier 2.
- Post-rollout (steady state): 0.5-1 FTE ongoing operations.
