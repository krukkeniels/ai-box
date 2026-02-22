# Configuration

AI-Box stores its configuration in a single YAML file at `~/.config/aibox/config.yaml`. The file is created automatically by `aibox setup` or manually with `aibox config init`.

See the main [README](../README.md) for installation and quickstart.

---

## Getting started

Generate a config file from one of the built-in templates:

```bash
# Open-source / personal use (default)
aibox config init --template minimal

# Local development with gVisor and network security
aibox config init --template dev

# Full enterprise stack (Harbor, Vault, Falco, audit)
aibox config init --template enterprise
```

To overwrite an existing config:

```bash
aibox config init --template dev --force
```

---

## Templates

Three templates ship with AI-Box. Pick the one closest to your environment and customize from there.

| Setting | `minimal` | `dev` | `enterprise` |
|---|---|---|---|
| gVisor | off | on | on |
| Network proxy | off | on (localhost) | on (Squid + CoreDNS) |
| Registry | ghcr.io | ghcr.io | harbor.internal |
| Signature verification | off | off | on |
| Credentials | fallback | fallback | vault |
| Audit | off | basic (local) | full (MinIO, Falco, recording) |
| Logging format | text | text | json |
| Allowed domains | none | none | harbor, nexus, foundry, git, vault |

---

## All config keys

Every key in `config.yaml` is listed below with its type, default, and description.

### Top-level

| Key | Type | Default | Description |
|---|---|---|---|
| `config_version` | int | `1` | Config schema version. Do not edit manually; use `aibox config migrate`. |
| `runtime` | string | `podman` | Container runtime. `podman` or `docker`. |
| `image` | string | `ghcr.io/krukkeniels/aibox/base:24.04` | Base container image. |
| `shell` | string | `bash` | Default shell inside the container. `bash`, `zsh`, or `pwsh`. |

### gvisor

| Key | Type | Default | Description |
|---|---|---|---|
| `gvisor.enabled` | bool | `false` | Enable gVisor sandbox for the container. |
| `gvisor.platform` | string | `systrap` | gVisor platform backend. `systrap` or `ptrace`. |
| `gvisor.require_apparmor` | bool | `false` | When true, AppArmor failure is fatal instead of a warning. |

### resources

| Key | Type | Default | Description |
|---|---|---|---|
| `resources.cpus` | int | `4` | CPU core limit for the container. Must be > 0. |
| `resources.memory` | string | `8g` | Memory limit (e.g. `8g`, `16384m`). |
| `resources.tmp_size` | string | `2g` | Size of the in-container `/tmp` tmpfs. |

### workspace

| Key | Type | Default | Description |
|---|---|---|---|
| `workspace.default_path` | string | `.` | Default host path to mount as the workspace. |
| `workspace.validate_fs` | bool | `true` | Validate the workspace filesystem before mounting. |

### registry

| Key | Type | Default | Description |
|---|---|---|---|
| `registry.url` | string | `ghcr.io/krukkeniels/aibox` | Container image registry URL. |
| `registry.verify_signatures` | bool | `false` | Require cosign signature verification on pull. |

### network

| Key | Type | Default | Description |
|---|---|---|---|
| `network.enabled` | bool | `false` | Enable the network security stack (Squid + nftables + CoreDNS). |
| `network.proxy_addr` | string | `127.0.0.1` | Squid proxy listen address. |
| `network.proxy_port` | int | `3128` | Squid proxy listen port (1-65535). |
| `network.dns_addr` | string | `127.0.0.1` | CoreDNS listen address. |
| `network.dns_port` | int | `53` | CoreDNS listen port (1-65535). |
| `network.allowed_domains` | list | `[]` | Domains allowed through the proxy. |
| `network.llm_gateway` | string | `""` | LLM API gateway hostname (e.g. `foundry.internal`). |

### policy

