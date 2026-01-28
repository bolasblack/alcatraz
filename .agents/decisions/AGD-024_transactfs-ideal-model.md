---
title: "TransactFs Ideal Model Design"
description: "Design decisions for transactional filesystem with CopyOnWrite semantics and callback-based commit"
tags: tooling, security, cli
---

## Context

The `transact` package needs to provide a transactional filesystem abstraction for batching privileged file operations (e.g., sudo writes to `/etc/`). Key requirements:

1. Stage changes in memory before committing
2. Diff comparison to detect actual changes (avoid unnecessary sudo prompts)
3. Batch sudo operations into single invocation
4. Full `afero.Fs` interface for composability
5. Correct handle behavior across commits

## Decision

### 1. Implement Full afero.Fs Interface with CopyOnWrite Semantics

TransactFs implements the complete `afero.Fs` interface internally using CopyOnWrite semantics:

- **Read**: Check staged first, fallback to actual
- **Write**: Goes to staged only
- Allows TransactFs to be passed wherever `afero.Fs` is expected

### 2. File Handle Behavior (Critical)

Old file handles must behave correctly after Commit, following Unix semantics:

| Scenario | Old Handle Behavior |
|----------|---------------------|
| Write -> Commit | Sees new content |
| Remove -> Commit | Retains old content (Unix: inode still referenced) |
| Write -> Remove -> Commit | Sees written content (last write before delete) |

Implementation requires a **wrapper File** that reads from CopyOnWrite overlay on each operation, with snapshot on delete.

### 3. Commit API Design

```go
type CommitContext struct {
    BaseFs afero.Fs   // actual filesystem to write to
    Ops    []FileOp   // operations to perform
}

type CommitOpsResult struct{}  // callback result, can be nil
type CommitResult struct{}     // commit result, currently empty

type CommitFunc func(ctx CommitContext) (*CommitOpsResult, error)

func (t *TransactFs) Commit(fn CommitFunc) (*CommitResult, error)
```

- **Blocking**: Commit holds exclusive lock; callback MUST NOT call tfs methods (deadlock)
- **Failure preserves state**: If callback returns error, staged is unchanged for retry
- **Success resets state**: Staged cleared, ready for next transaction

### 4. Return Value Type Design

Use pointer types `*CommitOpsResult` and `*CommitResult` rather than generics or `any`:

- Can return `nil` for "no result needed"
- Type-safe, no assertions needed
- Idiomatic Go pattern: `func() (*T, error)`
- CommitOpsResult (callback) and CommitResult (Commit) are separate types for future extensibility

### 5. Why Not Use afero.CopyOnWriteFs Directly

Evaluated but rejected:

1. **Cannot track changes**: No API to get "what was modified" for Diff
2. **Cannot clear layer**: `RemoveAll("/")` doesn't work on MemMapFs
3. **Would need wrapper anyway**: To track paths and implement Commit

## Consequences

### Positive

- Full `afero.Fs` compatibility for external code
- Correct Unix semantics for file handles
- Clean separation: callback handles actual writes (including sudo)
- Testable: mock actual fs, verify Diff output

### Negative

- Wrapper File adds complexity
- Must track open handles for snapshot-on-delete
- Exclusive lock during Commit (no concurrent reads)

## References

- Acceptance tests: `internal/transact/transact_ideal_test.go` (34 test cases)
- Implementation: `internal/transact/fs.go`
