# Granular Network Isolation for Claude Code

## Executive Summary

For selectively blocking LAN access (RFC1918 private IP ranges) while allowing internet access (Claude API, package registries), **macOS pf firewall is the ONLY truly secure solution** when the threat model includes a potentially compromised AI agent.

### Critical Security Insight: "AI Untouchable" Requirement

The key security requirement is that network isolation rules must be **completely inaccessible to the AI agent**. This eliminates approaches where rules run inside the VM that the AI can access.

| Approach | Where Rules Run | AI Can Access? | Security Level |
|----------|-----------------|----------------|----------------|
| **macOS pf** | macOS host kernel | ❌ NO | ✅ **PRIMARY** |
| DOCKER-USER iptables | Inside Colima/Docker VM | ⚠️ YES (via `colima ssh`) | ⚠️ Secondary |
| Container iptables | Inside container | ❌ YES (has NET_ADMIN) | ❌ **DO NOT USE** |

### Recommended Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    macOS Host                                │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  macOS pf Firewall (AI CANNOT TOUCH)                │    │
│  │  • Blocks RFC1918 outbound from VM interface        │    │
│  │  • Whitelist: specific IP:port exceptions           │    │
│  └─────────────────────────────────────────────────────┘    │
│                           │                                  │
│  ┌─────────────────────────────────────────────────────┐    │
│  │            Colima / Docker Desktop VM                │    │
│  │  ┌───────────────────────────────────────────────┐  │    │
│  │  │  DOCKER-USER iptables (Secondary Defense)     │  │    │
│  │  │  ⚠️ AI could bypass via: colima ssh + sudo    │  │    │
│  │  └───────────────────────────────────────────────┘  │    │
│  │                         │                            │    │
│  │  ┌─────────────────────────────────────────────┐    │    │
│  │  │         Claude Code Container               │    │    │
│  │  │         (AI runs here)                      │    │    │
│  │  └─────────────────────────────────────────────┘    │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

**Primary Defense**: macOS pf - runs on host, AI in container/VM has zero access
**Secondary Defense**: DOCKER-USER iptables - defense in depth, but AI could potentially bypass

---

## Approach 1: Docker Custom Network + iptables (DOCKER-USER Chain)

> ⚠️ **SECURITY WARNING**: This approach runs inside the Colima/Docker VM. A compromised AI agent could potentially bypass these rules via `colima ssh` followed by `sudo iptables -F`. Use as **secondary defense only**, with macOS pf as primary.

### Implementation

The DOCKER-USER chain is specifically designed for user-defined firewall rules that Docker won't modify.

#### Step 1: Create custom Docker network

```bash
# Create an isolated bridge network
docker network create --driver bridge --subnet 172.28.0.0/16 claude-isolated
```

#### Step 2: Add iptables rules to block RFC1918 outbound

```bash
# Block outbound to private IP ranges from Docker containers
# These rules go in DOCKER-USER chain (processed before Docker's rules)

# Block 10.0.0.0/8 (Class A private)
sudo iptables -I DOCKER-USER -s 172.28.0.0/16 -d 10.0.0.0/8 -j DROP

# Block 172.16.0.0/12 (Class B private) - exclude Docker's own subnet
sudo iptables -I DOCKER-USER -s 172.28.0.0/16 -d 172.16.0.0/12 -j DROP

# Block 192.168.0.0/16 (Class C private)
sudo iptables -I DOCKER-USER -s 172.28.0.0/16 -d 192.168.0.0/16 -j DROP

# Block localhost/loopback from container perspective
sudo iptables -I DOCKER-USER -s 172.28.0.0/16 -d 127.0.0.0/8 -j DROP

# Block link-local addresses
sudo iptables -I DOCKER-USER -s 172.28.0.0/16 -d 169.254.0.0/16 -j DROP

# Allow established connections (for return traffic)
sudo iptables -I DOCKER-USER -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
```

#### Step 3: Run container on isolated network

```bash
docker run --network claude-isolated \
  --cap-drop ALL \
  -v /path/to/project:/workspace \
  claude-code:latest
```

#### Step 4: Make rules persistent

```bash
# On Debian/Ubuntu
sudo apt-get install iptables-persistent
sudo netfilter-persistent save

# On RHEL/CentOS
sudo service iptables save

# Or create a startup script
cat > /etc/docker/iptables-claude.sh << 'EOF'
#!/bin/bash
iptables -I DOCKER-USER -s 172.28.0.0/16 -d 10.0.0.0/8 -j DROP
iptables -I DOCKER-USER -s 172.28.0.0/16 -d 172.16.0.0/12 -j DROP
iptables -I DOCKER-USER -s 172.28.0.0/16 -d 192.168.0.0/16 -j DROP
iptables -I DOCKER-USER -s 172.28.0.0/16 -d 127.0.0.0/8 -j DROP
iptables -I DOCKER-USER -s 172.28.0.0/16 -d 169.254.0.0/16 -j DROP
iptables -I DOCKER-USER -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
EOF
chmod +x /etc/docker/iptables-claude.sh
```

### Security Analysis

| Threat | Mitigation | Residual Risk |
|--------|------------|---------------|
| Container modifies iptables | Container lacks NET_ADMIN capability | **None** - cannot modify host iptables |
| Container escapes network namespace | Would require kernel exploit | **Very Low** - defense in depth needed |
| DNS exfiltration to LAN DNS | Block LAN DNS servers explicitly | **Low** - add rule for DNS servers |
| IPv6 bypass | Add ip6tables rules or disable IPv6 | **Medium** - must also configure IPv6 |

**Key Security Guarantee**: The DOCKER-USER chain is processed on the **host**, outside the container's network namespace. A container process cannot modify these rules without escaping the container entirely.

### rcc CLI Integration

```bash
# rcc command to set up isolation
rcc_setup_network_isolation() {
  local SUBNET="172.28.0.0/16"
  local NETWORK_NAME="claude-isolated"

  # Create network if not exists
  docker network inspect $NETWORK_NAME >/dev/null 2>&1 || \
    docker network create --driver bridge --subnet $SUBNET $NETWORK_NAME

  # Add iptables rules (idempotent with -C check)
  for DEST in "10.0.0.0/8" "172.16.0.0/12" "192.168.0.0/16" "127.0.0.0/8" "169.254.0.0/16"; do
    sudo iptables -C DOCKER-USER -s $SUBNET -d $DEST -j DROP 2>/dev/null || \
      sudo iptables -I DOCKER-USER -s $SUBNET -d $DEST -j DROP
  done

  # Ensure established connections allowed
  sudo iptables -C DOCKER-USER -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || \
    sudo iptables -I DOCKER-USER -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
}
```

