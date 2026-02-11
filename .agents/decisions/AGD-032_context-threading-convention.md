---
title: Context Threading Convention
description: context.Context is passed as the first parameter of every function that executes external commands or network calls, never stored in structs
tags: patterns
updates: AGD-029
---

## Context

Go's `context.Context` carries per-call lifecycle signals (cancellation, timeout) and request-scoped metadata (trace ID, auth token). The Go standard library and official documentation establish clear conventions for its use.

This project uses dependency injection (AGD-029) for component-level dependencies like `Fs` and `CommandRunner`. `context.Context` serves a different purpose — it is per-call metadata, not a component dependency — and requires its own conventions.

## Decision

### 1. context.Context is a call parameter, not a dependency

`context.Context` is per-call metadata, not a component-level dependency. It must NOT be stored in struct fields or Env types. It follows the call chain, not the DI graph.

```go
// Correct: ctx as first parameter
func (r *Runtime) Run(ctx context.Context, name string, args ...string) error

// Wrong: ctx stored in struct
type Runtime struct {
    ctx context.Context  // DO NOT do this
}
```

**Exception**: Parameter objects (option bags) like `ResolveParams` may carry ctx as a field since they represent a single call's parameters, not a long-lived component. This mirrors `http.Request`.

### 2. context.Background() only at entry points

`context.Background()` should only appear in:
- CLI command handlers (`cmd.Context()` or `context.Background()`)
- `main()` / `init()`
- Test functions

Internal business modules (`internal/runtime/`, `internal/sync/`, `internal/network/`, etc.) must receive ctx from callers and either pass it through or derive from it (`WithTimeout`, `WithCancel`).

### 3. Deriving is normal, replacing is exceptional

Intermediate layers commonly derive child contexts with tighter constraints:

```go
tickCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
defer cancel()
```

Creating a new `context.Background()` mid-chain is only justified when an operation must survive caller cancellation (e.g., audit logging). Such cases should be commented.

### 4. context.Value is for cross-cutting concerns only

Do not put business dependencies (Fs, CommandRunner, SyncEnv) in context. Only request-scoped metadata that transits process boundaries belongs in context values (trace IDs, auth tokens).

## Consequences

- All functions calling external commands or network operations accept `ctx context.Context` as first parameter
- CLI layer uniformly uses `cmd.Context()` to obtain ctx, enabling future signal handling (e.g., SIGINT graceful shutdown)
- AGD-029's Env DI pattern unchanged: Env carries Fs and CommandRunner (dependencies), ctx flows through call parameters (per-call metadata)
