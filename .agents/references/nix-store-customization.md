# Nix Store Customization for Alca

## Goal

Persist only newly installed packages from `nix develop`, not mount the entire `/nix`.
User expects:
- `/nix` keeps image's original content (read-only)
- `nix build`/`nix develop` newly downloaded files go to `/nix-cache` (persistent mount)

## Approach 1: OverlayFS on /nix/store (Recommended)

### Concept

Use Linux OverlayFS to overlay the container image's `/nix/store` (read-only lower layer) with a persistent writable upper layer. New packages downloaded by nix go to the upper layer, which is persisted across container restarts.

### How It Works

```
/nix/store (merged view) = lowerdir (image's /nix/store, readonly) + upperdir (/nix-cache/upper, persistent volume)
```

- **lowerdir**: The original `/nix/store` from the container image (read-only)
- **upperdir**: A persistent volume mounted at e.g., `/nix-cache/upper`
- **workdir**: Required by OverlayFS, same volume as upperdir, e.g., `/nix-cache/workdir`
- **merged**: The actual `/nix/store` that nix sees

### Setup (container entrypoint/init)

```bash
# Create directories on persistent volume
mkdir -p /nix-cache/upper /nix-cache/workdir

# Copy original /nix/store contents aside (first run only)
# Actually not needed - overlayfs reads from lowerdir directly

# Mount overlay
mount -t overlay overlay \
  -o lowerdir=/nix/store-base \
  -o upperdir=/nix-cache/upper \
  -o workdir=/nix-cache/workdir \
  /nix/store
```

**Container image preparation**: The image should have its original `/nix/store` at an alternative path (e.g., `/nix/store-base`) or the overlay mount replaces `/nix/store` at container startup.

Alternative approach without moving the base store:
```bash
# In container init, before nix runs:
mkdir -p /nix-cache/upper /nix-cache/workdir /nix/store-merged

mount -t overlay overlay \
  -o lowerdir=/nix/store \
  -o upperdir=/nix-cache/upper \
  -o workdir=/nix-cache/workdir \
  /nix/store
```

This works because OverlayFS can overlay onto the same mountpoint as the lowerdir source.

### Pros

- **Transparent to nix**: Nix sees a normal `/nix/store` and operates as usual
- **No nix configuration changes**: Standard nix commands work without `--store` flags
- **Efficient storage**: Only new/modified store paths are in the upper layer
- **Fast startup**: No copying needed on subsequent starts
- **Well-understood technology**: OverlayFS is battle-tested in Docker itself

### Cons

- **Requires Linux kernel** (works in alca containers since they run Linux VMs on macOS)
- **Requires CAP_SYS_ADMIN** or user namespace mount privileges in container
- **Upper layer not a valid standalone store**: Dangling references to lower layer paths
- **Garbage collection complexity**: Deleting paths present in lower layer requires whiteout handling

### Alca Integration

For alca, this is the simplest approach:
1. Container image has nix with packages pre-installed at `/nix/store`
2. At container start, alca mounts OverlayFS over `/nix/store`
3. Persistent volume at `/nix-cache` stores the upper layer
4. User runs `nix develop` / `nix build` - new packages go to upper layer
5. Container restart: overlay remounted, new packages available immediately

## Approach 2: Nix Local Overlay Store (Experimental)

### Concept

Nix has an experimental `local-overlay-store` feature that provides overlay semantics at the Nix store level, built on OverlayFS.

### How It Works

```bash
# Enable experimental feature
echo "extra-experimental-features = local-overlay-store" >> /etc/nix/nix.conf

# Set up OverlayFS manually
mount -t overlay overlay \
  -o lowerdir="/nix/store" \
  -o upperdir="/nix-cache/upper" \
  -o workdir="/nix-cache/workdir" \
  "/nix/store-merged"

# Use with nix commands
nix develop --store 'local-overlay://?root=/&lower-store=/nix/store-base&upper-layer=/nix-cache/upper'
```

### Pros

