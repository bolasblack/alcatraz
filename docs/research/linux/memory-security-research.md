# Linux Container Memory Management and Security Hardening Research

## Executive Summary

This document provides comprehensive research on Linux container memory management mechanisms and security hardening options. The key finding confirms that **Linux containers only consume host memory proportional to their actual usage, not their configured limits** - similar to macOS Virtualization.framework behavior.

---

## 1. Cgroups v2 Memory Controller Analysis

### 1.1 Memory Control Interfaces

| Interface | Type | Behavior |
|-----------|------|----------|
| `memory.current` | Read-only | Shows total current memory usage (RSS + page cache + kernel buffers) |
| `memory.max` | Hard limit | Triggers OOM killer if exceeded |
| `memory.high` | Throttle limit | Throttles processes, triggers aggressive reclaim (never OOMs) |
| `memory.low` | Soft protection | Memory below this is protected from reclaim unless necessary |
| `memory.min` | Hard protection | Memory below this is never reclaimed |

### 1.2 Key Behavior: Memory is NOT Pre-allocated

**Critical finding**: Cgroups impose upper limits, NOT reservations.

From kernel documentation:
> "A cgroup only imposes an upper limit on memory usage by applications in the cgroup. It does not reserve memory for these applications and as such, memory is allocated on demand."

This means:
- Setting `--memory=8g` on a container does NOT consume 8GB of host memory
- Memory is charged only when actually used (at page fault time)
- Host memory overcommit management must consider this behavior

### 1.3 Memory Accounting Components

`memory.current` includes:
- **Anonymous memory (RSS)**: Heap, stack allocations
- **Page cache (file)**: Cached filesystem data
- **Kernel data structures**: dentries, inodes
- **Network buffers**: TCP socket buffers

To get pure RSS (anonymous memory), read `anon` from `memory.stat`:
```bash
cat /sys/fs/cgroup/<path>/memory.stat | grep "^anon "
```

### 1.4 Memory Release Behavior

When processes free memory:

1. **munmap()**: Kernel immediately releases anonymous pages to free list
2. **free()**: Depends on glibc allocator behavior:
   - Small allocations: May stay in process heap (fragmentation)
   - Large allocations (mmap-backed): Immediately released via munmap

When process exits:
- All anonymous pages are immediately released
- Page cache may persist until memory pressure or explicit drop

Manual reclaim available via:
```bash
echo 1G > /sys/fs/cgroup/<path>/memory.reclaim
```

---

## 2. Container Runtime Comparison

### 2.1 Docker

**Memory Management:**
- Uses cgroups v2 (modern systems) or v1 (legacy)
- Default: no memory limit
- `--memory`: Hard limit (maps to `memory.max`)
- `--memory-reservation`: Soft limit (maps to `memory.low`)

**Verification:**
```bash
# Run container with 8GB limit
docker run -d --memory=8g --name test nginx

# Check actual usage
docker stats test
# Shows MEM USAGE / LIMIT: ~10MB / 8GB

# Host memory unaffected by limit, only by actual usage
free -m
```

### 2.2 Podman

**Key advantages over Docker for memory management:**
- **Rootless by design**: Uses cgroups v2 natively for resource limits in rootless mode
- **Daemonless**: Each container is a direct child process, better memory accounting
- **Systemd integration**: Native cgroups v2 support via systemd slice units

**Important**: Memory limits in rootless mode only work with cgroups v2:
```bash
# Check cgroups version
mount | grep cgroup
# Should show cgroup2 for v2 support
```

**Memory commands:**
```bash
podman run -d --memory=8g --name test nginx
podman stats test
```

### 2.3 LXC/LXD

**System containers vs Application containers:**
- LXC runs full OS (init system, multiple processes)
- More granular resource control available
- Better for long-lived, multi-process workloads

**Memory configuration:**
```bash
lxc config set <container> limits.memory 8GB
lxc config set <container> limits.memory.swap false
```

**Advantages:**
- Fine-grained control over every aspect
- Direct cgroup manipulation available
- Better suited for VM-like workloads

### 2.4 Runtime Comparison Summary

