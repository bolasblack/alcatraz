---
title: "Linux Firewall: nftables as Primary Solution"
description: Use nftables as the primary (and only) firewall implementation on Linux
tags: linux, network-isolation, security
updates: AGD-008
updated_by: AGD-028
---



## Context

Alcatraz needs to implement network isolation (`lan-access=[]`) on Linux to block container access to RFC1918 private IP ranges. There are two main firewall technologies available on Linux:

1. **iptables**: Traditional firewall framework (since 1998)
2. **nftables**: Modern firewall framework (since 2014, kernel 3.13+)

We need to decide on Alcatraz's primary implementation approach and whether to provide a fallback solution.

## Decision

**Single solution**: Support **nftables only**, without iptables fallback

If the system does not support nftables, Alcatraz will:
- Emit a warning that network isolation is unavailable
- Suggest installing nftables (kernel 3.13+ required)
- Continue running but without network isolation rules

## Rationale

### 1. High Market Coverage (2025-2026 Data)

**Major distributions using nftables by default**:

| Distribution | Market Share | nftables Status | Notes |
|--------------|--------------|-----------------|-------|
| Ubuntu 20.04+ | 33.9% | Default backend | Ubuntu 20.10+ uses nftables backend |
| Debian 10+ | 16% | Default framework | Default since Buster |
| CentOS | 9.3% | Default (via firewalld) | Uses nftables underneath |
| Fedora 32+ | 0.2% | Default (via firewalld) | Uses nftables underneath |
| RHEL 8+ | 0.8% | Default (via firewalld) | Uses nftables underneath |
| openSUSE/SLES | 0.1% | Default (via firewalld) | Switched to nftables in 2020 |
| Arch Linux | <0.1% | User choice | No default firewall, both available |

**Total coverage**: Ubuntu + Debian account for ~**50%** of the Linux market, plus Red Hat ecosystem brings it to ~**60%**.

**Key insights**:
- All major distributions (except NixOS, Gentoo with no defaults) have adopted nftables as the underlying or default framework
- Even systems using iptables commands (like Ubuntu) have switched to nftables backend (iptables-nft) underneath
- Distributions without pre-installed firewalls (Arch, Gentoo) allow users to choose freely - both nftables and iptables are supported

### 2. Atomic Operations Support (AGD-008 Requirement)

AGD-008 requires firewall operations to be **atomic and idempotent** to handle crash recovery scenarios.

**nftables perfectly satisfies these requirements**:

```bash
# Atomicity: Entire ruleset loaded at once, kernel accepts all or rejects all
nft -f /tmp/ruleset.nft

# Idempotency: Reloading ruleset completely replaces old rules
nft flush table inet alca-container1
nft -f /tmp/container1-rules.nft

# Kernel-level transaction guarantees:
# - All rules validated before applying
# - kill -9 will not result in partial state
# - Re-running the same nft -f command is safe
```

### 3. Simplified Implementation and Maintenance

**Reasons not to support iptables**:
- iptables lacks atomic operations, requiring complex idempotency checks and locking mechanisms
- iptables is in legacy maintenance mode, no longer the future direction
- Supporting two solutions increases code complexity and testing costs
- 60%+ market already supports nftables by default, coverage is sufficient

**Solutions for older system users**:
- Systems with kernel < 3.13 (before 2014) are rare (<5% estimate)
- These users can choose to:
  1. Upgrade kernel to 3.13+ (recommended)
  2. Manually install nftables package
  3. Accept that network isolation is unavailable

### 4. Industry Trends

> "nftables is no longer an optional upgrade; it is now the standard for Linux firewalls. As iptables is now officially in legacy maintenance mode, administrators are urged to move to nftables."
>
> — Web Asha Technologies, 2026

All major distributions are pushing migration from iptables to nftables. Supporting a single solution aligns with ecosystem evolution.

## Implementation

### Detection Logic

```go
func DetectFirewall() FirewallType {
    if runtime.GOOS == "darwin" {
        return TypePF
    }

    // Linux: nftables only
    if commandExists("nft") && nftablesWorking() {
        return TypeNFTables
    }

    // nftables not supported: return None, network isolation unavailable
    return TypeNone
}

func nftablesWorking() bool {
    // Test if nft can list tables (requires kernel support)
    cmd := exec.Command("nft", "list", "tables")
    return cmd.Run() == nil
}
```

### Warning Message

When nftables is not supported:

```go
func (r *Runtime) Start(ctx context.Context) error {
    // ...

    // Check network isolation configuration
    if len(r.config.LANAccess) == 0 {
        if r.firewall == nil || r.firewallType == TypeNone {
            log.Warnf(`
⚠️  Network isolation not available

Your system does not support nftables (requires Linux kernel 3.13+).
The container will start WITHOUT network isolation - it can access LAN.

To enable network isolation:
  1. Install nftables: sudo apt install nftables  # or yum/dnf/pacman
  2. Ensure kernel version >= 3.13: uname -r
  3. Restart Alcatraz

For more information: https://docs.alcatraz.dev/network-isolation
`)
            // Continue starting container, but without network isolation
        } else {
            // Apply firewall rules
            if err := r.firewall.BlockRFC1918(containerID, ip); err != nil {
                return fmt.Errorf("failed to apply network isolation: %w", err)
            }
        }
    }

    return nil
}
```

### nftables Rules Example

Rule files must be **idempotent** (safe to load multiple times):

