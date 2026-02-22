# WSL2 Polyglot Stack Validation Report

**Owner**: Platform Engineering Team
**Last Updated**: 2026-02-21
**Target Environment**: Windows 11 23H2+ with WSL2 (100% of developer fleet)
**Status**: Validation Plan (to be executed during Pilot, Weeks 15-18)

---

## 1. Executive Summary

All ~200 target developers run Windows 11 with WSL2. AI-Box runs Podman containers with gVisor isolation inside WSL2. This document defines the validation plan for the full polyglot stack (.NET, JVM, Node.js, Bazel, PowerShell, AI agents) under this environment, including known issues, workarounds, and resource recommendations.

---

## 2. Environment Specification

### 2.1 Host Machine Requirements

| Component | Minimum | Recommended (Polyglot) |
|-----------|---------|----------------------|
| OS | Windows 11 23H2 | Windows 11 24H2 |
| RAM | 16 GB | 32 GB |
| CPU | 8 cores | 8+ cores |
| Storage | 256 GB SSD | 512 GB NVMe SSD |
| WSL2 kernel | 5.15+ | Latest (6.x) |

### 2.2 Recommended `.wslconfig`

For polyglot workloads, the following `.wslconfig` is recommended (placed at `%USERPROFILE%\.wslconfig`):

```ini
[wsl2]
memory=24GB
swap=8GB
processors=8
localhostForwarding=true
nestedVirtualization=true

[experimental]
autoMemoryReclaim=dropcache
sparseVhd=true
```

**Rationale**:
- **24 GB memory**: Polyglot workloads (.NET + JVM + Node + Bazel + AI agent) can peak at 18-22 GB concurrent usage. 24 GB provides headroom.
- **8 GB swap**: Prevents OOM kills during memory spikes (e.g., Gradle daemon + Bazel analysis phase).
- **8 processors**: Matches the host core count. Limiting to fewer cores causes build slowdowns.
- **autoMemoryReclaim=dropcache**: Reclaims page cache memory when WSL2 is idle. Critical for long-running sessions.
- **sparseVhd=true**: Allows WSL2 disk to shrink, preventing unbounded VHD growth.

### 2.3 Resource Budget per Stack

| Stack | CPU (peak) | Memory (steady) | Memory (peak) | Disk |
|-------|-----------|-----------------|---------------|------|
| .NET SDK 8 | 2-4 cores | 1-2 GB | 3-4 GB | 5 GB |
| JVM (Gradle daemon) | 2-4 cores | 2-4 GB | 4-6 GB | 8 GB |
| JVM (Maven) | 2-4 cores | 1-2 GB | 3-4 GB | 5 GB |
| Node.js (npm/yarn) | 1-2 cores | 0.5-1 GB | 2-3 GB | 3 GB |
| Bazel | 4-6 cores | 2-4 GB | 6-8 GB | 10 GB |
| PowerShell Core | 0.5 core | 0.2 GB | 0.5 GB | 0.5 GB |
| AI Agent (Claude Code) | 0.5-1 core | 0.3-0.5 GB | 1 GB | 0.5 GB |
| **Combined (all stacks)** | **8 cores** | **8-14 GB** | **18-22 GB** | **32 GB** |

---

## 3. Validation Test Cases

### TC-01: .NET SDK 8

**ID**: TC-01
**Stack**: .NET SDK 8 (`dotnet@8` tool pack)
**Prerequisites**: Sandbox running with `--toolpacks dotnet@8`, Nexus NuGet proxy configured.

| Step | Action | Expected Result | Pass Criteria |
|------|--------|----------------|---------------|
| 1 | `dotnet --version` | Prints `8.0.x` | Version is 8.0+ |
| 2 | `dotnet new console -n TestApp` | Project created | Exit code 0 |
| 3 | `dotnet restore` | Dependencies restored via Nexus NuGet proxy | Exit code 0, packages from nexus.internal |
| 4 | `dotnet build` | Build succeeds | Exit code 0 |
| 5 | `dotnet test` (on test project) | Tests pass | Exit code 0, test results shown |
| 6 | `dotnet publish -c Release` | Publish succeeds | Exit code 0, output in bin/Release |
| 7 | Run published binary | Application executes | Output matches expected |

