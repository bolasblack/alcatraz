# Docker Container Network Connectivity Issue Research

## Problem Statement

A Docker container can ping some IPs on the same subnet (e.g., 10.10.42.1 - router) but cannot ping others (e.g., 10.10.42.230 - LAN service), while the host machine can reach both.

## Root Cause Analysis (Verified)

### Environment: macOS + OrbStack

After hands-on testing, the root cause was identified as an **OrbStack networking architecture limitation**.

### Network Architecture

```
Container (192.168.215.2)
    ↓ Docker bridge NAT
OrbStack VM (192.168.139.2)
    ↓ IP forwarding (NO NAT!)
macOS (192.168.139.1 / 10.10.42.232)
    ↓
Physical LAN (10.10.42.0/24)
```

### Verified Behavior

| From | To | Result | Reason |
|------|-----|--------|--------|
| macOS | 10.10.42.230 | ✓ OK | Direct LAN access |
| macOS | 10.10.42.1 | ✓ OK | Direct LAN access |
| OrbStack VM | 10.10.42.1 | ✓ OK | Router handles unknown source IPs |
| OrbStack VM | 10.10.42.230 | ✗ FAIL | Target can't route back to 192.168.139.x |
| Container | 10.10.42.1 | ✓ OK | Same as VM (double NAT) |
| Container | 10.10.42.230 | ✗ FAIL | Same as VM |

### Packet Capture Evidence

```bash
# From OrbStack VM:
19:58:39.395413 IP 192.168.139.2 > 10.10.42.230: ICMP echo request
19:58:39.395747 IP 0.250.250.1 > 192.168.139.2: ICMP host unreachable
```

- Source IP is `192.168.139.2` (OrbStack VM internal IP)
- NOT NAT'd to `10.10.42.232` (macOS physical IP)
- `0.250.250.1` is OrbStack's internal gateway returning "host unreachable"

### Why Gateway (10.10.42.1) Works

The router/gateway can respond because:
1. As the network gateway, it has broader routing knowledge
2. It can route back to any source via the MAC address in the ARP cache
3. macOS appears as the next-hop for 192.168.139.x traffic

### Why LAN Hosts (10.10.42.230) Fail

Regular LAN hosts cannot respond because:
1. They receive packets with source IP `192.168.139.2`
2. Their routing table has no route to `192.168.139.0/24`
3. Default route sends reply to gateway, which may not know where 192.168.139.x is
4. Result: reply never reaches the container

### OrbStack Known Limitation

This is a [documented limitation](https://github.com/orbstack/orbstack/issues/1491):
- `--network host` uses the Linux VM's network, not macOS's actual network
- OrbStack does not NAT VM traffic to macOS's physical IP
- mDNS and direct LAN access are affected

## Solution Comparison (OrbStack-specific)

| Solution | Works on OrbStack? | Complexity | Notes |
|----------|-------------------|------------|-------|
| `--network=host` | ❌ No | Trivial | Uses VM network, not macOS network |
| Macvlan/IPvlan | ❌ No | Medium | OrbStack VM can't bridge to physical NIC |
| pf NAT on macOS | ✅ Yes | Medium | Add NAT rule for 192.168.139.0/24 |
| Static route on target | ✅ Yes | Low | Add route on 10.10.42.230 |
| Docker Desktop | ⚠️ Maybe | Low | Different VM architecture, test needed |

## Recommended Solution: macOS pf NAT

Since OrbStack doesn't NAT VM traffic, configure macOS to do it:

### Option 1: macOS pf NAT (Recommended)

Add NAT rule to translate OrbStack VM traffic to macOS's physical IP:

```bash
# Create pf rules file
cat > /tmp/orbstack-nat.conf << 'EOF'
# NAT OrbStack VM traffic to physical interface
nat on en0 from 192.168.139.0/24 to any -> (en0)
EOF

# Load rules (requires sudo)
sudo pfctl -f /tmp/orbstack-nat.conf -e

# Verify
sudo pfctl -s nat
```

**Pros**: Simple, no changes to container or target
**Cons**: Requires sudo, may need to reapply after reboot

To make persistent, add to `/etc/pf.conf` or create a LaunchDaemon.

### Option 2: Static Route on Target Host

If you control 10.10.42.230, add a route back to OrbStack:

```bash
# On 10.10.42.230 (Linux example)
sudo ip route add 192.168.139.0/24 via 10.10.42.232

# On 10.10.42.230 (macOS example)
sudo route add -net 192.168.139.0/24 10.10.42.232
```

**Pros**: No changes to macOS
**Cons**: Requires access to target host, per-host configuration

### Option 3: Use Docker Desktop Instead

Docker Desktop uses a different VM architecture that may handle LAN access differently. Test if needed:

```bash
# Uninstall OrbStack, install Docker Desktop
# Then test: docker run --rm alpine ping -c 2 10.10.42.230
```

### ~~IPvlan/Macvlan~~ (Not Applicable)

These solutions **do not work on OrbStack** because:
- OrbStack's VM doesn't have direct access to macOS's physical NIC
- `parent=eth0` inside VM refers to VM's virtual interface, not macOS's en0

## Adding Future Network Isolation (with pf NAT)

If using pf NAT solution, you can add firewall rules later:

```bash
# In pf.conf - allow only specific destinations
pass out on en0 from 192.168.139.0/24 to 10.10.42.230
block out on en0 from 192.168.139.0/24 to any
```

Or use OrbStack VM's iptables:
```bash
docker run --rm --privileged --network host alpine sh -c "
  apk add iptables
  iptables -A FORWARD -s 192.168.215.0/24 -d 10.10.42.230 -j ACCEPT
  iptables -A FORWARD -s 192.168.215.0/24 -j DROP
"
```

## Diagnostic Commands (OrbStack-specific)

**Verify OrbStack network architecture**:
```bash
# Check container IP
docker run --rm alpine ip addr | grep inet

# Check OrbStack VM routing
docker run --rm --network host alpine ip route

# Test from OrbStack VM directly
docker run --rm --network host alpine ping -c 2 10.10.42.230

# Capture packets to see source IP
docker run --rm --privileged --network host alpine sh -c "
  apk add tcpdump
  tcpdump -i eth0 -n icmp &
  sleep 0.5
  ping -c 2 10.10.42.230
  sleep 1
"
```

**On macOS**:
```bash
# Check IP forwarding
sysctl net.inet.ip.forwarding

# Check pf NAT rules
sudo pfctl -s nat

# Check routing
netstat -rn | grep 192.168
```

## References

- [Docker Networking Documentation](https://docs.docker.com/engine/network/)
- [Macvlan Network Driver](https://docs.docker.com/engine/network/drivers/macvlan/)
- [IPvlan Network Driver](https://docs.docker.com/engine/network/drivers/ipvlan/)
- [Docker macvlan networks for direct LAN access](https://oneuptime.com/blog/post/2026-01-16-docker-macvlan-networks/view)
- [Docker MacVLAN and IPVLAN Guide](https://medium.com/@dyavanapellisujal7/docker-macvlan-and-ipvlan-explained-advanced-networking-guide-b3ba20bc22e4)
- [Docker Engine 28.0 Networking Changes](https://www.docker.com/blog/docker-engine-28-hardening-container-networking-by-default/)
- [Docker Forum: Container cannot ping local network](https://forums.docker.com/t/container-cannot-ping-local-network-but-can-ping-gateway-and-host/109451)
