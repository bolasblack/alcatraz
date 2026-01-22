# Firewall Declarative Management Research

Research findings for RCC (Claude Code isolation tool) firewall management with focus on crash recovery, atomicity, and clean uninstall guarantees.

## Executive Summary

| Platform | Atomicity | Crash Recovery | Recommended Approach |
|----------|-----------|----------------|----------------------|
| macOS (pf) | Partial (anchor-level) | Manual/Convention-based | Named anchors + idempotent operations |
| Linux (nftables) | Full (kernel-level) | Built-in via atomic transactions | `nft -f` with flush + rules in single file |

**Key Finding**: nftables provides true kernel-level atomicity guarantees. macOS pf requires careful design patterns to achieve similar safety.

---

## macOS: Packet Filter (pf)

### Overview

macOS uses PF (Packet Filter), based on OpenBSD/FreeBSD's implementation. Since macOS 10.7 (Lion), PF replaced the deprecated IPFW firewall.

### Anchor Mechanism

Anchors are named sub-rulesets that can be managed independently from the main ruleset.

```
# Main ruleset (/etc/pf.conf)
anchor "com.rcc/*"

# RCC-specific rules (/etc/pf.anchors/com.rcc)
block out quick proto tcp from any to 169.254.0.0/16
```

**Key Properties**:
- Anchors are isolated namespaces
- Flushing the main ruleset does NOT affect anchors
- Anchors persist until explicitly flushed AND have no child anchors
- System services dynamically insert/remove their own anchors

### Atomicity Analysis

**What IS atomic**:
- Loading rules into an anchor via `pfctl -a anchor -f file`
- The kernel applies the rule file as a single unit

**What is NOT atomic**:
- The sequence: create anchor → load rules → update state file
- Enable/disable operations (`pfctl -e` / `pfctl -d`)

**Reference Counting System**:
macOS provides `-E` and `-X` flags for reference-counted PF enable/disable:
```bash
# Enable PF and get token
TOKEN=$(pfctl -E 2>&1 | grep -o 'Token : [0-9]*' | cut -d' ' -f3)

# Disable using token (only disables when last reference released)
pfctl -X $TOKEN
```

### Crash Recovery Strategy for pf

Since pf lacks built-in crash recovery, use convention-based idempotent operations:

```bash
#!/bin/bash
# rcc-firewall.sh - Idempotent firewall management

ANCHOR="com.rcc"
RULES_FILE="/etc/pf.anchors/com.rcc.rules"

install() {
    # 1. Ensure anchor point exists in main ruleset (idempotent)
    if ! pfctl -sr | grep -q "anchor \"$ANCHOR\""; then
        echo "anchor \"$ANCHOR\"" | pfctl -a root -f -
    fi

    # 2. Flush existing rules in anchor (idempotent - safe if empty)
    pfctl -a "$ANCHOR" -F rules 2>/dev/null || true

    # 3. Load new rules (atomic within anchor)
    pfctl -a "$ANCHOR" -f "$RULES_FILE"

    # 4. Enable PF with reference counting
    pfctl -E
}

uninstall() {
    # Always safe to run - idempotent

    # 1. Flush anchor rules
    pfctl -a "$ANCHOR" -F rules 2>/dev/null || true

    # 2. Clear states associated with anchor
    pfctl -a "$ANCHOR" -F states 2>/dev/null || true

    # 3. Release our PF reference (if we have one)
    # Note: PF only disabled when ALL references released
    pfctl -X "$TOKEN" 2>/dev/null || true
}

# Crash recovery: just run uninstall then install
recover() {
    uninstall
    install
}
```

### Crash Scenario Analysis (pf)

| Crash Point | State After Crash | Recovery Action |
|-------------|-------------------|-----------------|
| Before anchor creation | No RCC rules | Run install (idempotent) |
| After anchor, before rules | Empty anchor exists | Run install (flushes then loads) |
| After rules loaded | Rules active, no state file | Run uninstall then install |
| During uninstall | Partial rules may remain | Run uninstall again (idempotent) |

**Key Insight**: Making every operation idempotent means "just run it again" is always the recovery strategy.

---

## Linux: nftables

### Overview

nftables is the modern Linux firewall framework (successor to iptables). It provides **true atomic operations** at the kernel level.

