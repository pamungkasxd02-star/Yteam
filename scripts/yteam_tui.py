#!/usr/bin/env python3
"""OpenCode-style native YTEAM terminal UI.

The UI is an original implementation of the interaction model shown in modern
agent consoles: an onboarding screen before the first turn, a full workspace
after the first turn, a composer at the bottom, and a persistent right-hand
status rail.  ``prompt_toolkit`` owns the full-screen application and input;
there is no competing line reader or terminal clear loop.

Keyboard controls
-----------------
Enter        submit the composer
Shift+Enter  insert a newline (Ctrl+J is an alternative)
Escape       interrupt the active model stream / close the palette
Ctrl+P       toggle the command palette
PageUp/Down  scroll the transcript
Ctrl+C       quit
"""

from __future__ import annotations

import argparse
import sys
import threading
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from yteam_models import DEFAULT_MODEL_CONFIG, discover_free_models
from yteam_runtime import YteamRuntime


ROOT = Path(__file__).resolve().parents[1]
COMMANDS = (
    "/help", "/models", "/model", "/status", "/history", "/clear", "/memory",
    "/events", "/jobs", "/skills", "/engine", "/plan", "/ctx", "/learn",
    "/verify", "/bb", "/auto", "/agents", "/approvals", "/approve", "/deny", "/cancel", "/doctor", "/quit",
)


@dataclass
class UIState:
    """Small thread-safe state projection used only by the renderer."""

    transcript: list[tuple[str, str]] = field(default_factory=list)
    activity: list[str] = field(default_factory=list)
    workspace: bool = False
    busy: bool = False
    interrupted: bool = False
    palette: bool = False
    scroll: int = 0
    started_at: float = field(default_factory=time.time)
    _lock: threading.RLock = field(default_factory=threading.RLock, repr=False)

    def add(self, role: str, content: str) -> None:
        with self._lock:
            self.transcript.append((role, content))
            self.workspace = True

    def update_last(self, content: str) -> None:
        with self._lock:
            if self.transcript and self.transcript[-1][0] == "assistant":
                self.transcript[-1] = ("assistant", content)

    def snapshot(self) -> dict[str, Any]:
        with self._lock:
            return {
                "transcript": list(self.transcript),
                "activity": list(self.activity),
                "workspace": self.workspace,
                "busy": self.busy,
                "interrupted": self.interrupted,
                "palette": self.palette,
                "scroll": self.scroll,
            }


class CommandCompleter:
    """Dependency-light completer for the slash command palette."""

    def get_completions(self, document, complete_event):
        from prompt_toolkit.completion import Completion

        word = document.get_word_before_cursor()
        if not word.startswith("/"):
            return
        for command in COMMANDS:
            if command.startswith(word):
                yield Completion(command, start_position=-len(word), display=command)


