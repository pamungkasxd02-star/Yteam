#!/usr/bin/env python3
"""Initialize a non-secret Yteam Hermes profile without overwriting user data."""

from __future__ import annotations

import argparse
import os
import shutil
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def default_home() -> Path:
    configured = os.environ.get("YTEAM_HERMES_HOME")
    if configured:
        return Path(configured).expanduser().resolve()
    existing = os.environ.get("HERMES_HOME")
    if existing:
        return Path(existing).expanduser().resolve()
    return (ROOT / "runtime" / "yteam-hermes-home").resolve()


def copy_if_missing(source: Path, target: Path) -> None:
    if target.exists():
        return
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, target)


def initialize(home: Path | None = None) -> Path:
    target = (home or default_home()).resolve()
    target.mkdir(parents=True, exist_ok=True)
    copy_if_missing(ROOT / "profile" / "SOUL.md", target / "SOUL.md")
    copy_if_missing(ROOT / "profile" / "memories" / "MEMORY.md", target / "memories" / "MEMORY.md")
    copy_if_missing(ROOT / "profile" / "memories" / "USER.md", target / "memories" / "USER.md")
    config_path = target / "config.yaml"
    if not config_path.exists():
        config = (ROOT / "profile" / "config.yaml").read_text(encoding="utf-8")
        config = config.replace("${YTEAM_PYTHON}", sys.executable)
        config_path.write_text(config, encoding="utf-8")
    (target / "skills").mkdir(exist_ok=True)
    (target / "logs").mkdir(exist_ok=True)
    (target / "sessions").mkdir(exist_ok=True)
    return target


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--home", type=Path, help="Yteam Hermes home; defaults to runtime/yteam-hermes-home")
    args = parser.parse_args()
    print(initialize(args.home))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
