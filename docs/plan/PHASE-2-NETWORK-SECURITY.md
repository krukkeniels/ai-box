# Phase 2: Network Security

## Overview

Network controls are the primary exfiltration prevention mechanism and the core security differentiator of AI-Box. This phase implements host-level enforcement that the container cannot modify, disable, or bypass -- even if the AI agent inside is compromised via prompt injection. The architecture layers L3/L4 packet filtering (nftables), L7 domain allowlisting (Squid proxy), controlled DNS resolution (CoreDNS), an LLM API sidecar proxy for credential injection and payload logging, and package manager proxying through Nexus/Artifactory mirrors. Every layer is enforced outside the container on the host network stack.

**Spec Sections Covered**: 5.3 (Connectivity Layer), 8.1-8.6 (Network Security), 20 (Threat Model -- network-related threats).

**Dependencies**: Phase 0 (Nexus for package proxying), Phase 1 (running container to route traffic through).

---

## Deliverables

1. **nftables ruleset** installed by `aibox setup`: container traffic allowed only to Squid proxy and CoreDNS, all other egress dropped.
2. **Squid proxy configuration** with SNI-based domain allowlisting for HTTPS (Harbor, Nexus, Git server, LLM gateway), access logging, and CONNECT tunnel support.
3. **CoreDNS configuration** with allowlist-only resolution, NXDOMAIN for all non-allowlisted domains, query logging, and Prometheus metrics.
4. **DNS tunneling mitigations**: block TXT/NULL/CNAME record types, rate-limit queries per source, monitor for high subdomain entropy.
5. **LLM API sidecar proxy** (`aibox-llm-proxy`): credential injection from Vault-mounted secret, full request/response payload logging, rate limiting (60 req/min, 100K tokens/min), payload size limits, sandbox identity headers.
6. **Package manager proxy configuration**: npm, Maven, pip, NuGet, Go modules, Cargo, and PowerShell Gallery (PSGallery) routed through Nexus via Squid.
7. **`aibox network test` CLI command** for developer-facing connectivity verification.
8. **Anti-bypass protections**: block DNS-over-HTTPS (DoH) to known resolver IPs, block DNS-over-TLS (DoT) on port 853, block direct IP egress.
9. **Integration with `aibox setup`**: all host-level components (nftables, Squid, CoreDNS) provisioned and verified automatically.
10. **Documentation**: network architecture diagram, proxy configuration reference, troubleshooting guide for network-related issues.

---

## Implementation Steps

### Work Stream 1: nftables Host Rules

**What to build**: A host-level nftables ruleset that confines all container network traffic to two permitted destinations (Squid proxy and CoreDNS), dropping everything else. This is the foundational enforcement layer -- even if Squid or CoreDNS are misconfigured, raw IP egress is blocked.

**Key tasks**:
- Write an nftables ruleset that:
  - Drops all forwarded traffic from container interfaces (`podman*` or `pasta` interfaces) by default.
  - Allows container traffic to the Squid proxy IP on TCP port 3128.
  - Allows container traffic to the CoreDNS IP on UDP port 53.
  - Explicitly drops UDP/TCP port 53 to all other destinations (prevents DNS bypass).
  - Explicitly drops TCP port 853 (blocks DNS-over-TLS).
  - Explicitly drops HTTPS (TCP 443) to known DoH resolver IPs (Google 8.8.8.8, Cloudflare 1.1.1.1, Quad9 9.9.9.9, etc.).
  - Logs dropped packets for audit (rate-limited to prevent log flooding).
- Integrate the ruleset into `aibox setup` so it is applied on host boot and survives reboots (systemd unit or nftables persistence).
- Handle the rootless Podman + `pasta` networking case: `pasta` uses a different network topology than bridged networking. The nftables rules must account for `pasta`'s use of the host network namespace with mapped ports. Research is needed here (see Research section).
- Write a verification script (`aibox network test` component) that confirms the rules are active and correctly blocking direct egress.
- Ensure rules do not interfere with non-AI-Box container traffic or other host services.

