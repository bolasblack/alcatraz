# RCC Final Recommendation

**Project**: Claude Code Isolated Execution Environment (rcc)
**Date**: 2026-01-22 (Updated)
**Advisor**: a5bc50aa

---

## Executive Summary

All MVP requirements are achievable on both macOS and Linux platforms. This document provides the final architecture recommendation based on comprehensive research and cross-platform review.

### MVP Requirements Recap

1. **File Isolation** - AI can only access specified project directory
2. **Network Isolation** - Block RFC1918 by default, configurable whitelist, allow public internet
3. **AI-Untouchable Isolation** - Enforcement at layer AI cannot bypass
4. **Memory Auto-Release** - Use 1G, occupy only 1G (no pre-allocation)

---

## Platform Recommendations

### macOS

| macOS Version          | Recommended Solution   | MVP Status | Cost     |
| ---------------------- | ---------------------- | ---------- | -------- |
| **macOS 26+ (Tahoe)**  | Apple Containerization | ✅ Full    | Free     |
| **macOS 15 and below** | OrbStack               | ✅ Full    | $8/month |

**Tier 1: Apple Containerization (macOS 26+)**

- Native Apple solution, open source
- Optimized for Apple Silicon
- Memory auto-release confirmed in documentation
- Network isolation via macOS pf (host-level, AI-untouchable)

**Tier 2: OrbStack (macOS 15-)**

- Only solution with verified memory balloon auto-release
- Commercial but affordable ($8/month)
- Excellent Docker compatibility

### Linux

| Distribution          | Recommended Solution        | MVP Status | Cost |
| --------------------- | --------------------------- | ---------- | ---- |
| **All major distros** | Podman + nftables + SELinux | ✅ Full    | Free |

**Implementation Stack:**

- **Container**: Podman (rootless by default)
- **Network**: nftables at host level (AI-untouchable)
- **MAC**: SELinux/AppArmor for additional isolation
- **Memory**: cgroups v2 (native auto-release)

**Deployment Note**: Initial setup requires `sudo rcc init` to install firewall rules via systemd service. Runtime operation is rootless.

---

## Security Analysis

### AI-Untouchable Guarantee

| Platform       | Firewall | AI Can Bypass? | Bypass Method                    |
| -------------- | -------- | -------------- | -------------------------------- |
| macOS          | pf       | ❌ No (direct) | Social engineering only          |
| Linux          | nftables | ❌ No (direct) | Social engineering only          |
| Linux (Colima) | iptables | ⚠️ Yes         | `colima ssh -- sudo iptables -F` |

### Social Engineering: Universal Limitation

**Critical Finding**: Social engineering is a universal limitation of ALL host-firewall solutions, not specific to any platform.

| Attack Vector                        | macOS pf    | Linux nftables | Risk Level   |
| ------------------------------------ | ----------- | -------------- | ------------ |
| Write malicious script to /workspace | ✅ Possible | ✅ Possible    | HIGH         |
| Trick user to run `sudo pfctl -d`    | ✅ Possible | N/A            | HIGH         |
| Trick user to run `sudo nft flush`   | N/A         | ✅ Possible    | HIGH         |
| Colima passwordless sudo bypass      | N/A         | ✅ Possible    | **CRITICAL** |

**Mitigation**:

