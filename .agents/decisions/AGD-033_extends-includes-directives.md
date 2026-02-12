---
title: "Extends/Includes Directives"
description: "Split includes into extends/includes with different merge directions"
tags: config
obsoletes: AGD-022
updates: AGD-009
---

## Context

AGD-022 introduced `includes` for composable config files. The current merge order — where the including file (main) overrides included files (local) — is counterintuitive and inconvenient for the common use case where `.local` files should override the main config for personal customizations.

## Decision

### Split `includes` into Two Directives

**`extends`** — "I inherit from these files, I override them"

- The file declaring `extends` is the overlay (wins)
- Same merge direction as old `includes`
- Use case: specialization files that build on a base

**`includes`** — "I pull in these files, they override me"

- Included files are the overlay (win)
- Conventional behavior: local overrides main
- Use case: `.alca.toml` includes `.alca.local.toml`

**Three-layer merge when both present:**

```
extends files (base) → self (middle) → includes files (top)
```

**Migration:** Direct rename of existing `includes` to `extends` or `includes` based on desired semantics. No backward compatibility shim for the old `includes` behavior.

### Merge Mechanism

Both directives process their arrays left-to-right, depth-first. Each referenced file is recursively resolved (its own extends/includes processed) before participating in the merge.

**Within-array priority differs by directive:**

- `extends = [B, C, D]`: left overrides right — B > C > D (like OOP: first parent has highest priority)
- `includes = [B, C, D]`: right overrides left — D > C > B (like CSS: last entry wins)

**Pairwise merge rules:**

- Objects: deep merge (recursive)
- Arrays: append (concatenate)
- Same key: overlay wins

**Processing a file with both directives:**

```
// 1. Resolve extends (self is overlay, first entry has highest priority among extends)
result = extends[last]
for i in (last-1)..0:
  result = merge(base=result, overlay=extends[i])
result = merge(base=result, overlay=self)

// 2. Resolve includes (included files are overlay, last entry has highest priority)
for each inc in includes:
  result = merge(base=result, overlay=inc)
```

**Example — extends: A extends [B, C, D]; D extends [E, F]; F extends [G]**

```
resolved_F = merge(base=G, overlay=F)              // F > G
resolved_D = merge(base=resolved_F, overlay=E)     // E > F > G
resolved_D = merge(base=resolved_D, overlay=D)     // D > E > F > G

temp       = merge(base=D_resolved, overlay=C)     // C > D-chain
temp2      = merge(base=temp, overlay=B)            // B > C > D-chain
final      = merge(base=temp2, overlay=A)           // A > B > C > D-chain

Priority (low→high): G < F < E < D < C < B < A
```

**Example — includes: A includes [B', C', D']; D' includes [E', F']; F' includes [G']**

```
resolved_F' = merge(base=F', overlay=G')            // G' > F'
temp        = merge(base=D', overlay=E')            // E' > D'
resolved_D' = merge(base=temp, overlay=resolved_F') // G' > F' > E' > D'

temp2       = merge(base=A, overlay=B')             // B' > A
temp3       = merge(base=temp2, overlay=C')         // C' > B' > A
final       = merge(base=temp3, overlay=resolved_D') // G' > F' > E' > D' > C' > B' > A

Priority (low→high): A < B' < C' < D' < E' < F' < G'
```

**Summary:** Both directives process left-to-right, depth-first. Self always has highest priority in extends, lowest in includes. Within the array, extends gives priority to earlier entries (like OOP inheritance), includes gives priority to later entries (like CSS cascade).

### Processing Rules

- Path resolution: relative to declaring file's directory (unchanged from AGD-022)
- Glob support: unchanged
- Circular reference detection: unchanged
- Error handling: unchanged
- `extends` and `includes` fields removed after processing (not in final Config struct)

## Consequences

### Positive

- Clear semantic distinction between two merge directions
- Conventional behavior available via `includes` (local overrides main)
- Both directives can coexist for layered configuration

### Negative

- Breaking change: existing `includes` field must be migrated to `extends` or new `includes`
