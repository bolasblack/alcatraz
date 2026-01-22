# Cross-Container Communication Patterns

Research findings for Claude agent cross-container communication.

---

## Pattern 1: Shared Unix Socket via Volume Mount

### Description
Mount a shared directory containing a Unix domain socket into multiple containers. One container creates and listens on the socket, others connect to it.

### Configuration Example

```bash
# Docker
docker run -v /shared/sockets:/sockets container-a  # Server creates /sockets/agent.sock
docker run -v /shared/sockets:/sockets container-b  # Client connects to /sockets/agent.sock

# Docker Compose
services:
  server:
    volumes:
      - socket-volume:/sockets
  client:
    volumes:
      - socket-volume:/sockets

volumes:
  socket-volume:

# Podman (same syntax)
podman run -v /shared/sockets:/sockets:z container-a
```

### Pros
- Very low latency (kernel-level IPC, no network stack)
- Simple protocol design (just read/write bytes)
- No port management needed
- Works with existing Unix socket libraries

### Cons
- Both containers must be on same host
- Requires shared volume configuration
- Socket file permissions must be coordinated
- SELinux/AppArmor may block access (needs `:z` or `:Z` suffix)

### File Isolation Implications
- **Shared**: Only the socket directory is shared
- **Risk**: Low - socket files don't expose filesystem contents
- **Mitigation**: Use dedicated directory, set restrictive permissions (0660)

### Network Isolation Implications
- **Isolation**: Preserved - no network ports exposed
- **Risk**: Very low - Unix sockets are local only
- **Benefit**: Cannot be accessed from outside the host

### Security Assessment
- Attack surface: Minimal (single socket file)
- SELinux context: Needs `svirt_sandbox_file_t` label with shared MCS
- Recommended: Use `:z` flag for shared label between containers

### Recommendation Score: 5/5
**Best choice for same-host agent communication.** Lowest latency, strongest isolation, minimal attack surface.

---

## Pattern 2: Shared Directory for File-Based IPC

### Description
Multiple containers mount a shared directory and communicate by reading/writing files. Messages are written as files, read by recipients, then deleted.

### Configuration Example

```bash
# Docker
docker run -v /shared/ipc:/ipc container-a
docker run -v /shared/ipc:/ipc container-b

# Docker Compose
services:
  agent-a:
    volumes:
      - ipc-volume:/ipc
  agent-b:
    volumes:
      - ipc-volume:/ipc

volumes:
  ipc-volume:

# Message passing example (inside container)
# Sender:
echo '{"from":"a","msg":"hello"}' > /ipc/msg-$(date +%s%N).json
# Receiver (polling):
for f in /ipc/msg-*.json; do cat "$f"; rm "$f"; done
```

### Pros
- Very simple to implement
- No special libraries needed
- Works with any language/runtime
- Human-readable messages (debugging)
- Persistent if needed

### Cons
- Higher latency than sockets (filesystem operations)
- Requires polling or inotify for notifications
- File locking complexity for concurrent access
- Disk I/O overhead

### File Isolation Implications
- **Shared**: Entire IPC directory is readable/writable by all containers
- **Risk**: Medium - any container can read all messages
- **Mitigation**: Use encryption, delete messages after reading, use per-agent subdirectories

### Network Isolation Implications
- **Isolation**: Fully preserved - no network involvement
- **Risk**: None from network perspective

### Security Assessment
- Attack surface: Medium (directory with readable files)
- Data exposure: Messages visible to all containers with mount
- Recommended: Use atomic writes, immediate deletion, consider encryption

### Recommendation Score: 4/5
**Good fallback when sockets are too complex.** Simple and reliable, but higher latency and less secure than Unix sockets.

---

## Pattern 3: Container Networking (Internal Network)

### Description
Containers on the same Docker/Podman network communicate via TCP/IP using container names as hostnames.

### Configuration Example

```bash
# Docker - create network and run containers
docker network create agent-net
docker run --network agent-net --name agent-a container-a  # Listens on port 9000
docker run --network agent-net --name agent-b container-b  # Connects to agent-a:9000

# Docker Compose
services:
  agent-a:
    networks:
      - agent-net
    # Container name becomes DNS hostname
  agent-b:
    networks:
      - agent-net
    # Can reach agent-a by name

networks:
  agent-net:
    driver: bridge

# Podman with DNS
podman network create agent-net
podman run --network agent-net --name agent-a container-a
```

### Pros
- Can scale to multiple hosts (with overlay networks)
- Standard TCP/IP - works with any networking library
- Built-in DNS resolution by container name
- Familiar programming model

### Cons
- Higher latency than Unix sockets (full network stack)
- Requires port management
- More complex network configuration
- Rootless Podman limitations (containers can't see each other's IPs)

