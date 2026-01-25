---
title: Runtimes
weight: 30
---

# Runtimes

Alcatraz supports multiple container runtimes for isolating code execution. This document covers the supported runtimes, their characteristics, and how to configure them.

## Runtime Overview

### Auto-Detection

By default (`runtime = "auto"`), Alcatraz automatically selects the best available runtime:

| Platform | Priority Order  |
| -------- | --------------- |
| Linux    | Podman > Docker |
| Other    | Docker          |

> **macOS Users**: We recommend [OrbStack](https://orbstack.dev/) as it provides automatic memory management (shrinking unused memory), which colima and lima do not support.

### Manual Selection

Override auto-detection by setting `runtime` in `.alca.toml`:

```toml
runtime = "docker"  # Force Docker
runtime = "auto"    # Auto-detect (default)
```

## Docker

Docker is the most widely supported runtime, available on macOS, Linux, and Windows.

### Availability

Alcatraz checks for Docker availability by running:

```bash
docker version --format '{{.Server.Version}}'
```

### macOS Behavior

On macOS, Docker runs containers inside a Linux VM (Docker Desktop):

- **VM Model**: Single shared VM for all containers
- **Memory**: Must be pre-allocated to the VM via Docker Desktop settings
- **Memory Release**: The VM does **not** release unused memory back to macOS (known limitation)
- **Resource Limits**: Container limits (`-m`, `--cpus`) constrained by VM allocation

To configure VM resources:

- Docker Desktop > Settings > Resources > Advanced
- Or edit `~/.docker/daemon.json`

### Linux Behavior

On Linux, Docker runs containers natively:

- **VM Model**: None (native kernel)
- **Memory**: Allocated on-demand from host
- **Memory Release**: Automatic, managed by cgroups
- **Resource Limits**: Direct cgroups control, no VM constraint

### Resource Limits

```bash
# Via alca.toml
[resources]
memory = "4g"
cpus = 4

# Translates to:
docker run -m 4g --cpus 4 ...
```

### Troubleshooting

| Issue                             | Solution                                              |
| --------------------------------- | ----------------------------------------------------- |
| "Docker not found"                | Install Docker Desktop or Docker Engine               |
| "Cannot connect to Docker daemon" | Start Docker Desktop or `sudo systemctl start docker` |
| Container slow / memory issues    | Increase Docker Desktop VM memory allocation          |

## Podman

Podman is preferred on Linux for its rootless container support.

### Availability

Alcatraz checks for Podman availability by running:

```bash
podman version --format '{{.Version}}'
```

### macOS Behavior

On macOS, Podman requires a VM (`podman machine`):

```bash
# Initialize VM
podman machine init --cpus 4 --memory 8192 --disk-size 100

# Modify existing VM (QEMU only, not Apple Hypervisor)
podman machine stop
podman machine set --cpus 8 --memory 16384
podman machine start
```

- **VM Model**: Single shared VM (similar to Docker Desktop)
- **Memory Release**: Manual (same limitation as Docker)

### Linux Behavior

On Linux, Podman runs natively with rootless support:

- **VM Model**: None (native kernel)
- **Memory**: Direct cgroups control
- **Rootless**: Can run containers without root privileges

### Resource Limits

```bash
# Via alca.toml (same as Docker)
[resources]
memory = "4g"
cpus = 4

# Translates to:
podman run -m 4g --cpus 4 ...
```

### Troubleshooting

| Issue                               | Solution                                                                  |
| ----------------------------------- | ------------------------------------------------------------------------- |
| "Podman not found"                  | Install via package manager (`brew install podman`, `apt install podman`) |
| macOS: "podman machine not running" | Run `podman machine start`                                                |
| macOS: Cannot set CPU/memory        | QEMU backend required, Apple Hypervisor has limitations                   |

## Comparison Table

| Feature                  | Docker (macOS)        | Docker (Linux)       | Podman (macOS)       | Podman (Linux)       |
| ------------------------ | --------------------- | -------------------- | -------------------- | -------------------- |
| **VM Model**             | Single shared VM      | None (native)        | Single shared VM     | None (native)        |
| **Idle Memory**          | 3-4 GB                | ~0                   | 3-4 GB               | ~0                   |
| **Memory Release**       | Manual                | Automatic            | Manual               | Automatic            |
| **Resource Isolation**   | Shared VM             | Native cgroups       | Shared VM            | Native cgroups       |
| **Host Config Needed**   | Yes (VM settings)     | No                   | Yes (VM settings)    | No                   |
| **Per-container Limits** | Yes (`-m`, `--cpus`)  | Yes (`-m`, `--cpus`) | Yes (`-m`, `--cpus`) | Yes (`-m`, `--cpus`) |
| **Live Update**          | `docker update`       | `docker update`      | Limited              | Limited              |
| **Rootless**             | No                    | Optional             | No                   | Yes (default)        |
| **Platform**             | macOS, Linux, Windows | Linux                | macOS, Linux         | Linux                |

## Verifying Runtime Selection

Check which runtime Alcatraz is using:

```bash
# Show detected runtime
alca status
```

The status output indicates the active runtime.

## References

- [Docker Resource Constraints](https://docs.docker.com/engine/containers/resource_constraints/)
- [Docker Desktop Settings](https://docs.docker.com/desktop/settings-and-maintenance/settings/)
- [Podman Machine Init](https://docs.podman.io/en/latest/markdown/podman-machine-init.1.html)
- [OrbStack](https://orbstack.dev/) — Recommended Docker runtime for macOS