### Atomicity Guarantees

**Kernel-level atomicity**:
```bash
# This is ATOMIC - all or nothing
nft -f /etc/nftables/rcc.conf

# If ANY rule fails validation, NONE are applied
```

**How it works**:
1. nft reads entire config file into memory
2. Validates all rules
3. Creates new ruleset alongside existing one
4. Single kernel operation swaps old → new
5. No intermediate state is ever visible

### Atomic Flush + Load Pattern

```bash
# NON-ATOMIC (bad - creates window with no rules):
nft flush ruleset
nft -f /etc/nftables/rcc.conf

# ATOMIC (good - single operation):
nft -f - <<'EOF'
flush table inet rcc
table inet rcc {
    chain output {
        type filter hook output priority 0;
        ip daddr 169.254.0.0/16 drop
    }
}
EOF
```

### Recommended nftables Structure for RCC

```nft
#!/usr/sbin/nft -f

# /etc/nftables/rcc.conf
# Self-contained, idempotent ruleset

# Delete table if exists (handles re-apply)
table inet rcc
delete table inet rcc

# Create fresh table with rules
table inet rcc {
    chain output {
        type filter hook output priority 0; policy accept;

        # Block link-local
        ip daddr 169.254.0.0/16 drop
        ip6 daddr fe80::/10 drop

        # Block metadata services
        ip daddr 169.254.169.254 drop
    }
}
```

### Crash Recovery Strategy for nftables

**Built-in via atomicity**: Since `nft -f` is atomic, crash recovery is trivial:

```bash
#!/bin/bash
# rcc-firewall.sh - nftables version

RULES_FILE="/etc/nftables/rcc.conf"

install() {
    # Single atomic operation - inherently idempotent
    nft -f "$RULES_FILE"
}

uninstall() {
    # Also atomic and idempotent
    nft delete table inet rcc 2>/dev/null || true
}

# Crash recovery is just re-running install
recover() {
    install  # Atomic, idempotent
}
```

### Crash Scenario Analysis (nftables)

| Crash Point | State After Crash | Recovery Action |
|-------------|-------------------|-----------------|
| Before `nft -f` starts | No RCC rules | Run install |
| During `nft -f` | **No partial state** - either old rules or no rules | Run install |
| After `nft -f` completes | Rules fully applied | Done |
| During uninstall | Table either exists or doesn't | Run uninstall again |

**Key Insight**: Kernel atomicity eliminates partial state entirely.

---

## Nix Integration

### NixOS (nftables)

NixOS provides first-class nftables support with atomic activation:

```nix
# configuration.nix
{
  networking.nftables = {
    enable = true;

    # Atomic ruleset loading
    ruleset = ''
      table inet rcc {
        chain output {
          type filter hook output priority 0;
          ip daddr 169.254.0.0/16 drop
        }
      }
    '';

    # Or use tables option (auto-cleanup on removal)
    tables.rcc = {
      family = "inet";
      content = ''
        chain output {
          type filter hook output priority 0;
          ip daddr 169.254.0.0/16 drop
        }
      '';
    };
  };
}
```

**Atomic guarantees**:
- Uses `nft --check --file` for pre-validation
- Single `nft -f` for application
- systemd `oneshot` service with `RemainAfterExit`

**Rollback**: NixOS generations provide system-level rollback including firewall config.

### nix-darwin (macOS pf)

**Current state**: No built-in pf module. Manual configuration required.

```nix
# darwin-configuration.nix
{
  # Launch daemon to enable pf at boot
  launchd.daemons.pfctl = {
    serviceConfig = {
      Label = "org.nixos.pfctl";
      ProgramArguments = [ "/sbin/pfctl" "-e" "-f" "/etc/pf.anchors/rcc.conf" ];
      RunAtLoad = true;
    };
  };

  # Deploy rule files (use copy, not symlink!)
  environment.etc."pf.anchors/rcc.conf" = {
    text = ''
      # RCC firewall rules
      block out quick proto tcp from any to 169.254.0.0/16
    '';
    copy = true;  # Important: symlinks cause issues with pf
  };
}
```

**Limitations**:
- No automatic anchor management
- Manual crash recovery needed
- `copy = true` required (symlinks problematic)

---

## Existing Tools and Libraries

