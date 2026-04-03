---
name: "release-note"
description: "Generate changelog entries for alcatraz releases following Common Changelog conventions. Use when preparing a release, writing changelogs, creating release notes, or when user mentions release-note, changelog, or release preparation."
user_invocable: true
---

# Release Note Generator

Generate a changelog entry for an alcatraz release in `docs/changelogs/v{VERSION}.md`.

## Arguments

- Optional version string (e.g., `v0.3.0`). If omitted, detect the next version using `svu next`.

## Instructions

### Step 1: Determine Version

If a version argument was provided, use it (strip leading `v` for the heading, keep it for the filename).

If no version was provided, run:

```bash
svu next
```

If `svu` is not available, ask the user for the version.

### Step 2: Calculate Weight

Weight determines sort order in the docs site (newer versions sort first).

Formula: `-(major × 100 + minor × 10 + patch)`. Examples:
- v0.1.0 → weight: -10
- v0.2.2 → weight: -22
- v0.3.0 → weight: -30
- v1.0.0 → weight: -100
- v1.2.3 → weight: -123

### Step 3: Gather Changes

Run these commands to understand what changed since the last release:

```bash
# Find the last release tag
git describe --tags --abbrev=0

# Commits since last tag (one-line format)
git log $(git describe --tags --abbrev=0)..HEAD --oneline
```

Read individual commits (via `git show`) as needed when writing entries — don't bulk-diff upfront.

### Step 4: Read Style Reference

Read the most recent existing changelog for style reference:

```bash
ls docs/changelogs/v*.md
```

Read the latest one to match formatting conventions exactly.

### Step 5: Filter and Classify Changes

**Include only user-facing changes:**
- New features, CLI commands, config options
- Bug fixes that affect user behavior
- Breaking changes to existing behavior
- New installation methods or platform support

**Exclude:**
- Internal refactoring, code reorganization
- Test infrastructure changes
- CI/CD pipeline changes
- Developer tooling changes

**Classify into categories (in this order, only include non-empty):**
1. Changed (breaking or behavioral changes)
2. Added (new features)
3. Removed (removed features)
4. Fixed (bug fixes)

### Step 6: Write Each Entry

Each entry must be:
- **Imperative mood, self-describing** — makes sense without its category heading
- **Linked to commit** — include short SHA linked to GitHub commit URL
- **With code example** — copy-pasteable CLI command or TOML snippet when applicable
- **Linked to docs** — use portable relative links (e.g., `../config/fields.md`)

Entry format:
```markdown
- Brief imperative description ([`abcdef0`](https://github.com/bolasblack/alcatraz/commit/abcdef0))

  ```toml
  # Code example if applicable
  ```

  Additional context. See [Relevant Docs](../config/relevant-page.md).
```

### Step 7: Generate the File

Write `docs/changelogs/v{VERSION}.md` with this structure:

```markdown
---
title: v{VERSION}
weight: {WEIGHT}
---

## [{VERSION_NO_V}] - {YYYY-MM-DD}

### Added

- Entry 1 ...
- Entry 2 ...

[{VERSION_NO_V}]: https://github.com/bolasblack/alcatraz/releases/tag/v{VERSION}
```

### Step 8: Next Steps

**Do NOT commit the changelog file.** After generating, tell the user:

1. Review the generated changelog for accuracy
2. Suggest the appropriate release command based on the changes:
   - **New features** → `make release-minor`
   - **Bug fixes only** → `make release-patch`
   - **Breaking changes** → `make release-major`
3. The release target handles everything: updates `flake.nix` version, commits, tags, and pushes
4. CI automatically generates GitHub Release notes from the changelog (`make release-notes` runs in `.github/workflows/release.yml`)

## Important Rules

- **Portable links only** — use `../config/fields.md` not absolute URLs. Hugo resolves them via BookPortableLinks. `make release-notes` converts them to absolute URLs for GitHub Releases.
- **Today's date** — use the current date for the changelog entry.
- **One entry per feature** — don't split a feature across multiple entries. Group related commits.
- **No duplicate files** — check if the changelog file already exists before creating. If it exists, ask the user whether to overwrite or append.

## Version History

- v1.2.0 (2026-04-03): Fix weight formula to -(major×100+minor×10+patch); drop upfront --stat diff
- v1.0.0 (2026-03-02): Initial version
