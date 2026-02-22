# AI-Box Champions Handbook

**Version**: 1.0
**Audience**: AI-Box Champions
**Last Updated**: 2026-02-21

---

## 1. Welcome

You have been selected as an AI-Box Champion for your team. Champions are the connective tissue between the ~200 developers using AI-Box and the platform team that builds it. This handbook is your primary reference for resolving issues, escalating problems, and providing structured feedback.

Your time commitment is **1-2 hours per week**. Most of that time is answering quick questions from teammates and testing new features before they ship to everyone.

---

## 2. Champion Role Definition

### What You Do

| Responsibility | Frequency | Example |
|---------------|-----------|---------|
| Answer team questions about AI-Box | As needed | "How do I add a tool pack?" |
| Triage issues before escalating | As needed | Run `aibox doctor`, check FAQ |
| Test new tool packs and features | Before each release | Platform team pushes to `#aibox-champions` |
| Surface pain points to platform team | Weekly | Post in `#aibox-champions` or monthly sync |
| Assist with team onboarding | During general rollout | Walk teammates through quickstart |
| Submit tool pack requests | As needed | File request via process below |
| Review tool pack PRs | Optional | For power users comfortable with manifests |

### What You Do NOT Do

- You are not on-call. There is no pager.
- You are not responsible for fixing platform bugs. You report them.
- You are not expected to know the internals of gVisor, nftables, or OPA.
- You do not approve policy exceptions. That is the security team.

---

## 3. Common Issues and Resolutions

### 3.1 Startup and Setup

| Symptom | Likely Cause | Resolution |
|---------|-------------|------------|
| `aibox setup` fails on WSL2 | WSL2 not installed or outdated | Run `wsl --update` in PowerShell as admin, restart |
| `aibox setup` fails: "Podman not found" | Podman not installed in WSL2 | `aibox setup` should install it; if not, run `sudo apt install podman` |
| `aibox start` hangs | Image pull in progress (first run) | Wait for cold start (up to 90s). Check `aibox status` |
| `aibox start` fails: "image signature invalid" | Harbor signing key rotated or image tampered | Run `aibox update` to pull fresh image. If persists, escalate to Tier 2 |
| `aibox start` fails: "image too old" | Mandatory update required (critical CVE) | Run `aibox update` |
| Container starts but SSH fails | Port conflict on 2222 | Check `ss -tlnp | grep 2222`. Stop conflicting process or use `aibox start --ssh-port 2223` |
| Slow cold start (>90s) | Network congestion or Harbor slow | Run `aibox doctor`, check Harbor connectivity. Report if persistent |

### 3.2 IDE Connection

| Symptom | Likely Cause | Resolution |
|---------|-------------|------------|
| VS Code "Could not establish connection" | Container not running or SSH not ready | Run `aibox status`. If running, wait 10s and retry |
| VS Code reconnect loop | VS Code Server crashed inside container | `aibox stop && aibox start`. If persists, `aibox repair cache` |
| JetBrains Gateway "Connection refused" | SSH not ready or wrong port | Verify port in `aibox status`. Gateway SSH config must match |
| JetBrains indexing never completes | Insufficient RAM | Ensure WSL2 has at least 8GB allocated. Edit `.wslconfig` |
| Extensions not loading | Extension marketplace blocked by proxy | Check if extension is pre-approved. Request via tool pack process |
| Terminal inside IDE is slow | Shell init scripts doing network calls | Review `.bashrc`/`.zshrc` for external fetches |

### 3.3 Network and Proxy

| Symptom | Likely Cause | Resolution |
|---------|-------------|------------|
| `npm install` fails: ECONNREFUSED | Nexus mirror down or not configured | Run `aibox network test`. Check proxy config with `aibox doctor` |
| `pip install` fails: connection timeout | PyPI mirror not in allowlist | Verify `nexus.internal` is reachable via `aibox network test` |
| `git clone` fails for external repo | Repo host not in egress allowlist | Check project policy. Request allowlist addition if needed |
| `curl` fails inside container | `curl` is disabled by default (network policy) | Use package manager through Nexus mirror instead. `curl` to allowlisted hosts only |
| DNS resolution fails | CoreDNS not running or domain not allowlisted | Run `aibox doctor`. Check DNS status |
| LLM API calls fail | API endpoint unreachable or credential expired | Run `aibox network test`. Credential refresh is automatic; if stuck, `aibox stop && aibox start` |

