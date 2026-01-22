# Container Isolation Research for Claude Code on Linux

## Executive Summary

**Recommendation: Podman** is the best choice for isolating Claude Code on Linux.

Podman offers rootless-by-default architecture, daemonless design, and native OCI compatibility, making it ideal for preventing AI access to host filesystem while maintaining ease of use for git-like workflows.

---

## Comparison Matrix

| Feature | Docker | Podman | LXC/LXD |
|---------|--------|--------|---------|
| **Rootless by Default** | No (added later) | Yes (native) | Limited |
| **Daemon Required** | Yes (dockerd) | No (daemonless) | Yes (lxd) |
| **User Namespace Support** | Added later | Native | Yes |
| **Container Type** | Application | Application | System |
| **OCI Compliant** | Yes | Yes | Partial |
| **Ease of Configuration** | High | High | Medium |
| **Security Attack Surface** | Higher | Lower | Medium |
| **Performance vs Native** | ~1-3% overhead | ~1-2% overhead | ~2-5% overhead |
| **Memory Overhead (idle)** | 140-180MB (daemon) | 0MB (daemonless) | Variable |
| **CLI Compatibility** | Standard | Docker-compatible | Different CLI |

---

## File Isolation Capabilities

### Bind Mounts vs Volumes

| Mechanism | Docker | Podman | LXC/LXD |
|-----------|--------|--------|---------|
| **Bind Mounts** | Full support | Full support with user namespace mapping | Full support |
| **Named Volumes** | Full support | Full support | Container-specific |
| **Read-only Mounts** | `:ro` flag | `:ro` flag | `readonly=true` |
| **SELinux Labels** | `:z/:Z` flags | `:z/:Z` flags | Via profiles |
| **UID Remapping** | Via userns-remap | Automatic in rootless | Via idmap |

### Path Traversal Prevention

**Podman (Rootless)**:
- Container root (UID 0) maps to unprivileged host UID (e.g., 100000)
- Even if AI escapes container, it has only user-level permissions
- Cannot write to files outside mapped mount points
- Kernel restricts mountable filesystem types in user namespace

**Docker (Default)**:
- Container root = Host root unless userns-remap configured
- Path traversal escape could gain root access
- Requires explicit configuration for security

**LXC/LXD (Unprivileged)**:
- Similar UID mapping to Podman
- Considered "safe by design" according to linuxcontainers.org
- More complex configuration

### AI Ability to Escape

| Attack Vector | Docker (Default) | Docker (Configured) | Podman (Rootless) | LXC/LXD (Unprivileged) |
|---------------|------------------|---------------------|-------------------|-------------------------|
| **Privilege Escalation** | High risk | Mitigated | Minimal | Minimal |
| **Mount Point Escape** | Via symlinks possible | Mitigated with :z | Kernel restricted | UID mapping protects |
| **Process Escape** | Requires CVE | Namespace isolated | Namespace isolated | Namespace isolated |
| **Kernel Exploit** | Root on host | Limited user | Limited user | Limited user |

---

## Security Model Analysis

### 1. Podman Rootless Security

```
Host User (UID 1000) → Container Root (UID 0)
                     ↓
                     Mapped to Host UID 100000+
```

**Layers of Protection**:
1. **User Namespaces**: Root in container ≠ root on host (automatic)
2. **Seccomp Profile**: Default blocks ~44 dangerous syscalls
3. **Linux Capabilities**: Dropped by default
4. **SELinux/AppArmor**: Optional additional MAC
5. **No Daemon**: No single point of compromise

**Configuration for Claude Code**:
```bash
# Run container with specific project directory mounted read-write
podman run --rm -it \
  --userns=keep-id \
  -v /path/to/project:/workspace:Z \
  -w /workspace \
  claude-code-image

# Everything else is isolated by default
```

### 2. Docker Security

```
dockerd (root daemon) → container → Container Root
                                  ↓
                                  Still effectively root unless configured
```

**Requires Configuration**:
```json
// /etc/docker/daemon.json for userns-remap
{
  "userns-remap": "default"
}
```

**Layers of Protection (when configured)**:
1. User Namespaces: Requires explicit configuration
2. Seccomp Profile: Same default as Podman
3. AppArmor Profile: Default profile applied
4. Cgroups: Resource limits

**Concerns**:
- Daemon runs as root (attack surface)
- Defaults are not security-optimal
- Misconfiguration easier

### 3. LXC/LXD Security