**gVisor Compatibility Notes**:
- .NET CLR requires `/proc/self/exe` readlink support. gVisor supports this since 2023.
- .NET garbage collector uses `madvise` syscalls that gVisor supports in compatibility mode.
- `ReadyToRun` (R2R) pre-compiled assemblies work under gVisor without issues.
- **Known issue**: `dotnet watch` file watcher may have increased latency under gVisor's virtual filesystem. Workaround: use `--no-hot-reload` if watch mode is slow.

### TC-02: JVM (Gradle)

**ID**: TC-02
**Stack**: Java 21 (`java@21` tool pack), Gradle build system
**Prerequisites**: Sandbox with `--toolpacks java@21`, Nexus Maven proxy configured.

| Step | Action | Expected Result | Pass Criteria |
|------|--------|----------------|---------------|
| 1 | `java --version` | Prints Java 21 | Version is 21+ |
| 2 | `gradle --version` | Prints Gradle version | Gradle available |
| 3 | `gradle build` on test project | Build succeeds | Exit code 0 |
| 4 | `gradle test` | Tests pass | Exit code 0 |
| 5 | Stop sandbox, restart, `gradle build` | Daemon reuses cache | Build is incremental (faster) |
| 6 | Run 3 consecutive builds | Gradle daemon stable | No daemon crashes |

**gVisor Compatibility Notes**:
- JVM works well under gVisor. `futex` and `epoll` syscalls fully supported.
- Gradle daemon persists across sessions via named volume. Daemon PID file must handle stale PIDs after sandbox restart.
- **Known issue**: JVM `perf` agent and JFR (Java Flight Recorder) may not work under gVisor due to missing `perf_event_open` syscall. This affects profiling only, not normal builds.
- **Recommendation**: Set `org.gradle.jvmargs=-Xmx4g` in `gradle.properties` to cap heap usage.

### TC-03: JVM (Maven)

**ID**: TC-03
**Stack**: Java 21, Maven
**Prerequisites**: Sandbox with `--toolpacks java@21`, Nexus Maven proxy configured.

| Step | Action | Expected Result | Pass Criteria |
|------|--------|----------------|---------------|
| 1 | `mvn --version` | Prints Maven version | Maven available |
| 2 | `mvn compile` on test project | Compilation succeeds | Exit code 0, deps from nexus.internal |
| 3 | `mvn test` | Tests pass | Exit code 0 |
| 4 | `mvn package` | JAR created | Exit code 0, JAR in target/ |
| 5 | Second `mvn compile` (cached) | Uses local repo cache | Faster than first run |

**gVisor Compatibility Notes**: Same as TC-02. Maven does not use a persistent daemon, so stale PID issues do not apply.

### TC-04: Node.js (npm)

**ID**: TC-04
**Stack**: Node.js 22 (`node@22` tool pack)
**Prerequisites**: Sandbox with `--toolpacks node@22`, Nexus npm proxy configured.

| Step | Action | Expected Result | Pass Criteria |
|------|--------|----------------|---------------|
| 1 | `node --version` | Prints v22.x | Version is 22+ |
| 2 | `npm install` on test project | Dependencies installed via Nexus | Exit code 0 |
| 3 | `npm run build` | Build succeeds | Exit code 0 |
| 4 | `npm test` | Tests pass | Exit code 0 |
| 5 | `npm run dev` (dev server) | Server starts, responds on port | HTTP 200 from dev server |
| 6 | IDE port forwarding | VS Code forwards port | Browser access to dev server |

**gVisor Compatibility Notes**:
- Node.js (V8) works under gVisor without issues.
- `inotify` for file watching is supported by gVisor.
- **Known issue**: `node-gyp` native module compilation may fail if it requires syscalls not in seccomp allow list. Most common npm packages with native modules (e.g., `bcrypt`, `sharp`) have pre-built binaries that work fine.

### TC-05: Node.js (yarn)

**ID**: TC-05
**Stack**: Node.js 22, Yarn
**Prerequisites**: Same as TC-04, yarn installed.

| Step | Action | Expected Result | Pass Criteria |
|------|--------|----------------|---------------|
| 1 | `yarn --version` | Prints yarn version | Yarn available |
| 2 | `yarn install` | Dependencies installed | Exit code 0 |
| 3 | `yarn build` | Build succeeds | Exit code 0 |
| 4 | `yarn test` | Tests pass | Exit code 0 |

### TC-06: Angular (latest)

