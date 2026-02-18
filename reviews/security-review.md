# AI-Box Security Architecture Review

**Reviewer**: Security Architect
**Date**: 2026-02-18
**Spec version reviewed**: `spec.md` (v1, undated)
**Classification context**: Sensitive/classified development environment, ~200 developers, Windows 11 + Linux, IntelliJ + VS Code, agentic AI tools (Claude Code, Codex CLI, etc.)

---

## Executive Summary

The AI-Box spec establishes a sound foundational posture: default-deny networking, read-only base filesystems, policy-gated tooling, and credential isolation. However, for deployment in classified/sensitive environments the spec has significant gaps in its threat model, lacks depth on enforcement mechanisms, and underspecifies several critical controls. This review identifies those gaps and provides concrete, opinionated recommendations for closing them.

**Overall assessment**: Good starting framework. Not yet deployable for classified workloads without the additions outlined below.

---

## 1. Threat Model Gaps

The current threat model (Section 5) covers prompt injection, malicious dependencies, misconfigured allowlists, and IDE plugins running outside the sandbox. The following threats are **missing or underspecified**:

### 1.1 Supply Chain Attacks via Package Managers

**Gap**: The spec mentions "malicious dependencies or tool packs" but provides no mitigation strategy beyond the existence of allowlisted network egress.

**Missed threats**:
- `npm install` runs arbitrary `postinstall` scripts that can exfiltrate data via allowed endpoints (e.g., encoding source in HTTP headers to an allowed artifact repo).
- `pip install` executes `setup.py` with full filesystem access to `/workspace`.
- Maven/Gradle plugins can execute arbitrary code during builds.
- Typosquatting and dependency confusion attacks.

**Recommendation**:
- All package installations MUST go through a vetted internal mirror (Nexus/Artifactory) that scans and caches packages.
- Disable install-time script execution where possible (`npm install --ignore-scripts`, `pip install --no-build-isolation` with pre-built wheels).
- Maintain a curated allowlist of approved packages for sensitive projects.
- Run package installations in a further-restricted sub-sandbox with no network access (download first, install offline).

### 1.2 DNS Tunneling and DNS-over-HTTPS (DoH)

**Gap**: Section 6.1 says "DNS except via controlled resolver" but does not address:
- DNS tunneling (encoding data in subdomain queries: `<base64-encoded-source>.attacker-controlled.allowlisted-domain.com`).
- DNS-over-HTTPS (DoH) bypassing the controlled resolver entirely by using HTTPS to known DoH providers (Cloudflare 1.1.1.1, Google 8.8.8.8) which may share IP ranges with allowlisted services.
- DNS-over-TLS (DoT) on port 853.

**Recommendation**:
- Force all DNS through a local stub resolver (CoreDNS or systemd-resolved) configured to resolve ONLY allowlisted domains.
- Return NXDOMAIN for all non-allowlisted queries.
- Block outbound ports 53 (DNS), 853 (DoT) at the host level.
- Block known DoH provider IPs at the network level.
- Monitor DNS query patterns for tunneling signatures (high subdomain entropy, excessive query volume).

### 1.3 Clipboard and Paste-Based Exfiltration

**Gap**: Not mentioned in the spec. In hybrid mode (Section 7.1 -- IDE on host, tools in sandbox), the clipboard is a cross-boundary channel.

**Threat**: A developer (or compromised IDE extension on the host) could copy sensitive code from the sandbox terminal/IDE and paste it into an uncontrolled application (browser, messaging app, external IDE).

**Recommendation**:
- For highest-sensitivity environments: disable clipboard sharing between sandbox and host entirely.
- For moderate sensitivity: implement clipboard size limits and log clipboard transfers.
- For IDE remote connections (VS Code Remote, JetBrains Gateway): configure the remote connection to disable clipboard sync or gate it through a DLP inspection layer.
- Document this as a residual risk if clipboard is allowed.

### 1.4 Covert Channels

**Gap**: Not addressed in the spec.

**Threats**:
- **Timing channels**: Encoding data in the timing of allowed API requests (e.g., spacing between LLM API calls encodes bits).
- **Storage channels**: Encoding data in git commit metadata (author names, timestamps, commit message patterns), file timestamps, file permission bits.
- **Steganographic exfiltration via LLM API**: The LLM API channel legitimately carries source code. An attacker (or a compromised agent) could encode additional sensitive data within prompt payloads -- hidden instructions, base64-encoded files in "context" fields, etc. This is the hardest channel to close because the LLM needs to see code to function.
- **Build artifact exfiltration**: If build outputs leave the sandbox (pushed to artifact repo), they could contain embedded source code or secrets in binary padding, metadata, or debug symbols.

**Recommendation**:
- Accept that covert channels cannot be fully eliminated; focus on raising the cost and lowering the bandwidth.
- Rate-limit all allowed outbound connections (especially LLM API).
- Enforce payload size limits on LLM API requests.
- Log and retain all LLM API request/response payloads for audit (with appropriate access controls on the logs themselves).
- Strip unnecessary metadata from git commits before push (normalize timestamps, enforce standard author format).
- Scan build artifacts for embedded strings/secrets before allowing export from sandbox.
- Document covert channels as residual risk with detection-focused controls.

