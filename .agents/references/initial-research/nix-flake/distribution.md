# Nix Flake Distribution Mechanism Research

## Executive Summary

This research evaluates Nix Flake as a distribution mechanism for RCC (Restricted Claude Code). The analysis covers two deployment options:

- **Option A (Full Implementation)**: `nix run github:user/rcc` provides complete isolation
- **Option B (Environment Preparation)**: Nix prepares the environment, separate `rcc` binary handles isolation

**Key Finding**: Option B is more feasible. Nix Flake excels at distributing software and dependencies but cannot directly manage privileged operations (firewall rules, container runtime setup) that RCC requires.

---

## 1. How `nix run github:repo` Works

### Mechanism

When you run `nix run github:user/repo#app`:

1. Nix fetches the flake from GitHub (clones the repo)
2. Evaluates `flake.nix` to find the requested output
3. Builds the derivation if not cached
4. Executes the specified program

### What It Can Distribute

| Capability | Supported |
|------------|-----------|
| Pre-compiled binaries | Yes |
| Scripts with dependencies | Yes |
| Configuration files | Yes |
| Complete applications | Yes |
| System services | No (requires NixOS) |
| Privileged operations | No |

### URL Syntax

```bash
nix run github:owner/repo           # Run default app
nix run github:owner/repo#appname   # Run specific app
nix run github:owner/repo/branch    # Specific branch
nix run github:owner/repo?rev=abc   # Specific commit
```

### Flake Outputs: Apps vs Packages

```nix
{
  # Packages: derivations built by `nix build`
  packages.x86_64-linux.default = pkgs.hello;

  # Apps: explicit program execution via `nix run`
  apps.x86_64-linux.default = {
    type = "app";
    program = "${pkgs.hello}/bin/hello";
  };
}
```

**Key difference**: If `apps` output is not defined, `nix run` will attempt to run `packages.<system>.default/bin/<name>` automatically.

