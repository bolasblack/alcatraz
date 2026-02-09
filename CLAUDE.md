See [.agents/CLAUDE.md](.agents/CLAUDE.md) for the Agent Centric framework.

## AGD Operations

When creating or updating AGD files, **always load the `/agent-centric` skill first**. This ensures proper validation, indexing, and relationship maintenance.

## Config Changes Checklist

When modifying config-related code (`internal/config/`):

1. **Update documentation**: `docs/config/` - add/update field descriptions, examples
2. **Regenerate schema**: Run `make schema` to update `alca-config.schema.json` for editor autocomplete
3. **Create AGD if needed**: Record significant config design decisions

## Quality Checks

After modifying code, always run `make lint` and fix any issues before reporting completion.

## Code Patterns

Project-wide code patterns that should be followed. See AGDs tagged with `#patterns`.

### Struct Field Exhaustiveness Check (AGD-015)

When implementing comparison or processing functions for structs, use a mirror type conversion to ensure all fields are explicitly handled:

```go
func (s *MyStruct) Equals(other *MyStruct) bool {
    // Mirror type - must match MyStruct fields exactly
    type fields struct { /* copy all fields */ }
    _ = fields(*s)  // Compile error if fields mismatch

    // Explicit comparison logic...
}
```

This ensures adding a new field to the struct will cause a compile error, forcing review of the comparison logic.

### Compile-Time Interface Assertions

When a concrete type implements an interface, add a compile-time assertion at package level to guarantee it:

```go
var _ MyInterface = (*MyImplementation)(nil)
```

This catches interface drift at compile time rather than runtime. Apply this in all packages where interfaces are defined and implemented.

### Env Dependency Injection (AGD-029)

All `internal/` business modules receive `Fs` and `CommandRunner` from external callers — never create them internally. CLI is the entry point that creates and injects deps.

- **Simple modules**: use `util.Env` directly
- **Complex modules** (network, runtime, etc.): define own `XxxEnv` with `NewXxxEnv(fs, cmd)` constructor
- **CLI pattern**: create `cmdRunner`, `fs` once, pass the same instance to all Env constructors

### Cross-Platform Testability via DI

Platform-specific code should avoid platform-specific imports when possible. Inject platform behavior via DI so code compiles and tests on all platforms.

- Exemplar: `darwin/vmhelper/` — zero build constraints, 89% test coverage, fully testable on Linux
- If a function doesn't call platform syscalls, it doesn't need a build constraint
- Use `runtime.GOOS` or injected platform values for runtime dispatch instead of build-time file separation

### Platform-Specific File Naming (Dot Separator)

Platform-specific files use a dot separator: `name.platform.go` (e.g., `helper.darwin.go`, `helper.linux.go`).

- Keeps the descriptive name first and platform last
- Avoids Go compiler's implicit build constraint from `_GOOS.go` suffix
- Code inside these files has no platform-specific imports — testable on all platforms
- Test files follow the same pattern: `firewall.darwin_test.go`

### Testing Principles

Tests exist to discover bugs — mismatches between implementation and expectation.

- **Write expected behavior first**, run the test, then fix the implementation if it fails
- **Test behavior, not implementation**: test inputs/outputs from the caller's perspective. If a test breaks when you refactor internals without changing behavior, it's a bad test
- **No implementation mirroring**: don't assert concrete types, internal field values, or call sequences. Test through interfaces

## Build

### Makefile Embedded File Tracking

Non-`.go` embedded files (e.g., `entry.sh` via `//go:embed`) must be added to `EMBED_SRC` in the Makefile. Otherwise `make build` won't detect changes to these files and will skip rebuilding.

### TransactFs Commit Ordering

Files that need to exist on the real filesystem (e.g., scripts read by Docker containers via volume mounts) must be written in the **pre-commit** phase via TransactFs. The post-commit phase (e.g., Docker operations) can only read files that have already been committed to disk.

- Pre-commit: `WriteEntryScript()` — stages file in TransactFs
- `commitIfNeeded()` — flushes to disk
- Post-commit: `InstallHelper()` — Docker ops that read from disk
