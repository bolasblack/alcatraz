#!/usr/bin/env bash
#
# Convert OpenSSF Scorecard JSON output to SARIF format
#
# Usage: scorecard-json-to-sarif.sh <input.json> <output.sarif>
#
# Severity mapping:
#   score < 0:  error (check failed/errored)
#   score <= 3: error
#   score <= 6: warning
#   score >  6: note
#   score = 10: not reported (perfect score)
#

set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "Usage: $0 <input.json> <output.sarif>" >&2
    exit 1
fi

INPUT_FILE="$1"
OUTPUT_FILE="$2"

if [[ ! -f "$INPUT_FILE" ]]; then
    echo "Error: Input file '$INPUT_FILE' not found" >&2
    exit 1
fi

if ! command -v jq &> /dev/null; then
    echo "Error: jq is required but not installed" >&2
    exit 1
fi

# Validate input is valid JSON
if ! jq empty "$INPUT_FILE" 2>/dev/null; then
    echo "Error: Input file is not valid JSON" >&2
    exit 1
fi

# Check if input has expected structure
if ! jq -e '.checks' "$INPUT_FILE" > /dev/null 2>&1; then
    echo "Error: Input JSON does not have 'checks' field" >&2
    echo "Input structure:" >&2
    jq 'keys' "$INPUT_FILE" >&2
    exit 1
fi

# Convert to SARIF
SARIF_FILTER='
{
  "$schema": "https://json.schemastore.org/sarif-2.1.0.json",
  "version": "2.1.0",
  "runs": [{
    "tool": {
      "driver": {
        "name": "Scorecard",
        "informationUri": "https://github.com/ossf/scorecard",
        "version": (.scorecard.version // "unknown"),
        "rules": [.checks[] | {
          "id": .name,
          "name": .name,
          "shortDescription": { "text": .name },
          "fullDescription": { "text": (.reason // .name) },
          "help": {
            "text": (.documentation.short // ""),
            "markdown": ("See: " + (.documentation.url // "https://github.com/ossf/scorecard"))
          },
          "defaultConfiguration": { "level": "warning" },
          "properties": {
            "precision": "high",
            "tags": ["security", "supply-chain"]
          }
        }]
      }
    },
    "results": [.checks[] | select(.score != null and .score < 10) | {
      "ruleId": .name,
      "level": (if .score <= 3 then "error" elif .score <= 6 then "warning" else "note" end),
      "message": { "text": (.reason // "No details available") },
      "locations": [{
        "physicalLocation": {
          "artifactLocation": {
            "uri": "README.md",
            "uriBaseId": "%SRCROOT%"
          }
        }
      }]
    }]
  }]
}
'

if ! jq "$SARIF_FILTER" "$INPUT_FILE" > "$OUTPUT_FILE"; then
    echo "Error: jq conversion failed" >&2
    exit 1
fi

# Validate output is valid JSON
if ! jq empty "$OUTPUT_FILE" 2>/dev/null; then
    echo "Error: Output file is not valid JSON" >&2
    rm -f "$OUTPUT_FILE"
    exit 1
fi

# Show summary
RULES_COUNT=$(jq '.runs[0].tool.driver.rules | length' "$OUTPUT_FILE")
RESULTS_COUNT=$(jq '.runs[0].results | length' "$OUTPUT_FILE")
echo "Converted $INPUT_FILE to $OUTPUT_FILE ($RULES_COUNT rules, $RESULTS_COUNT findings)"
