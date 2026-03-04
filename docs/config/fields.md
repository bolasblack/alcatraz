---
title: Field Reference
weight: 2.1
---

# Field Reference

## image

The container image to use for the isolated environment.

```toml
image = "nixos/nix"
```

- **Type**: string
- **Required**: Yes
- **Default**: None (must be specified)
- **Examples**: `"ubuntu:22.04"`, `"alpine:latest"`, `"nixos/nix"`

## workdir

The working directory inside the container where your project will be mounted.

```toml
workdir = "/workspace"
```

- **Type**: string
- **Required**: No
- **Default**: `"/workspace"`
- **Notes**: Must be an absolute path

## workdir_exclude

Patterns to exclude from the workdir mount. When specified, Alcatraz uses [Mutagen](https://mutagen.io/) for file synchronization instead of direct bind mounts.

```toml
workdir = "/workspace"
workdir_exclude = ["node_modules", ".git", "dist"]
```

- **Type**: array of strings
- **Required**: No
- **Default**: `[]`
- **Notes**: Patterns follow gitignore-like syntax (see [Exclude Patterns](#exclude-patterns))

This is a convenience shorthand for configuring excludes on the workdir mount. The following configurations are equivalent:

```toml
# Using workdir_exclude (recommended)
workdir = "/workspace"
workdir_exclude = ["node_modules", ".git"]
```

```toml
# Using extended mount format
workdir = "/workspace"
[[mounts]]
source = "."
target = "/workspace"
exclude = ["node_modules", ".git"]
```

**Note**: You cannot add a mount targeting the same path as `workdir`. If you need to exclude subdirectories from syncing, use `workdir_exclude` instead of creating a separate mount.

> When using `workdir_exclude`, Alcatraz monitors for sync conflicts (simultaneous edits on both sides). See [Sync Conflicts](../sync-conflicts.md) for detection and resolution.

## runtime

Selects which container runtime to use.

```toml
runtime = "auto"
```

- **Type**: string
- **Required**: No
- **Default**: `"auto"`
- **Valid values**:
  - `"auto"` - Auto-detect best available runtime (Linux: Podman > Docker; macOS: Docker / OrbStack)
  - `"docker"` - Force Docker regardless of other available runtimes

## commands.up

Setup command executed once when the container is created. Use this for one-time initialization tasks.

```toml
[commands]
up = "nix-channel --update && nix-env -iA nixpkgs.git"
```

- **Type**: string or object
- **Required**: No
- **Default**: None
- **Examples**:
  - `"apt-get update && apt-get install -y vim"`
  - `"nix-channel --update"`

## commands.enter

Entry command executed each time you enter the container shell. Use this for environment setup.

```toml
[commands]
enter = "[ -f flake.nix ] && exec nix develop"
```

- **Type**: string or object
- **Required**: No
- **Default**: `"[ -f flake.nix ] && exec nix develop"`
- **Notes**: If the command uses `exec`, it replaces the shell process

### How enter interacts with `alca run`

When you run `alca run <command>`, the enter command is **space-concatenated** with your quoted arguments and wrapped in `sh -c`:

```
sh -c "<enter_command> '<arg1>' '<arg2>'"
```

This means the enter command must either:

1. **Accept trailing arguments naturally** — e.g., `nix develop --command` treats everything after `--command` as the command to run
2. **End with a statement separator** — a trailing newline (multi-line string) or semicolon, so user arguments execute as a separate statement

Without a separator, arguments become positional parameters to the last command instead of running independently:

```toml
# Wrong — 'ls' becomes $1 to the source command, never executes
[commands]
enter = ". ~/.bashrc"
# sh -c ". ~/.bashrc 'ls'"

# Correct — newline separates the statements
[commands]
enter = """
. ~/.bashrc
"""
# sh -c ". ~/.bashrc\n'ls'"  →  sources bashrc, then runs ls
```

## Command Formats

Commands support both a simple string format and a struct format with merge control.

### String Format

```toml
[commands]
up = "docker compose up -d"
```

### Struct Format

```toml
[commands.up]
command = "docker compose up -d"
append = false
```

| Field     | Type   | Required | Default | Description                                |
| --------- | ------ | -------- | ------- | ------------------------------------------ |
| `command` | string | Yes      | -       | The command string                         |
| `append`  | bool   | No       | `false` | Append to base command during config merge |

Both formats are equivalent when `append` is not needed.

### Command Append

The `append` flag controls how a command merges with its base during config layering (via `extends` or `includes`):

- `append = false` (default): overlay replaces the base command entirely
- `append = true`: result is `base_command + " " + overlay_command` (space-concatenated)

**Only the overlay's `append` flag is consulted.** The base's `append` value is ignored during merge.

```toml
# .alca.toml
includes = [".alca.local.toml"]
[commands.up]
command = "nix develop"

# .alca.local.toml (overlay via includes)
[commands.up]
command = "bash"
append = true

# Result: "nix develop bash"
```

See [AGD-034](https://github.com/bolasblack/alcatraz/blob/master/.agents/decisions/AGD-034_command-append.md) for design rationale.

## mounts

Additional mount points beyond the default project mount. Supports both simple string format and extended object format with exclude patterns.

### Simple String Format

```toml
mounts = [
  "/path/on/host:/path/in/container",
  "~/.ssh:/root/.ssh:ro"
]
```

- **Format**: `"host_path:container_path"` or `"host_path:container_path:ro"`
- **Options**: `ro` (read-only)

### Extended Object Format

Use the extended format when you need to exclude files from being visible inside the container. See [AGD-025](https://github.com/bolasblack/alcatraz/blob/master/.agents/decisions/AGD-025_mount-exclude-with-mutagen.md) for design rationale.

```toml
[[mounts]]
source = "/Users/me/project"
target = "/workspace"
readonly = false
exclude = [
  "**/.env.prod",
  "**/.env.local",
  "**/secrets/",
  "**/*.key",
]
```

| Field      | Type   | Required | Default | Description              |
| ---------- | ------ | -------- | ------- | ------------------------ |
| `source`   | string | Yes      | -       | Host path                |
| `target`   | string | Yes      | -       | Container path           |
| `readonly` | bool   | No       | `false` | Read-only mount          |
| `exclude`  | array  | No       | `[]`    | Glob patterns to exclude |

### Environment Variables

Mount source paths support `${VAR}` environment variable expansion:

```toml
mounts = ["${HOME}/data:/data"]

[[mounts]]
source = "${PROJECT_ROOT}/configs"
target = "/configs"
```

- Both `$VAR` and `${VAR}` syntax are supported
- **Source paths only** — target paths are container-internal and are not expanded
- Expansion happens before path resolution
- Undefined variables cause an error (e.g., `undefined environment variable: $PROJECT_ROOT`)

Environment variable expansion is also supported in [`extends` and `includes`](./extends-includes.md#environment-variables) paths and in [`envs`](#variable-expansion) values. In all cases, expansion happens early — before path resolution, glob matching, or config merging.

### Exclude Patterns

Exclude patterns follow gitignore-like syntax (Mutagen ignore format):

| Pattern       | Matches                           |
| ------------- | --------------------------------- |
| `**/`         | Any directory depth               |
| `*.ext`       | Files with extension              |
| `dir/`        | Directory (trailing slash)        |
| `**/.env`     | `.env` file at any depth          |
| `**/secrets/` | `secrets/` directory at any depth |

**Security: hide sensitive files from your agent:**

```toml
[[mounts]]
source = "."
target = "/workspace"
exclude = [
  "**/.env",       # Environment secrets
  "**/.env.*",     # Environment variants
  "**/secrets/",   # Secret directories
  "**/*.key",      # Private keys
  "**/*.pem",      # Certificates
]
```

Excluded files are invisible inside the container — even with full agent access within the sandbox.

**Recommended excludes for Node.js projects:**

```toml
[[mounts]]
source = "."
target = "/workspace"
exclude = [
  "node_modules/",     # Container runs its own npm install
  ".pnpm-store/",      # pnpm cache
  "dist/",             # Build output
  ".next/",            # Next.js cache
]
```

**Note**: When excludes are specified, Alcatraz uses [Mutagen](https://mutagen.io/) for file synchronization instead of direct bind mounts. This provides file filtering but introduces 50-200ms sync latency.

- **Type**: array (strings or objects)
- **Required**: No
- **Default**: `[]`

## resources.memory

Memory limit for the container.

```toml
[resources]
memory = "4g"
```

- **Type**: string
- **Required**: No
- **Default**: None (no limit, uses runtime default)
- **Format**: Number followed by suffix
- **Suffixes**: `b` (bytes), `k` (KB), `m` (MB), `g` (GB)
- **Examples**: `"512m"`, `"2g"`, `"16g"`

## resources.cpus

CPU limit for the container.

```toml
[resources]
cpus = 4
```

- **Type**: integer
- **Required**: No
- **Default**: None (no limit, uses runtime default)
- **Examples**: `1`, `2`, `4`, `8`

## envs

Environment variables for the container. See [AGD-017](https://github.com/bolasblack/alcatraz/blob/master/.agents/decisions/AGD-017_env-config-design.md) for design rationale.

```toml
[envs]
# Static value - set at container creation
NIXPKGS_ALLOW_UNFREE = "1"

# Read from host environment at container creation
MY_TOKEN = "${MY_TOKEN}"

# Read from host and refresh on each `alca run`
EDITOR = { value = "${EDITOR}", override_on_enter = true }
```

- **Type**: table (key-value pairs)
- **Required**: No
- **Value formats**:
  - `"string"` - Static value or `${VAR}` reference, set at container creation
  - `{ value = "...", override_on_enter = true }` - Also refresh on each `alca run`

### Variable Expansion

Use `${VAR}` to read from host environment. Only simple syntax is supported:

```toml
[envs]
# Valid
TERM = "${TERM}"
MY_VAR = "${MY_CUSTOM_VAR}"

# Invalid - will error
GREETING = "hello${NAME}"    # Complex interpolation not supported
```

### Default Environment Variables

The following are passed by default with `override_on_enter = true`:

| Variable      | Description                  |
| ------------- | ---------------------------- |
| `TERM`        | Terminal type                |
| `COLORTERM`   | Color terminal capability    |
| `LANG`        | Default locale               |
| `LC_ALL`      | Override all locale settings |
| `LC_COLLATE`  | Collation order              |
| `LC_CTYPE`    | Character classification     |
| `LC_MESSAGES` | Message language             |
| `LC_MONETARY` | Monetary formatting          |
| `LC_NUMERIC`  | Numeric formatting           |
| `LC_TIME`     | Date/time formatting         |

User-defined values override these defaults.

## caps

Linux capabilities configuration for container security. See [AGD-026](https://github.com/bolasblack/alcatraz/blob/master/.agents/decisions/AGD-026_container-capabilities-config.md) for design rationale.

**Security rationale**: Docker's default capabilities include dangerous ones like `NET_RAW` (network sniffing) and `MKNOD` (device creation) that AI code agents don't need. Alcatraz drops all capabilities by default and only adds the minimal set needed for development workflows, keeping your agent sandboxed with least-privilege access.

### Default Behavior (No `caps` field)

```toml
# No caps field - secure defaults applied
image = "nixos/nix"
```

**Result**: `--cap-drop ALL --cap-add CHOWN --cap-add DAC_OVERRIDE --cap-add FOWNER --cap-add KILL --cap-add SETUID --cap-add SETGID`

Default capabilities:

- `CHOWN`: Package managers (npm, pip, cargo) need to modify file ownership
- `DAC_OVERRIDE`: Bypass file read/write/execute permission checks for file operations in containers
- `FOWNER`: Modify file permissions and attributes during builds
- `KILL`: Terminate child processes (test runners, dev servers)
- `SETUID`: Required by package managers (apt, nix) for sandbox/daemon builds
- `SETGID`: Required by package managers (apt, nix) for sandbox/daemon builds

### Mode 1: Additive (Array)

Add capabilities beyond the defaults:

```toml
caps = ["NET_BIND_SERVICE"]
```

**Result**: `--cap-drop ALL --cap-add CHOWN --cap-add DAC_OVERRIDE --cap-add FOWNER --cap-add KILL --cap-add SETUID --cap-add SETGID --cap-add NET_BIND_SERVICE`

Use this when you need additional capabilities but want to keep the secure default base.

### Mode 2: Full Control (Object)

Take complete control over capabilities:

```toml
[caps]
drop = ["NET_RAW", "MKNOD", "AUDIT_WRITE"]
add = ["CHOWN", "FOWNER", "KILL", "SETUID", "SETGID"]
```

**Result**: `--cap-drop NET_RAW --cap-drop MKNOD --cap-drop AUDIT_WRITE --cap-add CHOWN --cap-add FOWNER --cap-add KILL --cap-add SETUID --cap-add SETGID`

Use this when you want explicit control. No implicit defaults are applied in this mode.

### Example: Keep Docker Defaults, Drop Dangerous Ones

```toml
[caps]
drop = ["NET_RAW", "MKNOD", "SYS_CHROOT"]
# No add field - keeps Docker defaults minus dropped ones
```

**Result**: `--cap-drop NET_RAW --cap-drop MKNOD --cap-drop SYS_CHROOT`

### Troubleshooting

| Error                                     | Solution                                                           |
| ----------------------------------------- | ------------------------------------------------------------------ |
| `Permission denied` when writing files    | Add `DAC_OVERRIDE` capability                                      |
| `Operation not permitted` with setuid     | Ensure `SETUID` and `SETGID` are in add list (included by default) |
| Package manager fails to change ownership | Ensure `CHOWN` and `FOWNER` are in add list                        |

## extends

Extend other configuration files. The declaring file overrides extended files.

```toml
extends = [".alca.base.toml"]
```

- **Type**: array of strings
- **Required**: No
- **Default**: `[]`
- **Notes**: Paths are resolved relative to the declaring file's directory. Supports glob patterns (`*.toml`). The declaring file's values win over extended files.

## includes

Include other configuration files. Included files override the declaring file.

```toml
includes = [".alca.local.toml"]
```

- **Type**: array of strings
- **Required**: No
- **Default**: `[]`
- **Notes**: Paths are resolved relative to the declaring file's directory. Supports glob patterns (`*.toml`). Included files' values win over the declaring file.

See [Extends & Includes](./extends-includes.md) for full documentation including three-layer merge, processing order, and migration guide.

## network.ports

Map container ports to the host machine. Each port entry creates a Docker `-p` flag at container creation time. Port changes trigger a container rebuild (detected via drift detection).

Supports both a simple string format (Docker-style) and an extended object format. Both forms can coexist in the same array.

### String Format

```toml
[network]
ports = [
  "8080",                       # container 8080 → host 8080
  "3001:3000",                  # container 3000 → host 3001
  "127.0.0.1:5432:5432",       # container 5432 → host 127.0.0.1:5432
  "53:53/udp",                  # container 53/udp → host 53/udp
  "0.0.0.0:8080:80/tcp",       # full form
]
```

- **Format**: `[hostIp:]hostPort:containerPort[/protocol]` or just `containerPort`
- **Single number**: treated as the container port (host port defaults to the same)
- **Protocol**: append `/tcp` or `/udp` (default: `tcp`)

### Object Format

```toml
[network]
ports = [
  { port = 8080 },                                           # container 8080 → host 8080
  { port = 3000, hostPort = 3001 },                          # container 3000 → host 3001
  { port = 5432, hostIp = "127.0.0.1" },                     # container 5432 → host 127.0.0.1:5432
  { port = 53, protocol = "udp" },                            # container 53/udp → host 53/udp
  { port = 80, hostIp = "0.0.0.0", hostPort = 8080, protocol = "tcp" },  # full form
]
```

| Field      | Type   | Required | Default          | Description                         |
| ---------- | ------ | -------- | ---------------- | ----------------------------------- |
| `port`     | int    | Yes      | -                | Container port (1-65535)            |
| `hostPort` | int    | No       | Same as `port`   | Host port (1-65535)                 |
| `hostIp`   | string | No       | `""` (all interfaces) | Host IP to bind to             |
| `protocol` | string | No       | `"tcp"`          | Protocol: `"tcp"` or `"udp"`       |

### Mixed Forms

Both formats can be combined in the same array:

```toml
[network]
ports = [
  "8080",
  "3001:3000",
  { port = 9090 },
  "53:53/udp",
]
```

- **Type**: array (strings or objects)
- **Required**: No
- **Default**: `[]` (no port mappings)
- **Notes**: Changing ports triggers a container rebuild since Docker `-p` flags are set at creation time

## network.lan-access

Control container access to your local network (LAN).

```toml
[network]
lan-access = ["*://${alca:HOST_IP}:8080"]
```

- **Type**: array of strings
- **Required**: No
- **Default**: `[]` (no LAN access — containers can reach the internet but not local machines)
- **Valid values**:
  - `"*"` — allow all LAN access
  - Specific host rules with optional token expansion (see below)

### Token Expansion

The `lan-access` field supports special `${alca:<NAME>}` tokens that are resolved at runtime:

- `${alca:HOST_IP}` — Resolves to the container host's gateway IP address. Currently the only supported token.

**Example**: Access a local dev server on the host:

```toml
[network]
lan-access = ["*://${alca:HOST_IP}:8080"]
```

The `${alca:HOST_IP}` token expands to the actual host gateway IP at runtime (Docker: `172.17.0.1`, Podman: bridge gateway, etc.), making your config portable across different environments without hardcoding IP addresses.

See [Network Configuration](./network.md) for platform behavior, the network helper, and why nftables inside the VM is necessary on macOS.

## Runtime-Specific Notes

### Docker / Podman

Resource limits are passed as container-level flags:

- Memory: `-m` or `--memory` flag
- CPU: `--cpus` flag

**Important**: On macOS, Docker Desktop runs containers in a VM with fixed resource allocation. Container limits are constrained by the VM's allocated resources. Configure VM resources via Docker Desktop > Settings > Resources. (OrbStack manages resources automatically — no manual configuration needed.)

> **macOS Users**: We recommend [OrbStack](https://orbstack.dev/) as it provides automatic memory management (shrinking unused memory), which colima and lima do not support.
