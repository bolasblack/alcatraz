# Nix + Container Integration Research

## Executive Summary

This research investigates whether Nix can build/manage OCI containers and provide file isolation for the RCC use case. Three main technologies were evaluated: **nix2container**, **NixOS containers**, and **arion**. Additionally, **NixPak** and **jail.nix** were discovered as relevant sandboxing solutions.

**Key Finding**: For RCC's AI isolation requirements, **NixPak/jail.nix with bubblewrap** appears most promising for file isolation, while **nix2container** excels for OCI image building. NixOS containers are limited to NixOS hosts.

---

## Technology Overview

### 1. nix2container

**What it is**: An archive-less OCI container image builder for Nix, providing a faster alternative to `dockerTools.buildImage`.

**How it works**:
- Builds OCI-compliant container images directly from Nix derivations
- Skips tarball generation in Nix store (faster rebuild/repush cycles: ~1.8s vs ~7.5s)
- Uses Skopeo for registry push operations
- Supports layered images with explicit dependency isolation

**Key Features**:
- `buildImage`: Creates container images with name, config, entrypoint, copyToRoot options
- `buildLayer`: Isolates dependencies in dedicated layers
- `pullImage` / `pullImageFromManifest`: Registry image pulling with deduplication
- Permission management via `perms` attribute

**Code Example**:
```nix
{
  inputs.nix2container.url = "github:nlewo/nix2container";
  outputs = { self, nixpkgs, nix2container }: let
    pkgs = import nixpkgs { system = "x86_64-linux"; };
    n2c = nix2container.packages.x86_64-linux.nix2container;
  in {
    packages.x86_64-linux.myImage = n2c.buildImage {
      name = "my-app";
      config = { entrypoint = ["${pkgs.hello}/bin/hello"]; };
      copyToRoot = pkgs.buildEnv {
        name = "root";
        paths = [ pkgs.bash pkgs.coreutils ];
      };
    };
  };
}
```

**File Isolation**: None inherent. nix2container builds images but doesn't provide runtime isolation - that's the container runtime's job (Docker/Podman).

**Cross-Platform Support**:
- Linux: Native support
- macOS: Requires Linux builder (nix-darwin linux-builder module recommended)
  - `nix.linux-builder.enable = true` in darwin-configuration.nix
  - Alternative: rosetta-builder for ARM Macs