### 1.5 Container Escape Vectors

**Gap**: Section 5 puts "kernel-level escape vulnerabilities" out of scope with only "mitigated via updates + defense-in-depth." This is insufficient for classified environments.

**Missed specifics**:
- `runc` CVEs (e.g., CVE-2024-21626 Leaky Vessels, CVE-2019-5736) allow container escape.
- `/proc/self/exe` exploitation.
- Abuse of mounted volumes to escape container boundaries.
- Exploiting shared kernel via syscalls not covered by seccomp.

**Recommendation**:
- Use gVisor (runsc) as the container runtime to intercept syscalls in userspace, eliminating most kernel attack surface. This is the single highest-impact security control for container isolation.
- For highest-classification workloads, use Kata Containers (VM-level isolation).
- Do NOT rely solely on standard `runc` with seccomp/AppArmor.
- Maintain a 24-hour SLA for patching container runtime CVEs.
- See Section 3 (Container Isolation) for detailed evaluation.

### 1.6 IDE Plugin/Extension Data Leakage

**Gap**: The spec mentions "IDE plugins running outside the sandbox unintentionally" but does not address plugins **inside** the sandbox leaking data through legitimate channels.

**Threats**:
- VS Code extensions send telemetry to Microsoft and extension authors (including workspace file lists, open file names, error snippets).
- IntelliJ plugins report usage statistics, crash dumps (which may include code snippets), and license checks.
- Extensions that auto-update may phone home with workspace metadata.
- Language servers (TypeScript, Java LSP) may send anonymous usage data.

**Recommendation**:
- Disable all telemetry in IDE configuration by default (`"telemetry.telemetryLevel": "off"` for VS Code, disable in IntelliJ settings).
- Use a curated, pre-approved set of extensions. Block extension marketplace access from within the sandbox.
- Pre-install approved extensions in the container image. Prevent runtime extension installation.
- If extension marketplace access is needed, proxy it through the egress proxy and restrict to approved extension IDs.
- Block known telemetry endpoints at the proxy level (e.g., `dc.services.visualstudio.com`, `vortex.data.microsoft.com`).

---

## 2. Network Security Deep-Dive

### 2.1 Is Allowlist-Only Egress Sufficient?

**No. Necessary but not sufficient.** Allowlist-only controls WHERE traffic goes, but not WHAT is sent. For classified environments, you need both destination control and content inspection.

**Layered approach required**:

| Layer | Control | Purpose |
|-------|---------|---------|
| L3/L4 | eBPF (Cilium) or nftables on **host** | Block all traffic except to allowlisted IPs/ports |
| L7 (HTTP/S) | Forward proxy (Envoy or Squid) | Inspect HTTP headers, enforce URL paths, size limits |
| DNS | Restricted resolver (CoreDNS) | Only resolve allowlisted domains |
| Application | Sidecar proxy for LLM API | Inject auth, log payloads, enforce rate limits |

### 2.2 DNS Resolution Control

**Current spec**: "DNS except via controlled resolver" -- too vague.

**Required implementation**:

```
Container DNS config:
  nameserver: 127.0.0.1 (local CoreDNS instance or host-level resolver)
  options: ndots:0 edns0

CoreDNS config:
  - Respond to allowlisted domains only (forward to upstream)
  - Return NXDOMAIN for everything else
  - Log ALL queries (domain, source, timestamp)
  - Rate-limit queries per source (detect tunneling)
  - Block TXT/NULL/CNAME record types unless specifically needed
    (these are most commonly abused for tunneling)

Host-level enforcement:
  - Block outbound UDP/TCP 53 from container network (except to local resolver)
  - Block outbound TCP 853 (DoT)
  - Block known DoH provider IPs
```

### 2.3 Proxy Technology Recommendations

**Recommendation: Envoy as the primary L7 proxy, with Cilium for L3/L4 enforcement.**

| Technology | Strengths | Weaknesses | Verdict |
|-----------|-----------|------------|---------|
| **Squid** | Mature, well-understood, good HTTPS CONNECT handling | Limited L7 inspection without TLS interception, aging codebase | Acceptable for simple allowlisting |
| **Envoy** | Modern, extensible via WASM/Lua filters, gRPC-native, excellent observability, active development | More complex to configure | **Recommended** for L7 proxy |
| **Custom eBPF (Cilium)** | Kernel-level enforcement, no bypass possible from container, excellent performance | L3/L4 only (no content inspection), requires Linux 5.10+ | **Recommended** for L3/L4 enforcement |
| **iptables/nftables** | Universal, well-understood | Can be complex at scale, no L7 awareness | Acceptable fallback if Cilium is not feasible |

**Why Envoy over Squid**:
- Native support for gRPC (relevant for many LLM APIs).
- WASM filter support allows custom content inspection without forking.
- Built-in rate limiting, circuit breaking, and observability (Prometheus metrics, distributed tracing).
- Active CNCF project with strong security track record.
- Can enforce per-route policies (different rules for LLM traffic vs. artifact repo traffic).

