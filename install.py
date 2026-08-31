#!/usr/bin/env python3
"""Cross-platform YTEAM bootstrapper.

Run this file from a checkout, or download it by itself.  In the latter case
it clones YTEAM into a user-local directory and delegates to the native
installer.  No administrator privileges and no global OpenCode installation
are required.
"""

from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys
from pathlib import Path


REPOSITORY = "https://github.com/pamungkasxd02-star/Yteam.git"
SCRIPT_ROOT = Path(__file__).resolve().parent


def default_checkout() -> Path:
    configured = os.environ.get("YTEAM_HOME")
    if configured:
        return Path(configured).expanduser().resolve()
    if os.name == "nt":
        base = Path(os.environ.get("LOCALAPPDATA", str(Path.home() / "AppData" / "Local")))
        return (base / "Yteam").resolve()
    return (Path.home() / ".yteam").resolve()


def run(command: list[str], cwd: Path | None = None) -> None:
    print("+", " ".join(command))
    result = subprocess.run(command, cwd=cwd, check=False)
    if result.returncode:
        raise RuntimeError(f"command failed with exit code {result.returncode}: {' '.join(command)}")


def checkout(target: Path, dry_run: bool) -> Path:
    if (SCRIPT_ROOT / "scripts" / "install_yteam.py").exists():
        return SCRIPT_ROOT
    if target.exists() and not (target / ".git").exists():
        raise RuntimeError(f"install directory exists but is not a Git checkout: {target}")
    git = shutil.which("git")
    if not git:
        raise RuntimeError("Git is required for bootstrap; install Git 2.40+ and run again.")
    if not target.exists():
        if dry_run:
            print(f"Would clone {REPOSITORY} into {target}")
            return target
        target.parent.mkdir(parents=True, exist_ok=True)
        run([git, "clone", "--depth", "1", REPOSITORY, str(target)])
    return target


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo-dir", type=Path, help="User-local checkout directory when bootstrapping")
    parser.add_argument("--dry-run", action="store_true", help="Show bootstrap/install actions without changing files")
    args, forwarded = parser.parse_known_args()
    try:
        root = checkout((args.repo_dir or default_checkout()).resolve(), args.dry_run)
        if args.dry_run and not (root / "scripts" / "install_yteam.py").exists():
            return 0
        command = [sys.executable, str(root / "scripts" / "install_yteam.py"), *forwarded]
        if args.dry_run:
            command.append("--dry-run")
        run(command, cwd=root)
    except (OSError, RuntimeError) as error:
        print(f"yteam bootstrap: {error}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
