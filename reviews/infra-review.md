# AI-Box Infrastructure & Platform Architecture Review

**Reviewer**: Infrastructure/Platform Architect
**Date**: 2026-02-18
**Spec Version**: AI-Box spec.md (initial)

---

## Executive Summary

The AI-Box spec defines a strong security-focused sandbox concept but leaves critical infrastructure decisions unspecified. This review evaluates deployment models, container runtimes, image strategy, networking, storage, scaling, and operations for a ~200 developer team on Windows 11 + Linux, using IntelliJ and VS Code, in a classified/sensitive environment with Java/Scala/Node/Angular/Bazel stacks.

**Primary recommendation**: Local-first architecture using Podman (rootless) with centralized image registry and policy distribution. This approach minimizes infrastructure cost, eliminates runtime licensing fees, provides strong security via rootless containers and host-level network isolation, and avoids dependency on centralized compute infrastructure that may be restricted in classified environments.

---

## 1. Deployment Architecture Evaluation

### Option A: Local Containers on Each Dev Machine

Each developer runs the AI-Box sandbox as a local container on their workstation.

| Criterion | Assessment |
|-----------|-----------|
| **Security** | Strong. Each sandbox is isolated to one machine. No shared attack surface. Network policy enforced at host level. Compromise of one dev machine does not affect others. |
| **Performance** | Excellent. No network latency for file I/O. IDE-to-container communication is localhost. Build tools run on local hardware (typically 16-32GB RAM, 8+ cores on modern dev machines). |
| **Cost** | Low. No centralized compute infrastructure. Only cost is the container runtime license (if Docker Desktop) and a small registry server. |
| **Ops Burden** | Medium. Each dev machine must be configured correctly. Mitigation: `aibox` CLI automates setup. Image updates distributed via registry pull. |
| **Dev Experience** | Best. Instant startup, no remote latency, works offline (after initial image pull), full IDE integration via localhost. |
| **Classified Env** | Best fit. No dependency on centralized servers. Works in fully air-gapped environments (images distributed via internal registry or even USB/media if needed). |

**Verdict**: Strong default choice for this environment.

### Option B: Centralized Server with Remote Dev Environments

All sandboxes run on a shared Kubernetes cluster. Developers connect via Coder, Gitpod, or similar remote workspace platform.

| Criterion | Assessment |
|-----------|-----------|
| **Security** | Very strong. Centralized policy enforcement. Easier to audit. Network egress controlled at cluster level. |
| **Performance** | Variable. Depends on network latency between dev machine and cluster. Remote file I/O adds 2-20ms per operation. IDE responsiveness suffers on high-latency links. |
| **Cost** | High. Kubernetes cluster for 80 concurrent workspaces (see Section 6): ~320 CPU cores, ~640GB RAM, ~8TB storage. Plus: Coder licenses ($44/user/month = $105K/year for 200 devs), K8s operations staff (1-2 FTEs), network infrastructure. |
| **Ops Burden** | High. Kubernetes cluster management, storage provisioning, node scaling, monitoring, incident response, upgrade coordination. Requires dedicated platform team. |
| **Dev Experience** | Acceptable but degraded. Input latency on remote IDEs is noticeable. VS Code Remote and JetBrains Gateway are improving but not equivalent to local. Large monorepo operations (Bazel builds) may be slow if compute is shared. |
| **Classified Env** | Problematic. Centralized server may require additional security accreditation. May conflict with data compartmentalization requirements. Single point of failure for all development. |

**Verdict**: Overengineered for this use case. High cost and ops burden without proportional benefit given that local machines are capable.

### Option C: Hybrid (Local for Speed, Remote for Heavy/Secure)

Local containers for daily development. Centralized environment available for specific use cases (heavy builds, security-critical reviews, CI-like agentic tasks).

| Criterion | Assessment |
|-----------|-----------|
| **Security** | Strong, but two attack surfaces to secure and audit. Policy must be consistent across both. |
| **Performance** | Best of both. Local for interactive work, remote for compute-heavy tasks. |
| **Cost** | Medium. Smaller central cluster (sized for peak overflow, not all devs). Still requires some K8s ops. |
| **Ops Burden** | Highest. Two deployment models to maintain, test, and support. |
| **Dev Experience** | Good but confusing. Devs must understand when to use which environment. Context switching between local and remote. |
| **Classified Env** | Depends on central component. If the central piece is optional, this degrades gracefully to Option A. |

**Verdict**: Consider as a Phase 2 evolution. Start with Option A, add centralized capabilities only if specific needs arise (e.g., CI-driven agentic tasks that need stronger isolation than a dev machine provides).

### Recommendation: Option A (Local-First)

Start with local containers. The `aibox` CLI abstracts the container lifecycle, so migrating to centralized later is feasible without changing developer workflows. The classified environment constraint strongly favors local deployment.

---

## 2. Container Runtime Selection

### Comparison Matrix

| Feature | Docker Desktop | Podman | containerd (direct) |
|---------|---------------|--------|-------------------|
| **License cost (200 devs)** | $12K-$50K/year (Teams $5/user; Business $21/user) | $0 (Apache 2.0) | $0 (Apache 2.0) |
| **Rootless by default** | No (requires config) | Yes | Yes (with nerdctl) |
| **Daemon** | Yes (dockerd) | No (daemonless, fork-exec) | Yes (containerd daemon) |
| **Windows support** | Mature (WSL2 + Hyper-V) | Good (podman-machine via WSL2/Hyper-V; maturing rapidly) | Requires manual WSL2 setup |
| **OCI compliance** | Yes | Yes | Yes |
| **Dockerfile compat** | Native | Full (uses Buildah) | Via BuildKit |
| **Compose support** | Native | Via podman-compose or docker-compose compatibility socket | Via nerdctl compose |
| **systemd integration** | Limited | Excellent (generate systemd units, quadlet) | Manual |
| **Security features** | Seccomp, AppArmor, user namespaces (optional) | Seccomp, SELinux, user namespaces (default), no daemon to attack | Seccomp, AppArmor |
| **Dev familiarity** | Highest | High (CLI is alias-compatible: `alias docker=podman`) | Low |

