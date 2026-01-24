---
title: "Manual Maintenance for Config Documentation"
description: "Decision to manually maintain docs/config.md instead of auto-generating from Config struct"
tags: tooling, config
---

## Context

We explored auto-generating `docs/config.md` from the `Config` struct in `internal/config/config.go`, similar to how `cmd/genschema/main.go` generates `alca-config.schema.json`.

The goal was to keep code as the single source of truth for configuration documentation.

### Research Conducted

We evaluated several existing tools:

| Tool                                                                                                                            | Language | GitHub Stars | Approach                        | Issues                                                                                                                                                                                      |
| ------------------------------------------------------------------------------------------------------------------------------- | -------- | ------------ | ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [json-schema-docs](https://github.com/marcusolsson/json-schema-docs)                                                            | Go       | ~50          | Template-based from JSON Schema | Does not support `$ref` - our schema uses `$ref` for nested types (Commands, Resources, EnvValue), resulting in empty output                                                                |
| [schemadoc](https://github.com/twelvelabs/schemadoc)                                                                            | Go       | ~30          | Template-based from JSON Schema | Low adoption, has `replace` directives in go.mod preventing `go install`, limited documentation                                                                                             |
| [jsonschema2md](https://github.com/adobe/jsonschema2md)                                                                         | Node.js  | ~300         | Full JSON Schema support        | Requires Node.js runtime, output format is overly complex with excessive metadata tables (Abstract, Extensible, Status, Identifiable columns), generates multiple linked files per property |
| [gomarkdoc](https://github.com/princjef/gomarkdoc)                                                                              | Go       | ~400         | go/doc parsing                  | Package-level documentation only, not designed for struct field documentation                                                                                                               |
| [packer-sdc struct-markdown](https://pkg.go.dev/github.com/hashicorp/packer-plugin-sdk/cmd/packer-sdc/internal/struct-markdown) | Go       | N/A          | AST parsing                     | Internal HashiCorp tool, focused on `mapstructure` tags, not general-purpose                                                                                                                |

### Key Findings

1. **$ref support is critical**: Our JSON Schema uses `$ref` for nested types. Most lightweight Go tools (json-schema-docs) don't handle this.

2. **Output format mismatch**: Tools like jsonschema2md generate complex multi-file documentation with metadata tables. Our `docs/config.md` has a clean, human-friendly format with examples and runtime-specific notes.

3. **Low adoption risk**: The Go-based tools have very few stars (30-50), indicating limited community support and potential maintenance issues.

4. **Custom implementation cost**: Building our own generator would require significant effort for marginal benefit, given that config fields change infrequently.

## Decision

**Manually maintain `docs/config.md`** instead of auto-generating.

Rationale:

1. **Low change frequency**: Config struct fields rarely change. When they do, updating docs manually is straightforward.

2. **Richer documentation**: Manual docs include examples, runtime-specific notes, and formatting that would be difficult to generate.

3. **No external dependencies**: Avoid adding Node.js or unmaintained Go tools to the build process.

4. **Existing JSON Schema serves validation**: The auto-generated `alca-config.schema.json` provides editor autocomplete and validation. Docs serve a different purpose (human reading).

## Consequences

### Positive

- No additional build dependencies
- Full control over documentation format and content
- Can include context that code comments cannot express (runtime notes, examples, tips)

### Negative

- Must remember to update docs when Config struct changes
- Risk of docs drifting from code

### Mitigated

- JSON Schema is still auto-generated for editor validation
- Code review should catch config changes without doc updates
- Config struct has `jsonschema:"description=..."` tags that document fields in code

## References

- Current docs: `docs/config.md`
- Schema generator: `cmd/genschema/main.go`
