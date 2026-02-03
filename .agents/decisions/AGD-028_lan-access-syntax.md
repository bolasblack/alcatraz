---
title: LAN Access Configuration Syntax
description: Define granular lan-access rules with IP, port, and protocol support
tags: config, network-isolation, security
updates: AGD-027
---


## Context

AGD-027 established nftables as the primary firewall for Linux, implementing basic RFC1918 blocking when `lan-access = []`. However, users need more granular control:

- Allow specific services (e.g., internal APIs, databases)
- Restrict by port (only allow HTTP/HTTPS)
- Restrict by protocol (TCP vs UDP)

Both nftables (Linux) and pf (macOS) support IP + port + protocol filtering natively.

## Decision

Extend `lan-access` to support a rule-based syntax:

### Syntax Format

```toml
lan-access = [
  "*",                    # Allow all LAN access (no restrictions)
  "192.168.1.100",        # Allow all ports on this IP
  "192.168.1.100:*",      # Same as above (explicit)
  "192.168.1.100:8080",   # Allow TCP (default) to IP:port
  "tcp://192.168.1.100:8080",   # Explicit TCP
  "udp://192.168.1.100:53",     # UDP only
  "*://192.168.1.100:443",      # Any protocol (TCP + UDP)
]
```

### Rule Parsing

| Pattern | IP | Port | Protocol |
|---------|-----|------|----------|
| `*` | all | all | all (disables firewall entirely) |
| `192.168.1.100` | 192.168.1.100 | all | all |
| `192.168.1.100:*` | 192.168.1.100 | all | all |
| `192.168.1.100:8080` | 192.168.1.100 | 8080 | TCP (default) |
| `tcp://192.168.1.100:8080` | 192.168.1.100 | 8080 | TCP |
| `udp://192.168.1.100:53` | 192.168.1.100 | 53 | UDP |
| `*://192.168.1.100:443` | 192.168.1.100 | 443 | TCP + UDP |

### Default Protocol

When no protocol prefix is specified and a port is given, default to **TCP only**. This is the safer choice since most services use TCP.

### CIDR Support

Support CIDR notation for IP ranges:

```toml
lan-access = [
  "192.168.1.0/24:8080",       # Allow port 8080 on entire subnet
  "tcp://10.0.0.0/8:*",        # Allow all TCP to 10.x.x.x
]
```

### Firewall Implementation

**nftables (Linux)**:
```bash
# 192.168.1.100:8080 (TCP default)
ip daddr 192.168.1.100 tcp dport 8080 accept

# udp://192.168.1.100:53
ip daddr 192.168.1.100 udp dport 53 accept

# *://192.168.1.100:443
ip daddr 192.168.1.100 tcp dport 443 accept
ip daddr 192.168.1.100 udp dport 443 accept

# 192.168.1.100 (all ports)
ip daddr 192.168.1.100 accept
```

**pf (macOS)**:
```bash
# 192.168.1.100:8080 (TCP default)
pass out proto tcp to 192.168.1.100 port 8080

# udp://192.168.1.100:53
pass out proto udp to 192.168.1.100 port 53

# *://192.168.1.100:443
pass out proto {tcp udp} to 192.168.1.100 port 443

# 192.168.1.100 (all ports)
pass out to 192.168.1.100
```

### Wildcard Behavior

If `*` appears **anywhere** in the array, the entire firewall is disabled:

```toml
# All three are equivalent - firewall disabled
lan-access = ["*"]
lan-access = ["10.0.0.1", "*"]
lan-access = ["*", "192.168.1.100:8080", "udp://10.0.0.1:53"]
```

Implementation should check for `*` first before processing other rules.

### Rule Order

Rules are processed as allowlist entries. The firewall:
1. Check if any rule is `*` → skip firewall entirely
2. Allows established/related connections (return traffic)
3. Applies each `lan-access` rule as an `accept`
4. Drops all other RFC1918 traffic

### IPv6 Support

Support both IPv4 and IPv6 addresses. IPv6 addresses must be wrapped in brackets when port is specified:

```toml
lan-access = [
  "fe80::1",                      # IPv6, all ports
  "[fe80::1]:8080",               # IPv6 with port
  "tcp://[2001:db8::1]:443",      # IPv6 with protocol and port
  "[2001:db8::/32]:*",            # IPv6 CIDR
]
```

**Firewall mapping**:
- nftables: `ip6 daddr` instead of `ip daddr`
- pf: `inet6` family rules

### Validation

Config parser should validate:
- IP addresses are valid IPv4 or IPv6
- IPv6 with port must use bracket notation `[ip]:port`
- Ports are 1-65535 or `*`
- Protocol is `tcp`, `udp`, or `*`
- CIDR prefix is valid (0-32 for IPv4, 0-128 for IPv6)

Invalid rules should fail config loading with clear error message.

## Consequences

### Positive

- **Granular control**: Users can allow specific services without opening entire LAN
- **Protocol safety**: TCP-only default prevents accidental UDP exposure
- **Cross-platform**: Same syntax works on macOS and Linux
- **Familiar syntax**: URL-like protocol prefix is intuitive

### Negative

- **Complexity**: More parsing logic needed
- **Validation**: Need robust error handling for malformed rules
- **Documentation**: Must clearly explain syntax and defaults

### Examples

#### Example 1: Internal API Server

```toml
lan-access = ["10.10.42.230:8080"]
```

Allow only the internal API server on port 8080.

#### Example 2: Database + DNS

```toml
lan-access = [
  "tcp://192.168.1.50:5432",   # PostgreSQL
  "udp://192.168.1.1:53",      # DNS server
]
```

#### Example 3: Development Subnet

```toml
lan-access = ["192.168.1.0/24"]
```

Allow all traffic to the development subnet.

#### Example 4: Full LAN Access (Legacy Behavior)

```toml
lan-access = ["*"]
```

Disable network isolation entirely.

## Implementation Notes

### Package Consolidation

Move `internal/network/pf_*` files to `internal/firewall/` to consolidate all firewall implementations:

```
internal/firewall/
├── firewall.go       # Interface + factory
├── nftables.go       # Linux nftables
├── nftables_stub.go  # Non-Linux stub
├── pf.go             # macOS pf (moved from network/)
├── pf_stub.go        # Non-macOS stub (moved from network/)
└── firewall_test.go  # Tests
```

## References

- AGD-027: Linux nftables as Primary Solution
- AGD-023: macOS lan-access pf anchor
- nftables wiki: https://wiki.nftables.org/
- pf.conf(5): https://man.openbsd.org/pf.conf