**ID**: TC-06
**Stack**: Node.js 22 + Angular CLI (`angular@19` tool pack)
**Prerequisites**: Sandbox with `--toolpacks node@22,angular@19`.

| Step | Action | Expected Result | Pass Criteria |
|------|--------|----------------|---------------|
| 1 | `ng version` | Prints Angular CLI version | Angular CLI available |
| 2 | `ng build` | Production build succeeds | Exit code 0 |
| 3 | `ng test --watch=false` | Karma tests pass | Exit code 0 |
| 4 | `ng serve` | Dev server starts | HTTP 200 on port 4200 |

### TC-07: AngularJS 1.x (Legacy)

**ID**: TC-07
**Stack**: Node.js + AngularJS 1.x (`angularjs@1` tool pack)
**Prerequisites**: Sandbox with `--toolpacks node@22,angularjs@1`.

| Step | Action | Expected Result | Pass Criteria |
|------|--------|----------------|---------------|
| 1 | `grunt --version` | Prints Grunt version | Grunt available |
| 2 | `bower install` | Dependencies installed | Exit code 0 |
| 3 | `grunt build` | Build succeeds | Exit code 0 |
| 4 | `karma start --single-run` | Tests pass | Exit code 0 |

**Notes**: AngularJS 1.x is legacy. Grunt and Bower are end-of-life but required for existing projects. PhantomJS (if used for tests) does not work under gVisor; projects should migrate to headless Chrome.

### TC-08: Bazel

**ID**: TC-08
**Stack**: Bazel 7 (`bazel@7` tool pack)
**Prerequisites**: Sandbox with `--toolpacks bazel@7`.

| Step | Action | Expected Result | Pass Criteria |
|------|--------|----------------|---------------|
| 1 | `bazel --version` | Prints Bazel 7.x | Version is 7+ |
| 2 | `bazel build //...` on test project | Build succeeds | Exit code 0 |
| 3 | `bazel test //...` | Tests pass | Exit code 0 |
| 4 | `bazel build //...` (cached) | Rebuild is instant | 0 actions executed |

**gVisor Compatibility Notes**:
- **Critical**: Bazel uses its own sandboxing mechanism (`linux-sandbox`) which attempts to create mount namespaces and user namespaces. This is a **sandbox-within-gVisor** scenario.
- gVisor supports `clone(CLONE_NEWNS)` but may not support all mount operations Bazel's sandbox expects.
- **Required workaround**: Set `--spawn_strategy=local` in `.bazelrc` to disable Bazel's sandboxing when running inside gVisor:
  ```
  # .bazelrc for AI-Box
  build --spawn_strategy=local
  test --spawn_strategy=local
  ```
- Alternative: `--sandbox_writable_path=/` as a less disruptive option.
- **Impact**: Disabling Bazel sandboxing removes Bazel's hermeticity guarantee. This is acceptable because the AI-Box container itself provides the isolation boundary. Builds are still reproducible; they just rely on container isolation instead of Bazel-level sandboxing.
- Remote cache (`--remote_cache=grpc://bazel-cache.internal`) works under gVisor without issues.

### TC-09: PowerShell Core

**ID**: TC-09
**Stack**: PowerShell 7 (`powershell@7` tool pack)
**Prerequisites**: Sandbox with `--toolpacks powershell@7`.

| Step | Action | Expected Result | Pass Criteria |
|------|--------|----------------|---------------|
| 1 | `pwsh --version` | Prints PowerShell 7.x | Version is 7+ |
| 2 | `pwsh -File test-script.ps1` | Script executes | Exit code 0, correct output |
| 3 | `Install-Module Pester -Scope CurrentUser` | Module installs from PSGallery via Nexus | Exit code 0 |
| 4 | `Invoke-Pester ./tests/` | Pester tests pass | All tests pass |
| 5 | `pwsh` interactive session | Interactive mode works | Prompt appears, commands execute |

**gVisor Compatibility Notes**:
- PowerShell Core (pwsh) runs on .NET runtime; same CLR compatibility notes as TC-01 apply.
- **Known issue**: PSReadLine module (for tab completion and syntax highlighting) may have reduced functionality under gVisor's terminal emulation. Workaround: set `$env:TERM = "xterm-256color"` in `$PROFILE`.
- PowerShell remoting (`Enter-PSSession`, `Invoke-Command`) is not supported inside the sandbox (by design -- no outbound connections except allowlisted).
- Module installation from PSGallery requires Nexus proxy configuration for `https://www.powershellgallery.com/`.

