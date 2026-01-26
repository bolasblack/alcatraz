# Container Standard Interfaces Research

## Executive Summary

This document explores the container ecosystem standards and evaluates whether alca should switch from CLI-based Docker/Podman invocation to SDK-based approaches. The key finding is that while SDK approaches offer benefits, the current CLI approach provides maximum flexibility with minimal maintenance overhead.

## Standards Overview

### OCI (Open Container Initiative)

The [Open Container Initiative](https://opencontainers.org/) was established in 2015 by Docker and the Linux Foundation to create open industry standards for containers. OCI defines three main specifications:

1. **[Runtime Specification (runtime-spec)](https://github.com/opencontainers/runtime-spec)**: Defines how to run a "filesystem bundle" on disk. Specifies container lifecycle operations (create, start, kill, delete) and configuration format.

2. **[Image Specification (image-spec)](https://github.com/opencontainers/image-spec)**: Defines the container image format, including layers, manifests, and configuration. Reached v1.1.0 in February 2024.

3. **[Distribution Specification](https://specs.opencontainers.org/distribution-spec/)**: Standardizes the API for distributing container images via registries. Also v1.1.0 as of 2024.

Docker donated `runc` (the reference OCI runtime implementation) to the OCI, which now serves as the foundation for most container runtimes.

### CRI (Container Runtime Interface)

The [Container Runtime Interface (CRI)](https://kubernetes.io/docs/concepts/containers/cri/) is a Kubernetes-specific gRPC API introduced in Kubernetes 1.5 (December 2016). It defines:

- **RuntimeService**: RPCs for pod/container lifecycle management (create, start, stop, remove, exec, attach)
- **ImageService**: RPCs for image management (pull, list, remove)

CRI enables Kubernetes to work with any compliant container runtime without modification. Key characteristics:
- Communication via Unix socket using gRPC
- Protocol buffers for message serialization
- Kubelet acts as client, runtime as server

**Note**: CRI is designed for Kubernetes orchestration, not general-purpose container management.

### containerd

[containerd](https://containerd.io/) is a high-level container runtime donated by Docker to CNCF (now a graduated project). It provides:

- Complete container lifecycle management
- Image transfer and storage
- Container execution and supervision
- Network attachments via plugins
- CRI plugin for Kubernetes integration

containerd is designed to be embedded into larger systems (Docker, Kubernetes) rather than used directly by end users.

## Apple Containerization (macOS 26+)

### Overview

[Apple Containerization](https://github.com/apple/containerization) is a new Swift-based framework announced at WWDC 2025 for running Linux containers on macOS. It uses Apple's Virtualization.framework to run containers in lightweight VMs.

### OCI Compatibility

**Yes, Apple Containerization is OCI-compatible**:
- Consumes and produces OCI-compatible container images
- Can pull from any standard OCI registry (Docker Hub, Quay, etc.)
- Images built with `container` CLI can be pushed to registries and run in Docker/Podman
- CLI supports building images from Dockerfiles

### Architecture

Unlike Docker/Podman which share a Linux kernel (or single VM on macOS), Apple Containerization uses a **one VM per container** model:

```
Apple Containerization Architecture:
  container CLI → Containerization.framework → Virtualization.framework → Hypervisor.framework
                                                     ↓
                                              One micro-VM per container
```

Key characteristics:
- Each container runs in its own lightweight VM
- Hardware-level isolation (stronger security than shared kernel)
- Sub-second startup times despite VM overhead
- Optimized Linux kernel for containerization workloads
- Written in Swift, uses musl for static linking

### Requirements & Limitations

| Requirement | Detail |
|-------------|--------|
| **Hardware** | Apple Silicon only (M1/M2/M3/M4) |
| **macOS** | macOS 26 (Tahoe) or later |
| **Intel Mac** | Not supported |
| **Older macOS** | Not supported |

### CLI Compatibility

The `container` CLI is **not Docker-compatible**:
- Different command syntax from Docker/Podman
- No `alias container=docker` compatibility
- Different approach to networking, volumes, etc.

Example commands:
```bash
# Pull and run an image
container run --rm alpine echo "Hello"

# Build from Dockerfile
container build -t myimage .
```

### Integration with alca

**Current Assessment**: Apple Containerization is NOT a drop-in replacement for Docker/Podman:

1. **Different CLI interface**: Would require a separate runtime implementation
2. **Platform restrictions**: Only macOS 26+ with Apple Silicon
3. **OCI compatible but not CLI compatible**: Images work, commands don't
4. **No Go SDK**: Framework is Swift-only, no official Go bindings

**Potential Future Integration**:
- Could be added as a third runtime option for macOS users
- Would require wrapping the `container` CLI similar to Docker/Podman
- Images built on Docker/Podman would work in Apple Containerization and vice versa

### Comparison Table

| Feature | Docker | Podman | Apple Containerization |
|---------|--------|--------|------------------------|
| **OCI Images** | ✅ Yes | ✅ Yes | ✅ Yes |
| **CLI Compatible** | Native | Docker-compatible | Different |
| **Architecture** | Daemon + containerd | Daemonless | One VM per container |
| **macOS Support** | Via Docker Desktop | Via Podman Desktop | Native (Apple Silicon only) |
| **Linux Support** | ✅ Native | ✅ Native | ❌ No |
| **Windows Support** | ✅ WSL2/Hyper-V | ✅ WSL2 | ❌ No |
| **Go SDK** | ✅ Official | ✅ Official | ❌ None (Swift only) |
| **Isolation** | Shared kernel | Shared kernel | VM per container |

### References

- [Apple Containerization GitHub](https://github.com/apple/containerization)
- [Apple Container CLI GitHub](https://github.com/apple/container)
- [WWDC 2025 - Meet Containerization](https://developer.apple.com/videos/play/wwdc2025/346/)
- [Apple Containers vs Docker Comparison](https://thenewstack.io/apple-containers-on-macos-a-technical-comparison-with-docker/)

## Docker & Podman Standards Compliance

### Both Follow OCI Standards

| Aspect | Docker | Podman |
|--------|--------|--------|
| **Image Format** | OCI/Docker v2 | OCI/Docker v2 |
| **Runtime** | runc (via containerd) | runc or crun |
| **Registry Protocol** | OCI Distribution | OCI Distribution |
| **CLI Compatibility** | Native | Docker-compatible (`alias docker=podman`) |

Both tools are **fully OCI-compliant**, meaning:
- Images built with Docker work in Podman and vice versa
- Container configurations follow the same OCI runtime-spec
- Both can push/pull from any OCI-compliant registry

### Architectural Differences

```
Docker Architecture:
  docker CLI → dockerd daemon → containerd → runc

Podman Architecture:
  podman CLI → libpod → runc/crun
```

Key differences:
- **Docker**: Daemon-based, requires root for daemon
- **Podman**: Daemonless, native rootless support
- **Podman API**: HTTP REST API compatible with Docker API (since v3.0)

## Compatible Container Runtimes

### Low-Level OCI Runtimes

| Runtime | Language | Key Features | Used By |
|---------|----------|--------------|---------|
| **[runc](https://github.com/opencontainers/runc)** | Go | OCI reference implementation, industry standard | Docker, containerd, CRI-O |
| **[crun](https://github.com/containers/crun)** | C | Better performance, cgroups v2 support | Podman (default on Fedora) |
| **[Kata Containers](https://katacontainers.io/)** | Multiple | VM-based isolation, enhanced security | Optional for CRI-O, containerd |
| **[gVisor](https://gvisor.dev/)** | Go | User-space kernel, sandboxed | Optional for Docker, containerd |

### High-Level Runtimes (CRI-compatible)

| Runtime | Focus | Default Low-Level Runtime |
|---------|-------|--------------------------|
| **containerd** | General purpose, Docker's backend | runc |
| **CRI-O** | Kubernetes-specific, lightweight | runc/crun |

### Switching to Standard Interface Benefits

Using a standard OCI-compliant interface could support:
- containerd directly (bypassing Docker)
- CRI-O (for Kubernetes-like setups)
- Any future OCI-compliant runtime
- Podman via its Docker-compatible API

## Available Go Libraries

### Primary Options

| Library | Runtime | API Style | Maturity | Pros | Cons |
|---------|---------|-----------|----------|------|------|
| **[github.com/docker/docker/client](https://pkg.go.dev/github.com/docker/docker/client)** | Docker | HTTP REST | Stable | Official, well-documented, type-safe | Docker-specific, large dependency tree |
| **[github.com/containers/podman/v5/pkg/bindings](https://pkg.go.dev/github.com/containers/podman/v5/pkg/bindings)** | Podman | HTTP REST | Stable | Official, Docker-compatible endpoints available | Requires Podman service running |
| **[github.com/containerd/containerd/v2/client](https://pkg.go.dev/github.com/containerd/containerd/v2/client)** | containerd | gRPC | Stable | Low-level control, high performance | Lower-level, more complex, containerd-specific |
| **[github.com/fsouza/go-dockerclient](https://github.com/fsouza/go-dockerclient)** | Docker | HTTP REST | Maintained | Alternative to official SDK | Lags behind official SDK |

### Unified Abstraction Libraries

| Library | Status | Runtimes Supported |
|---------|--------|-------------------|
| **[containers-wrapper](https://github.com/linux-immutability-tools/containers-wrapper)** | Early stage (v0.x) | Docker, Podman, containerd (CLI wrapper) |

**Note**: `containers-wrapper` is a CLI wrapper, not an SDK abstraction. True unified SDK abstraction libraries are rare because:
1. Each runtime has different capabilities
2. API semantics differ significantly
3. Low-level operations don't map cleanly across runtimes

### Library Details

#### Docker SDK (`github.com/docker/docker/client`)
```go
import "github.com/docker/docker/client"

cli, err := client.NewClientWithOpts(client.FromEnv)
containers, err := cli.ContainerList(ctx, container.ListOptions{})
```

Features:
- Full Docker Engine API coverage
- Environment-based configuration (DOCKER_HOST, etc.)
- Concurrent-safe client
- Extensive type definitions

#### Podman Bindings (`github.com/containers/podman/v5/pkg/bindings`)
```go
import "github.com/containers/podman/v5/pkg/bindings"
import "github.com/containers/podman/v5/pkg/bindings/containers"

conn, err := bindings.NewConnection(ctx, "unix:///run/podman/podman.sock")
containers, err := containers.List(conn, nil)
```

Features:
- Requires Podman service running (`podman system service`)
- Separate packages for images, containers, pods, etc.
- Docker-compatible API endpoints also available

#### containerd Client (`github.com/containerd/containerd/v2/client`)
```go
import "github.com/containerd/containerd/v2/client"

client, err := client.New("/run/containerd/containerd.sock")
containers, err := client.Containers(ctx)
```

Features:
- Direct containerd access (bypasses Docker)
- Namespace isolation support
- Lower-level, more control over snapshots and content
- gRPC-based (higher performance than HTTP)

## Feasibility Assessment

### Current: CLI Approach

**How it works**: alca invokes `docker` or `podman` commands via `os/exec`.

**Advantages**:
- ✅ Maximum flexibility - works with any CLI-compatible runtime
- ✅ No SDK version coupling
- ✅ Simple implementation and debugging
- ✅ Users can customize Docker/Podman configuration independently
- ✅ No daemon connection management
- ✅ Works with remote Docker hosts via `DOCKER_HOST`
- ✅ Naturally supports both Docker and Podman

**Disadvantages**:
- ❌ CLI output parsing can be fragile
- ❌ Less type safety
- ❌ Slight performance overhead from process spawning
- ❌ Error handling depends on exit codes and stderr parsing

### Alternative: SDK Approach

**How it would work**: Use Docker SDK or Podman bindings directly.

**Advantages**:
- ✅ Type-safe API calls
- ✅ Structured error handling
- ✅ Better performance (no process spawning)
- ✅ Access to streaming APIs (logs, events)
- ✅ No CLI output parsing

**Disadvantages**:
- ❌ Must choose: Docker SDK OR Podman bindings (or both)
- ❌ SDK version must track runtime API version
- ❌ Larger binary size from dependencies
- ❌ Connection management complexity
- ❌ Two separate implementations for Docker/Podman support
- ❌ Testing becomes more complex (need mock servers)
- ❌ containerd access requires even more different approach

### Alternative: containerd Direct

**How it would work**: Use containerd client library directly.

**Advantages**:
- ✅ Works with Docker (which uses containerd)
- ✅ Potentially works with containerd-only setups
- ✅ High performance gRPC API

**Disadvantages**:
- ❌ Doesn't work with Podman (different architecture)
- ❌ Lower-level API requires more code
- ❌ Image pull/push more complex
- ❌ Would lose Podman support entirely

### Alternative: Podman Docker-Compatible API

**How it would work**: Use Docker SDK against Podman's compatibility endpoint.

**Advantages**:
- ✅ Single SDK for both runtimes
- ✅ Type-safe, structured

**Disadvantages**:
- ❌ Compatibility is not 100%
- ❌ Requires Podman service running (systemd or manual)
- ❌ Some Docker features may behave differently

## Recommendation

**Keep the CLI approach** for the following reasons:

1. **alca's use case is simple**: Pull images, create/start/stop/remove containers. These are straightforward CLI operations that don't benefit significantly from SDK type safety.

2. **Dual runtime support**: CLI approach naturally supports both Docker and Podman with minimal code differences. SDK approach would require either:
   - Two separate implementations
   - Podman's Docker-compatible API (imperfect compatibility)

3. **Minimal dependency footprint**: CLI approach has no external Go dependencies for container operations.

4. **User flexibility**: Users can configure Docker/Podman however they want, including remote hosts, custom sockets, rootless setups, etc.

5. **Maintenance burden**: SDK versions must track runtime versions. CLI commands are more stable over time.

### Suggested Improvements for Current CLI Approach

Instead of switching to SDKs, consider these improvements:

1. **Use JSON output where available**:
   ```bash
   docker inspect --format='{{json .}}' container_id
   docker ps --format='{{json .}}'
   ```
   This provides structured output without SDK dependencies.

2. **Structured error handling**: Parse stderr for known error patterns rather than just checking exit codes.

3. **Timeout handling**: Ensure all CLI commands have appropriate timeouts.

4. **Abstract runtime detection**: Clean interface to detect and select between Docker/Podman.

### When SDK Might Make Sense

Consider SDK migration if:
- alca needs streaming logs or real-time events
- alca needs to perform many operations rapidly (batch processing)
- CLI parsing becomes a significant maintenance burden
- A stable unified abstraction library emerges

### Apple Containerization Support

Apple Containerization could be added as an **optional third runtime** for macOS users:

**Pros**:
- Native macOS support without Docker Desktop licensing concerns
- Strong isolation via VM-per-container model
- OCI-compatible images work without modification

**Cons**:
- Requires macOS 26+ and Apple Silicon (limited user base currently)
- Different CLI requires separate implementation
- No Go SDK, would need CLI wrapper
- Platform-specific feature adds maintenance burden

**Recommendation**: Monitor Apple Containerization adoption. Consider adding support once:
1. macOS 26 reaches wider adoption
2. The `container` CLI stabilizes
3. User demand emerges for native macOS container support

## References

- [Open Container Initiative](https://opencontainers.org/)
- [OCI Runtime Specification](https://github.com/opencontainers/runtime-spec)
- [OCI Image Specification](https://github.com/opencontainers/image-spec)
- [Kubernetes CRI Documentation](https://kubernetes.io/docs/concepts/containers/cri/)
- [containerd](https://containerd.io/)
- [Docker Go SDK Documentation](https://pkg.go.dev/github.com/docker/docker/client)
- [Podman Go Bindings](https://podman.io/blogs/2020/08/10/podman-go-bindings.html)
- [containerd vs Docker](https://www.docker.com/blog/containerd-vs-docker/)
- [Docker OCI Specifications](https://www.docker.com/blog/demystifying-open-container-initiative-oci-specifications/)
