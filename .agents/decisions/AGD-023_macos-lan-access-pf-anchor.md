---
title: "macOS LAN Access via pf Anchor"
description: "Use pf anchor and LaunchDaemon WatchPaths for container LAN access on macOS with OrbStack"
tags: macos, network-isolation, security, config
updates: AGD-005
obsoleted_by: AGD-030
---

## Context

On macOS with OrbStack, containers cannot access LAN hosts (other than the gateway) due to a NAT issue:

```
Container (192.168.215.x)
    ↓ Docker bridge NAT
OrbStack VM (192.168.139.2)
    ↓ IP forwarding (NO NAT!)
macOS (192.168.139.1 / 10.10.42.232)
    ↓
Physical LAN (10.10.42.0/24)
```

The OrbStack VM forwards packets with source IP `192.168.139.x`, but LAN hosts don't know how to route back to this private subnet. The router/gateway works because it has broader routing knowledge, but regular LAN hosts fail.

**This issue is OrbStack-specific.** Docker Desktop handles LAN access natively without additional configuration.

**Research findings**: docker-mac-net-connect was evaluated but solves the opposite problem (macOS → container access via WireGuard tunnel). Our issue is container → LAN access, requiring outbound NAT.

**Requirements**:
1. Containers can access LAN hosts when user configures `network.lan-access = ["*"]`
2. Rules are isolated and identifiable (user can see what alca added)
3. Rules can be manually cleaned up even if alca is broken
4. Rules survive system reboot (containers auto-restart, rules should too)
5. Clean uninstall path
6. Shared rules should not be duplicated across projects
7. Only apply pf rules when using OrbStack (not Docker Desktop)

## Decision

### 1. Runtime Detection

**Detect container runtime**:
```bash
docker info --format '{{.OperatingSystem}}'
# Returns "OrbStack" for OrbStack, other values for Docker Desktop
```

**Get OrbStack network subnet** (only when OrbStack detected):
```bash
orbctl config show | grep "network.subnet4" | cut -d: -f2 | tr -d ' '
# Returns: 192.168.138.0/23 (or user-configured value)
```

**Behavior by runtime**:
| Runtime | `lan-access = ["*"]` behavior |
|---------|-------------------------------|
| OrbStack | Requires network-helper + pf NAT rules |
| Docker Desktop | Works natively, no additional setup |
| Other | Warning: LAN access may not work |

### 2. Use pf Anchor for Rule Isolation

All alcatraz NAT rules go into a named pf anchor `alcatraz/`:

```bash
# View alcatraz rules
sudo pfctl -a "alcatraz" -s nat

# Clean up all alcatraz rules (even if alca is broken)
sudo pfctl -a "alcatraz" -F all
```

### 3. File-based Rule Management with Shared Rules

Rules are stored as files in `/etc/pf.anchors/alcatraz/`:

```
/etc/pf.anchors/alcatraz/
├── _shared                      # Shared NAT rule (created by first project, removed by last)
├── -Users-alice-project1        # Project-specific rules (future: network isolation whitelist)
└── -Users-bob-project2
```

**`_shared`** (common NAT rule, only one copy, subnet from `orbctl config`):
```
nat on en0 from 192.168.138.0/23 to any -> (en0)
```

**Per-project files** (future network isolation rules):
```bash
# Future: network isolation whitelist
block out quick on en0 from 192.168.138.0/23 to 10.0.0.0/8
pass out quick on en0 from 192.168.138.0/23 to 10.10.42.230
```

Project path is encoded by replacing `/` with `-`.

### 4. LaunchDaemon with WatchPaths for Auto-reload

