---
title: Firewall Crash Safety
description: Atomic and idempotent firewall operations for crash recovery
tags: network-isolation
updated_by: AGD-027
---


## Context

Network isolation requires installing firewall rules. If the process crashes mid-operation (e.g., `kill -9`), the system must recover to a consistent state.

## Decision

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

**Note**: AGD-006 decided to use CLI tools (`pfctl`, `nft`) via exec calls instead of libraries. These libraries remain options if deeper integration is needed.

## Consequences

- Crash-safe operations without external state tracking
- Idempotent design simplifies error recovery
- CLI approach (AGD-006) works for initial implementation
- Libraries available for future optimization

## References

- `.agents/decisions/AGD-006_implementation-language-go.md`
- `.agents/references/initial-research/nix-flake/firewall-declarative.md`
