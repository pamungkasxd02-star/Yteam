#!/usr/bin/env python3
"""Fetch Yteam's upstream source dependencies into vendor/.

Uses shallow Git checkouts. Cybermes is sparse-checked out to avoid Windows
path-length failures in its optional knowledge image corpus while retaining
the Go engine, tests, skills, tools, docs, metadata, and selected text
knowledge sources used by its search tests.
"""

from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
VENDOR = ROOT / "vendor"
SOURCES = {
    "opencode": ("https://github.com/anomalyco/opencode.git", "dev"),
    "hermes-agent": ("https://github.com/NousResearch/hermes-agent.git", "main"),
    "cybermes": ("https://github.com/Zyrexnn/Cybermes.git", "main"),
}
CYBERMES_SPARSE = [
    ".hermes", "cmd", "docs", "examples", "pkg", "scripts", "skills",
    "templates", "tools", "targets", "assets", "README.md", "AGENTS.md",
    "ATTRIBUTION.md", "LICENSE", "ROADMAP.md", "go.mod", "go.sum",
    "package.json", "pyproject.toml", "requirements.txt", "scope.yaml",
    # Keep text-only JWT/auth sources needed by Cybermes's knowledge search.
    "knowledge/PayloadsAllTheThings/JSON Web Token",
    "knowledge/hack-skills/skills/jwt-oauth-token-attacks",
    "knowledge/Claude-BugHunter/skills/hunt-jwt-crypto",
    "knowledge/strix-skills/vulnerabilities/authentication_jwt.md",
]


def run(command: list[str], cwd: Path | None = None) -> None:
    result = subprocess.run(command, cwd=cwd, check=False)
    if result.returncode != 0:
        raise RuntimeError(f"command failed ({result.returncode}): {' '.join(command)}")


def clone(name: str, url: str, branch: str, refresh: bool) -> None:
    destination = VENDOR / name
    if destination.exists() and (destination / ".git").exists():
        if refresh:
            run(["git", "-C", str(destination), "fetch", "--depth", "1", "origin", branch])
            run(["git", "-C", str(destination), "checkout", branch])
            run(["git", "-C", str(destination), "reset", "--hard", f"origin/{branch}"])
        return
    if destination.exists():
        raise RuntimeError(f"destination exists but is not a Git checkout: {destination}")
    if name == "cybermes":
        run(["git", "clone", "--filter=blob:none", "--no-checkout", "--sparse", "--branch", branch, url, str(destination)])
        run(["git", "-C", str(destination), "config", "core.longpaths", "true"])
        run(["git", "-C", str(destination), "sparse-checkout", "set", "--skip-checks", *CYBERMES_SPARSE])
        run(["git", "-C", str(destination), "checkout", branch])
        return
    run(["git", "clone", "--depth", "1", "--branch", branch, url, str(destination)])


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--refresh", action="store_true", help="fetch and reset existing vendor checkouts")
    args = parser.parse_args()
    VENDOR.mkdir(parents=True, exist_ok=True)
    try:
        for name, (url, branch) in SOURCES.items():
            clone(name, url, branch, args.refresh)
    except (OSError, RuntimeError) as error:
        print(f"bootstrap_sources: {error}", file=sys.stderr)
        return 2
    print("Yteam upstream sources are ready under vendor/.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
