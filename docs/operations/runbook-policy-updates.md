# Runbook: Policy Update Process

**Owner**: Platform Engineering + Security Team
**Frequency**: As needed (policy changes via Git PR)
**Last Updated**: 2026-02-21

---

## 1. Policy Hierarchy

AI-Box uses a three-tier policy hierarchy (see SPEC-FINAL.md Section 12):

| Tier | Scope | Who Can Modify | Review Required |
|------|-------|---------------|----------------|
| Org Baseline | All sandboxes | Security team only | Security team lead approval |
| Team Policy | Team sandboxes | Team lead | Team lead approval (tighten only) |
| Project Policy | Single project | Developer | Team lead approval (tighten only) |

**Key constraint**: Team and project policies can only **tighten** the org baseline, never loosen it.

---

## 2. Org Baseline Policy Updates

### When to Update
- New security requirements from compliance.
- Response to security incidents or threat intelligence.
- Adding new allowed domains (e.g., new package registry).
- Adjusting resource limits or rate limits.

### Process
1. **Create PR** to `aibox-policies` repository:
   ```bash
   git checkout -b policy/add-nuget-mirror
   # Edit aibox-policies/org/org-baseline.yaml
   git commit -m "policy(org): allow nuget.internal mirror endpoint"
   git push -u origin policy/add-nuget-mirror
   ```

2. **Required reviewers**: At least 1 security team member + 1 platform engineer.

3. **Automated checks** (CI):
   - OPA syntax validation (`opa check`).
   - Policy unit tests (`opa test`).
   - Regression test: verify org baseline still passes 34 existing tests.
   - Tighten-only validation: confirm no existing restrictions are loosened.

4. **Security team lead approval**: Required for merge.

5. **Merge and propagation**:
   - PR merged to `main` branch.
   - Policy artifacts signed with Cosign.
   - Pushed to Harbor as OCI artifact.
   - Developers receive updated policy on next `aibox start`.

### Communication
For significant policy changes, post to `#aibox-announce`:
```
POLICY UPDATE: [brief description of change]
Effective: Next sandbox start after [date]
Impact: [what developers will notice, if anything]
Questions: Contact #aibox-help
```

---

## 3. Team Policy Updates

### When to Update
- Team needs access to additional network endpoints.
- Team needs additional tool packs enabled by default.
- Team wants stricter controls for sensitive projects.

### Process
1. Team lead or champion creates PR to `aibox-policies/teams/<team-name>/`.
2. **Required reviewers**: Team lead approval sufficient.
3. **Automated checks**:
   - OPA syntax validation.
   - Tighten-only check: team policy must not loosen org baseline.
   - Policy unit tests pass.
4. Team lead merges.
5. Changes propagate on next `aibox start` for team members.

### Tighten-Only Validation
The CI pipeline runs the following check:
```bash
# Verify team policy does not loosen org baseline
opa eval -d aibox-policies/org/ -d aibox-policies/teams/<team>/ \
  'data.aibox.tighten_only_check' --fail-defined
```

If the team policy attempts to loosen any org baseline rule, the CI check fails with:
```
ERROR: Team policy 'teams/alpha/policy.yaml' loosens org baseline rule 'network.default_deny'.
Team policies can only tighten the org baseline. Contact the security team to request
org-level changes.
```

---

## 4. Project Policy Updates

### Process
1. Developer adds or modifies `/aibox/policy.yaml` in the project repository.
2. Standard code review process (project's existing PR workflow).
3. `aibox start` validates the project policy against team and org policies.
4. If the project policy loosens team or org rules, `aibox start` rejects it:
   ```
   ERROR: Project policy loosens team restriction 'network.allowed_domains'.
   Project policies can only add restrictions, not remove them.
   ```

---

## 5. Emergency Policy Changes

For security incidents requiring immediate policy changes:

1. Security team lead approves the change verbally or via secure channel.
2. Platform engineer makes the change and pushes directly (bypassing PR review).
3. Post-incident: retroactive PR created for documentation and audit trail.
4. All active sandboxes receive the emergency policy on next start.

**Note**: There is no mechanism to force-update policies in running sandboxes. Emergency policies take effect when developers restart. For critical situations, platform team can notify developers to restart.

---

## 6. Policy Rollback

If a policy update causes unintended developer impact:

1. Revert the PR in `aibox-policies` repository.
2. Push reverted policy to Harbor.
3. Notify developers to restart sandboxes.
4. Post-mortem: understand why the policy change had unintended impact.

---

## 7. Audit Trail

All policy changes are tracked through:
- Git history in `aibox-policies` repository.
- PR reviews with approver records.
- Cosign signatures on policy artifacts.
- `aibox` CLI logs which policy version was loaded at startup.

---

## 8. Metrics

| Metric | Target |
|--------|--------|
| Org policy PRs merged without security review | 0 |
| Team policy tighten-only violations caught by CI | Track (informational) |
| Mean time from policy PR to propagation | < 24 hours |
| Policy-related developer support tickets | < 2/week |
