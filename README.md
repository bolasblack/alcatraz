# Alcatraz

Run code agents unrestricted, but fearlessly.

Alcatraz wraps your AI code agent in a sandbox with file, network, and capability isolation — so you can skip the guardrails without the danger.

## Why Alcatraz?

AI code agents work best with full access — but running `--dangerously-skip-permissions` on your machine means trusting an LLM with your SSH keys, cloud credentials, and local network. That's a lot of trust for a tool that hallucinates.

Alcatraz gives agents the freedom they need inside a container they can't escape:

- **File isolation** — Agents only see your project directory. Hide secrets and credentials with exclude patterns.
- **Network isolation** — Zero LAN access by default. nftables firewall blocks agents from reaching local services or exfiltrating data.
- **Zero-config sandbox** — `alca init && alca up`. Auto-detects Docker, OrbStack, or Podman.

Works with Claude Code, Codex, Gemini CLI, and any CLI-based AI agent.

## Quick Start

```bash
# Initialize configuration
alca init

# Start container
alca up

# Run commands in container
alca run make build
alca run npm test

# Run your AI agent — full permissions, safely sandboxed
alca run claude --dangerously-skip-permissions

# Stop container
alca down
```

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

## Configuration

Run `alca init` to generate `.alca.toml` in your project root. Alcatraz mounts your project directory into the container and isolates everything else.

```toml
image = "nixos/nix"
workdir = "/workspace"
runtime = "auto"  # auto, docker, podman
mounts = ["/Users/<UserName>/.claude/:/root/.claude/"]

[commands]
up = "sleep infinity"
enter = "nix develop"

[resources]
memory = "4g"
cpus = 2
```

| Field                | Description                                      |
| -------------------- | ------------------------------------------------ |
| `image`              | Container image                                  |
| `workdir`            | Working directory inside container               |
| `runtime`            | Container runtime (`auto`, `docker`, `podman`)   |
| `mounts`             | Volume mounts (default: current dir to workdir)  |
| `commands.up`        | Command to keep container running                |
| `commands.enter`     | Command to run on `alca run`                     |
| `resources.memory`   | Memory limit (e.g. `4g`, `512m`)                 |
| `resources.cpus`     | Number of CPUs to allocate                       |
| `network.lan-access` | LAN access for containers (`["*"]` to allow all) |

## Commands

| Command                                     | Description                                 |
| ------------------------------------------- | ------------------------------------------- |
| `init`                                      | Initialize `.alca.toml` configuration       |
| `up`                                        | Start container (use `-f` to force rebuild) |
| `down`                                      | Stop and remove container                   |
| `run <cmd>`                                 | Execute command in container                |
| `status`                                    | Show container status and config drift      |
| `list`                                      | List all Alcatraz containers                |
| `cleanup`                                   | Remove orphaned containers                  |
| `network-helper install\|uninstall\|status` | Manage network isolation helper             |

## Supported Runtimes

| Runtime                           | Platform     | Notes                                       |
| --------------------------------- | ------------ | ------------------------------------------- |
| Docker                            | Linux, macOS | Via Docker Desktop or Docker Engine         |
| [OrbStack](https://orbstack.dev/) | macOS        | Via `docker` command; recommended for macOS |
| Podman                            | Linux        | Auto-detected on Linux                      |

Runtime is auto-detected by default. Set `runtime` in config to override.

## License

MIT
