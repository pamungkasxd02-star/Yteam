#!/usr/bin/env python3
"""Run the Yteam TUI: upstream OpenCode UI with the upstream Hermes backend."""

from __future__ import annotations

import os
import shutil
import signal
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from secrets import token_urlsafe

try:
    import yaml
except ImportError:  # Optional convenience parser; launcher remains importable.
    yaml = None


ROOT = Path(__file__).resolve().parents[1]
HERMES_ROOT = ROOT / "vendor" / "hermes-agent"
OPENCODE_ROOT = ROOT / "vendor" / "opencode"
RUNTIME = ROOT / "runtime"
MODEL_CONFIGS = (ROOT / "yteam.local.yaml", RUNTIME / "yteam-model.yaml", RUNTIME / "yteam-model.local.yaml")
PORT = os.environ.get("HERMES_BRIDGE_PORT", "8642")
HOST = "127.0.0.1"
NATIVE_COMMANDS = {
    "acp", "agent", "attach", "console", "db", "debug", "export", "generate",
    "github", "login", "logout", "mcp", "models", "open", "plugin", "pr",
    "providers", "run", "serve", "session", "stats", "switch", "uninstall",
    "upgrade", "web",
}


def find_python() -> str:
    configured = os.environ.get("HERMES_PYTHON")
    if configured:
        return configured
    candidates = [
        HERMES_ROOT / ".venv" / ("Scripts/python.exe" if os.name == "nt" else "bin/python"),
        HERMES_ROOT / "venv" / ("Scripts/python.exe" if os.name == "nt" else "bin/python"),
    ]
    for candidate in candidates:
        if candidate.exists():
            return str(candidate)
    return sys.executable


def find_bun() -> str:
    configured = os.environ.get("BUN_BIN")
    if configured:
        return configured
    found = shutil.which("bun")
    if found:
        return found
    raise RuntimeError("Bun was not found. Install Bun or set BUN_BIN to its executable path.")


def wait_for_health(url: str, key: str, process: subprocess.Popen[bytes], timeout: float = 30.0) -> None:
    deadline = time.monotonic() + timeout
    request = urllib.request.Request(url, headers={"Authorization": f"Bearer {key}"})
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError(f"Hermes gateway exited early with code {process.returncode}.")
        try:
            with urllib.request.urlopen(request, timeout=2) as response:
                if response.status == 200:
                    return
        except (urllib.error.URLError, TimeoutError):
            time.sleep(0.25)
    raise TimeoutError(f"Hermes API server did not become ready at {url}.")


def load_model_config(path: Path | None = None) -> dict[str, str]:
    """Load the one-file local model config without returning it to stdout."""
    selected = path
    if selected is None:
        selected = next((candidate for candidate in MODEL_CONFIGS if candidate.exists()), None)
    if selected is None:
        return {}
    if yaml is None:
        raise RuntimeError("PyYAML is required to read yteam.local.yaml; install Hermes dependencies first.")
    try:
        data = yaml.safe_load(selected.read_text(encoding="utf-8")) or {}
    except (OSError, ValueError, yaml.YAMLError) as error:
        raise RuntimeError(f"Invalid model config {selected}: {error}") from error
    if not isinstance(data, dict):
        raise RuntimeError(f"Model config must be a YAML object: {selected}")
    result: dict[str, str] = {}
    for key in ("provider", "model", "api_key", "base_url"):
        value = data.get(key)
        if value is not None:
            result[key] = str(value).strip()
    if not result.get("provider") or not result.get("model"):
        raise RuntimeError("yteam.local.yaml requires non-empty provider and model fields.")
    if not result.get("api_key"):
        raise RuntimeError("yteam.local.yaml requires a local api_key value.")
    return result


def apply_model_config(env: dict[str, str], config: dict[str, str]) -> None:
    """Map the convenience file to Hermes-compatible child-process settings."""
    provider = config["provider"].strip().lower()
    env["HERMES_MODEL"] = config["model"]
    if provider == "openrouter":
        env["OPENROUTER_API_KEY"] = config["api_key"]
    elif provider == "anthropic":
        env["ANTHROPIC_API_KEY"] = config["api_key"]
    else:
        env["OPENAI_API_KEY"] = config["api_key"]


