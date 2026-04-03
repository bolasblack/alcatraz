## Integration Test Rules

1. Happy path only — verify features work end-to-end, no error cases or edge cases
2. One test per feature — each test proves one feature works, nothing more
3. No re-testing unit test coverage — parsing, validation, shorthand forms are covered by unit tests
4. Real binary, real Docker — no mocks, POSIWID (test what the system does)
5. Cross-platform — must run on Linux and macOS, use POSIX-compatible shell
6. Skip gracefully — when Docker, Mutagen, or network-helper unavailable, skip with message
7. Clean up always — trap-based cleanup, run_with_timeout to prevent hangs
8. Use alpine:3.21 as base image
9. Use lan-access = ['*'] to skip firewall unless testing network isolation
10. Verify outcomes, not process — check that the feature produced the right result, not that specific log messages appeared

## Adding New Tests

- Create test_<feature>.sh with test functions (no top-level execution)
- Source from run.sh, add to appropriate group
- Each function: setup_test_dir → write config → run alca commands → assert outcomes → teardown_test_dir
- Use run_with_timeout for all alca commands
- Use assert helpers from helpers.sh (grep -F for literal matching)

## Naming Convention

All test files are sourced into a single shell — function names share a global namespace.

- **Test functions**: `test_<name>` — exported API, called from run.sh
- **Helper functions**: `<file>__<name>()` — prefix with the file's feature name (e.g., `ports__get_container_name`, `network__helper_installed`)
- **Variables**: `<FILE>__<NAME>` for file-scoped globals (e.g., `PROXY__LISTENER_NAME`)
