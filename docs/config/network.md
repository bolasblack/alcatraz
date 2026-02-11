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
- **Valid values**: `"*"` (allow all LAN access)

## Platform Behavior

Both macOS and Linux use **nftables** for network isolation and LAN access rules.

| Platform | Runtime        | Mechanism                                         | Helper                                     |
| -------- | -------------- | ------------------------------------------------- | ------------------------------------------ |
| macOS    | OrbStack       | nftables via vmhelper container (nsenter into VM) | `alcatraz-network-helper` Docker container |
| macOS    | Docker Desktop | nftables via vmhelper container (nsenter into VM) | `alcatraz-network-helper` Docker container |
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

### macOS: vmhelper Container

On macOS, containers run inside a Linux VM managed by OrbStack or Docker Desktop. Alcatraz cannot use macOS-level firewalls (like pf) because container traffic is handled in userspace and never passes through the macOS kernel's network stack.

Instead, Alcatraz runs a helper container (`alcatraz-network-helper`) that applies nftables rules **inside the VM** using `nsenter`:

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
