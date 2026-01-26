---
title: "Alcatraz CLI Design"
description: "CLI command structure, configuration format, and workflow design for the Alcatraz isolation tool"
tags: cli, config
updated_by: AGD-012, AGD-014, AGD-022
---

## Context

The project was originally named "rcc" (Restricted Claude Code), but the scope expanded to a general-purpose isolation environment, not just for Claude Code. A new name and CLI design are needed.

## Decision

### Project Name: `Alcatraz` (command: `alca`)

**Rationale**: Alcatraz (恶魔岛监狱) - the prison island where surrounding water prevents escape. Perfect metaphor for "restricting agent from escaping the boundary". The water is not to keep outsiders away, but to keep inmates in.

- Software name: `Alcatraz` (full name, meaningful)
- Command: `alca` (short, 4 letters, easy to type)

Similar pattern: Kubernetes/kubectl, PostgreSQL/psql

**Rejected alternatives**:
| Name | Rejection Reason |
|------|------------------|
| `rcc` | Too specific to Claude Code |
| `moat` | Moat protects from outside attacks, not restricting inside |
| `iso` | Conflicts with ISO disk images |
| `cage` | Conflicts with cargo (Rust) |
| `box` | Too generic, many naming conflicts |
| `silo` | Already exists in brew |
| `vault` | Conflicts with HashiCorp Vault |

### Command Structure

```
# Stable commands
alca init               # Create .alca.toml configuration file
alca up                 # Start environment (container + future network isolation)
alca down               # Stop environment (cleanup container + future network rules)
alca run <command>      # Run command in isolated environment
alca status             # Show container status

# Experimental commands
alca experimental reload    # Re-apply mounts without rebuilding container
```

**Design decisions**:
- `alca run <command>` instead of `alca claude` - generic, any command can be isolated
- `alca up/down` instead of `setup/teardown` - shorter, familiar from docker-compose
- `alca shell` removed - users can use `alca run bash`
- `alca config` removed - users edit `.alca.toml` directly
- Experimental commands under `alca experimental` prefix with deprecation warning

### Configuration File: `.alca.toml`

```toml
image = "nixos/nix"         # Container image, default: nixos/nix
workdir = "/workspace"      # Mount target + working directory, default: /workspace

[commands]
up = """
nix-channel --update
"""
enter = "[ -f flake.nix ] && exec nix develop"  # Executed before run/shell

mounts = [
  "/host/path:/container/path"
]

# Future
# [network]
# allow = ["github.com", "api.anthropic.com"]
```

**Field decisions**:
- `enter` field name chosen over `prepare`, `activate`, `shell_init`
- `enter` default includes condition check: `[ -f flake.nix ] && exec nix develop`
- `mounts` uses simple string array format `"source:target"` for MVP
- No global config (`~/.config/alca/`) for now, only project-level `.alca.toml`

### Container Workflow

```
alca up      # Create and keep container running
alca run xx  # exec into running container
alca down    # Stop and destroy container
```

- `alca run` and `alca status` reuse container created by `alca up`
- `alca up` auto-creates `.alca.toml` if not exists (with defaults)

### Runtime Auto-Detection

| Platform | Priority |
|----------|----------|
| macOS | Apple Container (macOS 26+) > Docker |
| Linux | Podman > Docker |

Go integration:
- Docker/Podman: mature SDKs available
- Apple Container: may need CLI wrapper (new technology)

### Dynamic Mount (Experimental)

**Problem**: Docker auto-creates directories when source doesn't exist. User prefers to skip or wait.

**Solution**: Use `nsenter + mount` to dynamically add mounts without rebuilding container.

**Implementation phases**:
1. MVP: Skip mounts if source doesn't exist
2. Phase 2: `alca experimental reload` to manually re-apply mounts
3. Phase 3: Daemon watch for auto-mount (if needed)

### Experimental Command Convention

Unstable commands go under `alca experimental`:
- Show warning on execution
- May be removed in future versions

```
$ alca experimental reload
⚠️  This is an experimental command and may be removed in future versions.
Reloading mounts...
```

## Consequences

### Positive
- Simple, git-like CLI with minimal commands
- Generic isolation tool, not tied to Claude Code
- Familiar workflow for docker-compose users
- Clear experimental/stable boundary

### Negative
- No `alca shell` shortcut (minor inconvenience)
- Dynamic mount requires privileged operations (nsenter)
- Apple Container integration may need extra work

### Future Work
- Network isolation (`[network]` section in config)
- Global configuration file
- Daemon for auto-mount watching
