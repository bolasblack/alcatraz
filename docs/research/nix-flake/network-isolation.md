# Nix Network Isolation Research

## Executive Summary

**Critical Finding: Nix network isolation is NOT AI-untouchable in containers.**

While Nix can declaratively configure host-level firewalls (NixOS networking.firewall, nix-darwin pf), the fundamental security limitation is that NixOS containers do not provide strong isolation - root users inside containers can escape to the host system.

---

## 1. Nix Network Management Capabilities

### 1.1 NixOS (Linux) - Host Firewall

NixOS provides declarative firewall configuration through `networking.firewall.*` options:

```nix
# Basic firewall configuration
networking.firewall = {
  enable = true;
  allowedTCPPorts = [ 80 443 ];
  allowedUDPPortRanges = [
    { from = 4000; to = 4007; }
  ];
};

# Use nftables backend (recommended)
networking.nftables.enable = true;
```

**Key capabilities:**
- Default: iptables-based, can switch to nftables
- Interface-specific rules: `networking.firewall.interfaces."eth0".allowedTCPPorts`
- Custom nftables rules via `networking.nftables.tables` or `networking.firewall.extraInputRules`
- Logging: `networking.firewall.logRefusedPackets = true`

**Example: Block RFC1918 outbound (custom nftables):**
```nix
networking.nftables = {
  enable = true;
  ruleset = ''
    table inet filter {
      chain output {
        type filter hook output priority 0; policy accept;
        # Block RFC1918 addresses
        ip daddr 10.0.0.0/8 drop
        ip daddr 172.16.0.0/12 drop
        ip daddr 192.168.0.0/16 drop
      }
    }
  '';
};
```