- **Nix-native solution**: Uses nix's own store abstraction
- **Proper metadata handling**: Maintains separate SQLite DB for upper layer
- **RFC-backed design**: [RFC 0152](https://github.com/NixOS/rfcs/pull/152)

### Cons

- **Experimental**: Feature flag required, may change or break
- **Requires OverlayFS setup anyway**: Nix doesn't mount it for you
- **`nix develop` support unclear**: [Issue #5024](https://github.com/NixOS/nix/issues/5024) - chroot stores not fully supported with `nix develop`
- **More complex configuration**: Requires understanding nix store URLs
- **Limited documentation/community experience**

## Approach 3: NIX_STORE_DIR / Custom Store Path

### Concept

Use environment variables or `--store` flag to point nix at a different store location.

### Options

1. **`--store` flag with local store**:
   ```bash
   nix develop --store 'local?store=/nix-cache/store&state=/nix-cache/state&log=/nix-cache/log'
   ```

2. **Chroot store**:
   ```bash
   nix develop --store /nix-cache
   # Physical store at /nix-cache/nix/store
   ```

### Pros

- No OverlayFS required
- Pure nix solution

### Cons

- **Doesn't merge with base image packages**: New store is separate, can't see image's packages
- **`nix develop` doesn't fully support chroot stores** ([Issue #5024](https://github.com/NixOS/nix/issues/5024))
- **Binary cache miss**: Packages already in image's `/nix/store` would be re-downloaded
- **Path incompatibility**: Store paths encode the store directory, can't mix stores with different paths

## Approach 4: Bind-Mount Host Nix Store + Daemon Socket

### Concept

Share the host's `/nix/store` into the container and connect to the host's nix daemon.

```bash
docker run -v /nix:/nix:ro -v /nix/var/nix/daemon-socket:/nix/var/nix/daemon-socket ...
```

### Pros

- Shares cache with host
- No duplication

### Cons

- **Not applicable for alca**: Containers run in VMs, not direct host filesystem access
- **Security concern**: Exposes entire host store to container
- **Read-only mount issues**: Nix tries to remount writable ([Issue #6835](https://github.com/NixOS/nix/issues/6835))

## Approach 5: Nix Profile/Generation Persistence

### Concept

Only persist the nix profiles/generations (symlink trees) and their specific store paths, not the entire store.

### How It Works

```bash
# Persist profile directory
mount -v /nix-cache/profiles /nix/var/nix/profiles

# After nix develop installs packages, only profile-referenced paths need persistence
nix-store --query --requisites /nix/var/nix/profiles/default > /nix-cache/paths.txt
```

### Pros

- Minimal persistence footprint
- Only keeps what's actually used

### Cons

- **Incomplete**: Store paths referenced by profiles still need to be in `/nix/store`
- **Requires rebuilding on restart**: If store paths aren't persisted, they'll be re-downloaded
- **Complex garbage collection**: Need to track which paths are from image vs user

## Recommendation for Alca

### Primary: Approach 1 (OverlayFS on /nix/store)

This is the recommended approach because:

1. **Transparent**: No changes to how nix operates inside the container
2. **Efficient**: Only stores deltas (new packages) in persistent volume
3. **Compatible**: Works with all nix commands including `nix develop`
4. **Simple**: Well-understood filesystem technology
5. **alca-suitable**: Linux containers (even on macOS via Apple Containerization) support OverlayFS

### Implementation Plan for Alca

1. **Container image**: Build with nix packages pre-installed at `/nix/store`
2. **Persistent volume**: Mount at `/nix-cache` in container config
3. **Container init script** (run before user shell):
   ```bash
   #!/bin/bash
   # First run: create directories
   mkdir -p /nix-cache/upper /nix-cache/workdir

   # Mount overlay over /nix/store
   mount -t overlay overlay \
     -o lowerdir=/nix/store \
     -o upperdir=/nix-cache/upper \
     -o workdir=/nix-cache/workdir \
     /nix/store

   # Also persist nix state (DB, profiles, gcroots)
   mkdir -p /nix-cache/var
   if [ ! -d /nix-cache/var/nix ]; then
     cp -a /nix/var/nix /nix-cache/var/
   fi
   mount --bind /nix-cache/var/nix /nix/var/nix
   ```
4. **Config option**: Add `nix_cache_volume` or similar to alca config
5. **Garbage collection**: Periodically run `nix-collect-garbage` inside container to clean upper layer

### Edge Cases

- **First run**: Upper layer empty, all reads come from lower (image) layer
- **Package conflict**: If image updates change `/nix/store`, upper layer may have stale paths. Solution: clear upper layer on image rebuild.
- **DB consistency**: `/nix/var/nix/db` must also be persisted and may need migration if image's DB schema changes.

## References

- [Nix Local Overlay Store (v2.30)](https://nix.dev/manual/nix/2.30/store/types/experimental-local-overlay-store)
- [RFC 0152 - local-overlay store](https://github.com/NixOS/rfcs/pull/152)
- [NixCon 2023 - Layered Nix Stores](https://talks.nixcon.org/nixcon-2023/talk/GXW3EX/)
- [Using OverlayFS for Nix CI builds](https://fzakaria.com/2021/09/10/using-an-overlay-filesystem-to-improve-nix-ci-builds.html)
- [nix develop chroot store issue #5024](https://github.com/NixOS/nix/issues/5024)
- [Nix Issue #2107 - Custom store](https://github.com/NixOS/nix/issues/2107)
- [Sharing Nix store between containers](https://discourse.nixos.org/t/sharing-nix-store-between-containers/9733)
- [Snix as lower Nix Overlay Store](https://snix.dev/blog/snix-as-lower-nix-overlay-store/)
