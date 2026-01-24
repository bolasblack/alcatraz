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

Use simplified drift detection logic:

1. **Literal values** (no `${...}`) - Compare normally, trigger drift if changed
2. **Interpolated values** (contains `${...}`) - Always consider as no drift
3. **`override_on_enter`** - Excluded from comparison (similar to `Commands.Enter`, only affects enter behavior)

**Rationale:**
- Simpler mental model: "literals trigger rebuild, interpolations don't"
- Avoids non-deterministic behavior from host env changes
- `override_on_enter` is like `Commands.Enter` - only affects runtime behavior, not container creation
- **Documentation should clearly state:** env changes require manual `alca down && alca up`

## Implementation

```go
// envLiteralsDrift checks if literal (non-interpolated) env values have changed.
// Interpolated values (containing ${...}) are ignored.
// EnvValue.OverrideOnEnter is also ignored (only affects enter behavior).
func envLiteralsDrift(a, b map[string]config.EnvValue) bool {
    // Collect literal values only (skip ${...} interpolations)
    aLiterals := make(map[string]string)
    for k, v := range a {
        if !strings.Contains(v.Value, "${") {
            aLiterals[k] = v.Value
        }
    }
    // ... compare aLiterals with bLiterals
}
```

## Consequences

**Positive:**
- Simple, predictable behavior
- Easy to explain: "change literal env → rebuild prompt; change interpolated env → manual rebuild"
- Consistent with `Commands.Enter` treatment

**Negative:**
- Changes to interpolated env values require manual `alca down && alca up`
- **Documentation must clearly state this** (bold warning recommended)

## Documentation Note

> **Note:** Changes to environment variables with `${VAR}` interpolation do not trigger automatic container rebuild. Run `alca down && alca up` to apply these changes.
