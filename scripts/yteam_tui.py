#!/usr/bin/env python3
"""Native YTEAM terminal UI backed directly by the configured model API."""

from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path

from yteam_models import DEFAULT_MODEL_CONFIG, discover_free_models
from yteam_runtime import YteamRuntime


ROOT = Path(__file__).resolve().parents[1]
def print_models(models: list[str], selected: str) -> None:
    print("\nYTEAM Zen Free models (direct API):")
    for model in models:
        marker = " *" if model == selected else "  "
        print(f"{marker} {model}")
    print("Use /model <model-id> to switch. Use /help for commands.\n")


def run_bb(target: str) -> None:
    command = [sys.executable, str(ROOT / "scripts" / "yteam_run.py"), target, "--camoufox"]
    result = subprocess.run(command, cwd=ROOT, check=False)
    if result.returncode:
        print(f"YTEAM /bb exited with code {result.returncode}.", file=sys.stderr)


def run_tui() -> int:
    runtime = YteamRuntime(ROOT)
    print("YTEAM — native security workbench")
    print(f"Model: {runtime.selected_model} ({runtime.config['provider']})")
    print("Type /help, /models, /model <id>, /bb <authorized-target>, or /quit.")
    while True:
        try:
            message = input("\nYou> ").strip()
        except (EOFError, KeyboardInterrupt):
            print()
            return 0
        if not message:
            continue
        if message in {"/exit", "/q"}:
            return 0
        command_result = runtime.command(message)
        if command_result is not None:
            print(command_result)
            if runtime.pending_bb_target:
                target = runtime.pending_bb_target
                runtime.pending_bb_target = None
                run_bb(target)
            if runtime.quit_requested:
                return 0
            continue
        try:
            print("\nYTEAM> ", end="", flush=True)
            print(runtime.answer(message))
        except RuntimeError as error:
            print(f"\nModel error: {error}", file=sys.stderr)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--models", action="store_true", help="list Zen Free models and exit")
    args = parser.parse_args()
    if args.models:
        models = discover_free_models()
        print_models(models, DEFAULT_MODEL_CONFIG["model"])
        return 0
    return run_tui()


if __name__ == "__main__":
    raise SystemExit(main())
