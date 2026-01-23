---
title: "List and Cleanup Commands Design"
description: "Design decisions for alca list and alca cleanup commands"
tags: cli, state
updates: AGD-009
---

## Context

With the introduction of container identity stability (state file + labels), we need commands to:
1. View all alca-managed containers across projects
2. Clean up orphaned containers

## Decision

### alca list

Displays all alca-managed containers in a table format.

**Output format**:
```
NAME                STATUS    PROJECT ID    PROJECT PATH                 CREATED
alca-550e8400e29b   running   550e8400e29b  /Users/dev/my-project       2024-01-15T10:30:00
alca-abc123def456   stopped   abc123def456  /old/path                   2024-01-10T08:00:00
```

**Fields**:
- NAME: Container name
- STATUS: running/stopped/unknown
- PROJECT ID: First 12 chars of UUID
- PROJECT PATH: From `alca.project.path` label
- CREATED: Container creation time (truncated to 19 chars)

**Empty state**: "No Alcatraz containers found."

### alca cleanup

Finds and removes orphaned containers interactively.

**Orphan detection** (any condition = orphan):
1. No `alca.project.path` label
2. Project directory does not exist
3. State file (`.alca/state.json`) does not exist
4. Project ID mismatch between container label and state file

**Interaction flow**:
```
Found 2 orphan container(s):

  [1] alca-abc123
      Path: /deleted/path
      Reason: project directory does not exist

  [2] alca-xyz789
      Path: (no path)
      Reason: no project path label

Select containers to delete (comma-separated numbers, or Enter for all):
> 1,2
Removing alca-abc123... done
Removing alca-xyz789... done

Removed 2 container(s).
```

**Input options**:
- Numbers (comma-separated): `1,3,5` - delete selected
- Empty (Enter): delete all orphans
- Ctrl+C: cancel

**Flags**:
- `--all`: Skip interaction, delete all orphans directly

**No orphans**: Silent exit (no output)

**No second confirmation**: Selection is final, deletion proceeds immediately.

## Consequences

### Positive
- Users can see all alca containers at a glance
- Easy cleanup of orphaned containers
- Simple interaction without complex TUI dependencies

### Negative
- No undo for cleanup (containers are permanently removed)
- List requires runtime to be available