### Rootless vs Rootful

**Rootless is mandatory for AI-Box.** The spec requires least privilege, and the threat model includes prompt injection leading to container escape attempts. Rootless containers:

- Run entirely in user namespace (no real root privileges)
- Cannot modify host network configuration (iptables)
- Cannot access privileged kernel interfaces
- Significantly reduce container escape impact

With rootless Podman:
- Containers map to unprivileged user on host
- Even if an AI agent achieves root inside the container, it maps to an unprivileged UID on the host
- Network isolation via slirp4netns or pasta (user-mode networking)

**Important caveat**: Rootless networking has limitations. Containers cannot bind to privileged ports (<1024) and network performance is slightly reduced due to user-mode networking. For AI-Box this is acceptable -- the sandbox does not need to run production servers.

### Windows Support Deep Dive

Both Docker Desktop and Podman use a Linux VM on Windows (WSL2 or Hyper-V). The actual container runtime runs inside this VM. Key differences:

**Docker Desktop on Windows**:
- Polished GUI, seamless WSL2 integration
- Automatic WSL2 distro management
- VS Code and IntelliJ integration well-tested
- License required for organizations >250 employees or >$10M revenue
- $21/user/month (Business) for 200 devs = **$50,400/year**

**Podman on Windows**:
- `podman machine init/start` creates a Fedora CoreOS VM in WSL2 or Hyper-V
- CLI experience is equivalent to Docker (alias-compatible)
- VS Code Dev Containers extension works with Podman (configure `dev.containers.dockerPath` to `podman`)
- IntelliJ supports Podman as a Docker-compatible runtime
- No GUI equivalent to Docker Desktop (but CLI is sufficient for AI-Box)
- **$0/year**

**Podman Windows maturity assessment**: As of 2025-2026, Podman on Windows via WSL2 is production-ready for developer workstation use. The `podman machine` subsystem handles VM lifecycle. Remaining rough edges (slower image pulls on first use, occasional WSL2 networking quirks) are manageable with automation in the `aibox` CLI.

### Recommendation: Podman (Rootless)

- **$50K/year savings** vs Docker Desktop Business
- Rootless by default aligns with AI-Box security requirements
- Daemonless architecture reduces attack surface (no privileged daemon to compromise)
- OCI-compliant; all Docker images work unmodified
- CLI compatibility means minimal retraining
- `aibox` CLI abstracts runtime details; devs interact with `aibox`, not `podman` directly

Provide a compatibility layer: `aibox` detects available runtime (Podman preferred, Docker supported as fallback) so teams with existing Docker Desktop licenses are not blocked.

---

## 3. Image Strategy

### Base Image Selection

| Option | Pros | Cons |
|--------|------|------|
| **Ubuntu 24.04 LTS** | Largest ecosystem, best tool compatibility, devs know it, LTS support until 2029 | Larger base size (~80MB compressed), more packages than needed |
| **Fedora (latest)** | Cutting-edge packages, SELinux-native, Podman's home distro | 6-month release cycle, not LTS |
| **Wolfi/Chainguard** | Minimal CVEs, purpose-built for containers, apk-based | Small ecosystem, unfamiliar to most devs, may lack niche packages |
| **Debian Bookworm** | Stable, smaller than Ubuntu, good package availability | Older packages than Ubuntu, less dev tooling pre-packaged |

**Recommendation: Ubuntu 24.04 LTS** as the base.

Rationale:
- Java/Scala/Node/Angular/Bazel stacks all have first-class Ubuntu support
- Largest community and package ecosystem reduces "package not found" friction
- LTS provides 5-year security updates without base image churn
- Developers are most familiar with Ubuntu, reducing support burden
- `apt` package manager is well-understood

For organizations that prioritize minimal CVE surface, consider a **Wolfi-based variant** as a hardened alternative offered alongside the Ubuntu base.

### What Goes in the Base Image

The base image should include the minimum needed to function as a development shell. Tool packs add everything else.

**Base image contents**:
```
# OS layer
- Ubuntu 24.04 LTS (minimal)
- ca-certificates (for TLS)
- curl, wget (disabled by network policy, but needed for tool pack installs)
- git
- ssh-client (for git operations, not for host access)
- jq, yq (config parsing)
- vim, nano (basic editing, some agents need them)
- bash, zsh (shell options)
- tmux, screen (session management)
- build-essential (gcc, make -- many tools need compilation)
- python3 (many tools depend on it)

# AI-Box layer
- aibox-agent (policy enforcement daemon)
- aibox-proxy (egress proxy client config)
- /etc/aibox/policy.yaml (default deny-all policy)
- SSH server (for IDE remote connections)
```

**Explicitly NOT in base**:
- Java/Scala runtimes (tool pack)
- Node.js (tool pack)
- Bazel (tool pack)
- AI CLI tools like Claude Code, Codex CLI (tool pack or MCP pack)
- IDE server components (installed by IDE remote connection)
- Credentials of any kind

### Layer Strategy for Tool Packs

Two viable approaches:

**Approach 1: Pre-built image variants (recommended for common stacks)**
```
aibox-base:24.04                    # ~500MB
aibox-java:21-24.04                 # base + JDK 21 + Maven + Gradle  (~900MB)
aibox-node:20-24.04                 # base + Node 20 + npm/yarn       (~650MB)
aibox-full:24.04                    # base + Java + Node + Bazel      (~1.5GB)
```

Advantages:
- Single pull, no runtime install wait
- Deterministic, tested combinations
- Image signing covers the full stack