**Key config decisions**:
- Rules are owned by `aibox` and stored in `/etc/aibox/nftables.conf`. They are loaded via a dedicated nftables include or a systemd service.
- The proxy IP and DNS IP are configurable (default to `127.0.0.1` if Squid and CoreDNS run on localhost).
- Rules must be idempotent -- `aibox setup` can be run multiple times without duplicating rules.

**Spec references**: Section 8.2 (nftables Rules), Section 8.1 (Layered Architecture table, L3/L4 row).

---

### Work Stream 2: Squid Proxy

**What to build**: A Squid 6.x forward proxy running on the host that enforces a domain allowlist for all HTTP/HTTPS traffic originating from AI-Box containers. This is the L7 enforcement layer.

**Key tasks**:
- Install and configure Squid 6.x on the host (or in a dedicated host-network container managed by `aibox`).
- Configure the domain allowlist ACL:
  - `harbor.internal` (image registry)
  - `nexus.internal` (package mirrors)
  - `foundry.internal` (LLM gateway)
  - `git.internal` (source repos, policy repo)
  - Additional domains configurable via policy (team/project allowlists merged with tighten-only semantics).
- Configure HTTPS handling via `CONNECT` method with SNI-based filtering. **No TLS interception** (MITM) -- the spec explicitly calls this out as prohibited by default in classified environments. Squid inspects the SNI field in the TLS ClientHello to match against the allowlist.
- Configure `http_access deny all` as the final rule (default-deny).
- Enable structured access logging to `/var/log/aibox/proxy-access.log` with fields: timestamp, source IP, destination domain, HTTP method, status code, bytes transferred.
- Configure Squid to listen on `127.0.0.1:3128` (localhost only -- container reaches it via the `pasta`/bridge network path allowed by nftables).
- Set `aibox` container environment variables `http_proxy`, `https_proxy`, and `no_proxy` to route all traffic through Squid.
- Write health check for `aibox doctor`: verify Squid is running, listening on 3128, and responding to a test request against an allowlisted domain.

**Key config decisions**:
- **SNI-based filtering, not MITM**: This means Squid cannot inspect HTTP request bodies over HTTPS. It can only see the destination domain. This is a deliberate tradeoff -- MITM breaks certificate pinning, adds latency, and is often prohibited in classified environments.
- **Squid runs on the host network**: Not inside the container. The container cannot kill, reconfigure, or bypass it.
- **Allowlist is sourced from policy**: The base allowlist comes from the org baseline policy. Team and project policies can add domains (subject to tighten-only merge -- they cannot remove org-level entries). `aibox` reads the merged policy and generates `squid.conf` at setup/start time.
- **No caching by default**: Squid is used as a filtering proxy, not a caching proxy. Nexus handles package caching. Enabling Squid caching is a future optimization.

**Spec references**: Section 8.3 (Squid Proxy Configuration), Section 8.1 (L7 row).

---

### Work Stream 3: CoreDNS

**What to build**: A CoreDNS instance running on the host that resolves only allowlisted domains and returns NXDOMAIN for everything else. This prevents DNS-based data exfiltration and complements the Squid proxy.

**Key tasks**:
- Install CoreDNS on the host (binary or host-network container managed by `aibox`).
- Write a Corefile that:
  - Defines a `hosts` block mapping allowlisted internal domains to their IPs (`harbor.internal`, `nexus.internal`, `foundry.internal`, `git.internal`).
  - Uses a `forward` plugin for allowlisted domains that need upstream DNS resolution (if internal domains are not static-IP).
  - Uses a `template` plugin to return NXDOMAIN for all non-matching queries.
  - Enables the `log` plugin for all queries.
  - Enables the `prometheus` plugin on `:9153` for metrics.
- Implement DNS tunneling detection and mitigation:
  - Block (or log and alert) queries for TXT, NULL, and CNAME record types unless specifically needed by an allowlisted domain.
  - Rate-limit DNS queries per source IP (e.g., 100 queries/minute). Legitimate development generates predictable DNS patterns; tunneling generates high volume.
  - Monitor for high subdomain entropy (long random-looking subdomain labels are a tunneling signature). This can be a separate detection script or CoreDNS plugin.
