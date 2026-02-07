---
title: "macOS Network Isolation via VM nftables (All VM-Based Runtimes)"
description: "Replace macOS pf approach with nftables inside container runtime VM for network isolation and LAN access, supporting OrbStack, Docker Desktop, and future VM-based runtimes (Lima/Colima)"
tags: macos, network-isolation, security, runtime
obsoletes: AGD-023
updates: AGD-005
---

## Context

AGD-023 proposed using macOS pf (packet filter) with anchors and a LaunchDaemon to provide network isolation and LAN access for containers running under OrbStack. The approach assumed that OrbStack's VM forwards packets without NAT, requiring pf NAT rules on macOS to enable container-to-LAN connectivity, and pf block rules for network isolation.

**This premise is fundamentally flawed.** Empirical testing and architectural analysis demonstrate that the pf-based approach cannot work with OrbStack for the following reasons:

### 1. OrbStack Uses Userspace Networking — pf Is Bypassed

OrbStack implements a custom userspace virtual network stack, not standard macOS vmnet or `VZNATNetworkDeviceAttachment`. Container traffic flows through OrbStack's process in userspace, not through the macOS kernel's network stack. By the time traffic appears on macOS interfaces, it exits as connections from the OrbStack process itself — not as filterable packets on a bridge interface. macOS pf never sees the original container source IPs or can match on container subnets.

### 2. pf NAT Rules Were Never Necessary

Empirical testing confirms that containers under OrbStack can reach LAN hosts (e.g., 10.10.42.230) without any pf NAT rules. OrbStack handles NAT natively as part of its core architecture — the OrbStack documentation explicitly states "NAT is used for IPv4 and IPv6." The `ip nat POSTROUTING` chain in the OrbStack VM confirms this: `ip saddr 192.168.215.0/24 oifname != "docker0" MASQUERADE`. There was never a need for macOS-side NAT.

### 3. The Cited GitHub Issues Were All User Configuration Errors

The OrbStack issues originally cited as evidence of NAT problems were all closed as resolved — none were actual OrbStack bugs:

| Issue | Root Cause                                                          |
| ----- | ------------------------------------------------------------------- |
| #809  | User's Docker subnet (192.168.0.0/20) overlapped with LAN IP        |
| #1637 | macOS "Local Network" permission not enabled for OrbStack           |
| #1662 | WireGuard container `AllowedIPs=0.0.0.0/0` hijacked routing         |
| #98   | Working as designed — host mode showing docker0 interface is normal |

### 4. pf-Based VM Traffic Filtering Is Known To Be Unreliable

Cirrus Labs attempted pf-based filtering for Tart macOS VMs (also using Apple Virtualization.framework) and abandoned it. They described the approach as "racy by design" due to conflicts with macOS's InternetSharing daemon constantly rewriting pf rules. Their conclusion: pf is "a poor model in terms of security" for filtering virtualized workloads on macOS.

### 5. Source IP Translation Makes pf Matching Impossible

By the time container traffic reaches macOS network interfaces, source IPs are already NATted — either to the OrbStack VM's IP or to the Mac's IP. pf rules matching container subnets (e.g., 192.168.215.0/24) would never match because those addresses no longer appear in the packet headers at the macOS pf inspection point.

## Decision

### Replace pf with nftables inside the OrbStack VM

The correct approach for network isolation on macOS with OrbStack is to use nftables inside OrbStack's Linux VM, where all container traffic is visible on the Docker bridge before it leaves the VM.

### Access Method

From macOS, `alca` injects nftables rules into the OrbStack VM via nsenter:

```bash
docker run --rm --privileged --pid=host --net=host alpine \
  nsenter -t 1 -m -u -n -i nft [commands]
```

This enters PID 1's (the VM init) full namespace — mount, UTS, network, IPC — giving access to the VM host network namespace where Docker bridges and all container veth interfaces reside.

### Kernel Support

OrbStack's VM kernel (6.17.8-orbstack) has nftables built-in:

- 754 `nft_` symbols in `/proc/kallsyms`
- `CONFIG_NF_TABLES_INET=y`, `CONFIG_NF_TABLES_NETDEV=y`
- `nft_do_chain`, `nft_do_chain_ipv4`, `nft_do_chain_ipv6` all present