### Pros/Cons

| Pros | Cons |
|------|------|
| Rules enforced outside container | Requires sudo/root on host |
| Cannot be bypassed by container | Linux-specific (but runs in Docker VM) |
| Docker officially supports DOCKER-USER | Must handle IPv6 separately |
| Survives container restarts | Rules need persistence setup |
| Fine-grained control possible | Slight complexity vs --network none |

---

## Approach 2: Container-Internal iptables

### Implementation

Run iptables inside the container to block outbound RFC1918.

#### Dockerfile with iptables

```dockerfile
FROM node:20-slim

RUN apt-get update && apt-get install -y iptables

# Create startup script
RUN cat > /usr/local/bin/setup-firewall.sh << 'EOF'
#!/bin/bash
# Block RFC1918 outbound
iptables -A OUTPUT -d 10.0.0.0/8 -j DROP
iptables -A OUTPUT -d 172.16.0.0/12 -j DROP
iptables -A OUTPUT -d 192.168.0.0/16 -j DROP
iptables -A OUTPUT -d 127.0.0.0/8 -j DROP
iptables -A OUTPUT -d 169.254.0.0/16 -j DROP
# Allow established
iptables -I OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
EOF
chmod +x /usr/local/bin/setup-firewall.sh

ENTRYPOINT ["/usr/local/bin/setup-firewall.sh", "&&", "claude"]
```

#### Run with NET_ADMIN capability

```bash
docker run --cap-add NET_ADMIN \
  -v /path/to/project:/workspace \
  claude-code-with-firewall:latest
```

### Security Analysis

| Threat | Mitigation | Residual Risk |
|--------|------------|---------------|
| Container disables own iptables | **NONE** - process has NET_ADMIN | **CRITICAL** - can bypass |
| Malicious agent flushes rules | **NONE** - has capability | **CRITICAL** - trivial bypass |
| Race condition at startup | Rules may not be set before network use | **High** |

**CRITICAL SECURITY FLAW**: Any process with NET_ADMIN can run:
```bash
iptables -F  # Flush all rules - firewall disabled
```

A compromised or malicious Claude Code agent could trivially disable the firewall.

### rcc CLI Integration

```bash
# NOT RECOMMENDED - included only for completeness
rcc_internal_firewall() {
  echo "WARNING: Container-internal firewall can be bypassed by the container itself"
  docker run --cap-add NET_ADMIN ...
}
```

### Pros/Cons

| Pros | Cons |
|------|------|
| Self-contained in container | **CRITICAL: Easily bypassed** |
| No host sudo required | Requires NET_ADMIN capability |
| Portable across hosts | Security theater, not real isolation |
| | Adds attack surface (iptables binary) |

**Recommendation: DO NOT USE** - This approach provides no real security against a malicious agent.

---

## Approach 3: Colima VM-Level Firewall

### Implementation

Configure iptables inside the Colima Linux VM, applying to all containers.

#### Step 1: SSH into Colima VM

```bash
colima ssh
```

#### Step 2: Add iptables rules in VM

```bash
# Inside Colima VM
sudo iptables -I FORWARD -d 10.0.0.0/8 -j DROP
sudo iptables -I FORWARD -d 172.16.0.0/12 -j DROP
sudo iptables -I FORWARD -d 192.168.0.0/16 -j DROP
sudo iptables -I FORWARD -d 169.254.0.0/16 -j DROP

# Allow established connections
sudo iptables -I FORWARD -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

# Allow inter-container traffic on Docker bridge
sudo iptables -I FORWARD -s 172.17.0.0/16 -d 172.17.0.0/16 -j ACCEPT
```

#### Step 3: Persist rules across VM restarts

```bash
# Inside Colima VM
# Create persistence script
sudo tee /etc/init.d/claude-firewall << 'EOF'
#!/bin/sh
### BEGIN INIT INFO
# Provides:          claude-firewall
# Required-Start:    $network
# Default-Start:     2 3 4 5
### END INIT INFO

case "$1" in
  start)
    iptables -I FORWARD -d 10.0.0.0/8 -j DROP
    iptables -I FORWARD -d 172.16.0.0/12 -j DROP
    iptables -I FORWARD -d 192.168.0.0/16 -j DROP
    iptables -I FORWARD -d 169.254.0.0/16 -j DROP
    iptables -I FORWARD -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
    ;;
esac
EOF
sudo chmod +x /etc/init.d/claude-firewall
sudo update-rc.d claude-firewall defaults
```

### Security Analysis

| Threat | Mitigation | Residual Risk |
|--------|------------|---------------|
| Container modifies iptables | Container in separate namespace | **Low** - would need VM escape |
| Colima VM restart loses rules | Persistence script | **Low** - if properly configured |
| Container escapes to VM | Hypervisor isolation | **Very Low** |
| SSH access to VM | Colima controls access | **Low** |

**Security Guarantee**: Rules apply at VM level, containers cannot modify them without escaping to VM.

### rcc CLI Integration

```bash
rcc_colima_firewall_setup() {
  # Check if Colima is running
  if ! colima status 2>/dev/null | grep -q "Running"; then
    echo "Error: Colima is not running"
    return 1
  fi

  # Apply firewall rules via SSH
  colima ssh -- sudo iptables -I FORWARD -d 10.0.0.0/8 -j DROP
  colima ssh -- sudo iptables -I FORWARD -d 172.16.0.0/12 -j DROP
  colima ssh -- sudo iptables -I FORWARD -d 192.168.0.0/16 -j DROP
  colima ssh -- sudo iptables -I FORWARD -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

  echo "Colima VM firewall configured"
}
```

### Pros/Cons

| Pros | Cons |
|------|------|
| Applies to all containers in VM | Colima-specific (not Docker Desktop) |
| Container cannot bypass | Rules lost on `colima delete` |
| Single point of configuration | Requires manual persistence setup |
| Similar to Approach 1 security | Extra abstraction layer |

---

## Approach 4: macOS pf Firewall

### Implementation

Use macOS Packet Filter to control traffic from the Docker VM.

#### Step 1: Identify Docker VM network interface

```bash
# For Docker Desktop
# Traffic flows through com.docker.vpnkit process
# VM uses bridge100 or similar interface

# List network interfaces
ifconfig | grep -E "^[a-z]"

# Find Docker's interface (usually bridge100 for Docker Desktop)
ifconfig bridge100
```

#### Step 2: Create pf anchor file

```bash
sudo tee /etc/pf.anchors/claude-isolation << 'EOF'
# Block RFC1918 outbound from Docker VM interface
# Replace bridge100 with your actual Docker interface

# Define tables for blocked ranges
table <rfc1918> const { 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16 }

# Block outbound to private ranges from Docker interface
block drop out quick on bridge100 from any to <rfc1918>

# Allow everything else
pass out on bridge100 all
EOF
```