- Configure the container to use CoreDNS as its only DNS resolver (`--dns=<coredns_ip>` in Podman).
- Write health check for `aibox doctor`: verify CoreDNS is running, resolves an allowlisted domain, and returns NXDOMAIN for a non-allowlisted domain.

**Key config decisions**:
- **Static hosts vs. forward**: For internal infrastructure with known IPs, use the `hosts` plugin (no upstream dependency). For domains that require dynamic resolution, use `forward` to the organization's internal DNS -- but only for explicitly listed domains.
- **CoreDNS runs on localhost**: Listening on `127.0.0.1:53` (or a dedicated address). The container is configured to use this as its sole resolver.
- **Allowlist sourced from policy**: Same as Squid -- the domain list comes from the merged policy. CoreDNS Corefile is regenerated when policy changes.
- **No recursive resolution for unknown domains**: The `template` catch-all returns NXDOMAIN immediately. There is no fallback to public DNS.

**Spec references**: Section 8.4 (DNS Control), Section 20.1 (DNS tunneling threat).

---

### Work Stream 4: LLM Sidecar Proxy

**What to build**: A lightweight reverse proxy (`aibox-llm-proxy`) that runs inside the container, listens on `localhost:8443`, injects API credentials, logs full payloads, enforces rate limits, and forwards requests to the LLM gateway via the Squid egress proxy.

**Key tasks**:
- Build `aibox-llm-proxy` as a standalone binary (see Research section for language choice).
- Implement credential injection:
  - Read API key from a Vault-mounted secret file (e.g., `/run/secrets/llm-api-key`).
  - Strip any `Authorization` header the agent might include.
  - Inject the correct `Authorization: Bearer <key>` header on outbound requests.
  - The agent talks to `localhost:8443` with no credentials -- it never sees the key.
- Implement full payload logging:
  - Log complete request bodies (prompts, code context sent to LLM).
  - Log complete response bodies (LLM completions).
  - Log to a structured format (JSON lines) at `/var/log/aibox/llm-payloads.jsonl`.
  - Include metadata: timestamp, sandbox ID, user identity, model, token count, request size, response size.
  - This log is shipped to the immutable audit store by the Phase 5 log pipeline.
- Implement rate limiting:
  - 60 requests per minute (configurable via policy).
  - 100,000 tokens per minute (configurable via policy).
  - Return HTTP 429 with a clear error message when limits are exceeded.
- Implement payload size limits:
  - Maximum request body size (configurable, e.g., 1MB default).
  - Maximum response body size for logging (truncate very large responses in the log, but pass them through).
- Add sandbox identity headers to outbound requests:
  - `X-AIBox-Sandbox-ID`: unique sandbox identifier.
  - `X-AIBox-User`: authenticated user identity.
  - These enable audit correlation at the LLM gateway.
- Configure the proxy to egress through Squid (`https_proxy=http://host:3128`).
- Configure AI tool environment variables inside the container:
  - `ANTHROPIC_BASE_URL=http://localhost:8443` (for Claude Code).
  - `OPENAI_BASE_URL=http://localhost:8443` (for Codex CLI).
  - No `ANTHROPIC_API_KEY` or `OPENAI_API_KEY` set -- the sidecar handles auth.
- Integrate the proxy startup into the container entrypoint (managed by `aibox-agent` or the container init process).

**Key config decisions**:
- **Runs inside the container, not on the host**: The sidecar is inside the container because it needs access to Vault-mounted secrets and must present as `localhost` to the AI agent. However, it cannot egress except through the host Squid proxy (enforced by nftables). A compromised agent could kill the sidecar, but that would only break its own LLM access -- it cannot bypass network controls.
- **Transparent to the AI agent**: Agents are configured to point at `localhost:8443` instead of the upstream API. No agent code changes required.
- **Payload logging is append-only inside the container**: The log file is on a dedicated volume or tmpfs. The agent process does not have write access to the log directory (only the sidecar process does).