### macOS (pf)

#### pfctl-rs (Rust) - Recommended
- **Repository**: https://github.com/mullvad/pfctl-rs
- **Maintainer**: Mullvad VPN
- **Features**:
  - Transaction-based rule changes
  - Idempotent operations (`try_add_anchor`, `try_add_rule`)
  - Anchor management API
  - State clearing

```rust
use pfctl::{PfCtl, FilterRuleBuilder, AnchorKind};

fn install_rules() -> Result<(), pfctl::Error> {
    let mut pf = PfCtl::new()?;

    // Idempotent anchor creation
    pf.try_add_anchor("rcc", AnchorKind::Filter)?;

    // Build and add rules
    let rule = FilterRuleBuilder::default()
        .action(FilterRuleAction::Drop)
        .direction(Direction::Out)
        .to(Ip::from_str("169.254.0.0/16")?)
        .build()?;

    pf.add_rule("rcc", &rule)?;
    pf.enable()?;

    Ok(())
}

fn uninstall_rules() -> Result<(), pfctl::Error> {
    let mut pf = PfCtl::new()?;

    // Idempotent cleanup
    pf.flush_rules("rcc", RulesetKind::Filter)?;
    pf.try_remove_anchor("rcc", AnchorKind::Filter)?;

    Ok(())
}
```

### Linux (nftables)

#### nftables-rs (Rust)
- **Repository**: https://github.com/namib-project/nftables-rs
- **Features**: JSON API abstraction, safe Rust types

#### rustables (Rust)
- **Repository**: https://gitlab.com/rustwall/rustables
- **Features**: Direct netlink interface, batch operations

#### nftnl-rs (Rust) - by Mullvad
- **Repository**: https://github.com/mullvad/nftnl-rs
- **Features**: Low-level libnftnl bindings

#### firewall_toolkit (Go)
- **Repository**: https://github.com/ngrok/firewall_toolkit
- **Features**: High-level nftables API, eBPF integration

### Comparison

| Library | Platform | Atomicity | Idempotent Ops | Maturity |
|---------|----------|-----------|----------------|----------|
| pfctl-rs | macOS | Anchor-level | Yes (try_*) | Production (Mullvad) |
| nftables-rs | Linux | Kernel-level | Via atomic load | Active development |
| rustables | Linux | Batch operations | Via batches | Active development |
| firewall_toolkit | Linux | Via nft | Via abstractions | Production (ngrok) |

---

## Recommended Implementation for RCC

### Design Principles

1. **Idempotent operations**: Every operation must be safely re-runnable
2. **No external state files**: Firewall itself is source of truth
3. **Atomic where possible**: Use platform capabilities
4. **Graceful degradation**: Partial state should be detectable and recoverable

### Recommended Pattern

```
┌─────────────────────────────────────────────────────────────┐
│                    RCC Firewall Manager                      │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  install()                                                   │
│    ├── detect_platform()                                     │
│    ├── [Linux] nft_atomic_load(rules)     ← Kernel atomic   │
│    └── [macOS] pf_anchor_load(rules)      ← Anchor atomic   │
│                                                              │
│  uninstall()                                                 │
│    ├── [Linux] nft_delete_table("rcc")    ← Idempotent      │
│    └── [macOS] pf_flush_anchor("rcc")     ← Idempotent      │
│                                                              │
│  verify()                                                    │
│    ├── [Linux] nft_list_table("rcc")                        │
│    └── [macOS] pfctl -a rcc -sr                             │
│                                                              │
│  recover()                                                   │
│    └── uninstall() then install()         ← Always safe     │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Implementation Pseudocode

```rust
pub struct FirewallManager {
    platform: Platform,
}

impl FirewallManager {
    pub fn install(&self, rules: &RuleSet) -> Result<()> {
        match self.platform {
            Platform::Linux => {
                // Generate nftables config with flush + rules
                let config = format!(
                    "table inet rcc\ndelete table inet rcc\n{}",
                    rules.to_nftables()
                );
                // Single atomic operation
                Command::new("nft").arg("-f").arg("-")
                    .stdin(config)
                    .status()?;
            }
            Platform::MacOS => {
                let mut pf = PfCtl::new()?;
                // Idempotent anchor creation
                pf.try_add_anchor("rcc", AnchorKind::Filter)?;
                // Flush existing (idempotent)
                let _ = pf.flush_rules("rcc", RulesetKind::Filter);
                // Load new rules
                for rule in rules.to_pf_rules() {
                    pf.add_rule("rcc", &rule)?;
                }
                pf.enable()?;
            }
        }
        Ok(())
    }

