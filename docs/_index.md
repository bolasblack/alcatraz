---
title: Alcatraz Documentation
type: docs
bodyClass: page-home
---

# Alcatraz

Run code agents unrestricted, but fearlessly.

AI code agents like Claude Code, Codex, and Gemini CLI are most powerful when you remove the guardrails. But unrestricted agents can read your secrets, delete files, or make network calls you didn't expect.

Alcatraz wraps your agent in a configurable container with file and network isolation. Your agent gets full access inside the sandbox. Your system stays safe outside it.

## Why Alcatraz?

- **Full agent freedom** — No permission prompts. No guardrails. Maximum productivity inside the container.
- **Network on lockdown** — Zero LAN access by default. Kernel-level nftables firewall.
- **File isolation** — Mount only what you choose. Hide secrets with exclude patterns.
- **Selective file mounting** — `workdir_exclude` and per-mount `exclude` patterns hide secrets and sensitive files from the container, powered by Mutagen sync
- **Zero-config startup** — `alca init && alca up` gets you running
- **Auto-detect runtime** — Chooses Docker, OrbStack, or Podman automatically
- **Nix/Flake integration** — Automatically activates `nix develop` environments

## Quick Start

```bash
# Install
go install github.com/bolasblack/alcatraz/cmd/alca@latest

# Initialize in your project
cd my-project
alca init

# Start container and run commands
alca up
alca run make build
```

## Documentation

{{% columns %}}

- ### [Quickstart]({{< relref "quickstart" >}})

  Get started in under 5 minutes. Installation, basic commands, and your first container.

- ### [Configuration]({{< relref "config" >}})
  Complete `.alca.toml` reference. Images, mounts, commands, and resource limits.

{{% /columns %}}
{{% columns %}}

- ### [Runtimes]({{< relref "runtimes" >}})

  Docker and Podman (Linux-only). Platform differences and troubleshooting.

- ### [Commands]({{< relref "commands" >}})
  CLI reference for all `alca` commands and flags.

{{% /columns %}}
{{% columns %}}

- ### [Network]({{< relref "config/network" >}})

  Network isolation and LAN access. Platform-specific firewall setup and troubleshooting.

- ### [Sync Conflicts]({{< relref "sync-conflicts" >}})

  Detect and resolve file sync conflicts when using selective file mounting.

{{% /columns %}}