#### Step 3: Add anchor to pf.conf

```bash
# Backup original
sudo cp /etc/pf.conf /etc/pf.conf.backup

# Add anchor reference (append before last line)
sudo tee -a /etc/pf.conf << 'EOF'

# Claude Code isolation anchor
anchor "claude-isolation"
load anchor "claude-isolation" from "/etc/pf.anchors/claude-isolation"
EOF
```

#### Step 4: Load and enable pf

```bash
# Load the new configuration
sudo pfctl -f /etc/pf.conf

# Enable pf if not already enabled
sudo pfctl -e

# Verify rules
sudo pfctl -sr
sudo pfctl -a claude-isolation -sr
```

#### Step 5: Create LaunchDaemon for persistence

```bash
sudo tee /Library/LaunchDaemons/com.claude.pf-anchor.plist << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.claude.pf-anchor</string>
    <key>ProgramArguments</key>
    <array>
        <string>/sbin/pfctl</string>
        <string>-a</string>
        <string>claude-isolation</string>
        <string>-f</string>
        <string>/etc/pf.anchors/claude-isolation</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
EOF

sudo launchctl load /Library/LaunchDaemons/com.claude.pf-anchor.plist
```

### Security Analysis

| Threat | Mitigation | Residual Risk |
|--------|------------|---------------|
| Container modifies pf | pf runs on macOS host, outside VM | **None** |
| Docker process bypasses pf | All VM traffic goes through interface | **Very Low** |
| macOS upgrade overwrites pf.conf | Using anchor (separate file) | **Low** |
| Incorrect interface identification | Must verify Docker's interface | **Medium** |

**Security Guarantee**: pf operates at macOS kernel level, completely outside Docker's control.

### rcc CLI Integration

```bash
rcc_pf_firewall_setup() {
  local ANCHOR_FILE="/etc/pf.anchors/claude-isolation"
  local DOCKER_IF="bridge100"  # May need detection

  # Detect Docker interface
  if ifconfig bridge100 >/dev/null 2>&1; then
    DOCKER_IF="bridge100"
  elif ifconfig vmenet0 >/dev/null 2>&1; then
    DOCKER_IF="vmenet0"
  else
    echo "Warning: Could not detect Docker interface"
  fi

  # Create anchor file
  sudo tee $ANCHOR_FILE << EOF
table <rfc1918> const { 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16 }
block drop out quick on $DOCKER_IF from any to <rfc1918>
pass out on $DOCKER_IF all
EOF

  # Load anchor
  sudo pfctl -a claude-isolation -f $ANCHOR_FILE
  sudo pfctl -e 2>/dev/null || true

  echo "macOS pf firewall configured for Docker interface $DOCKER_IF"
}
```

### Pros/Cons

| Pros | Cons |
|------|------|
| Host-level, outside Docker entirely | macOS-specific |
| Survives Docker restarts | Requires correct interface identification |
| Cannot be bypassed from container | macOS updates may affect pf.conf |
| Defense in depth with Approach 1 | No GUI, CLI-only configuration |
| Works with Docker Desktop or Colima | Some expertise required |

---

## Comparison Matrix

| Criterion | macOS pf | DOCKER-USER iptables | Colima VM iptables | Container iptables |
|-----------|----------|----------------------|--------------------|--------------------|
| **AI Can Access Rules?** | ❌ NO | ⚠️ YES (colima ssh) | ⚠️ YES (colima ssh) | ❌ YES (NET_ADMIN) |
| **Security Level** | ✅ **HIGHEST** | ⚠️ Medium | ⚠️ Medium | ❌ **CRITICAL FLAW** |
| **Bypass Difficulty** | Requires macOS sudo | `colima ssh` + sudo | `colima ssh` + sudo | `iptables -F` |
| **Setup Complexity** | Medium | Low | Medium | Low |
| **Persistence** | LaunchDaemon | Needs setup | Needs setup | N/A |
| **Docker Desktop** | Yes | Yes (in VM) | No | Yes |
| **Colima** | Yes | Yes | Yes | Yes |
| **CLI Integration** | Medium | Easy | Medium | N/A |
| **Whitelist Support** | Yes (tables) | Yes | Yes | N/A |

### Security Ranking (AI Threat Model)

1. **✅ PRIMARY**: macOS pf - AI in container/VM cannot touch host kernel
2. **⚠️ SECONDARY**: DOCKER-USER iptables - Defense in depth, but AI could bypass via `colima ssh`
3. **⚠️ TERTIARY**: Colima VM iptables - Same limitation as DOCKER-USER
4. **❌ DO NOT USE**: Container iptables - AI has direct control

### Why macOS pf is the Only True Defense

```
AI Attack Vector Analysis:

1. Container iptables:
   AI runs in container → Has NET_ADMIN → Can run: iptables -F
   Result: ❌ BYPASSED TRIVIALLY

2. DOCKER-USER / Colima VM iptables:
   AI runs in container → Can execute: colima ssh
   → Inside VM: sudo iptables -F DOCKER-USER
   Result: ⚠️ BYPASSABLE (requires colima ssh access)

3. macOS pf:
   AI runs in container → Cannot escape to macOS host
   → Even if AI could run host commands, needs: sudo pfctl
   → User's macOS password required
   Result: ✅ AI CANNOT BYPASS
```

---

## Recommended Solution

### ✅ PRIMARY: macOS pf Firewall (AI Cannot Touch)

macOS pf is the **only** solution where AI in the container/VM has zero access to the firewall rules.

#### Complete pf Setup with Whitelist Support

