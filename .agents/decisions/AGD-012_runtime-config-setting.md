---
title: "Runtime Config Setting"
description: "Add runtime configuration option to specify container runtime preference"
tags: config, cli
updates: AGD-009
---

## Context

AGD-009 defined runtime auto-detection priority but didn't provide a way for users to override it. Users need a configuration option to force a specific runtime.

## Decision

Add a `runtime` configuration setting in `.alca.toml`:

| Value | Behavior |
|-------|----------|
| `auto` (default) | Auto-detect available runtime per AGD-009 priority |
| `docker` | Force Docker, ignore other runtimes |

### Configuration

```toml
# .alca.toml
runtime = "auto"  # or "docker"
```

No CLI flag - configuration only. This keeps the CLI simple and encourages declarative config.

### Auto-Detection Priority (from AGD-009)

When `runtime = "auto"`:
- macOS: Apple Containerization > Docker
- Linux: Podman > Docker

### Future Extensions

Additional values may be added:
- `podman` - Force Podman
- `container` - Force Apple Containerization

For now, only `auto` and `docker` are supported.

## Consequences

- Users can explicitly choose Docker via config
- Auto-detection provides best available runtime by default
- No CLI flag complexity
- Configuration is declarative and version-controllable

## References

- AGD-009: Alcatraz CLI Design (runtime auto-detection)
- AGD-011: Container Runtime Fallback Strategy