**Sources:**
- [NixOS Wiki - Firewall](https://wiki.nixos.org/wiki/Firewall)
- [NixOS Discourse - Custom Firewall Rules](https://discourse.nixos.org/t/is-it-possible-to-write-custom-rules-to-the-nixos-firewall/27900)

### 1.2 nix-darwin (macOS) - Host Firewall

nix-darwin provides two approaches for firewall on macOS:

**Approach A: Application Firewall (alf) - Limited**
```nix
# Built-in options (may have issues on recent macOS)
system.defaults.alf.globalstate = 1;
networking.applicationFirewall.enable = true;
networking.applicationFirewall.blockAllIncoming = true;
```

**Approach B: pf (Packet Filter) - Recommended**
```nix
# Enable pf at boot via launchd
launchd.daemons.pfctl = {
  command = "/sbin/pfctl -e -f /etc/pf.conf";
  serviceConfig = {
    RunAtLoad = true;
    KeepAlive = false;
  };
};

# Deploy pf.conf via environment.etc
environment.etc = {
  "pf.conf" = {
    copy = true;  # Important: use copy, not symlink
    text = ''
      # Block RFC1918 outbound
      block out quick on egress to 10.0.0.0/8
      block out quick on egress to 172.16.0.0/12
      block out quick on egress to 192.168.0.0/16
      # Allow all other outbound
      pass out all
    '';
  };
};
```

**Note:** The `system.defaults.alf` options have reported issues on recent macOS versions. The pf approach via launchd is more reliable.

**Sources:**
- [nix-darwin Manual](https://nix-darwin.github.io/nix-darwin/manual/)
- [Adventures with pf and nix-darwin](https://www.vczf.us/230729_macos-ventura-pfctl-nixdarwin-tailscale.html)

### 1.3 home-manager - NO Firewall Capability

**home-manager CANNOT configure firewalls.** It operates at user-level only:

- Manages user packages, dotfiles, user services
- Requires root/system privileges to modify firewall rules
- Firewall configuration must be done at system level (NixOS config or nix-darwin)

**Source:** [Home Manager Manual](https://nix-community.github.io/home-manager/)

---

## 2. Platform Comparison

| Capability | NixOS | nix-darwin | home-manager |
|------------|-------|------------|--------------|
| Host firewall config | ✅ networking.firewall | ⚠️ pf via launchd | ❌ |
| Declarative rules | ✅ Full support | ⚠️ Manual pf.conf | ❌ |
| nftables/iptables | ✅ Both | ❌ N/A | ❌ |
| pf (BSD firewall) | ❌ N/A | ✅ Manual | ❌ |
| Interface-specific | ✅ Yes | ⚠️ Manual | ❌ |
| Block RFC1918 | ✅ Custom rules | ✅ pf rules | ❌ |

---

## 3. AI-Untouchable Assessment (CRITICAL)

### 3.1 Container Security Limitations

**NixOS containers (systemd-nspawn) do NOT provide strong isolation:**

> "NixOS' containers do not provide full security out of the box (just like docker). They do give you a separate chroot, but a privileged user (root) in a container can escape the container and become root on the host system."
> - [NixOS Wiki - Containers](https://nixos.wiki/wiki/NixOS_Containers)

**Implications:**
- An AI running as root inside a NixOS container CAN escape to host
- Once on host, AI could modify host firewall rules
- Even with `privateNetwork = true`, root escape is possible

### 3.2 Mitigation Options (All Insufficient for AI-Untouchable)

| Mitigation | Effectiveness | Why Insufficient |
|------------|---------------|------------------|
| Drop capabilities | Partial | Doesn't prevent all escape vectors |
| Unprivileged containers (LXC) | Better | Still kernel-shared, potential exploits |
| Network namespaces | Partial | Configures inside container, not host FW |
| MicroVMs | Best | Overhead, but closest to AI-untouchable |

### 3.3 Critical Analysis: Is Nix Network Isolation AI-Untouchable?

**Answer: NO - with important caveats**

| Scenario | AI-Untouchable? | Reason |
|----------|-----------------|--------|
| AI in NixOS container | ❌ NO | Root can escape to host |
| AI in unprivileged container | ⚠️ Partial | Less escape risk, but kernel shared |
| AI in MicroVM | ✅ Mostly | VM boundary, but Nix manages it |
| AI running directly on NixOS | ❌ NO | If AI has any write access to /etc/nixos |

**The fundamental problem:** Nix's declarative nature means firewall rules are stored in configuration files that could potentially be modified by an AI with sufficient privileges, then applied via `nixos-rebuild switch` or `darwin-rebuild switch`.

### 3.4 Host Firewall vs Container Network Config

| Layer | What Nix Configures | AI Access Risk |
|-------|---------------------|----------------|
| Host firewall (nftables/pf) | System-level rules | If AI escapes container → HIGH |
| Container privateNetwork | Virtual interfaces | Contained but escape possible |
| Network namespaces | Isolated stack | Inside container, AI can modify |

---

## 4. Comparison with Direct pf/nftables Approach

| Aspect | Nix-Managed | Direct pf/nftables |
|--------|-------------|-------------------|
| Declarative | ✅ Yes | ❌ Imperative |
| Reproducible | ✅ Yes | ❌ Manual |
| AI-Untouchable | ❌ Config files accessible | ⚠️ If rules in separate file |
| Rollback | ✅ NixOS generations | ❌ Manual backup |
| Audit trail | ✅ Git-managed configs | ❌ Manual |

**Key insight:** The declarative nature of Nix is a **double-edged sword** for security:
- ✅ **Pro:** Easy to audit, reproduce, and rollback
- ❌ **Con:** All config in files that AI could theoretically modify

---

## 5. Recommendations for RCC Integration

### 5.1 For True AI-Untouchable Network Isolation

The research suggests Nix alone cannot provide AI-untouchable network isolation. Recommended architecture:

```
┌─────────────────────────────────────────────────────┐
│ HOST (macOS/Linux)                                  │
│ ┌─────────────────────────────────────────────────┐ │
│ │ HOST FIREWALL (pf/nftables)                     │ │
│ │ - Managed OUTSIDE of AI-accessible config      │ │
│ │ - Block RFC1918 for AI processes               │ │
│ │ - Whitelist specific IPs                       │ │
│ └─────────────────────────────────────────────────┘ │
│ ┌─────────────────────────────────────────────────┐ │
│ │ NIX DEVSHELL / CONTAINER                        │ │
│ │ ┌───────────────────────────────────────────┐   │ │
│ │ │ AI PROCESS                                │   │ │
│ │ │ - Cannot modify host firewall             │   │ │
│ │ │ - No root access                          │   │ │
│ │ │ - Limited to project directory            │   │ │
│ │ └───────────────────────────────────────────┘   │ │
│ └─────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────┘
```

### 5.2 Practical Approach

1. **Host firewall managed separately from Nix flake**
   - Store pf.conf/nftables rules outside project directory
   - Apply at system boot, not via project flake

2. **Use Nix flake for development environment only**
   - Define packages, dev tools
   - Do NOT define firewall rules in project flake

3. **Process-level enforcement**
   - Run AI as unprivileged user
   - Use cgroups/namespaces for resource limits
   - Consider nsjail or firejail for additional sandboxing

### 5.3 MicroVM Option (Best Isolation)

For strongest isolation, consider [microvm.nix](https://github.com/astro/microvm.nix):

```nix
# MicroVM provides VM-level isolation
microvm.vms.ai-sandbox = {
  config = {
    # Network config inside VM
    # Host firewall blocks VM's attempts to reach RFC1918
  };
};
```

---

## 6. Conclusions

### Key Findings

1. **Nix CAN configure host firewalls** (NixOS: nftables/iptables, nix-darwin: pf)
2. **home-manager CANNOT configure firewalls** (user-level only)
3. **NixOS containers are NOT secure against root escape**
4. **Nix network isolation is NOT AI-untouchable** because:
   - Containers share kernel with host
   - Config files are potentially AI-accessible
   - Root escape is documented and possible

### Recommendation for RCC

**Do NOT rely solely on Nix for AI-untouchable network isolation.**

Instead:
- Use Nix for reproducible dev environments
- Manage host firewall SEPARATELY from AI-accessible configs
- Consider MicroVMs for stronger isolation
- Implement defense-in-depth with multiple layers

---

## Sources

- [NixOS Wiki - Firewall](https://wiki.nixos.org/wiki/Firewall)
- [NixOS Wiki - Containers](https://wiki.nixos.org/wiki/NixOS_Containers)
- [NixOS Wiki - Security](https://wiki.nixos.org/wiki/Security)
- [nix-darwin Manual](https://nix-darwin.github.io/nix-darwin/manual/)
- [Home Manager Manual](https://nix-community.github.io/home-manager/)
- [nixos-nftables-firewall](https://thelegy.github.io/nixos-nftables-firewall/)
- [NixOS Discourse - Container Limitations](https://discourse.nixos.org/t/nixos-container-limitations/1835)
- [microvm.nix](https://github.com/astro/microvm.nix)
