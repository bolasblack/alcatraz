---
title: Configuration
weight: 2
bookCollapseSection: true
---

# Configuration Reference

Configure your agent's isolated environment. Each setting controls what the container can access — from filesystem mounts to network rules — so you stay in control while your agent works freely.

## Overview

Alcatraz uses TOML format for configuration. The configuration file should be named `.alca.toml` and placed in your project root.

## Field Reference

| Field                | Type             | Required | Default                                  | Description                                         |
| -------------------- | ---------------- | -------- | ---------------------------------------- | --------------------------------------------------- |
| `extends`            | array            | No       | `[]`                                     | Config files to extend (declaring file wins)        |
| `includes`           | array            | No       | `[]`                                     | Config files to include (included files win)        |
| `image`              | string           | Yes      | -                                        | Container image to use                              |
| `workdir`            | string           | No       | `"/workspace"`                           | Working directory inside container                  |
| `workdir_exclude`    | array            | No       | `[]`                                     | Patterns to exclude from workdir mount              |
| `runtime`            | string           | No       | `"auto"`                                 | Runtime selection mode                              |
| `commands.up`        | string or object | No       | -                                        | Setup command (run once on container creation)      |
| `commands.enter`     | string or object | No       | `"[ -f flake.nix ] && exec nix develop"` | Entry command (run on each shell entry)             |
| `mounts`             | array            | No       | `[]`                                     | Additional mount points                             |
| `resources.memory`   | string           | No       | -                                        | Memory limit (e.g., "4g", "512m")                   |
| `resources.cpus`     | int              | No       | -                                        | CPU limit (e.g., 2, 4)                              |
| `envs`               | table            | No       | See below                                | Environment variables for the container             |
| `network.lan-access` | array            | No       | `[]`                                     | LAN access configuration                            |
| `caps`               | array/table      | No       | See below                                | Container Linux capabilities configuration          |

## Full Example

```toml
# Container image
image = "nixos/nix"

# Working directory inside container
workdir = "/workspace"

# Runtime selection: auto, docker, or podman
runtime = "auto"

# Lifecycle commands
[commands]
up = "nix-channel --update"
enter = "[ -f flake.nix ] && exec nix develop"

# Additional mounts
mounts = [
  "~/.gitconfig:/root/.gitconfig:ro",
  "~/.ssh:/root/.ssh:ro"
]

# Resource limits
[resources]
memory = "16g"
cpus = 8

# Environment variables
[envs]
NIXPKGS_ALLOW_UNFREE = "1"
EDITOR = { value = "${EDITOR}", override_on_enter = true }

# Network configuration
[network]
# lan-access = ["*"]  # Uncomment to allow LAN access (blocked by default)
```
