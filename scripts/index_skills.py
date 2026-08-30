#!/usr/bin/env python3
"""Build a compact metadata catalog for the combined skill sources."""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SOURCE_DIRS = (
    ROOT / ".opencode" / "skills",
    ROOT / ".." / ".agents" / "skills",
    ROOT / ".." / ".opencode" / "skills",
    ROOT / "vendor" / "hermes-agent" / "skills",
    ROOT / "vendor" / "hermes-agent" / "optional-skills" / "security",
    ROOT / "vendor" / "cybermes" / "skills",
)


def frontmatter_value(text: str, key: str) -> str:
    match = re.search(rf"^\s*{re.escape(key)}:\s*(.*?)\s*$", text, re.MULTILINE)
    return match.group(1).strip().strip("\"'") if match else ""


def category_for(skill_file: Path, source: Path) -> str:
    relative = skill_file.parent.relative_to(source)
    parts = relative.parts
    return parts[0] if len(parts) > 1 else "root"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", default="runtime/skill-catalog.json")
    args = parser.parse_args()
    entries: dict[str, dict[str, str]] = {}
    for source in SOURCE_DIRS:
        if not source.exists():
            continue
        for skill_file in source.rglob("SKILL.md"):
            text = skill_file.read_text(encoding="utf-8", errors="replace")
            name = frontmatter_value(text, "name") or skill_file.parent.name
            entries.setdefault(
                name,
                {
                    "name": name,
                    "description": frontmatter_value(text, "description"),
                    "path": str(skill_file.relative_to(ROOT)).replace("\\", "/"),
                    "source": str(source.relative_to(ROOT)).replace("\\", "/"),
                    "category": category_for(skill_file, source),
                },
            )
    output = (ROOT / args.output).resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(sorted(entries.values(), key=lambda item: item["name"]), indent=2) + "\n", encoding="utf-8")
    print(f"Indexed {len(entries)} skills -> {output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
