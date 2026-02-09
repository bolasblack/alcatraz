# TransactFs

TransactFs provides transactional file system operations for Alcatraz. It stages file changes in memory, computes diffs against the real filesystem, and commits changes via a callback-based mechanism.

## Why It Exists

Alcatraz needs to write configuration files to system directories (e.g., `/etc/nftables.d/`, `/Library/LaunchDaemons/`). These writes may require `sudo`, and partial writes could leave the system in an inconsistent state. TransactFs solves this by:

1. Staging all changes in memory first
2. Computing a minimal diff (only changed files)
3. Committing atomically via a caller-provided callback that can handle sudo, batching, etc.

## How It Works

TransactFs wraps two filesystems:

- **staged** (`MemMapFs`): in-memory filesystem where all writes go
- **actual** (`OsFs` by default): the real filesystem

```
         +-----------+
Write -->| staged    |  (in-memory)
         +-----------+
Read  -->| staged    | --> fallback --> | actual |  (CopyOnWrite)
         +-----------+                 +--------+
```

### CopyOnWrite Semantics

- **Writes** always go to `staged` (never touch `actual` until commit)
- **Reads** check `staged` first, then fall back to `actual`
- **Writes are immediately visible**: after `WriteFile(tfs, path, data)`, a subsequent `ReadFile(tfs, path)` returns `data` without needing a commit

```go
tfs := transact.New()
afero.WriteFile(tfs, "/etc/myconf", []byte("new content"), 0644)

// Immediately readable from tfs (no commit needed)
content, _ := afero.ReadFile(tfs, "/etc/myconf")
// content == "new content"

// actual filesystem is NOT modified yet
```

### Diff Computation

`ComputeDiff` compares staged vs actual and produces a list of `FileOp`:

| OpType   | Meaning                           |
|----------|-----------------------------------|
| OpCreate | File exists in staged but not actual |
| OpUpdate | File exists in both, content differs |
| OpChmod  | Same content, permissions differ  |
| OpDelete | File marked for deletion          |

Each `FileOp` also carries `NeedSudo bool`, determined by checking actual write permissions via `unix.Access()`.

## Commit Flow

```go
result, err := tfs.Commit(func(ctx transact.CommitContext) (*transact.CommitOpsResult, error) {
    // ctx.BaseFs = the actual filesystem
    // ctx.Ops   = []FileOp (the diff)
    for _, op := range ctx.Ops {
        transact.ExecuteOp(ctx.BaseFs, op)
    }
    return nil, nil
})
```

### What happens during Commit:

1. Computes diff (staged vs actual)
2. If no changes, returns immediately
3. Calls the callback with `CommitContext{BaseFs, Ops}`
4. **On success**: resets staged to a fresh `MemMapFs`, clears tracked paths
5. **On failure**: preserves staged state for retry

### After Commit

After a successful commit, TransactFs is **immediately reusable**:

- Staged is reset to empty
- Reads fall through to actual (which now has the committed data)
- New writes go to the fresh staged layer
- No manual reset is needed

```go
// First cycle
afero.WriteFile(tfs, "/etc/conf", []byte("v1"), 0644)
tfs.Commit(commitFunc)

// Second cycle - tfs is already clean, just keep using it
afero.WriteFile(tfs, "/etc/conf", []byte("v2"), 0644)
tfs.Commit(commitFunc)
```

### Error Handling and Retry

On commit failure, staged state is preserved. The caller can fix the issue and retry:

```go
_, err := tfs.Commit(commitFunc)
if err != nil {
    // staged is preserved, NeedsCommit() still returns true
    // fix the issue, then retry:
    _, err = tfs.Commit(commitFunc)
}
```

Note: if the callback partially writes to actual before failing, actual may be in an inconsistent state. This is the caller's responsibility to handle.

## Sudo Support

Operations are grouped by `NeedSudo` to minimize sudo invocations:

```go
groups := transact.GroupOpsBySudo(ops)
transact.ExecuteGroupedOps(ctx.BaseFs, groups, func(script string) error {
    // Execute script with sudo
    return runSudo(script)
})
```

`GenerateBatchScript` creates a shell script using base64-encoded content for safe transfer to sudo.

## PostCommitAction Pattern

Network and firewall modules return a `PostCommitAction` from their write methods. The pattern is:

1. Module writes files to tfs (staging)
2. Module returns a `PostCommitAction` (e.g., "reload nftables")
3. CLI commits tfs to disk
4. CLI runs the post-commit action

```go
// In CLI (up.go):
action, err := fw.ApplyRules(containerID, containerIP, rules)  // writes to tfs
commitIfNeeded(env, tfs, out, "Writing firewall rules")         // commit to disk
if action != nil && action.Run != nil {
    action.Run(nil)                                              // reload firewall
}
```

This separation ensures files are written to disk before any system commands (like `nft -f`) try to load them.

## Usage in the Codebase

### CLI entry point (`internal/cli/up.go`)

The CLI creates a `TransactFs`, passes it as `env.Fs` to modules, then commits:

```go
tfs := transact.New()
env := &util.Env{Fs: tfs, Cmd: cmdRunner}
// ... modules write to env.Fs (which is tfs) ...
commitIfNeeded(env, tfs, out, "Writing system files")
```

### NetworkHelper

Writes network helper scripts and config files to system directories via tfs, returns `PostCommitAction` for post-install steps.

### Firewall (nft)

Writes nftables rule files via tfs, returns `PostCommitAction` to reload rules with `nft -f`.

## Thread Safety

All TransactFs methods are protected by `sync.RWMutex`. Concurrent reads and writes are safe. However, the commit callback **must not** call tfs methods (it holds an exclusive lock and would deadlock).

## File Handle Behavior

- Read-only handles use CopyOnWrite overlay (see latest staged or actual content)
- Write handles operate directly on staged files
- After `Remove`, existing handles retain a content snapshot (Unix semantics)
- File position is preserved across commits