**Approach 2: Runtime install via tool packs**
```bash
aibox install java@21 node@20 bazel@7
```
Tool packs download and install into a persistent volume (`/opt/toolpacks/`) that survives container restarts. First install is slow; subsequent starts are instant.

Advantages:
- Smaller base image pull
- Flexible combinations
- No combinatorial explosion of image variants

**Recommendation**: Use both. Ship 3-4 pre-built variants for the most common stacks (Java+Node is likely the 80% case). Support runtime tool packs for less common tools and custom combinations. The `aibox` CLI presents a unified interface regardless of which approach is used.

### Image Registry

**Recommendation: Harbor (self-hosted)**

- Open source (Apache 2.0), CNCF graduated project
- Built-in vulnerability scanning (Trivy integration)
- Image signing verification (Cosign/Notation support)
- Replication (for multi-site or air-gapped distribution)
- RBAC and audit logging
- Supports OCI artifacts (not just container images -- can store tool packs, policies)
- Garbage collection for storage management

Deployment: Run Harbor on a small dedicated server (4 CPU, 16GB RAM, 1TB storage). For air-gapped environments, Harbor's replication feature can sync from a connected staging environment.

### Image Signing

**Recommendation: Cosign (Sigstore)**

- All AI-Box images must be signed at build time
- Podman and Docker can verify signatures before pulling
- Policy: containers refuse to run unsigned images
- Signing keys managed by the platform team (not individual devs)

```bash
# Build pipeline signs images
cosign sign --key cosign.key harbor.internal/aibox/base:24.04

# Client verification (configured in /etc/containers/policy.json for Podman)
# Rejects unsigned or tampered images automatically
```

### Size Targets and Optimization

| Image | Target Size (compressed) | Strategy |
|-------|------------------------|----------|
| aibox-base | < 500MB | Minimal Ubuntu + essential tools only |
| aibox-java | < 900MB | Multi-stage: build JDK layer separately |
| aibox-node | < 650MB | Use official Node binary, not full nvm |
| aibox-full | < 1.5GB | Shared base layers reduce effective pull size |

Optimization techniques:
- Multi-stage builds: compile tools in builder stage, copy binaries to runtime stage
- Layer ordering: put rarely-changing layers (OS, runtime) first, frequently-changing layers (config, aibox-agent) last
- `.dockerignore` / build context minimization
- `apt-get clean && rm -rf /var/lib/apt/lists/*` after installs
- Consider `zstd` compression for OCI images (20-30% smaller than gzip, supported by Podman and recent Docker)

### Update Cadence

| Component | Cadence | Trigger |
|-----------|---------|---------|
| OS security patches | Weekly | CVE publication |
| Tool pack versions | Monthly | Upstream releases |
| Base image rebuild | Monthly | Accumulate patches |
| Emergency patches | Immediate | Critical CVE (CVSS 9+) |

Process:
1. Automated weekly build pipeline rebuilds images with latest patches
2. Images pushed to Harbor with new tag (date-stamped: `24.04-20260218`)
3. `aibox update` on dev machine pulls latest signed image
4. Old containers not disrupted; new sandbox sessions use new image
5. Optional: `aibox` warns if running image is >2 weeks old

---

## 4. Networking Architecture

This is the most critical section for security. The spec's core promise is network egress control. The implementation must be airtight.

### Egress Proxy Design

**Recommendation: Host-level proxy with iptables enforcement**

Architecture:
```
[Dev Machine / WSL2 VM]
  |
  |-- [iptables/nftables rules]  <-- enforced at host level
  |     |
  |     |-- ALLOW: container -> proxy (port 3128/8080)
  |     |-- ALLOW: container -> DNS resolver (port 53, local only)
  |     |-- DROP: container -> everything else
  |     |-- ALLOW: proxy -> allowlisted destinations
  |     |-- DROP: proxy -> everything else
  |
  |-- [Squid Proxy]  <-- runs on host network (or in privileged container)
  |     |-- Reads allowlist from /etc/aibox/proxy-allowlist.conf
  |     |-- Logs all requests (destination, decision, timestamp, container ID)
  |     |-- Supports CONNECT for HTTPS (no TLS interception by default)
  |
  |-- [AI-Box Container]  <-- runs in isolated network namespace
        |-- HTTP_PROXY / HTTPS_PROXY env vars point to proxy
        |-- /etc/resolv.conf points to controlled DNS
        |-- Cannot modify iptables (rootless, no NET_ADMIN capability)
        |-- Cannot reach any host except proxy
```

**Why host-level, not sidecar**:
- Sidecar runs in the same network namespace or a linked one. The container could potentially interfere with the sidecar if it gains enough privilege.
- Host-level iptables cannot be modified from inside a rootless container. The container lacks `CAP_NET_ADMIN` and runs in a separate user namespace.
- Host-level proxy is simpler to operate and debug.

**Why Squid**:
- Mature, battle-tested HTTP/HTTPS proxy
- ACL system supports domain allowlists, regex matching, time-based rules
- CONNECT method support for HTTPS tunneling without TLS interception
- Access logging built-in
- Available in Ubuntu repos, easy to install
- Low resource footprint (<100MB RAM for 200 concurrent connections)

Alternative: Envoy proxy (more modern, better metrics, but significantly more complex to configure for a simple allowlist use case).

### DNS Resolution Control

```
[AI-Box Container]
  |
  resolv.conf -> 127.0.0.53 (systemd-resolved on host)
                     |
                     v
              [systemd-resolved or CoreDNS on host]
                     |
                     |-- Responds to allowlisted domains only
                     |-- Returns NXDOMAIN for blocked domains
                     |-- Logs all queries
                     |-- Forwards allowed queries to upstream DNS
```

