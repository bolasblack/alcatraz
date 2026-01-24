---
title: Quickstart
weight: 10
---

# Quickstart Guide

Get started with Alcatraz in under 5 minutes.

## Installation

### Go Install

```bash
go install github.com/bolasblack/alcatraz/cmd/alca@latest
```

### Nix

```bash
nix profile install github:bolasblack/alcatraz
```

### Build from Source

```bash
git clone https://github.com/bolasblack/alcatraz.git
cd alcatraz
make build
```

The binary is created at `out/bin/alca`. Add it to your PATH:

```bash
export PATH="$PWD/out/bin:$PATH"
```

## Basic Commands

| Command          | Description                           |
| ---------------- | ------------------------------------- |
| `alca init`      | Create `.alca.toml` configuration     |
| `alca up`        | Start container                       |
| `alca run <cmd>` | Execute command in container          |
| `alca down`      | Stop and remove container             |
| `alca status`    | Show container status and config info |

## Your First Container

### Step 1: Initialize

Navigate to your project directory and initialize Alcatraz:

```bash
cd my-project
alca init
```

Output:

```
Created .alca.toml
Edit this file to customize your container settings.
```

This creates a default `.alca.toml`:

```toml
image = "nixos/nix"
workdir = "/workspace"
runtime = "auto"

[commands]
enter = "[ -f flake.nix ] && exec nix develop"
```

### Step 2: Configure (Optional)

Edit `.alca.toml` to customize your setup:

```toml
image = "nixos/nix"
workdir = "/workspace"
runtime = "auto"

[commands]
enter = "[ -f flake.nix ] && exec nix develop"

[resources]
memory = "8g"
cpus = 4
```

Configuration options:

| Field              | Description                        |
| ------------------ | ---------------------------------- |
| `image`            | Container image to use             |
| `workdir`          | Working directory inside container |
| `runtime`          | `auto` or `docker`                 |
| `commands.enter`   | Shell setup command for `alca run` |
| `resources.memory` | Memory limit (e.g., `4g`, `512m`)  |
| `resources.cpus`   | CPU limit (e.g., `4`)              |

### Step 3: Start the Container

```bash
alca up
```

Output:

```
→ Loading config from .alca.toml
→ Detecting runtime...
→ Detected runtime: docker
→ Created new state file: .alca.state.json
✓ Environment ready
```

### Step 4: Run Commands

Execute commands inside the container:

```bash
alca run ls -la
alca run make build
alca run npm test
```

Your project directory is mounted at `/workspace` by default.

### Step 5: Check Status

```bash
alca status
```

Output:

```
Status: Initialized
Config: /path/to/my-project/.alca.toml

Runtime: docker

Project ID: my-project-abc123

Container: Running
  ID:    container-id
  Name:  alca-my-project
  Image: nixos/nix
  Started: 2024-01-15 10:30:00

Run 'alca run <command>' to execute commands.
```

### Step 6: Stop the Container

```bash
alca down
```

Output:

```
Using runtime: docker
Container stopped successfully.
```

## Nix/Flake Integration

Alcatraz automatically detects `flake.nix` in your project and integrates with `nix develop`.

### How It Works

When you run commands with `alca run`:

1. Alcatraz checks if `flake.nix` exists in your project root
2. If found, commands are wrapped with `nix develop --command`
3. Your flake's development environment is automatically activated

### Example with Flake

Given a `flake.nix`:

```nix
{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { nixpkgs, ... }:
    let pkgs = nixpkgs.legacyPackages.x86_64-linux;
    in {
      devShells.x86_64-linux.default = pkgs.mkShell {
        packages = [ pkgs.go pkgs.nodejs ];
      };
    };
}
```

Commands automatically have access to `go` and `nodejs`:

```bash
alca run go version   # Uses Go from your flake
alca run node --version
```

### Default Configuration

The default `commands.enter` setting handles flake detection:

```toml
[commands]
enter = "[ -f flake.nix ] && exec nix develop"
```

## Configuration Drift Detection

Alcatraz tracks configuration changes and warns when the running container doesn't match your config.

```bash
# After modifying .alca.toml
alca status
```

Output:

```
⚠️  Configuration drift detected:
  Image: nixos/nix → ubuntu:latest
  Resources.memory: 4g → 8g

Run 'alca up -f' to rebuild with new configuration.
```

To apply changes:

```bash
alca up -f    # Force rebuild
```

Or during normal `alca up`, you'll be prompted:

```
Configuration has changed since last container creation:
  Image: nixos/nix → ubuntu:latest
Rebuild container with new configuration? [y/N]
```

## Next Steps

- See `alca --help` for all available commands
- Check `alca <command> --help` for command-specific options
- Review the [Configuration Reference](config.md) for all options
