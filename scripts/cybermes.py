#!/usr/bin/env python3
"""Run Cybermes Go utilities directly from the Yteam workspace."""

from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CYBERMES = ROOT / "vendor" / "cybermes"
COMMANDS = {
    "smart-pipe": "smart_pipe",
    "secret-scan": "secret_scan",
    "search-knowledge": "search_knowledge",
    "aggregate-reports": "aggregate_reports",
}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=sorted(COMMANDS))
    parser.add_argument("args", nargs=argparse.REMAINDER)
    parsed = parser.parse_args()
    if not CYBERMES.exists():
        print(f"Cybermes source checkout is missing: {CYBERMES}", file=sys.stderr)
        return 2
    name = COMMANDS[parsed.command]
    suffix = ".exe" if os.name == "nt" else ""
    binary = ROOT / "runtime" / "bin" / f"{name}{suffix}"
    if binary.exists():
        command = [str(binary), *parsed.args]
    else:
        go = shutil.which("go")
        if not go:
            print("Go is required, or build the selected utility into runtime/bin.", file=sys.stderr)
            return 2
        command = [go, "run", f"./cmd/{name}", *parsed.args]
    environment = os.environ.copy()
    environment.setdefault("CYBERMES_ROOT", str(CYBERMES))
    return subprocess.run(command, cwd=CYBERMES, env=environment, check=False).returncode


if __name__ == "__main__":
    raise SystemExit(main())
