---
title: "Transparent TCP Proxy for Containers via nftables DNAT"
description: "Route outbound container TCP traffic through a host-side proxy using nftables DNAT rules, without modifying the container. TCP-only by design; UDP has no working transparent-proxy path under container networking on Linux today."
tags: network-proxy, network-isolation, security, config
---

## Context

Users may need container traffic to go through a proxy (e.g., sing-box running on the host or a remote server). This is common for:

- Corporate network environments requiring proxy access
- Users in regions where direct internet access is restricted
- Security auditing of container traffic

Simply setting `HTTP_PROXY`/`HTTPS_PROXY` environment variables is insufficient because:

1. Only HTTP-aware applications respect these variables (curl, apt, etc.)
2. Arbitrary TCP connections (git+ssh, database clients, custom protocols) are not covered
3. UDP traffic (DNS, QUIC) is completely unaffected

We need a transparent proxy solution that intercepts outbound container traffic at the network layer, so applications do not need to cooperate.

## Decision: TCP-only transparent proxy

Alcatraz transparent-proxies **outbound TCP only**. UDP traffic is not intercepted and egresses via the container's normal network path.

```toml
[network]
proxy = "${alca:HOST_IP}:1080"
```

This scope is intentional, not a stopgap. The reasons UDP is excluded are rooted in how Linux composes DNAT, TPROXY, and bridge forwarding — they are not alcatraz-specific bugs to fix. Rather than ship a half-working UDP path that silently drops DNS or loops packets to itself, alcatraz ships only the parts that actually work and documents the gap.

## Why only TCP

Transparent proxying requires the proxy to recover each packet's **original destination** (pre-interception). Linux offers two mechanisms. Under container networking, one works cleanly for TCP; neither works for UDP.

### TCP path: DNAT + `SO_ORIGINAL_DST`

For TCP, `getsockopt(SO_ORIGINAL_DST)` walks the conntrack table and returns the pre-DNAT destination. Every mainstream redirect-mode proxy (sing-box, v2ray, clash) uses this. Nothing in Docker's bridge networking breaks it. DNAT + `SO_ORIGINAL_DST` is a well-trodden path.

### UDP path: blocked from two independent directions

1. **DNAT + UDP defeats `IP_RECVORIGDSTADDR`.** The standard UDP destination-recovery API reads the packet's IP header directly. nftables DNAT rewrites that header before the socket sees it, so the proxy gets its own address back as the "original" destination — useless. Every mainstream transparent proxy we examined (sing-box, v2ray, clash) uses `IP_RECVORIGDSTADDR`, so DNAT'ing UDP effectively breaks them all.

2. **TPROXY — the destination-preserving alternative — doesn't deliver packets across Linux bridges.** TPROXY steals packets to a local `IP_TRANSPARENT` listener without modifying the IP header, so `IP_RECVORIGDSTADDR` would read the correct destination. But when a packet traverses a Linux bridge with `br_netfilter` enabled (the topology Docker, OrbStack, and Docker Desktop universally use), the TPROXY expression fires but the socket lookup silently fails and the packet continues through bridge forwarding. Verified empirically on kernel 6.17 by comparing a direct-veth topology (TPROXY delivers to the listener) against an otherwise-identical bridge topology (TPROXY does not deliver). This is a kernel-level composition issue between bridge forwarding and TPROXY's socket delivery with no configuration-level workaround we could find.

Everything else we considered has its own blocker — see Alternatives B, C, E, F below. None is obviously worth taking on *today*, so we scope down to TCP and record the UDP work as open.

## Alternatives Considered

### A. nftables DNAT (host/VM side), TCP-only — **chosen**

Redirect outbound container TCP to a proxy address using nftables DNAT rules in the nat table.

**Pros:**
- Zero container invasiveness — no capabilities, no modifications inside the container
- Reuses existing nftables infrastructure (network helper, macOS/Linux dual-platform support)
- Orthogonal to existing lan-access isolation rules (separate nftables table)
- `${alca:HOST_IP}` token already available for portable config

**Cons:**
- TCP only — users needing UDP proxying must handle UDP out-of-band (DoT/DoH resolvers, disabling QUIC, or SOCKS5 UDP ASSOCIATE for specific applications)
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
- No bridge between container and proxy (conceptually sidesteps the bridge-TPROXY issue)
- Clean image separation

**Cons:**
- For outbound packets originating in the shared netns, the netfilter hook is `output`, not `prerouting`. TPROXY is only defined at `prerouting`, so it cannot catch locally-originated traffic — the shared-netns trick defeats itself
- Falling back to DNAT in `output` inherits the `IP_RECVORIGDSTADDR` problem from "Why only TCP"
- Adds sidecar-lifecycle complexity without solving UDP

**Rejected** — gains no capability over Alternative A for TCP, and still cannot deliver UDP.

### D. TPROXY (Transparent Proxy kernel mechanism)

