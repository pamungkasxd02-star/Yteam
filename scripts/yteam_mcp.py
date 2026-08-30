#!/usr/bin/env python3
"""Native YTEAM MCP stdio server with Cybermes-compatible safe tools.

Read-only metadata/search/scope tools are available by default. Active network
actions are not exposed here; autonomous assessment remains policy-gated via
the durable `/bb` job system.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPTS = ROOT / "scripts"
if str(SCRIPTS) not in sys.path:
    sys.path.insert(0, str(SCRIPTS))


def run_server() -> int:
    try:
        from mcp.server.fastmcp import FastMCP
    except ImportError as error:
        print(f"YTEAM MCP dependency missing: {error}", file=sys.stderr)
        return 2
    from yteam_native_tools import aggregate_reports, filter_stream, secret_scan
    from yteam_scope import validate
    from yteam_skills import get_skill, registry

    server = FastMCP("yteam-security")

    @server.tool()
    def yteam_list_skills(filter: str = "", limit: int = 30) -> dict[str, object]:
        """List metadata for native and configured Cybermes-compatible skills."""
        items = registry()
        wanted = filter.lower().strip()
        if wanted:
            items = [item for item in items if wanted in (str(item["name"]) + " " + str(item["description"])).lower()]
        return {"total_skills": len(registry()), "matched": len(items), "skills": items[:max(1, min(limit, 200))]}

    @server.tool()
    def yteam_get_skill(skill_name: str, section: str = "") -> dict[str, object]:
        """Load one safety-classified skill or return metadata-only quarantine."""
        return get_skill(registry(), skill_name, section, False)

    @server.tool()
    def yteam_validate_scope(target: str, target_slug: str = "") -> dict[str, object]:
        """Validate an exact target against the YTEAM scope policy."""
        return validate(target, target_slug).__dict__

    @server.tool()
    def yteam_filter_stream(content: str, limit: int = 25) -> dict[str, object]:
        """Return bounded, redacted high-signal lines from raw tool output."""
        return {"lines": filter_stream(content.splitlines(), max(1, min(limit, 100)))}

    @server.tool()
    def yteam_scan_secrets(content: str) -> dict[str, object]:
        """Detect and mask secret-like content without returning raw values."""
        return {"matches": secret_scan(content)}

    @server.tool()
    def yteam_aggregate_report(target_path: str) -> dict[str, object]:
        """Aggregate confirmed Markdown findings without executing PoC files."""
        path = Path(target_path).expanduser().resolve()
        allowed_root = (ROOT / "reports").resolve()
        if not path.is_relative_to(allowed_root):
            raise ValueError("target_path must remain under the YTEAM reports directory")
        return aggregate_reports(path)

    server.run(transport="stdio")
    return 0


def main() -> int:
    argparse.ArgumentParser(description=__doc__).parse_args()
    return run_server()


if __name__ == "__main__":
    raise SystemExit(main())
