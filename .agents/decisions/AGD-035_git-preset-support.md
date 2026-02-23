---
title: "Git Preset Support in alca init"
description: "Support fetching config presets from git repos via alca init, with source tracking via embedded comments"
tags: config, cli
updates: AGD-009, AGD-033
---

## Context

Teams want to share alcatraz configuration presets across projects. Possible approaches considered:

1. **Direct HTTP/HTTPS in extends/includes** — Rejected. Mixes network I/O into config loading, breaks offline usage, adds latency to `alca up`, and creates supply chain risk (remote configs can specify commands, mounts, capabilities).
2. **Scheme prefix (e.g. `github:`) in extends/includes** — Rejected. Same architectural problems: config loading pipeline (`loadFileRefs`) is filesystem-oriented (`afero.Fs`, `filepath.*`, `afero.Glob`), and inserting network resolution mid-pipeline adds complexity with cache management, fallback behavior, and URL-vs-path dispatch.
3. **Download presets as local files via `alca init`** — Chosen. Separates "fetching remote content" from "loading config" completely. Config loading stays local and untouched. Network operations only happen during explicit user commands.

### URL Format Rationale

Considered `github:<org>/<repo>` shorthand but rejected in favor of a full URL-based format. The `git+<protocol>://` convention is established (pip, npm), supports any git host without scheme proliferation, and makes the protocol explicit.

For separating the clone URL from the in-repo directory path, several approaches were considered:
- **`:<dir-path>` suffix** — Rejected. Conflicts with URL port syntax and `user:pass@` credentials, making parsing ambiguous.
- **`//<dir-path>` double-slash** — Rejected. While used by Terraform, it's visually confusing given `://` already in the URL.
- **`?dir=<path>` query parameter** — Rejected. Mixes client-side semantics with URL query params meant for servers.
- **`--dir` CLI flag** — Rejected. Separates related information across the URL and a flag.
- **`#<commit>:<dir-path>` in fragment** — Chosen. Following Docker build context convention. The URL fragment is opaque to parsers, so `:` inside it has no special meaning. `net/url` returns the fragment as a single string; we split on the first `:` ourselves.

## Decision

### URL Format

```
git+<clone-url>[#[commit-hash]:[dir-path]]
```

