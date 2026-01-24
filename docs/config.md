---
title: Configuration
weight: 20
---

# Configuration Reference

This document describes the `.alca.toml` configuration file format for Alcatraz.

## Table of Contents

- [Overview](#overview)
- [Field Reference](#field-reference)
- [Runtime-Specific Notes](#runtime-specific-notes)
- [Full Example](#full-example)

## Overview

Alcatraz uses TOML format for configuration. The configuration file should be named `.alca.toml` and placed in your project root.

## Field Reference

| Field              | Type   | Required | Default                                  | Description                                    |
| ------------------ | ------ | -------- | ---------------------------------------- | ---------------------------------------------- |
| `image`            | string | Yes      | `"nixos/nix"`                            | Container image to use                         |
| `workdir`          | string | No       | `"/workspace"`                           | Working directory inside container             |
| `runtime`          | string | No       | `"auto"`                                 | Runtime selection mode                         |
| `commands.up`      | string | No       | -                                        | Setup command (run once on container creation) |
| `commands.enter`   | string | No       | `"[ -f flake.nix ] && exec nix develop"` | Entry command (run on each shell entry)        |
| `mounts`           | array  | No       | `[]`                                     | Additional mount points                        |
| `resources.memory` | string | No       | -                                        | Memory limit (e.g., "4g", "512m")              |
| `resources.cpus`   | int    | No       | -                                        | CPU limit (e.g., 2, 4)                         |
| `envs`             | table  | No       | See below                                | Environment variables for the container        |

### image

The container image to use for the isolated environment.

```toml
image = "nixos/nix"
```

- **Type**: string
- **Required**: Yes
- **Default**: `"nixos/nix"`
- **Examples**: `"ubuntu:22.04"`, `"alpine:latest"`, `"nixos/nix"`

### workdir

The working directory inside the container where your project will be mounted.

```toml
workdir = "/workspace"
```

- **Type**: string
- **Required**: No
- **Default**: `"/workspace"`
- **Notes**: Must be an absolute path

### runtime

Selects which container runtime to use.

```toml
runtime = "auto"
```

- **Type**: string
- **Required**: No
- **Default**: `"auto"`
- **Valid values**:
  - `"auto"` - Auto-detect best available runtime
    - macOS: Apple Containerization > Docker
    - Linux: Podman > Docker
  - `"docker"` - Force Docker regardless of other available runtimes

### commands.up

Setup command executed once when the container is created. Use this for one-time initialization tasks.

```toml
[commands]
up = "nix-channel --update && nix-env -iA nixpkgs.git"
```

- **Type**: string
- **Required**: No
- **Default**: None
- **Examples**:
  - `"apt-get update && apt-get install -y vim"`
  - `"nix-channel --update"`

### commands.enter

Entry command executed each time you enter the container shell. Use this for environment setup.

```toml
[commands]
enter = "[ -f flake.nix ] && exec nix develop"
```

- **Type**: string
- **Required**: No
- **Default**: `"[ -f flake.nix ] && exec nix develop"`
- **Notes**: If the command uses `exec`, it replaces the shell process

### mounts

Additional mount points beyond the default project mount.

```toml
mounts = [
  "/path/on/host:/path/in/container",
  "~/.ssh:/root/.ssh:ro"
]
```

- **Type**: array of strings
- **Required**: No
- **Default**: `[]`
- **Format**: `"host_path:container_path"` or `"host_path:container_path:options"`
- **Options**: `ro` (read-only), `rw` (read-write, default)

### resources.memory

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

### resources.cpus

CPU limit for the container.

```toml
[resources]
cpus = 4
```

- **Type**: integer
- **Required**: No
- **Default**: None (no limit, uses runtime default)
- **Examples**: `1`, `2`, `4`, `8`

### envs

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

#### Variable Expansion

Use `${VAR}` to read from host environment. Only simple syntax is supported:

```toml
[envs]
# Valid
TERM = "${TERM}"
MY_VAR = "${MY_CUSTOM_VAR}"

# Invalid - will error
GREETING = "hello${NAME}"    # Complex interpolation not supported
```

#### Default Environment Variables

The following are passed by default with `override_on_enter = true`:

| Variable | Description |
|----------|-------------|
| `TERM` | Terminal type |
| `COLORTERM` | Color terminal capability |
| `LANG` | Default locale |
| `LC_ALL` | Override all locale settings |
| `LC_COLLATE` | Collation order |
| `LC_CTYPE` | Character classification |
| `LC_MESSAGES` | Message language |
| `LC_MONETARY` | Monetary formatting |
| `LC_NUMERIC` | Numeric formatting |
| `LC_TIME` | Date/time formatting |

User-defined values override these defaults.

## Runtime-Specific Notes

### Docker / Podman

Resource limits are passed as container-level flags:

- Memory: `-m` or `--memory` flag
- CPU: `--cpus` flag

**Important**: On macOS, Docker Desktop runs containers in a VM with fixed resource allocation. Container limits are constrained by the VM's allocated resources. Configure VM resources via Docker Desktop > Settings > Resources.

### Apple Containerization

Each container runs in its own lightweight VM with dedicated resources:

- Memory: `-m` flag (1 MiB granularity)
- CPU: `-c` or `--cpus` flag

**Characteristics**:

- Per-container micro-VM model
- Zero memory overhead when no containers running
- Memory automatically released when container stops
- Default per container: 1 GiB memory, 4 CPUs

## Full Example

```toml
# Container image
image = "nixos/nix"

# Working directory inside container
workdir = "/workspace"

# Runtime selection: auto or docker
runtime = "auto"

# Lifecycle commands
[commands]
up = "nix-channel --update"
enter = "[ -f flake.nix ] && exec nix develop"

# Additional mounts
mounts = [
  "~/.gitconfig:/root/.gitconfig:ro",
  "~/.ssh:/root/.ssh:ro"
]

# Resource limits
[resources]
memory = "16g"
cpus = 8

# Environment variables
[envs]
NIXPKGS_ALLOW_UNFREE = "1"
EDITOR = { value = "${EDITOR}", override_on_enter = true }
```
