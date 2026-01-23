---
title: "No 'Always Rebuild' Flag for alca up"
description: "Design decision to not add --rebuild flag since alca down && alca up achieves the same result"
tags: cli, state
---

## Context

When implementing configuration drift detection for `alca up`, we considered adding a `--rebuild` flag that would always rebuild the container regardless of whether configuration has changed.

The current implementation:
- `alca up` detects config drift and prompts for confirmation
- `alca up -f` forces rebuild without confirmation when config has changed
- If no config drift, container is reused

## Decision

We decided **not** to add an `--always-rebuild` or `--rebuild` flag.

The rationale:
1. **Composability**: `alca down && alca up` achieves the same effect
2. **Simplicity**: Fewer flags means simpler CLI
3. **Explicit Intent**: Using two commands makes the destructive action explicit
4. **Unix Philosophy**: Small tools that compose well

If users want to force a rebuild regardless of configuration:
```bash
alca down && alca up
```

This is explicit about stopping and removing the container before creating a new one.

## Consequences

### Positive
- Simpler CLI with fewer flags to remember
- Clear separation between "start if needed" (`alca up`) and "fresh start" (`alca down && alca up`)
- Users understand what's happening (down removes, up creates)

### Negative
- Two commands instead of one for forced rebuild
- Slightly more typing for the rebuild case

### Mitigated
- Users who frequently need forced rebuild can create a shell alias:
  ```bash
  alias alca-rebuild='alca down && alca up'
  ```
