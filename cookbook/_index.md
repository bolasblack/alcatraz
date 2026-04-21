---
title: Cookbook
weight: 4
bookCollapseSection: true
---

# Cookbook

Ready-to-adapt recipes for common Alcatraz setups. Each recipe is a complete, working example — copy it, tweak the bits marked `YOUR_...`, and go.

## Recipes

- [Transparent TCP Proxy with sing-box](./transparent-proxy-sing-box.md) — route all container **TCP** traffic through [sing-box](https://github.com/sagernet/sing-box) running as a sidecar, forwarding to your upstream SOCKS5 / VMess / Shadowsocks / … server. UDP support is not available — see [AGD-037](https://github.com/bolasblack/alcatraz/blob/master/.agents/decisions/AGD-037_transparent-proxy-for-containers.md).
