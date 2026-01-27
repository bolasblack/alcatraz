---
title: "Config Includes Support"
description: "Add includes directive for composable configuration files"
tags: config
updates: AGD-009
---

## Context

Configuration files can become large and repetitive, especially when:
- Different environments (dev, local, prod) need slightly different settings
- Teams want to share common base configurations
- Users want to layer machine-specific overrides on top of shared configs

Without includes, users must either:
1. Maintain duplicate configuration across multiple files
2. Use external tools to merge configs before use
3. Put everything in one monolithic file

## Decision

Add an `includes` field to the config file format:

```toml
includes = [".alca.dev.toml", ".alca.local.toml", ".alca.*.toml"]
```

### Path Resolution

- Paths are resolved relative to the current config file's directory (not cwd)
- Supports glob patterns (`*`, `?`, `[...]`)

### Merge Logic

1. **Objects**: Deep merge (recursive)
2. **Arrays**: Append (concatenate)
3. **Same key**: Later value wins (overlay overrides base)

### Processing Order

Includes are processed depth-first. Given:
```
.alca.toml includes [.alca.dev.toml, .alca.local.toml]
.alca.dev.toml includes [.alca.common.toml]
```

Processing order:
1. Load .alca.common.toml
2. Load .alca.dev.toml, merge with .alca.common.toml
3. Load .alca.local.toml
4. Load .alca.toml, merge all in order

### Error Handling

- **Circular reference**: Error with clear message
- **File not found (literal path)**: Error
- **Glob returns empty**: OK (no error)
- **includes field**: Removed after processing, not in final Config struct

## Consequences

### Positive

- Composable configuration enables better organization
- Easy to share base configs across projects
- Supports environment-specific overrides without duplication
- Glob patterns allow flexible matching

### Negative

- Adds complexity to config loading
- Debugging merged configs may be harder (need to trace multiple files)
- Circular reference detection adds some overhead

### Implementation

#### Config Type Separation

To support includes, we separated config types by responsibility:

- **`RawConfig`**: User-facing format for `.alca.toml` files
  - Used for TOML parsing (includes `Includes` field)
  - Used for JSON schema generation (editor autocomplete)
  - `Envs` is `RawEnvValueMap` (`map[string]any`) for flexible TOML parsing

- **`Config`**: Internal processed format
  - No `Includes` field (processed and removed)
  - No serialization tags (pure Go struct)
  - `Envs` is `map[string]EnvValue` for type safety

- **`RawEnvValueMap`**: Type alias `map[string]any` with `JSONSchema()` method
  - Allows TOML to parse both string and object env values
  - Generates correct JSON schema for editor autocomplete

#### Files

- `internal/config/includes.go`: Core logic for `LoadWithIncludes`, `rawToConfig`
- `internal/config/config.go`: `RawConfig`, `Config`, `RawEnvValueMap` types
- `internal/config/generator.go`: `configToRaw` for template serialization
- `internal/config/includes_test.go`: Full test coverage
