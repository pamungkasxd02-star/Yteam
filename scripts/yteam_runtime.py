#!/usr/bin/env python3
"""Standalone YTEAM runtime services used by the native terminal UI.

The runtime deliberately owns policy, command routing, model selection, session
state, and an append-only event ledger.  It does not import or shell out to an
upstream agent runtime.
"""

from __future__ import annotations

import json
import re
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Callable

from yteam_ai import stream_chat
from yteam_models import discover_free_models, load_model_config
from yteam_session import Session


@dataclass
class RuntimePolicy:
    authorized_only: bool = True
    read_only: bool = True
    max_requests_per_second: float = 1.0
    no_customer_objects: bool = True
    no_destructive_actions: bool = True
    no_auto_submit: bool = True


@dataclass
class RuntimeEvent:
    kind: str
    detail: str
    at: float = field(default_factory=time.time)


class EventLedger:
    def __init__(self, path: Path) -> None:
        self.path = path
        self.path.parent.mkdir(parents=True, exist_ok=True)

    def emit(self, kind: str, detail: str) -> None:
        record = RuntimeEvent(kind, detail)
        with self.path.open("a", encoding="utf-8") as handle:
            handle.write(json.dumps(record.__dict__, ensure_ascii=False) + "\n")


class YteamRuntime:
    """Own model selection, conversation state, commands, and policy display."""

    def __init__(self, root: Path) -> None:
        self.root = root
        self.runtime_dir = root / "runtime"
        self.config = load_model_config(root)
        self.models = discover_free_models()
        self.selected_model = self.config["model"] if self.config["model"] in self.models else self.models[0]
        self.session = Session(self.runtime_dir / "sessions")
        self.events = EventLedger(self.runtime_dir / "events.jsonl")
        self.policy = RuntimePolicy()
        self.pending_bb_target: str | None = None
        self.quit_requested = False
        self.commands: dict[str, Callable[[str], str]] = {
            "/help": lambda _: self.help_text(),
            "/models": lambda _: self.models_text(),
            "/status": lambda _: self.status_text(),
            "/history": lambda _: self.history_text(),
            "/clear": lambda _: self.clear_text(),
        }

    def help_text(self) -> str:
        return "\n".join([
            "/models                  list the live OpenCode Zen Free catalog",
            "/model <model-id>        select a free model for the next turn",
            "/status                  show runtime, policy, and session state",
            "/history                 show the local conversation summary",
            "/clear                   start a fresh local conversation",
            "/bb <authorized-target> run the scoped read-only security pipeline",
            "/doctor                  run the local dependency diagnostic",
            "/quit                    exit YTEAM",
        ])

    def models_text(self) -> str:
        lines = ["YTEAM Zen Free models (direct OpenAI-compatible API):"]
        lines.extend(f"{' *' if model == self.selected_model else '  '} {model}" for model in self.models)
        lines.append("Use /model <model-id> to switch.")
        return "\n".join(lines)

    def status_text(self) -> str:
        return json.dumps({
            "product": "YTEAM",
            "runtime": "standalone",
            "provider": self.config["provider"],
            "model": self.selected_model,
            "session_id": self.session.session_id,
            "message_count": len(self.session.messages),
            "policy": self.policy.__dict__,
        }, indent=2)

    def history_text(self) -> str:
        if not self.session.messages:
            return "No messages in the current session."
        return "\n".join(f"{item['role']}: {item['content'][:160]}" for item in self.session.messages[-12:])

    def clear_text(self) -> str:
        self.session = Session(self.runtime_dir / "sessions")
        self.events.emit("session.cleared", self.session.session_id)
        return f"Started fresh session: {self.session.session_id}"

    def select_model(self, value: str) -> str:
        requested = value.strip()
        if requested not in self.models:
            return f"Unknown free model: {requested}. Use /models."
        self.selected_model = requested
        self.events.emit("model.selected", requested)
        return f"Active model: {requested} (zen-free)"

    def request_bb(self, value: str) -> str:
        target = value.strip()
        if not target or target.startswith("-") or any(char in target for char in "\r\n"):
            return "Usage: /bb <authorized-http(s)-target>"
        self.pending_bb_target = target
        self.events.emit("bb.requested", target)
        return f"Queued read-only authorized assessment: {target}"

    def command(self, message: str) -> str | None:
        message = message.strip()
        if message in self.commands:
            return self.commands[message](message)
        if message.startswith("/model "):
            return self.select_model(message[7:])
        if message.startswith("/doctor"):
            from yteam_doctor import run
            return json.dumps(run(), indent=2)
        if message == "/quit":
            self.quit_requested = True
            self.events.emit("runtime.quit", self.session.session_id)
            return "Goodbye."
        if message.startswith("/bb") and (message == "/bb" or message.startswith("/bb ")):
            return self.request_bb(message[3:])
        return None

    def answer(self, message: str) -> str:
        self.session.append("user", message)
        self.events.emit("chat.requested", self.selected_model)
        response = "".join(stream_chat({**self.config, "model": self.selected_model}, self.session.conversation()))
        self.session.append("assistant", response)
        self.events.emit("chat.completed", self.selected_model)
        return response
