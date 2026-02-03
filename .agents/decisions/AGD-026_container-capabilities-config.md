---
title: "Container Capabilities Configuration"
description: "Add caps field to config for managing Linux capabilities with smart defaults for AI development"
tags: config, security, runtime
updates: AGD-009, AGD-007
---

## Context

Following Mutagen security research, we identified that Docker's default 14 capabilities include dangerous ones (NET_RAW for network sniffing, MKNOD for device creation) that AI development environments don't need. However, some capabilities (CHOWN, FOWNER, KILL) are essential for package managers and development workflows.

AGD-007 established the security threat model, but didn't specify container-level capability restrictions. AGD-009 defined the config format but didn't include capability management.

## Decision

Add a `caps` configuration field to `.alca.toml` with two modes:

### Mode 1: Additive (Array)

Default mode - adds capabilities to safe defaults:

```toml
caps = ["DAC_OVERRIDE", "SETUID"]
```

**Behavior**:

- Alcatraz adds `--cap-drop ALL`
- Alcatraz adds `--cap-add CHOWN --cap-add FOWNER --cap-add KILL` (defaults)
- Alcatraz adds `--cap-add DAC_OVERRIDE --cap-add SETUID` (user-specified)

**Default capabilities rationale**:

- `CHOWN`: Package managers (npm, pip, cargo) need to modify file ownership
- `FOWNER`: Modify file permissions and attributes during builds
- `KILL`: Terminate child processes (test runners, dev servers)

These three capabilities are sufficient for 90% of AI development workflows.

**Security note on CAP_NET_RAW**:

`NET_RAW` is explicitly excluded from defaults because it enables dangerous network operations (packet sniffing, ARP spoofing) that AI development doesn't require. Modern Docker (20.10+) auto-configures `ping_group_range` at the container level, allowing ping to work without `CAP_NET_RAW` via unprivileged ICMP sockets.

For network debugging, alternative tools are recommended:
- `curl` for HTTP/HTTPS connectivity testing
- `nc` (netcat) for TCP port testing
- `nslookup`/`dig` for DNS resolution

This approach aligns with cloud development environments like AWS CloudShell, which also prohibits `CAP_NET_RAW` for security ([source](https://kloudle.com/academy/a-technical-analysis-of-the-aws-cloudshell-service/)).

### Mode 2: Full Control (Object)

User wants complete control:

```toml
[caps]
drop = ["NET_RAW", "MKNOD", "AUDIT_WRITE"]
add = ["CHOWN", "DAC_OVERRIDE", "FOWNER", "KILL", "SETUID", "SETGID"]
```

**Behavior**:

- If `drop` field exists: Use `--cap-drop <each>` (NOT `--cap-drop ALL`)
- If `add` field exists: Use `--cap-add <each>` (ignore defaults)
- User has full control, Alcatraz doesn't add any implicit capabilities

### Default Behavior (No `caps` Field)

```toml
# No caps field specified
image = "nixos/nix"
```

**Behavior**:

- Alcatraz adds `--cap-drop ALL`
- Alcatraz adds `--cap-add CHOWN --cap-add FOWNER --cap-add KILL`

This provides secure defaults without breaking common development workflows.

## Consequences

### Positive

- **Secure by default**: Drops dangerous capabilities (NET_RAW, MKNOD)
- **Practical defaults**: Doesn't break npm/pip/cargo workflows
- **Flexible**: Power users can take full control via object mode
- **Simple**: Array syntax for 90% use case

### Negative

- **Two modes add complexity**: Need clear documentation
- **Defaults may not fit all workflows**: Some users may need to add DAC_OVERRIDE

### Migration

Existing configs without `caps` field get secure defaults automatically. No breaking changes.

## Examples

### Example 1: Default (Most Users)

```toml
# .alca.toml
image = "nixos/nix"
# No caps field - gets CHOWN, FOWNER, KILL
```

**Result**: `--cap-drop ALL --cap-add CHOWN --cap-add FOWNER --cap-add KILL`

### Example 2: Need DAC_OVERRIDE

```toml
# Some build tools need to write to protected directories
caps = ["DAC_OVERRIDE"]
```

**Result**: `--cap-drop ALL --cap-add CHOWN --cap-add FOWNER --cap-add KILL --cap-add DAC_OVERRIDE`

### Example 3: Full Control

```toml
# User wants exact capability set
[caps]
drop = ["ALL"]
add = ["CHOWN", "FOWNER", "KILL", "SETUID", "SETGID"]
```

**Result**: `--cap-drop ALL --cap-add CHOWN --cap-add FOWNER --cap-add KILL --cap-add SETUID --cap-add SETGID`

### Example 4: Keep Some Docker Defaults

```toml
# User wants to keep most Docker defaults, only drop dangerous ones
[caps]
drop = ["NET_RAW", "MKNOD", "SYS_CHROOT"]
# No add field - Docker defaults minus dropped ones
```

**Result**: `--cap-drop NET_RAW --cap-drop MKNOD --cap-drop SYS_CHROOT`

## Documentation Requirements

Update `docs/config.md` with:

1. Security rationale for capability restrictions
2. Clear explanation of two modes (additive vs full-control)
3. Default capabilities and why they're needed
4. Common scenarios (examples above)
5. Troubleshooting: "Permission denied" → add DAC_OVERRIDE

## References

- Linux capabilities(7): https://man7.org/linux/man-pages/man7/capabilities.7.html
- Docker security: https://docs.docker.com/engine/security/
- AGD-007: Security Threat Model
- AGD-009: Alcatraz CLI Design (config format)