### Critical: Chain Priority Must Be `filter - 2`

OrbStack's `table inet orbstack` includes an `early-forward` chain at `priority filter - 1` that offloads TCP/UDP flows to a flowtable:

```
chain early-forward {
    type filter hook forward priority filter - 1; policy accept;
    meta l4proto { tcp, udp } meta oifkind != "bridge" flow add @ft
}
```

Once a flow is added to the flowtable, **all subsequent packets bypass every forward chain** — they are processed at the ingress hook, skipping nftables entirely. This means:

- **`priority filter + 10` (or any priority after `filter - 1`) is WRONG**: The first packet would be blocked, but the flowtable entry is already created by `early-forward`. Packets 2+ would bypass the block via flowtable offload.
- **DOCKER-USER chain (priority `filter + 0`) has the same problem**: It runs after `early-forward`, so blocked flows may already be offloaded. Additionally, DOCKER-USER is fragile (Docker resets it on daemon restart), IPv4-only (`table ip filter`), and tightly coupled to Docker's internal iptables-nft structure.
- **`priority filter - 2` is correct**: Our chain runs before `early-forward`. Blocked traffic is dropped before it can be offloaded. Allowed traffic passes through to `early-forward` and benefits from flowtable performance acceleration.

### Rule Scheme

```nft
table inet alcatraz {
    set rfc1918 {
        type ipv4_addr
        flags interval
        elements = { 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16 }
    }

    chain forward {
        type filter hook forward priority filter - 2; policy accept;

        # Only process outbound traffic from Docker bridges
        iifname "docker0" goto alcatraz_filter
        iifname "br-*" goto alcatraz_filter
        # Everything else: accept (don't interfere with other traffic)
    }

    chain alcatraz_filter {
        # Allow return traffic for established connections
        ct state established,related accept

        # Per-container exceptions (populated by alca up)
        # Example: ip saddr 192.168.215.2 ip daddr @allow_abc123 accept

        # Default: block container → RFC1918 (LAN)
        ip daddr @rfc1918 counter drop

        # Non-RFC1918 (internet): falls through to chain policy accept
    }
}
```

### LAN Access Behavior

With OrbStack, containers can reach LAN hosts natively — OrbStack's NAT handles this without any additional rules. The `lan-access` config option (AGD-028) controls whether alca's nftables rules _allow_ or _block_ RFC1918 destinations:

- **`lan-access = []`** (default): nftables rules drop RFC1918 destinations → containers isolated from LAN
- **`lan-access = ["*"]`**: No RFC1918 drop rule → containers can reach LAN freely (OrbStack NAT works natively)
- **`lan-access = ["192.168.1.100:8080"]`**: Per-container exception added before the RFC1918 drop rule

No pf NAT rules are needed in any case.

### Impact on Apple Containerization

This decision does **not** affect Apple Containerization (the native macOS container runtime). Apple Containerization does not use OrbStack — it runs containers directly on macOS using Virtualization.framework with standard vmnet networking. For Apple Containerization, pf remains the correct firewall approach because traffic does pass through the macOS kernel's network stack.

### Runtime Detection and Dispatch

The code dispatches based on a `Runtime RuntimePlatform` enum (replacing the previous `IsOrbStack bool` flag in `internal/network/shared/types.go`):

```go
type RuntimePlatform string

const (
    RuntimeOrbStack      RuntimePlatform = "orbstack"
    RuntimeDockerDesktop RuntimePlatform = "docker-desktop"
    // Future:
    // RuntimeLima       RuntimePlatform = "lima"
    // RuntimeColima     RuntimePlatform = "colima"
    // RuntimeAppleContainerization RuntimePlatform = "apple-containerization"
)
```

The factory layer (`network.New()`) uses `Runtime` to select the firewall backend:

- **OrbStack / Docker Desktop** (and future Lima/Colima): nftables via helper container
- **Apple Containerization** (future): pf on macOS

`Runtime` also determines:

- Chain priority in generated `.nft` files (OrbStack: `filter - 2`, others: `filter - 1`)
- `ALCA_PLATFORM` env var passed to helper container at creation time
- Whether ECI detection is needed (Docker Desktop only)

