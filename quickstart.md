---
title: Quickstart
weight: 1
---

# Quickstart Guide

Set up a secure sandbox for your AI code agent in under 5 minutes. After this guide, your agent will have full autonomy inside the container while your host system stays protected.

## Installation

### Homebrew

```bash
brew tap bolasblack/alcatraz https://github.com/bolasblack/alcatraz
brew install alca
```

### Go

```bash
go install github.com/bolasblack/alcatraz/cmd/alca@latest
```

### Nix

```bash
nix profile install github:bolasblack/alcatraz
```

Or use in flake:

```nix
{
  inputs.alcatraz.url = "github:bolasblack/alcatraz";
}
```

### mise

Add to your project `.mise.toml`:

```toml
[tools]
"go:github.com/bolasblack/alcatraz/cmd/alca" = "latest"
```

Or install globally:

```bash
mise use -g "go:github.com/bolasblack/alcatraz/cmd/alca@latest"
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

You'll be prompted to select a template:

```
? Select a template:
  > Nix - NixOS-based development environment
    Debian - Debian-based environment with mise
```

This creates a `.alca.toml` tailored to the selected preset. For example, the **Nix** preset generates:

```toml
image = "nixos/nix"

[commands]
# prebuild, to reduce the time costs on enter
up = "[ -f flake.nix ] && exec nix develop --profile /nix/var/nix/profiles/devshell --command true"
enter = "[ -f flake.nix ] && exec nix develop --profile /nix/var/nix/profiles/devshell --command"

[envs]
NIX_CONFIG = "extra-experimental-features = nix-command flakes"
NIXPKGS_ALLOW_UNFREE = "1"
```

### Step 2: Configure (Optional)

Edit `.alca.toml` to customize your setup. For example, add resource limits:

```toml
[resources]
memory = "8g"
cpus = 4
```

Or hide sensitive files from the agent:

```toml
# Shorthand: exclude from workdir mount
workdir_exclude = ["**/.env", "**/.env.*", "**/secrets/"]

# Or use extended mount format for any mount
[[mounts]]
source = "~/.config/myapp"
target = "/root/.config/myapp"
exclude = ["**/*.key", "**/credentials.json"]
```

Excluded files are invisible inside the container.

Common configuration options:

| Field              | Description                        |
| ------------------ | ---------------------------------- |
| `image`            | Container image to use             |
| `workdir`          | Working directory inside container |
| `runtime`          | `auto` or `docker`                 |
| `commands.up`      | Setup command run on `alca up`     |
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

### Step 6: Run Your Agent

Run your AI code agent inside the isolated environment:

```bash
# Claude Code with full permissions — safe inside the container
alca run claude --dangerously-skip-permissions

# Or any other agent
alca run codex
alca run gemini
```

Your agent has full access inside the container but cannot affect your host system.

### Step 7: Stop the Container

```bash
alca down
```

Output:

```
Using runtime: docker
Container stopped successfully.
```

## Nix Workflow

Alcatraz does not have built-in Nix or flake integration. Instead, the **Nix preset** from `alca init` generates a `.alca.toml` that configures Nix workflows through standard container commands.

### How It Works

The Nix preset configures two commands:

- **`commands.up`** — runs on `alca up`, pre-builds the flake devshell so subsequent enters are fast
- **`commands.enter`** — runs on `alca run`, drops into `nix develop` if a `flake.nix` is present

Both use a shell conditional (`[ -f flake.nix ] && ...`), so they are no-ops if your project doesn't have a flake.

### Example

If your project has a `flake.nix` with Go and Node.js in its devshell:

```bash
alca up                    # Pre-builds the Nix devshell
alca run go version        # Uses Go from your flake
alca run node --version    # Uses Node.js from your flake
```

### Customizing

The preset is just a starting point. You can edit `commands.up` and `commands.enter` in `.alca.toml` to suit your workflow — for example, removing the flake conditional if you always use Nix, or switching to a different base image.

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
- Review the [Configuration Reference]({{< relref "config" >}}) for all options
- If using `workdir_exclude` or `mounts.exclude`, see [Sync Conflicts]({{< relref "sync-conflicts" >}}) for conflict detection and resolution
