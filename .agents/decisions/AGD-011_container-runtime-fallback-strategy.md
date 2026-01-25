---
title: "Container Runtime Fallback Strategy"
description: "Define fallback behavior when Apple container CLI is not installed vs not ready"
tags: macos, cli, config
updated_by: AGD-020
---

## Context

alca needs to decide how to handle different states of Apple Containerization setup:
1. `container` CLI not installed at all
2. `container` CLI installed but not ready (system not started, kernel not configured)

Different states imply different user intent, requiring different handling strategies.

## Decision

**Fallback to Docker only when `container` CLI is not installed.**

| State | User Intent | alca Behavior |
|-------|-------------|---------------|
| `container` not installed | User hasn't chosen Apple Containerization | Silent fallback to Docker |
| `container` installed, not ready | User chose Apple Containerization but setup incomplete | Error with setup guidance |

### Rationale

1. **Installation is an intentional act** - If user installed `container` CLI, they want to use it
2. **Avoid silent confusion** - Don't bypass user's explicit choice by falling back to Docker
3. **One-time setup** - Guiding user to complete setup solves the problem permanently

### Implementation

```go
func SelectRuntime(config Config) (Runtime, error) {
    // If user explicitly configured docker, use it
    if config.Runtime == "docker" {
        return NewDockerRuntime()
    }

    // Auto-detect mode
    if _, err := exec.LookPath("container"); err != nil {
        // Not installed → silent fallback to Docker
        return NewDockerRuntime()
    }

    if !isContainerReady() {
        // Installed but not ready → error with guidance
        return nil, fmt.Errorf(
            "Apple container CLI installed but not ready.\n" +
            "Run: container system start\n" +
            "Or set runtime = \"docker\" in .alca.toml")
    }

    return NewContainerRuntime()
}
```

### Override via Config

Users can force Docker by setting `runtime = "docker"` in `.alca.toml` (see AGD-012).

## Consequences

- Users without `container` CLI get seamless Docker experience
- Users who installed `container` CLI get guided to complete setup
- No silent fallback that might confuse users about which runtime is being used
- Config-based override available for users who want Docker despite having `container` installed

## References

- `.agents/references/apple-containerization-setup.md` - Setup flow documentation
- AGD-001: macOS Isolation Solution
- AGD-012: Runtime Config Setting
