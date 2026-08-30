#!/usr/bin/env python3
"""Install the user-local Yteam launcher without touching global OpenCode."""

from __future__ import annotations

import argparse
import os
import stat
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def default_bin() -> Path:
    configured = os.environ.get("YTEAM_BIN_DIR")
    if configured:
        return Path(configured).expanduser().resolve()
    if os.name == "nt":
        return (Path.home() / "bin").resolve()
    return (Path.home() / ".local" / "bin").resolve()


def launcher_text(target: Path) -> str:
    if os.name == "nt":
        return f'@echo off\npython "{target / "scripts" / "hermes_opencode.py"}" %*\n'
    return f'#!/usr/bin/env sh\nset -eu\nexec python3 "{target / "scripts" / "hermes_opencode.py"}" "$@"\n'


def install(destination: Path) -> Path:
    destination.mkdir(parents=True, exist_ok=True)
    name = "yteam.cmd" if os.name == "nt" else "yteam"
    path = destination / name
    path.write_text(launcher_text(ROOT), encoding="utf-8", newline="\n")
    if os.name != "nt":
        path.chmod(path.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    return path


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bin-dir", type=Path, help="User-local bin directory")
    args = parser.parse_args()
    path = install((args.bin_dir or default_bin()).resolve())
    print(f"Installed Yteam launcher: {path}")
    print("This is a project-local Yteam launcher; the global OpenCode command was not modified.")
    print("Add its parent directory to PATH once, then open a new terminal.")
    if os.name == "nt":
        print(f'PowerShell: [Environment]::SetEnvironmentVariable("Path", $env:Path + ";{path.parent}", "User")')
    else:
        print(f'POSIX shell: export PATH="{path.parent}:$PATH"')
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