### 2.4 TLS Inspection Considerations

**For classified environments, TLS inspection is often prohibited or heavily restricted.** The spec correctly notes this.

**Recommended approach -- avoid full TLS interception. Instead:**

1. **SNI-based filtering**: Inspect the TLS ClientHello SNI field (unencrypted) to enforce destination allowlists. No decryption needed. Works with Envoy and Cilium.
2. **Explicit forward proxy (HTTP CONNECT)**: Configure all clients to use the proxy explicitly. The proxy sees the target hostname in the CONNECT request. No certificate manipulation needed.
3. **Mutual TLS for internal services**: Use mTLS between sandbox and internal endpoints (Nexus, Foundry, git server) for authentication without content inspection.
4. **LLM API sidecar**: For LLM traffic specifically, use a localhost sidecar that terminates the connection, inspects/logs the plaintext payload, then re-encrypts to the upstream. This avoids org-wide CA deployment and limits inspection scope.

**If TLS inspection IS required** (e.g., regulatory mandate):
- Deploy a dedicated inspection CA, scoped only to AI-Box traffic.
- Use Envoy with SDS (Secret Discovery Service) for dynamic certificate management.
- Log that inspection occurred (for compliance evidence).
- Never inspect traffic to credential endpoints (Vault, identity providers).

### 2.5 Package Manager Proxy/Mirror Strategy

**Recommendation: Sonatype Nexus Repository Manager or JFrog Artifactory as pull-through cache.**

```
Architecture:
  Container --> Envoy proxy --> Nexus/Artifactory --> Upstream registries
                                     |
                               [vulnerability scan]
                               [license check]
                               [policy check]

Supported registries (via Nexus/Artifactory):
  - npm (npmjs.org)
  - Maven Central / Gradle Plugin Portal
  - PyPI
  - Docker Hub / container registries
  - NuGet
  - Go modules proxy
  - Cargo (crates.io)
```

**Key policies**:
- Block direct access to upstream registries from the sandbox. Only the mirror can reach them.
- Enable vulnerability scanning on ingestion (Nexus IQ, Xray, or Grype).
- Optionally: curated/approved package lists for highest-sensitivity projects.
- Cache aggressively to reduce external calls and enable air-gapped operation.
- Log all package downloads (name, version, requesting user/sandbox).

### 2.6 LLM API Traffic Handling

**This is the most challenging security control.** The LLM API channel MUST carry source code to function. This creates an inherent tension between security and utility.

**Recommended architecture**:

```
Agent (in sandbox)
  |
  | HTTP to localhost:8443
  v
LLM Sidecar Proxy (in sandbox, separate process)
  |
  | - Injects API key (agent never sees it)
  | - Logs full request/response payloads
  | - Enforces rate limits (requests/min, tokens/min)
  | - Enforces payload size limits
  | - Strips unnecessary context from requests
  | - Adds sandbox identity headers for audit correlation
  |
  | HTTPS to Foundry/LLM gateway
  v
Envoy Egress Proxy (on host)
  |
  | - Validates destination is allowlisted LLM endpoint
  | - Enforces TLS
  | - Secondary rate limiting
  |
  v
LLM API Endpoint (Foundry gateway)
```

**Mitigations for prompt/code smuggling**:
- Log ALL LLM API payloads to immutable audit store. This is the primary control -- detection over prevention, since prevention would break functionality.
- Enforce maximum context window size limits at the sidecar.
- Rate-limit to prevent bulk exfiltration (e.g., max 60 requests/min, max 100K tokens/min).
- Alert on anomalous patterns: sudden spikes in request size, unusual request patterns, requests that look like they contain base64-encoded data in unusual fields.
- For highest-sensitivity environments: consider an LLM gateway (like a Foundry instance) that runs inside the security boundary, so code never leaves the enclave.

---

## 3. Container Isolation Evaluation

### 3.1 Runtime Comparison

| Runtime | Isolation Level | Performance Overhead | Compatibility | Security Posture | Recommendation |
|---------|----------------|---------------------|---------------|------------------|----------------|
| **runc** (standard) | Namespace + cgroup + seccomp | Minimal (~1-2%) | Excellent | Shared kernel, large syscall surface | **Not recommended** as sole isolation |
| **gVisor (runsc)** | User-space kernel (Sentry) | Moderate (10-30% for syscall-heavy, ~5% for I/O) | Good (most dev workflows work) | Dramatically reduced kernel attack surface | **Recommended default** |
| **Kata Containers** | Full VM (QEMU/Cloud Hypervisor) | Higher (VM boot time, memory overhead ~128-256MB/sandbox) | Good | Strongest -- separate kernel per sandbox | **Recommended for highest classification** |

**Recommendation**: **gVisor as the default runtime, Kata Containers as an option for the highest-sensitivity workloads.**

