#!/usr/bin/env python3
"""Install YTEAM dependencies and the user-local launcher.

The installer owns only this repository's vendor checkouts, Python environment,
optional browser cache, and user-local launcher. It never modifies a global
OpenCode installation.
"""

from __future__ import annotations

import argparse
import os
import shutil
import stat
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
HERMES_ROOT = ROOT / "vendor" / "hermes-agent"
OPENCODE_ROOT = ROOT / "vendor" / "opencode"
VENV = HERMES_ROOT / ".venv"


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


def run_command(command: list[str], cwd: Path | None = None, env: dict[str, str] | None = None) -> None:
    print("+", " ".join(command))
    result = subprocess.run(command, cwd=cwd, env=env, check=False)
    if result.returncode != 0:
        raise RuntimeError(f"command failed with exit code {result.returncode}: {' '.join(command)}")


def python_in_venv() -> Path:
    return VENV / ("Scripts/python.exe" if os.name == "nt" else "bin/python")


def find_bun() -> str | None:
    configured = os.environ.get("BUN_BIN")
    if configured and Path(configured).exists():
        return configured
    return shutil.which("bun")


def install_bun() -> str:
    existing = find_bun()
    if existing:
        return existing
    install_dir = Path(os.environ.get("BUN_INSTALL", str(ROOT / "runtime" / "bun"))).expanduser().resolve()
    env = os.environ.copy()
    env["BUN_INSTALL"] = str(install_dir)
    if os.name == "nt":
        run_command(["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "irm https://bun.sh/install.ps1 | iex"], env=env)
        candidates = [install_dir / "bin" / "bun.exe", Path.home() / ".bun" / "bin" / "bun.exe"]
    else:
        run_command(["sh", "-c", "curl -fsSL https://bun.sh/install | bash"], env=env)
        candidates = [install_dir / "bin" / "bun", Path.home() / ".bun" / "bin" / "bun"]
    for candidate in candidates:
        if candidate.exists():
            return str(candidate)
    raise RuntimeError("Bun installation finished but the executable was not found; set BUN_BIN manually.")


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


def bootstrap_sources(full_sources: bool) -> None:
    profile = "full" if full_sources else "runtime"
    run_command([sys.executable, str(ROOT / "scripts" / "bootstrap_sources.py"), "--profile", profile], cwd=ROOT)


def install_dependencies(uv: str, bun: str, fetch_browser: bool) -> None:
    if not python_in_venv().exists():
        run_command([uv, "venv", "--python", "3.11", str(VENV)], cwd=HERMES_ROOT)
    run_command([uv, "pip", "install", "--python", str(python_in_venv()), "-e", ".[all]"], cwd=HERMES_ROOT)
    run_command([uv, "pip", "install", "--python", str(python_in_venv()), "-r", str(ROOT / "requirements.txt")], cwd=ROOT)
    bun_env = os.environ.copy()
    bun_env["PATH"] = str(Path(bun).resolve().parent) + os.pathsep + bun_env.get("PATH", "")
    # Install only the OpenCode TUI workspace and its runtime workspace
    # dependencies. A root-wide install also resolves unrelated console/stats
    # packages (including ephemeral pkg.pr.new previews) that are not shipped
    # by YTEAM's sparse runtime profile.
    run_command([bun, "install", "--frozen-lockfile", "--filter", "opencode"], cwd=OPENCODE_ROOT, env=bun_env)
    if fetch_browser:
        cache = Path(os.environ.get("CAMOUFOX_CACHE", str(ROOT / "runtime" / "cache" / "camoufox"))).expanduser().resolve()
        cache.mkdir(parents=True, exist_ok=True)
        env = os.environ.copy()
        env["PLAYWRIGHT_BROWSERS_PATH"] = str(cache)
        run_command([str(python_in_venv()), "-m", "camoufox", "fetch"], cwd=ROOT, env=env)


def setup(args: argparse.Namespace) -> Path:
    if not (ROOT / "scripts" / "bootstrap_sources.py").exists():
        raise RuntimeError(f"not a YTEAM checkout: {ROOT}")
    if args.dry_run:
        print(f"YTEAM root: {ROOT}")
        profile = "full" if args.full_sources else "runtime"
        print(f"Would bootstrap the {profile} upstream source profile, install Hermes, install Bun/OpenCode dependencies, and install the launcher.")
        return (args.bin_dir or default_bin()).resolve() / ("yteam.cmd" if os.name == "nt" else "yteam")
    bootstrap_sources(args.full_sources)
    uv = ensure_uv()
    bun = find_bun() or install_bun()
    install_dependencies(uv, bun, not args.skip_browser_download)
    path = install((args.bin_dir or default_bin()).resolve())
    print(f"Installed YTEAM launcher: {path}")
    print("Global OpenCode was not modified.")
    return path


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bin-dir", type=Path, help="User-local bin directory")
    parser.add_argument("--skip-browser-download", action="store_true", help="Install Camoufox package but do not download its browser")
    parser.add_argument("--full-sources", action="store_true", help="Fetch all upstream source, tests, and docs (developer/CI mode)")
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
