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
func (d *ConfigDrift) HasDrift() bool {
    old, new := d.Old, d.New

    // Compile-time check: must match config.Config fields exactly.
    // If Config adds a field, this line fails to compile,
    // forcing you to update 'fields' and decide whether to compare it.
    type fields struct {
        Image     string
        Workdir   string
        Runtime   config.RuntimeType
        Commands  config.Commands
        Mounts    []string
        Resources config.Resources
        Envs      map[string]config.EnvValue
    }
    _ = fields(*old)

    // Fields that trigger rebuild:
    if old.Image != new.Image ||
        old.Workdir != new.Workdir ||
        // ...
    {
        return true
    }
    // Commands.Enter: intentionally excluded, doesn't require rebuild

    return false
}
```

**Key points:**

1. Define a local `fields` struct that mirrors all fields of the target struct (same names, types, and order)
2. Use type conversion `fields(*c)` - if struct fields don't match exactly, compilation fails
3. In the actual comparison logic, explicitly handle each field (compare or skip with comment)

**When struct changes:**

- Add new field to struct → `fields(*c)` fails to compile → developer must update `fields` → sees comparison logic → decides whether to include field

## Limitation: Nested Structs

The basic pattern only checks top-level fields. If a nested struct (e.g., `Commands`, `Resources`) adds new fields, the mirror type still compiles because it only checks the type name, not internal fields:

```go
type fields struct {
    Commands  config.Commands   // Only checks type, not internal fields!
    Resources config.Resources  // Same issue
}
_ = fields(*old)  // Still compiles even if Commands adds new fields
```

**Solution:** Add separate mirror structs for each nested type:

```go
// Top-level check
type fields struct {
    Image     string
    Workdir   string
    Runtime   config.RuntimeType
    Commands  config.Commands
    Mounts    []string
    Resources config.Resources
    Envs      map[string]config.EnvValue
}
_ = fields(*old)

// Nested struct checks
type fieldsCommands struct {
    Up    string
    Enter string
}
_ = fieldsCommands(old.Commands)

type fieldsResources struct {
    Memory string
    CPUs   int
}
_ = fieldsResources(old.Resources)

type fieldsEnvValue struct {
    Value           string
    OverrideOnEnter bool
}
for _, v := range old.Envs {
    _ = fieldsEnvValue(v)
    break // Only need to check one value for type compatibility
}
```

This ensures that adding fields to any nested struct will also trigger a compile error.

## Consequences

**Positive:**
- Compile-time safety for struct field changes
- Forces explicit handling of every field
- Clear documentation of intentionally skipped fields via comments
- No runtime overhead (type conversion is optimized away)

**Negative:**
- More verbose code, especially with nested structs
- Must maintain field order consistency between struct and mirror type
- Pattern needs to be understood by team members

**Usage:**
- Implemented in `internal/state/state.go` for `ConfigDrift.HasDrift()`
