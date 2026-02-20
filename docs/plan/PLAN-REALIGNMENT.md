# PLAN-REALIGNMENT: Polyglot Stack Gap Analysis & Required Pivots

**Status**: Proposed
**Date**: 2026-02-19
**Triggered by**: Evaluation of AI-Box spec and plan against a real polyglot monolith stack
**Affects**: PLAN.md, Phases 0, 1, 4; SPEC-FINAL.md deliverable priorities

---

## 1. Purpose & Trigger

### Why This Realignment Exists

The AI-Box specification (SPEC-FINAL.md) makes a clear promise in **Section 3, Goal 3**:

> Stable runtime and policy layer regardless of agent (Claude Code, Codex CLI, Continue, Aider, custom bots). Standard interfaces: terminal, SSH, MCP.

And in **Section 4** (Design Principles):

> **Composable** -- Base runtime + tool packs + MCP packs + policies. Mix and match.

The architecture *supports* tool-agnosticism. The composable layer model -- base image, tool packs, MCP packs, policies -- is sound. But the *implementation plan* made implicit stack assumptions when selecting Phase 0/1/4 deliverables. The initial tool pack list (PHASE-4-DEVELOPER-EXPERIENCE.md, lines 127-135) covers Java, Node, Python, Bazel, Scala, Angular, and AI tooling. The base image variants (SPEC-FINAL.md, lines 1025-1030) are `aibox-java`, `aibox-node`, and `aibox-full`. The resource profiles (Section 18.2) top out at "Monorepo (Bazel/Nx)" at 6-8 cores / 12-16 GB.

This is a plan designed for a typical greenfield Java/Node/Python team on Linux.

### The Trigger

Evaluation against an actual consumer stack exposed the gaps. The stack in question:

