#!/usr/bin/env python3
"""Remove YTEAM launchers without touching assessment data by default."""

from __future__ import annotations

import argparse
import os
import shutil
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def default_bin() -> Path:
    configured = os.environ.get("YTEAM_BIN_DIR")
    if configured:
        return Path(configured).expanduser().resolve()
    return (Path.home() / ("bin" if os.name == "nt" else ".local/bin")).resolve()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--purge-runtime", action="store_true", help="Also remove the isolated virtualenv and caches; preserves runtime state")
    parser.add_argument("--yes", action="store_true", help="Do not ask for confirmation")
    args = parser.parse_args()
    if not args.yes:
        answer = input("Remove YTEAM launchers from this user account? [y/N] ").strip().lower()
        if answer not in {"y", "yes"}:
            print("Cancelled.")
            return 0
    names = ["yteam", "yteam.cmd", "yteam-control", "yteam-control.cmd", "yteam-worker", "yteam-worker.cmd", "localsolver", "localsolver.cmd", "yteam-mcp", "yteam-mcp.cmd", "yteam-doctor", "yteam-doctor.cmd"]
    removed = 0
    for name in names:
        path = default_bin() / name
        if path.exists() or path.is_symlink():
            path.unlink()
            removed += 1
    if args.purge_runtime:
        for target in (ROOT / "runtime" / ".venv", ROOT / "runtime" / "cache"):
            if target.exists():
                shutil.rmtree(target)
    print(f"Removed {removed} launcher(s). Assessment state and reports were preserved.")
    if args.purge_runtime:
        print("Removed isolated runtime and caches; repository files remain available for reinstall.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
