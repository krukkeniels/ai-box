# .NET Project Migration Checklist

**Tool Pack**: `dotnet@8`
**Build System**: MSBuild / `dotnet` CLI
**IDE**: Rider (JetBrains), VS Code with C# extension

---

## Pre-Migration

- [ ] Identify .NET SDK version required (6, 7, or 8).
- [ ] Identify project type: ASP.NET Core, console, library, Blazor, WPF/WinForms (WPF/WinForms is Linux-incompatible).
- [ ] Check for Windows-only dependencies (COM, Windows Registry, WMI, PInvoke to Windows DLLs).
- [ ] Verify all NuGet packages are available through Nexus NuGet proxy.
- [ ] Check for `global.json` specifying SDK version.

## Tool Pack Setup

```bash
aibox install dotnet@8
```

- [ ] `dotnet --version` shows 8.0.x.
- [ ] `dotnet --list-sdks` shows installed SDKs.
- [ ] `dotnet --list-runtimes` shows required runtimes.

## NuGet Configuration

- [ ] Verify `nuget.config` points to Nexus NuGet proxy (not nuget.org directly).
- [ ] If project uses a `nuget.config` in the repo, verify it includes the Nexus source.
- [ ] Test: `dotnet restore` resolves all packages.

## Build Validation

```bash
dotnet build
dotnet publish -c Release
```

- [ ] Build completes without errors.
- [ ] All NuGet packages resolve through Nexus.
- [ ] Build time within 15% of local baseline.
- [ ] No Windows-specific build targets failing.

## Test Validation

```bash
dotnet test
```

- [ ] All tests pass.
- [ ] Test results output correctly.
- [ ] No sandbox-specific test failures (gVisor JIT/GC issues, timing).

## IDE Validation

### Rider (via Gateway)

- [ ] Solution loads and indexes.
- [ ] Code completion, navigation, refactoring work.
- [ ] Debugging works (breakpoints, stepping, variable inspection).
- [ ] NuGet package manager works.

### VS Code

- [ ] C# extension (OmniSharp) starts and provides IntelliSense.
- [ ] Build and test tasks run from terminal.

## Known Issues and Limitations

- **WPF/WinForms projects**: Not supported on Linux. These projects must be developed locally or require a Windows container (out of scope).
- **.NET CLR under gVisor**: The JIT compiler and GC use system calls that gVisor intercepts. Thoroughly test performance-sensitive code paths.
- **Global tools**: `dotnet tool install -g` installs to `~/.dotnet/tools`. Ensure this is on `$PATH`.
- **PowerShell integration**: If the project uses PowerShell build scripts, also install `powershell@7`.
- **Entity Framework migrations**: `dotnet ef` commands should work but test database connectivity if using a local DB.
