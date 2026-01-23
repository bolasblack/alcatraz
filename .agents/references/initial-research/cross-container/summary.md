# Cross-Container Claude Communication - Research Report

## Executive Summary

Researched ccc-statusd architecture and evaluated four cross-container communication patterns. **Recommendation: Host Relay pattern using ccc-statusd** - it preserves file isolation, maintains RFC1918 network blocking, and provides centralized control.

---

## 1. ccc-statusd Architecture Overview

### Communication Mechanism
- **Unix socket** at `~/.cache/ccc-status/daemon.sock` (mode 0600)
- JSON-over-newline protocol with message types: register, associate, heartbeat, prompt, command, eval, query

### State Storage
| Storage | Location | Purpose |
|---------|----------|---------|
| SQLite | `scheduled.db` | Scheduled messages, session metadata |
| JSON cache | `<session-id>.json` | Fast session lookups |
| In-memory maps | `clients`, `sessionToInternal` | Active connection routing |

### Key Insight
ccc-statusd already implements the Host Relay pattern - daemon on host, injections connect via socket, daemon routes messages between sessions.

---

## 2. Communication Patterns Comparison

| Pattern | Latency | File Isolation | Network Isolation | Risk | Score |
|---------|---------|----------------|-------------------|------|-------|
| **Host Relay** | Medium | ✅ Preserved | ✅ Compatible | LOW | 5/5 |
| Unix Socket (direct) | Lowest | ✅ Preserved | ✅ Excellent | LOW | 5/5 |
| File IPC | Medium | ⚠️ At Risk | ✅ Excellent | MEDIUM | 4/5 |
| Container Network | Higher | ✅ Preserved | ❌ Conflicts RFC1918 | HIGH | 3/5 |

---

## 3. Recommended Approach

### Primary: Host Relay via ccc-statusd

**Configuration:**
```bash
# Host runs ccc-statusd daemon
ccc-statusd start

# Container A mounts socket
docker run -v ~/.cache/ccc-status/daemon.sock:/sockets/daemon.sock:z container-a

# Container B mounts same socket
docker run -v ~/.cache/ccc-status/daemon.sock:/sockets/daemon.sock:z container-b
```

**Why Host Relay:**
1. No direct container-to-container access
2. Central policy enforcement, rate limiting
3. Complete audit logging
4. RFC1918 compatible (no network between containers)
5. Already implemented in ccc-statusd

### Fallback: Shared Unix Socket (Direct)

For direct agent-to-agent on same host:
```bash
# Create shared socket directory
mkdir -p /shared/sockets

# Mount to both containers
docker run -v /shared/sockets:/ipc:z container-a  # Creates socket
docker run -v /shared/sockets:/ipc:z container-b  # Connects
```

---

## 4. Security Analysis

### Threat Model
- **Assets**: Project files, credentials, agent context
- **Threats**: Prompt injection, compromised agent, side-channels

### Risk Assessment

| Pattern | Interception | Modification | File Isolation | Overall |
|---------|--------------|--------------|----------------|---------|
| Host Relay | Low | Very Low | Preserved | **LOW** ✅ |
| Unix Socket | Low | Low | Preserved | **LOW** ✅ |
| File IPC | Medium | High | At Risk | **MEDIUM** ⚠️ |
| Container Network | Medium | Medium | Preserved | **HIGH** ❌ |

### Container Network Rejection
- Uses RFC1918 addresses (172.x, 10.x)
- Conflicts with network isolation requirement
- Opens large attack surface

---

## 5. Implementation Example

### Minimal Cross-Container Setup

```bash
# 1. Start ccc-statusd on host
ccc-statusd start

# 2. Get socket path
SOCKET_PATH=$(ccc-statusd path --socket)

# 3. Run Container A (Project X)
docker run \
  -v "$SOCKET_PATH:/run/ccc.sock" \
  -v /projects/x:/workspace:ro \
  --network none \
  claude-container-a

# 4. Run Container B (Project Y)
docker run \
  -v "$SOCKET_PATH:/run/ccc.sock" \
  -v /projects/y:/workspace:ro \
  --network none \
  claude-container-b

# 5. Agent A sends message to Agent B
# Inside container A:
ccc-statusd session send <session-b-id> "Hello from Agent A"
```

**Security guarantees:**
- Container A cannot access `/projects/y` (not mounted)
- Container B cannot access `/projects/x` (not mounted)
- No network between containers (`--network none`)
- Communication only via daemon socket

---

## 6. Deliverables Summary

| File | Content |
|------|---------|
| `/tmp/f3ea440e/arch-findings.md` | ccc-statusd architecture deep dive |
| `/tmp/f3ea440e/comm-findings.md` | Communication patterns comparison |
| `/tmp/f3ea440e/security-findings.md` | Security analysis and threat model |
| `/tmp/f3ea440e/final-report.md` | This synthesized report |

---

## 7. Conclusion

**Use ccc-statusd Host Relay pattern** for cross-container Claude communication:
- ✅ File isolation preserved
- ✅ Network isolation maintained (RFC1918 compatible)
- ✅ Minimal attack surface
- ✅ Already implemented and working
