---
title: "LAN Access Token Expansion"
description: "Support alca-managed special tokens like ${alca:HOST_IP} in lan-access rules, resolved at runtime before rule parsing"
tags: config, network-isolation
updates: AGD-028
---

## Context

Users often need containers to connect back to services on the host machine (e.g., local dev servers, databases). With AGD-028's lan-access syntax, this requires hardcoding the host IP:

```toml
lan-access = ["172.17.0.1:14242"]
```

This IP varies across environments — Docker uses bridge gateway (`172.17.0.1`), Podman and OrbStack may differ. Hardcoding breaks portability and forces users to know container networking details.

A common request is to say "allow access to the host" without knowing the concrete IP.

### Why Not Use OS Environment Variables?

AGD-017 established `${VAR}` syntax for OS env var expansion in config values. We could reuse that syntax (e.g., `${HOST_IP}`) and expect users to set the variable themselves. However `${HOST_IP}` looks like a user-defined env var, but it's a runtime-computed value. Mixing the two in the same namespace creates ambiguity about who is responsible for providing the value.

## Decision

### Token Syntax

Introduce **alca-managed tokens** with a namespaced syntax: `${alca:TOKEN_NAME}`.

```toml
lan-access = ["${alca:HOST_IP}:14242"]
```

The `alca:` prefix clearly distinguishes these from OS env vars (`${VAR}`), making it obvious that alca resolves them at runtime.

### Initial Token: `HOST_IP`

`${alca:HOST_IP}` resolves to the IP address at which the host machine is reachable from inside the container. Resolution strategy per runtime:

| Runtime  | Resolution method                                        |
| -------- | -------------------------------------------------------- |
| Docker   | Bridge network gateway (`docker network inspect bridge`) |
| Podman   | Bridge network gateway (`podman network inspect`)        |
| OrbStack | Bridge network gateway                                   |

The exact commands may vary, but the semantic is always "the host-reachable IP from the container's network".

### Expansion Architecture

**Pre-parse expansion** — tokens are expanded in the raw `lan-access` strings before they reach `ParseLANAccessRule`. This keeps the rule parser (AGD-028) unchanged.

- Known tokens → resolved to concrete values
- Unknown `${alca:...}` tokens → error (strict, fail-fast)
- `${VAR}` patterns (no `alca:` prefix) → left untouched (not this expander's responsibility)

### Expansion Ordering

When both OS env vars and alca tokens appear in config values:

1. **OS env var expansion** (`${VAR}`) — at config parse time via `StrictExpandEnv`
2. **Alca token expansion** (`${alca:...}`) — at runtime, when container runtime context is available

Currently `StrictExpandEnv` is not applied to `lan-access` values. If it is added in the future, the ordering above must be maintained to avoid conflicts.

### Validation

- Config loading: syntactically validate that `${alca:...}` patterns have valid token names (alphanumeric + underscore). Do not resolve yet — resolution happens later.
- Runtime: resolve tokens and fail with a clear error if resolution fails (e.g., runtime can't determine host IP).

## Consequences

### Positive

- **Portable configs** — `${alca:HOST_IP}` works across Docker, Podman, and different machines without modification
- **Clear namespace** — `alca:` prefix eliminates ambiguity with OS env vars
- **Extensible** — new tokens (e.g., `${alca:GATEWAY}`, `${alca:CONTAINER_IP}`) can be added by registering resolvers, no parser changes needed
- **Non-invasive** — pre-parse expansion means `ParseLANAccessRule` and firewall layers remain unchanged

### Negative

- **Runtime dependency** — token resolution requires a functioning container runtime; offline config validation can only check syntax, not resolved values
- **New concept** — users need to learn the `${alca:...}` syntax alongside existing `${VAR}` env var syntax

## References

- AGD-028: LAN Access Configuration Syntax
- AGD-017: Environment Variable Configuration Design
