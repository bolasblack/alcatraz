# Apple Containerization Setup Flow

Apple Containerization setup flow from CLI installation to running containers.

## Prerequisites

| Requirement | Detection | Notes |
|------------|-----------|-------|
| macOS 26+ (Tahoe) | `sw_vers -productVersion` | macOS 15 works with [limitations](#macos-15-limitations) |
| Apple Silicon | `uname -m` = `arm64` | Intel Macs not supported |
| container CLI | `which container` | Installed via .pkg from GitHub releases |

## Setup States

```
[Not Installed] -> [Installed] -> [System Running] -> [Kernel Configured] -> [Ready]
```

## Step 1: Install CLI

Download from [GitHub Releases](https://github.com/apple/container/releases):
- Download `container-*-installer-signed.pkg`
- Run installer (requires admin password)
- Installs to `/usr/local/bin/container`

**Verify:**
```bash
which container
container --version
```

## Step 2: Start System

```bash
container system start
```

This starts `container-apiserver` via launchd.

**Verify:**
```bash
container system status
```

## Step 3: Configure Kernel

On first `container system start`, you'll see:

```
No default kernel configured.
Install the recommended default kernel from [https://github.com/kata-containers/kata-containers/releases/download/3.17.0/kata-static-3.17.0-arm64.tar.xz]?
[Y/n]:
```

Type `y` to install. This is a **Linux kernel for VMs**, not a macOS kernel modification.

**Automation options:**
```bash
# Auto-install (non-interactive)
container system start --enable-kernel-install

# Skip kernel install
container system start --disable-kernel-install

# Manual install later
container system kernel set --recommended
```

**Verify:**
```bash
container system property get kernel.url
# or
ls ~/.container/kernels/
```

## Step 4: Verify Ready

```bash
container image list
# Success: exits 0 (may show empty list)
```

## macOS 15 Limitations

On macOS 15 (Sequoia), container CLI works with restrictions:
- No container-to-container networking
- Container IP not reachable from host
- Port publishing (`-p`) still works

## Links

### Official
- [apple/container](https://github.com/apple/container) - CLI tool
- [apple/containerization](https://github.com/apple/containerization) - Swift framework
- [Command Reference](https://github.com/apple/container/blob/main/docs/command-reference.md)
- [How-to Guide](https://github.com/apple/container/blob/main/docs/how-to.md)
- [Tutorial](https://github.com/apple/container/blob/main/docs/tutorial.md)

### Kernel
- [Kata Containers Releases](https://github.com/kata-containers/kata-containers/releases) - Default kernel source
- [Kernel Configuration](https://github.com/apple/containerization/tree/main/kernel) - Build custom kernels

### Community
- [Tutorial: Setting Up Apple Containerization](https://thenewstack.io/tutorial-setting-up-and-exploring-apple-containerization-on-macos/) - The New Stack
- [Kali Linux & Containerization](https://www.kali.org/blog/kali-apple-container-containerization/) - Kali Blog