**Source**: [NixOS Wiki - Flakes](https://nixos.wiki/wiki/Flakes), [Determinate Systems - nix run](https://determinate.systems/blog/nix-run/)

---

## 2. Cross-Platform Support Assessment

### Supported Systems

Nix Flake supports multiple platforms via the `<system>` placeholder:

- `x86_64-linux` (Linux Intel/AMD)
- `aarch64-linux` (Linux ARM64)
- `x86_64-darwin` (macOS Intel)
- `aarch64-darwin` (macOS Apple Silicon)

### Platform-Specific Binaries

Flakes **natively support** platform-specific builds:

```nix
{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        packages.default = pkgs.stdenv.mkDerivation {
          name = "rcc";
          # Platform-appropriate build
        };
      }
    );
}
```

### Cross-Compilation Limitations

| Scenario | Supported |
|----------|-----------|
| x86_64-darwin ↔ aarch64-darwin | Yes |
| x86_64-linux ↔ aarch64-linux | Yes |
| Linux ↔ Darwin | **No** (macOS not fully open-source) |

**Implication for RCC**: Each platform needs native builds. Cannot cross-compile macOS binaries on Linux CI.

**Source**: [nix.dev - Cross Compilation](https://nix.dev/tutorials/cross-compilation.html), [NixOS & Flakes Book](https://nixos-and-flakes.thiscute.world/development/cross-platform-compilation)

---

## 3. Dependency Management Capabilities

### System Dependencies (podman, docker)

Nix can **declare** and **provide** these dependencies, but with important caveats:

#### On NixOS (Full Control)

```nix
# NixOS configuration
{
  virtualisation.podman.enable = true;
  virtualisation.docker.enable = true;
}
```

#### On Non-NixOS Systems (Limited)

```nix
# Development shell with podman tools
devShells.default = pkgs.mkShell {
  packages = with pkgs; [
    podman
    runc
    conmon
    skopeo
    slirp4netns
    fuse-overlayfs
  ];
};
```

**Critical Limitation**: On non-NixOS systems, rootless podman requires `newuidmap` from shadow package with setuid bit - **Nix cannot provide setuid binaries**.

### Runtime Dependency Injection

For dependencies needed at runtime (not just build time):

```nix
packages.default = pkgs.stdenv.mkDerivation {
  name = "rcc";
  buildInputs = [ pkgs.makeWrapper ];

  postInstall = ''
    wrapProgram $out/bin/rcc \
      --prefix PATH : ${pkgs.lib.makeBinPath [ pkgs.podman pkgs.pf-tools ]}
  '';
};
```

**Source**: [NixOS Wiki - Podman](https://nixos.wiki/wiki/Podman), [NixOS Discourse](https://discourse.nixos.org/t/adding-runtime-dependency-to-flake/27785)

---

## 4. Privileged Operations Handling

### Firewall Setup (pf rules on macOS)

**Nix cannot directly configure firewall rules** because:

1. Firewall configuration requires root/sudo privileges
2. Nix builds are sandboxed and unprivileged
3. pf configuration is system-wide, not per-package

### What Nix CAN Do

```nix
# Provide scripts that user runs with sudo
packages.rcc-firewall-setup = pkgs.writeShellScriptBin "rcc-firewall-setup" ''
  echo "This script requires sudo to configure pf rules"
  sudo pfctl -e
  sudo pfctl -f ${./pf.conf}
'';
```

### What Nix CANNOT Do

- Automatically run privileged commands during `nix run`
- Modify system firewall without user intervention
- Set up containers with network isolation automatically

### NixOS Exception

On NixOS only, declarative firewall configuration is possible:

```nix
# Only works in NixOS configuration.nix
networking.firewall = {
  enable = true;
  allowedTCPPorts = [ 8080 ];
};
```

**Source**: [NixOS Wiki - Firewall](https://nixos.wiki/wiki/Firewall), [NixOS Manual - Firewall](https://nlewo.github.io/nixos-manual-sphinx/configuration/firewall.xml.html)

---

## 5. UX Comparison

### Traditional Package Managers

```bash
# Homebrew (macOS)
brew install rcc
rcc claude

# apt (Debian/Ubuntu)
apt install rcc
rcc claude
```

**Pros**: Simple, familiar, fast
**Cons**: Platform-specific, no reproducibility guarantees

### Nix Flake Approach

```bash
# One-liner (no install needed)
nix run github:user/rcc#claude

# Or with installation
nix profile install github:user/rcc
rcc claude
```

**Pros**:
- No installation required for `nix run`
- Reproducible (pinned versions)
- Cross-platform with single command
- Easy rollback

**Cons**:
- Requires Nix installed first (significant barrier)
- Steeper learning curve
- First run may be slow (building from source)
- Some users report 4+ hour setup times

### Detailed Comparison

| Aspect | brew/apt | nix run |
|--------|----------|---------|
| First-time setup | Minutes | 30min-4hrs |
| Installing packages | Seconds | Seconds-minutes |
| Running without install | Not possible | Supported |
| Reproducibility | Low | High |
| Cross-platform | No | Yes |
| Learning curve | Low | High |
| Community support | High | Medium |

**Source**: [Better Stack - Homebrew vs Nix](https://betterstack.com/community/guides/linux/homebrew-vs-nix/), [Willy's Blog](https://woile.dev/posts/nix-journey-part-2-replacing-apt-and-brew/)

---

## 6. Example flake.nix for RCC-like Tool

### Option A: Full Implementation Attempt

This shows why Option A is **not fully feasible**:

```nix
{
  description = "RCC - Restricted Claude Code (Option A attempt)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        # Problem: Cannot set up container runtime or firewall
        # from within a Nix derivation

      in {
        packages.default = pkgs.stdenv.mkDerivation {
          pname = "rcc";
          version = "0.1.0";
          src = ./.;

          buildInputs = with pkgs; [
            makeWrapper
            # These are available but won't have privileges
            podman
          ];

          installPhase = ''
            mkdir -p $out/bin
            cp rcc $out/bin/

            # Can wrap with podman in PATH, but cannot:
            # - Configure podman rootless (needs newuidmap setuid)
            # - Set up pf firewall rules
            # - Create isolated network namespaces
            wrapProgram $out/bin/rcc \
              --prefix PATH : ${pkgs.lib.makeBinPath [ pkgs.podman ]}
          '';
        };

        # LIMITATION: This app runs unprivileged
        # Cannot set up firewall or containers automatically
        apps.default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/rcc";
        };
      }
    );
}
```

### Option B: Environment Preparation (Recommended)

```nix
{
  description = "RCC - Restricted Claude Code (Option B - Environment Prep)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        isDarwin = pkgs.stdenv.isDarwin;
        isLinux = pkgs.stdenv.isLinux;

        # Platform-specific dependencies
        platformDeps = if isDarwin then [
          # macOS-specific tools
        ] else [
          # Linux-specific tools
          pkgs.podman
          pkgs.runc
          pkgs.slirp4netns
        ];

        rcc = pkgs.buildGoModule {
          pname = "rcc";
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-...";

          nativeBuildInputs = [ pkgs.makeWrapper ];

          postInstall = ''
            wrapProgram $out/bin/rcc \
              --prefix PATH : ${pkgs.lib.makeBinPath platformDeps}
          '';
        };

        # Setup script (user runs with sudo separately)
        setupScript = pkgs.writeShellScriptBin "rcc-setup" ''
          echo "RCC Environment Setup"
          echo "====================="
          echo ""

          ${if isDarwin then ''
            echo "macOS detected. Checking requirements..."

            if ! command -v pfctl &> /dev/null; then
              echo "Error: pfctl not found"
              exit 1
            fi

            echo ""
            echo "To complete setup, run:"
            echo "  sudo rcc-setup-firewall"
            echo ""
          '' else ''
            echo "Linux detected. Checking requirements..."

            # Check for newuidmap (required for rootless containers)
            if ! command -v newuidmap &> /dev/null; then
              echo "Warning: newuidmap not found"
              echo "Install uidmap package for rootless containers"
            fi

            echo ""
            echo "To complete setup, run:"
            echo "  sudo rcc-setup-firewall"
            echo ""
          ''}
        '';

      in {
        packages = {
          default = rcc;
          setup = setupScript;
        };

        apps = {
          default = {
            type = "app";
            program = "${rcc}/bin/rcc";
          };

          claude = {
            type = "app";
            program = "${rcc}/bin/rcc";
            # Note: Args cannot be passed in app definition
            # User runs: nix run .#claude -- claude
          };

          setup = {
            type = "app";
            program = "${setupScript}/bin/rcc-setup";
          };
        };

        # Development shell for contributors
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            podman
          ] ++ platformDeps;

          shellHook = ''
            echo "RCC development environment"
            echo "Run 'nix run .#setup' to check system requirements"
          '';
        };
      }
    );
}
```

### Usage Flow for Option B

```bash
# 1. Check/prepare environment
nix run github:user/rcc#setup

# 2. Run setup script with elevated privileges (one-time)
sudo $(which rcc-setup-firewall)  # Provided separately

# 3. Use RCC
nix run github:user/rcc#claude

# Or install permanently
nix profile install github:user/rcc
rcc claude
```

---

## 7. Feasibility Assessment

### Option A: Full Implementation via Nix

| Requirement | Feasibility | Notes |
|-------------|-------------|-------|
| Distribute binary | **YES** | `nix run github:user/rcc` works |
| Cross-platform | **YES** | Per-platform builds supported |
| Container creation | **PARTIAL** | Can provide podman, cannot configure rootless |
| Network isolation | **NO** | Requires privileged operations |
| File isolation | **PARTIAL** | Limited without container runtime |
| Firewall setup | **NO** | Cannot run pfctl/iptables from flake |

**Verdict**: Not fully feasible. Nix cannot handle privileged setup operations.

### Option B: Environment Preparation

| Requirement | Feasibility | Notes |
|-------------|-------------|-------|
| Distribute rcc binary | **YES** | Works perfectly |
| Provide dependencies | **YES** | podman, tools in PATH |
| Setup scripts | **YES** | Can provide scripts |
| User runs setup | **YES** | One-time sudo required |
| Isolated execution | **YES** | rcc binary handles isolation |

**Verdict**: Fully feasible. Nix prepares environment, rcc handles privileged operations.

---

## 8. Recommendations

### For RCC Distribution

1. **Use Option B** - Let Nix handle software distribution, let rcc handle isolation

2. **Distribution Strategy**:
   ```bash
   # Primary: Nix Flake
   nix run github:user/rcc

   # Alternative: Traditional (for users without Nix)
   brew install rcc  # macOS
   apt install rcc   # Debian/Ubuntu
   ```

3. **First-run Experience**:
   - `nix run github:user/rcc#claude` downloads and runs
   - On first run, rcc detects missing setup
   - Prompts user to run `sudo rcc setup`
   - Setup configures firewall, container runtime

### Flake Best Practices

1. Use `flake-utils.lib.eachDefaultSystem` for cross-platform support
2. Wrap binaries to include runtime dependencies in PATH
3. Provide separate `setup` app for privileged operations
4. Include comprehensive `devShell` for contributors

---

## Sources

- [NixOS Wiki - Flakes](https://nixos.wiki/wiki/Flakes)
- [Determinate Systems - nix run](https://determinate.systems/blog/nix-run/)
- [nix.dev - Cross Compilation](https://nix.dev/tutorials/cross-compilation.html)
- [NixOS & Flakes Book](https://nixos-and-flakes.thiscute.world/)
- [NixOS Wiki - Podman](https://nixos.wiki/wiki/Podman)
- [NixOS Wiki - Firewall](https://nixos.wiki/wiki/Firewall)
- [Better Stack - Homebrew vs Nix](https://betterstack.com/community/guides/linux/homebrew-vs-nix/)
- [Tweag - Nix Flakes Introduction](https://www.tweag.io/blog/2020-05-25-flakes/)