### TC-10: AI Agent (Concurrent)

**ID**: TC-10
**Stack**: AI tools (`ai-tools` tool pack) + any build stack
**Prerequisites**: Sandbox with `--toolpacks java@21,ai-tools`, LLM API credentials injected via Vault.

| Step | Action | Expected Result | Pass Criteria |
|------|--------|----------------|---------------|
| 1 | `claude --version` | Prints Claude Code version | CLI available |
| 2 | Start Claude Code in terminal | AI agent starts, connects to LLM API | "Connected" status |
| 3 | While Claude Code is running, `gradle build` | Build completes | Exit code 0, no interference |
| 4 | While Claude Code is running, `gradle test` | Tests pass | Exit code 0 |
| 5 | Claude Code performs file edits during build | Both operations complete | No file corruption |
| 6 | Check LLM API latency | Proxy overhead measured | < 50ms added (SLA) |

**gVisor Compatibility Notes**:
- AI agents are primarily I/O-bound (network to LLM API, disk for file edits). No gVisor-specific issues.
- LLM API traffic goes through `aibox-llm-proxy` sidecar, which handles authentication and payload logging.
- Concurrent AI agent + build is the most common real-world usage pattern. Must validate no resource starvation.

### TC-11: Combined Resource Pressure

**ID**: TC-11
**Stack**: Full polyglot (`java@21,node@22,dotnet@8,bazel@7,powershell@7,ai-tools`)
**Prerequisites**: Sandbox with all tool packs. 32 GB RAM machine with 24 GB allocated to WSL2.

| Step | Action | Expected Result | Pass Criteria |
|------|--------|----------------|---------------|
| 1 | Start sandbox with all tool packs | Sandbox starts | Startup < 90s |
| 2 | `gradle build` + `npm install` concurrently | Both complete | Exit code 0 for both |
| 3 | `dotnet build` + `bazel build` concurrently | Both complete | Exit code 0 for both |
| 4 | All 4 builds + Claude Code concurrently | All complete | No OOM kills, all exit 0 |
| 5 | Monitor memory during step 4 | Peak memory recorded | < 24 GB (within WSL2 allocation) |
| 6 | Monitor CPU during step 4 | CPU utilization recorded | All cores utilized, no starvation |
| 7 | Run for 4 hours with mixed workload | No degradation | No daemon crashes, no memory leaks |

**Pass/Fail Criteria**:
- No OOM kills (check `dmesg` for oom-killer events).
- No gVisor crashes or panics (check `runsc` logs).
- Build times within 30% of single-stack builds (some overhead expected).
- AI agent remains responsive during heavy builds.

---

## 4. Known Issues and Workarounds

### 4.1 gVisor Compatibility

| Issue | Affected Stack | Workaround | Status |
|-------|---------------|------------|--------|
| Bazel sandbox-within-gVisor | Bazel | `--spawn_strategy=local` in `.bazelrc` | Permanent workaround |
| JFR/perf_event_open not supported | JVM profiling | Use async-profiler with `--event cpu` instead | gVisor limitation |
| PhantomJS unsupported | AngularJS (legacy tests) | Migrate to headless Chrome/Chromium | PhantomJS EOL |
| `dotnet watch` latency | .NET | Use `--no-hot-reload` flag | gVisor inotify delay |
| PSReadLine reduced features | PowerShell | Set `$env:TERM = "xterm-256color"` | Terminal emulation |

### 4.2 WSL2-Specific Issues

| Issue | Description | Workaround |
|-------|-------------|------------|
| VHD growth | WSL2 VHD grows but doesn't shrink automatically | Enable `sparseVhd=true` in `.wslconfig` |
| Memory not reclaimed | WSL2 holds page cache, appears to "leak" memory | Enable `autoMemoryReclaim=dropcache` |
| DNS resolution after VPN connect | WSL2 DNS can break when host connects to VPN | Add `generateResolvConf=false` and set static DNS if needed |
| Clock drift after sleep | WSL2 clock can drift after host sleep/hibernate | Run `sudo hwclock -s` or restart WSL2 |
| Slow first file access | Initial file access after WSL2 start can be slow | Wait 10-15 seconds after WSL2 start before `aibox start` |
| `localhost` port forwarding | Port forwarding from WSL2 to Windows may fail | Use `localhostForwarding=true` in `.wslconfig` |

