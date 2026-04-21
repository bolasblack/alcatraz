---
title: Network Configuration
weight: 2.3
---

# Network Configuration

By default, Alcatraz containers can access the internet but **cannot access your local network (LAN)**. This prevents AI agents from reaching local services, databases, or other machines on your network.

## Quick Summary

| Scenario                  | Internet | LAN              | Config needed        |
| ------------------------- | -------- | ---------------- | -------------------- |
| Default (no config)       | Yes      | No               | None                 |
| Allow specific LAN access | Yes      | Configured hosts | `lan-access = [...]` |
| Allow all LAN access      | Yes      | Yes              | `lan-access = ["*"]` |
| Transparent TCP proxy     | TCP via proxy; UDP direct | Via proxy (TCP) | `proxy = "host:port"`|

## Why nftables Inside the VM?

On macOS, both Docker Desktop and OrbStack run containers inside a Linux VM. Container network traffic does not pass through macOS kernel network interfaces — instead it is handled entirely in userspace:

- **Docker Desktop** uses [vpnkit](https://www.docker.com/blog/how-docker-desktop-networking-works-under-the-hood/), a userspace TCP/IP stack that reads raw ethernet frames from the VM over a shared-memory channel and translates them into regular macOS socket calls via the `com.docker.backend` process. Docker's security documentation [explicitly describes](https://docs.docker.com/security/faqs/networking-and-vms/) this as "a user-space process (`com.docker.vpnkit`) for network connectivity instead of kernel-level networking."
- **OrbStack** uses a [custom-built virtual network stack](https://docs.orbstack.dev/architecture) that is "purpose-built from scratch." The [45 Gbps throughput](https://docs.orbstack.dev/docker/network) between macOS and Linux confirms shared-memory transport rather than kernel network interfaces.

Since macOS `pf` (packet filter) operates on kernel network interfaces, it **cannot intercept container traffic** on either platform. Docker's own documentation [recommends](https://docs.docker.com/security/faqs/networking-and-vms/) applying process-level firewall rules to `com.docker.backend` instead, but this only provides coarse-grained control (all containers or none). Per-container firewall rules must be applied inside the Linux VM using [nftables](https://docs.docker.com/engine/network/firewall-nftables), which is exactly what Alcatraz does automatically.

On Linux, containers run natively, and Alcatraz uses the system's native nftables directly.

## network.lan-access

Allow containers to access LAN hosts.

```toml
[network]
lan-access = ["*"]
```

- **Type**: array of strings
- **Required**: No
- **Default**: `[]` (no LAN access)
- **Valid values**: `"*"` (allow all LAN access), or specific host rules (see below)

### Token Expansion

The `lan-access` field supports special `${alca:<NAME>}` tokens that are resolved at runtime. Currently, only `HOST_IP` is supported:

```toml
[network]
# Allow access to host port 8080
lan-access = ["*://${alca:HOST_IP}:8080"]
```

#### `${alca:HOST_IP}`

Resolves to the container host's gateway IP address at runtime:

- **Docker**: Bridge network gateway (e.g., `172.17.0.1`)
- **Podman**: Bridge network gateway
- **OrbStack**: Bridge network gateway

This token allows your config to work across different environments without hardcoding the host IP. The IP varies by container runtime and network configuration, so using `${alca:HOST_IP}` makes your configuration portable.

**Example**: Accessing a local development server running on port 8080:

```toml
[network]
lan-access = ["*://${alca:HOST_IP}:8080"]
```

This expands at runtime to the actual host gateway IP and allows the container to connect to port 8080 on the host machine.

## Platform Behavior

Both macOS and Linux use **nftables** for network isolation and LAN access rules.

| Platform | Runtime        | Mechanism                                         | Helper                                     |
| -------- | -------------- | ------------------------------------------------- | ------------------------------------------ |
| macOS    | OrbStack       | nftables via network helper container (nsenter into VM) | `alcatraz-network-helper` Docker container |
| macOS    | Docker Desktop | nftables via network helper container (nsenter into VM) | `alcatraz-network-helper` Docker container |
| Linux    | Docker/Podman  | Native nftables                                   | Include in `/etc/nftables.conf`            |

## Network Helper

The network helper manages nftables firewall rules for container network isolation. It works on both macOS and Linux, with platform-specific implementations.

### Commands

```bash
# Install (requires sudo on Linux)
alca network-helper install

# Check status
alca network-helper status

# Uninstall
alca network-helper uninstall
```

### Automatic Installation

When running `alca up` with `lan-access` configured (or when network isolation is needed), Alcatraz checks if the network helper is installed. If not, it offers to install it automatically:

```
Network helper required for LAN access.
Install now? [y/N]
```

### macOS: Network Helper Container

On macOS, containers run inside a Linux VM managed by OrbStack or Docker Desktop. Alcatraz cannot use macOS-level firewalls (like pf) because container traffic is handled in userspace and never passes through the macOS kernel's network stack.

Instead, Alcatraz runs a network helper container (`alcatraz-network-helper`) that applies nftables rules **inside the VM** using `nsenter`:

1. Rule files (`.nft`) are written to `~/.alcatraz/files/alcatraz_nft/`
2. The helper container mounts `~/.alcatraz/files/` and watches for changes
3. Rules are loaded via `nsenter -t 1 -m -u -n -i sh -c 'nft -f /dev/stdin' < "$f"` to cross mount namespace boundaries
4. The helper also responds to SIGHUP for on-demand reloads

**Install** creates:

- `~/.alcatraz/files/alcatraz_nft/` directory for rule files
- `~/.alcatraz/files/alcatraz_network_helper/entry.sh` script
- `alcatraz-network-helper` Docker container (privileged, `--pid=host`, `--net=host`, `--restart=always`)

**Uninstall** removes the `alcatraz-network-helper` container.

### Linux: Native nftables

On Linux, Alcatraz uses the system's native nftables directly.

**Install** creates:

- `/etc/nftables.d/alcatraz/` directory for rule files
- Include line in `/etc/nftables.conf`: `include "/etc/nftables.d/alcatraz/*.nft"`
- Enables and reloads `nftables.service`

**Uninstall** removes:

- All rule files from `/etc/nftables.d/alcatraz/`
- The include line from `/etc/nftables.conf`
- All `alca-*` nftables tables
- The `/etc/nftables.d/alcatraz/` directory

## How It Works

1. On `alca up`, if `lan-access` is configured or network isolation is needed:
   - Checks if the network helper is installed; offers to install if not
   - Writes per-container nftables rule files
   - Loads rules via the platform-specific mechanism

2. On `alca down`:
   - Removes per-container rule files
   - Cleans up container-specific nftables tables

For design rationale, see [AGD-030](https://github.com/bolasblack/alcatraz/blob/master/.agents/decisions/AGD-030_orbstack-nftables-network-isolation.md).

## Transparent Proxy

Route all outbound container **TCP** traffic through a transparent proxy using nftables DNAT rules. Any TCP port, any protocol — git+ssh, database clients, plain HTTP, custom protocols — is redirected, not just what respects `HTTP_PROXY`. The proxy can run on the host, a LAN server, or any address reachable from the container.

> **Scope: TCP only.** Container UDP traffic is *not* redirected to the proxy; it egresses via the container's normal network path. Transparent UDP proxying inside a container runtime has no working path on Linux today — we are still researching alternatives. See [UDP: why it's not proxied](#udp-why-its-not-proxied) below.

### Configuration

```toml
[network]
# Proxy on the host (most common)
proxy = "${alca:HOST_IP}:1080"

# Proxy on a LAN server
proxy = "192.168.1.100:1080"

# Proxy on a remote server (if directly reachable)
proxy = "203.0.113.1:1080"
```

The `${alca:HOST_IP}` token resolves to the container host's gateway IP at runtime, making the config portable across environments. It's a convenience for the common case — any reachable IP address works.

### How It Works

1. On `alca up`, if `proxy` is configured:
   - Expands `${alca:HOST_IP}` to the actual host gateway IP (if used)
   - Writes nftables DNAT rules that redirect container **TCP** traffic to the proxy
   - Automatically allows the proxy address through LAN isolation rules (so RFC1918 block rules don't prevent reaching the proxy)

2. On `alca down`:
   - Removes the proxy DNAT rules alongside other container rules

### Proxy Requirement

The proxy must run in **redirect mode** (transparent TCP proxy) so it can determine the original destination of each connection via `SO_ORIGINAL_DST` — a `getsockopt` call that queries conntrack. [sing-box](https://github.com/sagernet/sing-box) is recommended; see the [Transparent TCP Proxy with sing-box](../cookbook/transparent-proxy-sing-box.md) cookbook recipe for a working setup that wires `hooks` and `network.proxy` together.

The proxy must run in the **same network namespace** as the Alcatraz nftables rules (the host on Linux, the container-runtime VM on macOS) so `SO_ORIGINAL_DST` can resolve the original destination from conntrack. Running sing-box as a host-networked sidecar container is the simplest cross-platform way to satisfy this.

### UDP: why it's not proxied

Short version: two independent Linux limitations combine, and neither has a clean workaround that fits alcatraz's constraints:

1. **DNAT + UDP loses the original destination for proxies that read the packet header.** The usual transparent-proxy recovery API for UDP is `IP_RECVORIGDSTADDR`, which reads the destination straight from the packet's IP header. nftables DNAT rewrites that header before the socket sees it, so the proxy sees its own address as the "original" destination — not the user's intent. Mainstream transparent proxies (sing-box, v2ray, clash) all use this API.

2. **TPROXY — the obvious alternative — does not deliver packets to a local socket when the traffic is routed through a Linux bridge.** Docker (and OrbStack / Docker Desktop) attaches containers via a bridge, so every container packet traverses the bridged path. In that path, TPROXY's socket lookup fails silently: the rule fires and the packet continues forwarding instead of being handed to the local listener. We verified this by comparing an isolated veth test (TPROXY works) against a bridge-based test in the same kernel (TPROXY does not deliver).

The rest of the landscape is worse: in-container TUN devices would require `CAP_NET_ADMIN` and `/dev/net/tun` (rejected by [AGD-037](https://github.com/bolasblack/alcatraz/blob/master/.agents/decisions/AGD-037_transparent-proxy-for-containers.md) because it weakens the sandbox), and embedding our own conntrack-querying UDP relay turns alcatraz into a data-plane participant with significant correctness and maintenance cost.

Full write-up and the options we considered live in [AGD-037](https://github.com/bolasblack/alcatraz/blob/master/.agents/decisions/AGD-037_transparent-proxy-for-containers.md). We expect to revisit this if the situation changes upstream (e.g., sing-box or another proxy adds UDP conntrack lookup support).

**In the meantime**, to handle UDP-heavy workloads:

- **DNS**: point the container at a TCP-capable DNS resolver (DoT/DoH, or `options use-vc` in `resolv.conf`) so queries ride TCP through the proxy.
- **QUIC (HTTP/3)**: disable it in the client so it falls back to HTTPS over TCP.
- **A specific UDP-speaking app**: configure it to speak SOCKS5 UDP ASSOCIATE to sing-box directly, no transparent redirection.

### Limitations

Alcatraz operates at the network layer (nftables DNAT) and does not participate in proxy protocol negotiation. This means:

- **No SOCKS5/HTTP CONNECT authentication** — DNAT'd packets arrive at the proxy as raw TCP without any proxy protocol handshake. If your upstream proxy requires authentication, run a local redirect-mode proxy (e.g., sing-box) that handles authentication to the upstream server:

  ```
  Container → DNAT → local sing-box (redirect, no auth)
                         ↓ (outbound with auth)
                     remote proxy server
  ```

  Configure `proxy = "${alca:HOST_IP}:1080"` pointing to the local sing-box, which forwards to the authenticated upstream. Authentication is between the local proxy and the remote server — transparent to the container.

- **No per-protocol filtering** — all outbound TCP is proxied. If you need to proxy only specific protocols, configure that on the proxy side.

### Why Not HTTP_PROXY?

Setting `HTTP_PROXY`/`HTTPS_PROXY` environment variables only works for applications that explicitly support them (curl, apt, pip, etc.). Many tools ignore these variables:

- `git` over SSH
- Database clients (psql, mysql, redis-cli)
- Custom TCP protocols

nftables DNAT operates at the network layer, intercepting **every** TCP packet regardless of what application sends it.

For design rationale and the TCP-only scoping decision, see [AGD-037](https://github.com/bolasblack/alcatraz/blob/master/.agents/decisions/AGD-037_transparent-proxy-for-containers.md).

## Without Alcatraz

For context, here's what manual LAN isolation requires on macOS:

1. Run a privileged container or use `nsenter` to access the Docker Desktop / OrbStack VM's network namespace
2. Write nftables or iptables rules blocking traffic to private IP ranges (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`)
3. Handle rule persistence — rules are lost on VM restart and must be reapplied
4. Manage per-container rule lifecycle (create on start, clean up on stop)

Alcatraz automates all of this through the network helper.

## Manual Cleanup

If Alcatraz is broken, you can clean up manually:

### macOS

```bash
# View running helper container
docker inspect alcatraz-network-helper

# Remove helper container
docker rm -f alcatraz-network-helper

# View active nftables rules inside the VM (OrbStack)
docker run --rm --privileged --pid=host alpine nsenter -t 1 -m -u -n -i nft list tables

# Delete all alcatraz nftables tables inside the VM
docker run --rm --privileged --pid=host alpine sh -c '
  nsenter -t 1 -m -u -n -i nft list tables | grep "inet alca-" | while read _ _ table; do
    nsenter -t 1 -m -u -n -i nft delete table inet "$table"
  done
'

# Remove rule files
rm -rf ~/.alcatraz/files/alcatraz_nft/
rm -rf ~/.alcatraz/files/alcatraz_network_helper/
```

### Linux

```bash
# View active alcatraz nftables tables
sudo nft list tables | grep alca-

# Delete all alcatraz tables
sudo nft list tables | grep "inet alca-" | while read _ _ table; do
  sudo nft delete table inet "$table"
done

# Remove rule files and directory
sudo rm -rf /etc/nftables.d/alcatraz/

# Remove include line from nftables.conf (edit manually)
sudo vi /etc/nftables.conf
# Remove the line: include "/etc/nftables.d/alcatraz/*.nft"

# Reload nftables
sudo nft -f /etc/nftables.conf
```
