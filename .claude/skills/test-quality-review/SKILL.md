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

## Version History

- v1.0.0 (2025-02-09): Initial version
