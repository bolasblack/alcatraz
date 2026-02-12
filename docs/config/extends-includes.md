---
title: Extends & Includes
weight: 2.2
---

# Extends & Includes

Configuration files can compose with other files using two directives that control merge direction:

- **`extends`** — "I inherit from these files; I override them"
- **`includes`** — "I pull in these files; they override me"

## Extends

The declaring file is the overlay — its values win over extended files.

```toml
# .alca.toml - extends a base (my values win)
extends = [".alca.base.toml"]
image = "myapp:latest"  # Overrides base image
```

```toml
# .alca.base.toml - shared base config
image = "nixos/nix"
workdir = "/workspace"

[commands]
enter = "[ -f flake.nix ] && exec nix develop"
```

Use case: specialization files that build on a shared base.

## Includes

Included files are the overlay — their values win over the declaring file.

```toml
# .alca.toml - includes local overrides (included files win)
includes = [".alca.local.toml"]
image = "nixos/nix"
```

```toml
# .alca.local.toml - personal overrides (gitignored)
image = "myapp:dev"  # Wins over .alca.toml
```

Use case: the conventional pattern where `.local` files override the main config.

## Three-Layer Merge

When both directives are present, merge follows three layers:

```
extends files (base) > self (middle) > includes files (top)
```

```toml
# .alca.toml
extends = [".alca.base.toml"]
includes = [".alca.local.toml"]
image = "myapp:latest"
```

1. Start with `.alca.base.toml` (base layer)
2. Merge `.alca.toml` values on top (middle layer — overrides extends)
3. Merge `.alca.local.toml` values on top (top layer — overrides everything)

## Path Resolution

- Paths are resolved **relative to the declaring file's directory** (not the current working directory)
- Absolute paths are also supported

## Glob Patterns

Use glob patterns to match multiple files:

```toml
includes = [".alca.*.toml"]  # Includes .alca.dev.toml, .alca.local.toml, etc.
```

- Supported patterns: `*`, `?`, `[...]`
- Empty glob results are OK (no error if no files match)
- Literal paths (without glob characters) must exist or will error

## Merge Behavior

| Type         | Behavior                                      |
| ------------ | --------------------------------------------- |
| **Objects**  | Deep merge (nested fields merged recursively) |
| **Arrays**   | Append (concatenate, no deduplication)        |
| **Same key** | Overlay value wins                            |

## Processing Order

Both directives process their arrays left-to-right, depth-first. Each referenced file is recursively resolved (its own extends/includes processed) before participating in the merge.

**Within-array priority differs by directive:**

- `extends = [B, C, D]`: left overrides right — B > C > D (like OOP: first parent has highest priority)
- `includes = [B, C, D]`: right overrides left — D > C > B (like CSS: last entry wins)

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

**Example — extends:** `A extends [B, C]`

```
temp  = merge(base=C, overlay=B)   // B > C
final = merge(base=temp, overlay=A) // A > B > C
Priority (low→high): C < B < A
```

**Example — includes:** `A includes [B, C]`

```
temp  = merge(base=A, overlay=B)   // B > A
final = merge(base=temp, overlay=C) // C > B > A
Priority (low→high): A < B < C
```

## Error Handling

- **Circular reference**: Error with clear message
- **File not found (literal path)**: Error
- **Empty glob result**: OK (continues without including anything)

## Example: Environment-specific Configuration

```toml
# .alca.toml
extends = [".alca.base.toml"]
includes = [".alca.local.toml"]
# .alca.local.toml is gitignored for machine-specific overrides

# .alca.base.toml (checked into repo)
image = "nixos/nix"
workdir = "/workspace"
mounts = ["~/.gitconfig:/root/.gitconfig:ro"]

# .alca.local.toml (gitignored, wins over everything)
mounts = ["/my/local/cache:/cache"]
[resources]
memory = "32g"
cpus = 16
```

See [AGD-033](https://github.com/bolasblack/alcatraz/blob/master/.agents/decisions/AGD-033_extends-includes-directives.md) for design rationale.