```bash
#!/bin/bash
# rcc-pf-isolation.sh
# PRIMARY network isolation using macOS pf firewall
# AI in container/VM CANNOT modify these rules

set -e

# Configuration
ANCHOR_NAME="claude-isolation"
ANCHOR_FILE="/etc/pf.anchors/$ANCHOR_NAME"
WHITELIST_FILE="$HOME/.rcc/network-whitelist.conf"

# Detect Docker/Colima VM network interface
detect_docker_interface() {
  # Docker Desktop uses bridge100 or similar
  # Colima uses col0 or similar
  for IF in bridge100 bridge101 col0 vmenet0; do
    if ifconfig $IF 2>/dev/null | grep -q "inet "; then
      echo $IF
      return
    fi
  done
  echo "bridge100"  # Default fallback
}

DOCKER_IF=$(detect_docker_interface)

echo "=== macOS pf Network Isolation Setup ==="
echo "Interface: $DOCKER_IF"
echo "Anchor: $ANCHOR_NAME"
echo ""

# Create whitelist file if not exists
if [ ! -f "$WHITELIST_FILE" ]; then
  mkdir -p "$(dirname "$WHITELIST_FILE")"
  cat > "$WHITELIST_FILE" << 'WHITELIST'
# rcc Network Whitelist Configuration
# Format: IP PORT PROTOCOL
# Example: 192.168.1.100 5432 tcp
#
# Special: Use "host.docker.internal" which resolves to:
#   - Docker Desktop: 192.168.65.254
#   - Colima: VM's gateway IP

# Uncomment to allow host PostgreSQL:
# host.docker.internal 5432 tcp

# Uncomment to allow host Redis:
# host.docker.internal 6379 tcp
WHITELIST
  echo "Created whitelist template: $WHITELIST_FILE"
fi

# Resolve host.docker.internal to actual IP
resolve_host_ip() {
  case "$1" in
    host.docker.internal)
      # Docker Desktop macOS
      if [ -n "$(ifconfig bridge100 2>/dev/null)" ]; then
        echo "192.168.65.254"
      else
        # Colima - get gateway
        colima ssh -- ip route | grep default | awk '{print $3}' 2>/dev/null || echo "192.168.65.254"
      fi
      ;;
    *)
      echo "$1"
      ;;
  esac
}

# Build whitelist rules
build_whitelist_rules() {
  local rules=""
  if [ -f "$WHITELIST_FILE" ]; then
    while read -r line; do
      # Skip comments and empty lines
      [[ "$line" =~ ^#.*$ ]] && continue
      [[ -z "$line" ]] && continue

      read -r ip port protocol <<< "$line"
      resolved_ip=$(resolve_host_ip "$ip")
      rules+="pass out quick on $DOCKER_IF proto $protocol from any to $resolved_ip port $port\n"
      echo "  Whitelist: $ip:$port ($protocol) -> $resolved_ip"
    done < "$WHITELIST_FILE"
  fi
  echo -e "$rules"
}

echo "[1/4] Building whitelist rules..."
WHITELIST_RULES=$(build_whitelist_rules)

echo "[2/4] Creating pf anchor file..."
sudo tee "$ANCHOR_FILE" > /dev/null << EOF
# Claude Code Network Isolation - macOS pf
# Generated by rcc - DO NOT EDIT MANUALLY
# AI in container/VM CANNOT modify these rules
#
# Interface: $DOCKER_IF
# Whitelist: $WHITELIST_FILE

# Table of blocked RFC1918 private IP ranges
table <rfc1918> const { \\
  10.0.0.0/8, \\
  172.16.0.0/12, \\
  192.168.0.0/16, \\
  127.0.0.0/8, \\
  169.254.0.0/16, \\
  100.64.0.0/10 \\
}

# WHITELIST RULES (processed first - pass quick)
$WHITELIST_RULES

# BLOCK all other traffic to RFC1918
block drop out quick on $DOCKER_IF from any to <rfc1918>

# ALLOW all other outbound (internet access)
pass out on $DOCKER_IF all
EOF

echo "[3/4] Loading pf anchor..."
sudo pfctl -a "$ANCHOR_NAME" -f "$ANCHOR_FILE"

# Enable pf if not already enabled
sudo pfctl -e 2>/dev/null || true

echo "[4/4] Verifying rules..."
echo ""
echo "=== Active pf Rules for $ANCHOR_NAME ==="
sudo pfctl -a "$ANCHOR_NAME" -sr
echo ""
echo "=== Security Guarantee ==="
echo "✅ These rules run on macOS host kernel"
echo "✅ AI in container/VM CANNOT access or modify them"
echo "✅ Modification requires: sudo + macOS user password"
echo ""
echo "To add whitelist entries, edit: $WHITELIST_FILE"
echo "Then re-run this script to apply changes."
```

#### Whitelist Configuration File

**Location**: `~/.rcc/network-whitelist.conf`

```conf
# rcc Network Whitelist Configuration
# Format: IP PORT PROTOCOL
#
# Each line creates a pf "pass" rule that allows traffic
# to the specified IP:port BEFORE the RFC1918 block rule.

# Host services (via Docker's magic hostname)
host.docker.internal 5432 tcp    # PostgreSQL
host.docker.internal 6379 tcp    # Redis
host.docker.internal 9000 tcp    # MinIO

# Specific LAN services
192.168.1.100 8080 tcp           # Internal API server
192.168.1.10 443 tcp             # Internal GitLab
192.168.1.10 22 tcp              # GitLab SSH
```

#### rcc CLI Integration

```bash
#!/bin/bash
# rcc network commands

case "$1" in
  init)
    # Initialize pf isolation
    ./rcc-pf-isolation.sh
    ;;

  whitelist-add)
    # Add whitelist entry
    # Usage: rcc network whitelist-add <ip> <port> <protocol>
    echo "$2 $3 ${4:-tcp}" >> "$HOME/.rcc/network-whitelist.conf"
    echo "Added: $2:$3 (${4:-tcp})"
    echo "Run 'rcc network init' to apply"
    ;;

  whitelist-remove)
    # Remove whitelist entry
    # Usage: rcc network whitelist-remove <ip> <port>
    sed -i '' "/$2 $3/d" "$HOME/.rcc/network-whitelist.conf"
    echo "Removed entries matching: $2:$3"
    echo "Run 'rcc network init' to apply"
    ;;

  whitelist-list)
    # List current whitelist
    grep -v '^#' "$HOME/.rcc/network-whitelist.conf" | grep -v '^$'
    ;;

  status)
    # Show current pf rules
    sudo pfctl -a claude-isolation -sr 2>/dev/null || echo "No rules loaded"
    ;;

  disable)
    # Disable isolation (for debugging)
    sudo pfctl -a claude-isolation -F all
    echo "pf isolation disabled"
    ;;
esac
```

### ⚠️ SECONDARY: DOCKER-USER iptables (Defense in Depth)

As a secondary layer, add DOCKER-USER rules. Note: AI could potentially bypass via `colima ssh`.

```bash
#!/bin/bash
# rcc-iptables-secondary.sh
# SECONDARY defense - adds iptables rules inside VM
# WARNING: AI could bypass via: colima ssh -> sudo iptables -F

SUBNET="172.28.0.0/16"

echo "=== Secondary Defense: DOCKER-USER iptables ==="
echo "⚠️  WARNING: AI could bypass via 'colima ssh' + sudo"
echo ""

# Create Docker network
docker network inspect claude-isolated >/dev/null 2>&1 || \
  docker network create --driver bridge --subnet $SUBNET claude-isolated

# Add iptables rules (these run INSIDE the VM)
for RANGE in "10.0.0.0/8" "172.16.0.0/12" "192.168.0.0/16" "127.0.0.0/8"; do
  sudo iptables -C DOCKER-USER -s $SUBNET -d $RANGE -j DROP 2>/dev/null || \
    sudo iptables -I DOCKER-USER -s $SUBNET -d $RANGE -j DROP
done

sudo iptables -C DOCKER-USER -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || \
  sudo iptables -I DOCKER-USER -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

echo "DOCKER-USER rules added (secondary defense)"
```

