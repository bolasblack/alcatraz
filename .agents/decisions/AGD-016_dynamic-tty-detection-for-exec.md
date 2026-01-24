---
title: "Dynamic TTY Detection for Container Exec"
description: "Detect stdin TTY status to conditionally allocate PTY in container exec"
tags: cli
---

## Context

`alca run` executes commands inside containers via `docker exec` / `podman exec` / `container exec`. Interactive commands like `bash` require:
- `-i` flag: Keep stdin attached
- `-t` flag: Allocate a pseudo-TTY for proper terminal handling (prompts, tab completion, colors)

However, unconditionally using `-it` breaks pipe scenarios:
```bash
echo "ls" | alca run bash  # Error: "the input device is not a TTY"
```

## Decision

Use `golang.org/x/term` to dynamically detect if stdin is a terminal:

```go
args := []string{"exec", "-i"}
if term.IsTerminal(int(os.Stdin.Fd())) {
    args = append(args, "-t")
}
```

This applies to all three runtimes: Docker, Podman, and Apple Containerization.

### Behavior Matrix

| Scenario | stdin is TTY? | Flags | Result |
|----------|---------------|-------|--------|
| `alca run bash` | Yes | `-it` | Full interactive shell |
| `echo cmd \| alca run bash` | No (pipe) | `-i` | Non-interactive, no error |
| `alca run bash < file` | No (file) | `-i` | Non-interactive, no error |
| CI environment | Usually no | `-i` | Works without TTY error |

### Limitations

This approach detects TTY status at alca startup time. It does NOT support:
- Forcing TTY allocation in non-TTY environments (would need `--tty` flag)
- Disabling TTY in terminal environments (would need `--no-tty` flag)

These flags can be added later if users request them.

## Consequences

- Interactive terminal usage works correctly (prompts, colors, tab completion)
- Pipe and script usage works without "not a TTY" errors
- Added dependency: `golang.org/x/term`
- Programs detect TTY and may change output format (e.g., `ls` shows colors in terminal)
