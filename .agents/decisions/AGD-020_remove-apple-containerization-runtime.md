---
title: "Remove Apple Containerization Runtime"
description: "Temporarily remove Apple Containerization support due to stability issues"
tags: macos, runtime
obsoletes: AGD-001
---

## Context
- Date: 2026-01-25
- macOS: 26.2 (Build 25C56)
- Apple Containerization has shown stability issues after extensive use
- Need reliable container runtime for development workflow

## Decision
Temporarily remove Apple Containerization runtime support:
1. Remove `internal/runtime/apple_containerization.go`
2. Remove "apple-containerization" from RuntimeType options
3. Update documentation to remove Apple references
4. Recommend OrbStack for macOS users (supports automatic memory shrinking, unlike colima/lima)

## Rationale
- Extensive testing revealed instability
- Future consideration when Apple Containerization matures
- OrbStack provides better developer experience on macOS

## Consequences
- Only "docker" and "auto" runtime options remain
- macOS users should use Docker Desktop or OrbStack
- Existing configs with `runtime = "apple-containerization"` will fail with clear error