### Usage Example

```bash
# 1. Set up PRIMARY defense (macOS pf) - REQUIRED
./rcc-pf-isolation.sh

# 2. Optionally add SECONDARY defense (iptables) - Recommended
./rcc-iptables-secondary.sh

# 3. Run Claude Code with isolation
docker run -it --rm \
  --network claude-isolated \
  --cap-drop ALL \
  -v "$(pwd):/workspace" \
  -e ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" \
  claude-code:latest

# 4. Verify isolation (from inside container)
# These should fail:
#   curl http://192.168.1.1      # LAN router - BLOCKED by pf
#   curl http://10.0.0.1         # Private network - BLOCKED by pf
# These should work:
#   curl https://api.anthropic.com    # Claude API - allowed
#   curl https://registry.npmjs.org   # npm - allowed
# Whitelisted should work:
#   psql -h host.docker.internal -p 5432  # If whitelisted
```

### Security Guarantees

| Defense Layer | AI Bypass Method | Prevention |
|---------------|------------------|------------|
| **macOS pf** | None from container/VM | ✅ Runs on host kernel |
| DOCKER-USER | `colima ssh` + sudo | ⚠️ Could restrict colima access |
| Container iptables | `iptables -F` | ❌ Never use |

**Bottom Line**: macOS pf is the only layer that provides true security against a compromised AI agent. DOCKER-USER adds defense in depth but should not be relied upon as the sole protection.

---

## Whitelist Configuration for Selective Access

While blocking all RFC1918 traffic provides maximum isolation, some use cases require selective access to specific LAN services. This section describes how to implement a whitelist mechanism that allows specific IP:port combinations while maintaining the default block policy.

### Implementation: iptables Rule Ordering

The key principle: **ACCEPT rules must come BEFORE DROP rules** in iptables. Since we use `-I` (insert) to add rules, they're added at the top of the chain. Therefore, we must add DROP rules first, then ACCEPT rules (so ACCEPT ends up above DROP).

#### Rule Processing Order

```
DOCKER-USER chain processing (top to bottom):
1. ACCEPT established/related connections  ← First (always needed)
2. ACCEPT whitelist entry: 192.168.1.100:5432  ← Whitelist exceptions
3. ACCEPT whitelist entry: host.docker.internal:8080
4. DROP 10.0.0.0/8        ← Block rules (added first, end up at bottom)
5. DROP 172.16.0.0/12
6. DROP 192.168.0.0/16
7. DROP 127.0.0.0/8
8. RETURN (implicit)      ← Continue to next chain
```

#### Basic Whitelist Implementation

```bash
#!/bin/bash
# Setup order matters: DROP rules first (end up at bottom), then ACCEPT (end up at top)

SUBNET="172.28.0.0/16"

# Step 1: Add DROP rules (will be at bottom after all inserts)
sudo iptables -I DOCKER-USER -s $SUBNET -d 10.0.0.0/8 -j DROP
sudo iptables -I DOCKER-USER -s $SUBNET -d 172.16.0.0/12 -j DROP
sudo iptables -I DOCKER-USER -s $SUBNET -d 192.168.0.0/16 -j DROP
sudo iptables -I DOCKER-USER -s $SUBNET -d 127.0.0.0/8 -j DROP
sudo iptables -I DOCKER-USER -s $SUBNET -d 169.254.0.0/16 -j DROP

# Step 2: Add whitelist ACCEPT rules (will be above DROP rules)
# Example: Allow PostgreSQL on host (via host.docker.internal -> 192.168.65.254 on Docker Desktop)
sudo iptables -I DOCKER-USER -s $SUBNET -d 192.168.65.254 -p tcp --dport 5432 -j ACCEPT

# Example: Allow specific LAN service
sudo iptables -I DOCKER-USER -s $SUBNET -d 192.168.1.100 -p tcp --dport 8080 -j ACCEPT

# Step 3: Allow established connections (MUST be at very top)
sudo iptables -I DOCKER-USER -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
```

### Config File Format

For rcc CLI integration, use YAML for the whitelist configuration:

#### Schema: `~/.rcc/network.yaml`

```yaml
# rcc Network Isolation Configuration
# Version: 1.0

network:
  # Docker network settings
  name: claude-isolated
  subnet: 172.28.0.0/16

  # Default policy: block all RFC1918 (private) IP ranges
  block_rfc1918: true

  # Whitelist: specific IP:port combinations allowed despite RFC1918 block
  # Each entry creates an iptables ACCEPT rule BEFORE the DROP rules
  whitelist:
    # Format: "description": { ip: "x.x.x.x", port: N, protocol: "tcp|udp" }

    # Host services via Docker's magic DNS
    - name: "host-postgres"
      ip: "host.docker.internal"  # Resolved at runtime
      port: 5432
      protocol: tcp

    - name: "host-redis"
      ip: "host.docker.internal"
      port: 6379
      protocol: tcp

    # Specific LAN services
    - name: "internal-api"
      ip: "192.168.1.100"
      port: 8080
      protocol: tcp

    - name: "local-minio"
      ip: "192.168.1.50"
      port: 9000
      protocol: tcp

  # Advanced: Additional blocked ranges (beyond RFC1918)
  additional_blocked:
    - "100.64.0.0/10"   # Carrier-grade NAT
    - "198.18.0.0/15"   # Benchmarking

  # IPv6 settings
  ipv6:
    enabled: false      # Block all IPv6 by default
    whitelist: []       # IPv6 whitelist entries (same format)
```

#### JSON Alternative: `~/.rcc/network.json`

```json
{
  "network": {
    "name": "claude-isolated",
    "subnet": "172.28.0.0/16",
    "block_rfc1918": true,
    "whitelist": [
      {
        "name": "host-postgres",
        "ip": "host.docker.internal",
        "port": 5432,
        "protocol": "tcp"
      },
      {
        "name": "internal-api",
        "ip": "192.168.1.100",
        "port": 8080,
        "protocol": "tcp"
      }
    ]
  }
}
```

### Example Configurations

#### Example 1: Allow Host PostgreSQL

**Use case**: Claude Code agent needs to access PostgreSQL running on the macOS host.

