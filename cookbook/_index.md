---
title: Cookbook
weight: 4
bookCollapseSection: true
---

# Cookbook

Ready-to-adapt recipes for common Alcatraz setups. Each recipe is a complete, working example — copy it, tweak the bits marked `YOUR_...`, and go.

## Recipes

- [Transparent TCP Proxy with sing-box](./transparent-proxy-sing-box.md) — route all container **TCP** traffic through [sing-box](https://github.com/sagernet/sing-box) running as a sidecar, forwarding to your upstream SOCKS5 / VMess / Shadowsocks / … server. First-class Alcatraz integration via [`network.proxy`](../config/fields.md#networkproxy). UDP not covered.
- [Transparent TCP+UDP Proxy with sing-box (sidecar TUN)](./transparent-tcp-udp-proxy-sing-box.md) — route **both TCP and UDP** through sing-box by attaching it as a TUN sidecar that shares the alca container's network namespace. Trades some per-packet overhead and a little setup for full UDP coverage. Pick this when you need DNS over UDP, QUIC/HTTP3, or anything else UDP — see [AGD-037](https://github.com/bolasblack/alcatraz/blob/master/.agents/decisions/AGD-037_transparent-proxy-for-containers.md) for why it's a separate recipe.
