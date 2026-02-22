# Runbook: Tool Pack Update Process

**Owner**: Platform Engineering Team
**Frequency**: Monthly cadence + emergency updates
**Last Updated**: 2026-02-21

---

## 1. Update Cadence

| Update Type | Frequency | Trigger | Review |
|-------------|-----------|---------|--------|
| Routine version bumps | Monthly | Upstream releases | Champions test first |
| Security patches | Immediate | Critical CVE in tool | Emergency process |
| New tool pack addition | As needed | Developer request | Platform + security review |
| Deprecation | Quarterly review | Low usage / EOL upstream | 30-day notice to users |

---

## 2. Monthly Update Process

### Week 1: Identify Updates
1. Check upstream releases for all active tool packs:
   - Java/JDK (Adoptium, Oracle)
   - Node.js (nodejs.org)
   - .NET SDK (Microsoft)
   - Bazel (bazel.build)
   - Scala (scala-lang.org)
   - PowerShell Core (Microsoft)
   - Angular/AngularJS (angular.io)
   - AI tools (Claude Code, Codex CLI)
2. Document version changes and changelogs.
3. Assess breaking changes or compatibility risks.

### Week 2: Build and Test
1. Update tool pack manifests in `aibox-toolpacks/packs/`:
   ```yaml
   # Example: aibox-toolpacks/packs/node/manifest.yaml
   name: node
   version: "22.0"  # Updated from 20.x
   install:
     - apt-get install -y nodejs=22.0.0-1nodesource1
   ```
2. Run automated build pipeline:
   ```bash
   # Build updated tool pack
   aibox-ci toolpack build --pack node@22

   # Run tool pack tests
   aibox-ci toolpack test --pack node@22
   ```
3. Verify gVisor compatibility for updated versions.
4. Run full integration test suite.

### Week 3: Champion Testing
1. Deploy updated tool packs to champion preview channel.
2. Champions test with their real projects for 3-5 business days.
3. Champions report issues via `#aibox-champions` Slack channel.
4. Fix any reported issues.

### Week 4: General Release
1. Merge updated tool pack manifests to `main`.
2. Sign updated manifests with Cosign.
3. Push to Harbor as OCI artifacts.
4. Announce in `#aibox-announce`:
   ```
   TOOL PACK UPDATE: Monthly updates for [month]
   Updated: node@22.0, dotnet@8.0.4, java@21.0.3
   Changes: [link to changelog]
   Action: Run 'aibox update' to get latest packs. Updates apply on next 'aibox start'.
   ```

---

## 3. Emergency Tool Pack Updates

### When
- Critical security vulnerability in a tool pack component.
- Breaking compatibility issue affecting developer productivity.

### Process
1. Platform engineer identifies the urgent issue.
2. Build patched tool pack version.
3. Run automated tests (skip champion testing phase).
4. Push directly to Harbor.
5. Notify developers:
   ```
   URGENT TOOL PACK UPDATE: [pack]@[version] patched for [CVE/issue].
   Run 'aibox update' and restart your sandbox.
   ```

---

## 4. New Tool Pack Requests

### Request Process
1. Developer or champion submits request via `#aibox-help` or GitHub issue.
2. Platform team evaluates:
   - Number of developers who would use it.
   - Security implications (new network endpoints, system requirements).
   - Maintenance burden.
3. If approved:
   a. Create manifest in `aibox-toolpacks/packs/<name>/`.
   b. Build and test.
   c. Security review for any new network access or capabilities.
   d. Champion testing.
   e. General release.

### Community Contributions
Champions and power users can submit tool pack PRs:
1. Follow the "Building Tool Packs" guide.
2. Submit PR to `aibox-toolpacks` repository.
3. Platform team reviews for security and compatibility.
4. Champion testing before merge.

---

## 5. Tool Pack Deprecation

### Process
1. Identify tool packs with zero or near-zero usage (from telemetry).
2. Confirm with affected teams (if any) that the pack is no longer needed.
3. Post 30-day deprecation notice:
   ```
   DEPRECATION NOTICE: Tool pack [name]@[version] will be removed on [date].
   If you use this pack, contact #aibox-help to discuss alternatives.
   ```
4. After 30 days, remove from active catalog but keep archived in Harbor.

---

## 6. Version Compatibility Matrix

Maintain a living document tracking validated combinations:

| Tool Pack | Current Version | Min gVisor Version | WSL2 Status | Notes |
|-----------|-----------------|--------------------|-------------|-------|
| java | 21.0.3 | 20240101+ | Validated | JVM heap needs memory.swap |
| node | 22.0.0 | 20240101+ | Validated | |
| dotnet | 8.0.4 | 20240101+ | Validated | CLR needs /proc readlink |
| bazel | 7.4.0 | 20240101+ | Validated | Disable sandbox-within-sandbox |
| scala | 3.5.0 | 20240101+ | Validated | Requires java pack |
| powershell | 7.5.0 | 20240101+ | Validated | PSReadLine may need fallback |
| angular | 19.0.0 | 20240101+ | Validated | |
| angularjs | 1.8.3 | 20240101+ | Validated | Legacy; Grunt/Bower support |
| ai-tools | latest | 20240101+ | Validated | Claude Code + Codex CLI |
| python | 3.12 | 20240101+ | Validated | |

---

## 7. Metrics

| Metric | Target |
|--------|--------|
| Monthly update cycle completion | 100% of months |
| Tool pack build success rate | > 95% |
| Champion testing issues found | Track (no target) |
| Emergency updates per quarter | < 3 |
| Tool pack installation failures | < 1% of installs |