| Feature | Docker | Podman | LXC/LXD |
|---------|--------|--------|---------|
| Memory backend | cgroups v1/v2 | cgroups v2 | cgroups v1/v2 |
| Rootless memory limits | Requires config | Native | Native (unprivileged) |
| Daemon | Yes (dockerd) | No | Yes (lxd) |
| Best for | Microservices | Security-focused | System containers |
| Memory overhead | Minimal | Minimal | Minimal |

---

## 3. MVP Verification: Memory Auto-Release

### 3.1 Expected Behavior

**Scenario:** 8GB limit, 1GB actual usage

| Metric | Expected Value |
|--------|----------------|
| Container limit | 8GB |
| Process RSS | ~1GB |
| `memory.current` | ~1GB + page cache |
| Host memory consumed | ~1GB |

### 3.2 Verification Method (on native Linux)

```bash
# 1. Create test container with 8GB limit
docker run -d --memory=8g --name memtest python:3.12-slim \
  python3 -c "
import time
data = [b'X' * (100*1024*1024) for _ in range(10)]  # 1GB
print('Allocated 1GB')
time.sleep(300)
"

# 2. Check container stats
docker stats memtest --no-stream
# Expected: MEM USAGE shows ~1GB, LIMIT shows 8GB

# 3. Check host memory (should NOT see 8GB consumed)
free -m

# 4. Check cgroup directly
CGROUP_PATH=$(docker inspect memtest --format '{{.HostConfig.CgroupParent}}')
cat /sys/fs/cgroup/${CGROUP_PATH}/memory.current
# Should show ~1GB (1073741824 bytes)
```

### 3.3 Results Comparison with macOS

| Platform | Limit | Used | Host Memory |
|----------|-------|------|-------------|
| macOS (Virtualization.framework) | 8GB | 1GB | ~1GB |
| Linux (cgroups v2) | 8GB | 1GB | ~1GB |
| VM (without balloon) | 8GB | 1GB | 8GB |
| VM (with balloon driver) | 8GB | 1GB | Variable* |

*Balloon driver requires guest cooperation and has latency

**Conclusion:** Linux containers behave identically to macOS Virtualization.framework - only actual memory usage is consumed on the host.

---

## 4. Security Hardening Options

### 4.1 Seccomp (Syscall Filtering)

**Purpose:** Restrict system calls available to containerized processes

**Default profile:**
- Blocks ~44 dangerous syscalls out of 300+
- Blocks: `mount`, `reboot`, `clock_settime`, `ptrace`, etc.
- Most containers only need 40-70 syscalls

**Usage:**
```bash
# Docker (uses default profile automatically)
docker run --security-opt seccomp=/path/to/profile.json ...

# Podman
podman run --security-opt seccomp=/path/to/profile.json ...
```

**Best practices:**
1. Use default profile as baseline
2. Generate custom profiles using `oci-seccomp-bpf-hook`
3. Monitor audit.log for blocked syscalls
4. Run tests with custom profiles before production

### 4.2 AppArmor

**Purpose:** Mandatory Access Control (MAC) using path-based rules

**Default behavior:**
- Debian/Ubuntu systems use AppArmor by default
- Docker provides `docker-default` profile
- Limits file access, capabilities, network

**Limitations:**
- Cannot separate containers from each other (no MCS support)
- Path-based rules may have bypasses
- Simpler but less comprehensive than SELinux

**Usage:**
```bash
docker run --security-opt apparmor=docker-default ...
docker run --security-opt apparmor=unconfined ...  # Disable
```

### 4.3 SELinux

**Purpose:** MAC using labels and contexts

**Key advantages:**
- **Multi-Category Security (MCS)**: Separates containers from each other
- **Type Enforcement**: Fine-grained access control
- Default on RHEL/Fedora systems

**Container isolation:**
- Each container gets unique MCS label
- Prevents container-to-container access
- Separates containers from host filesystem

**Podman advantage:** Automatic SELinux labeling by default

