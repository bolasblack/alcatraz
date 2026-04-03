---
name: "test-quality-review"
description: "Review test code quality against behavioral testing principles. Use when reviewing test files, evaluating test quality, auditing test suites, or when asked about test coverage and test design issues."
---

# Test Quality Review

Review test code quality by evaluating whether tests verify **behavior contracts** rather than **implementation details**.

## Core Principles

Three non-negotiable rules for good tests:

1. **Tests exist to discover BUGS** — mismatches between implementation and expectation
2. **Write expected behavior FIRST** — tests describe WHAT, not HOW
3. **Never mirror implementation** — if refactoring internals (without changing behavior) breaks a test, it's a bad test

## Evaluation Criteria

**Any unit test that violates criteria 1 (Behavior vs Implementation) or 2 (Mock Discipline) MUST be modified.**

### 1. Behavior vs Implementation

- **Good**: "given input X, expect output Y"
- **Bad**: "verify function calls A, then B, then C in order"
- **Bad**: mocking internal details that aren't part of the contract

### 2. Mock Discipline

- Mocks isolate external boundaries (filesystem, network, commands)
- Not mocking the unit under test's own internals
- Over-mocking = testing the mock, not the code

### 3. Expectation Clarity

- **Good**: expected values are literals, easy to see expected vs actual
- **Bad**: computed expected values, assertions hidden in helper functions
- Tests should have zero logic — no conditionals, no loops, no computation

### 4. Edge Cases

- Error paths tested?
- Boundary conditions (nil, empty, zero, max) covered?
- Not just happy path?

### 5. Test Isolation

- Each test stands alone, no order dependencies
- No shared mutable state between tests
- Setup is minimal and focused

### 6. Test Naming

- Name describes the scenario: `TestSubject_Condition_ExpectedResult`
- Readable without looking at code

### 7. Coverage Gaps

- Public functions/methods without tests?
- Obvious scenarios not covered?

### 8. Error Assertion Quality

Error path tests should verify **which** error occurred, not just **that** an error occurred.

- Never use `strings.Contains(err.Error(), ...)` or any string comparison on `.Error()` to identify **which error** occurred — use `errors.Is` or `errors.As` instead
- **Exception**: String checks on error messages are acceptable when verifying **diagnostic details** (e.g., that the error includes the relevant file path, parameter name, invalid value, or other context that helps users debug) rather than identifying the error type. The test should still assert `err != nil` first, and the string check should validate supplementary info, not substitute for a missing sentinel.
- When a test checks `err == nil` or `err != nil` on an error path, ask two questions: (1) does the function already expose a sentinel error? If yes, the test should use `errors.Is`. (2) If not, should it? Most distinguishable error conditions deserve a sentinel — suggest adding one rather than accepting a bare nil check.
- Happy-path `err != nil` is fine — when testing success behavior, a nil check is sufficient

Examples:
- **Bad**: `if !strings.Contains(err.Error(), "not found") { ... }` — using string match to identify error type
- **Bad**: `if err == nil { t.Fatal("expected error") }` — when a sentinel like `ErrNotFound` exists
- **Good**: `if !errors.Is(err, ErrNotFound) { t.Fatalf("expected ErrNotFound, got %v", err) }`
- **OK**: `if err != nil { t.Fatalf("unexpected error: %v", err) }` — happy-path guard
- **OK**: `if !strings.Contains(err.Error(), "config.toml") { ... }` — after `err != nil` guard, verifying the error includes relevant diagnostic details (file path, parameter name, invalid value, etc.)

## Version History

- v1.1.1 (2026-02-23): §8 exception: string checks on error messages are OK for verifying diagnostic details (file path, parameter name, invalid value, etc.), not for identifying error types
- v1.1.0 (2025-02-23): Add error assertion quality criterion (§8)
- v1.0.0 (2025-02-09): Initial version
