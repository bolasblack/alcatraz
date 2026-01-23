---
title: "Struct Field Exhaustiveness Check Pattern"
description: "Use type conversion to ensure all struct fields are explicitly handled in comparison/processing functions"
tags: patterns
---

## Context

When implementing comparison functions like `Equals()` for structs, it's easy to forget updating the comparison logic when new fields are added. This leads to subtle bugs where new fields are silently ignored.

We need a compile-time mechanism to force developers to review comparison logic when struct fields change.

## Decision

Use a **mirror type conversion** pattern to create compile-time checks:

```go
type ConfigSnapshot struct {
    Image    string
    Workdir  string
    Runtime  string
    Mounts   []string
    CmdUp    string
    CmdEnter string
}

func (c *ConfigSnapshot) Equals(other *ConfigSnapshot) bool {
    if c == nil || other == nil {
        return c == other
    }

    // Compile-time check: must match ConfigSnapshot fields exactly.
    // If ConfigSnapshot adds a field, this line fails to compile,
    // forcing you to update 'fields' and decide whether to compare it.
    type fields struct {
        Image    string
        Workdir  string
        Runtime  string
        Mounts   []string
        CmdUp    string
        CmdEnter string
    }
    _ = fields(*c)

    // Fields that trigger rebuild:
    return c.Image == other.Image &&
        c.Workdir == other.Workdir &&
        c.Runtime == other.Runtime &&
        c.CmdUp == other.CmdUp &&
        equalStringSlices(c.Mounts, other.Mounts)
    // CmdEnter: intentionally excluded, doesn't require rebuild
}
```

**Key points:**

1. Define a local `fields` struct that mirrors all fields of the target struct (same names, types, and order)
2. Use type conversion `fields(*c)` - if struct fields don't match exactly, compilation fails
3. In the actual comparison logic, explicitly handle each field (compare or skip with comment)

**When struct changes:**

- Add new field to struct → `fields(*c)` fails to compile → developer must update `fields` → sees comparison logic → decides whether to include field

## Consequences

**Positive:**
- Compile-time safety for struct field changes
- Forces explicit handling of every field
- Clear documentation of intentionally skipped fields via comments
- No runtime overhead (type conversion is optimized away)

**Negative:**
- Slightly more verbose code
- Must maintain field order consistency between struct and mirror type
- Pattern needs to be understood by team members

**Usage:**
- Implemented in `internal/state/state.go` for `ConfigSnapshot.Equals()`