**Config** (`~/.rcc/network.yaml`):
```yaml
network:
  whitelist:
    - name: "host-postgres"
      ip: "host.docker.internal"
      port: 5432
      protocol: tcp
```

**Generated iptables rules**:
```bash
# On Docker Desktop, host.docker.internal resolves to 192.168.65.254
# Detect actual IP at runtime:
HOST_IP=$(getent hosts host.docker.internal | awk '{print $1}')

# Or for Docker Desktop macOS, it's typically:
HOST_IP="192.168.65.254"

# Add whitelist rule (inserted BEFORE drop rules)
sudo iptables -I DOCKER-USER -s 172.28.0.0/16 -d $HOST_IP -p tcp --dport 5432 -j ACCEPT
```

**Docker run command**:
```bash
docker run --network claude-isolated \
  --add-host=host.docker.internal:host-gateway \
  -e DATABASE_URL="postgresql://user:pass@host.docker.internal:5432/db" \
  claude-code:latest
```

#### Example 2: Allow Specific LAN Development Server

**Use case**: Access an internal API server at 192.168.1.100:8080 for testing.

**Config**:
```yaml
network:
  whitelist:
    - name: "dev-api-server"
      ip: "192.168.1.100"
      port: 8080
      protocol: tcp
```

**Generated iptables rules**:
```bash
sudo iptables -I DOCKER-USER -s 172.28.0.0/16 -d 192.168.1.100 -p tcp --dport 8080 -j ACCEPT
```

#### Example 3: Multiple Services (Full Development Stack)

**Use case**: Local development with PostgreSQL, Redis, and MinIO on host, plus internal GitLab.

**Config**:
```yaml
network:
  whitelist:
    # Host services
    - name: "postgres"
      ip: "host.docker.internal"
      port: 5432
      protocol: tcp

    - name: "redis"
      ip: "host.docker.internal"
      port: 6379
      protocol: tcp

    - name: "minio"
      ip: "host.docker.internal"
      port: 9000
      protocol: tcp

    # LAN services
    - name: "gitlab"
      ip: "192.168.1.10"
      port: 443
      protocol: tcp

    - name: "gitlab-ssh"
      ip: "192.168.1.10"
      port: 22
      protocol: tcp
```

**Generated script**:
```bash
#!/bin/bash
SUBNET="172.28.0.0/16"
HOST_IP="192.168.65.254"  # Docker Desktop host.docker.internal

# Step 1: DROP rules (end up at bottom)
for RANGE in "10.0.0.0/8" "172.16.0.0/12" "192.168.0.0/16" "127.0.0.0/8"; do
  sudo iptables -I DOCKER-USER -s $SUBNET -d $RANGE -j DROP
done

# Step 2: Whitelist ACCEPT rules (end up above DROP)
sudo iptables -I DOCKER-USER -s $SUBNET -d $HOST_IP -p tcp --dport 5432 -j ACCEPT
sudo iptables -I DOCKER-USER -s $SUBNET -d $HOST_IP -p tcp --dport 6379 -j ACCEPT
sudo iptables -I DOCKER-USER -s $SUBNET -d $HOST_IP -p tcp --dport 9000 -j ACCEPT
sudo iptables -I DOCKER-USER -s $SUBNET -d 192.168.1.10 -p tcp --dport 443 -j ACCEPT
sudo iptables -I DOCKER-USER -s $SUBNET -d 192.168.1.10 -p tcp --dport 22 -j ACCEPT

# Step 3: Established connections (at top)
sudo iptables -I DOCKER-USER -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
```

### Security Validation

The rcc CLI must validate whitelist entries to prevent overly permissive or dangerous configurations.

#### Validation Rules

```python
# Python pseudocode for validation logic

import ipaddress
import re

class WhitelistValidator:
    # Patterns that are TOO permissive (never allow)
    FORBIDDEN_PATTERNS = [
        "0.0.0.0/0",        # All IPv4
        "::/0",             # All IPv6
        "10.0.0.0/8",       # Entire Class A private
        "172.16.0.0/12",    # Entire Class B private
        "192.168.0.0/16",   # Entire Class C private
    ]

    # Maximum CIDR prefix length (smaller = more IPs)
    MIN_PREFIX_LENGTH = 24  # /24 = 256 IPs max

    # Dangerous ports that require explicit confirmation
    SENSITIVE_PORTS = [22, 23, 3389, 5900]  # SSH, Telnet, RDP, VNC

    def validate_entry(self, entry: dict) -> tuple[bool, str]:
        ip = entry.get('ip', '')
        port = entry.get('port')
        protocol = entry.get('protocol', 'tcp')

        # Rule 1: No wildcards or CIDR ranges in whitelist
        if '/' in ip and ip != 'host.docker.internal':
            prefix = int(ip.split('/')[1])
            if prefix < self.MIN_PREFIX_LENGTH:
                return False, f"CIDR /{prefix} too broad. Maximum allowed: /{self.MIN_PREFIX_LENGTH}"

        # Rule 2: Check forbidden patterns
        if ip in self.FORBIDDEN_PATTERNS:
            return False, f"Forbidden pattern: {ip} would bypass isolation"

        # Rule 3: Validate IP address format
        if ip != 'host.docker.internal':
            try:
                ipaddress.ip_address(ip.split('/')[0])
            except ValueError:
                return False, f"Invalid IP address: {ip}"

        # Rule 4: Port must be specified (no wildcard ports)
        if port is None or port == '*' or port == 0:
            return False, "Port must be specified (no wildcards)"

        # Rule 5: Port range validation
        if not (1 <= int(port) <= 65535):
            return False, f"Invalid port: {port}"

        # Rule 6: Warn about sensitive ports
        if int(port) in self.SENSITIVE_PORTS:
            return True, f"WARNING: Port {port} is sensitive (SSH/RDP/VNC)"

        # Rule 7: Protocol validation
        if protocol not in ['tcp', 'udp']:
            return False, f"Invalid protocol: {protocol}"

        return True, "OK"

    def validate_config(self, config: dict) -> list[str]:
        errors = []
        whitelist = config.get('network', {}).get('whitelist', [])

        for entry in whitelist:
            valid, msg = self.validate_entry(entry)
            if not valid:
                errors.append(f"{entry.get('name', 'unknown')}: {msg}")

        return errors
```

#### Forbidden Configurations (Rejected by Validator)

```yaml
# REJECTED: Entire subnet whitelisted
whitelist:
  - name: "bad-entire-lan"
    ip: "192.168.0.0/16"    # ERROR: Would allow entire LAN
    port: 8080

# REJECTED: No port specified
whitelist:
  - name: "bad-no-port"
    ip: "192.168.1.100"
    # port: missing         # ERROR: Port required

# REJECTED: Wildcard port
whitelist:
  - name: "bad-wildcard-port"
    ip: "192.168.1.100"
    port: "*"               # ERROR: No wildcard ports

# WARNING: Sensitive port (allowed but warns)
whitelist:
  - name: "ssh-access"
    ip: "192.168.1.10"
    port: 22                # WARNING: SSH port
    protocol: tcp
```

