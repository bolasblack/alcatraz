# Linux Network Isolation Implementation Guide (nftables)

> This document is the technical implementation reference for the `lan-access=[]` feature, based on the decision in AGD-027.

## Executive Summary

**Technical approach**: Use nftables on Linux for network isolation

**Key features**:
- Atomic operations: Entire ruleset loaded at once, satisfying AGD-008 crash safety requirements
- Idempotency: Repeated execution of the same operation is safe
- Host-level enforcement: Containers cannot bypass (AI-untouchable)

**System requirements**:
- Linux kernel 3.13+ (released in 2014)
- nftables command-line tool (`nft`)

---

## 1. nftables Architecture

### How It Works

nftables executes firewall rules at the host kernel level. Container traffic must pass through these rules:

```
Container → Docker Bridge → Host (nftables) → Physical Network
                                    ↑
                            AI CANNOT REACH
```

### Why nftables

| Feature | nftables | Description |
|---------|----------|-------------|
| **Atomic operations** | ✅ Kernel-level | Transactional ruleset loading |
| **Performance** | ✅ Efficient | Single-pass, O(n) complexity |
| **Syntax** | ✅ Unified | IPv4/IPv6 use same syntax |
| **Maintenance** | ✅ Active | Linux official recommendation, ongoing development |
| **Adoption** | ✅ 60%+ | Mainstream distributions support by default |

### Security Guarantees

