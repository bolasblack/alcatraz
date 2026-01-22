---
title: "Linux Isolation Solution"
description: "Choose Podman + nftables + SELinux for Claude Code isolation on Linux"
tags: linux, file-isolation, network-isolation
---

## Context

Need to provide AI-untouchable isolation for Claude Code on Linux, with same requirements as macOS:
- File isolation
- Network isolation (block RFC1918, configurable whitelist)
- Memory auto-release

## Decision

**Recommended Stack: Podman + nftables + SELinux**

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Container | Podman | Rootless by default, no daemon attack surface |
| Network | nftables | Host-level (AI-untouchable), kernel-level atomicity |
| MAC | SELinux/AppArmor | Additional container-to-container separation |
| Memory | cgroups v2 | Native auto-release, same behavior as macOS |

**Deployment**: Initial setup requires `sudo rcc init` to install firewall rules via systemd service. Runtime operation is rootless.

## Consequences

- Free, open source solution
- Works on all major Linux distributions
- Root required for initial firewall setup only
- DNS-to-IP resolution needed for domain whitelist (ipset + systemd timer)

## References

- `docs/research/linux/final-report.md`
- `docs/research/linux/network-research.md`
- `docs/research/linux/container-research.md`
