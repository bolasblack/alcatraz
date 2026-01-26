---
title: "Keep CLI over SDK for Container Runtime"
description: "Use CLI commands instead of SDK for Docker/Podman operations"
tags: runtime
---

## Context

Evaluated whether to switch from CLI-based Docker/Podman invocation to SDK-based approaches (e.g., `github.com/docker/docker/client`).

Research covered:
- OCI/CRI/containerd standards
- Go SDK options for Docker, Podman, containerd
- Unified abstraction libraries
- Apple Containerization compatibility

Full research: `.agents/references/container-standards-research.md`

## Decision

**Keep CLI approach** for container runtime operations.

### Problems with SDK Approach

1. **Podman requires extra user configuration**
   - Podman is daemonless by design, no socket file by default
   - Users must manually start `podman system service` or configure systemd socket
   - CLI (`podman run`) works out of the box without any configuration

2. **No mature unified abstraction library**
   - Docker SDK (`github.com/docker/docker/client`) - Docker only
   - Podman bindings (`github.com/containers/podman/v5/pkg/bindings`) - Podman only
   - Only workaround: Docker SDK + Podman's Docker-compatible API (not 100% compatible)
   - No single library abstracts both runtimes cleanly

3. **Increased maintenance burden**
   - SDK versions must track Docker/Podman API versions
   - Two runtimes require separate testing paths
   - Larger binary size from SDK dependencies

4. **Limited benefit: cannot cover all target runtimes**
   - Apple Containerization (macOS 26+) has no Go SDK, only Swift
   - Even with SDK approach, would still need CLI wrapper for Apple Containerization
   - SDK approach doesn't fully achieve the goal of unified multi-runtime support

### Benefits of CLI Approach

- Works with both Docker and Podman out of the box
- No user configuration required
- Simple implementation, low maintenance cost
- Users can customize runtime configuration independently

## Consequences

- Continue using `os/exec` to invoke `docker`/`podman` commands
- Use JSON output format (`--format '{{json .}}'`) for structured parsing where available
- May revisit if a mature unified SDK abstraction emerges or specific SDK features become necessary