### Security Tradeoffs

| Whitelist Entry | Security Impact | Recommendation |
|-----------------|-----------------|----------------|
| Single IP + single port | **Minimal** | Preferred approach |
| Single IP + multiple ports | **Low** | OK for known services |
| /24 subnet + single port | **Medium** | Use only if necessary |
| host.docker.internal + port | **Low** | Safe for host services |
| Any + port 22/3389 | **High** | Avoid unless required |

### rcc CLI Integration

#### Command: `rcc network init`

```bash
#!/bin/bash
# rcc network init - Initialize network isolation with whitelist support

set -e

CONFIG_FILE="${RCC_CONFIG:-$HOME/.rcc/network.yaml}"
SUBNET="172.28.0.0/16"
NETWORK_NAME="claude-isolated"

# Parse YAML config (using yq or python)
parse_whitelist() {
  if command -v yq &> /dev/null; then
    yq eval '.network.whitelist[]' "$CONFIG_FILE" 2>/dev/null
  else
    python3 -c "
import yaml
import sys
with open('$CONFIG_FILE') as f:
    config = yaml.safe_load(f)
    for entry in config.get('network', {}).get('whitelist', []):
        print(f\"{entry['ip']}:{entry['port']}:{entry.get('protocol', 'tcp')}\")
"
  fi
}

# Resolve host.docker.internal to actual IP
resolve_host_internal() {
  local ip="$1"
  if [ "$ip" = "host.docker.internal" ]; then
    # Docker Desktop macOS
    echo "192.168.65.254"
  else
    echo "$ip"
  fi
}

# Validate whitelist entry
validate_entry() {
  local ip="$1"
  local port="$2"

  # Check for forbidden patterns
  if [[ "$ip" =~ ^(10\.0\.0\.0/8|172\.16\.0\.0/12|192\.168\.0\.0/16)$ ]]; then
    echo "ERROR: Forbidden pattern $ip - would bypass isolation" >&2
    return 1
  fi

  # Check port is numeric and valid
  if ! [[ "$port" =~ ^[0-9]+$ ]] || [ "$port" -lt 1 ] || [ "$port" -gt 65535 ]; then
    echo "ERROR: Invalid port $port" >&2
    return 1
  fi

  return 0
}

echo "=== rcc Network Isolation Setup ==="

# Step 1: Create Docker network
echo "[1/4] Creating Docker network..."
docker network inspect $NETWORK_NAME >/dev/null 2>&1 || \
  docker network create --driver bridge --subnet $SUBNET $NETWORK_NAME

# Step 2: Clear existing DOCKER-USER rules for our subnet
echo "[2/4] Clearing existing rules..."
while sudo iptables -D DOCKER-USER -s $SUBNET -j DROP 2>/dev/null; do :; done
while sudo iptables -D DOCKER-USER -s $SUBNET -j ACCEPT 2>/dev/null; do :; done

# Step 3: Add DROP rules for RFC1918 (will end up at bottom)
echo "[3/4] Adding block rules for RFC1918..."
for RANGE in "10.0.0.0/8" "172.16.0.0/12" "192.168.0.0/16" "127.0.0.0/8" "169.254.0.0/16"; do
  sudo iptables -I DOCKER-USER -s $SUBNET -d $RANGE -j DROP
  echo "  Blocked: $RANGE"
done

# Step 4: Add whitelist ACCEPT rules (will end up above DROP)
echo "[4/4] Adding whitelist rules..."
if [ -f "$CONFIG_FILE" ]; then
  while IFS=: read -r ip port protocol; do
    [ -z "$ip" ] && continue

    # Validate
    if ! validate_entry "$ip" "$port"; then
      echo "  Skipping invalid entry: $ip:$port"
      continue
    fi

    # Resolve host.docker.internal
    resolved_ip=$(resolve_host_internal "$ip")

    # Add ACCEPT rule
    sudo iptables -I DOCKER-USER -s $SUBNET -d "$resolved_ip" -p "$protocol" --dport "$port" -j ACCEPT
    echo "  Allowed: $ip:$port ($protocol)"
  done < <(parse_whitelist)
else
  echo "  No whitelist config found at $CONFIG_FILE"
fi

# Always add established connections at top
sudo iptables -I DOCKER-USER -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

echo ""
echo "=== Configuration Complete ==="
echo "Network: $NETWORK_NAME ($SUBNET)"
echo ""
echo "Current DOCKER-USER rules:"
sudo iptables -L DOCKER-USER -n --line-numbers
```

#### Command: `rcc network whitelist add`

```bash
#!/bin/bash
# rcc network whitelist add <name> <ip> <port> [protocol]
# Example: rcc network whitelist add postgres host.docker.internal 5432 tcp

NAME="$1"
IP="$2"
PORT="$3"
PROTOCOL="${4:-tcp}"

CONFIG_FILE="${RCC_CONFIG:-$HOME/.rcc/network.yaml}"

# Validate
if [ -z "$NAME" ] || [ -z "$IP" ] || [ -z "$PORT" ]; then
  echo "Usage: rcc network whitelist add <name> <ip> <port> [protocol]"
  exit 1
fi

# Add to config using yq
if command -v yq &> /dev/null; then
  yq eval -i ".network.whitelist += [{\"name\": \"$NAME\", \"ip\": \"$IP\", \"port\": $PORT, \"protocol\": \"$PROTOCOL\"}]" "$CONFIG_FILE"
else
  echo "Please install yq or manually edit $CONFIG_FILE"
  exit 1
fi

echo "Added whitelist entry: $NAME ($IP:$PORT/$PROTOCOL)"
echo "Run 'rcc network init' to apply changes"
```

### IPv6 Considerations

If IPv6 is enabled, add corresponding ip6tables rules:

```bash
# Block IPv6 private ranges
sudo ip6tables -I DOCKER-USER -s fd00::/8 -j DROP   # Unique Local Addresses
sudo ip6tables -I DOCKER-USER -s fe80::/10 -j DROP  # Link-Local

# Or disable IPv6 entirely in container
docker run --sysctl net.ipv6.conf.all.disable_ipv6=1 ...
```

### Quick Reference

| Task | Command |
|------|---------|
| Initialize isolation | `rcc network init` |
| Add whitelist entry | `rcc network whitelist add <name> <ip> <port>` |
| Remove whitelist entry | `rcc network whitelist remove <name>` |
| List current rules | `sudo iptables -L DOCKER-USER -n` |
| Test connectivity | `docker run --network claude-isolated alpine ping <ip>` |

