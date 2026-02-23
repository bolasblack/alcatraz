---
title: Presets
weight: 2.3
---

# Presets

Presets are shared configuration files fetched from git repositories. They let teams maintain common `.alca.*.toml` files in a central repo and distribute them across projects using `alca init`.

Downloaded preset files are regular local files — reference them via [`extends` or `includes`]({{< relref "extends-includes" >}}) in your `.alca.toml`.

## Usage

```bash
# Fetch presets from a shared repo
alca init git+https://github.com/myorg/alca-presets

# Scan a subdirectory
alca init git+https://github.com/myorg/alca-presets#:backend

# Pin to a specific commit
alca init git+https://github.com/myorg/alca-presets#a1b2c3d

# Both subdirectory and commit
alca init git+https://github.com/myorg/alca-presets#a1b2c3d:backend

# SSH
alca init git+ssh://git@github.com/myorg/private-presets.git#:backend

# Update all presets to latest
alca init --update
```

## URL Format

```
git+<clone-url>[#[commit-hash]:[dir-path]]
```

The `<clone-url>` is a standard git clone URL (the same URL you'd pass to `git clone`), prefixed with `git+`. Both with and without `.git` suffix are supported.

The optional `#` fragment encodes a commit hash and/or directory path, separated by `:`:

| Fragment | Commit | Directory |
| --- | --- | --- |
| `#a1b2c3d:backend` | Pinned to `a1b2c3d` | Scan `backend/` |
| `#a1b2c3d` | Pinned to `a1b2c3d` | Scan repo root |
| `#:backend` | Latest HEAD | Scan `backend/` |
| *(none)* | Latest HEAD | Scan repo root |

Examples:

```bash
# GitHub public repo
alca init git+https://github.com/myorg/presets

# GitLab nested group, subdirectory
alca init git+https://gitlab.com/group/subgroup/configs#:alcatraz

# Private repo via SSH
alca init git+ssh://git@github.com/myorg/private-presets.git#:backend

# Self-hosted with token
alca init git+https://token@gitea.company.com/team/presets#:frontend
```

## How It Works

1. **Fetch** — The repo is cloned (or updated) into a local cache
2. **List** — The target directory is scanned for `.alca.*.toml` and `.alca.*.toml.example` files
3. **Select** — An interactive multi-select list lets you choose which files to download
4. **Download** — Selected files are written to the current directory with a source comment prepended as line 1

`.example` files (e.g., `.alca.node.toml.example`) are templates meant to be copied and customized. They are downloaded as-is with the `.example` suffix preserved — rename them to `.alca.*.toml` after editing.

The source comment records where the file came from:

```
# Alcatraz Preset Source: git+<clone-url>#<commit-hash>:<in-repo-filepath>
```

For example:

```
# Alcatraz Preset Source: git+https://github.com/myorg/alca-presets#a1b2c3d:backend/.alca.node.toml
```

Note: the fragment in source comments uses `#commit:filepath` (the full in-repo file path), which differs from the URL format's `#commit:dirpath` (a directory to scan).

After downloading, reference the preset in your `.alca.toml`:

```toml
extends = [".alca.node.toml"]
```

## Updating Presets

Run `alca init --update` to update all managed preset files in the current directory:

1. Scans all `.alca.*.toml` files for source comments
2. Fetches the latest commit from each source repo
3. Overwrites local files with the latest version and updates the commit hash in the source comment

Behavior notes:

- If the source file was deleted from the remote repo, a warning is printed and the local file is left untouched
- If no files with source comments are found, the command succeeds silently
- Removing the source comment from a file opts it out of managed updates
- On network failure, the stale cache is used as a fallback

> **Security Warning**: If you include credentials in the URL (e.g., `git+https://token@...`),
> they will be stored in the source comment of downloaded files. These files may be committed
> to version control. Use SSH keys, git credential helpers, or `.netrc` instead unless you
> understand the implications.

## Cache

Preset repositories are cached locally at `~/.alcatraz/cache-presets/`.

- Shallow clones (depth 1) are used to minimize bandwidth and disk usage
- Each unique combination of host, protocol, and credentials gets its own cache directory
- No automatic cleanup — delete manually if needed:

```bash
rm -rf ~/.alcatraz/cache-presets/
```

See [AGD-035](https://github.com/bolasblack/alcatraz/blob/master/.agents/decisions/AGD-035_git-preset-support.md) for design rationale.
