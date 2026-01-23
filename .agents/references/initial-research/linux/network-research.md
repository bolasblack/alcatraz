# Network Isolation Research for Linux Containers

## Executive Summary

This document analyzes network isolation mechanisms for Linux containers (Docker/Podman) with focus on:
- Blocking RFC1918 addresses by default
- Allowing whitelisted IPs/domains
- Ensuring rules are **AI-untouchable** (host-level enforcement)

**Recommendation**: Use **OCI hooks with nftables** for Podman, or **DOCKER-USER chain with iptables** for Docker. Both approaches enforce rules at host level, making them immune to container-internal manipulation.

---

## 1. iptables DOCKER-USER Chain Analysis

### Overview

Docker creates a custom iptables chain called `DOCKER-USER` in the `filter` table. This chain is processed **before** Docker's own chains (`DOCKER-FORWARD`, `DOCKER`), allowing custom firewall rules.

### Implementation

```bash
# Allow established connections first
iptables -I DOCKER-USER -m state --state RELATED,ESTABLISHED -j ACCEPT

# Block RFC1918 addresses
iptables -I DOCKER-USER -i docker0 -d 10.0.0.0/8 -j DROP
iptables -I DOCKER-USER -i docker0 -d 172.16.0.0/12 -j DROP
iptables -I DOCKER-USER -i docker0 -d 192.168.0.0/16 -j DROP

# Whitelist specific hosts (using conntrack for original destination)
iptables -I DOCKER-USER -p tcp -m conntrack --ctorigdst 198.51.100.2 --ctorigdstport 443 -j ACCEPT

# Allow public internet (implicit - not matched by DROP rules above)
```

### Host-Level Enforcement

- Rules are in **host's iptables**, not container's namespace
- Container processes cannot modify host's iptables even with `CAP_NET_ADMIN`
- Docker daemon manages these rules on restart

### Limitations

1. **DNAT Complexity**: Packets in DOCKER-USER have already passed DNAT; matching original IPs requires conntrack
2. **Performance**: conntrack adds overhead
3. **Persistence**: Rules may be reset on Docker daemon restart
4. **UFW Incompatibility**: Docker bypasses UFW rules

### AI Escape Verification

**Can AI modify these rules from inside container?**

| Attack Vector | Result | Reason |
|--------------|--------|--------|
| `docker exec container iptables -F` | **Blocked** | Modifies container namespace only, not host |
| `docker exec container sudo iptables -F DOCKER-USER` | **Blocked** | Container has no access to host namespace |
| Container with `CAP_NET_ADMIN` | **Blocked** | Capability is namespace-scoped |
| Container with `--privileged` | **Vulnerable** | Full host access - never use |
| Container with `--network=host` | **Vulnerable** | Shares host network namespace - never use |

---

## 2. nftables Comparison

### Overview

Docker 29+ supports nftables as experimental backend. Unlike iptables, there's **no DOCKER-USER chain equivalent**. Custom rules go in separate tables.

### Implementation

```nft
table ip container-firewall {
    chain filter-forward {
        type filter hook forward priority filter - 1;  # Run before Docker's rules
        policy accept;

        # Block RFC1918 from containers
        iifname "docker0" ip daddr 10.0.0.0/8 counter drop
        iifname "docker0" ip daddr 172.16.0.0/12 counter drop
        iifname "docker0" ip daddr 192.168.0.0/16 counter drop

        # Whitelist specific hosts
        iifname "docker0" ip daddr 198.51.100.2 tcp dport 443 counter accept

        # Allow established connections
        ct state established,related counter accept
    }
}
```

### Advantages over iptables

| Feature | iptables | nftables |
|---------|----------|----------|
| Rule organization | Fixed chains | Flexible tables |
| Performance | Slower | Faster (single-pass) |
| Syntax | Complex | More readable |
| Atomicity | Rule-by-rule | Atomic ruleset updates |
| Future | Legacy | Modern replacement |

### Host-Level Enforcement

Same security guarantees as iptables:
- Rules in host's nftables, not container
- Container `CAP_NET_ADMIN` only affects container's namespace
- Cannot be modified from inside container