class OpenCodeUI:
    """Full-screen prompt_toolkit application matching the requested layout."""

    def __init__(self, runtime: YteamRuntime) -> None:
        self.runtime = runtime
        self.state = UIState()
        self.cancel_event = threading.Event()
        self.app = None
        self.buffer = None
        self._build_application()
        runtime.events.subscribe(self._on_event)

    # -- state/event plumbing -------------------------------------------
    def _on_event(self, event: dict[str, object]) -> None:
        kind = str(event.get("kind", "event"))
        detail = str(event.get("detail", ""))[:90]
        with self.state._lock:
            self.state.activity.append(f"{kind}: {detail}")
            self.state.activity = self.state.activity[-20:]
        if self.app:
            self.app.invalidate()

    def _snapshot(self) -> dict[str, Any]:
        result = self.state.snapshot()
        result["runtime"] = self.runtime.snapshot()
        return result

    # -- prompt_toolkit construction ------------------------------------
    def _build_application(self) -> None:
        from prompt_toolkit.application import Application
        from prompt_toolkit.buffer import Buffer
        from prompt_toolkit.filters import Condition
        from prompt_toolkit.key_binding import KeyBindings
        from prompt_toolkit.layout import Dimension, HSplit, Layout, VSplit, Window
        from prompt_toolkit.layout.containers import ConditionalContainer, Float, FloatContainer
        from prompt_toolkit.layout.controls import BufferControl, FormattedTextControl
        from prompt_toolkit.layout.processors import BeforeInput
        from prompt_toolkit.history import InMemoryHistory
        from prompt_toolkit.styles import Style

        self._Dimension = Dimension
        self._Window = Window
        self._FormattedTextControl = FormattedTextControl
        self._BufferControl = BufferControl
        self._HSplit = HSplit
        self._VSplit = VSplit
        self._VSplit = VSplit
        self._ConditionalContainer = ConditionalContainer
        self._Condition = Condition

        self.buffer = Buffer(
            multiline=True,
            history=InMemoryHistory(),
            completer=CommandCompleter(),
            complete_while_typing=True,
        )
        self.buffer.on_text_changed += self._on_buffer_changed
        def make_input_control() -> BufferControl:
            return BufferControl(
                buffer=self.buffer,
                include_default_input_processors=True,
                input_processors=[BeforeInput('Ask anything...  "Fix broken tests"', style="class:muted")],
                lexer=None,
            )

        onboarding_input = make_input_control()
        workspace_input = make_input_control()
        self._onboarding_input = onboarding_input
        self._workspace_input = workspace_input

        bindings = KeyBindings()

        @bindings.add("enter")
        def _(event) -> None:
            self._submit()

        @bindings.add("c-j")
        def _(event) -> None:
            event.current_buffer.insert_text("\n")

        @bindings.add("escape")
        def _(event) -> None:
            if self.state.snapshot()["palette"]:
                self.state.palette = False
                event.app.invalidate()
            elif self.state.snapshot()["busy"]:
                self.cancel_event.set()
                with self.state._lock:
                    self.state.interrupted = True
                event.app.invalidate()

        @bindings.add("c-p")
        def _(event) -> None:
            with self.state._lock:
                self.state.palette = not self.state.palette
            event.app.invalidate()

        @bindings.add("pageup")
        def _(event) -> None:
            with self.state._lock:
                self.state.scroll += 8
            event.app.invalidate()

        @bindings.add("pagedown")
        def _(event) -> None:
            with self.state._lock:
                self.state.scroll = max(0, self.state.scroll - 8)
            event.app.invalidate()

        @bindings.add("c-c")
        def _(event) -> None:
            self.state.interrupted = True
            event.app.exit()

        onboarding_composer = self._composer(onboarding_input)
        workspace_composer = self._composer(workspace_input)
        onboarding_palette = self._palette_window(Window, FormattedTextControl, Dimension)
        workspace_palette = self._palette_window(Window, FormattedTextControl, Dimension)
        palette = ConditionalContainer(workspace_palette, Condition(lambda: self.state.snapshot()["palette"]))

        onboarding = HSplit([
            Window(FormattedTextControl(self._logo_text), height=Dimension(min=5, max=8), align="CENTER"),
            Window(height=1),
            self._center(onboarding_composer),
            Window(FormattedTextControl(self._onboarding_hints), height=2, align="CENTER"),
            Window(height=1),
            Window(FormattedTextControl(self._tip_text), height=2, align="CENTER"),
            Window(height=Dimension(weight=1)),
            Window(FormattedTextControl(self._onboarding_status), height=2),
        ])

        workspace_left = HSplit([
            Window(FormattedTextControl(self._workspace_header), height=1, style="class:header"),
            Window(FormattedTextControl(self._transcript_text), style="class:main", wrap_lines=True),
            palette,
            workspace_composer,
            Window(FormattedTextControl(self._workspace_footer), height=2, style="class:footer"),
        ])
        workspace = VSplit([
            workspace_left,
            Window(FormattedTextControl(self._sidebar_text), width=Dimension(weight=1), style="class:sidebar", wrap_lines=True),
        ])
        workspace_left.width = Dimension(weight=2)

        onboarding_container = ConditionalContainer(onboarding, Condition(lambda: not self.state.snapshot()["workspace"]))
        workspace_container = ConditionalContainer(workspace, Condition(lambda: self.state.snapshot()["workspace"]))
        root = FloatContainer(
            HSplit([onboarding_container, workspace_container]),
            floats=[Float(ConditionalContainer(onboarding_palette, Condition(lambda: not self.state.snapshot()["workspace"])), top=3, left=8, right=36)],
        )
        style = Style.from_dict({
            "root": "bg:#0b0b0b #d4d4d4",
            "header": "bg:#0b0b0b #858585",
            "main": "bg:#0b0b0b #d4d4d4",
            "sidebar": "bg:#151515 #a0a0a0",
            "footer": "bg:#0b0b0b #888888",
            "composer": "bg:#202020 #dddddd",
            "composer-model": "bg:#202020 #888888",
            "accent": "#58a6ff",
            "muted": "#777777",
            "dim": "#646464",
            "user": "#e6e6e6",
            "assistant": "#d0d0d0",
            "heading": "bold #eeeeee",
            "success": "#78dba9",
            "warning": "#e6a84b",
            "palette": "bg:#282828 #dddddd",
        })
        application_options = {
            "layout": Layout(root, focused_element=onboarding_input),
            "key_bindings": bindings,
            "style": style,
            "full_screen": True,
            "mouse_support": False,
            "erase_when_done": True,
        }
        # The worker/test harness can construct the UI without a real Windows
        # console. The real interactive path uses the native Win32/ANSI output.
        if not sys.stdin.isatty() or not sys.stdout.isatty():
            from prompt_toolkit.output import DummyOutput

            application_options["output"] = DummyOutput()
        self.app = Application(**application_options)

    def _on_buffer_changed(self, event) -> None:
        text = event.buffer.text
        if text.startswith("/"):
            with self.state._lock:
                self.state.palette = True
        elif not text:
            with self.state._lock:
                self.state.palette = False
        if self.app:
            self.app.invalidate()

    def _center(self, container):
        from prompt_toolkit.layout import Dimension, VSplit, Window

        return VSplit([
            Window(width=Dimension(weight=1)),
            container,
            Window(width=Dimension(weight=1)),
        ])

    def _composer(self, input_control):
        from prompt_toolkit.layout import Dimension, HSplit, VSplit, Window
        from prompt_toolkit.layout.controls import FormattedTextControl

        return VSplit([
            Window(FormattedTextControl([("class:accent", "▌")]), width=1, style="class:composer"),
            HSplit([
                Window(input_control, height=Dimension(min=3, max=7), style="class:composer", wrap_lines=True),
                Window(FormattedTextControl(self._model_line), height=1, style="class:composer-model"),
            ], style="class:composer"),
        ], height=Dimension(min=5, max=9), style="class:composer")

    def _palette_window(self, window_cls, control_cls, dimension_cls):
        return window_cls(
            control_cls(self._palette_text),
            height=dimension_cls(min=3, max=12),
            style="class:palette",
            wrap_lines=True,
            dont_extend_height=True,
        )

    # -- formatted views -------------------------------------------------
    @staticmethod
    def _fragments(lines: list[tuple[str, str]]) -> list[tuple[str, str]]:
        result: list[tuple[str, str]] = []
        for style, line in lines:
            result.append((style, line if line.endswith("\n") else line + "\n"))
        return result

    def _logo_text(self):
        return self._fragments([
            ("class:dim", "                 ██████  ██████  ███████ ███    ██  ██████  ██████  ██████  ███████"),
            ("class:muted", "                 ██      ██   ██ ██      ████   ██ ██    ██ ██   ██ ██   ██ ██      "),
            ("class:muted", "                 ██      ██████  █████   ██ ██  ██ ██    ██ ██   ██ ██   ██ █████   "),
            ("class:muted", "                 ██      ██   ██ ██      ██  ████ ██    ██ ██   ██ ██   ██ ██      "),
            ("class:heading", "                 ██████  ██   ██ ███████ ██   ███  ██████  ██████  ██████  ███████"),
        ])

    def _model_line(self):
        snapshot = self._snapshot()
        model = snapshot["runtime"].get("model", DEFAULT_MODEL_CONFIG["model"])
        provider = snapshot["runtime"].get("provider", "YTEAM")
        busy = "  thinking…" if snapshot["busy"] else ""
        return self._fragments([
            ("class:accent", "Bb"), ("class:muted", "  auto  ·  "),
            ("class:user", str(model)), ("class:muted", f"  {provider}{busy}"),
        ])

    def _onboarding_hints(self):
        return self._fragments([("class:muted", "tab agents    ctrl+p commands")])

    def _tip_text(self):
        return self._fragments([("class:warning", "  • Tip  "), ("class:muted", "Configure YTEAM policy, MCP, and authorized targets before running /bb")])

    def _onboarding_status(self):
        snap = self._snapshot()
        return self._fragments([
            ("class:muted", f" {ROOT}"),
            ("class:success", "                                      ◉ 1 MCP "),
            ("class:muted", "/status"),
            ("class:muted", "                                      YTEAM 0.2.0"),
        ])

    def _workspace_header(self):
        snap = self._snapshot()
        label = "⏺ running" if snap["busy"] else "◉ ready"
        return self._fragments([("class:muted", f" {label}   {ROOT}   ·   {snap['runtime'].get('model', '')}")])

    def _transcript_text(self):
        snap = self._snapshot()
        rows: list[tuple[str, str]] = [("class:dim", "")]
        for role, content in snap["transcript"]:
            if role == "user":
                rows.append(("class:accent", "▌ "))
                rows.append(("class:user", content))
                rows.append(("class:dim", "\n"))
            elif role == "assistant":
                rows.append(("class:assistant", content or ("thinking…" if snap["busy"] else "")))
                rows.append(("class:dim", "\n\n"))
            else:
                rows.append(("class:muted", f"{role}: {content}\n"))
        if not snap["transcript"]:
            rows.extend([
                ("class:heading", "\n  New session\n"),
                ("class:muted", "  Ask YTEAM to inspect code, explain a design, or plan an authorized task.\n"),
            ])
        if snap["scroll"]:
            rows.append(("class:dim", f"\n  ↑ scrolled {snap['scroll']} lines — PageDown to return\n"))
        return self._fragments(rows)

    def _sidebar_text(self):
        snap = self._snapshot()
        runtime = snap["runtime"]
        try:
            from yteam_engine.context_guard import ContextGuard

            messages = self.runtime.session.messages
            guard = ContextGuard()
            tokens = guard.estimate(messages)
            ratio = guard.ratio(messages)
        except Exception:  # noqa: BLE001
            tokens, ratio = 0, 0.0
        memory = runtime.get("memory", {})
        agents = runtime.get("agents", {})
        latest = agents.get("latest", {}) if isinstance(agents, dict) else {}
        lines = [
            ("class:heading", " YTEAM Security Agent\n"),
            ("class:muted", f" Session {str(runtime.get('session_id', ''))[:20]}\n\n"),
            ("class:heading", " Context\n"),
            ("class:muted", f" {tokens:,} tokens\n {ratio * 100:.0f}% used\n $0.00 spent\n\n"),
            ("class:heading", " MCP\n"),
            ("class:success", " • "), ("class:user", "yteam "), ("class:muted", "Connected\n\n"),
            ("class:heading", " Memory\n"),
            ("class:muted", f" {memory.get('verified', 0)} verified  ·  {memory.get('proposals', 0)} pending\n\n"),
            ("class:heading", " Autonomous Agent\n"),
            ("class:success" if latest.get("status") in {"running", "completed"} else "class:warning", " • "),
            ("class:user", f"{latest.get('status', 'idle')} "),
            ("class:muted", f"r{latest.get('round', 0)} · g{latest.get('generation', 0)} · {latest.get('pending', 0)} pending\n"),
            ("class:muted", f" {agents.get('active_jobs', 0)} jobs · {agents.get('pending_approvals', 0)} approvals\n\n"),
            ("class:muted", f" {ROOT}\n\n"),
            ("class:success", " • "), ("class:user", "YTEAM "), ("class:muted", "0.2.0\n"),
        ]
        return self._fragments(lines)

    def _workspace_footer(self):
        snap = self._snapshot()
        runtime = snap["runtime"]
        try:
            from yteam_engine.context_guard import ContextGuard

            ratio = ContextGuard().ratio(self.runtime.session.messages)
        except Exception:  # noqa: BLE001
            ratio = 0.0
        left = "▣  esc interrupt" if snap["busy"] else "▣  ready"
        right = f"{ratio * 100:.0f}% context   ctrl+p commands"
        return self._fragments([("class:accent", f" {left}"), ("class:muted", f"{' ' * 40}{right}")])

    def _palette_text(self):
        text = self.buffer.text if self.buffer else "/"
        query = text.lower().strip()
        rows = [("class:heading", " Commands\n")]
        for command in COMMANDS:
            if not query or query == "/" or command.startswith(query):
                rows.append(("class:user", f"  {command}\n"))
        return self._fragments(rows[:10])

    # -- command/model execution ----------------------------------------
    def _submit(self) -> None:
        text = self.buffer.text.strip()
        if not text or self.state.snapshot()["busy"]:
            return
        self.buffer.reset()
        with self.state._lock:
            self.state.workspace = True
            self.state.busy = True
            self.state.interrupted = False
            self.state.scroll = 0
        self.cancel_event.clear()
        self.app.layout.focus(self._workspace_input)
        self.state.add("user", text)
        thread = threading.Thread(target=self._execute, args=(text,), daemon=True)
        thread.start()
        self.app.invalidate()

    def _execute(self, text: str) -> None:
        try:
            if text in {"/quit", "/exit", "/q"}:
                self._write_quit_marker()
                self.runtime.quit_requested = True
                return
            if text.startswith("/"):
                result = self.runtime.command(text)
                self.state.add("assistant", result or "")
                if text.startswith("/bb "):
                    try:
                        from yteam_worker import ensure_worker

                        ensure_worker(ROOT)
                    except Exception:  # noqa: BLE001
                        pass
                return
            self.state.add("assistant", "")
            chunks: list[str] = []
            for chunk in self.runtime.answer_stream(text):
                if self.cancel_event.is_set():
                    break
                chunks.append(chunk)
                self.state.update_last("".join(chunks))
                self.app.invalidate()
        except RuntimeError as error:
            self.state.update_last(f"Model error: {error}")
        finally:
            with self.state._lock:
                self.state.busy = False
            self.app.invalidate()
            if self.runtime.quit_requested:
                self.app.exit()

    def run(self) -> int:
        self.app.run()
        return 0

    def _write_quit_marker(self) -> None:
        """Signal the auto-restart launcher to stop instead of relaunching."""
        try:
            (ROOT / "runtime").mkdir(parents=True, exist_ok=True)
            (ROOT / "runtime" / "quit.marker").write_text("user quit", encoding="utf-8")
        except OSError:
            pass

    def resume_handoff(self, path: Path) -> None:
        """Display a bounded handoff note without copying raw evidence."""
        try:
            text = path.read_text(encoding="utf-8", errors="replace")
        except OSError as error:
            self.state.add("assistant", f"Handoff could not be opened: {error}")
            return
        self.state.add("assistant", f"Resumed from handoff: {path}\n{text[:6000]}")