```bash
# /etc/nftables.d/alcatraz/abc123.nft
table inet alca-abc123
delete table inet alca-abc123

table inet alca-abc123 {
    chain forward {
        type filter hook forward priority filter - 1; policy accept;

        # Allow established connections
        ct state established,related accept

        # Block RFC1918
        ip saddr 172.17.0.2 ip daddr 10.0.0.0/8 drop
        ip saddr 172.17.0.2 ip daddr 172.16.0.0/12 drop
        ip saddr 172.17.0.2 ip daddr 192.168.0.0/16 drop
        ip saddr 172.17.0.2 ip daddr 169.254.0.0/16 drop
        ip saddr 172.17.0.2 ip daddr 127.0.0.0/8 drop
    }
}
```

### Rule Persistence

Files are for persistence; `alca` handles live changes directly.

```
┌─────────────────────────────────────────────────────┐
│ Boot                                                │
│   nftables.service loads /etc/nftables.conf         │
│   which includes /etc/nftables.d/alcatraz/*.nft         │
├─────────────────────────────────────────────────────┤
│ Runtime                                             │
│   alca up   → write file + nft -f (immediate)       │
│   alca down → delete file + nft delete table        │
└─────────────────────────────────────────────────────┘
```

**Rule file location**: `/etc/nftables.d/alcatraz/<container-id>.nft`

**nftables.conf integration** (added by `alca network-helper install`):

```bash
include "/etc/nftables.d/alcatraz/*.nft"
```

Using `*.nft` glob pattern:
- Only loads our rules, doesn't interfere with other tools
- Safe even if system already has generic `*.nft` include
- Idempotent files handle duplicate loading gracefully

**Lifecycle**:

- **alca up**: Write file to `/etc/nftables.d/alcatraz/` + call `nft -f` directly → rules live immediately
- **alca down**: `nft delete table` + delete file → rules removed immediately
- **System reboot**: `nftables.service` loads `/etc/nftables.conf` → includes `*.nft` → rules restored

**`alca network-helper install`** (Linux):
1. Create `/etc/nftables.d/alcatraz/` directory if not exists
2. Add `include "/etc/nftables.d/alcatraz/*.nft"` to `/etc/nftables.conf`
3. Enable `nftables.service`: `systemctl enable nftables`

**`alca network-helper uninstall`**:
1. Remove all `/etc/nftables.d/alcatraz/*.nft` files
2. Flush all alca tables
3. Remove include line from nftables.conf

**Non-systemd systems** (OpenRC/sysvinit): Same file location, `alca up/down` handles live loading, boot script loads files with `for f in /etc/nftables.d/alcatraz/*.nft; do nft -f "$f"; done`.

**Manual cleanup**:

```bash
# View alca rules
nft list tables | grep alca-

# Remove all alca rules
for t in $(nft -j list tables | jq -r '.nftables[].table.name // empty' | grep ^alca-); do
    nft delete table inet "$t"
done
sudo rm /etc/nftables.d/alcatraz/*.nft
```

**Comparison with macOS**:

| | macOS | Linux |
|--|-------|-------|
| File location | `/etc/pf.anchors/alcatraz/` | `/etc/nftables.d/alcatraz/` |
| Live loading | WatchPaths auto-triggers | `alca` calls `nft -f` directly |
| Boot loading | LaunchDaemon | `nftables.service` include |
| External deps | None | None |

### Code Structure

```
internal/firewall/
├── firewall.go       // Interface definition + factory
├── pf.go            // macOS pf implementation
└── nftables.go      // Linux nftables implementation
```

## Consequences

### Positive Impact

1. **Covers mainstream users**: 60%+ of Linux users use distributions that support nftables by default
2. **Simple implementation**:
   - Only need to implement one Linux firewall solution (nftables)
   - Atomic operations naturally satisfy AGD-008 crash safety requirements
   - No need for complex idempotency checks and locking mechanisms
3. **Code quality**: Single solution reduces testing costs, improves maintainability
4. **Better performance**: nftables single-pass is more efficient than iptables multi-table traversal
5. **Future-oriented**: Aligns with Linux ecosystem evolution direction

### Negative Impact and Mitigation

**Impact**: Older systems (kernel < 3.13) cannot use network isolation feature

**Affected users estimate**: < 5% market (kernels before 2014)

**Mitigation measures**:
1. **Clear warning message**: Explicitly indicate network isolation unavailable at startup
2. **Documentation**: Clearly specify kernel version requirements in system requirements
3. **Installation guidance**: Provide commands to install nftables for various distributions
4. **Graceful degradation**: Container can still start and run normally, just without network isolation

### Special Case Handling

- **No firewall systems** (Arch, Gentoo): Users need to manually install nftables
- **NixOS**: Users need to enable `networking.nftables.enable = true` in configuration (or use default iptables, but will receive warning)
- **Old LTS systems**: Recommend upgrading kernel or accepting no network isolation

## References

### Distribution Data
- [Most Popular Linux Distributions Market Share 2026](https://commandlinux.com/statistics/most-popular-linux-distributions-market-share/)
- [Linux Statistics 2026 - SQ Magazine](https://sqmagazine.co.uk/linux-statistics/)
- Ubuntu Firewall: [Documentation](https://documentation.ubuntu.com/security/security-features/network/firewall/)
- Debian nftables: [Wiki](https://wiki.debian.org/nftables)
- SUSE firewalld: [Documentation](https://www.suse.com/support/kb/doc/?id=000020643)

### Technical Documentation
- [Docker with nftables](https://docs.docker.com/engine/network/firewall-nftables)
- [Deep Dive into NFTables vs IPTables](https://thesheryar.com/deep-dive-into-nftables-vs-iptables/)
- [nftables vs iptables Linux Firewall Setup](https://www.zenarmor.com/docs/linux-tutorials/nftables-vs-iptables-linux-firewall-setup)

### Internal Documentation
- AGD-008: Firewall Crash Safety (requires atomic and idempotent operations)
- `.agents/references/initial-research/linux/network-research.md`
- `/tmp/23894cca/lan-access-implementation-analysis.md` (research report)
