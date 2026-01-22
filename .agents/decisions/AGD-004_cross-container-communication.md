---
title: "Cross-Container Communication"
description: "Use Host Relay via ccc-statusd for inter-project Claude communication"
tags: cross-container
---

## Context

Multiple projects' Claudes may need to communicate while maintaining file isolation. Need a mechanism that:
- Preserves file isolation (Container A cannot access Project B's files)
- Maintains network isolation (no RFC1918 container network)
- Provides audit/rate limiting capability

## Decision

**Host Relay via ccc-statusd**

```bash
# Both containers mount the same daemon socket
docker run -v ~/.cache/ccc-status/daemon.sock:/run/ccc.sock \
           -v /projects/x:/workspace --network none  container-a

docker run -v ~/.cache/ccc-status/daemon.sock:/run/ccc.sock \
           -v /projects/y:/workspace --network none  container-b
```

### Security Guarantees
- File isolation: Container A cannot access /projects/y
- Network isolation: `--network none`, no container network needed
- Communication: Via host daemon socket only
- Audit/Rate limiting: Centralized at ccc-statusd

## Alternatives Considered

| Pattern | Score | Notes |
|---------|-------|-------|
| **Host Relay (chosen)** | 5/5 | Simple, secure, already implemented |
| Unix Socket (direct) | 5/5 | Good but requires custom socket |
| File IPC | 4/5 | File isolation at risk |
| Container Network | 3/5 | Works but more complex |

## References

- `docs/research/cross-container/summary.md`
- `docs/research/cross-container/ccc-statusd-architecture.md`
