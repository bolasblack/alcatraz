# VM Resource Configuration for Container Runtimes

Research on VM resource configuration requirements for different container runtimes.

## Research Questions & Answers

### 1. macOS Docker: Does using large memory require configuration?

**Answer: Yes, configuration required.**

Docker Desktop on macOS runs containers in a Linux VM. The VM has a **fixed resource allocation** set at startup:
- Default memory varies by system (typically 2-4GB)
- Memory must be allocated **before** Docker starts
- VM cannot dynamically resize memory while running

**Configuration methods:**
- GUI: Docker Desktop > Settings > Resources > Advanced
- File: `~/.docker/daemon.json`
- CLI: Not directly available (must restart Docker Desktop)

**Key limitation:** Docker Desktop's VM **does not release unused memory** back to macOS. This is a known long-standing issue ([docker/for-mac#6120](https://github.com/docker/for-mac/issues/6120)).

### 2. Linux Containers: Does using large memory require configuration?

**Answer: No special configuration needed for host memory; cgroups handles container limits.**

On native Linux, containers share the host kernel directly. Memory limits work via cgroups:

**No host-level config needed because:**
- Containers access host memory directly (no VM layer)
- Memory is allocated on-demand
- Unused memory is released automatically to OS

**Container-level limits via cgroups:**
```bash
# cgroups v1
echo 1G > /sys/fs/cgroup/memory/mygroup/memory.limit_in_bytes

# cgroups v2
echo 1073741824 > /sys/fs/cgroup/mygroup/memory.max
```

### 3. Apple Containerization: VM Resource Limit Mechanism

**Answer: Per-container VM with dynamic allocation.**

Apple Containerization uses a fundamentally different model:
- **Each container runs in its own lightweight VM**
- Resources allocated **per-container**, not to a shared VM
- **Zero overhead when no containers running** (no persistent VM)

**Resource management characteristics:**
| Feature | Apple Container | Docker Desktop |
|---------|-----------------|----------------|
| VM Model | Per-container micro-VM | Single shared VM |
| Idle Memory | ~0 | 3-4GB |
| Memory Release | Automatic on container stop | Manual/Never |
| Resource Isolation | Complete (separate VMs) | Shared (same VM) |

**Default resources per container:**
- Memory: 1 GiB
- CPUs: 4

**Limitations:**
- Memory ballooning only partially implemented
- Higher per-container overhead (separate VM per container)
- Apple Silicon IPA limit: 36 bits (max ~64GB VM)

### 4. Runtime Resource Limit APIs/CLIs

#### Docker

```bash
# Memory limits
docker run -m 512m myimage                    # Hard limit
docker run --memory-swap 1g myimage           # Total memory+swap
docker run --memory-reservation 256m myimage  # Soft limit

# CPU limits
docker run --cpus 0.5 myimage                 # Limit to 0.5 CPU
docker run --cpuset-cpus "0,1" myimage        # Pin to specific CPUs
docker run --cpu-shares 512 myimage           # Relative weight

# Update running container
docker update --memory 1g container_name
```

**Memory suffixes:** b, k, m, g (bytes, KB, MB, GB)

#### Podman (macOS)

Podman on macOS requires a VM (podman machine):

```bash
# Configure at machine creation
podman machine init --cpus 4 --memory 8192 --disk-size 100

# Modify existing machine (QEMU only)
podman machine stop
podman machine set --cpus 8 --memory 16384
podman machine start
```

**Note:** CPU/memory modification only works with QEMU backend, not Apple Hypervisor.

**Container-level limits (same as Docker):**
```bash
podman run -m 512m --cpus 2 myimage
```

#### Apple Containerization

```bash
# Per-container limits
container run --cpus 8 --memory 4g myimage
container run -c 2 -m 1G myimage              # Short flags

# Builder VM limits
container builder start --cpus 8 --memory 32g

# Modify builder (requires restart)
container builder stop
container builder delete
container builder start --cpus 8 --memory 32g
```

**Memory suffixes:** K, M, G, T, P (1 MiB granularity)

## Comparison Summary

| Feature | Docker (macOS) | Podman (macOS) | Apple Container | Docker (Linux) |
|---------|----------------|----------------|-----------------|----------------|
| Host Config Needed | Yes (VM) | Yes (VM) | No | No |
| Per-container Limits | Yes | Yes | Yes | Yes |
| Dynamic Memory | No | No | Per-container | Native |
| Memory Release | Manual | Manual | Auto on stop | Automatic |
| CLI Flags | `-m`, `--cpus` | `-m`, `--cpus` | `-m`, `-c` | `-m`, `--cpus` |
| Live Update | `docker update` | Limited | No | `docker update` |

## Recommendation for Alca

### Should alca support resource configuration?

**Recommendation: Yes, but as pass-through only.**

**Rationale:**
1. All target runtimes support resource limits via their native CLI
2. Resource needs vary significantly by use case
3. Alca can simply pass-through `--memory` and `--cpus` flags

### Proposed Implementation

**Minimal approach:**
```bash
# alca passes through to underlying runtime
alca run --memory 4g --cpus 2 myimage

# Translates to:
# Docker: docker run -m 4g --cpus 2 myimage
# Apple:  container run -m 4g -c 2 myimage
```

**Configuration file support (optional):**
```toml
# alca.toml
[defaults]
memory = "2g"
cpus = 4
```

### What alca should NOT do

1. **Don't abstract away runtime differences** - Each runtime has different capabilities
2. **Don't implement memory management** - Let runtimes handle it
3. **Don't set default limits** - Users know their needs better

### Priority

**Low priority for MVP.** Resource configuration is:
- Already supported by all runtimes
- Not core to alca's isolation purpose
- Can be added later as pass-through

## Sources

- [Docker Resource Constraints](https://docs.docker.com/engine/containers/resource_constraints/)
- [Docker Desktop Settings](https://docs.docker.com/desktop/settings-and-maintenance/settings/)
- [Apple Container How-To](https://github.com/apple/container/blob/main/docs/how-to.md)
- [Apple Container Command Reference](https://github.com/apple/container/blob/main/docs/command-reference.md)
- [Podman Machine Init](https://docs.podman.io/en/latest/markdown/podman-machine-init.1.html)
- [Cgroups Memory Controller](https://facebookmicrosites.github.io/cgroup2/docs/memory-controller.html)
- [Linux Kernel Cgroups v1 Memory](https://www.kernel.org/doc/Documentation/cgroup-v1/memory.txt)