### Docker Desktop Portability

The helper container + nsenter + nftables approach is **not OrbStack-specific**. It works on Docker Desktop (macOS) too, because:

- Docker Desktop also runs a Linux VM (LinuxKit-based)
- `nsenter -t 1` via `docker run --privileged --pid=host` is a well-established technique (documented by Docker maintainers, e.g., justincormack/nsenter1)
- Container traffic traverses docker0/bridge interfaces inside the VM — nftables forward chain rules can intercept it
- `--restart=always` works identically
- Bind mounting `~/.alcatraz/files/` works via VirtioFS (default macOS file sharing)

**Key differences from OrbStack:**

| Aspect                             | OrbStack                                                               | Docker Desktop                                                                                                                                          |
| ---------------------------------- | ---------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Flowtable offload                  | Yes (`early-forward` at `filter - 1`) — requires `priority filter - 2` | No flowtable — standard filter priority sufficient                                                                                                      |
| Priority                           | `filter - 2` (must beat flowtable offload)                             | `filter - 1` (no flowtable, but before Docker's filter chains)                                                                                          |
| Readiness check                    | Wait for `table inet orbstack`                                         | Verify `nft` binary available and `nft list tables` succeeds (Docker Desktop uses iptables by default, won't have nftables tables)                      |
| nftables kernel                    | Built-in (confirmed)                                                   | Confirmed available (`nf_tables` loads at boot; basic operations verified in docker/for-mac#6410). Runtime verification recommended as defense-in-depth |
| File sharing                       | OrbStack native FS sharing                                             | VirtioFS (supports inotify)                                                                                                                             |
| Enhanced Container Isolation (ECI) | N/A                                                                    | Hard incompatibility — ECI (Docker Business) blocks `--pid=host` and `--network=host` entirely; nsenter approach fails when ECI is enabled              |

**Runtime detection in entry.sh:** The helper container receives `ALCA_PLATFORM` env var at creation time and branches only for the readiness check:

```bash
readiness_check() {
  case "$ALCA_PLATFORM" in
    orbstack)
      wait_for "nft list table inet orbstack"
      ;;
    docker-desktop)
      nsenter -t 1 -m -u -n -i modprobe nf_tables 2>/dev/null
      wait_for "nft list tables"
      ;;
  esac
}
```

All other behavior (load, watch, reload, SIGHUP) is identical across platforms. Chain priority is baked into the `.nft` files at generation time on the host — entry.sh does not need to know about priorities.

When Lima/Colima support is added in the future, they share the `docker-desktop` readiness branch (no flowtable, same kernel characteristics).

**Risk: LOW-MEDIUM.** nftables kernel support is confirmed on both platforms. Main risks: (1) ECI (Docker Business) is a hard blocker when enabled — install must detect and error; (2) older Docker Desktop versions are less tested.

### Lima/Colima Compatibility (Verified)

The helper container + nsenter + nftables approach is confirmed compatible with Lima and Colima:

- `docker run --privileged --pid=host` + `nsenter -t 1` works identically — dockerd runs inside the Lima VM, Docker socket is SSH-forwarded to macOS
- `--pid=host` refers to the VM's PID namespace, not macOS
- Lima networking modes (slirp, vmnet, vzNAT) do not affect this mechanism
- PID 1 differs (Colima uses BusyBox init, Lima defaults to systemd) but does not affect nsenter or nft operations
- No flowtable offload — same code path as Docker Desktop (`priority filter - 1`)

**Conclusion:** Adding Lima/Colima support requires only new runtime detection logic in `DetectPlatform()` and a new `RuntimePlatform` constant. No architectural changes needed.

**Short-term scope:** Lima/Colima is not in the initial implementation. The architecture is verified and ready when needed.

### Implementation Scope

| Runtime                | Short-term       | Notes                                    |
| ---------------------- | ---------------- | ---------------------------------------- |
| OrbStack               | Supported        | Primary target                           |
| Docker Desktop         | Supported        | Same helper container pattern            |
| Docker Desktop + ECI   | Detect and error | Hard incompatibility                     |
| Lima/Colima            | Future           | Architecture verified, no changes needed |
| Podman Machine         | No plan          | Theoretically compatible, untested       |
| Apple Containerization | Future           | pf-based, separate code path             |

å

### Implementation Approach

#### File-Driven Model

`alca` on macOS communicates with the VM exclusively via files and `docker exec`. No VMCommandRunner decorator or per-command nsenter wrapping — the model is atomic file-based:

```
alca up (macOS host)
  → Generate .nft rule file (chain priority baked in per Runtime)
  → Write to ~/.alcatraz/files/alcatraz_nft/[project].nft
  → docker exec alcatraz-network-helper reload
  → Helper container: nsenter + nft -f to load all rules
```

This is preferred over command-wrapping because it is atomic (`nft -f` loads the entire file), idempotent (delete table + reload all), and simple (file + docker exec).

#### Steps

1. **Rule generation** (`alca up`): Generate `.nft` file with correct chain priority for the detected `Runtime`, write to `~/.alcatraz/files/alcatraz_nft/[project].nft`
2. **Trigger reload**: `docker exec alcatraz-network-helper reload` — active trigger, not relying solely on inotifywait
3. **Per-container rules**: Add elements to named sets and rules for specific container IPs
4. **Cleanup** (`alca down`): Remove per-project `.nft` file, trigger reload; helper reloads remaining rules (other projects stay active)
5. **Idempotency**: `table inet alcatraz` / `delete table inet alcatraz` pattern ensures clean slate on each reload (same pattern as AGD-027)

#### Firewall Interface

```
shared.Firewall (interface)
├── nft.NFTables  — OrbStack, Docker Desktop (future: Lima/Colima)
└── pf.PF         — future Apple Containerization
```

Platform differences are resolved at the factory layer (`network.New()`) and during `.nft` file generation. No interface changes needed from the current codebase.

The helper container entrypoint script is maintained at `internal/network/vmhelper/entry.sh` in the project repository. During `alca network-helper install`, this script is copied to `~/.alcatraz/files/alcatraz_network_helper/entry.sh`.

**`nft` binary requirement:** The helper container image must include the `nft` userspace tool (e.g., Alpine with `apk add nftables`). The LinuxKit VM (Docker Desktop) may not have `nft` pre-installed. The nsenter approach enters the VM's network namespace but uses the container's own binaries, so `nft` must be available in the container image.

### Rule Persistence

#### Host-Side Directory: `~/.alcatraz/`

All alcatraz state and file-based communication with the OrbStack VM lives under a single host directory:

```
~/.alcatraz/
  files/
    alcatraz_nft/              # Per-project nftables rule files
      project-abc.nft
    alcatraz_network_helper/   # Helper container entrypoint and support files
      entry.sh
```

This directory is bind-mounted into the helper container at `/files`. Because OrbStack shares the macOS filesystem into the VM, files written on macOS are visible inside containers — no nsenter needed for file delivery.

#### Network Helper Container (Platform Service)

A persistent Docker container acts as the network isolation daemon for all VM-based runtimes:

```bash
docker run -d --restart=always --privileged --pid=host --net=host \
  --name alcatraz-network-helper \
  -v ~/.alcatraz/files:/files \
  alpine sh /files/alcatraz_network_helper/entry.sh
```

**Lifecycle:**

- Created once during `alca` first setup on OrbStack
- `--restart=always` ensures it survives VM restarts (dockerd restarts it)
- NOT removed by `alca down` — it's a platform-level service, not a project container
- VM restart → dockerd starts → helper container auto-restarts → rules restored

**Entrypoint behavior (`entry.sh`):**

1. Readiness check: On OrbStack, poll for `table inet orbstack` (retry loop, 500ms interval, ~30s timeout) — OrbStack network init happens before dockerd, so rules should already be present when helper starts. On Docker Desktop, verify `nft` binary is available and `nft list tables` succeeds (Docker Desktop uses iptables by default, so there won't be nftables tables to wait for)
2. Initial load: `nsenter -t 1 -m -u -n -i nft -f` all `/files/alcatraz_nft/*.nft`
3. Watch: use inotifywait on `/files/alcatraz_nft/` directory for file changes, with a fallback periodic re-check every 30s. VirtioFS (Docker Desktop's file sharing) generally supports inotify but may have edge cases with delayed or missed events; OrbStack's native FS sharing may be more reliable. The fallback poll ensures rules are eventually applied regardless of inotify reliability.
4. On change: delete `table inet alcatraz`, then reload all `.nft` files via `nft -f`
5. SIGHUP handler: `alca` can `docker exec alcatraz-network-helper reload` or send SIGHUP to trigger immediate reload (not solely dependent on inotifywait, since macOS→VM file sharing notifications may have delay)

**`alca up` workflow:**

- Writes per-project rule file to `~/.alcatraz/files/alcatraz_nft/[project].nft`
- Triggers reload via `docker exec alcatraz-network-helper reload` (active trigger, not relying solely on inotifywait due to possible macOS→VM file sharing notification delay)
- Helper detects change and loads rules

**`alca down` workflow:**

- Removes per-project rule file from `~/.alcatraz/files/alcatraz_nft/`
- Triggers reload
- Helper reloads remaining rules (other projects stay active)

**Why not other approaches:**

- crond not running in OrbStack VM
- simplevisor is opaque, no hook mechanism
- No systemd, no rc.local
- Docker `--restart=always` is a native, reliable mechanism

#### Rule File Validity Check (Cross-Platform)

Each `.nft` file corresponds to a project directory. Stale rule files from deleted projects are automatically cleaned up. This mechanism is **cross-platform** — the same validity check logic applies to:

- macOS + OrbStack (helper container approach)
- macOS + Docker Desktop (same helper container approach)
- Linux native (nftables rules via systemd, per AGD-027)

**Rule file locations by platform:**

| Platform               | Rule file location                                                     |
| ---------------------- | ---------------------------------------------------------------------- |
| macOS + OrbStack       | `~/.alcatraz/files/alcatraz_nft/[project].nft`                         |
| macOS + Docker Desktop | `~/.alcatraz/files/alcatraz_nft/[project].nft` (same helper container) |
| Linux native           | `/etc/nftables.d/alcatraz/[project].nft`                               |

**Trigger points:**

- `alca network-helper install` (initial setup)
- `alca up` (every invocation)

**Check procedure (identical across platforms):**

1. Enumerate all `*.nft` files in the platform-appropriate rule file location
2. For each file, derive the corresponding project directory (the project path is encoded in the filename or stored as metadata)
3. Verify the project directory still exists on disk
4. Verify the directory contains a valid `.alca.toml` configuration file
5. If either check fails, the rule file is stale — remove it and reload

**Why this matters:**

- Projects may be deleted or moved without running `alca down`
- Without cleanup, orphaned nftables rules accumulate, potentially blocking network ranges that new projects need
- The check is cheap (stat calls) and runs on operations that already modify rules

**Example (macOS):**

```
~/.alcatraz/files/alcatraz_nft/
  project-abc.nft    # /Users/dev/project-abc/.alca.toml exists → keep
  old-project.nft    # /Users/dev/old-project/ deleted → remove, reload
```

#### Key findings from investigation:

- `/etc/nftables.nft` is NOT loaded at boot (confirmed: no process references nftables, simplevisor binary has zero nft strings)
- `/etc/` is persistent (`data.img` backed)
- `flush ruleset` risk is zero (`nftables.nft` is vestigial)
- simplevisor manages: dockerd (PID 13), orbstack-agent (PID 20), containerd
- OrbStack rules are injected by macOS-side OrbStack process (not by any VM-side binary)

## Consequences

### Positive

- **Actually works**: Unlike pf, nftables in the VM sees all container traffic on the Docker bridge — this is the same mechanism Docker itself uses for network isolation
- **Simpler**: No LaunchDaemon, no pf anchor management, no `/etc/pf.anchors/` directory, no WatchPaths daemon, no sudo for firewall rules
- **Unified with Linux**: Uses the same nftables approach as native Linux (AGD-027), reducing code divergence between platforms
- **Correct security model**: Blocks traffic at the source (VM level) before OrbStack's userspace networking processes it
- **No macOS sudo needed**: Docker socket access on macOS is already granted; nsenter into the VM requires `--privileged` on the Docker container but not macOS sudo

### Negative

- **Requires nsenter into OrbStack VM**: This is a standard pattern (used by Docker Desktop tooling, Docker extensions, etc.) but adds a dependency on being able to run privileged containers
- **Helper container dependency**: The `alcatraz-network-helper` container must be running for rule persistence; if removed manually, `alca` re-creates it on next `alca up`
- **OrbStack major updates**: OrbStack major version updates may trigger storage migration; rule files live on macOS (`~/.alcatraz/`) so they survive VM resets, but the helper container may need re-creation
- **Vestigial `flush ruleset`**: `/etc/nftables.nft` contains `flush ruleset` but nothing in the VM loads this file (verified); risk is effectively zero unless a user manually invokes it
- **VM kernel dependency**: OrbStack must continue shipping nftables support in its kernel (currently built-in, not a module, so this is low-risk)

### Risks

- **OrbStack flowtable offload behavior changes**: If OrbStack changes `early-forward` priority or flowtable configuration, our `filter - 2` priority may need adjustment. This can be detected by checking the OrbStack nftables ruleset on startup.
- **OrbStack removes nsenter capability**: Unlikely, as this is a fundamental Docker/container debugging pattern, but would require alternative VM access (e.g., `orbctl run`)
- **`nft` binary availability**: The `nft` userspace tool must be available in the nsenter environment. Alpine's `nftables` package provides it. Alternatively, a purpose-built container image with `nft` pre-installed could be used.
- **Enhanced Container Isolation (ECI)**: Docker Business includes ECI, which when enabled blocks `--pid=host`, `--network=host`, and effective privileged container access. Our entire nsenter approach fails when ECI is enabled — the helper container cannot be installed or run. The `alca network-helper install` command must detect ECI and provide a clear error message explaining the incompatibility. ECI maps container root to unprivileged UID 100000+, so even "privileged" containers cannot perform kernel-level operations. Source: https://docs.docker.com/enterprise/security/hardened-desktop/enhanced-container-isolation/limitations/
- **Missing LinuxKit kernel sub-modules**: Docker Desktop's LinuxKit kernel is missing `CONFIG_NFT_BRIDGE_META` (bridge-level nftables) and `CONFIG_NFT_OBJREF` (named objects/counters). Neither affects our approach — we use `inet` family tables with basic rules, not bridge-level filtering or named objects. Sources: docker/for-mac#6410, linuxkit/linuxkit#3826

## Alternatives Considered

### 1. macOS pf (AGD-023) — Rejected

The original approach. Rejected because OrbStack's userspace networking bypasses macOS pf entirely. Container traffic is not visible to pf with original source IPs. The pf NAT rules were unnecessary (OrbStack handles NAT natively) and the pf block rules would never match container traffic. See Context section for full analysis.

### 2. DOCKER-USER Chain — Rejected

Tempting because it's Docker's official customization point, but fundamentally broken for our use case:

- Runs at `priority filter + 0`, after OrbStack's `early-forward` at `filter - 1` — flowtable offload bypasses it
- Docker resets DOCKER-USER on daemon restart, wiping our rules
- IPv4-only (`table ip filter`) — would need separate `table ip6 filter` changes
- Mixing native nft commands with Docker's iptables-nft compatibility layer is brittle

### 3. pf for Virtualized Workloads (Cirrus Labs Approach) — Rejected

Cirrus Labs tried pf-based filtering for Tart VMs on macOS and abandoned it. They found pf rules were "racy by design" due to macOS InternetSharing daemon conflicts. Their experience confirms that pf is not a reliable mechanism for filtering traffic from virtualized workloads on macOS.

## References

- [Cirrus Labs: Isolating Network Between Tart's macOS VMs](https://medium.com/cirruslabs/isolating-network-between-tarts-macos-virtual-machines-9a4ae3dcf7be)
- [OrbStack Architecture](https://docs.orbstack.dev/architecture)
- AGD-005: Defer Network Isolation Implementation
- AGD-023: macOS LAN Access via pf Anchor (obsoleted by this decision)
- AGD-027: Linux nftables as Primary Solution
- AGD-028: LAN Access Configuration Syntax