    pub fn uninstall(&self) -> Result<()> {
        match self.platform {
            Platform::Linux => {
                // Idempotent delete
                let _ = Command::new("nft")
                    .args(["delete", "table", "inet", "rcc"])
                    .status();
            }
            Platform::MacOS => {
                let mut pf = PfCtl::new()?;
                // Idempotent cleanup
                let _ = pf.flush_rules("rcc", RulesetKind::Filter);
                let _ = pf.clear_states("rcc");
                let _ = pf.try_remove_anchor("rcc", AnchorKind::Filter);
            }
        }
        Ok(())
    }

    pub fn verify(&self) -> Result<bool> {
        match self.platform {
            Platform::Linux => {
                let output = Command::new("nft")
                    .args(["list", "table", "inet", "rcc"])
                    .output()?;
                Ok(output.status.success())
            }
            Platform::MacOS => {
                let output = Command::new("pfctl")
                    .args(["-a", "rcc", "-sr"])
                    .output()?;
                Ok(!output.stdout.is_empty())
            }
        }
    }

    pub fn recover(&self, rules: &RuleSet) -> Result<()> {
        // Simple and always safe
        self.uninstall()?;
        self.install(rules)
    }
}
```

### Edge Case Handling

| Scenario | Linux (nftables) | macOS (pf) |
|----------|------------------|------------|
| `kill -9` during install | No partial state (atomic) | Anchor may exist without rules - verify() detects, recover() fixes |
| `kill -9` during uninstall | Table either exists or not | May have partial rules - uninstall() is idempotent |
| Reboot during install | Rules not persisted | Rules not persisted |
| Double install | Atomic replace | Flush then load (idempotent) |
| Double uninstall | No-op (table doesn't exist) | No-op (anchor empty/gone) |

### Verification Questions Answered

1. **If `kill -9` during install, can subsequent uninstall still work?**
   - **Linux**: Yes. Either table exists (uninstall works) or doesn't (no-op).
   - **macOS**: Yes. Anchor flush is always safe. May need to also flush states.

2. **Are there existing tools/libraries?**
   - **Yes**: pfctl-rs (macOS), nftables-rs/rustables (Linux) provide safe abstractions.

3. **Can Nix activation scripts guarantee atomicity?**
   - **NixOS**: Yes, via `nft -f` atomic loading.
   - **nix-darwin**: No built-in support; manual implementation needed.

---

## Conclusion

### For RCC Implementation

1. **Use nftables on Linux** - kernel-level atomicity eliminates partial state concerns
2. **Use pf anchors on macOS** - with idempotent operations via pfctl-rs
3. **No external state files** - query firewall directly for current state
4. **Recovery = uninstall + install** - always safe due to idempotent design
5. **Consider pfctl-rs** - Mullvad uses it in production for their VPN client

### Key Takeaways

- **nftables is superior** for crash safety due to true atomic transactions
- **pf requires careful design** but can achieve equivalent safety with idempotent operations
- **Avoid state files** - they create synchronization problems; let the firewall be the source of truth
- **Test crash scenarios** - simulate `kill -9` at various points to verify recovery

---

## References

- [OpenBSD PF Anchors](https://www.openbsd.org/faq/pf/anchors.html)
- [nftables Atomic Rule Replacement](https://wiki.nftables.org/wiki-nftables/index.php/Atomic_rule_replacement)
- [nftables Scripting](https://wiki.nftables.org/wiki-nftables/index.php/Scripting)
- [NixOS nftables Module](https://github.com/NixOS/nixpkgs/blob/master/nixos/modules/services/networking/nftables.nix)
- [pfctl-rs](https://github.com/mullvad/pfctl-rs)
- [nftables-rs](https://github.com/namib-project/nftables-rs)
- [pfctl man page](https://man.freebsd.org/pfctl)
- [FreeBSD PF Manual](https://ss64.com/mac/pfctl.html)