gVisor rationale:
- Intercepts ~400 syscalls in userspace. Even if an attacker achieves code execution, they attack gVisor's Sentry (a Go application in a constrained sandbox) rather than the host kernel.
- Compatible with standard OCI images -- no changes to tool packs needed.
- Supported by Podman and containerd.
- Performance overhead is acceptable for development workloads (the bottleneck is typically I/O and network, not syscalls).
- IntelliJ and VS Code remote development work correctly under gVisor.

### 3.2 Seccomp Profile

The spec does not specify a seccomp profile. The default Docker/Podman seccomp profile blocks ~44 syscalls but allows ~300+. For AI-Box, use a stricter custom profile.

**Recommended blocklist** (in addition to default Docker seccomp denials):

```json
{
  "defaultAction": "SCMP_ACT_ERRNO",
  "defaultErrnoRet": 1,
  "comment": "AI-Box: allowlist-based seccomp profile",
  "syscalls": [
    {
      "comment": "Essential syscalls for dev tooling",
      "names": [
        "read", "write", "open", "openat", "close", "stat", "fstat",
        "lstat", "poll", "lseek", "mmap", "mprotect", "munmap", "brk",
        "ioctl", "access", "pipe", "select", "sched_yield", "dup",
        "dup2", "dup3", "nanosleep", "getpid", "getuid", "getgid",
        "geteuid", "getegid", "getppid", "getgroups", "sigaction",
        "sigprocmask", "sigreturn", "clone", "execve", "wait4",
        "kill", "fcntl", "flock", "fsync", "fdatasync", "truncate",
        "ftruncate", "getdents", "getdents64", "getcwd", "chdir",
        "rename", "renameat", "renameat2", "mkdir", "rmdir", "link",
        "unlink", "symlink", "readlink", "chmod", "chown", "umask",
        "gettimeofday", "getrlimit", "getrusage", "times",
        "connect", "accept", "sendto", "recvfrom", "sendmsg",
        "recvmsg", "bind", "listen", "getsockname", "getpeername",
        "socket", "socketpair", "setsockopt", "getsockopt",
        "shutdown", "pipe2", "epoll_create", "epoll_create1",
        "epoll_ctl", "epoll_wait", "epoll_pwait",
        "clock_gettime", "clock_getres", "futex",
        "set_tid_address", "set_robust_list", "get_robust_list",
        "pread64", "pwrite64", "readv", "writev",
        "arch_prctl", "prctl", "getrandom",
        "memfd_create", "statx", "io_uring_setup",
        "io_uring_enter", "io_uring_register",
        "newfstatat", "prlimit64", "eventfd2",
        "timerfd_create", "timerfd_settime", "timerfd_gettime",
        "signalfd4", "inotify_init1", "inotify_add_watch",
        "inotify_rm_watch", "faccessat", "faccessat2"
      ],
      "action": "SCMP_ACT_ALLOW"
    }
  ]
}
```

**Critical syscalls to DENY** (beyond defaults):
- `ptrace` -- prevents debugging/tracing other processes (container escape vector)
- `mount`, `umount2` -- prevents filesystem manipulation
- `pivot_root`, `chroot` -- prevents filesystem escape
- `kexec_load`, `kexec_file_load` -- prevents kernel replacement
- `bpf` -- prevents eBPF program loading (could bypass network controls)
- `userfaultfd` -- commonly used in kernel exploits
- `keyctl`, `request_key` -- prevents kernel keyring access
- `add_key` -- prevents kernel keyring manipulation
- `unshare` -- prevents creating new namespaces
- `setns` -- prevents joining other namespaces
- `reboot` -- obvious
- `swapon`, `swapoff` -- prevents swap manipulation
- `init_module`, `finit_module`, `delete_module` -- prevents kernel module loading
- `acct` -- prevents process accounting manipulation
- `settimeofday`, `adjtimex` -- prevents time manipulation

### 3.3 AppArmor Profile

```
#include <tunables/global>

profile aibox-sandbox flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>

  # Deny all networking except what seccomp/eBPF allows
  # (AppArmor as defense-in-depth, not primary network control)
  network inet stream,
  network inet dgram,
  network inet6 stream,
  network inet6 dgram,
  deny network raw,
  deny network packet,

  # Filesystem: read-only root, writable workspace
  / r,
  /** r,
  /workspace/** rw,
  /tmp/** rw,
  /var/tmp/** rw,
  /dev/null rw,
  /dev/zero r,
  /dev/random r,
  /dev/urandom r,
  /dev/pts/* rw,
  /dev/shm/** rw,
  /proc/self/** r,
  /proc/sys/kernel/random/uuid r,
  deny /proc/*/mem rw,
  deny /proc/kcore r,
  deny /proc/sysrq-trigger rw,
  deny /sys/** w,

  # Deny access to sensitive paths
  deny /home/** rw,
  deny /root/** rw,
  deny /**/.ssh/** rw,
  deny /**/docker.sock rw,

  # Allow execution of approved binaries
  /usr/bin/* ix,
  /usr/local/bin/* ix,
  /workspace/** ix,

  # Deny mount operations
  deny mount,
  deny umount,
  deny pivot_root,

  # Deny ptrace
  deny ptrace,

  # Deny capability escalation
  deny capability sys_admin,
  deny capability sys_ptrace,
  deny capability sys_module,
  deny capability sys_rawio,
  deny capability net_admin,
  deny capability net_raw,
  deny capability sys_boot,
  deny capability sys_chroot,
}
```

