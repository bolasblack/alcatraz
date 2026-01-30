---
title: "Config Drift Detection for Envs: Simplicity Over Smartness"
description: "Environment variable drift detection uses simplified logic for predictability"
tags: config, state
---

## Context

When detecting configuration drift for container rebuild, we need to decide how to handle `envs` field changes. The challenges:

1. **Literal values** - Easy to compare (e.g., `FOO=bar`)
2. **Interpolated values** - Complex (e.g., `TERM=${TERM}`)
   - Would need to resolve host env at comparison time
   - Host env may change between runs
   - Makes drift detection non-deterministic
3. **`override_on_enter`** - Only affects `alca run` behavior, not container creation

## Rejected Alternative: Smart Detection

We could build smarter detection:

1. Resolve `${VAR}` at comparison time by reading host environment
2. Compare resolved values with stored resolved values
3. For `override_on_enter=true`, always consider as no drift (since it's re-applied on every enter anyway)

**Why rejected:** The complexity is not in implementation, but in **user understanding**. Users would need to understand:
- When does interpolation get resolved?
- What happens if host env changed since container creation?
- Why does `override_on_enter` affect drift detection?
- When exactly does drift trigger vs not trigger?

This mental model is too complex. Better to keep it simple and document clearly.

## Decision

Use two-phase drift detection logic:

### Phase 1: Structural drift (key set changes)
- **Key added or removed** - Always triggers drift, regardless of value type
- Adding `FOO=${BAR}` (interpolated) triggers drift because the key set changed
- Removing any key triggers drift

### Phase 2: Value drift (literal values only)
- **Literal values** (no `${...}`) - Compare normally, trigger drift if changed
- **Interpolated values** (contains `${...}`) - Value changes do not trigger drift
- **`override_on_enter`** - Excluded from comparison (similar to `Commands.Enter`, only affects enter behavior)

**Rationale:**
- Key set changes are structural: they change what environment the container sees
- Value comparison for interpolated values is still skipped (non-deterministic, depends on host env)
- Simpler mental model: "adding/removing any env key triggers rebuild; changing interpolated values doesn't"
- `override_on_enter` is like `Commands.Enter` - only affects runtime behavior, not container creation
- **Documentation should clearly state:** interpolated env *value* changes require manual `alca down && alca up`

## Implementation

```go
func hasEnvLiteralDrift(a, b map[string]config.EnvValue) bool {
    // Phase 1: structural drift (key set changes)
    if len(a) != len(b) {
        return true
    }
    for k := range a {
        if _, ok := b[k]; !ok {
            return true // Key removed or renamed
        }
    }

    // Phase 2: value drift for literal (non-interpolated) values only
    for k, va := range a {
        vb := b[k]
        if !va.IsInterpolated() && !vb.IsInterpolated() {
            if va.Value != vb.Value {
                return true
            }
        }
    }
    return false
}
```

## Consequences

**Positive:**
- Simple, predictable behavior
- Structural changes (adding/removing env keys) always detected — no silent misses
- Easy to explain: "add/remove any env key → rebuild; change interpolated value → manual rebuild"
- Consistent with `Commands.Enter` treatment for `override_on_enter`

**Negative:**
- Changes to interpolated env *values* still require manual `alca down && alca up`
- **Documentation must clearly state this** (bold warning recommended)

## Documentation Note

> **Note:** Adding or removing environment variables always triggers automatic container rebuild. However, changes to interpolated *values* (e.g., changing `${HOME}` to `${USER}`) do not trigger rebuild. Run `alca down && alca up` to apply interpolated value changes.
