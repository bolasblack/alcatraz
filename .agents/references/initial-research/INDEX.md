# RCC Documentation Index

**Project**: Claude Code Isolated Execution Environment (rcc)
**Last Updated**: 2026-01-22

---

## Overview

RCC provides AI-untouchable isolation for Claude Code execution, ensuring file isolation, network isolation, and memory management across macOS and Linux platforms.

---

## Directory Structure

```
research/
├── INDEX.md                          ← You are here
├── decision/
├── macos/                        ← macOS platform research
├── linux/                        ← Linux platform research
├── cross-review/                 ← Cross-platform verification
├── nix-flake/                    ← Nix Flake feasibility study
└── cross-container/              ← Cross-container communication
```

---

## Documents by Topic

### Architecture Decisions

| File                                               | Description                                                        |
| -------------------------------------------------- | ------------------------------------------------------------------ |
| [final-recommendation.md](final-recommendation.md) | Final architecture recommendation with platform-specific solutions |

### macOS Research

| File                                                                       | Description                                     |
| -------------------------------------------------------------------------- | ----------------------------------------------- |
| [research/macos/final-report.md](research/macos/final-report.md)           | Comprehensive macOS isolation solution analysis |
| [research/macos/network-isolation.md](research/macos/network-isolation.md) | pf firewall and network isolation deep dive     |
| [research/macos/memory-management.md](research/macos/memory-management.md) | Memory balloon and auto-release behavior        |

### Linux Research

| File                                                                                     | Description                                     |
| ---------------------------------------------------------------------------------------- | ----------------------------------------------- |
| [research/linux/final-report.md](research/linux/final-report.md)                         | Comprehensive Linux isolation solution analysis |
| [research/linux/container-research.md](research/linux/container-research.md)             | Podman vs Docker vs LXC comparison              |
| [research/linux/network-research.md](research/linux/network-research.md)                 | nftables and iptables isolation strategies      |
| [research/linux/memory-security-research.md](research/linux/memory-security-research.md) | cgroups v2 memory management verification       |

### Cross-Platform Review

| File                                                                                                   | Description                           |
| ------------------------------------------------------------------------------------------------------ | ------------------------------------- |
| [research/cross-review/review-macos.md](research/cross-review/review-macos.md)                         | Linux team's review of macOS solution |
| [research/cross-review/review-linux.md](research/cross-review/review-linux.md)                         | macOS team's review of Linux solution |
| [research/cross-review/review-linux-corrections.md](research/cross-review/review-linux-corrections.md) | Post-verification corrections         |

### Nix Flake Research

| File                                                                                       | Description                                                   |
| ------------------------------------------------------------------------------------------ | ------------------------------------------------------------- |
| [research/nix-flake/summary.md](research/nix-flake/summary.md)                             | Executive summary of Nix Flake feasibility                    |
| [research/nix-flake/container-integration.md](research/nix-flake/container-integration.md) | nix2container, NixOS containers, arion, NixPak analysis       |
| [research/nix-flake/network-isolation.md](research/nix-flake/network-isolation.md)         | Nix network config capabilities and AI-untouchable assessment |
| [research/nix-flake/distribution.md](research/nix-flake/distribution.md)                   | Flake distribution mechanism with example flake.nix           |
| [research/nix-flake/firewall-declarative.md](research/nix-flake/firewall-declarative.md)   | Crash recovery and atomicity for pf/nftables                  |

### Cross-Container Communication

| File                                                                                                         | Description                                                 |
| ------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------- |
| [research/cross-container/summary.md](research/cross-container/summary.md)                                   | Cross-container communication recommendation                |
| [research/cross-container/ccc-statusd-architecture.md](research/cross-container/ccc-statusd-architecture.md) | ccc-statusd daemon architecture analysis                    |
| [research/cross-container/communication-patterns.md](research/cross-container/communication-patterns.md)     | 4 patterns compared (Host Relay, Socket, File IPC, Network) |
| [research/cross-container/security-analysis.md](research/cross-container/security-analysis.md)               | Threat model and risk assessment                            |

---

## Key Findings Summary

### MVP Requirements

1. **File Isolation** - AI can only access specified project directory
2. **Network Isolation** - Block RFC1918 by default, configurable whitelist
3. **AI-Untouchable** - Enforcement at layer AI cannot bypass
4. **Memory Auto-Release** - Use 1G, occupy only 1G

### Platform Recommendations

| Platform  | Solution                    | Status     |
| --------- | --------------------------- | ---------- |
| macOS 26+ | Apple Containerization      | ✅ All MVP |
| macOS 15- | OrbStack ($8/mo)            | ✅ All MVP |
| Linux     | Podman + nftables + SELinux | ✅ All MVP |

### Nix Flake Verdict

- **Option A (Full Nix)**: NOT feasible - cannot handle privileged operations
- **Option B (Nix + rcc)**: RECOMMENDED - Nix for distribution, rcc for isolation
- **Critical**: Nix network isolation is NOT AI-untouchable

---

## Reading Order

1. Start with [decision/final-recommendation.md](decision/final-recommendation.md) for architecture overview
2. Read platform-specific report ([macos](research/macos/final-report.md) or [linux](research/linux/final-report.md))
3. Review [nix-flake/summary.md](research/nix-flake/summary.md) for distribution strategy
4. Dive into specific topics as needed

---

## Research Team

| Role           | Session  | Contribution                         |
| -------------- | -------- | ------------------------------------ |
| Advisor        | a5bc50aa | Architecture decisions, coordination |
| Leader (macOS) | d9ee4e3d | macOS research coordination          |
| Leader (Linux) | 514e02ad | Linux research coordination          |
| Leader (Nix)   | 857a9432 | Nix Flake research coordination      |