### 3.4 Rootless Containers

**Mandatory.** The spec should require rootless container execution.

**Implementation**:
- Use **Podman in rootless mode** (no daemon, no root). Podman runs entirely in user namespace.
- Set `"no-new-privileges": true` in container security options.
- Use `user` directive in Containerfile to run as non-root (UID 1000).
- Disable `setuid`/`setgid` bits on all binaries in the image (`RUN find / -perm /6000 -type f -exec chmod a-s {} \;`).
- Map container UID to unprivileged host UID via `/etc/subuid` and `/etc/subgid`.

### 3.5 Capability Dropping

**Drop ALL capabilities.** Development workloads should not need any.

```bash
podman run \
  --cap-drop=ALL \
  --security-opt=no-new-privileges:true \
  --security-opt=seccomp=/etc/aibox/seccomp.json \
  --security-opt=apparmor=aibox-sandbox \
  --runtime=runsc \
  ...
```

If specific tool packs require capabilities (extremely rare for dev work), document and explicitly add back only what's needed, with justification.

### 3.6 /proc and /sys Filtering

- **gVisor**: Provides its own `/proc` implementation that exposes only safe, sandboxed information. This is one of its key advantages.
- **For runc fallback**: Use `--security-opt=proc-opts=subset` (Podman) or Docker's default masking which masks `/proc/kcore`, `/proc/keys`, `/proc/timer_list`, etc. Additionally mount `/proc` and `/sys` as read-only.
- Block `/proc/self/mem`, `/proc/*/environ`, `/proc/sysrq-trigger`.
- Mount `/sys` as read-only and with limited visibility.

---

## 4. Credential Management

### 4.1 Git Authentication

**Recommendation: HTTPS tokens via credential helper, NOT SSH keys.**

| Method | Pros | Cons | Verdict |
|--------|------|------|---------|
| SSH keys | Familiar to devs | Hard to scope (repo-level access difficult), long-lived, key management complexity | **Not recommended** |
| HTTPS personal access tokens | Scoped (repo, read/write), time-limited, revocable | Stored in plaintext if not managed | **Recommended** with credential helper |
| HTTPS via credential helper + Vault | Scoped, short-lived, auto-rotating, never touches disk | Requires Vault infrastructure | **Strongly recommended** |

**Implementation**:
```bash
# In container, configure git to use a credential helper that fetches from Vault
git config --global credential.helper '/usr/local/bin/aibox-credential-helper'

# The helper script:
# 1. Requests a short-lived token from Vault (1-4 hour TTL)
# 2. Returns it in git-credential format
# 3. Token is scoped to specific repos (read+write to assigned repo only)
# 4. Token auto-expires; helper fetches a new one when needed
```

### 4.2 LLM API Key Injection

**The agent must NEVER see the API key.** If the agent is compromised via prompt injection, it should not be able to exfiltrate the key.

**Recommended architecture: Sidecar proxy**:

```
Agent process (in container)
  |
  | HTTP request to localhost:8443 (NO auth header)
  v
aibox-llm-proxy (sidecar, separate process in same pod/container)
  |
  | - Reads API key from mounted Vault secret (or env var from entrypoint)
  | - Injects Authorization header
  | - Logs request metadata
  | - Enforces rate limits
  | - Strips any auth headers the agent might try to add
  |
  | HTTPS to LLM endpoint
  v
```

**Why sidecar over env var**:
- Env vars are visible via `/proc/self/environ` (even with /proc filtering, defense-in-depth says minimize exposure).
- The sidecar can enforce additional controls (rate limiting, logging) that an env var cannot.
- If the agent is compromised, it can read env vars but cannot read the sidecar's memory.

### 4.3 Secret Management Stack

**Recommendation: HashiCorp Vault for secrets, SPIFFE/SPIRE for workload identity.**

```
Architecture:

SPIRE Server (on management host)
  |
  | Issues SVID (SPIFFE Verifiable Identity Document)
  v
SPIRE Agent (on each host running AI-Box sandboxes)
  |
  | Attests workload identity based on:
  |   - Container metadata (image hash, labels)
  |   - Process attributes
  |   - Node attestation
  v
Sandbox Workload
  |
  | Presents SVID to Vault
  v
Vault
  |
  | Validates SVID, checks policy
  | Returns scoped secret (git token, API key)
  v
Sandbox receives time-limited credential
```

**Why this combination**:
- **SPIFFE/SPIRE** solves "how does the sandbox prove who it is?" without pre-shared secrets.
- **Vault** solves "how do we issue scoped, short-lived secrets?" with rich policy support.
- Together, they enable: sandbox boots -> gets identity from SPIRE -> presents identity to Vault -> receives scoped git token + LLM API key -> credentials expire when sandbox terminates.