### 3.4 Build and Development

| Symptom | Likely Cause | Resolution |
|---------|-------------|------------|
| Build slower than local | Build cache cold (first build) | Subsequent builds use persistent cache. Run build twice to confirm |
| `gradle build` OOM | Insufficient memory for JVM + tools | Increase WSL2 memory in `.wslconfig`. Recommend 16GB+ for JVM projects |
| `.NET restore` fails | NuGet mirror not configured | Verify dotnet tool pack installed: `aibox install dotnet@8` |
| Bazel sandbox-in-sandbox issues | gVisor + Bazel sandboxing conflict | Use `--spawn_strategy=local` flag for Bazel. Known limitation |
| File permission errors in `/workspace` | UID mapping issue | Run `aibox repair cache`. If persists, escalate to Tier 2 |
| Hot reload not working | File watcher limit reached | Increase inotify limit: documented in troubleshooting FAQ |

### 3.5 Policy Violations

| Symptom | Likely Cause | Resolution |
|---------|-------------|------------|
| "Operation blocked by policy" | OPA policy denied the action | Run `aibox policy explain --log-entry <id>` to see why |
| `git push` pending approval | Push gating enabled in project policy | Check `aibox push status`. Approver will be notified |
| Tool blocked from running | Tool not in project allowlist | Check project `policy.yaml`. Request addition if needed |
| MCP server failed to start | MCP pack requires endpoints not in policy | Run `aibox mcp list` to check status. May need policy update |

---

## 4. Escalation Procedures

### When to Handle It Yourself (Tier 1)

- Issue is in the Common Issues table above
- `aibox doctor` identifies and suggests a fix
- Issue is documented in the Troubleshooting FAQ
- Issue is specific to one developer's environment (not systemic)

### When to Escalate to Tier 2 (Platform Team)

Escalate to `#aibox-help` on Slack when:

- `aibox doctor` reports a problem it cannot fix
- The issue affects multiple developers on your team
- The issue is not in this handbook or the FAQ
- You suspect a platform bug (reproducible on multiple machines)
- A tool pack is missing or broken
- A policy change is needed
- Image signature verification consistently fails

**How to escalate**:

1. Post in `#aibox-help` with:
   - What the developer was trying to do
   - The error message (screenshot or text)
   - Output of `aibox doctor`
   - Output of `aibox status`
   - Whether the issue is reproducible
2. Platform team SLA: acknowledge within 4 hours, resolve within 1 business day

### When to Escalate to Tier 3 (Infrastructure/Security)

The platform team handles this escalation, not you. But flag these to the platform team urgently:

- Suspected security incident (data exfiltration attempt, container escape)
- Central infrastructure down (Harbor, Nexus, Vault)
- Compliance/audit issues
- Zero-day vulnerability affecting the platform

---

## 5. Tool Pack Request Process

### Requesting an Existing Tool

If a tool pack exists but is not enabled for your project:

1. Check available packs: `aibox mcp list` or `aibox install --list`
2. Install it: `aibox install <pack>@<version>`
3. If blocked by policy, request policy update from your team lead

### Requesting a New Tool Pack

If the tool or runtime you need does not have a pack:

1. **Check if it is already planned**: Ask in `#aibox-champions`
2. **File a request** in `#aibox-help` with:
   - Tool name and version
   - Why your team needs it
   - How many developers would use it
   - Any known network requirements (external endpoints, mirrors)
   - Urgency (can wait for next monthly cycle, or needed sooner)

**Request SLA**:
- Known/registered tools: available within 1 business day
- New tools requiring security review: 3-5 business days
- Emergency requests: same-day with platform team approval

### Building Your Own Tool Pack

If you are comfortable building tool packs, see the "Building Tool Packs" guide. Champions can submit PRs to the `aibox-toolpacks` repository. PRs require:
- Platform team review (manifest correctness, install script quality)
- Security team review (for new network endpoints or elevated permissions)

---

## 6. Structured Feedback Templates

Use these templates when reporting to the platform team. Copy-paste and fill in.

### Bug Report