### File Isolation Implications
- **Shared**: Nothing - no filesystem sharing required
- **Risk**: None from filesystem perspective
- **Benefit**: Complete file isolation maintained

### Network Isolation Implications
- **Isolation**: Partial - internal network only, not exposed to host
- **Risk**: Medium - other containers on same network can connect
- **Mitigation**: Use dedicated networks per agent group, network policies

### Security Assessment
- Attack surface: Network ports within internal network
- Data exposure: Traffic visible to other containers on network
- Recommended: Use TLS for sensitive communications

### Recommendation Score: 3/5
**Best for multi-host deployments.** Standard approach but adds latency and complexity compared to IPC methods.

---

## Pattern 4: Host Relay/Daemon Pattern

### Description
A daemon/proxy runs on the host, containers connect to it via mounted socket or host network. The daemon relays messages between containers.

### Configuration Example

```bash
# Host runs relay daemon listening on /var/run/relay.sock
# (e.g., ccc-statusd, socat relay, custom daemon)

# Docker - mount the relay socket
docker run -v /var/run/relay.sock:/relay.sock container-a
docker run -v /var/run/relay.sock:/relay.sock container-b

# Or use host network (less isolated)
docker run --network host container-a

# Using socat as relay
# On host:
socat UNIX-LISTEN:/var/run/relay.sock,fork TCP-LISTEN:9000,bind=127.0.0.1

# Docker socket proxy pattern (for Docker API access)
docker run -v /var/run/docker.sock:/var/run/docker.sock proxy
```

### Pros
- Centralized message routing/logging
- Can implement access control at relay
- Single point of coordination
- Containers don't need to know about each other
- Relay can persist messages

### Cons
- Additional component to deploy/maintain
- Single point of failure
- Extra hop adds latency
- Host daemon must be trusted

### File Isolation Implications
- **Shared**: Only the relay socket is shared
- **Risk**: Low - controlled by relay daemon
- **Benefit**: Relay can enforce access policies

### Network Isolation Implications
- **Isolation**: Depends on relay configuration
- **Risk**: Relay has access to all messages
- **Mitigation**: Run relay with minimal privileges, audit logging

### Security Assessment
- Attack surface: Relay daemon + socket
- Data exposure: All messages pass through relay
- Recommended: Implement authentication, use restricted socket permissions
- Note: Docker socket proxy (like Tecnativa) can restrict API access by env vars

### Recommendation Score: 4/5
**Best for coordinated multi-agent systems.** Provides central control and logging, matches ccc-statusd pattern well.

---

## Summary Comparison

| Pattern | Latency | Complexity | File Isolation | Network Isolation | Multi-Host | Score |
|---------|---------|------------|----------------|-------------------|------------|-------|
| Unix Socket | Lowest | Low | Good | Excellent | No | 5/5 |
| File IPC | Medium | Lowest | Medium | Excellent | No | 4/5 |
| Container Network | Higher | Medium | Excellent | Medium | Yes | 3/5 |
| Host Relay | Medium | Higher | Good | Good | Possible | 4/5 |

## Recommendations for Claude Agent Communication

1. **For ccc-statusd pattern (current approach)**: Host Relay (Pattern 4) is ideal
   - Centralized session management
   - Logging and audit trail
   - Cross-session message routing
   - Already implemented and working

2. **For direct agent-to-agent (same host)**: Unix Socket (Pattern 1)
   - Lowest latency for real-time coordination
   - Strong isolation
   - Simple implementation

3. **For multi-host deployments**: Container Network (Pattern 3)
   - Scale across machines
   - Use overlay networks with encryption

4. **For simple coordination**: File IPC (Pattern 2)
   - Worktable files are already using this pattern
   - Good for shared state documents
   - Combine with Unix socket for notifications

---

## References

- [Unix Sockets in Docker - Medium](https://medium.com/@moaminsharifi/unix-sockets-in-a-docker-environment-a-comprehensive-guide-6b7588e5c2c4)
- [Docker Socket File for IPC](https://lobster1234.github.io/2019/04/05/docker-socket-file-for-ipc/)
- [Podman Basic Networking](https://github.com/containers/podman/blob/main/docs/tutorials/basic_networking.md)
- [Comunicating among containers - Red Hat](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/8/html/building_running_and_managing_containers/assembly_communicating-among-containers_building-running-and-managing-containers)
- [Docker Socket Proxy - Tecnativa](https://github.com/Tecnativa/docker-socket-proxy)
- [SELinux and Docker - Red Hat](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux_atomic_host/7/html/container_security_guide/docker_selinux_security_policy)
- [Docker Shared Volumes Permissions - Baeldung](https://www.baeldung.com/ops/docker-shared-volumes-permissions)
- [Rootless Podman Communication - Baeldung](https://www.baeldung.com/linux/rootless-podman-communication-containers)
