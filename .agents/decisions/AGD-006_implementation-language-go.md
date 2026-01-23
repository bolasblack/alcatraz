---
title: "Implementation Language: Go"
description: "Choose Go as the implementation language for RCC CLI tool"
tags: tooling
---

## Context

RCC is a CLI tool that provides isolated execution environments for Claude Code. This document records the language selection process and all considerations evaluated.

### Functional Requirements

1. **Cross-platform support** - macOS (Darwin) and Linux
2. **Container management** - Execute `container` (Apple), `docker`/OrbStack, `podman`
3. **Firewall management** - Execute `pfctl` (macOS), `nft` (Linux)
4. **Unix socket communication** - ccc-statusd daemon integration
5. **Single binary distribution** - Package via Nix Flake
6. **Reproducible builds** - Critical for Nix ecosystem

### Required Language Features

Based on requirements analysis:

| Feature | Necessity | Notes |
|---------|-----------|-------|
| Cross-platform compilation | Required | macOS + Linux |
| System command execution | Required | Container and firewall CLI tools |
| Process management | Required | Start/manage container processes |
| Unix socket support | Required | ccc-statusd communication |
| CLI framework | Required | User-friendly command interface |
| Single binary output | Required | Simplify Nix Flake distribution |
| Memory safety | Recommended | Security-critical application |
| Async I/O | Optional | Multi-container communication |

## Candidates Evaluated

### Initial Candidates

| Language | Binary Size | Nix Support | Learning Curve |
|----------|-------------|-------------|----------------|
| Go | 10-20MB | Excellent | Easy |
| Rust | 5-15MB | Fragmented | Steep |
| Deno/TypeScript | 40-80MB | Problematic | Easy |

### Firewall Library vs CLI Consideration

Original `final-recommendation.md` suggested using `pfctl-rs` (macOS) and `nftables-rs` (Linux) for firewall management.

**Finding**: Both `pfctl` and `nft` have complete CLI interfaces:

```bash
# macOS
pfctl -a rcc -f /tmp/rcc-rules.conf

# Linux
nft -f /tmp/rcc-rules.nft
```

**Conclusion**: Firewall libraries are optional. All languages can use exec calls, removing Rust's library advantage.

### Deno/TypeScript Deep Dive

**Binary Size** (from Deno blog and GitHub discussions):
- Hello world: ~58-70MB (platform dependent)
- macOS ARM: ~70MB
- Linux stripped: ~38MB
- Windows: ~80MB

Reason: `deno compile` embeds the entire Deno runtime (`denort`) into the output binary, regardless of application code size.

**Native Dependency Support** (Deno 2.3, May 2025):
- FFI (Foreign Function Interface) supported
- Node native add-ons supported
- Can compile to single binary including native dependencies

**Fatal Flaw - Non-reproducible Builds**:

Issue: [denoland/deno#27284](https://github.com/denoland/deno/issues/27284)

`deno compile` output contains `sourceMappingURL` with base64-encoded JSON that includes the build directory path (PWD). Different machines produce different binary hashes.

This breaks Nix's core value proposition: reproducible builds.

**Deno Verdict**: Functionally capable, but disqualified due to Nix incompatibility.

### Apple Containerization Research

**Question**: Does macOS 26+ Apple Containerization require Swift?

**Research Result**: Apple provides two layers:

| Component | Type | Usage |
|-----------|------|-------|
| [Container CLI](https://github.com/apple/container) | Command-line tool | `container run`, `container system start` |
| [Containerization](https://github.com/apple/containerization) | Swift Package | Swift API for advanced use |

Container CLI usage:
```bash
# Install
curl -LO https://github.com/apple/container/releases/download/0.1.0/container-0.1.0-installer-signed.pkg
sudo installer -pkg container-0.1.0-installer-signed.pkg -target /

# Run (Docker-like syntax)
container run -it --rm alpine sh
```

**Conclusion**: No Swift required. All languages can use exec calls to `container` CLI.

### Nix Flake Packaging Comparison

| Language | Nix Builder | Maturity | Complexity |
|----------|-------------|----------|------------|
| Go | `buildGoModule` | Stable, official | Simple |
| Rust | crane/naersk/crate2nix/etc. | 8+ competing solutions | Decision fatigue |
| Deno | deno2nix | Experimental | Reproducibility broken |

**Go**: Single official solution in nixpkgs, well-documented, widely used.

**Rust**: From [devenv blog](https://devenv.sh/blog/2025/08/22/closing-the-nix-gap-from-environments-to-packaged-applications-for-rust/): "At the time of writing, there are now no less than 8 different solutions for building Rust code with Nix. This fragmentation is a key source of difficulty."

**Deno**: [deno2nix](https://github.com/SnO2WMaN/deno2nix) exists but cannot solve the PWD hash issue.

### Additional Considerations Discussed

**1. Privilege Escalation Model**

`sudo rcc init` requires root, but `rcc claude` should be rootless. Options:
- Single binary + setuid helper
- Two separate binaries
- polkit/sudo for elevation

This is implementation detail, not language-dependent.

**2. Relationship with Claude Code**

Claude Code itself is TypeScript. Considerations:
- **Independent tool**: Language doesn't matter
- **Potential integration**: TypeScript might share code/types

Current assessment: RCC is independent. No integration planned.

**3. Maintainer Ecosystem**

- Personal project: Choose most familiar language
- Open source community: Go/Rust have active communities
- Anthropic official: Might prefer TypeScript

Current assessment: Treated as independent project.

## Decision

Use **Go** as the implementation language for RCC.

### Rationale Summary

1. **Nix packaging**: Simplest and most mature (`buildGoModule`)
2. **Reproducible builds**: Guaranteed, unlike Deno
3. **No library dependencies**: All operations via exec (pfctl, nft, container, podman, docker)
4. **Cross-compilation**: Single command (`GOOS=linux GOARCH=amd64 go build`)
5. **Development velocity**: Fast compile times, easy debugging
6. **Binary size acceptable**: 10-20MB is reasonable for CLI tool

## Consequences

### Positive

- Simplest Nix Flake integration via `buildGoModule`
- Fast development iteration (quick compile times)
- Easy cross-platform compilation
- Mature CLI ecosystem (cobra, viper)
- Reproducible builds guaranteed
- Single static binary, no runtime dependencies
- Large community, easy to find contributors

### Negative

- Larger binary than Rust (~2x, 10-20MB vs 5-15MB)
- No compile-time memory safety guarantees (unlike Rust)
- GC pauses (negligible for CLI tool)
- Verbose error handling (`if err != nil`)

### Neutral

- All container/firewall operations via exec (same for all candidates)
- Team familiarity equal across Go/Rust/Deno
- Privilege escalation model independent of language choice

## References

- [Deno 2.3 Release Notes](https://deno.com/blog/v2.3)
- [Deno PWD Hash Issue #27284](https://github.com/denoland/deno/issues/27284)
- [Apple Container CLI](https://github.com/apple/container)
- [Apple Containerization Framework](https://github.com/apple/containerization)
- [devenv Rust Nix Blog](https://devenv.sh/blog/2025/08/22/closing-the-nix-gap-from-environments-to-packaged-applications-for-rust/)
- [deno2nix](https://github.com/SnO2WMaN/deno2nix)