**Usage:**
```bash
# Docker (on SELinux-enabled system)
docker run --security-opt label=type:container_t ...

# Podman (automatic on Fedora/RHEL)
podman run --security-opt label=disable ...  # Disable if needed
```

### 4.4 Linux Capabilities

**Purpose:** Break down root privileges into discrete units

**Default dropped capabilities:**
- `CAP_SYS_ADMIN`, `CAP_NET_ADMIN` (dangerous)
- `CAP_SYS_PTRACE`, `CAP_SYS_MODULE`

**Best practice:** Drop all, add only what's needed
```bash
docker run --cap-drop=ALL --cap-add=NET_BIND_SERVICE ...
```

### 4.5 Security Comparison Matrix

| Feature | Seccomp | AppArmor | SELinux |
|---------|---------|----------|---------|
| Scope | Syscalls | Files/Capabilities | Everything |
| Container-to-container isolation | No | No | Yes (MCS) |
| Complexity | Medium | Low | High |
| Default on | All | Debian/Ubuntu | RHEL/Fedora |
| Performance impact | Minimal | Minimal | Minimal |

### 4.6 Defense in Depth Stack

Recommended layered security (from outer to inner):

```
1. Host isolation (VM or bare metal hardening)
   └── 2. SELinux/AppArmor (MAC)
       └── 3. Seccomp (syscall filtering)
           └── 4. Capabilities (privilege reduction)
               └── 5. User namespaces (unprivileged containers)
                   └── 6. Application
```

### 4.7 Advanced Isolation: gVisor and Kata Containers

For maximum isolation (untrusted workloads):

| Runtime | Approach | Overhead | Compatibility |
|---------|----------|----------|---------------|
| **gVisor** | User-space kernel (Sentry) | 10-20% | ~70-80% syscalls |
| **Kata Containers** | Lightweight VMs | Higher | ~100% |

**gVisor:**
- Intercepts syscalls, processes in user-space
- 50-100ms startup time
- Good for user-facing, untrusted code

**Kata Containers:**
- Full hardware virtualization boundary
- 150-300ms startup time
- Required for compliance, multi-tenant clouds

---

## 5. Recommendations

### 5.1 For MVP (Memory Auto-Release)

**Confirmed:** Linux cgroups v2 provides memory auto-release behavior:
- Containers only consume memory they actually use
- Limits are upper bounds, not reservations
- Identical behavior to macOS Virtualization.framework

**Recommended setup:**
```bash
# Use cgroups v2 (verify)
mount | grep cgroup2

# Set sensible defaults
docker run \
  --memory=8g \           # Hard limit
  --memory-reservation=1g # Soft limit (guaranteed)
  ...
```

### 5.2 For Container Runtime

1. **Development/CI**: Docker (ecosystem, tooling)
2. **Production (security-focused)**: Podman (rootless, SELinux integration)
3. **VM-like workloads**: LXC/LXD (system containers)

### 5.3 For Security Hardening

**Minimum security stack:**
1. Use default seccomp profile (automatic with Docker/Podman)
2. Enable SELinux or AppArmor (distro default)
3. Drop unnecessary capabilities
4. Run as non-root user inside container

**Maximum isolation (untrusted code):**
1. gVisor for user-facing containers
2. Kata Containers for compliance requirements
3. Combine with all above measures

---

## References

- [Linux Kernel cgroups v2 Documentation](https://docs.kernel.org/admin-guide/cgroup-v2.html)
- [Facebook cgroup2 Memory Controller](https://facebookmicrosites.github.io/cgroup2/docs/memory-controller.html)
- [Docker Resource Constraints](https://docs.docker.com/engine/containers/resource_constraints/)
- [Docker Seccomp Profiles](https://docs.docker.com/engine/security/seccomp/)
- [Podman Rootless Containers](https://podman.io/blogs/2019/10/29/podman-crun-f31.html)
- [Container Security Fundamentals - Datadog](https://securitylabs.datadoghq.com/articles/container-security-fundamentals-part-5/)
- [gVisor vs Kata Containers](https://dev.to/rimelek/comparing-3-docker-container-runtimes-runc-gvisor-and-kata-containers-16j)
