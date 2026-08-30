#!/usr/bin/env python3
"""Small native JSONL session store for YTEAM conversations."""

from __future__ import annotations

import secrets
from pathlib import Path

from yteam_state import StateStore


class Session:
    def __init__(self, root: Path, session_id: str | None = None, state_path: Path | None = None) -> None:
        self.root = root
        self.root.mkdir(parents=True, exist_ok=True)
        self.session_id = session_id or f"yteam_{secrets.token_urlsafe(12)}"
        # All sessions in one runtime share the runtime-level WAL database.
        # Keeping the facade path as ``runtime/sessions`` preserves the public
        # layout while making session state visible to the worker and control
        # plane after the TUI exits.
        self.store = StateStore(state_path or (self.root / "state.db"))
        self.store.ensure_session(self.session_id)
        self.messages: list[dict[str, str]] = self.store.messages(self.session_id)

    @classmethod
    def resume_or_new(cls, root: Path) -> "Session":
        store = StateStore(root.parent / "state.db")
        return cls(root, store.latest_session_id(), root.parent / "state.db")

    def append(self, role: str, content: str) -> None:
        self.store.append_message(self.session_id, role, content)
        self.messages.append({"role": role, "content": content})

    def conversation(self, limit: int = 40) -> list[dict[str, str]]:
        self.messages = self.store.messages(self.session_id, limit=200)
        return self.messages[-max(1, min(limit, 200)):]