**Spec references**: Section 8.5 (LLM API Sidecar Proxy), Section 11.5 (LLM API Key Injection), Section 14.1/14.2 (AI tool integration).

---

### Work Stream 5: Package Manager Proxying

**What to build**: Configuration that routes all package manager traffic (npm, Maven, pip, NuGet, Go modules, Cargo) through the Squid proxy to Nexus/Artifactory mirrors. Direct access to upstream public registries is blocked.

**Key tasks**:
- Configure package manager registry URLs inside the container to point at Nexus mirrors:
  - npm: `.npmrc` with `registry=https://nexus.internal/repository/npm-proxy/`
  - Maven: `settings.xml` with Nexus mirror for Maven Central and Gradle Plugin Portal.
  - pip: `pip.conf` with `index-url=https://nexus.internal/repository/pypi-proxy/simple/`
  - NuGet: `NuGet.Config` with Nexus source. **Note**: `dotnet restore` has specific proxy/feed configuration requirements -- validate feed discovery, authentication, and package resolution through the Nexus proxy.
  - Go modules: `GOPROXY=https://nexus.internal/repository/go-proxy/`
  - Cargo: `.cargo/config.toml` with Nexus registry.
  - PowerShell Gallery (PSGallery): Configure `Register-PSRepository` with Nexus mirror if PowerShell modules need to be installed inside the sandbox.
- Bake these configurations into the base image (`/etc/aibox/package-manager-configs/`).
- Ensure `aibox setup` verifies that Nexus mirrors are accessible through the Squid proxy.
- Verify that `npm install`, `gradle build`, `pip install`, `dotnet restore`, etc. all succeed through the proxy chain.
- Document how teams can request additional registry mirrors (e.g., a team-specific internal registry).

**Key config decisions**:
- **No direct upstream access**: The Squid allowlist does not include `registry.npmjs.org`, `repo1.maven.org`, `pypi.org`, etc. All traffic goes to `nexus.internal`, which in turn proxies/caches from upstream.
- **Nexus handles vulnerability scanning and license checks**: This is configured in Phase 0 as part of Nexus setup. Phase 2 ensures traffic flows correctly.
- **Config is baked into the image, not injected at runtime**: This ensures package managers work even if the config injection mechanism fails. The image always points at the internal mirrors.

**Spec references**: Section 8.6 (Package Manager Proxying).

---

### Work Stream 6: Anti-Bypass Protections

**What to build**: Additional hardening to close known bypass vectors for the network security stack.

**Key tasks**:
- **Block DNS-over-HTTPS (DoH)**:
  - Identify known DoH resolver IPs (Google, Cloudflare, Quad9, Mozilla, etc.) and add explicit nftables drop rules for TCP 443 to those IPs.
  - Maintain a curated list of DoH resolver IPs in `/etc/aibox/doh-blocklist.txt`, updatable via policy repo.
  - Alternatively, since the Squid allowlist blocks all non-approved HTTPS destinations, DoH is inherently blocked at L7. The nftables rules are defense-in-depth.
- **Block DNS-over-TLS (DoT)**:
  - nftables rule: drop all TCP port 853 from container interfaces (already in the base ruleset).
- **Block direct IP access**:
  - Ensure nftables drops all traffic that is not destined for the proxy or DNS. Even if someone hardcodes an IP instead of a domain, the traffic is blocked at L3/L4.
- **Block ICMP tunneling**:
  - Drop ICMP from container interfaces (nftables). ICMP is not needed for development workflows.
- **Prevent eBPF-based bypass**:
  - The seccomp profile from Phase 1 blocks the `bpf()` syscall. Verify this is in place.
- **Prevent raw socket creation**:
  - `--cap-drop=ALL` removes `CAP_NET_RAW`. Verify no capability grants it back.