def run_plain(runtime: YteamRuntime) -> int:
    """Line-oriented fallback for pipes, CI, and terminals without prompt_toolkit."""
    quit_marker = ROOT / "runtime" / "quit.marker"
    while True:
        try:
            message = input("YTEAM> ").strip()
        except (EOFError, KeyboardInterrupt):
            break
        if not message or message in {"/quit", "/exit", "/q"}:
            try:
                quit_marker.parent.mkdir(parents=True, exist_ok=True)
                quit_marker.write_text("user quit", encoding="utf-8")
            except OSError:
                pass
            break
        result = runtime.command(message)
        if result is not None:
            print(result)
        else:
            try:
                print(runtime.answer(message))
            except RuntimeError as error:
                print(f"Model error: {error}")
    return 0


def print_models() -> int:
    print("YTEAM Zen Free models (direct API):")
    for model in discover_free_models():
        print(f"  {'*' if model == DEFAULT_MODEL_CONFIG['model'] else ' '} {model}")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--models", action="store_true", help="list Zen Free models and exit")
    parser.add_argument("--plain", action="store_true", help="use line-oriented fallback")
    parser.add_argument("--handoff", type=Path, help="display a previous YTEAM context handoff while resuming the durable session")
    args = parser.parse_args()
    if args.models:
        return print_models()
    runtime = YteamRuntime(ROOT)
    if args.plain or not sys.stdin.isatty():
        return run_plain(runtime)
    try:
        import prompt_toolkit  # noqa: F401
    except ImportError:
        print("prompt_toolkit missing; using --plain fallback", file=sys.stderr)
        return run_plain(runtime)
    ui = OpenCodeUI(runtime)
    if args.handoff:
        ui.resume_handoff(args.handoff)
    return ui.run()


if __name__ == "__main__":
    raise SystemExit(main())
