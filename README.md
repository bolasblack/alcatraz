# Alcatraz

Lightweight container isolation for AI coding assistants. Wraps AI agent processes in configurable containers for enhanced security.

## Installation

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

## Quick Start

```bash
# Initialize configuration
alca init

# Start container
alca up

# Run commands in container
alca run make build
alca run npm test

# Stop container
alca down
```

## Commands

| Command     | Description                                 |
| ----------- | ------------------------------------------- |
| `init`      | Initialize `.alca.toml` configuration       |
| `up`        | Start container (use `-f` to force rebuild) |
| `down`      | Stop and remove container                   |
| `run <cmd>` | Execute command in container                |
| `status`    | Show container status and config drift      |
| `list`      | List all Alcatraz containers                |
| `cleanup`   | Remove orphaned containers                  |

## Configuration

Create `.alca.toml` in your project root:

```toml
image = "nixos/nix"
workdir = "/workspace"
runtime = "auto"  # auto, docker
mounts = [".:/workspace"]

[commands]
up = "sleep infinity"
enter = "nix develop"

[resources]
memory = "4g"
cpus = 2
```

| Field              | Description                                     |
| ------------------ | ----------------------------------------------- |
| `image`            | Container image                                 |
| `workdir`          | Working directory inside container              |
| `runtime`          | Container runtime (`auto`, `docker`)            |
| `mounts`           | Volume mounts (default: current dir to workdir) |
| `commands.up`      | Command to keep container running               |
| `commands.enter`   | Command to run on `alca run`                    |
| `resources.memory` | Memory limit (e.g. `4g`, `512m`)                |
| `resources.cpus`   | Number of CPUs to allocate                      |

## Supported Runtimes

| Runtime                | Platform     | Notes                            |
| ---------------------- | ------------ | -------------------------------- |
| Docker                 | Linux, macOS | Recommended                      |
| Podman                 | Linux        | Auto-detected on Linux           |
| Apple Containerization | macOS 26+    | Native, requires `container` CLI |

Runtime is auto-detected by default. Set `runtime` in config to override.

## License

MIT
