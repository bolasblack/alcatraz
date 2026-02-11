---
title: Mount Exclude Implementation with Mutagen
description: Use Mutagen file sync for mount filtering instead of FUSE, with platform-specific optimization
tags: file-isolation, config, macos, linux, runtime
updates: AGD-009
updated_by: AGD-031
---


## Context

We need to implement a `mount_excludes` feature that allows users to exclude specific files/directories from being visible inside the container. Requirements:

1. **Bidirectional real-time sync**: Project directory mounted into container, read-write, changes sync both ways
2. **Files completely invisible**: Excluded files must not exist (not empty files, which would cause parsing errors)
3. **Excluded directories can be empty**: Acceptable behavior
4. **New files at excluded paths persist**: If user creates a file at an excluded path inside container, it should persist (orphan storage)
5. **Orphan detection on down**: Warn user about orphaned files when stopping
6. **Auto-sync new files**: New files on host automatically appear in container (no manual reload)

### Why Not FUSE?

FUSE (Filesystem in Userspace) was the initial candidate because it can implement a "filtering filesystem" that:

- Passes through all read/write to underlying directory
- Filters out excluded items in `lookup`/`readdir`
- Redirects writes to excluded paths to orphan storage

However, **FUSE is blocked on macOS** due to Docker's VM-based architecture.

## Alternatives Considered

### Alternative A: FUSE Sidecar + Mount Propagation

```
Sidecar Container: FUSE daemon → /workspace-filtered
Main Container: sees /workspace-filtered via mount propagation
```

**Result**: ❌ Failed

macOS Host → Linux VM file sharing uses VirtioFS/osxfs with `private` propagation mode. This is a VM-level limitation that cannot be changed. Tested configurations:

| Test                    | Result | Reason                                              |
| ----------------------- | ------ | --------------------------------------------------- |
| bind mount + rshared    | ❌     | VM mount namespace is private                       |
| Docker volume + rshared | ❌     | volumes don't support propagation                   |
| volumes_from            | ❌     | only inherits volume definition, not runtime mounts |

References:

