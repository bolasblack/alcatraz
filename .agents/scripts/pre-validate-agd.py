#!/usr/bin/env python3
"""
Pre-validate AGD file content before creation.
Used by PreToolUse hook to block invalid AGD creation.

DO NOT MODIFY THIS FILE - it will be automatically updated from the skill directory.
To disable auto-update, add this filename to disableAutoUpdateScripts in config.json.

Exit codes:
- 0: Valid, allow creation
- 2: Invalid, BLOCK creation
"""

import json
import sys

from utils import get_agents_dir, get_project_dir, load_config, parse_frontmatter


def main():
    project_dir = get_project_dir()

    # Read hook input from stdin
    try:
        hook_input = json.load(sys.stdin)
    except (json.JSONDecodeError, IOError) as e:
        print(f"❌ BLOCKED: Failed to parse hook input: {e}", file=sys.stderr)
        sys.exit(2)

    tool_input = hook_input.get('tool_input', {})
    file_path = tool_input.get('file_path', '')
    content = tool_input.get('content', '')

    # Only check AGD files in .agents/decisions/
    agents_dir = get_agents_dir(project_dir)
    decisions_path = str(agents_dir / 'decisions')
    if decisions_path not in file_path or '/AGD-' not in file_path:
        sys.exit(0)

    # Parse frontmatter from content
    frontmatter = parse_frontmatter(content)
    if not frontmatter:
        sys.exit(0)  # No frontmatter, allow

    # Check tags
    tags_str = frontmatter.get('tags', '')
    if not tags_str:
        sys.exit(0)  # No tags, allow

    # Read allowed tags from config
    config_path = agents_dir / 'config.json'
    config = load_config(config_path)
    if not config:
        sys.exit(0)  # No config, allow

    allowed_tags = config.get('tags', [])
    tags = [t.strip() for t in tags_str.split(',') if t.strip()]
    invalid_tags = [t for t in tags if t not in allowed_tags]

    if invalid_tags:
        print(f"❌ BLOCKED: Invalid tags: {', '.join(invalid_tags)}", file=sys.stderr)
        if allowed_tags:
            print(f"   Allowed tags: {', '.join(allowed_tags)}", file=sys.stderr)
        else:
            print("   No tags defined in config.json yet", file=sys.stderr)
        print("   Fix: Add tags to .agents/config.json first, then retry", file=sys.stderr)
        sys.exit(2)  # Exit code 2 = BLOCK tool call

    sys.exit(0)


if __name__ == '__main__':
    main()