| Layer | Technologies |
|-------|-------------|
| **Backend** | .NET (C#), Scala, Java |
| **Build** | Gradle, Bazel, MSBuild/NuGet |
| **Scripting** | PowerShell (Windows-origin scripts) |
| **Frontend** | Angular 18, AngularJS 1.x (legacy) |
| **Dev environment** | Windows 11, custom project CLI |

The question from this team: *"Will this box make it possible to switch to AI agentic tools without too much friction?"*

The honest answer: **not without a pivot.** The spec's architecture can accommodate this stack, but the plan's deliverables don't cover it.

### Root Cause

- The spec *architecturally* supports agnosticism (the tool pack system, composable design)
- The *plan* made implicit stack assumptions when selecting Phase 0/1/4 deliverables
- The Phase 4 tool pack list was chosen based on a typical greenfield stack, not the actual consumer stacks
- No stakeholder with a polyglot/legacy stack reviewed the plan before finalization

---

## 2. Gap Analysis

Spec promise vs. plan reality, evaluated against the polyglot stack:

| # | Gap | Spec Promise | Plan Reality | Impact |
|---|-----|-------------|-------------|--------|
| G1 | **.NET absent** | "Tool-agnostic" (S3, Goal 3); "Easy extension" (S3, Goal 4); NuGet listed in supported registries (S8.6) | Zero .NET coverage -- no tool pack, no image variant, no SDK mention in Phase 4 initial set (PHASE-4, lines 127-135) | Blocks entire .NET part of the monolith. Developers cannot build, test, or run .NET projects inside AI-Box. |
| G2 | **PowerShell on Linux** | Base image is Ubuntu 24.04 LTS (S17.1, line 1021) with bash and zsh (lines 1035-1046) | No PowerShell Core (`pwsh`) in base image or any tool pack. No mention of Windows-origin scripts. | Build and automation scripts that use PowerShell won't run. Team must port scripts or install pwsh manually. |
| G3 | **Custom CLI compatibility** | `aibox` CLI abstracts infrastructure (S5.6); composable design (S4) | No guidance for project-specific CLIs that wrap Windows-native tooling (e.g., `cmd.exe`, `.bat` files, Windows paths). | Custom CLI needs a Linux port or compatibility wrapper. No migration path documented. |
| G4 | **AngularJS (1.x) not covered** | `angular@18` tool pack defined (S15.3, PHASE-4 line 135) | AngularJS 1.x is a different framework with different build tooling (Bower, Grunt/Gulp, possibly older Node). The `angular@18` pack does not cover it. | Legacy frontend builds may fail. Different Node version requirements possible. |
| G5 | **Polyglot resource pressure** | Resource profiles max at "Monorepo" -- 6-8 cores, 12-16 GB RAM, 60 GB disk (S18.2, lines 1094-1102) | Monorepo profile assumes one primary build system. A polyglot monolith runs .NET + JVM + Node + Bazel + AI agent simultaneously. | OOM risk and build slowdowns. A single `dotnet build` + `gradle build` + `ng serve` + AI agent can exceed 16 GB. |
| G6 | **gVisor compatibility surface area** | Risk R1 in PLAN.md (medium likelihood, high impact): "gVisor compatibility breaks developer tooling" (lines 404-405) | Mitigation says "early compatibility testing with target tool stacks" but Phase 1 research only targets Java/Node/Python stacks. | .NET CLR, Bazel sandboxing (sandbox-within-sandbox), Gradle daemons, and PowerShell Core add untested compatibility surface. Risk is higher than assessed. |
| G7 | **Windows-only dev team** | WSL2 supported (S7, lines 319-331); `.wslconfig` recommendations provided | Risk R2 in PLAN.md (medium likelihood, high impact, lines 406-407) treats WSL2 instability as a per-developer edge case. | 100% of developers on this team hit Risk R2. It's not a minority case -- it's the primary deployment path. The plan's mitigation ("dedicate testing effort to Windows path") lacks specifics. |
| G8 | **NuGet package mirror** | "Supported registries: npm, Maven Central, Gradle Plugin Portal, PyPI, **NuGet**, Go modules, Cargo" (S8.6, line 452) | NuGet listed in the spec, and Phase 0 deliverable D2 (PHASE-0, lines 32-41) says "caching proxy for npm, Maven Central, PyPI, NuGet, Go modules, Cargo." But no .NET tool pack or image variant consumes it. | NuGet mirror will be configured but never validated. `dotnet restore` has not been tested through the Nexus proxy. Configuration may be incomplete. |
| G9 | **Tool pack schema coverage** | Composable, manifest-driven (S15.2, lines 922-962); designed for extensibility | Schema exists and is sound, but only 7 packs designed: `java@21`, `node@20`, `python@3.12`, `bazel@7`, `scala@3`, `angular@18`, `ai-tools` | No `dotnet@8` pack, no `powershell` pack, no `angularjs@1` pack. The schema supports them, but no one has written them. |

---

## 3. Required Pivots

### Pivot 1: Add .NET Tool Pack to Phase 4 Initial Set

**Gap addressed**: G1, G8, G9

Create a `dotnet@8` tool pack and associated infrastructure:

- **Tool pack manifest** (`dotnet@8`): .NET SDK 8, NuGet CLI, MSBuild
- **Image variant**: `aibox-dotnet:8-24.04` (following the existing pattern from S17.1 lines 1025-1030)
- **NuGet mirror validation**: Phase 0 D2 already lists NuGet -- add explicit validation that `dotnet restore` succeeds through Nexus
- **Build cache persistence**: Add `~/.nuget/packages` to the cache volume list (following the pattern in the tool pack schema, S15.2: `caches` field)
- **Resource requirements**: Document min/recommended memory for .NET SDK (recommend 4 GB min, 6 GB recommended)

**Estimated tool pack manifest**:

```yaml
name: dotnet
version: "8.0"
description: ".NET SDK 8, NuGet CLI, MSBuild"
maintainer: "platform-team"

install:
  method: "docker-layer"
  base_image: "mcr.microsoft.com/dotnet/sdk:8.0"
  packages: []

network:
  requires:
    - id: "nuget-org"
      hosts: ["nexus.internal"]
      ports: [443]

filesystem:
  creates:
    - "/usr/share/dotnet"
  caches:
    - "$HOME/.nuget/packages"
    - "$HOME/.dotnet/tools"

resources:
  min_memory: "4GB"
  recommended_memory: "6GB"

tags: ["language", "clr", "dotnet"]
```

### Pivot 2: Add PowerShell Core to Base Image or Tool Pack

**Gap addressed**: G2, G3, G9

- **Option A** (recommended): Install `powershell` (`pwsh`) in the base image alongside bash and zsh. Rationale: if the org has significant PowerShell usage, it's a shell, not a language runtime -- it belongs in the base layer.
- **Option B**: Create a `powershell@7` tool pack for teams that need it.
- **Migration guidance**: Document which PowerShell modules are available on Linux and which are Windows-only. Provide a compatibility matrix:

| Pattern | Windows PowerShell | PowerShell Core (Linux) | Migration Path |
|---------|-------------------|------------------------|---------------|
| `Get-ChildItem` | Yes | Yes | No change |
| `Invoke-WebRequest` | Yes | Yes | No change |
| `Get-WmiObject` | Yes | No | Use `Get-CimInstance` |
| `[System.Windows.Forms]` | Yes | No | Replace with CLI alternatives |
| `Start-Process` (GUI) | Yes | No | Use background jobs or `nohup` |
| Registry access (`Get-ItemProperty HKLM:\...`) | Yes | No | Use environment variables or config files |
| COM objects | Yes | No | Rewrite in bash or use .NET APIs |

- **Windows-origin script audit**: Recommend teams audit their PowerShell scripts for Windows-only dependencies before migration.

### Pivot 3: Add AngularJS Support

**Gap addressed**: G4, G9

- **Option A** (recommended): Create an `angularjs@1` tool pack that includes legacy build tooling:
  - Node.js version compatible with the project's AngularJS build (may need Node 16 or 18)
  - Bower (if used for dependency management)
  - Grunt/Gulp (if used as task runners)
  - Karma/Protractor (if used for testing)
- **Option B**: Document how to use `node@20` with legacy build tooling installed manually.
- **Key risk**: AngularJS may require an older Node version than the `node@20` pack provides. The tool pack system should support multiple Node versions side-by-side (e.g., via `nvm` or separate packs `node@16`, `node@20`).

### Pivot 4: Expand gVisor Compatibility Testing Matrix

**Gap addressed**: G6

Add to Phase 1 research (gVisor compatibility, Risk R1 mitigation):

| Component | Why It Needs Testing | Risk Level |
|-----------|---------------------|------------|
| .NET CLR | CLR uses memory-mapped files, thread pool, and JIT compilation patterns that may differ from JVM | High |
| Bazel sandboxing | Bazel creates its own sandboxes -- sandbox-within-gVisor-sandbox is untested | High |
| PowerShell Core | Uses .NET CLR internally; adds another syscall surface | Medium |
| Gradle daemon | Long-lived JVM process with file watchers and IPC | Medium |
| Combined stack | JVM + .NET + Node running simultaneously under gVisor memory pressure | High |

- If any component fails under gVisor, document the `runc` escape hatch with compensating controls (as already noted in R1 mitigation).

### Pivot 5: Increase Windows 11 + WSL2 Testing Priority

**Gap addressed**: G7

- **Elevate from risk register item to explicit Phase 1 deliverable.** Risk R2 (PLAN.md, lines 406-407) currently says "dedicate testing effort to Windows path." This is too vague when 100% of the target team is on Windows.
- **Dedicated WSL2 validation sprint**: Test the full polyglot stack under WSL2 + Podman + gVisor, not just Java/Node.
- **Expanded .wslconfig recommendations**: The spec recommends 16 GB (S7, lines 325-330). For a polyglot monolith with .NET + JVM + Node + Bazel + AI agent, recommend 24 GB.
- **Known issues registry**: Maintain a living document of WSL2-specific issues and workarounds, updated throughout all phases.

### Pivot 6: Add Polyglot Resource Profile

**Gap addressed**: G5

Add a new resource profile to Section 18.2:

| Workload Profile | CPU | RAM | Disk | Notes |
|-----------------|-----|-----|------|-------|
| Frontend (React/Angular) | 2 cores | 4-6 GB | 20 GB | *existing* |
| Backend (Java/Kotlin) | 4 cores | 8-12 GB | 40 GB | *existing* |
| Full-stack + AI agent | 4-6 cores | 10-16 GB | 50 GB | *existing* |
| Monorepo (Bazel/Nx) | 6-8 cores | 12-16 GB | 60 GB | *existing* |
| **Polyglot Monolith** | **8 cores** | **16-24 GB** | **80 GB** | **.NET + JVM + Node + Bazel + AI agent** |

- Update `.wslconfig` recommendation for this profile:

```ini
[wsl2]
memory=24GB
processors=8
swap=8GB
```

- Document combined resource budget: .NET SDK (~4 GB) + JVM (~4 GB) + Node (~2 GB) + Bazel (~4 GB) + AI agent (~2 GB) + OS overhead (~2 GB) = ~18 GB working set.

### Pivot 7: Custom CLI Migration Guidance

**Gap addressed**: G3

Add a documentation section to Phase 4 deliverables: **"Migrating Project-Specific CLIs to AI-Box"**.

Content:

1. **Inventory**: List all commands the custom CLI provides. Classify each as:
   - **Cross-platform** (already works on Linux): no change needed
   - **PowerShell-based**: port to bash/POSIX or ensure `pwsh` compatibility (see Pivot 2)
   - **Windows-native** (calls `cmd.exe`, uses Windows paths, COM, etc.): requires rewrite or wrapper
2. **Wrapper pattern**: For commands that call Windows-native tooling, provide a pattern:
   ```bash
   # wrapper.sh -- translates Windows paths and delegates to Linux equivalents
   case "$1" in
     build-dotnet) dotnet build "${@:2}" ;;
     build-angular) ng build "${@:2}" ;;
     *) echo "Unknown command: $1" >&2; exit 1 ;;
   esac
   ```
3. **Gradual migration**: The custom CLI doesn't need to be fully ported before AI-Box adoption. Critical paths (build, test, serve) should work first; convenience commands can follow.

---

## 4. Impact on Timeline

| Phase | Current Estimate | Adjusted Estimate | Delta | Reason |
|-------|-----------------|-------------------|-------|--------|
| Phase 0 | 3-4 eng-weeks | 4-5 eng-weeks | +1 week | NuGet mirror validation (`dotnet restore` through Nexus), .NET base image variant (`aibox-dotnet:8-24.04`) |
| Phase 1 | 5-6 eng-weeks | 6-8 eng-weeks | +1-2 weeks | Expanded gVisor compatibility matrix (.NET CLR, Bazel sandboxing, PowerShell Core, combined-stack tests), deeper WSL2 validation for full polyglot stack |
| Phase 2 | 5-6 eng-weeks | 5-6 eng-weeks | -- | No change: network security layer is stack-agnostic |
| Phase 3 | 6-7 eng-weeks | 6-7 eng-weeks | -- | No change: policy engine is stack-agnostic |
| Phase 4 | 6-7 eng-weeks | 8-10 eng-weeks | +2-3 weeks | `dotnet@8` tool pack, `powershell@7` tool pack (or base image addition), `angularjs@1` tool pack, polyglot resource profile, custom CLI migration docs |
| Phase 5 | 4-5 eng-weeks | 4-5 eng-weeks | -- | No change: audit/monitoring is stack-agnostic |
| Phase 6 | 8-10 eng-weeks | 9-11 eng-weeks | +1 week | Broader rollout testing with polyglot stack, Windows-specific rollout procedures |
| **Total** | **37-45 eng-weeks** | **42-52 eng-weeks** | **+5-7 weeks** | |

Calendar impact: ~22 weeks becomes ~24-25 weeks (phases overlap; added work is distributed, not sequential).

---

## 5. Recommendation

1. **The pivots are necessary.** If AI-Box is to deliver on its "agnostic" promise (S3, Goal 3), the plan must cover the stacks that actually exist in the org. Without these pivots, the polyglot monolith team cannot adopt AI-Box, which undermines the ~200 developer target.

2. **The incremental cost is modest.** 5-7 additional eng-weeks is 12-18% more effort. The risk of *not* doing it is building a platform that serves only half the organization and fails to deliver on its stated goals.

3. **Conduct a stack audit before Phase 0 begins.** Survey all target teams for their actual technology stacks, build systems, scripting languages, and dev environments. Size the tool pack backlog from real data, not assumptions. This audit should take 1-2 days and can prevent further realignment later.

4. **Prioritize the pivots.** Not all gaps are equal:

| Priority | Pivot | Rationale |
|----------|-------|-----------|
| **P0 -- Blocker** | Pivot 1 (.NET tool pack) | Cannot adopt AI-Box without it |
| **P0 -- Blocker** | Pivot 5 (WSL2 priority) | 100% of devs are on Windows |
| **P1 -- High** | Pivot 4 (gVisor testing) | Unvalidated compatibility = deployment risk |
| **P1 -- High** | Pivot 6 (Resource profile) | OOM on day 1 = failed adoption |
| **P2 -- Medium** | Pivot 2 (PowerShell) | Needed for build scripts; workaround exists (manual install) |
| **P2 -- Medium** | Pivot 7 (Custom CLI) | Documentation only; can be iterative |
| **P3 -- Low** | Pivot 3 (AngularJS) | Legacy; may not need AI agent tooling immediately |

---

## Appendix: Cross-Reference to Source Documents

| Reference | Document | Location |
|-----------|----------|----------|
| S3, Goal 3 | SPEC-FINAL.md | Section 3, lines 106-108 |
| S3, Goal 4 | SPEC-FINAL.md | Section 3, lines 110-112 |
| S4 (Composable) | SPEC-FINAL.md | Section 4, line 138 |
| S7 (WSL2) | SPEC-FINAL.md | Section 7, lines 319-331 |
| S8.6 (Registries) | SPEC-FINAL.md | Section 8.6, lines 440-452 |
| S15.2 (Tool pack schema) | SPEC-FINAL.md | Section 15, lines 922-962 |
| S15.3 (Initial packs) | SPEC-FINAL.md | Section 15, lines 964-974 |
| S17.1 (Base image) | SPEC-FINAL.md | Section 17.1, lines 1021-1046 |
| S18.2 (Resource profiles) | SPEC-FINAL.md | Section 18.2, lines 1094-1102 |
| R1 (gVisor risk) | PLAN.md | Lines 404-405 |
| R2 (WSL2 risk) | PLAN.md | Lines 406-407 |
| Phase timeline | PLAN.md | Lines 419-428 |
| D2 (Nexus mirrors) | PHASE-0-INFRASTRUCTURE.md | Lines 32-41 |
| Phase 4 tool packs | PHASE-4-DEVELOPER-EXPERIENCE.md | Lines 127-135 |
