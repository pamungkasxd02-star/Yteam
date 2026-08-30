#!/usr/bin/env python3
"""Install the standalone YTEAM runtime and the user-local launcher."""

from __future__ import annotations

import argparse
import os
import shutil
import stat
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
VENV = ROOT / "runtime" / ".venv"


def default_bin() -> Path:
    configured = os.environ.get("YTEAM_BIN_DIR")
    if configured:
        return Path(configured).expanduser().resolve()
    if os.name == "nt":
        return (Path.home() / "bin").resolve()
    return (Path.home() / ".local" / "bin").resolve()


def launcher_text(target: Path) -> str:
    if os.name == "nt":
        return f'@echo off\npython "{target / "scripts" / "yteam_tui.py"}" %*\n'
    return f'#!/usr/bin/env sh\nset -eu\nexec python3 "{target / "scripts" / "yteam_tui.py"}" "$@"\n'


def control_launcher_text(target: Path) -> str:
    if os.name == "nt":
        return f'@echo off\npython "{target / "scripts" / "yteam_control.py"}" %*\n'
    return f'#!/usr/bin/env sh\nset -eu\nexec python3 "{target / "scripts" / "yteam_control.py"}" "$@"\n'


def install(destination: Path) -> Path:
    destination.mkdir(parents=True, exist_ok=True)
    name = "yteam.cmd" if os.name == "nt" else "yteam"
    path = destination / name
    path.write_text(launcher_text(ROOT), encoding="utf-8", newline="\n")
    if os.name != "nt":
        path.chmod(path.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    return path


def install_control(destination: Path) -> Path:
    destination.mkdir(parents=True, exist_ok=True)
    name = "yteam-control.cmd" if os.name == "nt" else "yteam-control"
    path = destination / name
    path.write_text(control_launcher_text(ROOT), encoding="utf-8", newline="\n")
    if os.name != "nt":
        path.chmod(path.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    return path


def run_command(command: list[str], cwd: Path | None = None, env: dict[str, str] | None = None) -> None:
    print("+", " ".join(command))
    result = subprocess.run(command, cwd=cwd, env=env, check=False)
    if result.returncode != 0:
        raise RuntimeError(f"command failed with exit code {result.returncode}: {' '.join(command)}")


def python_in_venv() -> Path:
    return VENV / ("Scripts/python.exe" if os.name == "nt" else "bin/python")


def find_uv() -> str | None:
    return shutil.which("uv")


def ensure_uv() -> str:
    uv = find_uv()
    if uv:
        return uv
    run_command([sys.executable, "-m", "pip", "install", "--user", "uv"])
    uv = find_uv()
    if uv:
        return uv
    user_bin = Path.home() / (".local" / "bin" if os.name != "nt" else "AppData" / "Roaming" / "Python" / "Python311" / "Scripts")
    candidate = user_bin / ("uv.exe" if os.name == "nt" else "uv")
    if candidate.exists():
        return str(candidate)
    raise RuntimeError("uv was installed but is not on PATH; add its user bin directory and run the installer again.")


def install_dependencies(uv: str, fetch_browser: bool) -> None:
    if not python_in_venv().exists():
        run_command([uv, "venv", "--python", "3.11", str(VENV)], cwd=ROOT)
    run_command([uv, "pip", "install", "--python", str(python_in_venv()), "-r", str(ROOT / "requirements.txt")], cwd=ROOT)
    if fetch_browser:
        cache = Path(os.environ.get("CAMOUFOX_CACHE", str(ROOT / "runtime" / "cache" / "camoufox"))).expanduser().resolve()
        cache.mkdir(parents=True, exist_ok=True)
        env = os.environ.copy()
        env["PLAYWRIGHT_BROWSERS_PATH"] = str(cache)
        run_command([str(python_in_venv()), "-m", "camoufox", "fetch"], cwd=ROOT, env=env)


def setup(args: argparse.Namespace) -> Path:
    if not (ROOT / "scripts" / "yteam_tui.py").exists():
        raise RuntimeError(f"not a YTEAM checkout: {ROOT}")
    if args.dry_run:
        print(f"YTEAM root: {ROOT}")
        print("Would install the standalone YTEAM runtime and native TUI; no upstream vendor checkout is required.")
        return (args.bin_dir or default_bin()).resolve() / ("yteam.cmd" if os.name == "nt" else "yteam")
    uv = ensure_uv()
    install_dependencies(uv, not args.skip_browser_download)
    destination = (args.bin_dir or default_bin()).resolve()
    path = install(destination)
    control_path = install_control(destination)
    print(f"Installed YTEAM launcher: {path}")
    print(f"Installed YTEAM control launcher: {control_path}")
    print("Global OpenCode was not modified.")
    return path


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bin-dir", type=Path, help="User-local bin directory")
    parser.add_argument("--skip-browser-download", action="store_true", help="Install Camoufox package but do not download its browser")
    parser.add_argument("--dry-run", action="store_true", help="Show setup plan without downloading or modifying anything")
    args = parser.parse_args()
    try:
        path = setup(args)
    except (OSError, RuntimeError) as error:
        print(f"install_yteam: {error}", file=sys.stderr)
        return 2
    if not args.dry_run:
        print("Add its parent directory to PATH once, then open a new terminal.")
        if os.name == "nt":
            print(f'PowerShell: [Environment]::SetEnvironmentVariable("Path", $env:Path + ";{path.parent}", "User")')
        else:
            print(f'POSIX shell: export PATH="{path.parent}:$PATH"')
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