---

## File System Isolation

The second MVP security requirement is ensuring containers can only access explicitly mounted project directories.

### Docker Volume Mount Configuration

#### Principle: Explicit Mounts Only

Docker containers have no default access to the host filesystem. Only explicitly mounted volumes are accessible.

```bash
# SECURE: Only mount the specific project directory
docker run --rm \
  -v "/Users/me/projects/myapp:/workspace" \
  claude-code:latest

# Container can ONLY access /workspace (mapped to /Users/me/projects/myapp)
# Container CANNOT access /Users/me/Documents, /etc, /var, etc.
```

#### Read-Only Mounts for Extra Security

```bash
# Make project read-only, use tmpfs for writable areas
docker run --rm \
  --read-only \
  -v "/Users/me/projects/myapp:/workspace:ro" \
  --tmpfs /tmp:exec,size=1G \
  --tmpfs /home/node/.cache:exec,size=500M \
  claude-code:latest
```

#### Recommended Mount Configuration

```bash
docker run --rm \
  # Project directory (read-write for code changes)
  -v "$(pwd):/workspace" \
  \
  # Read-only container filesystem
  --read-only \
  \
  # Writable temp areas (in-memory, not persisted)
  --tmpfs /tmp:exec,size=2G \
  --tmpfs /home/node/.npm:exec,size=500M \
  --tmpfs /home/node/.cache:exec,size=500M \
  \
  # No other mounts - container cannot access anything else
  claude-code:latest
```

### Verification: Proving Directory Isolation

#### Test 1: Attempt to Access Unmounted Directories

```bash
# Inside container, try to access host directories
ls /Users              # Should fail: No such file or directory
ls /etc/passwd         # Shows container's /etc, not host
cat /var/log/system.log  # Should fail: No such file or directory

# Only mounted directory is accessible
ls /workspace          # SUCCESS: Shows project files
```

#### Test 2: Attempt to Escape Mount

```bash
# Try to escape via symlinks (blocked by Docker)
ln -s /Users/me/secrets /workspace/escape
cat /workspace/escape  # Fails: symlink doesn't resolve to host

# Try to escape via relative paths
cat /workspace/../../../etc/passwd  # Shows container's /etc, not host
```

#### Test 3: Verify Mount Boundaries

```bash
# From host, create a test file outside the mounted directory
echo "secret" > /Users/me/projects/OTHER_PROJECT/secret.txt

# Inside container, try to access it
cat /workspace/../OTHER_PROJECT/secret.txt
# Result: No such file or directory
# The container's /workspace/.. is NOT /Users/me/projects/
```

### Potential Escape Risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| **Docker socket mount** | ❌ CRITICAL | Never mount `/var/run/docker.sock` |
| **Privileged mode** | ❌ CRITICAL | Never use `--privileged` |
| **SYS_ADMIN capability** | ❌ HIGH | Never add `--cap-add SYS_ADMIN` |
| **Host PID namespace** | ❌ HIGH | Never use `--pid=host` |
| **Host network namespace** | ⚠️ MEDIUM | Avoid `--network=host` |
| **Bind mount rename escape** | ⚠️ LOW | Rare kernel bug; keep Docker updated |
| **Symlink following** | ✅ LOW | Docker blocks symlink escape by default |

### Secure Container Configuration

```bash
#!/bin/bash
# rcc-run-isolated.sh
# Run Claude Code with full isolation

PROJECT_DIR="${1:-.}"
PROJECT_DIR=$(cd "$PROJECT_DIR" && pwd)  # Absolute path

docker run -it --rm \
  --name claude-isolated \
  \
  # Network isolation (use pf-isolated network)
  --network claude-isolated \
  \
  # File isolation: only mount project directory
  -v "$PROJECT_DIR:/workspace" \
  \
  # Read-only root filesystem
  --read-only \
  --tmpfs /tmp:exec,size=2G \
  --tmpfs /home/node/.npm:exec,size=500M \
  --tmpfs /home/node/.cache:exec,size=500M \
  \
  # Drop ALL capabilities
  --cap-drop ALL \
  \
  # Security options
  --security-opt no-new-privileges:true \
  --security-opt seccomp=default \
  \
  # Resource limits
  --memory=4g \
  --cpus=2 \
  \
  # Environment
  -e ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" \
  -w /workspace \
  \
  claude-code:latest "$@"
```

### File Isolation Security Guarantee

```
AI Attack Vector Analysis (File System):

1. Access unmounted directories:
   AI in container → Tries: cat /Users/me/secrets
   → Result: "No such file or directory"
   → Container filesystem is isolated namespace

2. Escape via path traversal:
   AI in container → Tries: cat /workspace/../../../etc/passwd
   → Result: Shows container's /etc/passwd, not host
   → /workspace/.. is container's /, not host's mount point parent

3. Escape via symlink:
   AI in container → Creates: ln -s /host-secrets /workspace/link
   → Result: Dangling symlink (target doesn't exist in container)
   → Docker doesn't follow symlinks outside mount

4. Escape via Docker socket:
   AI in container → Tries: docker run -v /:/host alpine cat /host/etc/passwd
   → Result: "docker: command not found" (socket not mounted)
   → CRITICAL: Never mount Docker socket

5. Escape via privileged mode:
   AI in container → Tries: mount /dev/sda1 /mnt
   → Result: "Permission denied" (not privileged)
   → CRITICAL: Never use --privileged

✅ CONCLUSION: With proper configuration, AI cannot access host files
   outside the explicitly mounted project directory.
```

---

## References

- [Docker DOCKER-USER Chain Documentation](https://docs.docker.com/engine/network/firewall-iptables/)
- [Docker Engine 28 Security Hardening](https://www.docker.com/blog/docker-engine-28-hardening-container-networking-by-default/)
- [Docker Packet Filtering](https://docs.docker.com/engine/network/packet-filtering-firewalls/)
- [macOS pf Firewall Guide](https://blog.neilsabol.site/post/quickly-easily-adding-pf-packet-filter-firewall-rules-macos-osx/)
- [Colima Documentation](https://github.com/abiosoft/colima)
- [ufw-docker Security Fix](https://github.com/chaifeng/ufw-docker)
- [Firewalld Docker Filtering](https://firewalld.org/2024/04/strictly-filtering-docker-containers)
- [Docker Bind Mounts Security](https://docs.docker.com/engine/storage/bind-mounts/)
- [Docker Security Best Practices](https://cheatsheetseries.owasp.org/cheatsheets/Docker_Security_Cheat_Sheet.html)
