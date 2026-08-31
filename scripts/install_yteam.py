#!/usr/bin/env python3
"""Install the standalone YTEAM runtime and the user-local launcher."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import stat
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
VENV = ROOT / "runtime" / ".venv"
SUPPORTED_PYTHON = {(3, 11), (3, 12), (3, 13)}
INSTALL_MANIFEST = ROOT / "runtime" / "install-manifest.json"


def venv_python(target: Path = ROOT) -> Path:
    """Return the interpreter installed inside this checkout."""
    virtualenv = target / "runtime" / ".venv"
    return virtualenv / ("Scripts" if os.name == "nt" else "bin") / ("python.exe" if os.name == "nt" else "python")


def _quote_command_path(path: Path) -> str:
    """Return a safe absolute path for a generated launcher."""
    return str(path.resolve()).replace('"', '')


def _quote_powershell_path(path: Path) -> str:
    """Quote a filesystem path for a single-quoted PowerShell literal."""
    return str(path.resolve()).replace("'", "''")


def default_bin() -> Path:
    configured = os.environ.get("YTEAM_BIN_DIR")
    if configured:
        return Path(configured).expanduser().resolve()
    if os.name == "nt":
        return (Path.home() / "bin").resolve()
    return (Path.home() / ".local" / "bin").resolve()


def validate_python() -> None:
    version = (sys.version_info.major, sys.version_info.minor)
    if version not in SUPPORTED_PYTHON:
        supported = ", ".join(f"{major}.{minor}" for major, minor in sorted(SUPPORTED_PYTHON))
        raise RuntimeError(
            f"Python {version[0]}.{version[1]} is not supported; use Python {supported}. "
            "On Windows try 'py -3.12', on macOS/Linux try 'python3.12'."
        )


def _atomic_write(path: Path, content: str, *, executable: bool = False) -> None:
    """Write a launcher atomically so an interrupted install cannot corrupt it."""
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(f".{path.name}.tmp-{os.getpid()}")
    temporary.write_text(content, encoding="utf-8", newline="\n")
    if executable and os.name != "nt":
        temporary.chmod(temporary.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    os.replace(temporary, path)


def launcher_text(target: Path) -> str:
    """Interactive TUI launcher: restart automatically on crash, stop on /quit."""
    python = _quote_command_path(venv_python(target))
    script = _quote_command_path(target / "scripts" / "yteam_tui.py")
    root = _quote_command_path(target)
    if os.name == "nt":
        marker = root + "\\runtime\\quit.marker"
    else:
        marker = root + "/runtime/quit.marker"
    if os.name == "nt":
        return (
            "@echo off\n"
            "setlocal\n"
            f'set "YTEAM_PYTHON={python}"\n'
            f'set "QUIT_MARKER={marker}"\n'
            ":loop\n"
            'del /q "%QUIT_MARKER%" 2>nul\n'
            f'"%YTEAM_PYTHON%" "{script}" %*\n'
            "set RC=%ERRORLEVEL%\n"
            'if exist "%QUIT_MARKER%" (\n'
            '  del /q "%QUIT_MARKER%" 2>nul\n'
            "  exit /b 0\n"
            ")\n"
            "echo.\n"
            "echo  [!] YTEAM keluar sendiri (rc=%RC%) - restarting otomatis...\n"
            "echo  Tekan Ctrl+C berulang kali untuk berhenti.\n"
            "timeout /t 2 /nobreak >nul\n"
            "goto loop\n"
        )
    return (
        "#!/usr/bin/env sh\n"
        "set -eu\n"
        f'PYTHON="{python}"\n'
        f'SCRIPT="{script}"\n'
        f'QUIT_MARKER="{marker}"\n'
        "while :; do\n"
        '  rm -f "$QUIT_MARKER"\n'
        "  set +e\n"
        '  "$PYTHON" "$SCRIPT" "$@"\n'
        "  RC=$?\n"
        "  set -e\n"
        '  if [ -f "$QUIT_MARKER" ]; then\n'
        '    rm -f "$QUIT_MARKER"\n'
        "    exit 0\n"
        "  fi\n"
        "  echo ''\n"
        "  echo \"[!] YTEAM keluar sendiri (rc=$RC) - restarting otomatis...\"\n"
        "  echo 'Tekan Ctrl+C berulang kali untuk berhenti.'\n"
        "  sleep 2\n"
        "done\n"
    )


def control_launcher_text(target: Path) -> str:
    python = _quote_command_path(venv_python(target))
    script = _quote_command_path(target / "scripts" / "yteam_control.py")
    if os.name == "nt":
        return f'@echo off\n"{python}" "{script}" %*\n'
    return f'#!/usr/bin/env sh\nset -eu\nexec "{python}" "{script}" "$@"\n'


def worker_launcher_text(target: Path) -> str:
    python = _quote_command_path(venv_python(target))
    script = _quote_command_path(target / "scripts" / "yteam_worker.py")
    if os.name == "nt":
        return f'@echo off\n"{python}" "{script}" %*\n'
    return f'#!/usr/bin/env sh\nset -eu\nexec "{python}" "{script}" "$@"\n'


def localsolver_launcher_text(target: Path) -> str:
    python = _quote_command_path(venv_python(target))
    script = _quote_command_path(target / "scripts" / "localsolver.py")
    if os.name == "nt":
        return f'@echo off\n"{python}" "{script}" %*\n'
    return f'#!/usr/bin/env sh\nset -eu\nexec "{python}" "{script}" "$@"\n'


def mcp_launcher_text(target: Path) -> str:
    python = _quote_command_path(venv_python(target))
    script = _quote_command_path(target / "scripts" / "yteam_mcp.py")
    if os.name == "nt":
        return f'@echo off\n"{python}" "{script}" %*\n'
    return f'#!/usr/bin/env sh\nset -eu\nexec "{python}" "{script}" "$@"\n'


def doctor_launcher_text(target: Path) -> str:
    python = _quote_command_path(venv_python(target))
    script = _quote_command_path(target / "scripts" / "yteam_doctor.py")
    if os.name == "nt":
        return f'@echo off\n"{python}" "{script}" %*\n'
    return f'#!/usr/bin/env sh\nset -eu\nexec "{python}" "{script}" "$@"\n'


def install(destination: Path) -> Path:
    destination.mkdir(parents=True, exist_ok=True)
    name = "yteam.cmd" if os.name == "nt" else "yteam"
    path = destination / name
    _atomic_write(path, launcher_text(ROOT), executable=True)
    return path


def install_control(destination: Path) -> Path:
    destination.mkdir(parents=True, exist_ok=True)
    name = "yteam-control.cmd" if os.name == "nt" else "yteam-control"
    path = destination / name
    _atomic_write(path, control_launcher_text(ROOT), executable=True)
    return path


def install_worker(destination: Path) -> Path:
    destination.mkdir(parents=True, exist_ok=True)
    name = "yteam-worker.cmd" if os.name == "nt" else "yteam-worker"
    path = destination / name
    _atomic_write(path, worker_launcher_text(ROOT), executable=True)
    return path


def install_localsolver(destination: Path) -> Path:
    destination.mkdir(parents=True, exist_ok=True)
    name = "localsolver.cmd" if os.name == "nt" else "localsolver"
    path = destination / name
    _atomic_write(path, localsolver_launcher_text(ROOT), executable=True)
    return path


def install_mcp(destination: Path) -> Path:
    destination.mkdir(parents=True, exist_ok=True)
    name = "yteam-mcp.cmd" if os.name == "nt" else "yteam-mcp"
    path = destination / name
    _atomic_write(path, mcp_launcher_text(ROOT), executable=True)
    return path


def install_doctor(destination: Path) -> Path:
    destination.mkdir(parents=True, exist_ok=True)
    name = "yteam-doctor.cmd" if os.name == "nt" else "yteam-doctor"
    path = destination / name
    _atomic_write(path, doctor_launcher_text(ROOT), executable=True)
    return path


def run_command(command: list[str], cwd: Path | None = None, env: dict[str, str] | None = None) -> None:
    print("+", " ".join(command))
    result = subprocess.run(command, cwd=cwd, env=env, check=False)
    if result.returncode != 0:
        raise RuntimeError(f"command failed with exit code {result.returncode}: {' '.join(command)}")


def run_quiet(command: list[str], cwd: Path | None = None, env: dict[str, str] | None = None) -> None:
    """Run optional setup steps without exposing a platform traceback."""
    result = subprocess.run(command, cwd=cwd, env=env, capture_output=True, text=True, check=False)
    if result.returncode:
        details = (result.stderr or result.stdout or "no diagnostic output").strip().splitlines()
        detail = details[-1] if details else "unknown error"
        raise RuntimeError(f"command failed with exit code {result.returncode}: {detail}")


def python_in_venv() -> Path:
    return venv_python()


def find_uv() -> str | None:
    return shutil.which("uv")


def ensure_uv() -> str | None:
    uv = find_uv()
    if uv:
        return uv
    try:
        run_command([sys.executable, "-m", "pip", "install", "--user", "uv"])
    except RuntimeError:
        print("uv is unavailable; falling back to the standard-library venv + pip backend.", file=sys.stderr)
        return None
    uv = find_uv()
    if uv:
        return uv
    user_base = _python_user_base()
    candidate = user_base / ("Scripts" if os.name == "nt" else "bin") / ("uv.exe" if os.name == "nt" else "uv")
    if candidate.exists():
        return str(candidate)
    print("uv was installed outside PATH; falling back to the standard-library venv + pip backend.", file=sys.stderr)
    return None


def _python_user_base() -> Path:
    try:
        base = subprocess.check_output(
            [sys.executable, "-c", "import site; print(site.getuserbase())"],
            text=True,
            cwd=ROOT,
        ).strip()
        return Path(base)
    except (OSError, subprocess.CalledProcessError):
        if os.name == "nt":
            return Path.home() / "AppData" / "Roaming" / "Python" / "Scripts"
        return Path.home() / ".local"


def _path_contains(path_value: str, destination: Path) -> bool:
    wanted = str(destination.resolve()).casefold() if os.name == "nt" else str(destination.resolve())
    return any((item.strip().rstrip("/\\").casefold() if os.name == "nt" else item.strip().rstrip("/\\")) == wanted.rstrip("/\\") for item in path_value.split(os.pathsep) if item.strip())


def persist_user_path(destination: Path) -> tuple[bool, str]:
    """Make the launcher discoverable in future shells on the current OS.

    A child process cannot mutate its parent's environment. We therefore update
    the user's persistent PATH and return the exact one-line refresh command
    needed by the already-open shell. No administrator privileges are needed.
    """
    destination = destination.resolve()
    if os.name == "nt":
        try:
            import winreg

            with winreg.OpenKey(winreg.HKEY_CURRENT_USER, "Environment", 0, winreg.KEY_READ | winreg.KEY_WRITE) as key:
                try:
                    existing, value_type = winreg.QueryValueEx(key, "Path")
                except FileNotFoundError:
                    existing, value_type = "", winreg.REG_EXPAND_SZ
                existing = str(existing or "")
                if not _path_contains(existing, destination):
                    existing = existing.rstrip(";" + os.pathsep) + (os.pathsep if existing else "") + str(destination)
                    winreg.SetValueEx(key, "Path", 0, value_type, existing)
            return True, '$env:Path = [Environment]::GetEnvironmentVariable("Path", "User")'
        except (ImportError, OSError):
            return False, f'$env:Path = [Environment]::GetEnvironmentVariable("Path", "User")'

    current = os.environ.get("PATH", "")
    if _path_contains(current, destination):
        return True, f'export PATH="{destination}:$PATH"'
    shell = Path(os.environ.get("SHELL", "")).name
    rc = Path.home() / (".zshrc" if shell == "zsh" else ".bashrc" if shell in {"bash", "sh", "ksh"} else ".profile")
    marker_start = "# >>> yteam launcher >>>"
    marker_end = "# <<< yteam launcher <<<"
    block = f'{marker_start}\nexport PATH="{destination}:$PATH"\n{marker_end}\n'
    try:
        existing = rc.read_text(encoding="utf-8") if rc.exists() else ""
        if marker_start not in existing:
            rc.parent.mkdir(parents=True, exist_ok=True)
            rc.write_text(existing.rstrip() + "\n\n" + block, encoding="utf-8")
        return True, f'source "{rc}"'
    except OSError:
        return False, f'export PATH="{destination}:$PATH"'


def install_dependencies(uv: str | None, fetch_browser: bool) -> tuple[str, str]:
    backend = "uv" if uv else "pip"
    if not python_in_venv().exists():
        VENV.parent.mkdir(parents=True, exist_ok=True)
        if uv:
            python_version = f"{sys.version_info.major}.{sys.version_info.minor}"
            run_command([uv, "venv", "--python", python_version, str(VENV)], cwd=ROOT)
        else:
            run_command([sys.executable, "-m", "venv", str(VENV)], cwd=ROOT)
    if uv:
        python_version = f"{sys.version_info.major}.{sys.version_info.minor}"
        run_command([uv, "pip", "install", "--python", str(python_in_venv()), "-r", str(ROOT / "requirements.txt")], cwd=ROOT)
    else:
        run_command([str(python_in_venv()), "-m", "pip", "install", "--upgrade", "pip"], cwd=ROOT)
        run_command([str(python_in_venv()), "-m", "pip", "install", "-r", str(ROOT / "requirements.txt")], cwd=ROOT)
    if fetch_browser:
        cache = Path(os.environ.get("CAMOUFOX_CACHE", str(ROOT / "runtime" / "cache" / "camoufox"))).expanduser().resolve()
        cache.mkdir(parents=True, exist_ok=True)
        env = os.environ.copy()
        env["PLAYWRIGHT_BROWSERS_PATH"] = str(ROOT / "runtime" / "cache" / "playwright")
        env["CAMOUFOX_CACHE_DIR"] = str(cache)
        try:
            run_quiet([str(python_in_venv()), "-m", "camoufox", "fetch"], cwd=ROOT, env=env)
            browser_status = "installed"
        except RuntimeError as error:
            browser_status = "deferred"
            print(f"Warning: browser data ditunda ({error}). Core YTEAM tetap terpasang; jalankan 'yteam-doctor --fix' nanti.", file=sys.stderr)
    else:
        browser_status = "skipped"
    return backend, browser_status


def write_install_manifest(destination: Path, backend: str, browser: bool, browser_status: str) -> None:
    manifest = {
        "product": "YTEAM",
        "schema_version": 1,
        "platform": sys.platform,
        "os_name": os.name,
        "python": f"{sys.version_info.major}.{sys.version_info.minor}.{sys.version_info.micro}",
        "venv": str(VENV),
        "launcher_dir": str(destination),
        "package_backend": backend,
        "browser_data_requested": browser,
        "browser_data_status": browser_status,
    }
    INSTALL_MANIFEST.parent.mkdir(parents=True, exist_ok=True)
    _atomic_write(INSTALL_MANIFEST, json.dumps(manifest, indent=2, sort_keys=True) + "\n")


def setup(args: argparse.Namespace) -> Path:
    if not (ROOT / "scripts" / "yteam_tui.py").exists():
        raise RuntimeError(f"not a YTEAM checkout: {ROOT}")
    if args.dry_run:
        print(f"YTEAM root: {ROOT}")
        print("Would install the standalone YTEAM runtime and native TUI; no upstream vendor checkout is required.")
        return (args.bin_dir or default_bin()).resolve() / ("yteam.cmd" if os.name == "nt" else "yteam")
    validate_python()
    uv = ensure_uv()
    browser_requested = not args.skip_browser_download
    backend, browser_status = install_dependencies(uv, browser_requested)
    destination = (args.bin_dir or default_bin()).resolve()
    path = install(destination)
    control_path = install_control(destination)
    worker_path = install_worker(destination)
    localsolver_path = install_localsolver(destination)
    mcp_path = install_mcp(destination)
    doctor_path = install_doctor(destination)
    write_install_manifest(destination, backend, browser_requested, browser_status)
    print(f"Installed YTEAM launcher: {path}")
    print(f"Installed YTEAM control launcher: {control_path}")
    print(f"Installed YTEAM worker launcher: {worker_path}")
    print(f"Installed LocalSolver launcher: {localsolver_path}")
    print(f"Installed YTEAM MCP launcher: {mcp_path}")
    print(f"Installed YTEAM doctor launcher: {doctor_path}")
    print("Global OpenCode was not modified.")
    print(f"Install manifest: {INSTALL_MANIFEST}")
    return path


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bin-dir", type=Path, help="User-local bin directory")
    parser.add_argument("--skip-browser-download", "--no-browser", dest="skip_browser_download", action="store_true", help="Install Python packages without downloading Camoufox browser data")
    parser.add_argument("--repair", action="store_true", help="Reinstall dependencies and refresh all launchers in-place")
    parser.add_argument("--dry-run", action="store_true", help="Show setup plan without downloading or modifying anything")
    args = parser.parse_args()
    try:
        path = setup(args)
    except (OSError, RuntimeError) as error:
        print(f"install_yteam: {error}", file=sys.stderr)
        return 2
    if not args.dry_run:
        persisted, refresh = persist_user_path(path.parent)
        print("Launcher PATH configured automatically for future terminals." if persisted else "Launcher created; automatic PATH persistence was unavailable.")
        print("Refresh the current terminal once, then run YTEAM:")
        if os.name == "nt":
            current_path = _quote_powershell_path(path.parent)
            print(f"$env:Path = '{current_path};' + $env:Path; yteam")
            print(f"Or run immediately (no PATH needed): & '{_quote_powershell_path(path)}'")
        else:
            print(f"{refresh} && yteam")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