```
**Summary**: [One-line description]
**Severity**: [Blocker / Major / Minor / Cosmetic]
**Steps to Reproduce**:
1. ...
2. ...
3. ...
**Expected Behavior**: ...
**Actual Behavior**: ...
**Environment**:
- Windows 11 build: [e.g., 22631]
- WSL2 distro: [e.g., Ubuntu 24.04]
- IDE: [VS Code / IntelliJ / other] + version
- Tool packs installed: [e.g., java@21, node@20]
**aibox doctor output**: [paste]
**aibox status output**: [paste]
**Reproducible**: [Always / Sometimes / Once]
**Workaround**: [if any]
```

### Feature Request

```
**Summary**: [One-line description]
**Use Case**: [Who needs this and why]
**Proposed Solution**: [How you think it should work]
**Alternatives Considered**: [Other approaches]
**Impact**: [How many developers / teams affected]
**Priority**: [Nice-to-have / Important / Critical]
```

### Pain Point Report (Weekly)

```
**Week of**: [date]
**Team**: [your team name]
**Top Issues This Week**:
1. [Issue + frequency + severity]
2. [Issue + frequency + severity]
3. [Issue + frequency + severity]
**Positive Feedback**: [What is working well]
**Adoption Status**: [X/Y team members using AI-Box daily]
**Fallback Events**: [Number of times someone fell back to local dev, and why]
```

---

## 7. Champions Communication Channels

| Channel | Purpose | Who |
|---------|---------|-----|
| `#aibox-champions` (Slack) | Champion discussion, early feature access, announcements | Champions + Platform team |
| `#aibox-help` (Slack) | Tier 2 escalation, tool pack requests | Everyone (champions route here) |
| Monthly Champions Sync | 30-min video call, roadmap updates, feedback review | Champions + Platform team |
| Champions email list | Low-frequency announcements (release notes, breaking changes) | Champions only |

### Response Expectations

- Champions are not expected to respond instantly. Best-effort within your working hours.
- Platform team responds in `#aibox-champions` within 2 hours during business hours.
- Monthly sync attendance is expected but not mandatory (recording shared).

---

## 8. Testing New Features

Before each release, the platform team pushes preview builds to champions:

1. You receive a notification in `#aibox-champions` with what changed
2. Update your sandbox: `aibox update --channel preview`
3. Test your normal workflow for 1-2 days
4. Report any regressions using the Bug Report template above
5. If no issues after champion testing period (usually 3 days), the release goes to all developers

Your feedback directly shapes what ships. If you find a problem, it does not ship.

---

## 9. Onboarding Assistance Playbook

During general rollout, you will help your team onboard. Here is the playbook:

### Before Team Onboarding Day

1. Verify your own AI-Box is working and up to date
2. Identify which tool packs your team needs (Java? Node? .NET?)
3. Check that those packs are available and tested
4. Share the appropriate quickstart guide (VS Code or JetBrains) with your team

### On Onboarding Day

1. Be available on Slack for questions
2. Walk through the first `aibox start` with anyone who is stuck
3. Help with IDE connection (most common friction point)
4. Remind teammates: `aibox doctor` is their first stop for issues

### After Onboarding

1. Check in with teammates after 2-3 days
2. Collect any issues and report via the Pain Point template
3. Ensure no one has silently fallen back to local development

---

## 10. Quick Reference Card

```
aibox setup                    # One-time setup
aibox start --workspace ~/proj # Start sandbox
aibox stop                     # Stop sandbox
aibox status                   # Check what is running
aibox doctor                   # Diagnose problems
aibox update                   # Pull latest image
aibox install <pack>@<ver>     # Add a tool pack
aibox shell                    # Open shell in sandbox
aibox network test             # Test connectivity
aibox policy validate          # Check policy
aibox policy explain --log-entry <id>  # Why was I blocked?
aibox mcp enable <pack>        # Enable MCP server
aibox mcp list                 # List MCP packs
aibox push status              # Check pending pushes
aibox repair cache             # Fix cache issues
aibox telemetry show           # What telemetry is collected
aibox telemetry opt-out        # Opt out of optional telemetry
```

---

*Questions about this handbook? Post in `#aibox-champions`. This is a living document updated based on champion feedback.*
