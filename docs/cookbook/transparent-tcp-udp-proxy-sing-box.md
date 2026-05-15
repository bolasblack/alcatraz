---
title: Transparent TCP+UDP Proxy with sing-box (sidecar TUN)
weight: 4.2
---

# Transparent TCP+UDP Proxy with sing-box (sidecar TUN)

Route **every outbound packet** the container emits — TCP *and* UDP, any protocol, any port — through a [sing-box](https://github.com/sagernet/sing-box) sidecar that sits in the container's own network namespace and intercepts traffic at the TUN (L3) layer.

Pick this recipe over the [TCP-only recipe](./transparent-proxy-sing-box.md) when you need UDP coverage: DNS over UDP, QUIC/HTTP3, WireGuard, anything. The TCP-only path via [`network.proxy`](../config/fields.md#networkproxy) uses DNAT + `redirect`, which is structurally incapable of doing transparent UDP on container networks — see [AGD-037](https://github.com/bolasblack/alcatraz/blob/master/.agents/decisions/AGD-037_transparent-proxy-for-containers.md) for the full story. This recipe takes the sidecar-TUN path: no DNAT, no TPROXY, no `br_netfilter` dance.

## Tradeoffs vs. the TCP-only recipe

| Aspect                  | TCP-only ([recipe](./transparent-proxy-sing-box.md))                        | TCP+UDP (this recipe)                                              |
| ----------------------- | --------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| UDP coverage            | No — UDP uses the container's normal network path                           | Yes — all UDP flows through sing-box                               |
| Alcatraz integration    | First-class: `[network.proxy]` handles everything                           | Cookbook only — you wire up the sidecar via `[hooks]`              |
| Sidecar privileges      | `--network host` is enough                                                  | `--cap-add NET_ADMIN --device /dev/net/tun`                        |
| Per-packet overhead     | Redirect at the socket layer — near-zero                                    | Every packet is copied into userspace by TUN — measurable          |
| Alca container          | Unprivileged, untouched                                                     | Unprivileged, untouched — sidecar holds the capabilities           |

If you don't need UDP, prefer the TCP-only recipe: it's less moving parts and smaller blast radius.

## How it works

```
alca container ─┐
                │ shared netns (via --network container:$ALCA_CONTAINER_NAME)
sidecar       ──┤
 (sing-box)     ├──> utun0 (TUN)  <── sing-box auto_route policy rules
                │                     divert container flows to TUN,
                │                     let sing-box's own egress go direct
                └──> eth0         ──> your upstream proxy (bind_interface pins it)
```

The sidecar container joins alca's network namespace so its `utun0` interface is visible to the alca workload. sing-box's `auto_route` adds policy routing rules that send everything out via TUN, except sing-box's own outbound sockets, which are pinned to `eth0` by `bind_interface` to avoid a self-loop. See the experiment log in `docs_internal/udp-proxy-sidecar-tun-plan.md` for the gory details of why each of these is the way it is.

## Prerequisites

- `alca up` already works for your project (see [Quickstart](../quickstart.md)).
- Docker with `/dev/net/tun` accessible. OrbStack works out of the box. Docker Desktop *should* work but isn't covered by CI.
- Your upstream proxy endpoint (SOCKS5 / VMess / Shadowsocks / …).

## Step 1 — sing-box config

Create `sing-box.json` next to your `.alca.toml`. Replace `YOUR_UPSTREAM_HOST` / credentials with your real upstream.

```json
{
  "log": { "level": "info" },

  "inbounds": [
    {
      "type": "tun",
      "tag": "tun-in",
      "interface_name": "utun0",
      "address": ["172.20.0.1/30"],
      "auto_route": true,
      "strict_route": false,
      "stack": "system"
    }
  ],

  "outbounds": [
    {
      "type": "socks",
      "tag": "upstream",
      "server": "YOUR_UPSTREAM_HOST",
      "server_port": 1080,
      "version": "5",
      "bind_interface": "eth0"
    },
    {
      "type": "direct",
      "tag": "direct",
      "bind_interface": "eth0"
    }
  ],

  "route": {
    "final": "upstream"
  }
}
```

- **`bind_interface: "eth0"` on every outbound is mandatory.** Without it, sing-box's own egress packets match the policy rules its `auto_route` installed, get re-captured into the TUN, and spin in a self-loop until the CPU melts. `process_name` routing is a red herring in this topology — sing-box's `/proc` view is the sidecar's PID namespace, which doesn't contain the alca workload.
- Swap the `upstream` outbound for whatever protocol your upstream speaks — [sing-box has dozens](https://sing-box.sagernet.org/configuration/outbound/).
- The `172.20.0.1/30` TUN subnet is arbitrary; just don't collide with your container network (`172.17.0.0/16` on Docker default bridge).

## Step 2 — Alcatraz config

```toml
image = "alpine:3.21"

# No [network.proxy] — that's the TCP-only path. The sidecar TUN replaces it.

[hooks]
post_up = """
docker rm -f alca-singbox-tun >/dev/null 2>&1 || true
docker run -d --name alca-singbox-tun \\
  --network container:"$ALCA_CONTAINER_NAME" \\
  --cap-add NET_ADMIN \\
  --device /dev/net/tun \\
  --restart unless-stopped \\
  -v "$PWD/sing-box.json:/etc/sing-box/config.json:ro" \\
  ghcr.io/sagernet/sing-box:latest \\
  run -c /etc/sing-box/config.json
"""
pre_down = "docker rm -f alca-singbox-tun >/dev/null 2>&1 || true"
```

Notes:

- `$ALCA_CONTAINER_NAME` is set by Alcatraz when the hook runs — see [hooks.post_up](../config/fields.md#hookspost_up). It matches what `docker ps` shows (e.g. `alca-abc123def456`), so the sidecar lands in exactly the right netns.
- `--network container:<name>` means the sidecar **shares the alca container's entire network namespace**: same interfaces, same IPs, same port bindings. `.alca.toml`'s `ports` still bind in this shared namespace, so they work as usual — but the sidecar and alca can't both bind the same host port.
- `post_up` `rm -f`s any stale sidecar first — handy when `Ctrl+C` on a previous `alca up` skipped `pre_down`.
- `alca status` reports hook changes as drift, but Alcatraz doesn't watch `sing-box.json`. If you edit it, restart the sidecar: `docker restart alca-singbox-tun`.

## Step 3 — verify

```bash
alca up
alca run curl -sS https://ifconfig.me           # TCP — should show upstream exit IP
alca run dig @1.1.1.1 example.com +time=3 +short # UDP — should resolve via upstream
alca down                                        # stops the sandbox AND the sidecar
```

Check `docker logs alca-singbox-tun` if something looks off — look for `inbound/tun[tun-in]: inbound connection` (TCP) and `inbound/tun[tun-in]: inbound packet connection` (UDP) lines. The destination in those lines should be the **real** upstream IP, not a DNAT-rewritten address — that's the signal that the TUN path is working.

## Pinning the sing-box version

`ghcr.io/sagernet/sing-box:latest` is fine for tinkering. For anything stable, pin to a released tag (e.g., `ghcr.io/sagernet/sing-box:v1.11.0`) so your sandbox doesn't drift when `latest` moves.

## Known limitations

- **IPv4 only in this recipe.** sing-box's TUN inbound supports IPv6, but it isn't covered here — you'd need to extend `address`, `strict_route`, and likely add matching v6 rules.
- **Performance overhead is real.** Every packet is copied into userspace. Fine for normal interactive workloads; you'll feel it on sustained high-throughput flows.
- **One sidecar per alca container.** Alcatraz currently assumes one container per project. If that ever changes, the mapping between sidecar and alca container becomes a design question.
- **Host port bindings now share the namespace.** Since `--network container:...` puts the sidecar in alca's netns, if the sidecar wants to listen on a port it contends with `.alca.toml`'s `ports`. In practice this recipe's sing-box only consumes outbound — no conflict — but be aware if you extend it.
- **Runtime coverage.** Only verified on OrbStack (macOS). Docker Desktop and native Linux should work (the underlying mechanism is runtime-agnostic) but aren't CI-covered.