def apply_model_profile(home: Path, config: dict[str, str]) -> None:
    """Persist only non-secret model routing in the active Hermes config."""
    if yaml is None:
        raise RuntimeError("PyYAML is required to update the active Hermes model profile.")
    path = home / "config.yaml"
    try:
        data = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    except (OSError, ValueError, yaml.YAMLError) as error:
        raise RuntimeError(f"Invalid Hermes profile config {path}: {error}") from error
    if not isinstance(data, dict):
        raise RuntimeError(f"Hermes profile config must be a YAML object: {path}")
    model = data.get("model") if isinstance(data.get("model"), dict) else {}
    model["provider"] = config["provider"]
    model["default"] = config["model"]
    if config.get("base_url"):
        model["base_url"] = config["base_url"]
    data["model"] = model
    # Never copy api_key into a persisted config file.
    path.write_text(yaml.safe_dump(data, sort_keys=False), encoding="utf-8")


def main() -> int:
    if not HERMES_ROOT.exists() or not OPENCODE_ROOT.exists():
        print("Missing upstream checkout. Run git clone for vendor/opencode and vendor/hermes-agent first.", file=sys.stderr)
        return 2
    RUNTIME.mkdir(exist_ok=True)
    bun = find_bun()
    opencode_entry = str(OPENCODE_ROOT / "packages" / "opencode" / "src" / "index.ts")
    requested = sys.argv[1:]
    first = requested[0] if requested else ""
    if first in NATIVE_COMMANDS or first in {"-h", "--help", "-v", "--version", "completion"}:
        return subprocess.run([bun, "run", "--conditions=browser", opencode_entry, *requested], cwd=ROOT, env=os.environ.copy()).returncode

    from init_yteam import initialize

    hermes_home = initialize()
    model_config = load_model_config()
    key = token_urlsafe(32)
    env = os.environ.copy()
    env.update(
        {
            "API_SERVER_ENABLED": "true",
            "API_SERVER_KEY": key,
            "API_SERVER_HOST": HOST,
            "API_SERVER_PORT": PORT,
            "YTEAM_BRIDGE_KEY": key,
            "HERMES_HOME": str(hermes_home),
            "YTEAM_PROJECT_ROOT": str(ROOT),
            "YTEAM_WORKSPACE_ROOT": str(ROOT.parent),
            "YTEAM_INTEL_DIR": str(hermes_home / "intelligence"),
            "PYTHONIOENCODING": "utf-8",
        }
    )
    if model_config:
        apply_model_profile(hermes_home, model_config)
        apply_model_config(env, model_config)
    log_path = RUNTIME / "hermes-gateway.log"
    log_handle = log_path.open("ab")
    gateway = subprocess.Popen(
        [find_python(), str(HERMES_ROOT / "hermes"), "gateway", "run", "--force", "--no-supervise"],
        # Hermes resolves the agent's default terminal/context directory from
        # the process working directory. Keep source imports absolute while
        # giving the security workbench its own project as the active cwd.
        cwd=ROOT,
        env=env,
        stdout=log_handle,
        stderr=log_handle,
    )
    try:
        wait_for_health(f"http://{HOST}:{PORT}/health", key, gateway)
        opencode_args = sys.argv[1:]
        tui = subprocess.Popen(
            [
                bun,
                "run",
                "--conditions=browser",
                opencode_entry,
                str(ROOT),
                *opencode_args,
            ],
            cwd=ROOT,
            env=env,
        )
        return tui.wait()
    except KeyboardInterrupt:
        return 130
    except (RuntimeError, TimeoutError) as error:
        print(f"Yteam startup failed: {error}", file=sys.stderr)
        print(f"See {log_path} for Hermes gateway output.", file=sys.stderr)
        return 2
    finally:
        if "tui" in locals() and tui.poll() is None:
            tui.send_signal(signal.SIGINT)
            try:
                tui.wait(timeout=5)
            except subprocess.TimeoutExpired:
                tui.terminate()
        if gateway.poll() is None:
            gateway.terminate()
            try:
                gateway.wait(timeout=5)
            except subprocess.TimeoutExpired:
                gateway.kill()
        log_handle.close()


if __name__ == "__main__":
    raise SystemExit(main())
