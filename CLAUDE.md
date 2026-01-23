See [.agents/CLAUDE.md](.agents/CLAUDE.md) for the Agent Centric framework.

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
