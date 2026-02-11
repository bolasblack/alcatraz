---
title: Runtimes
weight: 3
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

On macOS, Docker runs containers inside a Linux VM (Docker Desktop or OrbStack):

- **VM Model**: Single shared VM for all containers
- **Memory**: Must be pre-allocated to the VM via Docker Desktop settings (OrbStack manages memory automatically)
- **Memory Release**: Docker Desktop does **not** release unused memory back to macOS; OrbStack shrinks unused memory automatically
- **Resource Limits**: Container limits (`-m`, `--cpus`) constrained by VM allocation

To configure VM resources (Docker Desktop only; not needed for OrbStack):

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

| Issue                             | Solution                                                                    |
| --------------------------------- | --------------------------------------------------------------------------- |
| "Docker not found"                | Install Docker Desktop, [OrbStack](https://orbstack.dev/), or Docker Engine |
| "Cannot connect to Docker daemon" | Start Docker Desktop / OrbStack or `sudo systemctl start docker`            |
| Container slow / memory issues    | Increase Docker Desktop VM memory (OrbStack manages this automatically)     |

## OrbStack

[OrbStack](https://orbstack.dev/) is the recommended runtime on macOS. It provides its own `docker` CLI, so Alcatraz detects and uses it the same way as Docker — no additional configuration needed.

OrbStack offers automatic memory management (shrinks unused memory), unlike Docker Desktop which requires manual pre-allocation.

## Podman

Podman is preferred on Linux for its rootless container support.

### Availability

Alcatraz checks for Podman availability by running:

```bash
podman version --format '{{.Version}}'
```

### macOS

Podman is not supported on macOS. Use Docker (we recommend [OrbStack](https://orbstack.dev/)) instead.

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

| Issue              | Solution                                           |
| ------------------ | -------------------------------------------------- |
| "Podman not found" | Install via package manager (`apt install podman`) |

## Comparison Table

| Feature                  | Docker (macOS)        | Docker (Linux)       | Podman (Linux)       |
| ------------------------ | --------------------- | -------------------- | -------------------- |
| **VM Model**             | Single shared VM      | None (native)        | None (native)        |
| **Idle Memory**          | 3-4 GB                | ~0                   | ~0                   |
| **Memory Release**       | Manual                | Automatic            | Automatic            |
| **Resource Isolation**   | Shared VM             | Native cgroups       | Native cgroups       |
| **Host Config Needed**   | Yes (VM settings)     | No                   | No                   |
| **Per-container Limits** | Yes (`-m`, `--cpus`)  | Yes (`-m`, `--cpus`) | Yes (`-m`, `--cpus`) |
| **Live Update**          | `docker update`       | `docker update`      | Limited              |
| **Rootless**             | No                    | Optional             | Yes (default)        |
| **Platform**             | macOS, Linux, Windows | Linux                | Linux                |

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
- [OrbStack](https://orbstack.dev/) — Recommended Docker runtime for macOS