- [docker/for-mac#3431](https://github.com/docker/for-mac/issues/3431)
- [moby/moby#39093](https://github.com/moby/moby/issues/39093)

### Alternative B: FUSE Sidecar + NFS Export

```
Sidecar Container: FUSE daemon + NFS server
Main Container: NFS client mounts from sidecar
```

**Result**: ❌ Failed

Linux kernel NFS server cannot export FUSE filesystems. Error: `exportfs: /filtered does not support NFS export`

NFS export requires `export_operations` kernel interface that FUSE doesn't implement.

### Alternative C: FUSE Sidecar + 9P/SSHFS

```
Sidecar Container: FUSE daemon + 9P/SSHFS server
Main Container: 9P/SSHFS client
```

**Result**: ⚠️ Technically possible, but poor performance

9P over TCP is ~3x slower than SSHFS, and SSHFS is ~50% of NFS performance. For `node_modules` (30k-100k files) this is unacceptable:

| Operation       | Native | Network FS |
| --------------- | ------ | ---------- |
| `npm install`   | 30s    | 5-15 min   |
| `git status`    | 0.1s   | 2-10s      |
| webpack rebuild | 1s     | 10-60s     |

### Alternative D: Host FUSE Daemon

```
macOS Host: alca runs FUSE daemon → /tmp/filtered
Docker: mounts /tmp/filtered
```

**Result**: ⚠️ Technically possible, but complex

Issues:

- Daemon lifecycle management (orphan processes)
- macFUSE dependency on macOS
- Apple Silicon requires lowering system security (macOS < 26)
- Crash recovery complexity

### Alternative E: tmpfs Overlay

```bash
docker run \
  -v /host/project:/workspace \
  --mount type=tmpfs,dst=/workspace/secrets
```

**Result**: ⚠️ Partial solution

- ✅ Directory exclusion: empty directory (acceptable)
- ❌ File exclusion: /dev/null overlay → empty file (causes parsing errors)
- ❌ Cannot achieve "file completely invisible"

### Alternative F: Mutagen File Sync

```
Host: /project
  ↓ Mutagen sync (--ignore patterns)
Container: /workspace (ext4 volume)
```

**Result**: ✅ Chosen solution

## Decision

Use **Mutagen** for mount filtering with platform-specific optimization:

### Platform Strategy

| Platform                   | Condition    | Mount Method | Status       |
| -------------------------- | ------------ | ------------ | ------------ |
| macOS + Docker Desktop     | Always       | Mutagen      | ✅ Supported |
| macOS + OrbStack           | Has excludes | Mutagen      | ✅ Supported |
| macOS + OrbStack           | No excludes  | Bind mount   | ✅ Supported |
| Linux + Docker             | Has excludes | Mutagen      | ✅ Supported |
| Linux + Docker             | No excludes  | Bind mount   | ✅ Supported |
| Linux + Rootful Podman     | Has excludes | Mutagen      | ✅ Supported |
| Linux + Rootful Podman     | No excludes  | Bind mount   | ✅ Supported |
| Linux + Rootless Podman    | Has excludes | —            | ❌ Blocked   |
| Linux + Rootless Podman    | No excludes  | Bind mount   | ✅ Supported |

**Rationale**:

- Docker Desktop (free) on macOS has poor bind mount performance (~35% of native), Mutagen brings it to ~90-95%
- OrbStack already achieves 75-95% native performance, Mutagen overhead unnecessary without excludes
- Linux bind mounts are native performance (100%), Mutagen adds sync latency (50-200ms)

### Rootless Podman Limitation

**Mutagen does not work reliably with rootless Podman** due to:

1. **Label length limit**: Mutagen follows Kubernetes conventions with a 63-character label limit. Rootless Podman uses long socket paths like `/home/[username]/.local/share/containers/storage/...` which easily exceed this limit.

2. **No native Podman transport**: Mutagen's transport layer is designed for Docker. It requires Podman's Docker compatibility layer, which has inconsistencies in rootless mode.

3. **Volume creation failures**: When Mutagen creates volumes for sync, the label length issue causes volume creation to fail.

**Podman Native Alternatives Evaluated**

Podman has some mount features that Docker doesn't have (like glob mount), but none meet our exclude requirements:

| Approach                 | Result | Issue                                 |
| ------------------------ | ------ | ------------------------------------- |
| Glob mount (`type=glob`) | ❌     | Include-only pattern, not exclude     |
| Overlay mount (`:O`)     | ❌     | Files still visible, only protects host |
| tmpfs overlay            | ❌     | Empty file/dir, not "not exist"       |
| Multiple bind mounts     | ⚠️     | Manual, doesn't scale                 |

**Implementation**: Block `alca up` when mount excludes are configured on rootless Podman, with clear error message offering alternatives:
1. Remove `exclude` from mount configuration
2. Use rootful Podman
3. Use Docker instead

### Mutagen Integration

- **CLI wrapper**: Shell out to `mutagen` binary (stable interface)
- **No Go SDK**: Mutagen's internal API is unstable, CLI is the supported interface
- **Daemon management**: Mutagen daemon auto-starts on first sync command

### Performance Characteristics

| Metric                | Mutagen               | Native Bind |
| --------------------- | --------------------- | ----------- |
| Sync latency          | 50-200ms              | 0ms         |
| Throughput (synced)   | ~95% native           | 100%        |
| `node_modules` safety | ✅ (via ignore)       | N/A         |
| Storage               | 2x (host + container) | 1x          |

### Configuration Format

#### workdir_exclude

Shorthand for excluding patterns from the workdir mount:

```toml
image = "ubuntu:latest"
workdir = "/workspace"
workdir_exclude = ["node_modules", ".git", "dist"]
```

During config normalization (`LoadConfig`), the workdir is inserted as `Mounts[0]`:

```go
MountConfig{Source: ".", Target: cfg.Workdir, Exclude: cfg.WorkdirExclude}
```

This eliminates special-casing in the runtime layer — all mounts (including workdir) are processed uniformly. The `-w` flag is still set separately from `cfg.Workdir`.

A mount targeting the same path as workdir is rejected as a conflict (use `workdir_exclude` instead).

#### mounts with excludes

Extend `mounts` to support both simple strings and detailed objects:

```toml
# Simple format (existing, backward compatible)
mounts = [
  "/host/path:/container/path",
]

# Extended format with excludes
[[mounts]]
source = "/Users/me/project"
target = "/data"
readonly = false
exclude = [
  "**/.env.prod",
  "**/secrets/",
]
```

**Field definitions**:

| Field      | Type     | Default  | Description              |
| ---------- | -------- | -------- | ------------------------ |
| `source`   | string   | required | Host path                |
| `target`   | string   | required | Container path           |
| `readonly` | bool     | `false`  | Read-only mount          |
| `exclude`  | []string | `[]`     | Glob patterns to exclude |

**Glob pattern syntax**: Follows Mutagen's ignore syntax (gitignore-like)

- `**/` matches any directory depth
- `*.ext` matches files with extension
- `dir/` matches directory

### Mutagen Availability Check

When any mount requires Mutagen (determined by `ShouldUseMutagen`), `alca up` validates that the `mutagen` binary is available before proceeding. If not found, the command fails early with an install link.

### Recommended Default Excludes

For Node.js development, recommend excluding:

```toml
exclude = [
  "node_modules/",      # Container runs its own npm install
  ".pnpm-store/",       # pnpm cache
  "dist/",              # Build output
  ".next/",             # Next.js
  ".nuxt/",             # Nuxt
]
```

**Note**: `node_modules` and `.git` should almost always be excluded for performance. Syncing these directories through any mechanism (Mutagen, VirtioFS, or network FS) severely degrades development experience.

### Runtime Detection

```go
func DetectRuntime() string {
    if runtime.GOOS == "linux" {
        return "docker-engine"
    }
    // macOS: check for OrbStack
    if isOrbStack() {
        return "orbstack"
    }
    return "docker-desktop"
}

func isOrbStack() bool {
    out, _ := exec.Command("docker", "context", "show").Output()
    return strings.Contains(string(out), "orbstack")
}
```

## Consequences

### Positive

- **Cross-platform**: Works on Linux, macOS Docker Desktop, and OrbStack
- **Native performance**: Container operations at ext4 speed after sync
- **File watching works**: Unlike network FS, Mutagen syncs to real ext4, so inotify/fsevents work
- **No kernel dependencies**: No macFUSE, no kernel extensions
- **Graceful degradation**: Falls back to bind mount when no excludes needed

### Negative

- **Sync latency**: 50-200ms delay between host save and container visibility (acceptable for HMR)
- **Double storage**: Files exist on both host and container volume
- **External dependency**: Requires `mutagen` binary installed
- **Complexity**: More moving parts than simple bind mount

### Exclude Change Strategy

When exclude patterns change, drift detection triggers a **full container rebuild** (current behavior). This is intentional:

- **Adding excludes** (more patterns): Mutagen-only refresh would suffice (files just stop syncing), but rebuild is simpler and safe
- **Removing excludes** (fewer patterns): Previously excluded files may have diverged between host and container. Mutagen uses `two-way-safe` mode — same-name files with different content become **conflicts** that require manual resolution. Rebuilding avoids this entirely since the new container starts fresh

**Future optimization**: For "adding excludes" only, a Mutagen-only session refresh (terminate + recreate) could skip container rebuild. "Removing excludes" should always rebuild to avoid conflicts.

### Future Work

- Orphan file detection: Track files created at excluded paths inside container
- Mutagen health monitoring: Detect and recover from sync failures
- Mutagen-only refresh: Skip container rebuild when only adding exclude patterns (see Exclude Change Strategy above)

## Appendix: Ideal FUSE Sidecar Solution (Blocked)

If FUSE mount propagation worked on macOS, the ideal architecture would be pure **FUSE Sidecar + Mount Propagation** — no NFS needed:

### Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                        macOS Host                            │
│                                                              │
│   /Users/project/  ─────────────────────────────┐            │
│                          VirtioFS               │            │
│   ┌─────────────────────────────────────────────┼──────────┐ │
│   │                  Linux VM                   │          │ │
│   │                                             ▼          │ │
│   │  ┌──────────────────────────────────────────────────┐  │ │
│   │  │              Sidecar Container                   │  │ │
│   │  │                                                  │  │ │
│   │  │  /source (bind mount from host)                  │  │ │
│   │  │      │                                           │  │ │
│   │  │      ▼                                           │  │ │
│   │  │  ┌────────────────────┐                          │  │ │
│   │  │  │  FUSE Filter       │                          │  │ │
│   │  │  │  - hide .env       │                          │  │ │
│   │  │  │  - hide secrets/   │                          │  │ │
│   │  │  │  - passthrough rest│                          │  │ │
│   │  │  └────────┬───────────┘                          │  │ │
│   │  │           ▼                                      │  │ │
│   │  │  /shared/filtered (FUSE mount, rshared)          │  │ │
│   │  │                                                  │  │ │
│   │  └──────────────────────────────────────────────────┘  │ │
│   │              │                                         │ │
│   │              │ Mount propagation (rshared → rslave)    │ │
│   │              ▼                                         │ │
│   │  ┌──────────────────────────────────────────────────┐  │ │
│   │  │              Main Container                      │  │ │
│   │  │                                                  │  │ │
│   │  │  /workspace (rslave from sidecar's /shared)      │  │ │
│   │  │      ├── src/          ← visible, r/w to host    │  │ │
│   │  │      ├── package.json  ← visible, r/w to host    │  │ │
│   │  │      │                 ← .env does not exist     │  │ │
│   │  │      └── secrets/      ← empty dir (orphan)      │  │ │
│   │  │                                                  │  │ │
│   │  └──────────────────────────────────────────────────┘  │ │
│   │                                                        │ │
│   └────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

### Why This Is Ideal

- **No network protocol**: Direct filesystem access via mount propagation
- **Zero latency**: FUSE passthrough is near-native for data I/O
- **Single storage**: No duplication
- **Simple**: Just FUSE + standard Docker volume sharing

### Performance Optimization

#### FUSE Caching

Enable aggressive caching to minimize userspace round-trips:

```go
opts := &fuse.MountOptions{
    EntryTimeout:    3600 * time.Second,  // cache lookup results
    AttrTimeout:     3600 * time.Second,  // cache file attributes
    NegativeTimeout: 3600 * time.Second,  // cache "not found" results
}
```

With caching, repeated operations (e.g., `git status` scanning same files) hit kernel cache instead of FUSE daemon.

| Scenario                  | Without Cache   | With Cache     |
| ------------------------- | --------------- | -------------- |
| First `ls`                | FUSE call       | FUSE call      |
| Second `ls`               | FUSE call       | Kernel cache   |
| `git status` (1000 files) | 1000 FUSE calls | ~10 FUSE calls |

#### Linux FUSE Passthrough (Kernel 6.9+)

On Linux kernel 6.9+, FUSE supports **passthrough mode** for file I/O:

```go
// Enable passthrough for read/write operations
opts := &fuse.MountOptions{
    EnablePassthrough: true,  // kernel 6.9+
}
```

With passthrough:

- `read()`/`write()` bypass FUSE daemon entirely
- Kernel directly accesses underlying file
- Near-native I/O performance (~95-99%)
- Only metadata operations (lookup, readdir, stat) go through FUSE

| Operation    | Without Passthrough    | With Passthrough |
| ------------ | ---------------------- | ---------------- |
| read/write   | FUSE → daemon → kernel | Kernel direct    |
| stat/readdir | FUSE → daemon          | FUSE → daemon    |
| Overall I/O  | ~70-80% native         | ~95-99% native   |

Reference: [FUSE Passthrough merged in kernel 6.9](https://www.phoronix.com/news/FUSE-Passthrough-V6)

### Advantages Over Mutagen

| Aspect         | FUSE Sidecar     | Mutagen          |
| -------------- | ---------------- | ---------------- |
| Sync latency   | 0ms (real-time)  | 50-200ms         |
| Storage        | 1x               | 2x (double)      |
| Daemon on host | No               | Yes              |
| File watching  | Native (inotify) | Polling-based    |
| Network layer  | None             | TCP (localhost)  |
| Complexity     | Higher (sidecar) | Lower (CLI tool) |

### FUSE Filter Implementation (Go)

```go
type FilteredRoot struct {
    fs.LoopbackRoot
    excludes  map[string]bool  // top-level excludes
    orphanDir string           // orphan file storage
}

// Lookup: return ENOENT for excluded files
func (r *FilteredRoot) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
    if r.excludes[name] {
        if orphanExists(r.orphanDir, name) {
            return orphanLookup(r.orphanDir, name, out)
        }
        return nil, syscall.ENOENT
    }
    return r.LoopbackRoot.Lookup(ctx, name, out)
}

// Create: redirect excluded paths to orphan storage
func (r *FilteredRoot) Create(ctx context.Context, name string, ...) {
    if isExcludedPath(name, r.excludes) {
        return createInOrphanDir(r.orphanDir, name, ...)
    }
    return r.LoopbackRoot.Create(ctx, name, ...)
}
```

### Orphan File Storage

Files created at excluded paths persist in a Docker named volume:

```
Docker Volume: alca_<project-hash>_orphans
Mount: sidecar:/orphans

/orphans/
├── secrets/           ← created via mkdir /workspace/secrets
│   └── new-key.txt    ← created inside container
└── .env.local         ← created inside container
```

### Why This Is Blocked

**Two blocking issues on macOS**:

#### 1. Mount Propagation Doesn't Work

macOS container runtimes run in a VM, and the VM's mount namespace uses `private` propagation mode.

- All macOS container runtimes use VMs (Apple Virtualization.framework)
- Host → VM file sharing (VirtioFS, NFS, etc.) doesn't support `shared` propagation
- FUSE mounts created inside sidecar container cannot propagate to other containers
- This is a fundamental VM limitation, not specific to any runtime
- Tested on OrbStack: `bind mount + rshared`, `Docker volume + rshared`, `volumes_from` — all failed

References:

- [docker/for-mac#3431](https://github.com/docker/for-mac/issues/3431)
- [moby/moby#39093](https://github.com/moby/moby/issues/39093)

#### 2. VM-Based Bind Mount Performance

Even if propagation worked, macOS VM file sharing adds significant overhead. For Docker Desktop (free) and Lima, bind mounts run at ~35% of native — adding FUSE on top would compound the problem, making `node_modules`-heavy workloads impractical.

OrbStack (~85% native) is the exception, where FUSE overhead would be acceptable.

### If Mount Propagation Were Fixed

| Platform                             | Bind Mount Performance | FUSE Sidecar Viable?  |
| ------------------------------------ | ---------------------- | --------------------- |
| Linux native                         | 100%                   | ✅ Yes (ideal)        |
| macOS + OrbStack                     | ~85%                   | ✅ Yes (acceptable)   |
| macOS + Docker Desktop (paid + sync) | ~90%                   | ✅ Yes (with Mutagen) |
| macOS + Docker Desktop (free)        | ~35%                   | ❌ Too slow           |
| macOS + Lima                         | ~37%                   | ❌ Too slow           |

Note: Docker Desktop free tier and Lima have similar poor bind mount performance ([benchmark](https://www.paolomainardi.com/posts/docker-performance-macos-2025/)).

**Key insight**: If mount propagation were fixed, FUSE Sidecar would immediately become viable on:

- **Linux**: Native performance, already works
- **macOS + OrbStack**: 75-95% performance is acceptable for most workloads

### Future: When This Becomes Viable

| Condition                        | Status                   | FUSE Sidecar Support     |
| -------------------------------- | ------------------------ | ------------------------ |
| Linux deployment                 | **Already works**        | ✅ Full support          |
| OrbStack fixes propagation       | Unlikely (VM limitation) | ✅ Would work (perf OK)  |
| Docker Desktop fixes propagation | Unlikely (VM limitation) | ⚠️ Marginal (perf issue) |
| macOS 26 Apple Containers        | Uncertain (still VM-based) | ❓ Needs investigation |

Note: Apple Containers (macOS 26+) still uses VMs (one VM per container). Mount propagation behavior is unknown and needs investigation.

## References

### FUSE & Mount Propagation

- [docker/for-mac#3431](https://github.com/docker/for-mac/issues/3431) — Bind mounts with shared propagation don't work
- [moby/moby#39093](https://github.com/moby/moby/issues/39093) — Unable to propagate mounts of containers
- [Docker bind mount docs](https://docs.docker.com/engine/storage/bind-mounts/) — Mount propagation only on Linux
- [Linux kernel shared subtree](https://www.kernel.org/doc/Documentation/filesystems/sharedsubtree.txt)

### NFS & FUSE Export Limitations

- [Linux kernel NFS re-export docs](https://docs.kernel.org/filesystems/nfs/reexport.html) — fsid requirements
- [NFS-Ganesha FUSE wiki](https://github.com/phdeniel/nfs-ganesha/wiki/FUSE) — Userspace NFS with FUSE (deprecated)

### Network Filesystem Performance

- [NAS Performance: NFS vs SMB vs SSHFS](https://blog.ja-ke.tech/2019/08/27/nas-performance-sshfs-nfs-smb.html)
- [rust-9p performance issue](https://github.com/pfpacket/rust-9p/issues/19) — 9P slower than SSHFS

### Mutagen

- [Mutagen official site](https://mutagen.io/)
- [Mutagen daemon docs](https://mutagen.io/documentation/introduction/daemon/)
- [Mutagen Go package](https://pkg.go.dev/github.com/mutagen-io/mutagen)
- [Docker acquires Mutagen](https://www.docker.com/blog/announcing-synchronized-file-shares/) — Synchronized File Shares

### Docker Desktop Performance

- [Docker on macOS is slow (CNCF)](https://www.cncf.io/blog/2023/02/02/docker-on-macos-is-slow-and-how-to-fix-it/)
- [Docker on macOS is still slow? (2025)](https://www.paolomainardi.com/posts/docker-performance-macos-2025/)
- [DDEV macOS Docker provider performance (2023)](https://ddev.com/blog/docker-performance-2023/)
- [Docker Desktop VirtioFS 4x faster](https://www.jeffgeerling.com/blog/2022/new-docker-mac-virtiofs-file-sync-4x-faster)

### OrbStack

- [OrbStack fast filesystem blog](https://orbstack.dev/blog/fast-filesystem)
- [OrbStack vs Docker Desktop docs](https://docs.orbstack.dev/compare/docker-desktop)
- [OrbStack + fuse-t incompatibility](https://github.com/orbstack/orbstack/issues/1696)

### FUSE Performance Research

- [To FUSE or Not to FUSE (USENIX FAST '17)](https://www.usenix.org/system/files/conference/fast17/fast17-vangoor.pdf)
- [FUSE Performance and Resource Utilization (ACM TOS)](https://dl.acm.org/doi/fullHtml/10.1145/3310148)
- [FUSE Passthrough for kernel 6.9+](https://www.phoronix.com/news/FUSE-Passthrough-V6)
