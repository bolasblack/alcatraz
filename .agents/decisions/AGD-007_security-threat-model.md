---
title: Security Threat Model
description: AI-Untouchable guarantee analysis and social engineering threat vectors
tags: security
updated_by: AGD-026
---


## Context

RCC provides isolation for Claude Code. A key requirement is "AI-Untouchable" - enforcement at a layer AI cannot bypass. This document analyzes the security guarantees and limitations.

## Decision

### AI-Untouchable Guarantee

Host-level firewalls provide AI-untouchable network isolation:

| Platform       | Firewall | AI Can Bypass? | Bypass Method           |
| -------------- | -------- | -------------- | ----------------------- |
| macOS          | pf       | ❌ No (direct) | Social engineering only |
| Linux          | nftables | ❌ No (direct) | Social engineering only |
| Linux (Colima) | iptables | ⚠️ Yes         | `colima ssh -- sudo iptables -F` |

### Social Engineering: Universal Limitation

Social engineering is a universal limitation of ALL host-firewall solutions, not specific to any platform.

| Attack Vector                        | macOS pf    | Linux nftables | Risk Level   |
| ------------------------------------ | ----------- | -------------- | ------------ |
| Write malicious script to /workspace | ✅ Possible | ✅ Possible    | HIGH         |
| Trick user to run `sudo pfctl -d`    | ✅ Possible | N/A            | HIGH         |
| Trick user to run `sudo nft flush`   | N/A         | ✅ Possible    | HIGH         |
| Colima passwordless sudo bypass      | N/A         | ✅ Possible    | **CRITICAL** |

### Mitigation Strategy

1. **User education** - Don't run scripts AI suggests with sudo
2. **Avoid Colima** - For security-critical workloads
3. **Defense in depth** - Technical isolation + user vigilance = complete security

## Consequences

- Technical isolation prevents direct bypass
- Social engineering remains theoretical attack vector
- User awareness is critical part of security model
- Colima explicitly not recommended for security-sensitive use

## References

- `.agents/references/initial-research/macos/network-isolation.md`
- `.agents/references/initial-research/linux/network-research.md`
- `.agents/references/initial-research/cross-container/security-analysis.md`