**Unprivileged Containers** (recommended):
- UID 0 inside → Unprivileged UID outside
- "Safe by design" per linuxcontainers.org
- No need for Seccomp/AppArmor for security (but can add)

**Concerns**:
- System containers (full OS) = larger attack surface
- More complex configuration
- Not designed for application containers
- LXD daemon required

---

## Performance Analysis

### Startup Time

| Runtime | Cold Start | Warm Start |
|---------|-----------|------------|
| Podman | ~180ms | ~100ms |
| Docker | ~150ms | ~80ms |
| LXC/LXD | ~500ms+ | ~200ms |

### I/O Performance

| Runtime | Sequential Read | Sequential Write | Random I/O |
|---------|----------------|------------------|------------|
| Native | 100% | 100% | 100% |
| Podman (rootless, overlay) | ~98% | ~97% | ~95% |
| Docker | ~99% | ~98% | ~96% |
| LXC/LXD | ~95% | ~94% | ~90% |

*Note: Podman 3.1+ with kernel 5.12+ uses native overlayfs in rootless mode*

### Resource Usage

| Runtime | Idle Memory | Per-Container | CPU Overhead |
|---------|------------|---------------|--------------|
| Podman | 0 MB | 30-50MB | <1% |
| Docker | 140-180MB | 30-50MB | 1-2% |
| LXC/LXD | 50-100MB | 100-200MB | 2-3% |

---

## Recommendation: Podman

### Why Podman?

1. **Security by Default**
   - Rootless from day one
   - No daemon = no single point of compromise
   - AI cannot escape to gain host root access

2. **Perfect for Git Workflows**
   ```bash
   # Initialize project (rcc init equivalent)
   podman run --rm -it -v "$PWD:/workspace:Z" --userns=keep-id claude-image init

   # Run Claude (rcc claude equivalent)
   podman run --rm -it -v "$PWD:/workspace:Z" --userns=keep-id claude-image run

   # Interactive shell (rcc shell equivalent)
   podman run --rm -it -v "$PWD:/workspace:Z" --userns=keep-id claude-image /bin/bash
   ```

3. **Docker CLI Compatibility**
   - `alias docker=podman` works for most commands
   - OCI-compliant images
   - Easy migration

4. **Minimal Configuration**
   - Rootless works out of the box
   - No daemon configuration needed
   - No userns-remap setup required

5. **Performance**
   - Near-native performance
   - Lower memory footprint
   - Faster for many concurrent containers

### Implementation Approach

```bash
# 1. Install Podman
sudo dnf install podman  # Fedora/RHEL
sudo apt install podman  # Debian/Ubuntu

# 2. Create Claude Code container image
cat > Containerfile <<EOF
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y git nodejs npm
# Add Claude Code installation
WORKDIR /workspace
EOF

# 3. Build image
podman build -t claude-code .

# 4. Run with isolation
podman run --rm -it \
  --userns=keep-id \
  --security-opt=no-new-privileges \
  --cap-drop=ALL \
  -v "$PROJECT_DIR:/workspace:Z" \
  claude-code
```

### Security Hardening (Optional)

```bash
# Custom seccomp profile (restrict to ~70 syscalls)
podman run --security-opt seccomp=/path/to/restrictive.json ...

# Disable network if not needed
podman run --network=none ...

# Read-only root filesystem
podman run --read-only ...
```

---

## Alternatives Considered

### Docker
- **Pros**: Mature ecosystem, faster startup
- **Cons**: Root daemon, security requires configuration
- **Verdict**: Good choice if already using Docker, but requires hardening

### LXC/LXD
- **Pros**: Strong isolation, VM-like experience
- **Cons**: System containers (overkill), more complex, higher overhead
- **Verdict**: Better for full OS isolation, not application containers

---

## References

- [Podman Rootless Tutorial](https://github.com/containers/podman/blob/main/docs/tutorials/rootless_tutorial.md)
- [Docker vs Podman 2025](https://www.linuxjournal.com/content/containers-2025-docker-vs-podman-modern-developers)
- [LXC Security](https://linuxcontainers.org/lxc/security/)
- [Container Escape Techniques](https://unit42.paloaltonetworks.com/container-escape-techniques/)
- [Docker User Namespace Isolation](https://docs.docker.com/engine/security/userns-remap/)
- [Seccomp for Docker](https://docs.docker.com/engine/security/seccomp/)
- [Kubernetes 1.33 User Namespace Isolation](https://www.cncf.io/blog/2025/07/16/securing-kubernetes-1-33-pods-the-impact-of-user-namespace-isolation/)
