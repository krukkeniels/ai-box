# Contributing

## Development setup

**Requirements:**

| Tool | Version | Required |
|------|---------|----------|
| Go | 1.24+ | Yes |
| Podman | 4.x+ (rootless) | Yes |
| gVisor (runsc) | Any recent release | Recommended |
| golangci-lint | Latest | For linting |
| OPA CLI | 1.x+ | For policy tests |

**Build and test:**

```bash
git clone https://github.com/krukkeniels/ai-box.git
cd ai-box

make build    # Compiles all 4 binaries to bin/
make test     # Runs unit tests across all modules
make lint     # Runs golangci-lint on all modules
make fmt      # Formats all Go code
make install  # Installs to ~/.local/bin
```

Individual binaries:

```bash
make build-aibox
make build-credential-helper
make build-llm-proxy
make build-git-remote-helper
```

---

## Repository structure

```
ai-box/
  cmd/
    aibox/                          Main CLI binary
      cmd/                          Cobra subcommands (start, stop, shell, doctor, ...)
      internal/                     Internal packages (see below)
      configs/                      Security profiles and config templates
        seccomp.json                Syscall allowlist
        squid.conf                  Proxy config
        Corefile                    DNS config
        nftables.conf               Firewall rules
        falco.yaml                  Runtime detection config
        falco_rules.yaml            eBPF detection rules
        templates/                  Config file templates
      tests/                        Integration and exit criteria tests
    aibox-credential-helper/        Git credential helper
    aibox-llm-proxy/                LLM API proxy sidecar
    aibox-git-remote-helper/        Non-blocking git push approval

  aibox-policies/                   Policy-as-code
    org/                            Org baseline (immutable, 34 Rego tests)
    teams/                          Team-level overrides
    project/                        Project-level overrides

  aibox-images/                     Container images
    base/                           Base Containerfile (Ubuntu 24.04)
    java/, node/, dotnet/, full/    Variant images

  aibox-toolpacks/                  Tool and MCP pack definitions
    packs/                          12 packs (java, node, python, ...)

  docs/                             Documentation and phase plans
  infra/                            Infrastructure configs (Vault, SPIRE)
```

### Internal packages (`cmd/aibox/internal/`)

| Package | Purpose |
|---------|---------|
| `audit` | Event schema (25 types), hash chain, file logger |
| `config` | Config loading, validation, defaults |
| `container` | Container lifecycle (start, stop, shell, status) |
| `credentials` | Credential broker, Vault, SPIFFE, keychain |
| `dashboards` | Grafana dashboard and alert rule generation |
| `doctor` | System health checks |
| `dotfiles` | Developer dotfiles sync into sandbox |
| `falco` | Falco eBPF rule management |
| `feedback` | User feedback collection |
| `host` | Host detection (Linux, WSL2) |
| `logging` | Structured logging setup |
| `mcppacks` | MCP pack manifest, config generator, policy validator |
| `mounts` | Filesystem mount layout |
| `network` | nftables, Squid, CoreDNS management |
| `operations` | Day-2 operations runbooks |
| `policy` | OPA engine, merge, toolgate, decision logging |
| `recording` | AES-256-GCM encrypted session recording |
| `runtime` | Container runtime abstraction |
| `security` | Seccomp, AppArmor, arg validation |
| `setup` | Host setup automation, SSH key generation |
| `siem` | SIEM detection rules, Vector sinks, MITRE mapping |
| `storage` | Immutable append-only log storage |
| `toolpacks` | Tool pack manifest, registry, installer |
| `vector` | Vector log collection pipeline |

---

## Adding a CLI command

1. Create `cmd/aibox/cmd/mycommand.go`:

```go
package cmd

import (
    "fmt"
    "github.com/spf13/cobra"
)

var mycommandCmd = &cobra.Command{
    Use:   "mycommand",
    Short: "One-line description",
    Long:  `Longer description of what this command does.`,
    RunE:  runMycommand,
}

func init() {
    mycommandCmd.Flags().String("flag", "", "flag description")
    rootCmd.AddCommand(mycommandCmd)
}

func runMycommand(cmd *cobra.Command, args []string) error {
    // Cfg (global *config.Config) is available from root.go
    fmt.Println("done")
    return nil
}
```

