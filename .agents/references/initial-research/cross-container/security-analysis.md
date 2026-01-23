# Security Analysis: Cross-Container Claude Communication

## Executive Summary

This document analyzes security implications of enabling communication between Claude agents running in isolated containers. The primary concern is maintaining file isolation while enabling controlled message passing.

---

## Threat Model

### Assets to Protect
1. **Project Files**: Each container's project files must remain isolated (Container A cannot access Project Y's files)
2. **Credentials/Secrets**: API keys, tokens, SSH keys within each container
3. **Agent Context**: Conversation history, tool permissions, session state
4. **Host System**: The underlying system running the containers

### Threat Actors
1. **Malicious Prompt Injection**: Crafted messages attempting to manipulate receiving agent
2. **Compromised Agent**: An agent executing malicious code from untrusted project
3. **Side-Channel Attacks**: Timing, resource usage inference between containers

### Security Requirements (from spec)
- File isolation: Container A CANNOT access Project Y's files
- Network isolation: Both containers blocked from RFC1918 (private networks)
- Minimal attack surface: Only enable what's necessary for communication

---

## Communication Pattern Analysis

### Pattern 1: Shared Unix Socket

**Description**: A Unix domain socket mounted into both containers at a shared path (e.g., `/var/run/claude-ipc.sock`).

**Mechanism**:
```
Container A  <-->  /var/run/claude-ipc.sock  <-->  Container B
```

**Security Analysis**:

| Aspect | Assessment |
|--------|------------|
| Message Interception | **Low Risk** - Unix sockets are local only, no network exposure. Only processes with socket access can read. |
| Message Modification | **Low Risk** - Stream-based; requires MITM position on same socket. |
| Filesystem Access | **No Risk** - Socket provides data channel only, no filesystem traversal capability. |
| Permissions Required | Socket file permissions (e.g., `srw-rw----`). Both containers need read/write access. |
| Blast Radius | **Medium** - Compromised agent can send arbitrary messages to other agents but cannot access their files. |

**Vulnerabilities**:
- No built-in authentication: Any process with socket access can send messages
- No encryption: Messages readable by host processes with access
- Denial of Service: Malicious agent can flood socket

**Mitigations**:
- Use socket file permissions (mode 0660) with dedicated group
- Implement message signing/verification at application layer
- Rate limiting at daemon level

**Risk Rating**: **LOW** ✅

---

### Pattern 2: Shared Message Directory

**Description**: A shared filesystem directory where agents read/write message files.

**Mechanism**:
```
Container A  -->  /shared/messages/a-to-b.json  -->  Container B
Container B  -->  /shared/messages/b-to-a.json  -->  Container A
```

**Security Analysis**:

| Aspect | Assessment |
|--------|------------|
| Message Interception | **Medium Risk** - Any process with directory access can read all messages. |
| Message Modification | **High Risk** - Race conditions possible; attacker can modify files between write and read. |
| Filesystem Access | **Critical Concern** - Must ensure mount is strictly limited to message directory. Misconfiguration could expose project files. |
| Permissions Required | Directory read/write permissions. More complex ACL needed for multi-agent scenarios. |
| Blast Radius | **Medium-High** - Depends entirely on mount point isolation. |

**Vulnerabilities**:
- **Symlink Attacks**: Malicious agent creates symlink to target agent's project files
- **Race Conditions (TOCTOU)**: File can be modified between existence check and read
- **Path Traversal**: If message content includes file paths, could be exploited
- **Disk Exhaustion**: Agent can fill shared directory

**Mitigations**:
- Mount with `nosuid`, `nodev`, `noexec` options
- Use `O_NOFOLLOW` when opening files (reject symlinks)
- Atomic file operations (write to temp, rename)
- Dedicated message format with content validation
- Quota enforcement on shared directory

**Risk Rating**: **MEDIUM** ⚠️

---

### Pattern 3: Container Network

**Description**: Docker/container network allowing TCP/UDP communication between containers.

**Mechanism**:
```
Container A (172.18.0.2:5000)  <-->  Container Network  <-->  Container B (172.18.0.3:5000)
```

**Security Analysis**:

| Aspect | Assessment |
|--------|------------|
| Message Interception | **Medium Risk** - Other containers on same network can sniff traffic. |
| Message Modification | **Medium Risk** - ARP spoofing possible within container network. |
| Filesystem Access | **No Risk** - Network provides no filesystem access mechanism. |
| Permissions Required | Network namespace configuration. Firewall rules needed. |
| Blast Radius | **High** - Opens network attack surface; conflicts with RFC1918 blocking requirement. |

**Vulnerabilities**:
- **RFC1918 Conflict**: Container networks use private IPs (172.x, 10.x). Enabling inter-container networking may conflict with requirement to block RFC1918.
- **Network Scanning**: Compromised agent can scan for other services
- **Service Exploitation**: If agent has network access, could probe other containers' services
- **No Default Authentication**: TCP connections don't verify identity