- User education (don't run scripts AI suggests with sudo)
- Avoid Colima for security-critical workloads
- Technical isolation + user vigilance = complete security

---

## Implementation Complexity

| Component                    | macOS      | Linux      | Notes                                    |
| ---------------------------- | ---------- | ---------- | ---------------------------------------- |
| Container setup              | LOW        | LOW        | Both have mature tooling                 |
| Network isolation            | MEDIUM     | MEDIUM     | pf anchors / nftables rules              |
| Whitelist (domain + IP:port) | MEDIUM     | MEDIUM     | Firewall rules with DNS-to-IP resolution |
| Memory management            | LOW        | LOW        | Native in both platforms                 |
| **Total**                    | **MEDIUM** | **MEDIUM** | Similar complexity                       |

---

## Distribution Strategy: Nix Flake

### Recommendation: Nix for Distribution, rcc for Isolation

**Option A (Full Nix Implementation)**: ❌ NOT feasible

- Nix cannot handle privileged operations (firewall setup requires root)
- Nix network isolation is NOT AI-untouchable (config files accessible to AI)

**Option B (Nix + rcc hybrid)**: ✅ RECOMMENDED

```bash
nix run github:user/rcc#setup   # Check requirements
sudo rcc setup                  # One-time privileged setup
nix run github:user/rcc#claude  # Run isolated Claude
```

### What Nix Provides

| Value                     | Description                   |
| ------------------------- | ----------------------------- |
| Reproducible distribution | Same binary everywhere        |
| Dependency management     | Podman, tools bundled         |
| Cross-platform builds     | Darwin + Linux from one flake |
| Easy updates              | `nix flake update`            |

### What Nix Cannot Replace

| Component                   | Why                                |
| --------------------------- | ---------------------------------- |
| Host firewall setup         | Requires root, outside Nix sandbox |
| Runtime container isolation | Podman/Docker still needed         |
| AI-untouchable guarantee    | Config files are AI-accessible     |

---

## Cross-Container Communication

### Use Case

Multiple projects' Claudes need to communicate while maintaining file isolation.

### Recommendation: Host Relay via ccc-statusd

```bash
# Both containers mount the same daemon socket
docker run -v ~/.cache/ccc-status/daemon.sock:/run/ccc.sock \
           -v /projects/x:/workspace --network none  container-a

docker run -v ~/.cache/ccc-status/daemon.sock:/run/ccc.sock \
           -v /projects/y:/workspace --network none  container-b
```

### Security Guarantees

| Guarantee           | Status                                    |
| ------------------- | ----------------------------------------- |
| File isolation      | ✅ Container A cannot access /projects/y  |
| Network isolation   | ✅ `--network none`, no container network |
| Communication       | ✅ Via host daemon socket only            |
| Audit/Rate limiting | ✅ Centralized at ccc-statusd             |

### Why Not Container Network?

Container networks could work with whitelist mechanism to allow specific container-to-container traffic, but mounting a shared socket is simpler and requires no firewall rule changes.

---

## Firewall Crash Safety

### Atomicity Comparison

| Platform         | Atomicity       | `kill -9` Recovery                    |
| ---------------- | --------------- | ------------------------------------- |
| Linux (nftables) | ✅ Kernel-level | No partial state, just re-run install |
| macOS (pf)       | ⚠️ Anchor-level | Idempotent uninstall + install        |

### Design Principles

1. **Idempotent operations** - Every operation safely re-runnable
2. **No external state files** - Firewall itself is source of truth
3. **Recovery = uninstall() + install()** - Always safe

### Recommended Libraries

| Platform | Library     | Notes                          |
| -------- | ----------- | ------------------------------ |
| macOS    | pfctl-rs    | Mullvad VPN uses in production |
| Linux    | nftables-rs | Active development             |

---

## Deliverables

### Research Reports

- `docs/research/macos/final-report.md` - macOS solution details
- `docs/research/linux/final-report.md` - Linux solution details

### Cross-Platform Reviews

- `docs/research/cross-review/review-macos.md` - Linux team reviewed macOS
- `docs/research/cross-review/review-linux.md` - macOS team reviewed Linux
- `docs/research/cross-review/review-linux-corrections.md` - Self-corrections after web verification

### Supporting Research

- `docs/research/macos/network-isolation.md` - Network isolation deep dive
- `docs/research/macos/memory-management.md` - Memory behavior analysis
- `docs/research/linux/container-research.md` - Container runtime comparison
- `docs/research/linux/network-research.md` - Linux firewall analysis
- `docs/research/linux/memory-security-research.md` - cgroups v2 verification

### Nix Flake Research

- `docs/research/nix-flake/summary.md` - Executive summary
- `docs/research/nix-flake/distribution.md` - Flake distribution with example flake.nix
- `docs/research/nix-flake/firewall-declarative.md` - Crash recovery and atomicity

### Cross-Container Communication

- `docs/research/cross-container/summary.md` - Host Relay recommendation
- `docs/research/cross-container/ccc-statusd-architecture.md` - Daemon architecture
- `docs/research/cross-container/security-analysis.md` - Threat model

---

## Next Steps

1. **Choose target platform(s)** - macOS only, Linux only, or both
2. **Implement CLI** - `rcc init` / `rcc claude` / `rcc shell`
3. **Package distribution** - Nix Flake (recommended) or Homebrew/apt
4. **Firewall management** - Use pfctl-rs (macOS) / nftables-rs (Linux) for crash-safe operations
5. **Multi-project support** - Integrate with ccc-statusd for cross-container communication
6. **Documentation** - User guide, security model explanation

---

## Research Team

| Role    | ID                                     | Topic           | Contribution                               |
| ------- | -------------------------------------- | --------------- | ------------------------------------------ |
| Advisor | a5bc50aa                               | All             | Architecture decisions, coordination       |
| Leader  | d9ee4e3d                               | macOS           | Research coordination, Linux review        |
| Leader  | 514e02ad                               | Linux           | Research coordination, macOS review        |
| Leader  | 857a9432                               | Nix Flake       | Distribution and firewall atomicity        |
| Leader  | f3ea440e                               | Cross-Container | ccc-statusd integration                    |
| Workers | 4b6534d5, 6b4781e0, 42f47a28           | macOS           | Docker, VM, hybrid research                |
| Workers | 5b2a4fe4, 2f4dbe8d, cd832480           | Linux           | Container, network, memory research        |
| Workers | 72a00e4f, 0ddb102a, 4e70a175, c38826e0 | Nix             | Container, network, distribution, firewall |
| Workers | afa4dc6d, ece8dc7c, 229e2bf5           | Cross-Container | Architecture, patterns, security           |

---

_Research phase complete. 21 documents across 5 research areas. Ready for implementation._