- **Verify proxy cannot be bypassed by changing env vars**:
  - Even if the agent unsets `http_proxy`/`https_proxy`, the nftables rules block direct egress. The proxy env vars are a convenience, not a security control.
- **Write bypass test suite**:
  - A set of automated tests that attempt every known bypass vector from inside the container and verify they all fail:
    - `curl --noproxy '*' https://example.com` (direct HTTPS)
    - Direct DNS query to `8.8.8.8`
    - DoH request to `https://dns.google/resolve?name=example.com`
    - DoT request to `dns.google:853`
    - ICMP ping to an external host
    - Raw socket creation attempt
    - High-entropy subdomain DNS queries (tunneling simulation)

**Key config decisions**:
- **Defense-in-depth**: Multiple layers prevent the same attack. Even if one layer fails, others catch it.
- **Bypass test suite is a release gate**: No release of AI-Box ships without the bypass test suite passing.

**Spec references**: Section 8.2 (nftables rules blocking DoT), Section 8.4 (DNS tunneling mitigations), Section 20.1 (Threat Model -- DNS tunneling, prompt injection exfiltration).

---

## Research Required

### 1. Rootless Podman + pasta Networking + nftables Interaction

**Why**: Rootless Podman uses `pasta` (or `slirp4netns`) for networking instead of a traditional bridge. `pasta` maps container ports into the host network namespace differently than bridged networking. The nftables rules in the spec assume bridge-style interfaces (`podman*`), which may not exist with `pasta`.

**Questions to answer**:
- How does `pasta` expose container traffic on the host? Does it use a tap interface, port mapping, or something else?
- Can nftables filter `pasta`-routed traffic? If so, which interface or chain do we filter on?
- Are there `pasta` CLI options that constrain egress natively (e.g., `--map-gw`, `--dns-forward`)?
- Should we use `pasta` options for DNS and proxy routing instead of (or in addition to) nftables?
- Does the answer change between Podman 4.x and 5.x?

**Approach**: Set up a test environment with rootless Podman + `pasta`, trace traffic with `tcpdump`, and document the network topology. Prototype nftables rules that work with `pasta`.

---

### 2. Squid SNI-Based Filtering Behavior

**Why**: Squid's `CONNECT` method with SNI filtering is the primary HTTPS allowlisting mechanism. We need to understand its limitations and edge cases.

**Questions to answer**:
- Does Squid reliably extract SNI from the TLS ClientHello for all common TLS libraries? Are there edge cases (TLS 1.3 ECH, ESNI)?
- What happens if a client does not send SNI (e.g., using an IP address directly)? Squid should deny, but verify.
- How does Squid handle wildcard domain matching in ACLs (e.g., `.internal` to match all subdomains)?
- What is the performance overhead of SNI inspection at the expected request volume (~200 developers, moderate traffic)?
- Does Squid correctly handle HTTP/2 and HTTP/3 (QUIC) through CONNECT tunnels?

**Approach**: Set up Squid 6.x, test with various TLS clients (curl, Node.js, Java HttpClient, Python requests), and document behavior for each edge case.

---

### 3. CoreDNS DNS Tunneling Detection

**Why**: DNS tunneling is a well-known exfiltration technique that encodes data in DNS queries. CoreDNS's built-in plugins may not be sufficient for detection.

**Questions to answer**:
- Does CoreDNS have a built-in plugin for entropy-based query detection, or do we need a custom plugin or external tool?
- What is the false positive rate for entropy-based detection in a development environment (long package names, CDN domains, etc.)? Note: since we use allowlist-only resolution, tunneling queries would be for subdomains of allowlisted domains (e.g., `exfiltrated-data.nexus.internal`).
- Is rate limiting per-source sufficient, or do we need per-domain rate limiting?
- Should we use an external tool (e.g., `dnsmonster`, `passivedns`) for deep DNS analytics instead of building detection into CoreDNS?

**Approach**: Review CoreDNS plugin ecosystem, test with simulated DNS tunneling tools (e.g., `iodine`, `dnscat2`), and evaluate detection accuracy.

---

