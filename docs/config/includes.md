---
title: Includes
weight: 2.2
---

# Includes

Configuration files can include other files for composable configuration:

```toml
includes = [".alca.dev.toml", ".alca.local.toml"]
```

## Basic Usage

```toml
# .alca.toml - main config
includes = [".alca.base.toml"]
image = "myapp:latest"  # Override base image
```

```toml
# .alca.base.toml - shared base config
image = "nixos/nix"
workdir = "/workspace"

[commands]
enter = "[ -f flake.nix ] && exec nix develop"
```

## Path Resolution

- Paths are resolved **relative to the including file's directory** (not the current working directory)
- Absolute paths are also supported

## Glob Patterns

Use glob patterns to include multiple files:

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
| **Same key** | Later value wins (overlay overrides base)     |

## Processing Order

Includes are processed depth-first:

```
.alca.toml includes [.alca.dev.toml]
.alca.dev.toml includes [.alca.common.toml]
```

1. Load `.alca.common.toml`
2. Load `.alca.dev.toml`, merge with `.alca.common.toml`
3. Load `.alca.toml`, merge with result

## Error Handling

- **Circular reference**: Error with clear message
- **File not found (literal path)**: Error
- **Empty glob result**: OK (continues without including anything)

## Example: Environment-specific Configuration

```toml
# .alca.toml
includes = [".alca.base.toml", ".alca.local.toml"]
# .alca.local.toml is gitignored for machine-specific overrides

# .alca.base.toml (checked into repo)
image = "nixos/nix"
workdir = "/workspace"
mounts = ["~/.gitconfig:/root/.gitconfig:ro"]

# .alca.local.toml (gitignored)
mounts = ["/my/local/cache:/cache"]
[resources]
memory = "32g"
cpus = 16
```

See [AGD-022](https://github.com/bolasblack/alcatraz/blob/master/.agents/decisions/AGD-022_config-includes-support.md) for design rationale.
