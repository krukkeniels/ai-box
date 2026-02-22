# Polyglot / Monorepo Migration Checklist

**Tool Packs**: Multiple (e.g., `java@21` + `node@20` + `dotnet@8`)
**Build Systems**: Bazel, Gradle multi-project, npm workspaces, or mixed
**IDE**: IntelliJ, VS Code
**Special Considerations**: High resource usage, complex dependency resolution

---

## Pre-Migration

- [ ] Inventory all languages/runtimes used in the monorepo.
- [ ] Identify the primary build system (Bazel, Gradle, or mixed).
- [ ] Map each sub-project to its required tool pack.
- [ ] Check total resource requirements (RAM, CPU, disk).
- [ ] Verify machine has >= 32 GB RAM (recommended for polyglot workloads).
- [ ] Identify shared dependencies across sub-projects.

## Tool Pack Setup

```bash
# Install all required packs
aibox install java@21 node@20 dotnet@8 python@3
aibox install bazel@7          # if using Bazel
aibox install powershell@7     # if using PowerShell scripts
```

- [ ] All SDKs available and on `$PATH`.
- [ ] No version conflicts between tool packs.
- [ ] `JAVA_HOME`, `DOTNET_ROOT`, etc., set correctly.

## Resource Configuration

```bash
# Polyglot workloads need more resources
aibox config set resources.memory 16g    # or higher
aibox config set resources.cpus 8
```

- [ ] Sandbox starts without resource allocation errors.
- [ ] `aibox status` shows allocated resources.
- [ ] CLI does not warn about > 80% system resource usage (if it does, reduce or close other sandboxes).

## Build Validation

### Bazel Projects

```bash
bazel build //...
bazel test //...
```

- [ ] Bazel sandbox-within-gVisor works (Spec Section 7.2: Bazel sandboxing compatibility).
- [ ] Bazel remote cache (if configured) is accessible.
- [ ] All Bazel targets build.
- [ ] Bazel daemon does not exhaust memory.

### Multi-Language Builds

- [ ] Each sub-project builds independently.
- [ ] Cross-project dependencies resolve.
- [ ] Full build completes.
- [ ] Build time within 20% of local baseline (higher tolerance for polyglot).

## Dependency Resolution

- [ ] Java/Gradle/Maven dependencies resolve through Nexus.
- [ ] npm packages resolve through Nexus.
- [ ] NuGet packages resolve through Nexus (if .NET sub-projects exist).
- [ ] Python packages resolve through Nexus (if Python sub-projects exist).
- [ ] No dependency manager tries to reach the public internet directly.

## Test Validation

- [ ] Tests for each sub-project pass.
- [ ] Integration tests that span sub-projects pass.
- [ ] Total test suite time is acceptable.

## IDE Validation

- [ ] IntelliJ or VS Code can index the full monorepo.
- [ ] Language servers for all languages start and provide completions.
- [ ] Navigating across language boundaries works (e.g., Java calling a gRPC service defined in a .proto file).

## Known Issues

- **Resource pressure**: Running JVM + .NET CLR + Node.js + Bazel simultaneously consumes significant RAM. Monitor with `aibox status` and increase resources if needed.
- **Bazel + gVisor**: Bazel uses its own sandboxing. Verify `--spawn_strategy` and `--genrule_strategy` work under gVisor. May need `--spawn_strategy=local` if Bazel sandboxing conflicts.
- **Build cache contention**: Multiple build systems writing to disk simultaneously may cause I/O contention. Named volumes help with persistence.
- **IDE memory**: IntelliJ indexing a large monorepo can consume 2-4 GB. Ensure IDE JVM heap is configured (`-Xmx`) appropriately.
- **One sandbox per project**: Per PRODUCT-DECISIONS.md, multi-project developers should use one sandbox per project with `aibox start --project <name>`. For monorepos, use a single sandbox with all tool packs installed.
