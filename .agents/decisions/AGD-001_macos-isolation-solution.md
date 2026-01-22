---
title: "macOS Isolation Solution"
description: "Choose Apple Containerization (macOS 26+) or OrbStack (macOS 15-) for Claude Code isolation"
tags: macos, file-isolation, network-isolation
---

## Context

Need to provide AI-untouchable isolation for Claude Code on macOS, with requirements:
- File isolation (AI can only access project directory)
- Network isolation (block RFC1918, configurable whitelist)
- Memory auto-release (use 1G, occupy only 1G)

## Decision

**Tier 1: Apple Containerization (macOS 26+ Tahoe)**
- Native Apple solution, open source at github.com/apple/container
- Optimized for Apple Silicon
- Memory auto-release confirmed in documentation
- Network isolation via macOS pf (host-level, AI-untouchable)
- Free

**Tier 2: OrbStack (macOS 15 and below)**
- Only solution with verified memory balloon auto-release
- Commercial ($8/month)
- Excellent Docker compatibility

## Consequences

- macOS 26+ users get free, native solution
- macOS 15- users need commercial OrbStack or accept no memory auto-release (Colima)
- Network isolation relies on macOS pf firewall anchors
- Social engineering remains universal limitation (user education required)

## References

- `docs/research/macos/final-report.md`
- `docs/research/macos/network-isolation.md`
- `docs/research/macos/memory-management.md`