| Key | Type | Default | Description |
|---|---|---|---|
| `policy.org_baseline_path` | string | `/etc/aibox/org-policy.yaml` | Path to the organization-level OPA policy. |
| `policy.team_policy_path` | string | `""` | Path to team-level policy (optional). |
| `policy.project_policy_path` | string | `aibox/policy.yaml` | Project-level policy, relative to the workspace root. |
| `policy.decision_log_path` | string | `/var/log/aibox/decisions.jsonl` | Path for the OPA decision log. |
| `policy.hot_reload_secs` | int | `0` | Policy reload interval in seconds. `0` disables hot reload. |

### credentials

| Key | Type | Default | Description |
|---|---|---|---|
| `credentials.mode` | string | `fallback` | Credential provider. `fallback` (host credential passthrough) or `vault`. |
| `credentials.vault_addr` | string | `""` | HashiCorp Vault server address (required when mode is `vault`). |
| `credentials.spiffe_trust_domain` | string | `""` | SPIFFE trust domain for workload identity. |
| `credentials.spiffe_socket_path` | string | `""` | Path to the SPIRE agent socket. |

### logging

| Key | Type | Default | Description |
|---|---|---|---|
| `logging.format` | string | `text` | Log output format. `text` or `json`. |
| `logging.level` | string | `info` | Log level. `debug`, `info`, `warn`, or `error`. |

### ide

| Key | Type | Default | Description |
|---|---|---|---|
| `ide.ssh_port` | int | `2222` | Host port mapped to the container's SSH server (1-65535). |

### dotfiles

| Key | Type | Default | Description |
|---|---|---|---|
| `dotfiles.repo` | string | `""` | Git URL for a dotfiles repository to sync into the container. |

### audit

| Key | Type | Default | Description |
|---|---|---|---|
| `audit.enabled` | bool | `false` | Enable the audit logging subsystem. |
| `audit.storage_backend` | string | `local` | Audit log storage backend. `local`, `minio`, or `s3`. |
| `audit.storage_endpoint` | string | `""` | Object storage endpoint URL (for `minio` or `s3`). |
| `audit.storage_bucket` | string | `aibox-audit` | Object storage bucket name. |
| `audit.log_path` | string | `/var/log/aibox/audit.jsonl` | Local audit log file path. |
| `audit.vector_config_path` | string | `/etc/aibox/vector.toml` | Path to the Vector log pipeline config. |
| `audit.falco_enabled` | bool | `false` | Enable Falco eBPF runtime monitoring. |
| `audit.recording_enabled` | bool | `false` | Enable encrypted session recording. |
| `audit.recording_policy` | string | `disabled` | Session recording policy. `required`, `optional`, or `disabled`. |
| `audit.recording_notice_text` | string | *(see below)* | Notice shown to users when recording is active. |
| `audit.retention_tier1` | string | `730d` | Retention for lifecycle, policy, credential, and tool events. |
| `audit.retention_tier2` | string | `365d` | Retention for network, DNS, LLM, and file events. |
| `audit.required_for_rollout` | bool | `false` | Require audit to pass before allowing rollout (for classified environments). |
| `audit.classification_level` | string | `standard` | Environment classification. `standard` or `classified`. |
| `audit.llm_logging_mode` | string | `hash` | LLM prompt/response logging. `full`, `hash`, or `metadata_only`. |
| `audit.runtime_backend` | string | `none` | Runtime security backend. `falco`, `auditd`, or `none`. |

> Default `recording_notice_text`: "This session is being recorded for security and compliance purposes."

---

## Environment variable overrides

Every config key can be overridden with an environment variable. The rule is simple:

1. Add the `AIBOX_` prefix
2. Replace dots with underscores
3. Uppercase everything

Environment variables take precedence over the YAML file.

```bash
# Examples:
AIBOX_RUNTIME=docker aibox start
AIBOX_RESOURCES_MEMORY=16g aibox start
AIBOX_GVISOR_ENABLED=true aibox start
AIBOX_CREDENTIALS_MODE=vault aibox start
```

---

## Validation

Run `aibox config validate` to check your config for errors and warnings.

```bash
$ aibox config validate
Configuration is valid.
```

If there are problems the output shows each issue with the field path, message, and current value:

```
ERROR  resources.cpus: must be greater than 0 (got "0")
WARN   credentials.vault_addr: should be set when credentials.mode is "vault"
```