**Implementation**:
- Container's `/etc/resolv.conf` is bind-mounted (read-only) pointing to the host's DNS resolver
- Host runs CoreDNS with a policy plugin or systemd-resolved with response policy zones (RPZ)
- iptables blocks DNS (UDP/TCP 53) to any address except the local resolver
- This prevents the container from using hardcoded DNS servers or DNS-over-HTTPS to bypass filtering

### Network Namespace Isolation

Podman rootless containers already run in a separate network namespace. The `aibox` CLI must ensure:

1. **No `--network=host`**: Container must use its own network namespace (Podman default)
2. **No `--cap-add=NET_ADMIN`**: Container cannot modify its own network config
3. **No `--privileged`**: Obvious, but must be enforced
4. **iptables on host/WSL2**: Block all direct egress from the container's veth interface except to the proxy

```bash
# Example iptables rules (set by aibox setup on host/WSL2)
# Assuming container bridge is podman0 (or similar)
iptables -I FORWARD -i podman0 -d <proxy_ip> -p tcp --dport 3128 -j ACCEPT
iptables -I FORWARD -i podman0 -d <dns_ip> -p udp --dport 53 -j ACCEPT
iptables -I FORWARD -i podman0 -j DROP
```

**Rootless networking caveat**: Podman rootless uses `pasta` (or older `slirp4netns`) for user-mode networking. This does NOT use a bridge interface on the host -- it uses a TAP device in the user namespace. The iptables approach above applies to rootful mode. For rootless:

