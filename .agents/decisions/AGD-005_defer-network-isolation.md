---
title: "Defer Network Isolation Implementation"
description: "Network isolation is researched but not implemented in initial release"
tags: network-isolation
---

## Context

Network isolation (block RFC1918, configurable whitelist) is a core MVP requirement. Research phase has produced comprehensive documentation on implementation approaches for both macOS (pf) and Linux (nftables).

## Decision

**Defer network isolation implementation to future release.**

Initial release will focus on:
- File isolation (container bind mounts)
- Basic container runtime setup
- CLI interface (`rcc init` / `rcc claude` / `rcc shell`)

Network isolation will be implemented later, referencing existing research documentation.

## Rationale

1. **Complexity**: Network isolation requires privileged operations, crash-safe firewall management, DNS resolution for whitelists
2. **Research complete**: All technical details documented, ready for implementation when needed
3. **Incremental value**: File isolation alone provides significant security improvement
4. **Risk reduction**: Implement simpler features first, validate approach before adding complexity

## Implementation References

When implementing network isolation, refer to:

| Platform | Documentation |
|----------|---------------|
| macOS pf | `docs/research/macos/network-isolation.md` |
| Linux nftables | `docs/research/linux/network-research.md` |
| Crash safety | `docs/research/nix-flake/firewall-declarative.md` |
| Libraries | pfctl-rs (macOS), nftables-rs (Linux) |

## Consequences

- Initial release has weaker security (file isolation only)
- Users wanting network isolation must wait or implement manually
- Documentation enables future implementation without re-research