### 4.3 Resource Contention

| Issue | Description | Mitigation |
|-------|-------------|-----------|
| OOM kills under polyglot load | Multiple JVM + .NET + Bazel can exceed memory | Set per-process heap limits; ensure 24 GB allocated |
| Gradle daemon memory leak | Long-running Gradle daemon accumulates memory | Restart daemon daily: `gradle --stop` |
| Bazel analysis phase memory spike | Bazel analysis can briefly use 6-8 GB | Ensure swap is configured (8 GB recommended) |
| Disk I/O bottleneck | Concurrent npm install + Maven download saturates I/O | Use SSD (NVMe preferred); stagger large operations |

---

## 5. Fallback to runc with Compensating Controls

For edge cases where gVisor compatibility prevents a specific workflow, AI-Box supports fallback to standard `runc` (OCI runtime) with compensating security controls:

### When to Use runc Fallback
- Workflow requires syscalls not supported by gVisor.
- Performance degradation under gVisor exceeds 30% threshold.
- Specific tool has a hard incompatibility (e.g., hardware-level profiling).

### Compensating Controls (Required)

When using `runc` instead of `gVisor`, the following additional controls must be active:

| Control | Purpose | Implementation |
|---------|---------|----------------|
| Enhanced seccomp profile | Restrict syscalls that gVisor would intercept | Tighter seccomp.json with additional blocked syscalls |
| Falco monitoring (mandatory) | Runtime syscall monitoring | Falco rules for container escape patterns |
| Reduced capability set | Drop more Linux capabilities | `--cap-drop=ALL --cap-add=...` (minimal set) |
| Read-only rootfs | Prevent rootfs modification | `--read-only` flag with tmpfs for /tmp |
| No new privileges | Prevent privilege escalation | `--security-opt=no-new-privileges` |
| Network policy unchanged | Egress controls still host-level | Squid + nftables unchanged |

### Requesting runc Fallback
```bash
# Team lead requests via policy (must be approved by security team)
# In team policy:
runtime:
  engine: "runc"
  compensating_controls: true
  justification: "Bazel requires clone(CLONE_NEWUSER) for hermetic builds"
```

This requires security team approval as an org baseline exception.

---

## 6. Recommendations

### 6.1 Hardware
- **Strong recommendation**: 32 GB RAM for developers working on polyglot projects.
- Hardware audit should start during pilot (Week 15). Platform team identifies below-spec machines via `aibox doctor`.
- Machines below 16 GB get a soft warning; no hard block (per PO decision).
- IT/Procurement should provision upgrades before general rollout (Week 22).

### 6.2 `.wslconfig` Distribution
- Include recommended `.wslconfig` in `aibox setup` for Windows machines.
- `aibox doctor` checks `.wslconfig` and warns if memory allocation is below 16 GB.
- For polyglot developers, `aibox doctor` recommends 24 GB allocation.

### 6.3 Per-Stack Heap Limits
To prevent any single stack from consuming all available memory:

```bash
# JVM (in gradle.properties or MAVEN_OPTS)
org.gradle.jvmargs=-Xmx4g
MAVEN_OPTS="-Xmx2g"

# .NET
DOTNET_GCHeapHardLimit=0x100000000  # 4 GB

# Bazel (in .bazelrc)
startup --host_jvm_args=-Xmx4g

# Node.js
NODE_OPTIONS="--max-old-space-size=4096"
```

These limits are set as defaults in tool pack manifests and can be overridden per project.

### 6.4 Monitoring Under WSL2
- `aibox status` should report WSL2 memory usage and warn at 80% of allocation.
- Dashboard panel shows per-developer resource utilization.
- Alert if a developer's sandbox consistently uses > 90% of allocated memory.

---

## 7. Validation Schedule

| Week | Activity | Deliverable |
|------|----------|-------------|
| 15 | Execute TC-01 through TC-09 (individual stacks) | Per-stack pass/fail |
| 16 | Execute TC-10 (AI agent concurrent) and TC-11 (combined pressure) | Resource consumption report |
| 17 | Address any failures, retest | Updated known issues |
| 18 | Final validation report, sign-off | This document updated with results |

Results will be recorded in the "Pass Criteria" column of each test case table above.