- **Host-level execution**: Rules run in host kernel, container processes cannot access
- **Namespace isolation**: Container's `CAP_NET_ADMIN` only affects container's own namespace
- **Requires sudo**: Modifying rules requires host sudo privileges (which containers don't have)

---

## 2. Basic Commands

### Detecting nftables Availability

```bash
# Check if command exists
command -v nft

# Test kernel support
nft list tables

# Check kernel version (requires >= 3.13)
uname -r
```

### Tables and Chains Concepts

```bash
# Create table
nft add table inet alca-container1

# Create chain
# type filter: filter type
# hook forward: forwarded traffic (container traffic)
# priority filter-1: priority (before Docker rules)
nft add chain inet alca-container1 forward \
    '{ type filter hook forward priority filter - 1; policy accept; }'

# List all tables
nft list tables

# Show rules for specific table
nft list table inet alca-container1

# Delete table (deletes all related chains and rules)
nft delete table inet alca-container1
```

---

## 3. Implementation Approach

### Rule File Template

Create an independent rule file for each container:

```bash
CONTAINER_ID="alca-abc123"
CONTAINER_IP="172.17.0.2"

cat > /tmp/alca-${CONTAINER_ID}.nft << EOF
table inet alca-${CONTAINER_ID} {
    chain forward {
        type filter hook forward priority filter - 1; policy accept;

        # Allow established connections (important: avoid breaking existing connections)
        ct state established,related accept

        # Block RFC1918 private IP ranges
        ip saddr ${CONTAINER_IP} ip daddr 10.0.0.0/8 drop
        ip saddr ${CONTAINER_IP} ip daddr 172.16.0.0/12 drop
        ip saddr ${CONTAINER_IP} ip daddr 192.168.0.0/16 drop
        ip saddr ${CONTAINER_IP} ip daddr 169.254.0.0/16 drop
        ip saddr ${CONTAINER_IP} ip daddr 127.0.0.0/8 drop
    }
}
EOF
```

**Explanation**:
- `ip saddr`: Source IP address (container's IP)
- `ip daddr`: Destination IP address (ranges to block)
- `drop`: Drop packets
- `ct state established,related`: Allow return traffic for established connections

### Installing Rules

```bash
# Atomic rule loading (idempotent)
nft -f /tmp/alca-${CONTAINER_ID}.nft

# Verify rules are in effect
nft list table inet alca-${CONTAINER_ID}
```

### Removing Rules

```bash
# Delete entire table (atomic operation, idempotent)
nft delete table inet alca-${CONTAINER_ID} 2>/dev/null || true

# Clean up temporary file
rm -f /tmp/alca-${CONTAINER_ID}.nft
```

---

## 4. Atomicity and Idempotency

### Atomicity Guarantee

The `nft -f` command is a **kernel-level atomic operation**:

```bash
# All rules as a single transaction
nft -f /tmp/rules.nft

# Kernel behavior:
# 1. Parse and validate all rules
# 2. If any error → reject entire file
# 3. If all correct → atomically apply all rules
```

**Crash scenario**:
```bash
# Even if killed with kill -9 during execution
nft -f /tmp/rules.nft  # ← killed here with kill -9

# Result: Either all applied, or none applied
# No partial rule application
```

### Idempotency Implementation

```go
// Go code example
func (f *NFTablesFirewall) BlockRFC1918(containerID, containerIP string) error {
    // Generate ruleset file
    ruleset := f.generateRuleset(containerID, containerIP)
    tmpFile := f.writeTempFile(ruleset)
    defer os.Remove(tmpFile)

    // Atomic load (automatically replaces old rules, idempotent)
    cmd := exec.Command("nft", "-f", tmpFile)
    return cmd.Run()
}

func (f *NFTablesFirewall) RemoveRules(containerID string) error {
    // Delete table (idempotent: no error even if table doesn't exist)
    cmd := exec.Command("nft", "delete", "table", "inet", "alca-"+containerID)
    _ = cmd.Run()  // Ignore error (table might not exist)
    return nil
}
```

---

## 5. Container Lifecycle Integration

### Getting Container IP

```bash
# Using docker inspect
CONTAINER_IP=$(docker inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' alca-abc123)

# Or using more explicit format
CONTAINER_IP=$(docker inspect --format '{{.NetworkSettings.IPAddress}}' alca-abc123)
```

**Go code example**:
```go
func (r *Runtime) getContainerIP(ctx context.Context, containerID string) (string, error) {
    output, err := exec.CommandContext(ctx, "docker", "inspect",
        "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}",
        containerID).Output()
    if err != nil {
        return "", fmt.Errorf("failed to inspect container: %w", err)
    }

    ip := strings.TrimSpace(string(output))
    if ip == "" {
        return "", errors.New("container has no IP address")
    }

    return ip, nil
}
```

### Installing Rules on Startup

```go
func (r *Runtime) Start(ctx context.Context) error {
    // 1. Create and start container
    containerID, err := r.docker.Create(ctx, r.config)
    if err != nil {
        return err
    }

    if err := r.docker.Start(ctx, containerID); err != nil {
        return err
    }

    // 2. Get container IP
    ip, err := r.getContainerIP(ctx, containerID)
    if err != nil {
        return fmt.Errorf("failed to get container IP: %w", err)
    }

    // 3. Apply network isolation (if lan-access=[] configured)
    if len(r.config.LANAccess) == 0 {
        // Check firewall availability
        if r.firewall == nil {
            log.Warnf("⚠️  Network isolation not available (nftables not found)")
            log.Warnf("Container will start WITHOUT network isolation")
            log.Warnf("Install nftables: sudo apt install nftables")
            // Continue starting, but without network isolation
        } else {
            // Apply firewall rules
            if err := r.firewall.BlockRFC1918(containerID, ip); err != nil {
                // Firewall failure is critical error - stop container
                _ = r.docker.Stop(ctx, containerID)
                return fmt.Errorf("failed to apply network isolation: %w", err)
            }
            log.Infof("✓ Network isolation enabled for %s (IP: %s)", containerID[:12], ip)
        }
    }

    return nil
}
```

### Removing Rules on Stop

```go
func (r *Runtime) Stop(ctx context.Context) error {
    // 1. Remove firewall rules (before stopping container)
    if r.firewall != nil {
        if err := r.firewall.RemoveRules(r.containerID); err != nil {
            // Log error but continue stopping container
            log.Warnf("Failed to remove firewall rules: %v", err)
        }
    }

    // 2. Stop container
    if err := r.docker.Stop(ctx, r.containerID); err != nil {
        return err
    }

    return nil
}
```

### Crash Recovery

```go
// Clean up stale rules when Alcatraz starts
func (r *Runtime) Initialize(ctx context.Context) error {
    if r.firewall == nil {
        return nil  // No firewall support, skip
    }

    // 1. Get all alca-* managed firewall rules
    managedTables := r.firewall.ListAlcaTables()

    // 2. Get all running alca containers
    containers, err := r.docker.ListContainers(ctx, "name=alca-")
    if err != nil {
        return err
    }

    runningIDs := make(map[string]bool)
    for _, c := range containers {
        runningIDs[c.ID] = true
    }

    // 3. Remove rules for containers no longer running
    for _, table := range managedTables {
        containerID := strings.TrimPrefix(table, "alca-")
        if !runningIDs[containerID] {
            log.Infof("Cleaning up stale firewall rules for %s", containerID[:12])
            if err := r.firewall.RemoveRules(containerID); err != nil {
                log.Warnf("Failed to remove stale rules: %v", err)
            }
        }
    }

    return nil
}
```

**ListAlcaTables implementation**:
```go
func (f *NFTablesFirewall) ListAlcaTables() []string {
    // List all inet tables
    output, err := exec.Command("nft", "list", "tables", "inet").Output()
    if err != nil {
        return nil
    }

    var tables []string
    scanner := bufio.NewScanner(bytes.NewReader(output))
    for scanner.Scan() {
        line := scanner.Text()
        // Output format: "table inet alca-abc123"
        if strings.Contains(line, "alca-") {
            parts := strings.Fields(line)
            if len(parts) >= 3 {
                tables = append(tables, parts[2])  // "alca-abc123"
            }
        }
    }

    return tables
}
```

---

## 6. Testing Strategy

### Unit Tests

```go
func TestNFTablesBlockRFC1918(t *testing.T) {
    // Requires root privileges
    if os.Geteuid() != 0 {
        t.Skip("Skipping test: requires root")
    }

    fw := NewNFTablesFirewall()
    containerID := "test-container"
    containerIP := "172.17.0.2"

    // Install rules
    err := fw.BlockRFC1918(containerID, containerIP)
    require.NoError(t, err)

    // Verify rules exist
    tables := fw.ListAlcaTables()
    assert.Contains(t, tables, "alca-"+containerID)

    // Test idempotency: install same rules again
    err = fw.BlockRFC1918(containerID, containerIP)
    require.NoError(t, err)

    // Cleanup
    err = fw.RemoveRules(containerID)
    require.NoError(t, err)

    // Verify rules deleted
    tables = fw.ListAlcaTables()
    assert.NotContains(t, tables, "alca-"+containerID)

    // Test idempotency: delete again
    err = fw.RemoveRules(containerID)
    require.NoError(t, err)  // Should not error
}

func TestNFTablesCrashRecovery(t *testing.T) {
    if os.Geteuid() != 0 {
        t.Skip("Skipping test: requires root")
    }

    fw := NewNFTablesFirewall()

    // Create rules for multiple containers
    containers := []struct{ ID, IP string }{
        {"container1", "172.17.0.2"},
        {"container2", "172.17.0.3"},
        {"container3", "172.17.0.4"},
    }

    for _, c := range containers {
        err := fw.BlockRFC1918(c.ID, c.IP)
        require.NoError(t, err)
    }

    // Simulate: container2 stopped, but rules not cleaned up (crash scenario)
    // Execute cleanup logic
    allTables := fw.ListAlcaTables()
    runningContainers := map[string]bool{
        "container1": true,
        "container3": true,
        // container2 missing
    }

    for _, table := range allTables {
        containerID := strings.TrimPrefix(table, "alca-")
        if !runningContainers[containerID] {
            err := fw.RemoveRules(containerID)
            require.NoError(t, err)
        }
    }

    // Verify: only container2's rules removed
    tables := fw.ListAlcaTables()
    assert.Contains(t, tables, "alca-container1")
    assert.NotContains(t, tables, "alca-container2")
    assert.Contains(t, tables, "alca-container3")

    // Cleanup
    fw.RemoveRules("container1")
    fw.RemoveRules("container3")
}
```

### Integration Tests

```bash
#!/bin/bash
# test-network-isolation.sh

set -e

echo "=== Testing Network Isolation ==="

# 1. Start container with lan-access=[]
echo "Starting container with network isolation..."
CONTAINER_ID=$(alca up --lan-access='[]' --detach)

# 2. Verify firewall rules created
echo "Verifying firewall rules..."
if ! nft list table inet alca-${CONTAINER_ID} >/dev/null 2>&1; then
    echo "❌ FAIL: Firewall rules not found"
    exit 1
fi
echo "✓ Firewall rules created"

# 3. Test LAN access blocked
echo "Testing LAN access (should be blocked)..."
if alca exec curl -m 5 http://192.168.1.1 2>&1; then
    echo "❌ FAIL: LAN is accessible (should be blocked)"
    exit 1
fi
echo "✓ LAN access blocked"

# 4. Test internet access works
echo "Testing internet access (should work)..."
if ! alca exec curl -m 5 https://api.anthropic.com 2>&1; then
    echo "❌ FAIL: Internet not accessible"
    exit 1
fi
echo "✓ Internet access works"

# 5. Stop container
echo "Stopping container..."
alca down

# 6. Verify firewall rules cleaned up
echo "Verifying firewall cleanup..."
if nft list table inet alca-${CONTAINER_ID} >/dev/null 2>&1; then
    echo "❌ FAIL: Firewall rules not cleaned up"
    exit 1
fi
echo "✓ Firewall rules cleaned up"

echo ""
echo "=== All tests passed ==="
```

---

## 7. Troubleshooting

### Common Issues

**Issue 1**: `command not found: nft`

```bash
# Solution: Install nftables
# Debian/Ubuntu
sudo apt install nftables

# Fedora/RHEL/CentOS
sudo dnf install nftables

# Arch Linux
sudo pacman -S nftables

# Verify installation
nft --version
```

**Issue 2**: `Error: Could not process rule: No such file or directory`

```bash
# Cause: Kernel doesn't support nftables
# Check kernel version
uname -r

# If < 3.13, need to upgrade kernel
# Or accept network isolation unavailable
```

**Issue 3**: `Permission denied`

```bash
# Cause: Requires root privileges
# Solution: Use sudo

sudo nft list tables
```

**Issue 4**: Rules active but container can still access LAN

```bash
# Check rules loaded correctly
sudo nft list table inet alca-<container-id>

# Check container IP matches
docker inspect <container-id> | grep IPAddress

# Check rule priority
sudo nft list chain inet alca-<container-id> forward

# Should see priority filter - 1 (before Docker)
```

### Debug Commands

```bash
# List all tables
nft list tables

# Show all rules for specific table
nft list table inet alca-<container-id>

# Show rule execution counters (add counter)
nft add rule inet alca-<container-id> forward \
    ip saddr 172.17.0.2 ip daddr 10.0.0.0/8 counter drop

# View counters
nft list table inet alca-<container-id>

# Packet capture verification (inside container)
docker exec <container-id> tcpdump -i eth0 -n
```

---

## 8. Performance Considerations

### Rule Count

Approximately 5-7 rules per container:
- 1 established/related rule
- 5 rules blocking RFC1918 ranges

**Impact**: nftables single-pass, performance impact < 1ms

### Multi-Container Scenarios

- Each container has independent table: `inet alca-{containerID}`
- Tables don't affect each other
- Can run hundreds of containers without performance issues

---

## 9. Security Considerations

### AI Bypass Possibilities

| Attack Vector | Possible | Reason |
|---------------|----------|--------|
| Modify nftables rules | ❌ Impossible | Requires host sudo, container doesn't have |
| Modify with `CAP_NET_ADMIN` | ❌ Impossible | Capability limited to container namespace |
| Using `--privileged` mode | ⚠️ Possible | **Never use** |
| Using `--network=host` | ⚠️ Possible | **Never use** |
| IPv6 bypass | ⚠️ Possible | Currently only blocking IPv4 |

### Limitations

**Current solution blocks**:
- ✅ Direct IP connections to RFC1918
- ✅ TCP/UDP protocols
- ✅ All ports

**Current solution does NOT block**:
- ❌ Access to internal network via external proxy (application layer attack)
- ❌ DNS tunneling
- ❌ IPv6 private addresses (not implemented)

---

## 10. Code Structure

### Recommended File Organization

```
internal/firewall/
├── firewall.go       # Interface definition
├── detect.go         # Platform detection logic
└── nftables.go       # Linux nftables implementation
```

### Interface Definition

```go
// firewall.go
package firewall

type Firewall interface {
    // BlockRFC1918 blocks container access to all RFC1918 private IP ranges
    BlockRFC1918(containerID string, containerIP string) error

    // RemoveRules removes all firewall rules for container (idempotent)
    RemoveRules(containerID string) error

    // ListAlcaTables lists all alca-managed firewall tables
    ListAlcaTables() []string
}

type FirewallType int

const (
    TypeNone FirewallType = iota
    TypeNFTables
)

// New creates firewall instance (nftables only)
func New() (Firewall, FirewallType, error) {
    fwType := Detect()

    switch fwType {
    case TypeNFTables:
        return NewNFTablesFirewall(), TypeNFTables, nil
    default:
        return nil, TypeNone, errors.New("nftables not available")
    }
}
```

---

## 11. References

### Technical Documentation
- [nftables Wiki](https://wiki.nftables.org/)
- [nftables Quick Reference](https://wiki.nftables.org/wiki-nftables/index.php/Quick_reference-nftables_in_10_minutes)
- [Docker with nftables](https://docs.docker.com/engine/network/firewall-nftables)

### Internal Documentation
- AGD-027: Linux Firewall: nftables as Primary Solution
- AGD-008: Firewall Crash Safety
- `linux-firewall-distribution-analysis.md`: Distribution firewall status analysis

### RFC Documentation
- RFC 1918: Address Allocation for Private Internets
- RFC 3927: Dynamic Configuration of IPv4 Link-Local Addresses (169.254.0.0/16)
