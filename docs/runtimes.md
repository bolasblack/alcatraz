---
title: Runtimes
weight: 30
---

# Runtimes

Alcatraz supports multiple container runtimes for isolating code execution. This document covers the supported runtimes, their characteristics, and how to configure them.

## Runtime Overview

### Auto-Detection

By default (`runtime = "auto"`), Alcatraz automatically selects the best available runtime:

| Platform | Priority Order                  |
| -------- | ------------------------------- |
| macOS    | Apple Containerization > Docker |
| Linux    | Podman > Docker                 |
| Other    | Docker                          |

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

## Apple Containerization

Apple Containerization is a native macOS solution using per-container lightweight VMs. Available on macOS 26+ (Tahoe) with Apple Silicon.

### Key Characteristics

- **Per-container micro-VM model**: Each container runs in its own isolated VM
- **Zero overhead when idle**: No persistent VM consuming resources
- **Automatic memory release**: Memory freed when container stops
- **macOS 26+ only**: Requires macOS Tahoe or later
- **Apple Silicon only**: Intel Macs not supported

### Prerequisites

| Requirement       | Check Command             | Notes                                                    |
| ----------------- | ------------------------- | -------------------------------------------------------- |
| macOS 26+ (Tahoe) | `sw_vers -productVersion` | macOS 15 works with [limitations](#macos-15-limitations) |
| Apple Silicon     | `uname -m` = `arm64`      | Intel not supported                                      |
| container CLI     | `which container`         | Install from GitHub releases                             |

### Setup Process

1. **Install CLI**

   Download from [GitHub Releases](https://github.com/apple/container/releases):
   - Download `container-*-installer-signed.pkg`
   - Run installer (requires admin password)
   - Installs to `/usr/local/bin/container`

   ```bash
   # Verify installation
   which container
   container --version
   ```

2. **Start System**

   ```bash
   container system start
   ```

   This starts `container-apiserver` via launchd.

   ```bash
   # Verify system is running
   container system status
   ```

3. **Configure Kernel**

   On first start, you'll be prompted to install a Linux kernel for the VMs:

   ```
   No default kernel configured.
   Install the recommended default kernel? [Y/n]:
   ```

   Type `y` to install the Kata Containers kernel.

   **Automation options:**

   ```bash
   # Auto-install (non-interactive)
   container system start --enable-kernel-install

   # Skip kernel install
   container system start --disable-kernel-install

   # Manual install later
   container system kernel set --recommended
   ```

4. **Verify Ready**

   ```bash
   container image list
   # Success: exits 0 (may show empty list)
   ```

### Setup States

Alcatraz detects these setup states for Apple Containerization:

| State                 | Meaning                       | Resolution                                      |
| --------------------- | ----------------------------- | ----------------------------------------------- |
| Ready                 | Fully configured              | ✓                                               |
| Not Installed         | CLI not found                 | Install from GitHub releases                    |
| System Not Running    | CLI installed, system stopped | Run `container system start`                    |
| Kernel Not Configured | System running, no kernel     | Run `container system kernel set --recommended` |

**Fallback Behavior** (auto-detect mode):

- **Not Installed**: Silent fallback to Docker
- **Partially Configured**: Error with setup guidance (user chose Apple Containerization but didn't finish setup)

### Resource Limits

```bash
# Via alca.toml
[resources]
memory = "4g"
cpus = 4

# Translates to:
container run -m 4g -c 4 ...
```

**Default per-container resources:**

- Memory: 1 GiB
- CPUs: 4

**Notes:**

- Each container has independent resource limits (no shared VM)
- Memory released immediately when container stops
- 1 MiB memory granularity

### macOS 15 Limitations

On macOS 15 (Sequoia), the container CLI works with restrictions:

- No container-to-container networking
- Container IP not reachable from host
- Port publishing (`-p`) still works

### Troubleshooting

| Issue                         | Solution                                                                    |
| ----------------------------- | --------------------------------------------------------------------------- |
| "container CLI not installed" | Download and install from GitHub releases                                   |
| "system not running"          | Run `container system start`                                                |
| "kernel not configured"       | Run `container system kernel set --recommended`                             |
| "not on macOS"                | Apple Containerization is macOS-only                                        |
| Build issues                  | Check `container builder start --cpus 8 --memory 32g` for builder VM config |

## Comparison Table

| Feature                  | Docker (macOS)        | Docker (Linux)       | Podman (macOS)       | Podman (Linux)       | Apple Containerization  |
| ------------------------ | --------------------- | -------------------- | -------------------- | -------------------- | ----------------------- |
| **VM Model**             | Single shared VM      | None (native)        | Single shared VM     | None (native)        | Per-container micro-VM  |
| **Idle Memory**          | 3-4 GB                | ~0                   | 3-4 GB               | ~0                   | ~0                      |
| **Memory Release**       | Manual                | Automatic            | Manual               | Automatic            | Automatic on stop       |
| **Resource Isolation**   | Shared VM             | Native cgroups       | Shared VM            | Native cgroups       | Complete (separate VMs) |
| **Host Config Needed**   | Yes (VM settings)     | No                   | Yes (VM settings)    | No                   | No                      |
| **Per-container Limits** | Yes (`-m`, `--cpus`)  | Yes (`-m`, `--cpus`) | Yes (`-m`, `--cpus`) | Yes (`-m`, `--cpus`) | Yes (`-m`, `-c`)        |
| **Live Update**          | `docker update`       | `docker update`      | Limited              | Limited              | No                      |
| **Rootless**             | No                    | Optional             | No                   | Yes (default)        | N/A                     |
| **Platform**             | macOS, Linux, Windows | Linux                | macOS, Linux         | Linux                | macOS 26+ only          |

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
- [Apple Container Repository](https://github.com/apple/container)
- [Apple Container Command Reference](https://github.com/apple/container/blob/main/docs/command-reference.md)
- [Apple Container How-To Guide](https://github.com/apple/container/blob/main/docs/how-to.md)
