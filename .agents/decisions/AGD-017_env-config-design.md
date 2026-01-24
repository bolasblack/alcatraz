---
title: "Environment Variable Configuration Design"
description: "Design for [envs] section in .alca.toml with two-tier timing support"
tags: config, cli
---

## Context

Users need to configure environment variables for containers. There are two timing points:
1. Container creation (`alca up` / `docker run`)
2. Command execution (`alca run` / `docker exec`)

Key finding: Environment variables set via `docker run -e` persist through all `docker exec` sessions.

Some variables (like `NIXPKGS_ALLOW_UNFREE`) should be set once at creation. Others (like `TERM`, `LC_LANG`) should reflect the current terminal session at each execution.

### Why Not Other Approaches

**Ideal: Automatic propagation like SSH** - SSH automatically forwards certain env vars. However, the host environment contains many variables, and blindly passing all of them could leak sensitive information (e.g., API tokens, credentials) to AI agents running inside the container. This is a security concern.

**CLI flag (`-e`)** - Too cumbersome for daily use. Having to type `alca run -e TERM=$TERM -e LC_ALL=$LC_ALL ...` every time defeats the purpose of a convenient workflow.

**Pure static values** - Inflexible. Terminal settings like `TERM` and locale settings like `LC_*` vary between sessions and machines. Hardcoding them doesn't work.

**Chosen approach: Explicit config with `${}`** - Balances security and convenience. Users explicitly declare which host variables to pass (avoiding accidental leaks), while `${VAR}` syntax provides the flexibility to read from host environment at runtime.

## Decision

### Config Schema

```toml
[envs]
# Simple string - set at container creation only
NIXPKGS_ALLOW_UNFREE = "1"
TERM = "${TERM}"

# Object with override_on_enter - also passed at docker exec
LC_LANG = { value = "${LC_LANG}", override_on_enter = true }
```

### Value Types

1. **Simple string**: Set via `docker run -e` at creation, available in all exec sessions
2. **Object with `override_on_enter: true`**: Also passed at `docker exec` time with fresh host env expansion

### Variable Expansion

Only simple `${VAR}` syntax supported. Validation: `^\$\{[a-zA-Z_][a-zA-Z0-9_-]*\}$`

Complex interpolation like `"hello${NAME}"` or `"${PATH}:/extra"` will error.

**Why not support complex interpolation?** The implementation in Go is straightforward (~5 lines with `regexp.ReplaceAllStringFunc`), but we chose not to implement it to avoid unnecessary complexity. The simple `${VAR}` syntax covers 99% of use cases. If complex interpolation is needed, users can set the full value in their shell environment first.

### Default Environment Variables

The following are passed by default, all reading from host environment (`${VAR}`) with `override_on_enter: true`:

- Terminal: `TERM`, `COLORTERM`
- Locale: `LANG`, `LC_ALL`, `LC_COLLATE`, `LC_CTYPE`, `LC_MESSAGES`, `LC_MONETARY`, `LC_NUMERIC`, `LC_TIME`

Reference: [GNU Locale Environment Variables](https://www.gnu.org/software/gettext/manual/html_node/Locale-Environment-Variables.html)

These defaults ensure the container inherits the user's terminal and locale settings at each `alca run` invocation. User-defined values in `[envs]` with the same name override these defaults.

### Precedence

1. User `[envs]` with `override_on_enter: true` (highest, at exec time)
2. Built-in defaults (TERM, LC_*, etc.) (at exec time)
3. User `[envs]` simple string (at creation time)
4. Container image defaults (lowest)

## Consequences

- Users get unified config without thinking about timing for most cases
- `override_on_enter` provides explicit control for terminal-session-dependent vars
- Simple `${VAR}` validation prevents subtle bugs from complex interpolation
- Config drift detection should include envs to trigger rebuild prompts

Reference: `.agents/references/env-config-design.md` for full design document.
