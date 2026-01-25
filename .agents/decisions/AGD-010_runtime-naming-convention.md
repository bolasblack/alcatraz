---
title: "Runtime Naming Convention"
description: "Naming conventions for container runtime implementations"
tags: naming, macos
updated_by: AGD-020
---

## Context

The project supports multiple container runtimes (Docker, Podman, Apple's containerization solution). We need consistent naming conventions to avoid confusion between:

1. **System identifiers** (OS platform values like `darwin`, `linux`)
2. **Runtime solution names** (user-facing names for container technologies)

### Apple's Container Technology

Apple provides two related projects:

| Project | Description | Links |
|---------|-------------|-------|
| **Containerization** | Swift package/library providing APIs for running Linux containers on macOS using Virtualization.framework | [opensource.apple.com](https://opensource.apple.com/projects/containerization/) · [GitHub](https://github.com/apple/containerization) |
| **Container** | CLI tool (`container`) built on Containerization for creating and running Linux containers as lightweight VMs | [opensource.apple.com](https://opensource.apple.com/projects/container/) · [GitHub](https://github.com/apple/container) |

## Decision

### Principle: Separate OS from Runtime

- **Darwin** = Operating System identifier (`runtime.GOOS == "darwin"`)
- **AppleContainerization** = Runtime framework/solution

Use the runtime solution name for all runtime-related code, not the OS name.

### System Identifiers

Use lowercase OS identifiers for platform detection only:

| Platform | Identifier |
|----------|------------|
| macOS    | `darwin`   |
| Linux    | `linux`    |

### Code Naming (Internal)

| Element | Convention | Example |
|---------|------------|---------|
| File name | `{runtime_name}.go` | `apple_containerization.go` |
| Type name | `{RuntimeName}` | `AppleContainerization` |
| Constructor | `New{RuntimeName}()` | `NewAppleContainerization()` |
| State type | `{RuntimeName}State` | `AppleContainerizationState` |
| State constants | `{RuntimeName}State{Value}` | `AppleContainerizationStateReady` |

### Config Values

| Runtime | Config Value |
|---------|--------------|
| Auto-detect | `runtime = "auto"` |
| Docker | `runtime = "docker"` |
| Apple Containerization | `runtime = "apple-containerization"` |

### User-Facing Names (External)

| Runtime | Display Name |
|---------|-------------|
| macOS native | "Apple Containerization" |
| Docker | "Docker" |
| Podman | "Podman" |

### Implementation

```go
// apple_containerization.go
type AppleContainerization struct{}

func (ac *AppleContainerization) Name() string {
    return "Apple Containerization"
}

// SetupState returns the current state
func (ac *AppleContainerization) SetupState() AppleContainerizationState {
    if runtime.GOOS != "darwin" {  // OS check uses "darwin"
        return AppleContainerizationStateNotOnMacOS
    }
    // ...
}
```

## Consequences

### Positive

- Clear separation: `darwin` for OS detection, `AppleContainerization` for runtime
- Type names clearly indicate what they represent (a container runtime)
- Config values are human-readable (`apple-containerization` not `darwin`)
- Consistent with other runtime naming (Docker, Podman)

### Negative

- Longer type names (`AppleContainerization` vs `Darwin`)
- More verbose state constant names

### Migration (Completed)

Renamed in `internal/runtime/`:
- `darwin.go` → `apple_containerization.go`
- `type Darwin` → `type AppleContainerization`
- `NewDarwin()` → `NewAppleContainerization()`
- `DarwinState` → `AppleContainerizationState`
- `DarwinStateReady` → `AppleContainerizationStateReady` (etc.)

Added in `internal/config/`:
- `RuntimeAppleContainerization = "apple-containerization"`
