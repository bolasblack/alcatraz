#!/usr/bin/env python3
"""
Generate index files for the Agent Centric framework.

DO NOT MODIFY THIS FILE - it will be automatically updated from the skill directory.
To disable auto-update, add this filename to disableAutoUpdateScripts in config.json.

Usage:
    python3 generate-index.py <project_dir>

Generates:
    - INDEX-TAGS.md: Files with their tags
    - INDEX-AGD-RELATIONS.md: AGD relationship index (obsoletes/updates/related)
    - Bidirectional sync: Rewrites managed reverse references on AGD files
"""

import os
import sys
import tempfile
from pathlib import Path

from utils import (
    AGD_PATTERN,
    DECISIONS_DIR,
    RELATION_FIELDS,
    REVERSE_REF_FIELDS,
    find_agd_file,
    get_agd_sort_key,
    get_agents_dir,
    get_decisions_dir,
)
from simple_yaml import parse_frontmatter, serialize_frontmatter


def empty_managed_reverse_refs() -> dict[str, set[str]]:
    """Create a fresh mapping for auto-managed reverse-reference fields."""
    return {managed_field: set() for managed_field in REVERSE_REF_FIELDS.values()}


def collect_agd_data(decisions_dir: Path) -> tuple[list, list, dict]:
    """Collect tags and relations data from all AGD files."""
    tags_data = []       # [(relative_path, [tags])]
    relations_data = []  # [(source_path, target_path, relation_type)]
    reverse_refs = {}    # {target_file: {'updated_by': set(), 'obsoleted_by': set()}}

    for agd_file in sorted(decisions_dir.glob(AGD_PATTERN), key=lambda f: get_agd_sort_key(f.name)):
        try:
            content = agd_file.read_text()
        except IOError:
            continue

        frontmatter, _ = parse_frontmatter(content)
        relative_path = f"{DECISIONS_DIR}/{agd_file.name}"

        # Collect tags
        if 'tags' in frontmatter and frontmatter['tags']:
            tags = [f"#{t.strip()}" for t in frontmatter['tags'].split(',') if t.strip()]
            if tags:
                tags_data.append((relative_path, tags))

        # Collect relationships
        for field, rel_type in RELATION_FIELDS:
            if field in frontmatter and frontmatter[field]:
                refs = [r.strip() for r in frontmatter[field].split(',') if r.strip()]
                for ref in refs:
                    target_file = find_agd_file(decisions_dir, ref)
                    if target_file:
                        target_path = f"{DECISIONS_DIR}/{target_file.name}"
                        relations_data.append((relative_path, target_path, rel_type))

                        # Build reverse mapping for managed reverse-reference fields only
                        reverse_field = REVERSE_REF_FIELDS.get(field)
                        if not reverse_field:
                            continue

                        if target_file not in reverse_refs:
                            reverse_refs[target_file] = empty_managed_reverse_refs()

                        from utils import get_agd_id
                        source_id = get_agd_id(agd_file.name)
                        if source_id:
                            reverse_refs[target_file][reverse_field].add(source_id)

    return tags_data, relations_data, reverse_refs


def write_tags_index(agents_dir: Path, tags_data: list) -> None:
    """Write INDEX-TAGS.md file."""
    content = "# Tags Index\n\n"
    content += "<!-- AUTO-GENERATED - DO NOT EDIT -->\n"
    content += "<!-- Search with: grep \"#tagname\" INDEX-TAGS.md -->\n\n"

    for path, tags in sorted(tags_data, key=lambda x: get_agd_sort_key(x[0])):
        content += f"{path}: {', '.join(tags)}\n"

    (agents_dir / 'INDEX-TAGS.md').write_text(content)


def write_relations_index(agents_dir: Path, relations_data: list) -> None:
    """Write INDEX-AGD-RELATIONS.md file."""
    content = "# AGD Relations Index\n\n"
    content += "<!-- AUTO-GENERATED - DO NOT EDIT -->\n"
    content += "<!-- -(o)-> : obsoletes, -(u)-> : updates, -(r)-> : related -->\n"
    content += "<!-- Search with: grep \"AGD-001\" INDEX-AGD-RELATIONS.md -->\n\n"

    for source, target, rel_type in sorted(relations_data, key=lambda x: get_agd_sort_key(x[0])):
        content += f"{source} -({rel_type})-> {target}\n"

    (agents_dir / 'INDEX-AGD-RELATIONS.md').write_text(content)


def sync_reverse_references(decisions_dir: Path, reverse_refs: dict) -> int:
    """
    Rewrite managed reverse references on all AGD files.

    The managed fields (`updated_by`, `obsoleted_by`) are derived entirely from
    forward references, so this step both adds missing values and prunes stale
    ones.

    Returns number of files modified.
    """
    modified_count = 0

    for agd_file in sorted(decisions_dir.glob(AGD_PATTERN), key=lambda f: get_agd_sort_key(f.name)):
        try:
            content = agd_file.read_text()
        except IOError:
            continue

        refs = reverse_refs.get(agd_file, empty_managed_reverse_refs())

        # Parse current frontmatter
        frontmatter, _ = parse_frontmatter(content)

        # Extract body (everything after second ---)
        if content.startswith('---'):
            parts = content.split('---', 2)
            body = parts[2] if len(parts) >= 3 else ''
        else:
            body = content

        needs_update = False
        for field in REVERSE_REF_FIELDS.values():
            computed_refs = ', '.join(sorted(refs[field], key=get_agd_sort_key))
            existing_refs = frontmatter.get(field)

            if computed_refs:
                if existing_refs != computed_refs:
                    frontmatter[field] = computed_refs
                    needs_update = True
            elif field in frontmatter:
                del frontmatter[field]
                needs_update = True

        if not needs_update:
            continue

        # Write atomically using temp file + rename
        new_content = serialize_frontmatter(frontmatter, body)

        with tempfile.NamedTemporaryFile(
            mode='w',
            dir=agd_file.parent,
            delete=False,
            prefix='.tmp_',
            suffix='.md'
        ) as tmp:
            tmp.write(new_content)
            tmp_path = Path(tmp.name)

        # Atomic rename
        tmp_path.replace(agd_file)
        modified_count += 1

    return modified_count


def generate_indexes(project_dir: Path) -> None:
    """Generate all index files and sync bidirectional references."""
    agents_dir = get_agents_dir(project_dir)
    decisions_dir = get_decisions_dir(project_dir)

    if not decisions_dir.exists():
        return

    tags_data, relations_data, reverse_refs = collect_agd_data(decisions_dir)
    write_tags_index(agents_dir, tags_data)
    write_relations_index(agents_dir, relations_data)

    # Sync reverse references back to target files
    modified_count = sync_reverse_references(decisions_dir, reverse_refs)
    if modified_count > 0:
        print(f"Updated {modified_count} AGD file(s) with reverse references")


def main():
    # Get project directory from: 1) argument, 2) CLAUDE_PROJECT_DIR env, 3) current directory
    if len(sys.argv) >= 2:
        project_dir = Path(sys.argv[1])
    elif 'CLAUDE_PROJECT_DIR' in os.environ:
        project_dir = Path(os.environ['CLAUDE_PROJECT_DIR'])
    else:
        project_dir = Path(os.getcwd())
    generate_indexes(project_dir)
    sys.exit(0)


if __name__ == '__main__':
    main()