Validation is also run automatically every time the config is loaded. Errors prevent AI-Box from starting; warnings are logged but do not block startup.

### Validation rules

**Errors** (block startup):

| Field | Rule |
|---|---|
| `runtime` | Must be `podman` or `docker` |
| `shell` | Must be `bash`, `zsh`, or `pwsh` |
| `resources.cpus` | Must be > 0 |
| `resources.memory` | Must be a valid size (e.g. `8g`, `16384m`) |
| `resources.tmp_size` | Must be a valid size if set |
| `logging.format` | Must be `text` or `json` |
| `logging.level` | Must be `debug`, `info`, `warn`, or `error` |
| `credentials.mode` | Must be `fallback` or `vault` |
| `ide.ssh_port` | Must be 1-65535 |
| `network.proxy_port` | Must be 1-65535 |
| `network.dns_port` | Must be 1-65535 |
| `audit.storage_backend` | Must be `local`, `minio`, or `s3` |
| `audit.recording_policy` | Must be `required`, `optional`, or `disabled` |
| `audit.llm_logging_mode` | Must be `full`, `hash`, or `metadata_only` |
| `audit.runtime_backend` | Must be `falco`, `auditd`, or `none` |

**Warnings** (logged, do not block startup):

| Field | Rule |
|---|---|
| `gvisor.platform` | Should be `systrap` or `ptrace` |
| `credentials.vault_addr` | Should be set when `credentials.mode` is `vault` |

---

## Config migration

AI-Box uses a `config_version` field to track the config schema. When the schema changes in a new release, the migration system upgrades your file automatically.

### How it works

1. Reads the current `config_version` from your config file.
2. Creates a backup at `~/.config/aibox/config.yaml.backup.v<N>`.
3. Applies each migration step sequentially until the config reaches the latest version.
4. Validates the migrated config before writing.

### Commands

Preview what would change without writing:

```bash
aibox config migrate --dry-run
```

Apply the migration:

```bash
aibox config migrate
```

If your config is already at the latest version:

```
Config is already at version 1 (current).
```

---

## Commands

### `aibox config init`

Create a config file from a template.

```bash
aibox config init                       # uses "minimal" template
aibox config init --template dev        # local dev with security
aibox config init --template enterprise # full stack
aibox config init --force               # overwrite existing file
```

### `aibox config set`

Update a single config key.

```bash
aibox config set shell zsh
aibox config set resources.memory 16g
aibox config set dotfiles.repo https://github.com/user/dotfiles.git
aibox config set ide.ssh_port 2223
aibox config set runtime docker
aibox config set image ghcr.io/krukkeniels/aibox/full:24.04
aibox config set resources.cpus 8
```

Supported keys for `set`: `dotfiles.repo`, `ide.ssh_port`, `image`, `resources.cpus`, `resources.memory`, `runtime`, `shell`.

### `aibox config get`

Read a single config key.

```bash
aibox config get shell        # bash
aibox config get resources.cpus  # 4
```

### `aibox config validate`

Check the current config for errors and warnings.

```bash
aibox config validate
```

### `aibox config migrate`

Upgrade the config schema to the latest version.

```bash
aibox config migrate            # apply migration
aibox config migrate --dry-run  # preview only
```

---

## Examples

Generate a complete config from any template:

```bash
aibox config init --template minimal      # open-source / personal use
aibox config init --template enterprise   # full security stack
```

The key differences between minimal and enterprise:

```yaml
# Minimal: no security stack, public registry
gvisor:
  enabled: false
network:
  enabled: false
registry:
  url: ghcr.io/krukkeniels/aibox
audit:
  enabled: false

# Enterprise: full security stack, private registry
gvisor:
  enabled: true
network:
  enabled: true
  allowed_domains: [harbor.internal, nexus.internal, foundry.internal, git.internal, vault.internal]
registry:
  url: harbor.internal
  verify_signatures: true
credentials:
  mode: vault
  vault_addr: "https://vault.internal:8200"
audit:
  enabled: true
  falco_enabled: true
  recording_enabled: true
  classification_level: classified
```

See the full template files in `cmd/aibox/configs/templates/`.