---

## 3. Network Namespaces

### How Isolation Works

Linux network namespaces provide complete isolation of network stack:
- Each container gets its own namespace
- Contains its own interfaces, routing tables, iptables
- Host namespace is separate and protected

### Capability Scoping

**Critical insight**: Since Linux 3.8, capabilities are namespace-scoped:

```
CAP_NET_ADMIN granted to container
    → Only affects container's network namespace
    → Cannot modify host's firewall rules
    → Cannot affect other containers' namespaces
```

### User Namespace Enhancement

When combined with user namespaces:
- Container's "root" maps to unprivileged host UID
- Even `CAP_NET_ADMIN` in container = unprivileged on host
- Provides defense-in-depth

---

## 4. Security Verification: "AI 无法触及"

### Threat Model

AI running inside container attempts to bypass network restrictions via:
1. `podman exec` / `docker exec` with firewall commands
2. Container with elevated capabilities
3. Exploiting container runtime vulnerabilities

### Analysis

#### Scenario 1: AI runs `podman exec + iptables -F`

```bash
# From inside container
podman exec container iptables -F DOCKER-USER
```

**Result**: BLOCKED
- `iptables -F` only affects container's namespace
- `DOCKER-USER` chain doesn't exist in container's namespace
- Host rules remain untouched

#### Scenario 2: Container has CAP_NET_ADMIN

```bash
# Container started with
docker run --cap-add=NET_ADMIN ...
```

**Result**: BLOCKED
- `CAP_NET_ADMIN` is namespace-scoped
- Container can only modify its own network namespace
- Host firewall rules unaffected

#### Scenario 3: AI attempts privilege escalation

**Result**: Depends on configuration
- Default containers: BLOCKED
- `--privileged` containers: VULNERABLE
- `--network=host` containers: VULNERABLE

### Required Configuration for "AI-Untouchable" Rules

```bash
# NEVER use these:
--privileged
--network=host
--cap-add=SYS_ADMIN

# Safe defaults (implicit):
# - Separate network namespace (default)
# - No CAP_SYS_ADMIN (default)
# - User namespace remapping (recommended)
```

---

## 5. Whitelist Mechanism Design

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      Host System                        │
│  ┌───────────────────────────────────────────────────┐  │
│  │            nftables / iptables                    │  │
│  │  ┌─────────────────────────────────────────────┐  │  │
│  │  │  DOCKER-USER / custom nftables table        │  │  │
│  │  │  - Block RFC1918 (default)                  │  │  │
│  │  │  - Allow whitelist (config)                 │  │  │
│  │  │  - Allow public internet                    │  │  │
│  │  └─────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────┘  │
│                           │                             │
│  ┌────────────────────────┼────────────────────────┐    │
│  │     Container Network  │  Namespace              │    │
│  │                        ▼                         │    │
│  │  ┌───────────────────────────────────────────┐   │    │
│  │  │  Container (AI Agent)                     │   │    │
│  │  │  - Cannot modify host rules               │   │    │
│  │  │  - CAP_NET_ADMIN affects only this ns     │   │    │
│  │  └───────────────────────────────────────────┘   │    │
│  └──────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────┘
```

### Configuration File Design

```yaml
# /etc/container-firewall/whitelist.yaml
version: 1
defaults:
  block_rfc1918: true
  allow_public: true

whitelist:
  - host: api.anthropic.com
    ports: [443]
  - host: github.com
    ports: [443]
  - ip: 198.51.100.0/24
    ports: [80, 443]
```

### Implementation Script

```bash
#!/bin/bash
# /usr/local/bin/apply-container-firewall.sh

# Flush existing rules
nft flush table ip container-firewall 2>/dev/null || true

# Create table
nft add table ip container-firewall

# Create chain with priority before Docker
nft add chain ip container-firewall forward \
    '{ type filter hook forward priority filter - 1; policy accept; }'

# Allow established connections
nft add rule ip container-firewall forward \
    ct state established,related accept