The `<clone-url>` is a standard git clone URL (the same URL you'd pass to `git clone`), prefixed with `git+`. Both with and without `.git` suffix are supported. The optional `#` fragment encodes commit hash and/or directory path, separated by `:`.

Fragment variations:
- `#<commit>:<dir>` — both commit and directory
- `#<commit>` — commit only, scan repo root
- `#:<dir>` — directory only, use latest HEAD
- (no fragment) — latest HEAD, scan repo root

Parsing: strip `git+` prefix, parse with `net/url`, split `.Fragment` on first `:`.

Examples:

```bash
# GitHub public, repo root (both forms equivalent)
alca init git+https://github.com/myorg/presets.git
alca init git+https://github.com/myorg/presets

# Subdirectory only
alca init git+https://github.com/myorg/presets.git#:backend

# Commit only
alca init git+https://github.com/myorg/presets.git#a1b2c3d

# Both subdirectory and commit
alca init git+https://github.com/myorg/presets.git#a1b2c3d:backend

# GitLab nested group
alca init git+https://gitlab.com/group/subgroup/configs#:alcatraz

# Private repo via SSH
alca init git+ssh://git@github.com/myorg/private-presets.git#:backend

# Self-hosted with token
alca init git+https://token@gitea.company.com/team/presets#:frontend
```

### Two Modes for `alca init`

- `alca init` (no args): existing template flow (Debian/Nix selection, creates `.alca.toml`) — unchanged
- `alca init git+<protocol>://...`: preset flow — downloads supplementary `.alca.*.toml` files from a remote repo

Modes are distinguished by argument presence. The preset flow does NOT create `.alca.toml` — it creates supplementary files that users reference via `extends`/`includes`.

### Preset Flow

1. Clone/fetch the repo to cache (see Cache section)
2. If commit hash specified in fragment, checkout that commit
3. Scan the target directory (repo root or dir-path from fragment) non-recursively for files matching `.alca.*.toml` and `.alca.*.toml.example`
4. Present an interactive multi-select list (using `huh`) showing found files
   - `.example` files shown with a hint that the `.example` suffix will be kept as-is
5. For each selected file:
   - If a local file with the same name exists: prompt to overwrite or skip
   - Download to current directory (flattened, no subdirectory structure preserved)
   - Insert source comment as line 1 of the file

### Source Comment Format

```
# Alcatraz Preset Source: git+<clone-url>#<commit-hash>:<filepath>
```

- Stores the clone URL as provided by the user (including credentials if present — see Security section)
- `<filepath>` is the full in-repo path to the file (dir-path + filename)
- `<commit-hash>` is always present — pinned to the repo's HEAD (or specified commit) at fetch time
- Must appear within the leading comment block (consecutive `#` lines from top of file)
- When writing: always line 1
- When parsing: scan consecutive comment lines from top, stop at first non-comment line

Note: the source comment fragment format is `#<commit>:<filepath>` (always has both commit and filepath), even though the CLI input fragment may omit either. The source comment is the fully resolved form.

Users can manually add this comment to any `.alca.*.toml` file to opt in to managed updates. Removing the comment opts out.

### Update Flow: `alca init --update`

1. Scan all `.alca.*.toml` files in current directory for source comments
2. Group by source repo
3. For each repo: `git pull` in the cached clone; on network failure, warn and use stale cache
4. For each file:
   - Source file still exists in repo: overwrite local file unconditionally, update (or add) commit hash in source comment to the new HEAD
   - Source file deleted from repo: warn, leave local file untouched
5. Silently succeed if no files with source comments are found

The `#commit-hash` in the source comment is a record of what version is currently downloaded, not a lock. `--update` always fetches latest and rewrites the hash.

### Cache Layout

Cache location: `~/.alcatraz/cache-presets/<host>/<protocol>/<credentials>/<repo-path>/`

Normalization rules for cache path derivation:
- Protocol: invalid path characters replaced with `-` (e.g., `git+https` → `git-https`)
- Credentials: included as a path component; invalid path characters replaced with `-`. No credentials → use `-` as placeholder (keeps path structure consistent)
- `.git` suffix: NOT stripped. Instead, `.` replaced with `-` (e.g., `presets.git` → `presets-git`). `presets.git` and `presets` are different repos and get separate cache directories.
- Fragment (`#...`): not part of cache path (it's per-invocation, not per-repo)

Examples:

```
git+https://github.com/myorg/presets.git
  → ~/.alcatraz/cache-presets/github.com/git-https/-/myorg/presets-git/

git+https://github.com/myorg/presets
  → ~/.alcatraz/cache-presets/github.com/git-https/-/myorg/presets/

git+ssh://git@github.com/myorg/presets
  → ~/.alcatraz/cache-presets/github.com/git-ssh/git/myorg/presets/

git+https://token@gitea.company.com/team/presets
  → ~/.alcatraz/cache-presets/gitea.company.com/git-https/token/team/presets/

git+https://user:pass@gitea.company.com/team/presets
  → ~/.alcatraz/cache-presets/gitea.company.com/git-https/user-pass/team/presets/
```

Different protocols and different credentials get separate cache directories.

### Fetch Strategy

Always fetch only a single commit to minimize bandwidth and disk usage:

- **With `#commit-hash`**: fetch that specific commit only (`git fetch --depth 1 origin <hash>`)
- **Without `#commit-hash`** (or `--update`): fetch only the latest commit of the default branch (`git fetch --depth 1 origin HEAD`)

Note: fetching by commit hash requires the server to support `uploadpack.allowReachableSHA1InWant`. GitHub, GitLab, and Bitbucket all support this. Some self-hosted Git servers may not — in that case the fetch fails with a clear error.

| Scenario | Action |
|---|---|
| `alca init git+...`, no cache | `git init` + `git fetch --depth 1 origin HEAD` |
| `alca init git+...`, cache exists | `git fetch --depth 1 origin HEAD` |
| `alca init git+...#hash:...`, no cache | `git init` + `git fetch --depth 1 origin <hash>` |
| `alca init git+...#hash:...`, cache exists, commit present | Checkout only (no network) |
| `alca init git+...#hash:...`, cache exists, commit missing | `git fetch --depth 1 origin <hash>` |
| `alca init --update`, cache exists | `git fetch --depth 1 origin HEAD`, fall back to stale cache on network failure |
| `alca init --update`, no cache | `git init` + `git fetch --depth 1 origin HEAD` |

No automatic cache cleanup — users can delete `~/.alcatraz/cache-presets/` manually.

### Security: Inline Credentials

The URL format supports `user:pass@` for private repos behind token auth. **The source comment stores the full URL as provided, including credentials.**

Documentation must include a prominent warning:

> **Security Warning**: If you include credentials in the URL (e.g., `git+https://token@...`), they will be stored in the source comment of downloaded files. These files may be committed to version control. Use git credential helpers, SSH keys, or `.netrc` instead unless you understand the implications.

The CLI should also print a warning when it detects `user:pass@` in the URL during `alca init`.

### Typical Usage

```bash
# Fetch presets from a shared repo
alca init git+https://github.com/myorg/alca-presets

# Select .alca.node.toml from the interactive list
# File is downloaded with source comment:
# # Alcatraz Preset Source: git+https://github.com/myorg/alca-presets#a1b2c3d:.alca.node.toml

# Reference it in your .alca.toml
# extends = [".alca.node.toml"]

# Later, update all presets to latest
alca init --update

# Pin to a specific commit for reproducibility
alca init git+https://github.com/myorg/alca-presets#a1b2c3d

# Scan a subdirectory
alca init git+https://github.com/myorg/alca-presets#:backend
```

## Consequences

### Positive

- Config loading pipeline (`loadFileRefs`) is completely untouched — no architectural changes
- Network operations only during explicit user commands — offline always works for `alca up`
- Downloaded files are local, visible, inspectable, and version-controllable
- Source comment is self-documenting — no separate lockfiles or metadata
- `git+<protocol>://` format supports any git host without scheme proliferation
- Docker-style `#commit:dir` fragment is parseable with standard `net/url` + minimal custom logic
- Composable with env var expansion and extends/includes (AGD-033)
- Interactive selection via `huh` is consistent with existing `alca init` UX

### Negative

- One extra step compared to inline URL references (download first, then reference in config)
- Cache directory (`~/.alcatraz/cache-presets/`) is a new piece of state outside the project
- Unconditional overwrite on `--update` means local edits to managed files are lost — users must remove the source comment to opt out
- Unpinned refs (`git pull` on every `--update`) may fetch unexpected changes
