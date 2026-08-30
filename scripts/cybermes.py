#!/usr/bin/env python3
"""Run native YTEAM security utilities."""

from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path

from yteam_native_tools import main as native_tools_main


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
    # Keep the compatibility command names hyphenated; the native utility CLI
    # intentionally exposes the same stable public spelling.
    sys.argv = [sys.argv[0], parsed.command, *parsed.args]
    return native_tools_main()


if __name__ == "__main__":
    raise SystemExit(main())