**Sources**:
- [GitHub - nlewo/nix2container](https://github.com/nlewo/nix2container)
- [Building Docker images with Nix on MacOS](https://emilio.co.za/blog/nix-oci-images-macos/)
- [nix.dev - Building and running Docker images](https://nix.dev/tutorials/nixos/building-and-running-docker-images.html)

---

### 2. NixOS Containers

**What it is**: Native systemd-nspawn containers managed declaratively through NixOS configuration.

**How it works**:
- Uses systemd-nspawn as the container runtime
- Containers run a full NixOS instance with init system
- Configured via `containers.<name>` in NixOS configuration
- Managed via `machinectl`, `systemctl`, `nixos-container`

**Key Features**:
- Declarative or imperative management
- Network isolation (`privateNetwork = true`)
- Bind mounts for selective directory sharing
- Ephemeral mode (`ephemeral = true`)
- User namespace support for enhanced security

**Code Example**:
```nix
containers.webserver = {
  autoStart = true;
  privateNetwork = true;
  hostAddress = "192.168.100.10";
  localAddress = "192.168.100.11";

  bindMounts = {
    "/data" = {
      hostPath = "/var/lib/webserver-data";
      isReadOnly = false;
    };
  };

  config = { config, pkgs, ... }: {
    services.nginx.enable = true;
    system.stateVersion = "24.05";
  };
};
```

**File Isolation**:
- Containers have isolated filesystems by default
- Host directories inaccessible unless explicitly bound
- **Shared Nix store** between host and containers (security consideration: no secrets in store)
- Bind mounts for selective access

**Security Concerns**:
- Default containers are "privileged" - root inside can potentially escape
- User namespacing (`enablePick = true`) recommended for enhanced isolation
- Not considered fully secure against sophisticated attacks

**Cross-Platform Support**:
- **NixOS only** - requires NixOS as the host system
- Cannot be used on macOS, other Linux distros, or Windows

**Sources**:
- [NixOS Wiki - NixOS Containers](https://wiki.nixos.org/wiki/NixOS_Containers)
- [Application Isolation using NixOS Containers](https://msucharski.eu/posts/application-isolation-nixos-containers/)
- [Running NixOS in systemd-nspawn](https://nixcademy.com/posts/nixos-nspawn/)

---

### 3. Arion

**What it is**: A tool for running Docker Compose with Nix/NixOS module system integration.

**How it works**:
- Evaluates Nix configuration → generates docker-compose.yaml
- Invokes docker-compose
- Uses NixOS module system for service configuration

**Key Features**:
- Single language for deployments, configuration, and packaging
- Parallel image building
- Package sharing between images
- Skip container image creation for performance
- NixOS-style module system configuration

**Composition Options**:
1. Minimal - Plain command using nixpkgs
2. NixOS unit - Single systemd service
3. Full NixOS - Complete OS environment
4. DockerHub image - Pre-built images

**Code Example**:
```nix
# arion-compose.nix
{
  services.webserver = {
    service.useHostStore = true;
    service.command = [ "python" "-m" "http.server" ];
    service.ports = [ "8000:8000" ];
    service.volumes = [
      "${toString ./.}/data:/data"
    ];
  };
}
```

**File Isolation**: Relies on Docker/Podman for runtime isolation. `useHostStore = true` binds host Nix store (not for production).

**Cross-Platform Support**:
- Primary: Linux (NixOS with CI testing)
- Requires Docker or Podman
- macOS: Theoretically works with Docker Desktop (limited)

**Sources**:
- [GitHub - hercules-ci/arion](https://github.com/hercules-ci/arion)
- [Arion Documentation](https://docs.hercules-ci.com/arion/)
- [NixOS Discourse - Arion announcement](https://discourse.nixos.org/t/arion-docker-compose-with-nix-or-nixos/2874)

---

## Additional Technologies (Highly Relevant for RCC)

### 4. NixPak / jail.nix (Bubblewrap Integration)

**What it is**: Runtime sandboxing for Nix applications using bubblewrap.

**How it works**:
- Uses bubblewrap (bwrap) for namespace isolation
- Declarative sandbox configuration via NixOS module system
- "Sloth values" for runtime resolution

**Key Features**:
- Fine-grained file system isolation (bind mounts)
- Network isolation
- D-Bus access control
- Device binding
- Flatpak shim for xdg-desktop-portal integration

**Code Example (jail.nix for AI agents)**:
```nix
jail = config: {
  bindMounts = {
    # Only mount current project directory
    "/project" = {
      hostPath = ".";
      writable = true;
    };
  };

  # Approved tools only
  packages = [ bash curl git ripgrep ];

  # Network enabled
  network = true;
};
```

**File Isolation**: **Excellent** - Zero trust model:
- No default access to home, SSH keys, sensitive files
- Only explicitly approved directories mounted
- "Brick wall" everywhere else

**Cross-Platform Support**:
- Linux only (bubblewrap requires Linux namespaces)
- Cannot work on macOS directly

**Sources**:
- [GitHub - nixpak/nixpak](https://github.com/nixpak/nixpak)
- [How I Run LLM Agents in a Secure Nix Sandbox](https://dev.to/andersonjoseph/how-i-run-llm-agents-in-a-secure-nix-sandbox-1899)

### 5. devenv Containers

**What it is**: OCI container generation from devenv development environments.

**Commands**: `devenv container build`, `devenv container run`, `devenv container copy`

**File Isolation**: Via `copyToRoot` configuration - control what gets included in container.

**Cross-Platform**: Linux native, macOS requires Linux builder.

**Source**: [devenv.sh/containers](https://devenv.sh/containers/)

---

## Comparison Table

| Feature | nix2container | NixOS Containers | Arion | NixPak/jail.nix |
|---------|---------------|------------------|-------|-----------------|
| **Purpose** | OCI image building | System containers | Docker Compose + Nix | Runtime sandboxing |
| **File Isolation** | None (build-time) | Yes (bind mounts) | Via Docker/Podman | Excellent (zero trust) |
| **Network Isolation** | N/A | Yes (privateNetwork) | Via Docker/Podman | Yes (bubblewrap) |
| **AI-Untouchable** | No | Partial (root escape risk) | Via container runtime | Yes (user namespaces) |
| **Memory Control** | N/A | Via systemd | Via Docker/Podman | Via cgroups |
| **macOS Support** | Via Linux builder | No | Limited | No |
| **Linux Support** | Yes | NixOS only | Yes | Yes |
| **Requires Docker/Podman** | For running | No | Yes | No |
| **Declarative Config** | Yes | Yes | Yes | Yes |
| **Setup Complexity** | Low-Medium | Low (on NixOS) | Medium | Medium |

---

## Feasibility Assessment for RCC Use Case

### MVP Requirements Review

| Requirement | nix2container | NixOS Containers | Arion | NixPak |
|-------------|---------------|------------------|-------|--------|
| **1. File Isolation** | ❌ Build-time only | ⚠️ Partial | ⚠️ Via runtime | ✅ Excellent |
| **2. Network Isolation** | ❌ N/A | ✅ Yes | ✅ Via Docker | ✅ Yes |
| **3. AI-Untouchable** | ❌ N/A | ⚠️ Root escape risk | ⚠️ Via runtime | ✅ User namespaces |
| **4. Memory Auto-Release** | ❌ N/A | ✅ Via systemd | ✅ Via Docker | ✅ Via cgroups |
| **Cross-Platform** | ⚠️ macOS needs builder | ❌ NixOS only | ⚠️ Limited macOS | ❌ Linux only |

### Key Insights

1. **No single Nix technology provides complete RCC solution**
   - nix2container: Great for building, not runtime isolation
   - NixOS containers: Limited to NixOS hosts
   - Arion: Still depends on Docker/Podman for isolation

2. **NixPak/jail.nix is closest to RCC requirements**
   - Provides file isolation at AI-untouchable level (bubblewrap)
   - Network isolation supported
   - But: Linux-only, no macOS support

3. **Hybrid approach may be needed**
   - Use nix2container to build minimal OCI images
   - Use Podman/Docker for cross-platform runtime isolation
   - Configuration can be declarative via Nix

4. **macOS remains the challenge**
   - All strong isolation solutions require Linux namespaces
   - macOS would need: VM-based solution (Docker Desktop, Lima, etc.)

---

## Recommendations

### For RCC Implementation

1. **Primary Path: Nix + Podman/Docker (Hybrid)**
   - Use Nix Flakes for environment definition
   - Use nix2container/dockerTools for OCI image building
   - Use Podman rootless containers for runtime isolation
   - This achieves all 4 MVP requirements but requires container runtime

2. **Alternative for Linux-only: NixPak/jail.nix**
   - If macOS support not critical initially
   - Provides excellent file isolation without Docker
   - Closer to "native Nix" solution
   - Based on proven technology (bubblewrap powers Flatpak)

3. **Not Recommended: NixOS Containers**
   - Too limited (NixOS-only host requirement)
   - Security concerns with privileged containers

### Developer Experience Considerations

| Approach | DX Rating | Notes |
|----------|-----------|-------|
| nix2container + Podman | ⭐⭐⭐⭐ | Familiar container workflow |
| NixPak | ⭐⭐⭐ | Learning curve, Linux-only |
| NixOS Containers | ⭐⭐ | NixOS knowledge required |
| Arion | ⭐⭐⭐⭐ | Docker Compose familiarity |

---

## Conclusion

**Can Nix provide file isolation guarantees without manual Docker/Podman setup?**

**Partial Yes** - NixPak/jail.nix can provide strong file isolation on Linux without Docker, using bubblewrap namespaces. However:
- macOS requires containerization (Docker Desktop or Lima VM)
- No pure-Nix cross-platform isolation solution exists

**Recommendation for RCC**: A **hybrid approach** combining:
1. Nix Flakes for reproducible environment definition
2. nix2container for efficient OCI image building
3. Podman rootless for cross-platform runtime isolation

This provides the best balance of Nix benefits (reproducibility, declarative config) with proven container isolation technology.

---

## References

- [nix2container GitHub](https://github.com/nlewo/nix2container)
- [NixOS Containers Wiki](https://wiki.nixos.org/wiki/NixOS_Containers)
- [Arion Documentation](https://docs.hercules-ci.com/arion/)
- [NixPak GitHub](https://github.com/nixpak/nixpak)
- [Running LLM Agents in Nix Sandbox](https://dev.to/andersonjoseph/how-i-run-llm-agents-in-a-secure-nix-sandbox-1899)
- [Application Isolation with NixOS Containers](https://msucharski.eu/posts/application-isolation-nixos-containers/)
- [Building Docker images on macOS](https://emilio.co.za/blog/nix-oci-images-macos/)
- [devenv Containers](https://devenv.sh/containers/)
- [nix.dev Container Tutorial](https://nix.dev/tutorials/nixos/building-and-running-docker-images.html)
