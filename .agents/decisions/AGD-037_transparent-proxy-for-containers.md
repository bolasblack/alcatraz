---
title: "Transparent Proxy for Containers via nftables DNAT"
description: "Route all container traffic (TCP+UDP) through a host-side proxy using nftables DNAT rules, without modifying the container"
tags: network-proxy, network-isolation, security, config
---

## Context

Users may need all container traffic to go through a proxy (e.g., sing-box running on the host or a remote server). This is common for:

- Corporate network environments requiring proxy access
- Users in regions where direct internet access is restricted
- Security auditing of container traffic

Simply setting `HTTP_PROXY`/`HTTPS_PROXY` environment variables is insufficient because:

1. Only HTTP-aware applications respect these variables (curl, apt, etc.)
2. Arbitrary TCP connections (git+ssh, database clients, custom protocols) are not covered
3. UDP traffic (DNS, QUIC) is completely unaffected

We need a transparent proxy solution that intercepts **all** outbound traffic from the container.

## Alternatives Considered

### A. nftables DNAT (host/VM side) — **chosen**

Redirect all container outbound traffic to a proxy address using nftables DNAT rules in the nat table.

**Pros:**
- Zero container invasiveness — no capabilities, no modifications inside the container
- Reuses existing nftables infrastructure (network helper, macOS/Linux dual-platform support)
- Orthogonal to existing lan-access isolation rules (separate nftables table)
- `${alca:HOST_IP}` token already available for portable config

**Cons:**
- UDP relies on conntrack for original destination lookup (mitigated by ct timeout, see Decision)
- Proxy must support redirect/transparent mode (e.g., sing-box `redirect` inbound)

### B. tun2socks / sing-box inside the container

Run a tun-based proxy client inside the container that captures all traffic via a tun device.

**Pros:**
- Perfect TCP+UDP coverage via tun device
- No reliance on conntrack

**Cons:**
- Requires `NET_ADMIN` capability and `/dev/net/tun` — weakens container security model
- Invasive to the container environment
- Requires embedding or installing additional binaries inside the container

**Rejected** because it contradicts alca's principle of minimal container privileges and zero container modification.

### C. Sidecar proxy container with `--network container:`

Share network namespace with a dedicated proxy container running sing-box.

**Pros:**
- Clean separation of concerns
- Full protocol support

**Cons:**
- Requires managing additional container lifecycle (create, start, stop, health check)
- Significantly increases orchestration complexity
- Network namespace sharing complicates port mappings

**Rejected** due to excessive orchestration complexity for a development tool.

### D. TPROXY (Transparent Proxy kernel mechanism)

Use nftables TPROXY instead of DNAT. TPROXY delivers packets to a local proxy without modifying the destination address, so the proxy reads the original destination directly from the packet header — no conntrack lookup needed.

**Pros:**
- Perfect UDP handling — no conntrack dependency at all
- More reliable than DNAT for edge cases

**Cons:**
- Hard constraint: proxy must run in the **same network namespace** as the nftables rules
- On macOS, nftables rules are inside the VM but the user's proxy runs on macOS — different namespaces, so TPROXY is impossible
- On Linux it would work, but cross-platform inconsistency is unacceptable

**Rejected** because it cannot work on macOS (the primary platform for alca).

### E. Built-in proxy relay (embed SOCKS5 forwarding in alca)

Embed a lightweight transparent-to-SOCKS5 converter in alca (or the network helper container), so users can directly configure `proxy = "socks5://user:pass@remote:1080"` without running a local proxy.

**Pros:**
- Best user experience — single config line, no external dependencies

**Cons:**
- alca becomes a data-plane participant — must handle connection pooling, timeouts, error recovery, restarts
- SOCKS5 UDP ASSOCIATE is unreliable and poorly implemented by many servers
- Limited protocol support (SOCKS5, maybe HTTP CONNECT) vs. sing-box's dozens of protocols
- Every container connection flows through this process — single point of failure
- Significant maintenance burden for a development tool

**Deferred** to a potential future phase. If user feedback shows that requiring a local proxy is a significant pain point, this can be revisited. A practical approach would be running sing-box inside the network helper container rather than writing custom forwarding logic.

## Decision

### Configuration

Add a `proxy` field to the `[network]` section:

```toml
[network]
proxy = "${alca:HOST_IP}:1080"
```

- **Type**: string (host:port address)
- **Required**: No
- **Default**: none (no proxying)
- Supports `${alca:HOST_IP}` token for portable config
- The proxy can run anywhere the container can reach — on the host, on a LAN server, or on a remote machine. `${alca:HOST_IP}` is a convenience for the common case of a local proxy
- No protocol type configuration — all traffic (TCP+UDP) is proxied by default. Users who need protocol-level control can handle it on the proxy side

### How It Works

Alcatraz generates nftables DNAT rules in a separate `ip` table (orthogonal to the existing `inet` isolation table). All outbound TCP and UDP traffic from the container is redirected to the configured proxy address. Key details:

- **Routing loop prevention**: traffic destined to the proxy address itself is excluded from DNAT
- **UDP reliability**: a `ct timeout` object extends UDP conntrack lifetime to 300 seconds, preventing conntrack expiry for long-idle UDP flows (the default 30s could cause the proxy to lose the original destination)
- **Cleanup**: proxy rules are removed alongside isolation rules on `alca down`
- **Unified interface**: isolation and proxy rules are applied in a single operation; the implementation decides how to organize nftables tables internally

### Proxy Requirement and Limitations

The user must run a **redirect-mode transparent proxy** that can receive DNAT'd traffic and determine the original destination via `SO_ORIGINAL_DST` / conntrack. Example: [sing-box](https://github.com/sagernet/sing-box) with `redirect` inbound.

Alcatraz operates purely at the network layer (nftables DNAT) — it does not participate in the data plane. This means:

- **No protocol-level features**: SOCKS5 authentication, HTTP CONNECT, or other application-layer proxy protocols are not supported directly. The DNAT'd packets arrive at the proxy as raw TCP/UDP — no SOCKS5 handshake occurs
- **Workaround for authentication**: users who need authenticated proxy access should run a local redirect-mode proxy (e.g., sing-box) that handles authentication to the upstream server. The container DNATs to the local proxy, and the local proxy forwards with authentication — authentication is between the local proxy and the remote server, transparent to the container
- **Future**: a built-in proxy relay may be added if user feedback warrants it (see Alternative E)

### No Protocol Type Configuration

All traffic (TCP+UDP) is proxied by default. Rationale:

- The common intent is "all traffic through proxy"
- Protocol-level filtering can be handled on the proxy side
- Adding a `protocols` field later is not a breaking change if needed

## Consequences

- Users can transparently proxy all container traffic with a single config line
- The proxy can be local or remote — any address reachable from the container
- Requires a redirect-mode proxy running at the configured address
- Proxy rules use a separate nftables table, orthogonal to existing network isolation
- Implementation reuses the existing nftables infrastructure (rule generation, network helper reload, platform abstraction)
- Future: built-in proxy relay may be added if user feedback warrants it (Alternative E)
