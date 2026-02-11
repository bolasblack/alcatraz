---
title: "Sync Conflicts"
date: 2026-02-10
---

# Sync Conflicts

## Overview

Sync conflicts occur when the same file is modified on both your local machine and inside the container simultaneously. Alcatraz detects these conflicts automatically and provides tools to resolve them.

## Detection

Conflict detection runs automatically in the background. Results are cached in `.alca/sync-conflicts-cache.json` within your project directory using a stale-while-revalidate (SWR) pattern:

- **Stale read**: Commands like `alca up`, `alca run`, and `alca down` read from the cache for instant display, then refresh the cache asynchronously in the background.
- **Fresh read**: `alca status` and `alca experimental sync check` perform a synchronous fresh check.

The `.alca/` directory is auto-created when needed and should be added to `.gitignore`.

## Notification

When sync conflicts exist, a warning banner is displayed:

- **`alca up`, `alca run`, `alca down`**: Banner is shown on stderr (does not interfere with command output)
- **`alca status`**: Conflict banner is shown on stderr after the status display

If conflict detection fails (e.g., mutagen daemon not running), a warning is printed to stderr instead of failing silently.

The banner shows up to 3 conflicting file paths with descriptions of each side's state (e.g., "modified locally, deleted in container"), plus a count of additional conflicts if more than 3 exist.

## Resolution

### Interactive Resolution

Use `alca experimental sync resolve` to walk through conflicts one by one:

```
$ alca experimental sync resolve
2 sync conflicts found:

[1/2] src/main.go
  Local (your machine):  modified
  Container:             modified
? How to resolve?
  > Local overwrites container
    Container overwrites local
    Skip
```

For each conflict, choose one of:

- **Local overwrites container** — Your local file replaces the container version
- **Container overwrites local** — The container file replaces your local version
- **Skip** — Leave the conflict unresolved

The container must be running for resolution to work.

### Machine-Readable Check

Use `alca experimental sync check` for scripting:

```bash
if ! alca experimental sync check; then
    echo "Sync conflicts detected"
    alca experimental sync resolve
fi
```

Exit codes: 0 = no conflicts, 1 = conflicts found.

#### Custom Output with `--template`

The `--template` flag accepts a Go [`text/template`](https://pkg.go.dev/text/template) string, giving you full control over the output format.

**Template data:**

The root object has the following fields:

| Field        | Type             | Description                  |
|--------------|------------------|------------------------------|
| `.Count`     | `int`            | Number of conflicts          |
| `.Conflicts` | `[]ConflictInfo` | List of conflict details     |

Each `ConflictInfo` in `.Conflicts` has:

| Field             | Type        | Description                                                        |
|-------------------|-------------|--------------------------------------------------------------------|
| `.Path`           | `string`    | Relative file path from project root                               |
| `.LocalState`     | `string`    | State on your machine: `modified`, `created`, `deleted`, `directory` |
| `.ContainerState` | `string`    | State in the container: `modified`, `created`, `deleted`, `directory` |
| `.DetectedAt`     | `time.Time` | When the conflict was first detected                               |

A built-in `json` helper function is available for JSON serialization.

**Examples:**

```bash
# JSON output (for parsing by other tools)
alca experimental sync check --template='{{ json . }}'

# Count only
alca experimental sync check --template='{{ .Count }} conflicts'

# List conflicting paths, one per line
alca experimental sync check --template='{{ range .Conflicts }}{{ .Path }}{{ "\n" }}{{ end }}'

# Show path and state details
alca experimental sync check --template='{{ range .Conflicts }}{{ .Path }}: {{ .LocalState }} vs {{ .ContainerState }}{{ "\n" }}{{ end }}'
```

When `--template` is used, the template output is written to stdout. Exit codes remain the same (0 = no conflicts, 1 = conflicts found).

## Related Commands

- [`alca status`](./commands/alca_status.md) — Shows sync conflict information
- [`alca experimental sync check`](./commands/alca_experimental_sync_check.md) — Machine-readable conflict check
- [`alca experimental sync resolve`](./commands/alca_experimental_sync_resolve.md) — Interactive conflict resolution
