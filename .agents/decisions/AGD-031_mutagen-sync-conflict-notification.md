---
title: "Mutagen Sync Conflict Detection, Notification, and Resolution"
description: "Surface mutagen two-way-safe sync conflicts to users via stderr banners, status integration, and interactive resolution, while keeping mutagen invisible"
tags: cli, file-isolation, runtime
updates: AGD-025
---

## Context

AGD-025 introduced mutagen for bidirectional file sync with mount excludes. Mutagen's default `two-way-safe` mode can produce file conflicts when both host and container modify the same file simultaneously. These conflicts are silently recorded by mutagen — the conflicted files retain their local version on each side, but neither propagates.

Currently alca has **zero visibility** into sync conflicts. Users have no way to discover them through alca, and no way to resolve them without knowing mutagen internals. Non-conflicted files continue syncing normally, so conflicts can go unnoticed until the user realizes their changes aren't appearing on the other side.

### Conflict Scenarios (from mutagen's three-way merge)

| Scenario | Result |
|----------|--------|
| Both sides edit same file | CONFLICT |
| Both sides create same file (different content) | CONFLICT |
| One side deletes, other edits | Auto-resolved (edit wins, data preserved) |
| Both sides only delete | Auto-resolved |
| Session recreated while both sides diverged | Many false conflicts |

### Design Constraints

- alca is CLI-first — no GUI, no desktop notifications
- Users should never see the word "mutagen" or concepts like alpha/beta/flush/session
- Conflicts are rare but critical — must be impossible to miss
- Detection requires polling `mutagen sync list` (no push/callback mechanism exists)
- alca commands are short-lived processes with no persistent memory

## Decision

### Detection

Poll mutagen via `mutagen sync list --template='{{ json . }}' <session-name>` to get structured conflict data. Parse the JSON `conflicts` array per session.

### Cache: SWR (Stale-While-Revalidate) Pattern

Since alca commands are independent processes, use a disk cache at `<projectRoot>/.alca/sync-conflicts-cache.json` following the SWR pattern:

- **Every alca command**: read stale cache for stderr banner, then trigger **async** (non-blocking) background cache update
- **Sync-update commands** (poll fresh, block until result): `alca status`, `alca experimental sync check`, `alca experimental sync resolve`
- **No cache / first run**: show no banner, trigger async update for next time
- Cache format: JSON with conflict list + `updatedAt` timestamp

### Notification: stderr Warning Banner

After any alca command that involves an active sync session, append a warning to **stderr** if the (stale) cache contains conflicts:

```
⚠ 2 sync conflicts need attention:
  src/config.yaml    (modified on both sides)
  data/users.json    (modified locally, deleted in container)
Run 'alca experimental sync resolve' to resolve.
```

- Output to stderr (visible even when stdout is piped)
- Yellow/warning color (not red/error)
- Max 3 conflict paths inline, then `...and N more`
- Always end with the actionable command

### Status Integration

`alca status` shows conflict info with **synchronous** cache update (always fresh):

```
Container: running (my-project)
Sync: active, 1,247 files synced

⚠ 2 sync conflicts:
  src/config.yaml    (modified on both sides)
  data/users.json    (modified locally, deleted in container)

Run 'alca experimental sync resolve' to resolve.
```

### Resolution: `alca experimental sync resolve`

Interactive, one conflict at a time, three options only. Uses [charmbracelet/huh](https://github.com/charmbracelet/huh) for the selection UI (consistent with other interactive prompts in alca).

Each conflict presents a `huh.Select` with three options: local overwrites container, container overwrites local, skip.

Resolution mechanics (hidden from user):
- "Local overwrites container" → delete file on container side, mutagen propagates local version
- "Container overwrites local" → delete file on host side, mutagen propagates container version
- "Skip" → leave conflict unresolved

After all conflicts processed, show summary and synchronously update cache.

### Check: `alca experimental sync check`

Machine-readable conflict check. Exit 0 = no conflicts, exit 1 = conflicts exist. Always synchronous (fresh poll). Intended for scripting.

Supports `--template` flag (Go `text/template`) for custom output formatting, enabling scriptability (e.g., JSON output via `--template='{{ json . }}'`).

### Exit Codes on Existing Commands

**No change.** Existing commands (`up`, `run`, `down`) keep their current exit code semantics. Conflicts are communicated via stderr banner only.

### Command Namespace

All new sync commands under `experimental`:

| Command | Purpose | Cache behavior |
|---------|---------|----------------|
| `alca experimental sync check` | Machine-readable conflict check | Sync update |
| `alca experimental sync resolve` | Interactive resolution | Sync update after each resolution |

### Terminology

| Mutagen concept | alca terminology |
|----------------|-----------------|
| alpha | "local" / "your machine" |
| beta | "container" |
| two-way-safe conflict | "sync conflict" |
| sync session / flush / endpoint | Never mentioned |

The word "mutagen" never appears in user-facing output.

## Consequences

### Positive

- Users discover conflicts through normal alca usage (stderr banners on every command)
- Resolution is guided and simple — no mutagen knowledge required
- SWR pattern avoids adding latency to every command (async update, stale read)
- `alca status` gives a complete picture including sync health
- Machine-readable `sync check` enables scripting and CI integration
- Commands under `experimental` allow iteration before stabilizing the API

### Negative

- SWR means banner may show stale data (conflict already resolved, or new conflict not yet shown) — acceptable tradeoff for zero-latency UX
- Polling is the only detection method (mutagen has no push mechanism)
- Resolution via file deletion depends on mutagen detecting the change and propagating — small latency window
- If container is stopped, conflicts can be detected but not resolved

### Implementation Notes

- New module: `internal/sync/` with `Env` receiving `Fs` + `CommandRunner` via DI (AGD-029)
- Add `ListSessionJSON()` to `internal/runtime/mutagen.go` for structured session data
- Cache file in `<projectRoot>/.alca/sync-conflicts-cache.json`
- Mutagen v0.15+ required for `--template` JSON output (already enforced: v0.18.1+ on macOS)
