# Running Integration Tests on macOS

This guide covers running alcatraz integration tests on macOS. You have two options: run tests inside an OrbStack NixOS VM (Linux environment) or run them natively on macOS with a Docker-compatible runtime.

## Prerequisites

- **Project cloned locally** — Your local alcatraz repository
- **Nix installed** — Required for both paths (can be via host or OrbStack NixOS VM)

### For Linux Tests (OrbStack NixOS VM)

- **OrbStack installed** — Download from https://orbstack.dev/
- **OrbStack NixOS VM** — Create or configure a NixOS VM in OrbStack
- **Docker enabled in NixOS VM** — See [OrbStack NixOS VM Setup](#orbstack-nixos-vm-setup) below

### For macOS Tests (Native)

- **Docker-compatible runtime** — One of:
  - Docker Desktop
  - OrbStack (macOS native mode)
  - or another Docker-compatible container runtime (Not tested)

## Running Tests

Choose the appropriate section for your environment.

### Linux Tests (via OrbStack NixOS VM)

This approach runs tests inside OrbStack's NixOS VM, providing a consistent Linux environment and avoiding platform-specific issues.

**Command:**

```bash
CONTAINER_RUNTIME=docker orb run nix --extra-experimental-features "nix-command flakes" develop .#integration --command bash ./test_integration/run.sh
```

**How it works:**

- **`orb run`** — Executes the following command inside OrbStack's NixOS VM
- **`nix develop .#integration`** — Enters the `integration` development shell, which provisions:
  - `alca` (the alcatraz CLI tool)
  - `mutagen` (for optimized file synchronization)
  - `bash`, `python` (scripting tools)
  - `docker` (container runtime on Linux)
- **`CONTAINER_RUNTIME=docker`** — Explicitly selects Docker as the container runtime
- **`bash ./test_integration/run.sh`** — Runs the test suite

#### OrbStack NixOS VM Setup

If you haven't already configured Docker in your OrbStack NixOS VM, do so now:

1. Open the NixOS VM's `/etc/nixos/configuration.nix`
2. Add these lines to enable Docker:

```nix
virtualisation.docker.enable = true;
networking.nftables.enable = true;
boot.kernelModules = [ "br_netfilter" ];
users.users.<user>.extraGroups = [ "docker" ];

# Optional: add useful tools for debugging inside the VM
environment.systemPackages = with pkgs; [
  vim
];
```

Replace `<user>` with your actual username in the VM.

3. Apply the configuration:

```bash
sudo nixos-rebuild switch
```

After this, Docker will be available for the integration tests.

### macOS Tests (Native)

Run tests natively on macOS with a Docker-compatible container runtime already running on your machine.

**Command:**

```bash
nix --extra-experimental-features "nix-command flakes" develop .#integration --command bash ./test_integration/run.sh
```

**Requirements:**

- Docker-compatible runtime must be running (Docker Desktop, OrbStack, Colima, Lima, etc.)
- The runtime must be accessible via the `docker` CLI
- Nix must be installed on your macOS host

**How it works:**

- **`nix develop .#integration`** — Enters the `integration` development shell on your macOS host
- The shell uses the Docker runtime that's already running on your machine
- **`bash ./test_integration/run.sh`** — Runs the test suite against the host's Docker runtime

## Docker Daemon Auto-Start

The test runner (`test_integration/run.sh`) automatically manages the Docker daemon. You don't need to start it manually.

### How It Works

1. The test script checks `docker info` to see if the daemon is running
2. If the daemon is not running, it starts `dockerd` in the background
3. The daemon is automatically stopped on exit via a shell trap
4. No manual intervention needed — the script handles lifecycle completely

This automatic management ensures clean test runs and prevents leftover daemon processes.

## Tips

- Run the test command from your project root
- The NixOS VM isolates dependencies — your host Nix configuration won't interfere
- Test output is streamed to your terminal in real time
- If you modify test code, re-run the full command — the devShell is fresh each time
