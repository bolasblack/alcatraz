---
title: "Nix Flake Distribution Strategy"
description: "Use Nix Flake for distribution, rcc binary for isolation"
tags: nix
---

## Context

Evaluated whether Nix Flake can implement the full isolation solution or only serve as distribution mechanism.

## Decision

**Hybrid approach: Nix for distribution, rcc binary for isolation**

```bash
nix run github:user/rcc#setup   # Check requirements
sudo rcc setup                  # One-time privileged setup
nix run github:user/rcc#claude  # Run isolated Claude
```

### What Nix Provides
- Reproducible distribution (same binary everywhere)
- Dependency management (Podman, tools bundled)
- Cross-platform builds (Darwin + Linux from one flake)
- Easy updates (`nix flake update`)

### What Nix Cannot Replace
- Host firewall setup (requires root, outside Nix sandbox)
- Runtime container isolation (Podman/Docker still needed)
- AI-untouchable guarantee (Nix config files are AI-accessible)

## Alternatives Rejected

**Full Nix Implementation**: NOT feasible because:
- Nix cannot handle privileged operations
- Nix network isolation is NOT AI-untouchable

## References

- `docs/research/nix-flake/summary.md`
- `docs/research/nix-flake/distribution.md`