### 4.4 Token Scoping and Rotation

| Credential | Scope | TTL | Rotation |
|-----------|-------|-----|----------|
| Git token | Single repository, read+write | 4 hours | Auto-renew via credential helper |
| LLM API key | Rate-limited, specific model access | 8 hours | Injected fresh on sandbox start |
| Nexus/Artifactory token | Read-only package download | 8 hours | Injected via Vault |
| IDE license token | License validation only | Session-length | Tied to sandbox lifecycle |

All tokens MUST be:
- Revoked immediately when sandbox is destroyed.
- Logged on issuance and use.
- Impossible to persist (no writing to workspace, no git commit of tokens).

---

## 5. Policy Enforcement That Developers Cannot Bypass

### 5.1 Network Policies at Host Level

**Critical principle**: Network controls MUST be enforced OUTSIDE the container, on the host or hypervisor. Anything enforced inside the container can be bypassed by a sufficiently motivated attacker (or a compromised agent with root-equivalent access).

**Implementation**:

```
Host-level enforcement (Cilium eBPF):
  - Attached to host network namespace
  - Container traffic passes through host eBPF programs BEFORE reaching network
  - Container cannot modify, detach, or bypass these programs
  - Even if container escapes to host namespace, eBPF programs persist

Fallback (nftables):
  - nftables rules in host network namespace
  - iptables inside container are irrelevant (container has its own network namespace)
  - Host rules filter traffic on the veth bridge
```

**Why Cilium over raw nftables**:
- Policy-as-code via CiliumNetworkPolicy CRDs (even without Kubernetes, Cilium can run standalone).
- L7 visibility (can filter by HTTP host header, not just IP).
- Identity-based policies (tag sandboxes, apply rules by identity not IP).
- Better performance than iptables at scale.
- Built-in Hubble observability for network flow logging.

### 5.2 Read-Only Root Filesystem

```bash
podman run \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=2g \
  --tmpfs /var/tmp:rw,noexec,nosuid,size=1g \
  -v /workspace:/workspace:rw \
  ...
```

**Additionally**:
- Mount `/workspace` with `nosuid,nodev` options.
- Use overlayfs with a read-only lower layer for the base image.
- `/tmp` and `/var/tmp` are tmpfs (in-memory) with `noexec` to prevent executing downloaded binaries from tmp.
- Tool pack installations happen at image build time, not at runtime.

### 5.3 No Privilege Escalation

Enforced at multiple layers:

1. **Container runtime**: `--security-opt=no-new-privileges:true` -- prevents `execve` from gaining privileges via setuid/setgid bits or file capabilities.
2. **Seccomp**: Block `setuid`, `setgid`, `setreuid`, `setregid`, `setresuid`, `setresgid` syscalls.
3. **AppArmor**: `deny capability sys_admin`, deny all dangerous capabilities.
4. **Image build**: Strip all setuid/setgid bits, remove `sudo` binary entirely, remove `su` binary.
5. **User namespace**: Container root (UID 0) maps to unprivileged host UID. Even if attacker becomes root inside container, they're unprivileged on the host.

### 5.4 Seccomp-BPF Enforcement