Use nftables TPROXY instead of DNAT. TPROXY delivers packets to a local proxy without modifying the destination address, so the proxy reads the original destination directly from the packet header — no conntrack lookup needed.

**Pros:**
- Would make UDP transparent proxying viable if it worked in the target environment

**Cons:**
- Does not deliver packets to the local listener when traffic traverses a Linux bridge with `br_netfilter` enabled (see "Why only TCP" → point 2). Container runtimes universally place containers on a bridge, so TPROXY is a no-op for the traffic we care about

**Rejected** — the bridge + TPROXY composition issue blocks this for every realistic alcatraz deployment. We tested `meta pkttype set host` in bridge-family prerouting, multiple TPROXY syntaxes, and different listener implementations; none changed the outcome.

### E. Built-in proxy relay (embed conntrack-aware UDP forwarding in alca)

Embed a lightweight relay in alca (or the network helper container) that listens on `:port`, recovers the original UDP destination via `getsockopt(SO_ORIGINAL_DST)` (kernel 6.x supports this for UDP), and forwards to a user-configured upstream proxy.

**Pros:**
- Solves UDP even when the user's proxy doesn't support conntrack lookups itself
- No dependency on upstream proxy changes

**Cons:**
- alca becomes a data-plane participant — must handle connection pooling, timeouts, error recovery, restarts
- SOCKS5 UDP ASSOCIATE (the natural upstream protocol) is unreliable and poorly implemented by many servers
- Every container connection flows through this process — single point of failure
- Significant maintenance burden for a development tool

**Deferred** to a potential future phase. If user feedback shows that UDP proxying is a significant pain point, this can be revisited.

### F. Point-to-point veth between container and proxy sidecar (bypassing Docker's bridge)

At `alca up`, create a veth pair with one end in the alca container's netns and the other in a dedicated proxy sidecar, skipping Docker's bridge entirely. Route container egress through the veth. TPROXY on the sidecar works because no bridge is involved.

**Pros:**
- Unblocks TPROXY, which unblocks UDP
- Verified in the direct-veth topology that originally isolated the bridge issue

**Cons:**
- Requires alca to orchestrate netns plumbing outside Docker's networking model — IP allocation, routing, MASQUERADE, DNS visibility all become alca's responsibility
- Non-trivial rewrite of the container-networking story

**Deferred** — second viable future direction for UDP. Lower ongoing maintenance burden than Alternative E once in place, but higher up-front complexity.

## How It Works

Alcatraz generates nftables DNAT rules in a separate `ip` table (orthogonal to the existing `inet` isolation table). Outbound TCP traffic from the container is redirected to the configured proxy address. Key details:

- **Routing loop prevention**: traffic destined to the proxy address itself is excluded from DNAT
- **Cleanup**: proxy rules are removed alongside isolation rules on `alca down`
- **Unified interface**: isolation and proxy rules are applied in a single operation; the implementation decides how to organize nftables tables internally

## Proxy Requirement and Limitations

The user must run a **redirect-mode transparent TCP proxy** that can receive DNAT'd traffic and determine the original destination via `SO_ORIGINAL_DST` / conntrack. Example: [sing-box](https://github.com/sagernet/sing-box) with `redirect` inbound.

Alcatraz operates purely at the network layer (nftables DNAT) — it does not participate in the data plane. This means:

- **No protocol-level features**: SOCKS5 authentication, HTTP CONNECT, or other application-layer proxy protocols are not supported directly. The DNAT'd packets arrive at the proxy as raw TCP — no SOCKS5 handshake occurs
- **Workaround for authentication**: users who need authenticated proxy access should run a local redirect-mode proxy (e.g., sing-box) that handles authentication to the upstream server. The container DNATs to the local proxy, and the local proxy forwards with authentication — authentication is between the local proxy and the remote server, transparent to the container

## Consequences

- Users can transparently proxy all container TCP traffic with a single config line
- UDP is explicitly out of scope — documentation and the cookbook recipe make this visible and guide users to workarounds (DoT/DoH resolvers, disabling QUIC, per-app SOCKS5 UDP ASSOCIATE)
- The proxy can be local or remote — any address reachable from the container
- Requires a redirect-mode TCP proxy running at the configured address
- Proxy rules use a separate nftables table, orthogonal to existing network isolation
- Implementation reuses the existing nftables infrastructure (rule generation, network helper reload, platform abstraction)
- The TCP-only scope is a design decision, not a bug. Changing it requires taking on one of Alternative E, F, or a meaningful upstream change

## Future Work

UDP transparent proxying remains open. The two alcatraz-side paths we would pursue if user demand justifies the cost are Alternative **E** (embedded conntrack-aware UDP relay) and Alternative **F** (point-to-point veth bypassing Docker's bridge). A third path is purely upstream: if a mainstream proxy (sing-box et al.) adopts `getsockopt(SO_ORIGINAL_DST)` for UDP, DNAT'd UDP becomes viable with no alcatraz changes.

This AGD should be revisited when one of those paths is pursued.