# Block RFC1918
for cidr in 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16; do
    nft add rule ip container-firewall forward \
        iifname "docker0" ip daddr $cidr drop
done

# Apply whitelist from config
while read -r host port; do
    # Resolve hostname if needed
    ip=$(dig +short "$host" | head -1)
    nft add rule ip container-firewall forward \
        iifname "docker0" ip daddr "$ip" tcp dport "$port" accept
done < /etc/container-firewall/whitelist.conf
```

---

## 6. OCI Hooks Alternative (Podman)

### Overview

For Podman, OCI hooks provide per-container firewall rules applied at `createContainer` stage:
- Runs **after** namespace creation
- Runs **before** capability drop
- Uses **host binaries** (immune to container tampering)

### Implementation

```json
// /etc/containers/oci/hooks.d/firewall.json
{
    "version": "1.0.0",
    "hook": {
        "path": "/usr/local/bin/container-firewall.sh"
    },
    "when": {
        "annotations": {
            "^io.container.firewall$": "enabled"
        }
    },
    "stages": ["createContainer"]
}
```

```bash
#!/bin/bash
# /usr/local/bin/container-firewall.sh

# Block RFC1918
iptables -P OUTPUT DROP
iptables -A OUTPUT -d 10.0.0.0/8 -j DROP
iptables -A OUTPUT -d 172.16.0.0/12 -j DROP
iptables -A OUTPUT -d 192.168.0.0/16 -j DROP

# Allow whitelist
iptables -A OUTPUT -d api.anthropic.com -j ACCEPT

# Allow public
iptables -A OUTPUT -j ACCEPT
```

### Security Guarantee

Even if container image contains malicious `iptables` binary:
- OCI hook uses **host's** iptables binary
- Rules applied in container namespace
- Container app cannot modify (CAP_NET_ADMIN dropped before execution)

---

## 7. Recommendation

### Primary: nftables with Docker/Podman

**For Docker:**
```bash
# Enable nftables backend
echo '{"firewall-backend": "nftables"}' > /etc/docker/daemon.json

# Apply custom rules
nft -f /etc/container-firewall/rules.nft
```

**For Podman:**
```bash
# Use OCI hooks for per-container rules
# Or systemd-based nftables for global rules
```

### Secondary: iptables DOCKER-USER (if nftables unavailable)

```bash
# Add to startup scripts
iptables -I DOCKER-USER -m state --state RELATED,ESTABLISHED -j ACCEPT
iptables -I DOCKER-USER -i docker0 -d 10.0.0.0/8 -j DROP
# ... etc
```

### Must-Have Configuration

| Setting | Value | Reason |
|---------|-------|--------|
| `--privileged` | Never | Full host access |
| `--network=host` | Never | Shares host namespace |
| `--cap-add=SYS_ADMIN` | Never | Broad attack surface |
| User namespace | Enable | Defense-in-depth |
| Rootless mode | Prefer | Additional isolation |

---

## 8. References

- [Docker iptables documentation](https://docs.docker.com/engine/network/firewall-iptables/)
- [Docker nftables documentation](https://docs.docker.com/engine/network/firewall-nftables)
- [Docker packet filtering](https://docs.docker.com/engine/network/packet-filtering-firewalls/)
- [firewalld strict Docker filtering](https://firewalld.org/2024/04/strictly-filtering-docker-containers)
- [Podman OCI hooks for firewalling](https://jerabaul29.github.io/jekyll/update/2025/10/17/Firewall-a-podman-container.html)
- [Container security fundamentals: namespaces](https://securitylabs.datadoghq.com/articles/container-security-fundamentals-part-2/)
- [Netflix: User namespaces for container security](https://netflixtechblog.com/evolving-container-security-with-linux-user-namespaces-afbe3308c082)
- [Docker capabilities: namespaced vs global](https://wikitwist.com/docker-capabilities-namespaced-vs-global/)
- [Network namespace sandboxing](https://sigma-star.at/blog/2023/05/sandbox-netns/)
- [Container escape via capabilities](https://www.cybereason.com/blog/container-escape-all-you-need-is-cap-capabilities)