2. The `init()` function registers the command automatically -- no other wiring needed.

3. Business logic goes in `internal/` packages. The `cmd/` file should be thin: parse flags, call internal logic, format output.

Read `cmd/aibox/cmd/status.go` or `cmd/aibox/cmd/install.go` for real examples of this pattern.

---

## Creating a tool pack

Tool packs live in `aibox-toolpacks/packs/<name>/` and need two files:

**`manifest.yaml`** -- declares metadata, dependencies, network requirements, and verification:

```yaml
name: mytool
version: "1.0"
description: "Short description"
maintainer: platform-team
tags:
  - language

install:
  method: script
  script: install.sh

network:
  requires:
    - id: registry-name
      hosts: ["nexus.internal"]
      ports: [443]

filesystem:
  creates:
    - "/opt/toolpacks/mytool"
  caches:
    - "$HOME/.mytool/cache"

resources:
  min_memory: "2GB"
  recommended_memory: "4GB"

security:
  signed_by: platform-team@aibox.internal

environment:
  MYTOOL_HOME: "/opt/toolpacks/mytool"

verify:
  - command: "mytool --version"
    expect_exit_code: 0
```

**`install.sh`** -- runs inside the container to install the tool:

```bash
#!/bin/bash
set -euo pipefail
# Install logic here (apt, curl, extract, etc.)
# Target directory: /opt/toolpacks/<name>/
```

Test with:

```bash
aibox start --workspace ~/test-project
aibox install mytool@1.0 --dry-run   # Preview what would be installed
aibox install mytool@1.0             # Actually install
```

See `aibox-toolpacks/packs/java/` for a complete example.

---

## Creating an MCP pack

MCP packs also live in `aibox-toolpacks/packs/<name>/` but only need a `manifest.yaml`:

```yaml
name: my-mcp
version: "1.0.0"
description: "What this MCP server does"
command: aibox-mcp-myserver
args:
  - "--root"
  - "/workspace"
filesystem_requires:
  - /workspace
permissions:
  - read
  - write
```

Key fields:

- `command` -- the MCP server binary (must exist in the container image or be installable)
- `permissions` -- what the MCP server can do (`read`, `write`, `execute`)
- `filesystem_requires` -- directories the server needs access to

Enable with `aibox mcp enable my-mcp`. See `aibox-toolpacks/packs/filesystem-mcp/` for a real example.

---

## Testing

```bash
# Unit tests (all modules)
make test

# Single package
cd cmd/aibox && go test ./internal/audit/...

# Integration tests (needs Podman + seccomp installed)
cd cmd/aibox && go test -tags=integration ./tests/... -v

# OPA policy tests
opa test aibox-policies/org/ -v

# Run with coverage
cd cmd/aibox && go test -cover ./internal/...
```

**Test stats:** 635+ tests across 60 files, plus 34 OPA policy tests.

Every internal package has a `*_test.go` file. Follow the existing test patterns -- table-driven tests with `t.Run()` subtests.

---

## PR process

### Branch naming

```
<type>/<short-description>

feat/add-ruby-toolpack
fix/wsl2-memory-detection
docs/update-installation-guide
```

### Commit convention

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): subject

feat(toolpacks): add Ruby tool pack
fix(network): handle Squid timeout on WSL2
docs(readme): update quick start section
test(audit): add hash chain verification tests
refactor(config): extract validation to separate file
chore(deps): update OPA to v1.14.0
```

Types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`, `ci`

### Review checklist

Before submitting a PR:

- [ ] `make build` succeeds
- [ ] `make test` passes
- [ ] `make lint` has no new warnings
- [ ] New code has tests
- [ ] No secrets or credentials in committed files
- [ ] Commit messages follow `type(scope): subject` format