### 4. LLM Sidecar Proxy Implementation Approach

**Why**: The sidecar proxy is a new component that must be lightweight, reliable, and secure. The implementation language and approach affects performance, binary size, and maintainability.

**Questions to answer**:
- **Go** (pro: matches gVisor/Podman ecosystem, fast HTTP handling, small binary, easy cross-compile; con: GC pauses, larger than Rust binary)
- **Rust** (pro: smallest binary, no GC, best performance; con: harder to maintain for a platform team that may not have Rust expertise)
- **nginx + Lua** (pro: proven proxy, extensible; con: complex config, Lua scripting for credential injection is fragile, harder to do structured payload logging)
- **Envoy** (pro: mature proxy with rich filtering; con: heavy for a sidecar, complex configuration, overkill for this use case)
- What is the expected request/response size for LLM API calls? (Affects memory usage for payload logging.)
- Should the sidecar stream responses (SSE) or buffer them? LLM APIs commonly use streaming -- the proxy must handle this correctly.

**Approach**: Build a minimal prototype in Go (most likely choice given ecosystem alignment). Benchmark latency overhead. Test with Claude Code and Codex CLI streaming responses.

---

## Open Questions

**Q1**: How does rootless Podman `pasta` networking interact with nftables? Can we filter container egress at the host level, or do we need `pasta`-native options? -- *Who should answer*: Platform engineering (hands-on testing required)

**Q2**: Should we run Squid and CoreDNS as host-level systemd services or as Podman containers on the host network? Containers are easier to manage/update but add a dependency on Podman for host-level security infrastructure. -- *Who should answer*: Platform engineering + security team

**Q3**: Should the LLM payload logs include full response bodies (which can be very large for long completions), or should we log hashes/summaries above a size threshold? Full logging provides maximum forensic value but consumes significant storage. -- *Who should answer*: Security team + compliance

**Q4**: How do we handle the case where a developer needs temporary access to a domain not on the allowlist (e.g., a new internal API)? What is the self-service request and approval flow? -- *Who should answer*: Security team + platform engineering

**Q5**: Should QUIC (HTTP/3, UDP 443) be explicitly blocked in nftables? Squid does not support QUIC proxying, so QUIC traffic would bypass the proxy. -- *Who should answer*: Security team

---

## Dependencies

| Dependency | Source Phase | What is Needed | Impact if Delayed |
|-----------|-------------|----------------|-------------------|
| Harbor registry operational | Phase 0 | Container images pullable with signature verification | Cannot test container networking |
| Nexus repository operational | Phase 0 | Package mirrors for npm, Maven, PyPI, etc. | Package manager proxying cannot be tested |
| `aibox` CLI with `setup` and `start` commands | Phase 1 | Running container to route traffic through | No container traffic to filter |
| Podman + gVisor container runtime | Phase 1 | Working container with correct network namespace | Cannot test nftables rules |
| Seccomp profile blocking `bpf()` syscall | Phase 1 | Prevents eBPF-based network bypass | Anti-bypass protection gap |
| `--cap-drop=ALL` enforced | Phase 1 | No `CAP_NET_RAW` for raw sockets | Anti-bypass protection gap |
| Policy file format defined | Phase 1/3 | Domain allowlist sourced from policy | Squid/CoreDNS config cannot be dynamically generated |

---

## Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|-----------|--------|-----------|
| R1 | `pasta` networking incompatible with nftables-based filtering | Medium | High | Research `pasta` network topology early (week 1). If nftables cannot filter `pasta` traffic, use `pasta`'s built-in `--dns-forward` and `--map-gw` options, or switch to bridge networking with `slirp4netns`. |
| R2 | Squid SNI filtering fails for TLS 1.3 ECH connections | Low | Medium | ECH is not widely deployed on internal infrastructure. Block ECH-capable connections as a compensating control. Monitor TLS ecosystem for adoption timeline. |
| R3 | False-positive DNS blocks break developer workflows | High | Medium | Start with a generous allowlist. Provide clear error messages (`aibox policy explain`). Fast allowlist request turnaround (<1 business day). Log all NXDOMAIN responses for analysis. |
| R4 | LLM sidecar proxy adds >50ms latency | Low | High | Sidecar is localhost-only (no network hop). Benchmark early with prototype. If latency is an issue, optimize hot path (credential injection + header rewrite is minimal work). Defer payload logging to async write. |
| R5 | Payload logging consumes excessive disk for LLM responses | Medium | Medium | Implement configurable size limits for logged payloads. Truncate response bodies above threshold in log (but still pass full response to agent). Rotate logs aggressively; ship to central store promptly. |
| R6 | Package manager proxy configuration conflicts with developer-customized configs | Medium | Low | AI-Box package manager configs are baked into the image at system level (e.g., `/etc/npmrc`). Document that user-level overrides in `~/.npmrc` are supported but must still point at `nexus.internal`. |
| R7 | QUIC (HTTP/3) bypasses Squid proxy | Medium | Medium | Explicitly block UDP port 443 in nftables. Squid does not support QUIC. All HTTPS must go through TCP-based CONNECT. |
| R8 | Network controls break `aibox doctor` or other host-to-container communication | Low | Medium | Ensure nftables rules only apply to container-originated traffic, not host-to-container traffic. Test `aibox shell`, `aibox doctor`, and SSH connections after rules are applied. |

---

## Exit Criteria

1. **Egress blocked**: From inside the container, `curl https://google.com` and `curl -k https://1.2.3.4` both fail (connection refused/timed out).
2. **Allowlisted egress works**: From inside the container, `git clone` from `git.internal` succeeds. `curl https://nexus.internal/service/rest/v1/status` returns 200.
3. **DNS controlled**: `dig example.com` returns NXDOMAIN. `dig harbor.internal` returns the correct IP.
4. **LLM API functional**: Claude Code / Codex CLI can make API calls via the sidecar proxy. The agent process cannot read the API key from the environment or filesystem.
5. **Payload logging works**: LLM API request and response bodies are captured in `/var/log/aibox/llm-payloads.jsonl` with correct metadata.
6. **Rate limiting works**: Exceeding 60 requests/minute to the LLM API returns HTTP 429.
7. **Package managers work**: `npm install`, `gradle build` (with dependencies), `pip install`, and `dotnet restore` succeed through the Nexus mirror. NuGet feed discovery and package resolution verified through proxy.
8. **nftables persistent**: Rules survive container restart and host reboot. Rules cannot be modified from inside the container.
9. **Bypass test suite passes**: All bypass vectors tested (direct HTTPS, direct DNS, DoH, DoT, ICMP, raw sockets, env var unsetting) are blocked.
10. **`aibox network test` reports all-green**: Proxy reachable, DNS resolver responding, allowlisted domains accessible, blocked domains inaccessible.
11. **`aibox doctor` network checks pass**: Squid health, CoreDNS health, nftables active.

---

## Estimated Effort

| Work Stream | Effort | Notes |
|-------------|--------|-------|
| nftables Host Rules | 1 engineer-week | Includes `pasta` research and testing |
| Squid Proxy | 1 engineer-week | Configuration, SNI testing, integration |
| CoreDNS | 1 engineer-week | Configuration, tunneling mitigations, integration |
| LLM Sidecar Proxy | 1.5-2 engineer-weeks | New component; prototype, build, test with streaming |
| Package Manager Proxying | 0.5 engineer-week | Mostly configuration; verify each package manager |
| Anti-Bypass Protections | 0.5-1 engineer-week | Includes writing the bypass test suite |
| Integration & Testing | 0.5 engineer-week | End-to-end testing of all components together |
| **Total** | **5-6 engineer-weeks** | **2 engineers, calendar weeks 5-8** |

**Team composition**: 2 engineers. One focuses on host-level infrastructure (nftables, Squid, CoreDNS). The other focuses on the LLM sidecar proxy and package manager configuration. Both collaborate on anti-bypass protections and integration testing.
