---
title: Alcatraz Documentation
type: docs
bodyClass: page-home
---

# Alcatraz

Lightweight container isolation for AI coding assistants.

Alcatraz (`alca`) wraps AI agent processes in configurable containers, providing enhanced security without complex setup. It auto-detects the best available runtime and manages container lifecycle with simple commands.

## Features

- **Zero-config startup** — `alca init && alca up` gets you running
- **Auto-detect runtime** — Chooses Docker, OrbStack, or Podman (Linux) automatically
- **Nix/Flake integration** — Automatically activates `nix develop` environments
- **Network isolation** — Restrict container network access with nftables-based firewall rules (see [Configuration]({{< relref "config" >}}))

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

{{% /columns %}}