- Seccomp profiles are loaded by the container runtime (Podman/containerd), not inside the container.
- Once loaded, the container process CANNOT modify or remove the seccomp filter (it's a one-way ratchet in the Linux kernel).
- Use `SCMP_ACT_LOG` for syscalls you want to audit but not block (useful during initial deployment to identify needed syscalls).
- Use `SCMP_ACT_ERRNO` for blocked syscalls (returns error to application, doesn't kill it).
- Use `SCMP_ACT_KILL_PROCESS` for critical syscalls that should NEVER be called (e.g., `kexec_load`).

### 5.5 OPA/Rego for Policy-as-Code

**Recommendation: Use Open Policy Agent (OPA) with Rego for declarative policy validation.**

Use cases:
1. **Admission control**: Validate `policy.yaml` files before they're applied. Ensure no sandbox can be launched with overly permissive policies.
2. **Runtime policy decisions**: Query OPA for tool permission decisions (should `git push` be allowed for this user at this time?).
3. **Compliance checking**: Continuously evaluate running sandboxes against policy. Flag drift.
4. **Audit**: OPA decision logs provide a complete record of every policy evaluation.

```rego
# Example: Deny any policy that allows raw internet access
deny[msg] {
    input.network.allow[_].hosts[_] == "*"
    msg := "Wildcard host allowlist is prohibited"
}

# Example: Require gVisor runtime for classified workloads
deny[msg] {
    input.classification == "classified"
    input.runtime != "runsc"
    msg := "Classified workloads must use gVisor runtime"
}

# Example: Enforce maximum LLM API rate limit
deny[msg] {
    input.network.allow[i].id == "llm-api"
    not input.network.allow[i].rate_limit
    msg := "LLM API endpoint must have rate_limit configured"
}
```

---

## 6. Audit and Compliance

### 6.1 What to Log (Immutable Audit Logs)

| Event Category | Specific Events | Retention |
|---------------|----------------|-----------|
| **Sandbox lifecycle** | Create, start, stop, destroy, configuration used | 2+ years |
| **Network** | All connections (allowed and denied), destination, bytes transferred, duration | 1+ year |
| **DNS** | All queries and responses | 1+ year |
| **Tool invocations** | Every command executed, arguments, exit code, user who approved | 2+ years |
| **File access** | Reads/writes to sensitive paths, workspace modifications (via auditd/Falco) | 1+ year |
| **Credential access** | Token issuance, use, rotation, revocation | 2+ years |
| **Policy decisions** | Every OPA evaluation: input, decision, reason | 2+ years |
| **LLM API traffic** | Full request/response payloads (or hashes if full storage infeasible) | 1+ year |
| **Authentication** | Sandbox user authentication, privilege changes | 2+ years |
| **Clipboard** | Clipboard transfers between sandbox and host (if enabled) | 1+ year |

**Immutability requirements**:
- Write logs to append-only storage (e.g., S3 with Object Lock, WORM-compliant storage).
- Cryptographically chain log entries (hash chain or Merkle tree) to detect tampering.
- Separate log storage from sandbox operators (no one who manages sandboxes can modify logs).
- Dual-control access to raw logs.

### 6.2 Session Recording

**Recommended for classified environments.**

**Implementation**:
- Use `script` command wrapper or custom PTY interceptor to record all terminal I/O.
- Alternative: Teleport session recording or Boundary session recording.
- Store recordings in encrypted, append-only storage.
- Enable on-demand playback for incident investigation.
- For IDE sessions: log file open/close/save events, debugging session starts, terminal commands.

**Privacy consideration**: Inform developers that sessions are recorded. This is both a legal/ethical requirement and a deterrent.

### 6.3 SIEM Integration

**Recommended: Ship structured logs to enterprise SIEM (Splunk, Elastic Security, Microsoft Sentinel).**

```
Log pipeline:

Sandbox --> Falco (runtime detection) --> Fluentd/Vector --> SIEM
                                      |
Host --> auditd --> Fluentd/Vector ----+
                                      |
Envoy proxy --> access logs -----------+
                                      |
OPA --> decision logs -----------------+
                                      |
Vault --> audit logs ------------------+
```

**Detection rules to implement**:
- Anomalous outbound data volume (>X MB to any single endpoint in Y minutes).
- DNS query spike (>Z queries/minute from a single sandbox).
- Credential access outside normal hours.
- Repeated blocked network attempts (reconnaissance indicator).
- Tool execution that was previously rare for this user/project.
- LLM API request size anomaly.

### 6.4 Compliance Evidence Generation

For classified/regulated environments, generate periodic compliance reports:

- **Control mapping**: Map AI-Box controls to NIST 800-53, CMMC, or organization-specific frameworks.
- **Continuous compliance**: Use OPA to continuously evaluate infrastructure against policy. Export compliance posture as machine-readable evidence.
- **Penetration testing**: Quarterly penetration testing of AI-Box infrastructure. Include container escape attempts, network bypass attempts, credential extraction attempts.
- **Configuration drift detection**: Alert when sandbox configurations deviate from approved baselines.

---

## 7. Concrete Tech Stack Recommendations

### Full Security Control Stack

| Control | Technology | Rationale |
|---------|-----------|-----------|
| **Container runtime** | **Podman** (rootless, daemonless) | No root daemon, native rootless support, OCI-compliant, drop-in Docker replacement |
| **Sandbox isolation** | **gVisor (runsc)** default, **Kata Containers** for highest classification | gVisor eliminates kernel syscall attack surface; Kata adds full VM isolation when needed |
| **L3/L4 network enforcement** | **Cilium** (eBPF) | Host-level enforcement, identity-aware policies, excellent observability via Hubble, cannot be bypassed from container |
| **L7 proxy** | **Envoy** with custom WASM filters | Modern, extensible, gRPC-native, CNCF graduated, per-route policies |
| **DNS control** | **CoreDNS** with restricted zone config | Lightweight, plugin-based, easy allowlist configuration, native Prometheus metrics |
| **Package mirror** | **Sonatype Nexus Repository** or **JFrog Artifactory** | Pull-through cache, vulnerability scanning, supports all major package formats, audit logging |
| **Secret management** | **HashiCorp Vault** | Dynamic secrets, short-lived tokens, rich policy engine, audit logging, extensive integrations |
| **Workload identity** | **SPIFFE/SPIRE** | Cryptographic workload identity without pre-shared secrets, attestation-based, CNCF graduated |
| **Policy engine** | **Open Policy Agent (OPA)** | Declarative Rego policies, admission control, decision logging, CNCF graduated |
| **Runtime security monitoring** | **Falco** | Kernel-level syscall monitoring, rule-based alerting, detects container escape attempts, CNCF graduated |
| **Log shipping** | **Vector** (or Fluentd) | High-performance, Rust-based, supports all output formats, built-in transforms |
| **Audit log storage** | **S3-compatible with Object Lock** (MinIO on-prem or cloud S3) | WORM-compliant, append-only, cost-effective for large log volumes |
| **SIEM** | **Elastic Security** or org-existing SIEM | Correlate events across sandboxes, hosts, and network; alert on anomalies |
| **Session recording** | **Teleport** or custom `asciinema` integration | Terminal session recording with centralized playback and access control |
| **Image signing** | **Sigstore (cosign + Rekor)** | Keyless signing, transparency log, ensures only trusted images run |
| **Vulnerability scanning** | **Grype** (image scanning) + **Trivy** (runtime) | Fast, accurate, integrates with CI/CD, covers OS packages and app dependencies |
| **Container image build** | **Buildah** (rootless, daemonless) | Builds OCI images without Docker daemon, integrates with Podman |

### Deployment Architecture Summary

```
+------------------------------------------+
|              Developer Host               |
|  (Windows 11 / Linux)                     |
|                                           |
|  +-------+  +-------------------------+  |
|  | IDE   |  | Podman (rootless)        |  |
|  | (host)|  |  +-------------------+   |  |
|  |       |<--->| AI-Box Container  |   |  |
|  |       |  |  | (gVisor runtime)  |   |  |
|  +-------+  |  |                   |   |  |
|             |  | - Read-only rootfs|   |  |
|  Cilium     |  | - /workspace (rw) |   |  |
|  eBPF  ------->| - seccomp locked  |   |  |
|  (host)     |  | - AppArmor locked |   |  |
|             |  | - No capabilities |   |  |
|             |  | - LLM sidecar     |   |  |
|             |  +-------------------+   |  |
|             +---|----------------------+  |
|                 |                          |
|  +--------------v-----------+             |
|  | Envoy L7 Proxy (host)   |             |
|  | - SNI filtering          |             |
|  | - Rate limiting          |             |
|  | - Request logging        |             |
|  +------|------|-----|------+             |
|         |      |     |                    |
+---------+------+-----+-------------------+
          |      |     |
     LLM API  Nexus  Git Server
    (Foundry) (pkgs) (internal)
```

---

## 8. Spec Improvement Recommendations (Summary)

| Section | Gap | Recommended Addition |
|---------|-----|---------------------|
| 5 (Threat Model) | Missing supply chain, DNS tunneling, clipboard, covert channels, steganographic exfiltration | Add subsections for each; see Section 1 of this review |
| 5 (Threat Model) | Container escape is "out of scope" | Bring in-scope with gVisor/Kata as primary mitigation |
| 6.1 (Network) | DNS control underspecified | Specify CoreDNS with domain allowlist, block all alternative DNS paths |
| 6.1 (Network) | No L7 inspection specified | Add Envoy proxy with content inspection for LLM traffic |
| 6.1 (Network) | No package mirror strategy | Require Nexus/Artifactory; block direct registry access |
| 6.2 (Filesystem) | No mention of `noexec` on tmpfs | Add `noexec,nosuid,nodev` to all writable mounts except workspace |
| 6.3 (Credentials) | "Short-lived env vars or temp files" too vague | Specify Vault + SPIFFE/SPIRE + sidecar proxy architecture |
| 6 (Security) | No seccomp/AppArmor profiles specified | Include reference profiles (see Section 3 of this review) |
| 6 (Security) | No container runtime recommendation | Specify gVisor as default runtime |
| 6 (Security) | No mention of rootless containers | Require rootless execution (Podman) |
| 7 (Functional) | No audit/logging requirements | Add Section 6.6 with comprehensive audit requirements |
| 7 (Functional) | No session recording mentioned | Add optional session recording for classified environments |
| 8 (Policy) | Policy lives in-repo -- who validates it? | Add OPA-based policy validation; policies must be signed |
| 8 (Policy) | No LLM-specific controls | Add LLM API rate limiting, payload logging, sidecar proxy |
| NEW | No compliance mapping | Add section mapping controls to NIST 800-53 / CMMC |
| NEW | No image supply chain security | Add Sigstore/cosign for image signing and verification |

---

## 9. Risk Residuals (Accepted Risks After All Controls)

Even with all recommended controls, the following risks remain:

1. **LLM API channel as exfiltration path**: The LLM must see code to function. A sufficiently sophisticated attacker can encode data in legitimate-looking prompts. Mitigated by logging and anomaly detection, not prevention.
2. **Zero-day container escapes**: Even gVisor can have vulnerabilities. Mitigated by defense-in-depth (host-level controls persist even after escape) and rapid patching.
3. **Insider with host admin access**: Explicitly out of scope, but noted: a host admin can disable all controls. Mitigate with privileged access management (PAM) and separation of duties.
4. **Timing-based covert channels**: Cannot be fully eliminated without destroying utility. Mitigated by rate limiting and anomaly detection.
5. **Developer screenshots of IDE**: Physical/visual exfiltration cannot be stopped by software controls. Mitigate with physical security and DLP policies.

---

*End of Security Architecture Review*