- `pasta` creates a network namespace connected via a pipe to the host
- Traffic exits through the host's network stack
- To enforce egress control in rootless mode, configure `pasta` with `--map-gw` to route traffic through a specific gateway (the proxy)
- Alternatively, use Podman's `--network=pasta:--dns-forward,...` options to control DNS and routing
- Or run the proxy inside the same network namespace as the container but with elevated configuration (using pasta's port forwarding)

**Practical rootless approach**: Run Squid on the host, expose it on localhost. Configure the container's `pasta` networking to only allow connections to localhost:3128 (proxy) and localhost:53 (DNS). This is achievable via pasta's `--tcp-ports` and `--udp-ports` options.

### IDE Remote Connections

**VS Code Remote - Containers / SSH**:
- VS Code connects to the container via SSH (port 2222 mapped to localhost)
- Or via the Dev Containers extension, which connects to the Podman socket
- Connection is localhost only -- no network traversal, no egress proxy involvement
- VS Code server installs inside the container on first connect (needs internet or pre-installed)
- **Pre-install VS Code Server** in the base image to avoid first-connect download

**IntelliJ Remote / Gateway**:
- JetBrains Gateway connects via SSH to the container
- IDE backend runs inside the container
- UI renders on the host
- Same localhost-only connection model

```
[Windows Host]
  |
  [VS Code / IntelliJ] ---SSH (localhost:2222)---> [WSL2 VM port forward]
                                                        |
                                                   [Container SSH :22]
```

The IDE connection path does NOT traverse the egress proxy. It is a direct host-to-container connection via localhost port mapping. This is by design: IDE traffic is not an exfiltration vector (it stays on the local machine).

### Internal Services Architecture

```
                        [Internal Network]
                              |
                    [Squid Egress Proxy]
                      /       |        \
            [Harbor]  [Nexus/Artifactory]  [LLM Gateway]
            (images)  (Maven/npm/etc.)     (Foundry/vLLM)
```

The proxy allowlist includes:
- `harbor.internal:443` -- Container image registry
- `nexus.internal:443` -- Maven Central mirror, npm registry mirror, PyPI mirror
- `foundry.internal:443` -- LLM API gateway
- `docs.internal:443` -- Internal documentation (optional)

All of these should be internal mirrors/proxies, not direct internet access. This is especially important in classified/air-gapped environments.

---

## 5. Storage Architecture

### Workspace Persistence

**Recommendation: Named volumes for workspace, bind mounts for git repos**

```
[Container]
  /workspace        <-- bind mount from host: ~/projects/my-repo
  /opt/toolpacks    <-- named volume: aibox-toolpacks (persists across sessions)
  /home/dev         <-- named volume: aibox-home-<project> (user config, shell history)
  /tmp              <-- tmpfs (ephemeral, cleared on stop)
  /                 <-- read-only image filesystem
```

**Why bind mount for /workspace**:
- Developer's source code stays on the host filesystem
- IDE file watchers and git tools work on host and in container
- No data loss if container is deleted
- Files visible in both host and container (critical for hybrid IDE workflow)
- Developer explicitly chooses what to mount (least privilege)

**Why named volumes for toolpacks and home**:
- Survive container recreation (image updates)
- Faster than bind mounts for random I/O (no filesystem translation layer)
- Not exposed on host filesystem (isolation)

### Git Repository Strategy

**Recommendation: Mount from host via bind mount**

```bash
aibox start --workspace ~/projects/my-service
# Internally: podman run -v ~/projects/my-service:/workspace:Z ...
```

The `:Z` flag (SELinux relabel) ensures the container can access the mounted directory with proper SELinux contexts. On non-SELinux systems, omit this.

**Why not clone inside container**:
- Developers already have repos cloned on their machines
- Git credentials are managed on the host (SSH keys, credential helpers)
- Host-side IDE features (local git blame, file history) continue to work
- No need to mount SSH keys into the container (a security risk the spec explicitly blocks)

**Git operations inside the container**:
- `git status`, `git diff`, `git log` -- work on mounted repo, no network needed
- `git pull`, `git push` -- need network access through proxy to git server
- Add git server to proxy allowlist: `git.internal:443`
- Git credential: inject short-lived token via `aibox` CLI at session start, stored in container-only tmpfs

### Build Cache Sharing

Build caches are critical for developer productivity. A cold Maven or npm cache adds 5-30 minutes to first build.

```
[Named Volumes for Build Caches]
  aibox-maven-cache    -> /home/dev/.m2/repository   (shared across projects)
  aibox-gradle-cache   -> /home/dev/.gradle/caches   (shared across projects)
  aibox-npm-cache      -> /home/dev/.npm             (shared across projects)
  aibox-bazel-cache    -> /home/dev/.cache/bazel      (shared across projects)
```

These volumes are:
- Per-user (not shared between developers on centralized setup)
- Persistent across container sessions
- Populated on first build, reused on subsequent sessions
- Can be pre-warmed by the `aibox` CLI pulling a cache snapshot from the registry

### Performance: Windows Volume Mounts

This is a known pain point. File I/O performance through the Windows <-> WSL2 boundary varies:

| Method | Read Perf | Write Perf | Notes |
|--------|-----------|------------|-------|
| **9p (WSL2 default for Windows paths)** | Slow (2-5x) | Slow (3-10x) | `/mnt/c/Users/...` paths. Avoid for source code. |
| **ext4 inside WSL2** | Native | Native | Files stored in WSL2's ext4 filesystem. Best performance. |
| **virtio-fs (WSL2 2.x)** | Good (1.2-1.5x) | Good (1.5-2x) | Newer, faster than 9p. Available in recent WSL2 builds. |

**Recommendation**:
- Source code should live inside the WSL2 ext4 filesystem (e.g., `~/projects/`), NOT on the Windows NTFS filesystem (`/mnt/c/Users/...`)
- The `aibox setup` command should detect and warn if workspace is on a Windows path
- IDE accesses files via WSL2 path (`\\wsl$\Ubuntu\home\dev\projects\...`) which is fast for editors
- For users who insist on Windows-native paths, document the performance impact and recommend `git clone` inside WSL2

### Backup Strategy

- **Source code**: Git is the backup. Repos are cloned from the server; container loss does not lose code (uncommitted changes excepted).
- **Build caches**: Disposable. Can be rebuilt. Optional: export/import named volumes.
- **Container configuration**: Stored in `aibox.yaml` in the project repo. Recreatable via `aibox start`.
- **Tool packs**: Re-downloaded from registry on new setup.

Recommendation: No special backup infrastructure needed. Everything is reproducible from the image + repo + configuration. Document this as a feature, not a gap.

---

## 6. Scaling for 200 Developers

### Local-First Model (Recommended)

With local containers, "scaling" means ensuring 200 developer machines are correctly configured, not provisioning centralized compute.

**What scales centrally**:
| Component | Sizing | Notes |
|-----------|--------|-------|
| Harbor registry | 4 CPU, 16GB RAM, 2TB storage | Serves image pulls for 200 devs. Cache layer keeps hot images in memory. |
| Squid proxy (if centralized) | N/A for local model | Each dev machine runs its own proxy. |
| Internal mirrors (Nexus) | 4 CPU, 16GB RAM, 1TB storage | Maven/npm/PyPI mirrors. Likely already exists in most orgs. |
| LLM gateway | Depends on model serving | Separate concern; AI-Box consumes it, does not provide it. |
| Policy distribution | Git repository | No dedicated server needed. Policies are files in a repo. |

**Total centralized infra for local model**: 1-2 servers (8 CPU, 32GB RAM, 3TB storage). This is minimal -- most organizations already have registry and artifact servers.

### If Centralized Were Chosen (For Reference)

Modeling for 200 developers, not all active simultaneously:

**Concurrency model**:
- 200 total developers
- ~40% active at any given time during work hours = 80 concurrent sessions
- Peak: 60% = 120 concurrent sessions (Monday morning, deadline days)
- Each workspace: 4 vCPU, 8GB RAM, 20GB storage (Java/Bazel builds are memory-hungry)

**Cluster sizing**:
| Resource | Normal (80 concurrent) | Peak (120 concurrent) |
|----------|----------------------|---------------------|
| vCPU | 320 | 480 |
| RAM | 640 GB | 960 GB |
| Storage (workspaces) | 1.6 TB | 2.4 TB |
| Storage (caches, images) | 2 TB | 2 TB |

**Kubernetes cluster**:
- Control plane: 3 nodes (4 CPU, 16GB each) -- HA
- Workers: 12-20 nodes (32 CPU, 128GB each) depending on overcommit ratio
- Storage: Ceph or Longhorn for persistent volumes, or NFS for simplicity
- Ingress: Nginx or Envoy for IDE WebSocket connections

**Cost framework** (hardware, not cloud):
| Item | Estimated Cost |
|------|---------------|
| 20 worker nodes (rack servers) | $200K-$400K |
| 3 control plane nodes | $30K-$60K |
| Network switches (10GbE) | $20K-$40K |
| Storage (Ceph: 3 nodes, 20TB each) | $60K-$120K |
| Coder licenses (200 devs, $44/mo) | $105K/year |
| Platform team (2 FTEs) | $300K-$500K/year |
| **Total Year 1** | **$715K - $1.22M** |
| **Total Ongoing/Year** | **$405K - $605K/year** |

This is why local-first is recommended. The cost difference is dramatic.

### Resource Quotas (If Centralized)

```yaml
# Kubernetes ResourceQuota per workspace namespace
apiVersion: v1
kind: ResourceQuota
metadata:
  name: workspace-quota
spec:
  hard:
    requests.cpu: "4"
    requests.memory: 8Gi
    limits.cpu: "8"       # burst allowed
    limits.memory: 12Gi   # burst allowed
    persistentvolumeclaims: "5"
    requests.storage: 50Gi
```

---

## 7. Operations

### Base Image Updates Without Disrupting Developers

**Process**:
1. Automated CI pipeline builds new image weekly (or on-demand for CVEs)
2. New image pushed to Harbor with date tag: `aibox-base:24.04-20260218`
3. `latest` tag updated to point to new image
4. `aibox` CLI checks for updates on `aibox start` (compares local image digest to registry)
5. If update available: pull new image in background, notify dev
6. Running containers are NOT affected (they use the old image until restarted)
7. On next `aibox start` or `aibox update`, new image is used
8. Old images garbage-collected after 30 days

**Key principle**: Never interrupt a running session. Updates are opt-in at session boundaries.

**Emergency process** (critical CVE):
1. Build and push patched image immediately
2. `aibox` CLI shows prominent warning: "Critical security update available"
3. Organization can push a policy requiring update within 24 hours
4. After deadline: `aibox start` refuses to launch outdated image

### Monitoring and Alerting

**For local-first model, monitoring is lightweight**:

| What | How | Where |
|------|-----|-------|
| Image pull failures | `aibox` CLI logs | Local, reported to central if telemetry enabled |
| Proxy denials | Squid access.log on dev machine | Local, optionally forwarded |
| Policy violations | aibox-agent logs | Local, optionally forwarded |
| Container health | `aibox status` command | Local |
| Registry health | Harbor's built-in metrics | Central Prometheus + Grafana |
| Image scan results | Harbor Trivy integration | Central dashboard |

**Optional centralized telemetry** (if classified environment permits):
- Forward proxy denial logs and policy violation logs to central SIEM
- Aggregate: which endpoints are most requested (tune allowlists)
- Aggregate: which tool pack combinations are most used (optimize pre-built images)
- Dashboard: how many devs are on current image vs. outdated

**Stack**: Prometheus + Grafana for the central components (Harbor, mirrors). Local monitoring via `aibox` CLI output and log files.

### Alerting on Security Events

| Event | Severity | Action |
|-------|----------|--------|
| Repeated proxy denials to same blocked host | Warning | May indicate compromised agent. Alert dev + security team. |
| Attempted access to `/home`, `/.ssh` from container | Critical | Policy engine blocks it. Log and alert security team. |
| Unsigned image execution attempted | Critical | Blocked by policy. Alert platform team. |
| CVE found in running image (scan) | High | Notify dev to update. |
| Container escape attempt detected | Critical | Kill container. Alert security team. Requires host-level monitoring (Falco). |

**Recommendation: Deploy Falco on developer machines** (lightweight, runs in WSL2) for runtime security monitoring. Falco detects suspicious syscalls (e.g., attempts to access `/etc/shadow`, unexpected network connections, privilege escalation).

### Self-Service vs. Managed Model

**Recommendation: Self-service with guardrails**

| Aspect | Self-Service | Guardrails |
|--------|-------------|------------|
| Starting/stopping sandbox | Dev controls lifecycle | `aibox` enforces image signing, policy loading |
| Choosing tool packs | Dev installs what they need | Only signed tool packs from approved registry |
| Modifying policy | Dev can tighten policy | Cannot loosen below org baseline policy |
| Updating images | Dev runs `aibox update` | Mandatory update deadline for critical CVEs |
| Troubleshooting | Dev uses `aibox doctor` | Escalation path to platform team Slack channel |

**Support model**:
- Tier 1: `aibox doctor` (automated diagnostics + fix suggestions)
- Tier 2: Platform team Slack channel (responded within 4 hours)
- Tier 3: Platform team direct support (for infrastructure issues)
- Expected support load: 1-2 tickets/day once stable, higher during rollout

### Incident Response for Sandbox Failures

| Scenario | Response |
|----------|----------|
| Container won't start | `aibox doctor` checks: image present, runtime running, disk space, WSL2 health |
| Network blocked that shouldn't be | Check proxy allowlist, verify DNS resolution, `aibox network test` |
| Build tools missing/broken | `aibox repair toolpacks` (re-downloads from registry) |
| Data loss (workspace corruption) | Git repo is source of truth. Uncommitted work: check container's writable layer. Named volumes may be recoverable. |
| Suspected security breach | Kill container (`aibox stop --force`). Preserve logs. Escalate to security team. |

### Disaster Recovery

For local-first:
- **Dev machine dies**: Re-image machine, install `aibox`, pull images from registry, clone repos from git server. Time to recovery: 1-2 hours.
- **Registry (Harbor) down**: Devs continue working with cached local images. No new image pulls. Fix registry. Capacity for outage: days (all images cached locally).
- **Artifact mirror (Nexus) down**: Builds fail if dependencies not cached locally. Priority fix. Mitigation: build caches in named volumes reduce dependency on live mirror.

---

## 8. Recommended Architecture

### Architecture: Local-First with Podman, Centralized Policy and Images

**Description**:

Every developer workstation runs AI-Box sandboxes locally using Podman in rootless mode. The `aibox` CLI manages the container lifecycle, network isolation, and policy enforcement. A small centralized infrastructure provides signed container images (Harbor), artifact mirrors (Nexus), and policy distribution (Git). All network egress from the sandbox is routed through a local Squid proxy that enforces domain allowlists. IDE connections (VS Code, IntelliJ) connect to the container via localhost SSH tunnels, bypassing the egress proxy. The organization's baseline security policy is distributed via a Git repository and enforced by the `aibox` CLI -- developers can tighten but not loosen it.

### Component List

| Component | Technology | Runs Where | Purpose |
|-----------|-----------|------------|---------|
| Container runtime | Podman 5.x (rootless) | Dev machine (WSL2 on Windows) | Runs sandboxed containers |
| Container networking | pasta (Podman default) | Dev machine | User-mode networking for rootless containers |
| Egress proxy | Squid 6.x | Dev machine (host/WSL2) | Enforces domain allowlist for container traffic |
| DNS resolver | CoreDNS or systemd-resolved | Dev machine (host/WSL2) | Controlled DNS for containers |
| Network enforcement | nftables rules | Dev machine (host/WSL2) | Blocks container traffic except to proxy |
| Image registry | Harbor 2.x | Central server (1 node) | Stores signed images, scans for CVEs |
| Image signing | Cosign (Sigstore) | Build pipeline | Signs images at build time |
| Artifact mirror | Nexus Repository 3.x | Central server (1 node) | Mirrors Maven Central, npm, PyPI |
| Policy distribution | Git repository | Central git server (existing) | Stores org baseline policy.yaml |
| Base image | Ubuntu 24.04 LTS | Built in CI | Foundation for all sandbox images |
| CLI | `aibox` (custom, Go or Rust) | Dev machine | Orchestrates all local components |
| Runtime monitoring | Falco (optional) | Dev machine (WSL2) | Detects suspicious container behavior |
| Central monitoring | Prometheus + Grafana | Central server | Monitors Harbor, Nexus health |
| IDE integration | SSH tunnel (port 2222) | Dev machine (localhost) | VS Code Remote / JetBrains Gateway |

### Network Topology

```
[DEVELOPER WORKSTATION - Windows 11]
 |
 |-- [Windows Host]
 |     |-- VS Code / IntelliJ (UI)
 |     |-- SSH tunnel to localhost:2222
 |
 |-- [WSL2 VM (Ubuntu)]
       |
       |-- [Podman (rootless)]
       |     |-- [AI-Box Container]
       |           |-- /workspace (bind mount from WSL2 fs)
       |           |-- aibox-agent (policy enforcement)
       |           |-- SSH server (:22 -> host :2222)
       |           |-- HTTP_PROXY=http://localhost:3128
       |           |-- Isolated network namespace (pasta)
       |
       |-- [Squid Proxy (:3128)]
       |     |-- Allowlist: harbor.internal, nexus.internal,
       |     |              foundry.internal, git.internal
       |     |-- Logs: /var/log/squid/access.log
       |
       |-- [CoreDNS (:53)]
       |     |-- Policy: allow only internal domains
       |     |-- Forward allowed queries to org DNS
       |
       |-- [nftables rules]
             |-- Container traffic -> proxy only
             |-- Block all direct egress from container

[CENTRAL INFRASTRUCTURE]
 |
 |-- [Harbor Registry]  harbor.internal:443
 |     |-- AI-Box images (signed)
 |     |-- Trivy vulnerability scanning
 |
 |-- [Nexus Repository]  nexus.internal:443
 |     |-- Maven Central mirror
 |     |-- npm registry mirror
 |     |-- PyPI mirror
 |
 |-- [LLM Gateway]  foundry.internal:443
 |     |-- AI model API proxy
 |
 |-- [Git Server]  git.internal:443
       |-- Source code repositories
       |-- AI-Box policy repository
       |-- AI-Box image build pipeline
```

### High-Level Deployment Steps

**Phase 0: Central Infrastructure (Week 1-2)**
1. Deploy Harbor registry on a dedicated server (or VM)
2. Configure Nexus as Maven/npm/PyPI mirror (may already exist)
3. Create Git repository for AI-Box policies and image build pipeline
4. Set up CI pipeline to build, sign, and push AI-Box images to Harbor
5. Build initial image set: `aibox-base`, `aibox-java`, `aibox-node`, `aibox-full`

**Phase 1: CLI and Image Development (Week 2-4)**
1. Develop `aibox` CLI with core commands: `setup`, `start`, `stop`, `status`, `update`, `doctor`
2. CLI handles: Podman install/config, Squid setup, nftables rules, DNS config
3. CLI detects OS (Windows/WSL2 vs native Linux) and adapts
4. Test on Windows 11 + WSL2 and native Linux

**Phase 2: Pilot Rollout (Week 4-6)**
1. Select 10-15 developers from different teams (Java, Node, Bazel users)
2. Provide `aibox setup` one-liner install
3. Collect feedback on: setup friction, performance, tool compatibility, IDE integration
4. Iterate on image contents, tool packs, and policy defaults

**Phase 3: Broad Rollout (Week 6-10)**
1. Create setup documentation and troubleshooting guide
2. Record setup walkthrough video
3. Roll out in waves: 50 devs/week
4. Platform team available in Slack for support
5. Monitor support ticket volume; iterate on `aibox doctor` to auto-resolve common issues

**Phase 4: Hardening (Week 10-14)**
1. Enable Falco runtime monitoring (optional, based on security team input)
2. Centralize proxy denial logs for security analysis
3. Implement mandatory image update enforcement
4. Conduct security assessment of the deployed system
5. Document runbook for incident response

### Day-2 Operations Model

| Activity | Frequency | Owner | Automation |
|----------|-----------|-------|-----------|
| Image rebuild (patches) | Weekly | CI pipeline | Fully automated |
| Image signing | Every build | CI pipeline | Fully automated |
| CVE triage (Harbor scan results) | Daily review | Platform team | Alert-driven |
| Policy updates | As needed | Security team | Git PR + review |
| Tool pack updates | Monthly | Platform team | Semi-automated (test + promote) |
| Dev support | Ongoing | Platform team (0.5 FTE) | `aibox doctor` handles most issues |
| Nexus mirror sync | Continuous | Nexus | Automatic |
| Harbor garbage collection | Weekly | Harbor cron job | Automatic |
| WSL2/Podman compatibility testing | Monthly | Platform team | Manual (test matrix) |
| Capacity monitoring (registry, mirrors) | Continuous | Prometheus alerts | Automatic |

**Staffing**: The local-first model requires approximately 0.5-1 FTE for ongoing platform operations, compared to 2+ FTEs for a centralized Kubernetes-based approach. During initial rollout (Phase 1-3), allocate 2 FTEs.

---

## 9. Spec Gaps and Recommendations

The following items are not addressed in the current spec and should be added:

### 9.1 Container Runtime Not Specified
**Gap**: The spec does not specify which container runtime to use.
**Recommendation**: Specify Podman (rootless) as the primary runtime with Docker as a supported fallback. Add a section on runtime requirements.

### 9.2 No Windows-Specific Guidance
**Gap**: The spec does not mention Windows, WSL2, or cross-platform concerns.
**Recommendation**: Add a "Platform Support" section covering Windows 11 + WSL2 requirements, macOS support (if needed), and native Linux. Document the performance implications of filesystem paths (NTFS vs ext4).

### 9.3 Proxy Implementation Unspecified
**Gap**: The spec mentions an "egress proxy" but does not specify where it runs, what software powers it, or how it integrates with the container.
**Recommendation**: Add a "Connectivity Layer Implementation" section specifying Squid (or chosen proxy), deployment model (host-level), and network namespace enforcement mechanism.

### 9.4 IDE Integration Mechanics Missing
**Gap**: The spec says "agents connect via standard interfaces (terminal, IDE remote, HTTP, MCP)" but does not specify how IDE remote connections work through the isolation boundary.
**Recommendation**: Add an "IDE Integration" section specifying SSH tunnel approach, port mapping, and which IDE server components are pre-installed in the image.

### 9.5 No Image Lifecycle
**Gap**: No mention of how images are built, signed, distributed, or updated.
**Recommendation**: Add an "Image Lifecycle" section covering build pipeline, signing, registry, update cadence, and rollback procedure.

### 9.6 Build Cache Strategy Missing
**Gap**: No mention of build caches (Maven, Gradle, npm, Bazel). Without shared caches, every new session starts with a cold build -- devastating for productivity.
**Recommendation**: Add a "Build Caches" section specifying named volumes for caches that persist across sessions.

### 9.7 No Offline/Air-Gapped Mode
**Gap**: The spec does not address fully air-gapped environments, which are common in classified settings.
**Recommendation**: Add an "Air-Gapped Deployment" section covering image distribution via media, local artifact mirrors, and operation without any network connectivity.

### 9.8 Policy Distribution Mechanism Undefined
**Gap**: Policies are "declarative and live in-repo" but there is no mechanism for an organization to enforce a baseline policy that individual developers cannot override.
**Recommendation**: Define a policy hierarchy: org baseline (immutable, signed) -> team policy (can tighten) -> project policy (can tighten further). The `aibox` CLI enforces the merge.

### 9.9 No Telemetry or Usage Metrics
**Gap**: No mechanism to understand adoption, usage patterns, or security events across the organization.
**Recommendation**: Add an optional telemetry section (respecting classified environment constraints) for aggregating anonymized usage metrics and security events.

### 9.10 Ephemeral Mode Underspecified
**Gap**: "Optional ephemeral mode (auto-wipe on exit)" is mentioned but not detailed.
**Recommendation**: Specify what "ephemeral" means: container deleted, volumes deleted, caches preserved? Define clearly what persists and what does not.

---

## 10. Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| Podman Windows bugs (WSL2 edge cases) | Medium | Medium | Maintain Docker fallback path. Automated compatibility testing. Active `aibox doctor` diagnostics. |
| Developers bypass proxy by modifying WSL2 config | Low | High | WSL2 config locked via GPO. `aibox` verifies proxy/iptables on every start. Falco alerts on network anomalies. |
| Image pull storms (200 devs pull new image Monday morning) | Medium | Low | Harbor caching + CDN layer. Stagger update notifications. P2P image distribution (Dragonfly) if needed. |
| Build cache corruption in named volumes | Low | Medium | `aibox repair cache` command to clear and rebuild. Document as expected occasional issue. |
| WSL2 filesystem performance insufficient for large Bazel builds | Medium | High | Ensure workspace on ext4 (not NTFS mount). Optimize Bazel remote cache if available. Monitor and benchmark during pilot. |
| Classified environment prohibits centralized telemetry | High | Low | Telemetry is optional. All monitoring works locally. Security events logged locally and reviewed by dev. |
| Squid proxy becomes bottleneck | Low | Medium | Squid handles thousands of concurrent connections easily. Local-only traffic, no network bottleneck. |
| Developer resistance to sandboxed workflow | Medium | High | Invest in DX during pilot. Fast startup (<30s). Pre-warmed caches. Clear value proposition (security enables AI tool usage). |

---

## Appendix A: Alternative Considered -- Kubernetes + Coder

For completeness, if the organization later decides to move to centralized infrastructure (e.g., for stronger audit controls or thin-client scenarios):

**Coder + Kubernetes architecture**:
- Coder OSS or Enterprise deployed on K8s
- Each developer gets a Kubernetes pod as their workspace
- Persistent volumes for workspace and caches
- Network policies enforce egress restrictions at the cluster level
- Coder handles IDE integration (VS Code, JetBrains Gateway)
- Templates define workspace configurations (equivalent to AI-Box images)

**When to consider this migration**:
- Organization grows beyond 500 developers
- Security requirements mandate centralized audit of all developer activity
- Thin-client deployment model is adopted (Chromebooks, locked-down desktops)
- Compliance requires centralized data-at-rest controls

The `aibox` CLI abstraction means this migration would not change the developer-facing interface.

---

## Appendix B: Quick Reference -- `aibox` CLI Commands

```bash
# Initial setup (installs Podman, Squid, configures networking)
aibox setup

# Start a sandbox for a project
aibox start --workspace ~/projects/my-service --toolpacks java@21,node@20

# Check sandbox status
aibox status

# Open shell in sandbox
aibox shell

# Update to latest image
aibox update

# Validate policy
aibox policy validate

# Explain why something was blocked
aibox policy explain --log-entry <id>

# Run diagnostics
aibox doctor

# Install additional tool pack in running sandbox
aibox install bazel@7

# Stop sandbox
aibox stop

# Network connectivity test
aibox network test
```
