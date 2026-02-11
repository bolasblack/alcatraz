---
title: Env Dependency Injection Pattern
description: All internal business modules receive Fs and CommandRunner from external callers via Env structs, never creating them internally
tags: patterns
updated_by: AGD-032
---


## Context

Internal business modules (e.g., `internal/network`, `internal/runtime`) previously created their own `CommandRunner` and filesystem instances internally. This led to:

1. **Duplicate instances** — CLI files created one `CommandRunner`, then modules created another internally
2. **Untestable code** — modules with hardcoded dependencies are difficult to test with mocks
3. **Inconsistent Fs usage** — CLI needs to control whether the module writes to a `TransactFs` (batched sudo writes) or `ReadOnlyOsFs` (status queries), but modules that create their own Fs bypass this

## Decision

### Core Rule

All `internal/` business modules receive `Fs` and `CommandRunner` from external callers. Modules **never** create these dependencies internally.

**Excluded modules** (they are the "outside" that creates and passes deps):

- `internal/cli` — entry point, creates and injects deps
- `internal/transact` — provides `TransactFs`
- `internal/util` — provides `Env`, `CommandRunner`, base types

### Env Tiers

**Simple modules** — use `util.Env` directly:

```go
func DoSomething(env *util.Env) error {
    // env.Fs and env.Cmd injected from CLI
}
```

**Complex modules** (or modules expected to grow complex, with evolving dependencies) — define their own `XxxEnv`:

```go
// e.g., internal/network/shared/types.go
type NetworkEnv struct {
    Fs  afero.Fs
    Cmd util.CommandRunner
}

func NewNetworkEnv(fs afero.Fs, cmd util.CommandRunner) *NetworkEnv {
    return &NetworkEnv{Fs: fs, Cmd: cmd}
}

func NewTestNetworkEnv() *NetworkEnv {
    return &NetworkEnv{Fs: afero.NewMemMapFs(), Cmd: &MockCommandRunner{}}
}
```

```go
// e.g., internal/runtime/runtime.go
type RuntimeEnv struct { ... }

func NewRuntimeEnv(cmd util.CommandRunner) *RuntimeEnv { ... }
```

### CLI Caller Pattern

CLI creates deps once, passes the same instances everywhere:

```go
cmdRunner := util.NewCommandRunner()
tfs := transact.New()

env := &util.Env{Fs: tfs, Cmd: cmdRunner}
runtimeEnv := runtime.NewRuntimeEnv(cmdRunner)
networkEnv := network.NewNetworkEnv(tfs, cmdRunner)
```

For read-only operations:

```go
cmdRunner := util.NewCommandRunner()
fs := util.NewReadOnlyOsFs()

networkEnv := network.NewNetworkEnv(fs, cmdRunner)
```

### Constructor Rules

- `NewXxxEnv(fs, cmd)` — takes external deps, stores them. No internal creation.
- `NewTestXxxEnv()` — only exception. Creates test doubles for unit tests.

## Consequences

- Single `CommandRunner` instance per CLI command invocation — no duplicates
- CLI controls `Fs` type (`TransactFs` for writes, `ReadOnlyOsFs` for reads)
- All modules are fully testable via dependency injection
- Adding a new dependency to a module's Env is explicit and visible in the constructor signature
