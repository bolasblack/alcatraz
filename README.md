# Alcatraz

Run code agents unrestricted, but fearlessly.

Alcatraz wraps your AI code agent in a sandbox with file, network, and capability isolation — so you can skip the guardrails without the danger.

## Why Alcatraz?

AI code agents work best with full access — but running `--dangerously-skip-permissions` on your machine means trusting an LLM with your SSH keys, cloud credentials, and local network. That's a lot of trust for a tool that hallucinates.

Alcatraz gives agents the freedom they need inside a container they can't escape:

- **File isolation** — Agents only see your project directory. Hide secrets and credentials with exclude patterns.
- **Selective file mounting** — Exclude patterns (`workdir_exclude` or per-mount `exclude`) keep `.env`, keys, and other sensitive files invisible inside the container, powered by [Mutagen](https://mutagen.io/) sync.
- **Network isolation** — Zero LAN access by default. Automated nftables firewall blocks agents from reaching local services, databases, or other machines on your network. Works on macOS (Docker Desktop, OrbStack) and Linux.
- **Zero-config sandbox** — `alca init && alca up`. Auto-detects Docker, OrbStack, or Podman.

Works with Claude Code, Codex, Gemini CLI, and any CLI-based AI agent.

## How Is This Different?

|                              | Alcatraz                   | Dev Containers       | Cloud Sandboxes (E2B, Daytona) | Distrobox / Devbox |
| ---------------------------- | -------------------------- | -------------------- | ------------------------------ | ------------------ |
| **Runs locally**             | Yes                        | Yes                  | No (cloud)                     | Yes                |
| **Network isolation**        | Automated nftables         | Manual scripts       | Cloud-managed                  | None / opt-in      |
| **Selective file exclusion** | `workdir_exclude` patterns | No                   | No                             | No                 |
| **AI agent focus**           | Yes                        | No (general-purpose) | Yes                            | No                 |
| **Multi-runtime**            | Docker, OrbStack, Podman   | Docker only          | Proprietary                    | Podman/Docker      |

**Why not just Docker?** Docker provides the building blocks, but isolating LAN access requires writing nftables rules inside the Linux VM on macOS — and keeping them in sync with container lifecycle. Alcatraz automates this.

**Why not Dev Containers?** Dev Containers have no built-in network isolation. Claude Code's devcontainer implementation adds custom iptables scripts as a workaround, but there's no pattern-based file exclusion (`workdir_exclude`) and no automated firewall management.

**Why not cloud sandboxes?** E2B, Daytona, and Modal are cloud-first. If you want your code and data to stay on your machine, they don't apply.

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

### Homebrew

```bash
brew tap bolasblack/alcatraz
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

## Configuration

Run `alca init` to generate `.alca.toml` in your project root. Alcatraz mounts your project directory into the container and isolates everything else.

```toml
image = "nixos/nix"
workdir = "/workspace"
runtime = "auto"  # auto, docker, podman
mounts = ["/Users/<UserName>/.claude/:/root/.claude/"]

# Hide sensitive files from the agent
workdir_exclude = ["**/.env", "**/.env.*", "**/secrets/", "**/*.key"]

[commands]
up = "sleep infinity"
enter = "nix develop"

[resources]
memory = "4g"
cpus = 2
```

| Field                | Description                                                                                             |
| -------------------- | ------------------------------------------------------------------------------------------------------- |
| `image`              | Container image                                                                                         |
| `workdir`            | Working directory inside container                                                                      |
| `workdir_exclude`    | Patterns to hide from the container ([details](docs/config/fields.md#workdir_exclude))                  |
| `runtime`            | Container runtime (`auto`, `docker`, `podman`)                                                          |
| `mounts`             | Volume mounts; supports `exclude` patterns in extended format ([details](docs/config/fields.md#mounts)) |
| `commands.up`        | Command to keep container running                                                                       |
| `commands.enter`     | Command to run on `alca run`                                                                            |
| `resources.memory`   | Memory limit (e.g. `4g`, `512m`)                                                                        |
| `resources.cpus`     | Number of CPUs to allocate                                                                              |
| `network.lan-access` | LAN access for containers (`["*"]` to allow all)                                                        |
| `extends`/`includes` | Compose config files ([details](docs/config/extends-includes.md))                                       |

See the [full configuration reference](docs/config/fields.md) for all options.

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
| `experimental sync check`                   | Check for sync conflicts                    |
| `experimental sync resolve`                 | Interactively resolve sync conflicts        |

## Network Isolation

On macOS, container traffic is proxied through a userspace process (`com.docker.backend` for Docker Desktop, OrbStack's network stack for OrbStack). macOS-level firewalls like `pf` cannot intercept this traffic. Network isolation must happen **inside the Linux VM** using nftables.

Alcatraz handles this automatically:

1. On `alca up`, per-container nftables rules are written and loaded
2. On macOS, a helper container (`alcatraz-network-helper`) applies rules inside the VM via `nsenter`
3. On Linux, native nftables is used directly
4. On `alca down`, rules are cleaned up

No manual firewall configuration required. See the [network documentation](docs/config/network.md) for details.

## Supported Runtimes

| Runtime                           | Platform     | Notes                                       |
| --------------------------------- | ------------ | ------------------------------------------- |
| Docker                            | Linux, macOS | Via Docker Desktop or Docker Engine         |
| [OrbStack](https://orbstack.dev/) | macOS        | Via `docker` command; recommended for macOS |
| Podman                            | Linux        | Auto-detected on Linux                      |

Runtime is auto-detected by default. Set `runtime` in config to override.

## License

MIT
