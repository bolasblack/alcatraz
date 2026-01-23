# Nix Flake Research Summary

**Leader**: 857a9432
**Workers**: 72a00e4f (container), 0ddb102a (network), 4e70a175 (distribution), c38826e0 (firewall)

---

## Executive Summary

### Core Finding

**Nix Flake CANNOT implement full RCC isolation solution (Option A).**

**Nix Flake CAN provide excellent distribution + environment preparation (Option B).**

| Requirement | Nix Alone | Nix + rcc |
|-------------|-----------|-----------|
| File Isolation | ❌ (Linux-only: NixPak) | ✅ Via Podman |
| Network Isolation | ❌ Not AI-untouchable | ✅ Host firewall |
| AI-Untouchable | ❌ Config files accessible | ✅ Separate management |
| Cross-Platform | ⚠️ Partial | ✅ Yes |
| Distribution | ✅ Excellent | ✅ Excellent |

---

## Research Area 1: Nix + Container Integration

**Worker**: 72a00e4f

### Key Technologies

| Technology | Purpose | Cross-Platform | File Isolation |
|------------|---------|----------------|----------------|
| nix2container | OCI image building | macOS needs Linux builder | ❌ Build-time only |
| NixOS Containers | systemd-nspawn | ❌ NixOS only | ⚠️ Root escape risk |
| Arion | Docker Compose + Nix | ⚠️ Limited macOS | Via Docker |
| NixPak/jail.nix | Bubblewrap sandboxing | ❌ Linux only | ✅ Excellent |

### Recommendation

**Hybrid approach**: Nix Flakes for environment definition + nix2container for OCI building + Podman rootless for runtime isolation.

---

## Research Area 2: Nix Network Isolation

**Worker**: 0ddb102a

### CRITICAL FINDING

**Nix network isolation is NOT AI-untouchable.**

| Reason | Impact |
|--------|--------|
| NixOS containers allow root escape to host | AI could modify host firewall |
| Config files are AI-accessible | Could modify rules via `nixos-rebuild` |
| Kernel shared between container and host | Namespace-level isolation only |

### Platform Capabilities

| Platform | Firewall Config | AI-Untouchable |
|----------|-----------------|----------------|
| NixOS | networking.nftables ✅ | ❌ Config accessible |
| nix-darwin | pf via launchd ✅ | ❌ Config accessible |
| home-manager | ❌ Cannot configure | N/A |

### Recommendation

**Do NOT rely solely on Nix for AI-untouchable network isolation.** Manage host firewall SEPARATELY from AI-accessible configs.

---

## Research Area 3: Nix Flake Distribution

**Worker**: 4e70a175

### Option A (Full Implementation): NOT FEASIBLE

- `nix run github:user/rcc` works for binary distribution
- BUT cannot handle privileged operations (firewall, rootless container setup)
- Nix builds are sandboxed and unprivileged

### Option B (Environment Preparation): RECOMMENDED

```
nix run github:user/rcc#setup  → Check requirements
sudo rcc setup                 → One-time privileged setup
nix run github:user/rcc#claude → Run isolated Claude
```

### UX Comparison

| Aspect | brew/apt | nix run |
|--------|----------|---------|
| First-time setup | Minutes | 30min-4hrs |
| Reproducibility | Low | High |
| Cross-platform | No | Yes |
| Learning curve | Low | High |

### Flake Skeleton Provided

See `/tmp/857a9432/nix-distribution-research.md` for working flake.nix example.

---

## Research Area 4: Firewall Declarative Management

**Worker**: c38826e0

### Atomicity Comparison

| Platform | Atomicity | Crash Recovery |
|----------|-----------|----------------|
| Linux (nftables) | ✅ Kernel-level | Built-in (atomic load) |
| macOS (pf) | ⚠️ Anchor-level | Convention-based (idempotent ops) |

### Recommended Libraries

| Platform | Library | Maintainer |
|----------|---------|------------|
| macOS | pfctl-rs | Mullvad VPN (production use) |
| Linux | nftables-rs | namib-project |

### Key Design Principles

1. **Idempotent operations** - Every operation safely re-runnable
2. **No external state files** - Firewall is source of truth
3. **Recovery = uninstall + install** - Always safe

### Crash Scenario: `kill -9` During Install

| Platform | State After Crash | Recovery |
|----------|-------------------|----------|
| Linux | No partial state (atomic) | Run install |
| macOS | Anchor may exist without rules | Run recover() |

---

## Final Recommendations

### Architecture for RCC

```
┌────────────────────────────────────────────────────┐
│ Distribution Layer (Nix Flake)                     │
│ - nix run github:user/rcc                          │
│ - Cross-platform binaries                          │
│ - Dependency bundling                              │
└────────────────────────────────────────────────────┘
                         │
                         ▼
┌────────────────────────────────────────────────────┐
│ rcc Binary (Go/Rust)                               │
│ - sudo rcc setup (one-time)                        │
│ - rcc claude / rcc shell                           │
└────────────────────────────────────────────────────┘
                         │
          ┌──────────────┼──────────────┐
          ▼              ▼              ▼
    ┌──────────┐  ┌───────────┐  ┌───────────────┐
    │ Firewall │  │ Container │  │ File Isolation│
    │ (host)   │  │ (Podman)  │  │ (bind mounts) │
    │ pf/nft   │  │ rootless  │  │ per-project   │
    └──────────┘  └───────────┘  └───────────────┘
```

### What Nix Adds

| Value | Description |
|-------|-------------|
| ✅ Reproducible distribution | Same binary everywhere |
| ✅ Dependency management | Podman, tools in PATH |
| ✅ Cross-platform builds | Darwin + Linux from one flake |
| ✅ Easy updates | `nix flake update` |
| ✅ Declarative dev environments | `nix develop` |

### What Nix Cannot Replace

| Component | Why |
|-----------|-----|
| Host firewall setup | Requires root, outside Nix sandbox |
| Runtime container isolation | Podman/Docker still needed |
| AI-untouchable guarantee | Config files are accessible |

---

## Deliverables

| File | Description |
|------|-------------|
| /tmp/857a9432/nix-container-research.md | Container integration analysis |
| /tmp/857a9432/nix-network-research.md | Network isolation analysis |
| /tmp/857a9432/nix-distribution-research.md | Flake distribution + example flake.nix |
| /tmp/857a9432/firewall-declarative-research.md | Crash recovery + atomicity |
| /tmp/857a9432/nix-flake-research-summary.md | This summary |

---

## Conclusion

**Nix Flake adds significant value for distribution and reproducibility, but cannot replace direct container/firewall implementation for AI-untouchable isolation.**

Recommended path: **Option B** - Nix distributes rcc binary + dependencies, rcc handles privileged isolation setup.
