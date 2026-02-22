# Runbook: Compatibility Testing

**Owner**: Platform Engineering Team
**Frequency**: Monthly
**Last Updated**: 2026-02-21

---

## 1. Test Matrix

### Operating Systems and Environments

| Environment | Priority | Notes |
|-------------|----------|-------|
| Windows 11 23H2+ WSL2 | Primary | 100% of target developers |
| Windows 11 24H2+ WSL2 | Primary | Validate on latest feature update |
| Ubuntu 24.04 LTS (native) | Secondary | CI/CD and optional dev machines |

### IDE Combinations

| IDE | Version | Connection Method | Priority |
|-----|---------|-------------------|----------|
| VS Code | Latest stable | Remote - SSH | Primary |
| VS Code | Latest insiders | Remote - SSH | Secondary |
| IntelliJ IDEA | Latest stable | Gateway SSH | Primary |
| WebStorm | Latest stable | Gateway SSH | Secondary |
| Rider | Latest stable | Gateway SSH | Secondary (for .NET) |
| PyCharm | Latest stable | Gateway SSH | Tertiary |

### Runtime Stacks

| Stack | Test Scenarios |
|-------|---------------|
| Java 21 (Gradle) | `gradle build`, `gradle test`, daemon persistence, debug attach |
| Java 21 (Maven) | `mvn compile`, `mvn test`, dependency resolution via Nexus |
| Node.js 22 (npm) | `npm install`, `npm run build`, `npm test`, dev server |
| Node.js 22 (yarn) | `yarn install`, `yarn build`, PnP mode |
| .NET SDK 8 | `dotnet build`, `dotnet test`, `dotnet restore` via NuGet/Nexus |
| Python 3.12 | `pip install`, `pytest`, virtual environments |
| Bazel 7 | `bazel build`, `bazel test`, remote cache |
| Scala 3 (sbt) | `sbt compile`, `sbt test` |
| PowerShell 7 | Script execution, module installation |
| Angular 19 | `ng serve`, `ng build`, `ng test` |
| AngularJS 1.x | `grunt build`, `bower install`, Karma tests |

---

## 2. Monthly Test Process

### Week 1: Environment Preparation
1. Provision clean Windows 11 WSL2 test machine (or reset existing).
2. Update WSL2 to latest kernel.
3. Run `aibox setup` from scratch.
4. Verify `aibox doctor` passes all checks.

### Week 2: Individual Stack Testing
For each runtime stack:
1. Start sandbox with appropriate tool pack: `aibox start --toolpacks <stack>`.
2. Clone a representative test project.
3. Execute all test scenarios for the stack.
4. Record: startup time, build time, test pass/fail, any errors.
5. Test IDE integration (VS Code and IntelliJ at minimum).

### Week 3: Polyglot and Stress Testing
1. Start sandbox with full polyglot stack:
   ```bash
   aibox start --toolpacks java@21,node@22,dotnet@8,bazel@7,python@3.12,powershell@7,ai-tools
   ```
2. Run multiple stacks concurrently (see WSL2 Polyglot Validation).
3. Monitor resource usage: CPU, memory, disk I/O.
4. Test AI agent (Claude Code) running alongside builds.

### Week 4: Results and Reporting
1. Compile results into test report.
2. File issues for any failures or regressions.
3. Update compatibility matrix in tool pack runbook.
4. Share results with platform team and champions.

---

## 3. Test Report Template

```markdown
# AI-Box Compatibility Test Report - [Month Year]

## Environment
- Windows 11 version: [version]
- WSL2 kernel: [version]
- Podman version: [version]
- gVisor version: [version]
- AI-Box CLI version: [version]

## Results Summary
| Stack | Build | Test | IDE | Performance | Status |
|-------|-------|------|-----|-------------|--------|
| Java 21 (Gradle) | PASS | PASS | PASS | 95% of local | OK |
| Node.js 22 | PASS | PASS | PASS | 98% of local | OK |
| ...

## Issues Found
1. [Issue description, severity, ticket link]

## Performance Notes
- Cold start time: [seconds]
- Warm start time: [seconds]
- Memory usage under polyglot load: [GB]

## Recommendations
- [Any changes needed]
```

---

## 4. Automated Test Suite

Where possible, automate compatibility tests:

```bash
# Run automated compatibility suite
aibox-ci compat-test --matrix all --report /tmp/compat-report.json

# Tests include:
# - Tool pack installation verification
# - Basic build/test for each stack
# - Network connectivity (Nexus, Harbor, LLM API)
# - SSH connectivity from IDE
# - Build cache persistence verification
```

### What Cannot Be Automated
- IDE-specific workflows (debugger attach, IntelliJ indexing).
- Subjective performance assessment.
- Edge cases in developer workflows.
- Visual rendering and UI issues in remote IDEs.

These require manual testing during the monthly cycle.

---

## 5. Regression Tracking

Maintain a regression tracking spreadsheet:

| Month | New Issues | Regressions | Fixed | Open |
|-------|-----------|-------------|-------|------|
| 2026-03 | 2 | 0 | 2 | 0 |
| 2026-04 | 1 | 1 | 1 | 1 |

Any regression (something that worked last month but fails this month) is treated as a high-priority bug.

---

## 6. Metrics

| Metric | Target |
|--------|--------|
| Monthly test cycle completion | 100% |
| Stack pass rate | > 95% |
| Regressions per month | 0 |
| Mean time to fix regression | < 1 week |
| IDE compatibility pass rate | 100% for primary IDEs |
