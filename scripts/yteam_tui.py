#!/usr/bin/env python3
"""Full-screen native YTEAM terminal UI.

The renderer is dependency-free but follows a modern agent-console model:
transcript in the center, activity/events on the side, and a persistent status
bar. The runtime remains usable over SSH, pipes, and remote control adapters.
"""

from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys
import textwrap
from collections import deque
from pathlib import Path

from yteam_models import DEFAULT_MODEL_CONFIG, discover_free_models
from yteam_runtime import YteamRuntime


ROOT = Path(__file__).resolve().parents[1]


class TerminalUI:
    """Small OpenCode-like dashboard implemented with ANSI primitives."""

    def __init__(self, runtime: YteamRuntime) -> None:
        self.runtime = runtime
        self.transcript: deque[tuple[str, str]] = deque(maxlen=80)
        self.activity: deque[str] = deque(maxlen=18)
        self.running = True
        runtime.events.subscribe(self.on_event)

    def on_event(self, event: dict[str, object]) -> None:
        kind = str(event.get("kind", "event"))
        detail = str(event.get("detail", ""))[:90]
        self.activity.append(f"{kind}: {detail}")

    def _width(self) -> int:
        return max(80, shutil.get_terminal_size((120, 32)).columns)

    def _height(self) -> int:
        return max(24, shutil.get_terminal_size((120, 32)).lines)

    def _panel(self, title: str, lines: list[str], width: int) -> list[str]:
        inner = max(4, width - 4)
        result = [f"┌─ {title} " + "─" * max(0, width - len(title) - 5) + "┐"]
        for line in lines:
            for wrapped in textwrap.wrap(line, width=inner) or [""]:
                result.append("│ " + wrapped.ljust(inner) + " │")
        result.append("└" + "─" * (width - 2) + "┘")
        return result

    def render(self, prompt: str = "") -> None:
        width = self._width()
        height = self._height()
        snapshot = self.runtime.snapshot()
        rail_width = min(30, max(24, width // 4))
        content_width = width - rail_width - 1
        activity_width = min(34, max(26, content_width // 3))
        transcript_width = content_width - activity_width - 1
        rail = [
            "YTEAM",
            "native security workbench",
            "",
            f"model  {snapshot['model']}",
            f"provider {snapshot['provider']}",
            f"session {str(snapshot['session_id'])[:20]}",
            f"messages {snapshot['message_count']}",
            f"events {snapshot['event_count']}",
            "",
            "POLICY",
            "authorized-only  ON",
            "read-only        ON",
            "no destructive   ON",
            "no auto-submit   ON",
            "",
            "MEMORY",
            f"verified {snapshot['memory']['verified']}",
            f"proposals {snapshot['memory']['proposals']}",
            "",
            "Ctrl+C exits",
            "/help commands",
        ]
        transcript: list[str] = []
        for role, content in self.transcript:
            label = "YOU" if role == "user" else "YTEAM"
            transcript.append(f"{label}> {content}")
        if not transcript:
            transcript = ["Welcome to YTEAM.", "Direct model stream, durable sessions, replayable events.", "Type /help to see commands."]
        activity = list(self.activity) or ["waiting for activity"]
        left = self._panel("WORKSPACE", rail, rail_width)
        right = self._panel("ACTIVITY", activity[-max(4, height - 8):], activity_width)
        center = self._panel("SESSION TRANSCRIPT", transcript[-max(4, height - 8):], transcript_width)
        body_height = max(len(center), len(right))
        center += ["│" + " " * (transcript_width - 2) + "│"] * (body_height - len(center))
        right += ["│" + " " * (activity_width - 2) + "│"] * (body_height - len(right))
        lines = ["\x1b[H\x1b[2J\x1b[?25l", " YTEAM  •  OpenCode-style native console  •  F1 help  •  /quit exit", ""]
        for index in range(max(len(left), body_height)):
            left_line = left[index] if index < len(left) else " " * rail_width
            if index < len(center):
                main_line = center[index]
            else:
                main_line = right[index] if index < len(right) else ""
            if index < len(right):
                main_line = center[index] + " " + right[index]
            lines.append(left_line.ljust(rail_width) + " " + main_line)
        lines.extend(["", "─" * width, f" {prompt or 'message or /command'}", f" model={snapshot['model']}  session={snapshot['session_id']}  memory={snapshot['memory']['verified']} verified"])
        sys.stdout.write("\n".join(lines) + "\x1b[?25h\n")
        sys.stdout.flush()

    def run_bb(self, target: str) -> None:
        command = [sys.executable, str(ROOT / "scripts" / "yteam_run.py"), target, "--camoufox"]
        self.activity.append(f"assessment started: {target}")
        result = subprocess.run(command, cwd=ROOT, check=False)
        if result.returncode:
            self.activity.append(f"assessment exited with code {result.returncode}")

    def loop(self) -> int:
        while self.running:
            self.render()
            try:
                message = input("\x1b[1;36m YTEAM> \x1b[0m").strip()
            except (EOFError, KeyboardInterrupt):
                self.running = False
                break
            if not message:
                continue
            if message in {"/exit", "/q"}:
                self.running = False
                break
            result = self.runtime.command(message)
            if result is not None:
                if self.runtime.pending_bb_target:
                    target = self.runtime.pending_bb_target
                    self.runtime.pending_bb_target = None
                    self.transcript.append(("assistant", result))
                    self.render()
                    self.run_bb(target)
                else:
                    self.transcript.append(("assistant", result))
                if self.runtime.quit_requested:
                    self.running = False
                continue
            self.transcript.append(("user", message))
            self.render(message)
            response: list[str] = []
            try:
                for chunk in self.runtime.answer_stream(message):
                    response.append(chunk)
                    sys.stdout.write(chunk)
                    sys.stdout.flush()
                self.transcript.append(("assistant", "".join(response)))
            except RuntimeError as error:
                self.transcript.append(("assistant", f"Model error: {error}"))
        sys.stdout.write("\x1b[?25h\x1b[0m\n")
        return 0


def print_models(models: list[str], selected: str) -> None:
    print("\nYTEAM Zen Free models (direct API):")
    for model in models:
        print(f"{' *' if model == selected else '  '} {model}")
    print("Use /model <model-id> to switch. Use /help for commands.\n")


def run_tui() -> int:
    runtime = YteamRuntime(ROOT)
    return TerminalUI(runtime).loop()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--models", action="store_true", help="list Zen Free models and exit")
    parser.add_argument("--plain", action="store_true", help="use the line-oriented UI")
    args = parser.parse_args()
    if args.models:
        print_models(discover_free_models(), DEFAULT_MODEL_CONFIG["model"])
        return 0
    return run_tui()


if __name__ == "__main__":
    raise SystemExit(main())
