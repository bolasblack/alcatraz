---
title: Transparent TCP Proxy with sing-box
weight: 4.1
---

# Transparent TCP Proxy with sing-box

Route every outbound **TCP** byte the container emits — any protocol, any port — through a [sing-box](https://github.com/sagernet/sing-box) instance that you control, which in turn forwards to your real upstream proxy (SOCKS5, VMess, Shadowsocks, Trojan, …).

## Scope: TCP only

[`network.proxy`](../config/fields.md#networkproxy) only proxies **TCP**. UDP traffic (DNS, QUIC, anything else) goes out via the container's normal network path, not through sing-box. The *why* is non-obvious — see [Transparent Proxy](../config/network.md#transparent-proxy) in the network docs and [AGD-037](https://github.com/bolasblack/alcatraz/blob/master/.agents/decisions/AGD-037_transparent-proxy-for-containers.md) for the full story. Short version: there is currently no working path for transparent UDP proxying of container traffic on Linux, and making the recipe pretend otherwise would just mislead you.

Practical implications:

- **DNS**: point the container at a DNS-over-TCP resolver if you want DNS through the proxy. `/etc/resolv.conf` entries like `nameserver 1.1.1.1` use UDP by default — pair with a local DoT/DoH resolver, or use `resolv.conf` options that force TCP (e.g., `options use-vc`).
- **HTTP/3 (QUIC)**: disable in the client (browsers have a flag) so it falls back to HTTPS over TCP.
- **App-specific UDP proxying**: if one particular app must go through the proxy over UDP, configure that app to speak SOCKS5 UDP ASSOCIATE to sing-box directly — no transparent redirection.

## What you get

```
container → alca DNAT (TCP only) → sing-box (redirect inbound) → YOUR upstream proxy
```

- All outbound TCP is intercepted — git+ssh, database clients, plain HTTP, anything.
- sing-box starts/stops alongside the sandbox via [`hooks.post_up`](../config/fields.md#hookspost_up) / [`hooks.pre_down`](../config/fields.md#hookspre_down).
- Same config on Linux and macOS.

## Prerequisites

- `alca up` already works for your project (see [Quickstart](../quickstart.md)).
- Docker (or OrbStack) — we run sing-box as a sidecar container so it sits in the same network namespace as Alcatraz's nftables DNAT rules. sing-box's `redirect` inbound recovers the original destination via `SO_ORIGINAL_DST`, a `getsockopt` call that queries conntrack; conntrack only has that information if the proxy is in the same namespace as the DNAT rules. `--network host` satisfies that on both platforms (on macOS, "host" means the container-runtime VM, which is also where alcatraz's nftables live).

## Step 1 — sing-box config

Create `sing-box.json` next to your `.alca.toml`. Replace `YOUR_UPSTREAM_HOST` / credentials with your real upstream.

```json
{
  "log": { "level": "info" },

  "inbounds": [
    {
      "type": "redirect",
      "tag": "tcp-in",
      "listen": "0.0.0.0",
      "listen_port": 1080
    }
  ],

  "outbounds": [
    {
      "type": "socks",
      "tag": "upstream",
      "server": "YOUR_UPSTREAM_HOST",
      "server_port": 1080,
      "version": "5"
    },
    { "type": "direct", "tag": "direct" }
  ],

  "route": {
    "final": "upstream"
  }
}
```

- The single `redirect` inbound handles TCP; alcatraz does not redirect UDP so there is no `tproxy` inbound here.
- Swap the `upstream` outbound for whatever protocol your upstream speaks — [sing-box has dozens](https://sing-box.sagernet.org/configuration/outbound/). If your upstream requires authentication, put the credentials in the outbound — the container never sees them.

## Step 2 — Alcatraz config

```toml
image = "alpine:3.21"

[network]
proxy = "${alca:HOST_IP}:1080"

[hooks]
post_up = """
docker rm -f alca-singbox >/dev/null 2>&1 || true
docker run -d --name alca-singbox \\
  --network host \\
  --restart unless-stopped \\
  -v "$PWD/sing-box.json:/etc/sing-box/config.json:ro" \\
  ghcr.io/sagernet/sing-box:latest \\
  run -c /etc/sing-box/config.json
"""
pre_down = "docker rm -f alca-singbox >/dev/null 2>&1 || true"
```

Notes:

- `proxy = "${alca:HOST_IP}:1080"` — the `${alca:HOST_IP}` token expands to the bridge gateway IP at runtime, so the same config works on Docker, OrbStack, and Podman without hardcoding. See [network.proxy](../config/fields.md#networkproxy).
- No `lan-access` entry needed — Alcatraz automatically punches a hole for the proxy address (see [Transparent Proxy](../config/network.md#transparent-proxy)).
- `--network host` puts sing-box in the same network namespace as the nftables DNAT rules — required for `SO_ORIGINAL_DST` to return the pre-DNAT destination.
- The `post_up` hook `rm -f`s any stale container first — handy when `Ctrl+C` on a previous `alca up` skipped `pre_down`.
- `alca status` reports hook changes as drift, but alcatraz does not watch `sing-box.json`. If you edit it, restart the sidecar yourself (`docker restart alca-singbox`).

## Step 3 — verify

```bash
alca up
alca run curl -sS https://ifconfig.me   # should return the upstream's exit IP
alca down                                # stops the sandbox AND sing-box
```

Check `docker logs alca-singbox` if something looks off — sing-box logs each inbound connection along with the original destination it recovered.

## Pinning the sing-box version

`ghcr.io/sagernet/sing-box:latest` is fine for tinkering. For anything stable, pin to a released tag (e.g., `ghcr.io/sagernet/sing-box:v1.11.0`) so your sandbox doesn't drift when the `latest` tag moves.