A lightweight LaunchDaemon watches `/etc/pf.anchors/alcatraz/` and reloads rules when files change:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.alcatraz.pf-watcher</string>
    <key>WatchPaths</key>
    <array>
        <string>/etc/pf.anchors/alcatraz</string>
    </array>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/sh</string>
        <string>-c</string>
        <string>cat /etc/pf.anchors/alcatraz/* 2>/dev/null | pfctl -a alcatraz -f -</string>
    </array>
</dict>
</plist>
```

Key properties:
- **Not a daemon**: Only runs when directory changes (WatchPaths trigger)
- **Survives reboot**: launchd auto-loads from `/Library/LaunchDaemons/`
- **Minimal footprint**: Single shell command, no persistent process

### 5. User Commands

```bash
# Manual install (with confirmation prompt)
alca network-helper install

# Manual uninstall (removes LaunchDaemon and all rules)
alca network-helper uninstall

# Check status
alca network-helper status
```

### 6. Configuration

In `.alca.toml`:

```toml
[network]
lan-access = ["*"]   # Currently only "*" (all LAN) is supported
```

Future extensions may support specific IP ranges or hostnames.

### 7. Lifecycle

**alca network-helper install**:
1. Prompt user for confirmation: "This will install a LaunchDaemon to manage pf firewall rules. Continue? [y/N]"
2. Create `/etc/pf.anchors/alcatraz/` directory (requires sudo)
3. Copy plist to `/Library/LaunchDaemons/com.alcatraz.pf-watcher.plist`
4. Run `launchctl load /Library/LaunchDaemons/com.alcatraz.pf-watcher.plist`

**alca up** (with `network.lan-access = ["*"]`):
1. Detect runtime via `docker info --format '{{.OperatingSystem}}'`
2. **If Docker Desktop**: LAN access works natively, no action needed
3. **If OrbStack**:
   a. Check if network-helper is installed
   b. If not installed, prompt user: "OrbStack LAN access requires network-helper. This will install a LaunchDaemon and write rules to /etc/pf.anchors/alcatraz/. Install now? [y/N]"
   c. If user confirms, run install flow (with sudo password prompt)
   d. Get subnet via `orbctl config show | grep network.subnet4`
   e. Create `_shared` file with NAT rule if it doesn't exist
   f. Create project-specific file `-Project-Path` (currently empty, for future network isolation)
   g. WatchPaths triggers, LaunchDaemon loads rules into pf
4. **If other runtime**: Warning: "Unknown runtime, LAN access may not work as expected"

**alca down**:
1. If not OrbStack, no cleanup needed
2. Delete project-specific file `-Project-Path`
3. Check if any other project files remain (excluding `_shared`)
4. If no other projects, delete `_shared` file
5. WatchPaths triggers
6. Explicitly flush project anchor: `pfctl -a "alcatraz/-Project-Path" -F all`
7. If `_shared` was deleted, flush: `pfctl -a "alcatraz/_shared" -F all`

**alca network-helper uninstall**:
1. Prompt user for confirmation
2. `launchctl unload /Library/LaunchDaemons/com.alcatraz.pf-watcher.plist`
3. Delete plist file
4. `pfctl -a "alcatraz" -F all` (flush all alcatraz rules)
5. Remove `/etc/pf.anchors/alcatraz/` directory

**System reboot**:
1. launchd loads LaunchDaemon
2. Containers restart (if `restart: always`)
3. Rule files still exist in `/etc/pf.anchors/alcatraz/`
4. WatchPaths may not trigger on boot, but `alca up` will re-trigger

### 8. Manual Cleanup (Documentation)

For users to clean up without alca:

```bash
# View what alcatraz added
sudo pfctl -a "alcatraz" -s all

# Remove all alcatraz rules
sudo pfctl -a "alcatraz" -F all

# Remove LaunchDaemon
sudo launchctl unload /Library/LaunchDaemons/com.alcatraz.pf-watcher.plist
sudo rm /Library/LaunchDaemons/com.alcatraz.pf-watcher.plist

# Remove rule files
sudo rm -rf /etc/pf.anchors/alcatraz/
```

## Consequences

### Positive

- **Isolated**: alcatraz rules don't mix with system pf rules
- **Transparent**: Users can inspect exactly what alca added
- **Recoverable**: Manual cleanup always possible
- **Persistent**: Rules survive reboot via LaunchDaemon + file storage
- **Lightweight**: No persistent daemon process
- **No duplication**: Shared NAT rule stored once, referenced by all projects
- **User consent**: All privileged operations require explicit confirmation
- **Runtime-aware**: Only applies pf rules when necessary (OrbStack only)
- **Dynamic subnet**: Uses `orbctl config` to get correct subnet, not hardcoded

### Negative

- **Requires sudo**: Install and rule management need root (OrbStack only)
- **LaunchDaemon dependency**: Requires one-time setup for OrbStack users
- **macOS-specific**: This solution only works on macOS

### Risks

- **WatchPaths timing**: May not trigger immediately on boot; `alca up` handles this
- **pf conflicts**: Other tools using pf should not be affected (separate anchor)
- **Reference counting edge cases**: Concurrent `alca up/down` may race on `_shared` file
- **OrbStack config change**: If user changes `network.subnet4`, existing rules may become stale

## Alternatives Considered

1. **No persistence**: Require `alca up` after every reboot
   - Rejected: Poor UX when containers auto-restart

2. **Modify /etc/pf.conf**: Add anchor reference to system config
   - Rejected: System updates may overwrite; more invasive

3. **docker-mac-net-connect approach**: WireGuard tunnel + iptables
   - Not applicable: Solves opposite direction (host → container)

4. **Static route on LAN hosts**: Add route back to OrbStack subnet
   - Rejected: Requires access to each LAN host; not scalable

5. **Per-project NAT rules**: Each project has its own NAT rule file
   - Rejected: Duplicate rules; `_shared` file approach is cleaner

6. **Hardcoded IP range**: Use fixed 192.168.139.0/24
   - Rejected: OrbStack subnet is configurable; use `orbctl config` instead