**Mitigations**:
- Strict iptables/nftables rules allowing ONLY specific container pairs
- TLS with mutual authentication (mTLS)
- Isolated bridge network per communication pair
- No exposed ports to host

**Risk Rating**: **HIGH** ❌

**Note**: This pattern fundamentally conflicts with the RFC1918 blocking requirement unless implemented with extremely precise firewall rules.

---

### Pattern 4: Host Relay

**Description**: A daemon on the host system relays messages between containers.

**Mechanism**:
```
Container A  -->  Host Daemon (ccc-statusd)  -->  Container B
                        |
              [Message Validation]
              [Access Control]
              [Logging/Audit]
```

**Security Analysis**:

| Aspect | Assessment |
|--------|------------|
| Message Interception | **Low Risk** - Only host daemon has full message visibility. Containers see only their messages. |
| Message Modification | **Very Low Risk** - Daemon is trusted component; no intermediate attackers. |
| Filesystem Access | **No Risk** - Daemon mediates all communication; no direct container-to-container file access. |
| Permissions Required | Container needs access to daemon interface (socket/pipe). Host daemon runs with elevated privileges. |
| Blast Radius | **Low** - Compromised agent can only send messages through daemon. Daemon can enforce policies. |

**Advantages**:
- **Central Policy Enforcement**: Daemon can validate, filter, rate-limit messages
- **Audit Logging**: All inter-agent communication logged in one place
- **Session Isolation**: Daemon tracks sessions; can enforce who-talks-to-whom
- **No Direct Container Interaction**: Containers never directly access each other

**Vulnerabilities**:
- **Daemon Compromise**: If host daemon compromised, all communication exposed
- **Daemon Availability**: Single point of failure
- **Implementation Bugs**: Bugs in daemon could leak data between sessions

**Mitigations**:
- Run daemon with minimal privileges (not root)
- Implement message schema validation
- Session-based access control (agent A can only message agent B if authorized)
- Comprehensive logging and anomaly detection
- Daemon sandboxed (seccomp, AppArmor)

**Risk Rating**: **LOW** ✅

---

## Comparative Risk Matrix

| Pattern | Interception | Modification | File Isolation | Attack Surface | Overall Risk |
|---------|--------------|--------------|----------------|----------------|--------------|
| Unix Socket | Low | Low | Preserved | Small | **LOW** |
| Message Directory | Medium | High | At Risk | Medium | **MEDIUM** |
| Container Network | Medium | Medium | Preserved | Large | **HIGH** |
| Host Relay | Low | Very Low | Preserved | Small | **LOW** |

---

## Recommendation

### Recommended Pattern: **Host Relay** (with Unix Socket as fallback)

**Primary: Host Relay via `ccc-statusd`**

Justification:
1. **Strongest Isolation**: No direct container-to-container access of any kind
2. **Central Control**: Policy enforcement, rate limiting, access control in one place
3. **Auditability**: Complete logging of all cross-agent communication
4. **RFC1918 Compatible**: No network access required between containers
5. **Least Privilege**: Containers need minimal permissions (just daemon interface access)

**Fallback: Shared Unix Socket**

If host relay is unavailable (e.g., daemon not running):
1. Good security posture with proper permissions
2. Simple implementation
3. No filesystem exposure risk

### Patterns to Avoid

1. **Container Network**: Conflicts with RFC1918 requirement; too large attack surface
2. **Shared Message Directory**: Symlink/race condition risks require complex mitigations

---

## Implementation Recommendations

### For Host Relay Pattern

```yaml
# Recommended daemon security configuration
daemon:
  user: claude-ipc          # Non-root user
  socket_path: /var/run/claude-ipc.sock
  socket_mode: 0660
  socket_group: claude-agents

  security:
    max_message_size: 1MB
    rate_limit: 100 msg/min/session
    session_timeout: 24h
    require_session_auth: true

  audit:
    log_all_messages: true
    log_path: /var/log/claude-ipc/
    retention: 30d
```

### Container Security Hardening

```dockerfile
# Container should be read-only with minimal capabilities
RUN chmod -R 555 /app
USER claude
# Drop all capabilities except those needed
```

### Message Security

1. **Schema Validation**: All messages must conform to predefined schema
2. **Content Sanitization**: Strip/escape any filesystem paths in message content
3. **Size Limits**: Prevent memory exhaustion attacks
4. **Sequence Numbers**: Detect message replay/reordering

---

## Conclusion

The Host Relay pattern provides the best security posture for cross-container Claude communication:

- **File isolation**: ✅ Fully preserved (no direct container interaction)
- **Network isolation**: ✅ Compatible with RFC1918 blocking
- **Minimal attack surface**: ✅ Single, auditable communication channel

This pattern aligns with the principle of least privilege and defense in depth, making it the recommended approach for production deployments of multi-agent Claude systems.
