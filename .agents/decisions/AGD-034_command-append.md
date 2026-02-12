---
title: "Command Append for Config Merge"
description: "Add append flag to commands for composable command strings across config files"
tags: config
updates: AGD-009
---

## Context

When layering config files (e.g., `.alca.toml` + `.alca.local.toml`), commands like `commands.up` are fully replaced by the overlay. Users need a way to append to a base command instead of replacing it.

Example: `.alca.toml` defines `commands.up = "nix develop"` and `.alca.local.toml` wants to append `bash` to produce `nix develop bash`.

## Decision

### Commands: Support Both String and Struct Formats

```toml
# String format (unchanged)
[commands]
up = "docker compose up -d"

# Struct format (new)
[commands.up]
command = "docker compose up -d"
append = false
```

Both formats are equivalent when `append` is not needed. The struct field for the command string is named `command`.

### Append Semantics

`append` (bool, default false) controls merge behavior per command:

- `append = false` (default): overlay replaces base (normal override)
- `append = true`: result = `base_command + " " + overlay_command` (space-concatenated)

**Rule: only the overlay's `append` flag is consulted.** The base's `append` is ignored during that merge.

| Directive    | Overlay | append on overlay | Result                |
| ------------ | ------- | ----------------- | --------------------- |
| B extends A  | B       | B has append      | `A.cmd + " " + B.cmd` |
| B extends A  | B       | A has append      | `B.cmd` (replace)     |
| B includes A | A       | A has append      | `B.cmd + " " + A.cmd` |
| B includes A | A       | B has append      | `A.cmd` (replace)     |

**No separator.** Space-concatenation gives maximum flexibility — users control shell semantics themselves (e.g., write `&& npm install` or just `bash`).

### Example

```toml
# .alca.toml
includes = ["./.alca.*.toml"]
[commands.up]
command = "nix develop"

# .alca.local.toml (gitignored, personal)
[commands.up]
command = "bash"
append = true

# Result: "nix develop bash"
```

## Consequences

### Positive

- Enables command composition without full replacement
- Simple space-concatenation is flexible and predictable
- Backward compatible: string format still works as before

### Negative

- Commands config type becomes more complex (string or struct)
- Merge logic needs to handle `append` flag during command merging
